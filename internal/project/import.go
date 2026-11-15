package project

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"glog/internal/errs"
	"glog/pkg/models"
)

const (
	// importMaxEntries 限制压缩包条目数量。
	importMaxEntries = 100000
	// importMaxTotalUncompressed 限制总解压大小（20 GiB）。
	importMaxTotalUncompressed int64 = 20 << 30
	// importMaxMetaSize 限制 project.meta 大小。
	importMaxMetaSize int64 = 16 << 20
)

// zipEntry 是已通过安全校验的压缩包条目。
type zipEntry struct {
	file  *zip.File
	name  string // 使用 '/' 分隔且已 Clean 的相对路径
	isDir bool
}

// Import 导入 .glogpkg.zip 项目包。
//
// 安全校验：条目数 <= 100000、总解压大小 <= 20GB、拒绝绝对路径/../
// 符号链接/非普通文件/重复路径。解压到 projectsDir 下临时目录后 rename，
// 成功后再注册数据库；任一步失败都会清理临时/目标目录。
func (s *Service) Import(packagePath string) (*models.Project, error) {
	if s == nil || s.store == nil {
		return nil, errs.Internal("项目服务未初始化", nil)
	}
	packagePath = strings.TrimSpace(packagePath)
	if packagePath == "" {
		return nil, errs.InvalidInput("项目包路径不能为空", nil)
	}
	absPackage, err := filepath.Abs(packagePath)
	if err != nil {
		return nil, errs.InvalidInput("项目包路径无效", err)
	}
	info, err := os.Stat(absPackage)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, errs.NotFound("项目包文件不存在", err)
		}
		return nil, errs.Internal("读取项目包失败", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errs.InvalidInput("项目包必须是普通文件", nil)
	}

	zr, err := zip.OpenReader(absPackage)
	if err != nil {
		return nil, errs.InvalidInput("项目包不是有效的 ZIP 文件",
			fmt.Errorf("%w: %v", errs.ErrArchiveInvalid, err))
	}
	defer func() { _ = zr.Close() }()

	if len(zr.File) > importMaxEntries {
		return nil, archiveInvalid("压缩包条目数过多（%d，上限 %d）", len(zr.File), importMaxEntries)
	}

	entries, metaFile, err := validateZipEntries(zr.File)
	if err != nil {
		return nil, err
	}
	if metaFile == nil {
		return nil, archiveInvalid("压缩包根目录缺少 project.meta")
	}

	meta, err := readMetaFromZip(metaFile)
	if err != nil {
		return nil, err
	}
	if err := validateProjectID(meta.ID); err != nil {
		return nil, archiveInvalid("project.meta 中的项目ID无效: %v", err)
	}
	if strings.TrimSpace(meta.Name) == "" {
		return nil, archiveInvalid("project.meta 中的项目名称为空")
	}

	// 数据库已存在同 ID 项目时直接拒绝。
	if _, err := s.store.GetProject(meta.ID); err == nil {
		return nil, projectExistsError(meta.ID)
	} else if !errors.Is(err, errs.ErrNotFound) {
		return nil, errs.Internal("检查项目是否已存在失败", err)
	}

	if err := os.MkdirAll(s.projectsDir, 0o755); err != nil {
		return nil, errs.Internal("创建项目根目录失败", err)
	}
	finalDir := filepath.Join(s.projectsDir, meta.ID)
	if _, err := os.Stat(finalDir); err == nil {
		return nil, projectExistsError(meta.ID)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, errs.Internal("检查项目目录失败", err)
	}

	tempDir, err := os.MkdirTemp(s.projectsDir, ".import-*")
	if err != nil {
		return nil, errs.Internal("创建导入临时目录失败", err)
	}
	tempActive := true
	defer func() {
		if tempActive {
			_ = os.RemoveAll(tempDir)
		}
	}()

	if err := extractZipEntries(entries, tempDir); err != nil {
		return nil, err
	}

	if err := os.Rename(tempDir, finalDir); err != nil {
		if _, statErr := os.Stat(finalDir); statErr == nil {
			return nil, projectExistsError(meta.ID)
		}
		return nil, errs.Internal("移动导入项目目录失败", err)
	}
	tempActive = false

	registered := false
	defer func() {
		if !registered {
			_ = os.RemoveAll(finalDir)
		}
	}()

	if err := ensureProjectDirs(finalDir); err != nil {
		return nil, errs.Internal("初始化导入项目目录结构失败", err)
	}

	meta.ProjectPath = finalDir
	normalizeProject(meta)
	if err := s.WriteMeta(meta); err != nil {
		return nil, errs.Internal("写入导入项目元数据失败", err)
	}
	if err := s.store.CreateProject(meta); err != nil {
		if errors.Is(err, errs.ErrAlreadyExists) {
			if _, getErr := s.store.GetProject(meta.ID); getErr == nil {
				return nil, projectExistsError(meta.ID)
			}
			// 项目本身不存在，说明是包内日志源ID与既有数据冲突。
			return nil, errs.Internal("导入失败：项目包内的日志源ID与现有数据冲突", err)
		}
		return nil, errs.Internal("注册导入项目失败", err)
	}
	registered = true
	return meta, nil
}

// validateZipEntries 校验所有条目并返回安全条目列表与根目录 project.meta。
func validateZipEntries(files []*zip.File) ([]zipEntry, *zip.File, error) {
	entries := make([]zipEntry, 0, len(files))
	seen := make(map[string]struct{}, len(files))
	var metaFile *zip.File
	var declaredTotal uint64

	for _, f := range files {
		clean, isDir, err := validateZipEntryName(f.Name)
		if err != nil {
			return nil, nil, err
		}
		if f.Mode().IsDir() {
			isDir = true
		}

		key := strings.ToLower(clean)
		if _, dup := seen[key]; dup {
			return nil, nil, archiveInvalid("压缩包包含重复路径: %s", clean)
		}
		seen[key] = struct{}{}

		if f.Mode()&os.ModeSymlink != 0 {
			return nil, nil, archiveInvalid("压缩包包含符号链接: %s", clean)
		}
		if !isDir && !f.Mode().IsRegular() {
			return nil, nil, archiveInvalid("压缩包包含非普通文件: %s", clean)
		}
		if !isDir {
			if f.UncompressedSize64 > uint64(importMaxTotalUncompressed) ||
				declaredTotal > uint64(importMaxTotalUncompressed)-f.UncompressedSize64 {
				return nil, nil, archiveInvalid("压缩包解压后总大小超过 %d 字节", importMaxTotalUncompressed)
			}
			declaredTotal += f.UncompressedSize64
		}

		if clean == "project.meta" && !isDir {
			metaFile = f
		}
		entries = append(entries, zipEntry{file: f, name: clean, isDir: isDir})
	}
	return entries, metaFile, nil
}

// validateZipEntryName 校验压缩包内路径，防止 Zip Slip / 绝对路径 / ADS。
func validateZipEntryName(name string) (string, bool, error) {
	if name == "" {
		return "", false, archiveInvalid("压缩包包含空路径")
	}
	if strings.ContainsRune(name, '\x00') {
		return "", false, archiveInvalid("压缩包路径包含非法字符")
	}
	// ZIP 规范使用 '/'；反斜杠在 Windows 上可能构成路径穿越。
	if strings.Contains(name, `\`) {
		return "", false, archiveInvalid("压缩包路径包含非法分隔符: %s", name)
	}
	isDir := strings.HasSuffix(name, "/")
	clean := path.Clean(name)
	if clean == "." || clean == ".." || path.IsAbs(clean) || strings.HasPrefix(clean, "../") {
		return "", false, archiveInvalid("压缩包路径越界: %s", name)
	}
	if filepath.IsAbs(clean) || filepath.VolumeName(clean) != "" {
		return "", false, archiveInvalid("压缩包包含绝对路径: %s", name)
	}
	// Windows ADS（file:stream）与盘符均含 ':'。
	if strings.Contains(clean, ":") {
		return "", false, archiveInvalid("压缩包路径包含非法字符: %s", name)
	}
	for _, segment := range strings.Split(clean, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", false, archiveInvalid("压缩包路径越界: %s", name)
		}
		if err := validateArchiveSegment(segment, name); err != nil {
			return "", false, err
		}
	}
	return clean, isDir, nil
}

// validateArchiveSegment 拒绝 Windows 非法文件名/设备名，避免解压时
// 触发意外设备访问或生成无法使用的文件。
func validateArchiveSegment(segment, fullName string) error {
	if strings.TrimSpace(segment) != segment {
		return archiveInvalid("压缩包路径包含首尾空白: %s", fullName)
	}
	if strings.HasSuffix(segment, ".") || strings.HasSuffix(segment, " ") {
		return archiveInvalid("压缩包路径以点或空格结尾: %s", fullName)
	}
	if strings.ContainsAny(segment, `<>:"|?*`) {
		return archiveInvalid("压缩包路径包含非法字符: %s", fullName)
	}
	for _, r := range segment {
		if r < 0x20 || r == 0x7f {
			return archiveInvalid("压缩包路径包含控制字符: %s", fullName)
		}
	}

	base := strings.ToUpper(segment)
	if idx := strings.IndexByte(base, '.'); idx >= 0 {
		base = base[:idx]
	}
	if _, reserved := windowsReservedNames[base]; reserved {
		return archiveInvalid("压缩包路径包含系统保留名称: %s", fullName)
	}
	if base == "CONIN$" || base == "CONOUT$" {
		return archiveInvalid("压缩包路径包含系统保留名称: %s", fullName)
	}
	return nil
}

// readMetaFromZip 读取并解析根目录 project.meta（限制大小）。
func readMetaFromZip(f *zip.File) (*models.Project, error) {
	if f.UncompressedSize64 > uint64(importMaxMetaSize) {
		return nil, archiveInvalid("project.meta 过大")
	}
	rc, err := f.Open()
	if err != nil {
		return nil, archiveInvalid("无法读取 project.meta: %v", err)
	}
	data, readErr := io.ReadAll(io.LimitReader(rc, importMaxMetaSize+1))
	closeErr := rc.Close()
	if readErr != nil {
		return nil, archiveInvalid("读取 project.meta 失败: %v", readErr)
	}
	if closeErr != nil {
		return nil, archiveInvalid("关闭 project.meta 失败: %v", closeErr)
	}
	if int64(len(data)) > importMaxMetaSize {
		return nil, archiveInvalid("project.meta 过大")
	}

	var meta models.Project
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, archiveInvalid("project.meta 不是有效 JSON: %v", err)
	}
	return &meta, nil
}

// extractZipEntries 将条目安全解压到目标目录，并限制实际写入总量。
func extractZipEntries(entries []zipEntry, destDir string) error {
	remaining := importMaxTotalUncompressed

	for _, entry := range entries {
		target := filepath.Join(destDir, filepath.FromSlash(entry.name))
		if !isWithinDir(destDir, target) {
			return archiveInvalid("解压目标越界: %s", entry.name)
		}

		if entry.isDir {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return errs.Internal("创建导入目录失败", err)
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return errs.Internal("创建导入子目录失败", err)
		}

		rc, err := entry.file.Open()
		if err != nil {
			return archiveInvalid("无法读取压缩包条目 %s: %v", entry.name, err)
		}

		dst, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			_ = rc.Close()
			return errs.Internal("创建导入文件失败", err)
		}

		written, copyErr := io.Copy(dst, io.LimitReader(rc, remaining+1))
		rcCloseErr := rc.Close()
		dstCloseErr := dst.Close()
		if copyErr != nil {
			return errs.Internal("写入导入文件失败", copyErr)
		}
		if written > remaining {
			return archiveInvalid("解压后总大小超过 %d 字节", importMaxTotalUncompressed)
		}
		remaining -= written
		if rcCloseErr != nil {
			return archiveInvalid("关闭压缩包条目 %s 失败: %v", entry.name, rcCloseErr)
		}
		if dstCloseErr != nil {
			return errs.Internal("关闭导入文件失败", dstCloseErr)
		}
	}
	return nil
}

func archiveInvalid(format string, args ...any) error {
	msg := fmt.Sprintf(format, args...)
	return fmt.Errorf("%s: %w", msg, errs.ErrArchiveInvalid)
}

func projectExistsError(id string) error {
	return fmt.Errorf("项目已存在（ID=%s）: %w", id, errs.ErrProjectExists)
}
