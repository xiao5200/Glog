package project

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"glog/internal/errs"
)

const (
	exportPackageFormat  = "glog-project-package"
	exportPackageVersion = 1
)

// exportManifest 是导出包的 manifest.json 内容。
type exportManifest struct {
	Format    string            `json:"format"`
	Version   int               `json:"version"`
	ProjectID string            `json:"project_id"`
	CreatedAt time.Time         `json:"created_at"`
	Files     map[string]string `json:"files"` // 相对路径 -> sha256
}

type exportFile struct {
	rel string
	abs string
}

// Export 将项目打包为 .glogpkg.zip，返回实际生成的 zip 绝对路径。
//
// 打包内容：project.meta、raw/、indexes/（不存在的目录跳过）。
// 输出路径为空时写入 <projectsDir>/../exports/<id>.glogpkg.zip；
// outputPath 为已存在目录时自动追加文件名。
func (s *Service) Export(id, outputPath string) (string, error) {
	dir, err := s.projectDirChecked(id)
	if err != nil {
		return "", errs.InvalidInput("项目ID无效", err)
	}
	p, err := s.Get(id)
	if err != nil {
		return "", err
	}
	// 导出前刷新元数据文件，确保与数据库一致。
	if err := s.WriteMeta(p); err != nil {
		return "", err
	}

	files, err := collectExportFiles(dir)
	if err != nil {
		return "", errs.Internal("收集项目文件失败", err)
	}

	target, err := s.resolveExportPath(id, outputPath)
	if err != nil {
		return "", err
	}
	outDir := filepath.Dir(target)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", errs.Internal("创建导出目录失败", err)
	}

	tmpFile, err := os.CreateTemp(outDir, ".export-*.zip")
	if err != nil {
		return "", errs.Internal("创建导出临时文件失败", err)
	}
	tmpName := tmpFile.Name()
	cleanup := func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpName)
	}

	zw := zip.NewWriter(tmpFile)
	manifest := exportManifest{
		Format:    exportPackageFormat,
		Version:   exportPackageVersion,
		ProjectID: p.ID,
		CreatedAt: time.Now().UTC(),
		Files:     make(map[string]string, len(files)),
	}

	for _, file := range files {
		sum, err := addExportFile(zw, file.rel, file.abs)
		if err != nil {
			_ = zw.Close()
			cleanup()
			return "", errs.Internal("打包项目文件失败", err)
		}
		manifest.Files[file.rel] = sum
	}

	manifestJSON, err := json.MarshalIndent(&manifest, "", "  ")
	if err != nil {
		_ = zw.Close()
		cleanup()
		return "", errs.Internal("生成导出清单失败", err)
	}
	manifestJSON = append(manifestJSON, '\n')
	header := &zip.FileHeader{
		Name:     "manifest.json",
		Method:   zip.Deflate,
		Modified: manifest.CreatedAt,
	}
	manifestWriter, err := zw.CreateHeader(header)
	if err != nil {
		_ = zw.Close()
		cleanup()
		return "", errs.Internal("写入导出清单失败", err)
	}
	if _, err := manifestWriter.Write(manifestJSON); err != nil {
		_ = zw.Close()
		cleanup()
		return "", errs.Internal("写入导出清单失败", err)
	}

	if err := zw.Close(); err != nil {
		cleanup()
		return "", errs.Internal("完成导出压缩包失败", err)
	}
	if err := tmpFile.Sync(); err != nil {
		cleanup()
		return "", errs.Internal("同步导出文件失败", err)
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpName)
		return "", errs.Internal("关闭导出文件失败", err)
	}
	if err := os.Rename(tmpName, target); err != nil {
		_ = os.Remove(tmpName)
		return "", errs.Internal("保存导出压缩包失败", err)
	}

	abs, err := filepath.Abs(target)
	if err != nil {
		return target, nil
	}
	return abs, nil
}

// collectExportFiles 收集 project.meta、raw/ 与 indexes/ 下的普通文件。
func collectExportFiles(projectDir string) ([]exportFile, error) {
	files := make([]exportFile, 0, 16)

	metaPath := filepath.Join(projectDir, "project.meta")
	if info, err := os.Stat(metaPath); err != nil {
		return nil, fmt.Errorf("project.meta 不可用: %w", err)
	} else if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("project.meta 不是普通文件")
	}
	files = append(files, exportFile{rel: "project.meta", abs: metaPath})

	for _, sub := range []string{"raw", "indexes"} {
		root := filepath.Join(projectDir, sub)
		walkErr := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					return nil
				}
				return err
			}
			if d.IsDir() {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
				// 不跟随符号链接，也不打包设备文件等特殊文件。
				return nil
			}
			rel, err := filepath.Rel(projectDir, path)
			if err != nil {
				return err
			}
			if !isWithinDir(projectDir, path) {
				return fmt.Errorf("文件路径越界: %s", path)
			}
			files = append(files, exportFile{
				rel: filepath.ToSlash(rel),
				abs: path,
			})
			return nil
		})
		if walkErr != nil && !errors.Is(walkErr, os.ErrNotExist) {
			return nil, walkErr
		}
	}
	return files, nil
}

// addExportFile 将文件以 Deflate 方式写入 zip，并返回其 sha256。
func addExportFile(zw *zip.Writer, rel, abs string) (string, error) {
	f, err := os.Open(abs)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("不是普通文件: %s", abs)
	}

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return "", err
	}
	header.Name = filepath.ToSlash(rel)
	header.Method = zip.Deflate

	writer, err := zw.CreateHeader(header)
	if err != nil {
		return "", err
	}
	hasher := sha256.New()
	if _, err := io.Copy(io.MultiWriter(writer, hasher), f); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// resolveExportPath 计算最终 zip 输出路径。
func (s *Service) resolveExportPath(id, outputPath string) (string, error) {
	fileName := id + ".glogpkg.zip"
	var target string

	if strings.TrimSpace(outputPath) == "" {
		target = filepath.Join(filepath.Dir(s.projectsDir), "exports", fileName)
	} else {
		clean := filepath.Clean(outputPath)
		if info, err := os.Stat(clean); err == nil && info.IsDir() {
			target = filepath.Join(clean, fileName)
		} else if strings.HasSuffix(outputPath, string(os.PathSeparator)) ||
			strings.HasSuffix(outputPath, "/") || strings.HasSuffix(outputPath, `\`) {
			target = filepath.Join(clean, fileName)
		} else {
			target = clean
		}
	}

	abs, err := filepath.Abs(target)
	if err != nil {
		return "", errs.Internal("解析导出路径失败", err)
	}
	return abs, nil
}
