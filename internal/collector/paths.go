package collector

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"glog/internal/errs"
	"glog/pkg/models"
)

const (
	maxBrowseEntries = 5000
	maxPreviewLines  = 1000
	evtxMagic        = "ElfFile\x00"
)

// ExpandPaths 展开通配符与目录，返回去重、排序后的绝对文件路径。
// excludePaths 支持目录前缀与 filepath.Match 风格通配符。
func ExpandPaths(patterns []string, recursive bool, excludePaths []string) ([]string, error) {
	files, errsList := expandPaths(context.Background(), patterns, recursive, excludePaths)
	if len(errsList) > 0 {
		return files, errors.Join(errsList...)
	}
	return files, nil
}

// expandPaths 是 ExpandPaths 的可取消实现；返回成功文件和逐项错误。
func expandPaths(ctx context.Context, patterns []string, recursive bool, excludePaths []string) ([]string, []error) {
	var files []string
	var errsList []error
	seen := make(map[string]struct{})

	addFile := func(rawPath string) error {
		abs, err := filepath.Abs(rawPath)
		if err != nil {
			return fmt.Errorf("解析路径失败 %s: %w", rawPath, err)
		}
		abs = filepath.Clean(abs)
		if pathExcluded(abs, excludePaths) {
			return nil
		}
		info, err := os.Stat(abs)
		if err != nil {
			return fmt.Errorf("访问文件失败 %s: %w", rawPath, err)
		}
		if info.IsDir() {
			return fmt.Errorf("路径是目录而非文件: %s", rawPath)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("不是常规文件: %s", rawPath)
		}
		key := abs
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if _, ok := seen[key]; ok {
			return nil
		}
		seen[key] = struct{}{}
		files = append(files, abs)
		return nil
	}

	addDir := func(dir string) {
		root, err := filepath.Abs(dir)
		if err != nil {
			errsList = append(errsList, fmt.Errorf("解析目录失败 %s: %w", dir, err))
			return
		}
		walkErr := filepath.WalkDir(root, func(p string, d fs.DirEntry, walkErr error) error {
			if ctx != nil {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			if walkErr != nil {
				errsList = append(errsList, fmt.Errorf("遍历目录失败 %s: %w", p, walkErr))
				if d != nil && d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				if pathExcluded(p, excludePaths) {
					return fs.SkipDir
				}
				return nil
			}
			// 不跟随目录树中的符号链接，避免循环与越界扫描。
			if d.Type()&os.ModeSymlink != 0 {
				return nil
			}
			if !d.Type().IsRegular() {
				return nil
			}
			if pathExcluded(p, excludePaths) {
				return nil
			}
			if err := addFile(p); err != nil {
				errsList = append(errsList, err)
			}
			return nil
		})
		if walkErr != nil && !errors.Is(walkErr, context.Canceled) && !errors.Is(walkErr, context.DeadlineExceeded) {
			errsList = append(errsList, fmt.Errorf("遍历目录失败 %s: %w", dir, walkErr))
		}
	}

	for _, rawPattern := range patterns {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				errsList = append(errsList, err)
				break
			}
		}

		pattern := strings.TrimSpace(rawPattern)
		if pattern == "" {
			errsList = append(errsList, fmt.Errorf("日志路径不能为空"))
			continue
		}
		if strings.ContainsRune(pattern, 0) {
			errsList = append(errsList, fmt.Errorf("日志路径包含 NUL 字符: %q", pattern))
			continue
		}

		expanded, err := expandHome(pattern)
		if err != nil {
			errsList = append(errsList, fmt.Errorf("展开路径失败 %s: %w", pattern, err))
			continue
		}

		matches, globErr := filepath.Glob(expanded)
		if globErr != nil {
			errsList = append(errsList, fmt.Errorf("通配符路径无效 %s: %w", pattern, globErr))
			continue
		}
		if len(matches) == 0 {
			errsList = append(errsList, fmt.Errorf("路径不存在或没有匹配文件: %s", pattern))
			continue
		}

		for _, match := range matches {
			if ctx != nil {
				if err := ctx.Err(); err != nil {
					errsList = append(errsList, err)
					break
				}
			}
			info, statErr := os.Stat(match)
			if statErr != nil {
				errsList = append(errsList, fmt.Errorf("访问路径失败 %s: %w", match, statErr))
				continue
			}
			if info.IsDir() {
				if recursive {
					addDir(match)
				} else {
					errsList = append(errsList, fmt.Errorf("路径是目录，需启用递归扫描: %s", match))
				}
				continue
			}
			if err := addFile(match); err != nil {
				errsList = append(errsList, err)
			}
		}
	}

	sort.Strings(files)
	return files, errsList
}

// matchPathPattern 是 path.Match 的容错封装：模式非法时视为不匹配，
// 避免单个无效排除模式导致整个路径展开失败。
func matchPathPattern(pattern, name string) bool {
	matched, err := path.Match(pattern, name)
	if err != nil {
		return false
	}
	return matched
}

// pathExcluded 判断路径是否命中排除规则。支持：
//   - 目录前缀（/var/log/nginx 会排除其下所有文件）；
//   - 通配符（*.tmp、/var/log/*.gz）；
//   - 相对通配符（logs/*.log 会匹配路径后缀）。
func pathExcluded(filePath string, patterns []string) bool {
	if len(patterns) == 0 {
		return false
	}

	p := filepath.ToSlash(filepath.Clean(filePath))
	fold := runtime.GOOS == "windows"
	if fold {
		p = strings.ToLower(p)
	}
	base := path.Base(p)

	for _, raw := range patterns {
		raw = strings.TrimSpace(raw)
		if raw == "" || strings.ContainsRune(raw, 0) {
			continue
		}
		if expanded, err := expandHome(raw); err == nil {
			raw = expanded
		}
		pattern := filepath.ToSlash(raw)
		if fold {
			pattern = strings.ToLower(pattern)
		}

		if strings.ContainsAny(pattern, "*?[") {
			if matchPathPattern(pattern, p) || matchPathPattern(pattern, base) {
				return true
			}
			// 允许相对通配符匹配绝对路径的任意后缀。
			for i := 0; i < len(p); i++ {
				if p[i] == '/' && matchPathPattern(pattern, p[i+1:]) {
					return true
				}
			}
			continue
		}

		cleaned := path.Clean(pattern)
		if p == cleaned {
			return true
		}
		prefix := strings.TrimRight(cleaned, "/") + "/"
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}

// ExpandHome 展开以 ~/ 或 ~\ 开头的路径。
func expandHome(p string) (string, error) {
	if p == "~" {
		return os.UserHomeDir()
	}
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("无法获取用户目录: %w", err)
		}
		return filepath.Join(home, p[2:]), nil
	}
	return p, nil
}

// ValidatePath 检查路径存在且可读。目录会做一次 ReadDir 权限探测。
func ValidatePath(filePath string) (bool, error) {
	if strings.TrimSpace(filePath) == "" {
		return false, fmt.Errorf("路径不能为空")
	}
	if strings.ContainsRune(filePath, 0) {
		return false, fmt.Errorf("路径包含 NUL 字符")
	}

	expanded, err := expandHome(filePath)
	if err != nil {
		return false, err
	}

	info, err := os.Stat(expanded)
	if err != nil {
		switch {
		case os.IsNotExist(err):
			return false, fmt.Errorf("路径不存在: %s", filePath)
		case os.IsPermission(err):
			return false, fmt.Errorf("无权访问路径 %s: %w", filePath, err)
		default:
			return false, fmt.Errorf("检查路径失败 %s: %w", filePath, err)
		}
	}

	if info.IsDir() {
		f, err := os.Open(expanded)
		if err != nil {
			return false, fmt.Errorf("目录不可读 %s: %w", filePath, err)
		}
		_, readErr := f.Readdirnames(1)
		_ = f.Close()
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return false, fmt.Errorf("目录不可读 %s: %w", filePath, readErr)
		}
		return true, nil
	}

	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("不是常规文件: %s", filePath)
	}

	f, err := os.Open(expanded)
	if err != nil {
		return false, fmt.Errorf("文件不可读 %s: %w", filePath, err)
	}
	_ = f.Close()
	return true, nil
}

// ReadFilePreview 读取文件前 lines 行，支持 .gz。lines 非法时使用 100。
func ReadFilePreview(filePath string, lines int) ([]string, error) {
	if lines <= 0 || lines > maxPreviewLines {
		lines = 100
	}
	if strings.TrimSpace(filePath) == "" {
		return nil, fmt.Errorf("预览路径不能为空")
	}

	expanded, err := expandHome(filePath)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(expanded)
	if err != nil {
		return nil, fmt.Errorf("访问预览文件失败 %s: %w", filePath, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("不能预览目录: %s", filePath)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("不是常规文件: %s", filePath)
	}

	rd, closeFn, err := openLocalReader(expanded)
	if err != nil {
		return nil, fmt.Errorf("打开预览文件失败 %s: %w", filePath, err)
	}
	defer func() { _ = closeFn() }()

	br := bufio.NewReaderSize(rd, readBufferSize)
	preview := make([]string, 0, lines)
	for len(preview) < lines {
		line, _, _, readErr := readLineLimited(br, defaultBufferSize)
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return nil, fmt.Errorf("读取预览文件失败 %s: %w", filePath, readErr)
		}
		preview = append(preview, string(line))
	}
	return preview, nil
}

// BrowseDirectory 返回目录下的文件与子目录，最多 5000 项，目录排在前面。
func BrowseDirectory(dirPath string) ([]models.FileInfo, error) {
	if strings.TrimSpace(dirPath) == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("无法获取用户目录: %w", err)
		}
		dirPath = home
	}
	if strings.ContainsRune(dirPath, 0) {
		return nil, fmt.Errorf("目录路径包含 NUL 字符")
	}

	expanded, err := expandHome(dirPath)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(expanded)
	if err != nil {
		return nil, fmt.Errorf("访问目录失败 %s: %w", dirPath, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("不是目录: %s", dirPath)
	}

	f, err := os.Open(expanded)
	if err != nil {
		return nil, fmt.Errorf("打开目录失败 %s: %w", dirPath, err)
	}
	entries, readErr := f.ReadDir(maxBrowseEntries + 1)
	_ = f.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return nil, fmt.Errorf("读取目录失败 %s: %w", dirPath, readErr)
	}
	if len(entries) > maxBrowseEntries {
		entries = entries[:maxBrowseEntries]
	}

	infos := make([]models.FileInfo, 0, len(entries))
	for _, entry := range entries {
		entryInfo, infoErr := entry.Info()
		if infoErr != nil {
			// 单个条目不可访问时跳过，不影响整个目录浏览。
			continue
		}
		infos = append(infos, models.FileInfo{
			Name:    entry.Name(),
			Path:    filepath.Join(expanded, entry.Name()),
			IsDir:   entry.IsDir(),
			Size:    entryInfo.Size(),
			ModTime: entryInfo.ModTime(),
		})
	}

	sort.SliceStable(infos, func(i, j int) bool {
		if infos[i].IsDir != infos[j].IsDir {
			return infos[i].IsDir
		}
		return strings.ToLower(infos[i].Name) < strings.ToLower(infos[j].Name)
	})
	return infos, nil
}

// IsEVTX 通过扩展名或文件头魔数判断是否为 EVTX 文件。
func IsEVTX(filePath string) bool {
	if strings.EqualFold(filepath.Ext(filePath), ".evtx") {
		return true
	}
	f, err := os.Open(filePath)
	if err != nil {
		return false
	}
	defer f.Close()

	var magic [8]byte
	n, err := io.ReadFull(f, magic[:])
	if err != nil || n != len(magic) {
		return false
	}
	return string(magic[:]) == evtxMagic
}

// GetCommonLogPaths 按操作系统返回常见日志路径。路径中可能包含通配符与 ~。
func GetCommonLogPaths(goos string) map[string][]string {
	switch strings.ToLower(strings.TrimSpace(goos)) {
	case "windows":
		appData := os.Getenv("APPDATA")
		psHistory := `%APPDATA%\Microsoft\Windows\PowerShell\PSReadline\ConsoleHost_history.txt`
		if appData != "" {
			psHistory = appData + `\Microsoft\Windows\PowerShell\PSReadline\ConsoleHost_history.txt`
		}
		return map[string][]string{
			"Windows事件日志": {
				`C:\Windows\System32\winevt\Logs\System.evtx`,
				`C:\Windows\System32\winevt\Logs\Application.evtx`,
				`C:\Windows\System32\winevt\Logs\Security.evtx`,
				`C:\Windows\System32\winevt\Logs\Setup.evtx`,
				`C:\Windows\System32\winevt\Logs\ForwardedEvents.evtx`,
			},
			"IIS": {
				`C:\inetpub\logs\LogFiles\W3SVC1\*.log`,
				`C:\inetpub\logs\LogFiles\W3SVC2\*.log`,
			},
			"PowerShell": {
				psHistory,
			},
			"应用程序日志": {
				`C:\ProgramData\*.log`,
				`C:\Program Files\*\logs\*.log`,
				`C:\Program Files (x86)\*\logs\*.log`,
			},
			"系统日志": {
				`C:\Windows\Logs\CBS\CBS.log`,
				`C:\Windows\Logs\DISM\dism.log`,
			},
		}

	case "linux":
		return map[string][]string{
			"系统日志": {
				"/var/log/syslog",
				"/var/log/messages",
				"/var/log/kern.log",
				"/var/log/dmesg",
			},
			"认证日志": {
				"/var/log/auth.log",
				"/var/log/secure",
			},
			"审计日志": {
				"/var/log/audit/audit.log",
			},
			"计划任务": {
				"/var/log/cron",
				"/var/log/cron.log",
			},
			"用户历史": {
				"~/.bash_history",
				"~/.zsh_history",
			},
			"Web服务": {
				"/var/log/nginx/access.log",
				"/var/log/nginx/error.log",
				"/var/log/apache2/access.log",
				"/var/log/apache2/error.log",
			},
			"数据库": {
				"/var/log/mysql/error.log",
				"/var/log/postgresql/*.log",
			},
		}

	case "darwin":
		return map[string][]string{
			"系统日志": {
				"/var/log/system.log",
				"/var/log/install.log",
			},
			"用户历史": {
				"~/.bash_history",
				"~/.zsh_history",
			},
		}

	default:
		return map[string][]string{}
	}
}

// DefaultEventLogPaths 把 Windows 事件日志通道映射到默认 winevt 文件。
// 仅 Windows 可用；未知通道返回错误。
func DefaultEventLogPaths(channels []string) (map[string]string, error) {
	if runtime.GOOS != "windows" {
		return nil, fmt.Errorf("%w: Windows 事件日志路径仅支持在 Windows 上获取", errs.ErrUnsupportedOS)
	}
	if len(channels) == 0 {
		return nil, fmt.Errorf("未指定事件日志通道")
	}

	const baseDir = `C:\Windows\System32\winevt\Logs\`
	result := make(map[string]string, len(channels))
	for _, raw := range channels {
		channel := strings.TrimSpace(raw)
		var canonical, fileName string
		switch strings.ToLower(channel) {
		case "system":
			canonical, fileName = "System", "System.evtx"
		case "application":
			canonical, fileName = "Application", "Application.evtx"
		case "security":
			canonical, fileName = "Security", "Security.evtx"
		case "setup":
			canonical, fileName = "Setup", "Setup.evtx"
		case "forwardedevents", "forwarded events":
			canonical, fileName = "ForwardedEvents", "ForwardedEvents.evtx"
		default:
			return nil, fmt.Errorf("未知的 Windows 事件日志通道 %q（支持: System/Application/Security/Setup/ForwardedEvents）", raw)
		}
		fullPath := baseDir + fileName
		// internal/app 会用请求中的原始通道名回查 map，因此保留原始键；
		// 同时提供规范键，方便其他调用方按标准通道名查询。
		result[raw] = fullPath
		result[canonical] = fullPath
	}
	return result, nil
}
