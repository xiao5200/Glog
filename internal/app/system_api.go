package app

import (
	"net/url"
	"os"
	"os/exec"
	stdruntime "runtime"
	"strings"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"glog/internal/errs"
	"glog/pkg/models"
)

// GetSystemInfo 返回系统与路径信息。
func (a *App) GetSystemInfo() (*models.SystemInfo, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	cfg := a.configMgr
	exe, _ := os.Executable()
	info := &models.SystemInfo{
		OS:           stdruntime.GOOS,
		Arch:         stdruntime.GOARCH,
		Version:      cfg.Config().General.Version,
		DataPath:     cfg.DataDir(),
		ProjectsPath: cfg.ProjectsDir(),
		TempPath:     cfg.TempDir(),
		LogPath:      cfg.LogDir(),
		DBPath:       cfg.DBPath(),
		Executable:   exe,
	}
	if info.Version == "" {
		info.Version = "1.0.0"
	}
	return info, nil
}

// GetConfig 获取全局配置。
func (a *App) GetConfig() (models.AppConfig, error) {
	if err := a.ready(); err != nil {
		return models.AppConfig{}, err
	}
	return a.configMgr.Config(), nil
}

// UpdateConfig 更新全局配置。
func (a *App) UpdateConfig(cfg models.AppConfig) error {
	if err := a.ready(); err != nil {
		return err
	}
	if err := a.configMgr.Update(cfg); err != nil {
		return errs.Internal("保存配置失败", err)
	}
	a.reloadAI()
	return nil
}

// SelectFiles 打开文件选择对话框。
func (a *App) SelectFiles() ([]string, error) {
	if a.ctx == nil {
		return nil, errs.Internal("应用尚未就绪", nil)
	}
	paths, err := wailsruntime.OpenMultipleFilesDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "选择日志文件",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "日志文件", Pattern: "*.log;*.txt;*.json;*.csv;*.evtx;*.out"},
			{DisplayName: "压缩文件", Pattern: "*.gz;*.zip;*.zst;*.bz2"},
			{DisplayName: "项目包", Pattern: "*.zip;*.glogpkg"},
			{DisplayName: "所有文件", Pattern: "*.*"},
		},
	})
	if err != nil {
		return nil, errs.Internal("打开文件选择器失败", err)
	}
	if paths == nil {
		// 用户取消选择时返回空列表而不是错误。
		return []string{}, nil
	}
	return paths, nil
}

// SelectDirectory 打开目录选择对话框。
func (a *App) SelectDirectory(title string) (string, error) {
	if a.ctx == nil {
		return "", errs.Internal("应用尚未就绪", nil)
	}
	if strings.TrimSpace(title) == "" {
		title = "选择目录"
	}
	path, err := wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{Title: title})
	if err != nil {
		return "", errs.Internal("打开目录选择器失败", err)
	}
	return path, nil
}

// SelectSaveFile 打开保存文件对话框。
func (a *App) SelectSaveFile(defaultFilename, displayName, pattern string) (string, error) {
	if a.ctx == nil {
		return "", errs.Internal("应用尚未就绪", nil)
	}
	if strings.TrimSpace(defaultFilename) == "" {
		defaultFilename = "glog-export.zip"
	}
	if strings.TrimSpace(displayName) == "" {
		displayName = "导出文件"
	}
	if strings.TrimSpace(pattern) == "" {
		pattern = "*.*"
	}
	path, err := wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		Title:           "保存文件",
		DefaultFilename: defaultFilename,
		Filters: []wailsruntime.FileFilter{
			{DisplayName: displayName, Pattern: pattern},
			{DisplayName: "所有文件", Pattern: "*.*"},
		},
	})
	if err != nil {
		return "", errs.Internal("打开保存对话框失败", err)
	}
	return path, nil
}

// ShowInExplorer 在系统文件管理器中显示路径。
func (a *App) ShowInExplorer(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errs.InvalidInput("路径不能为空", nil)
	}
	info, err := os.Stat(path)
	if err != nil {
		return errs.NotFound("路径不存在", err)
	}

	var cmd *exec.Cmd
	switch stdruntime.GOOS {
	case "windows":
		if info.IsDir() {
			cmd = exec.Command("explorer.exe", path)
		} else {
			cmd = exec.Command("explorer.exe", "/select,"+path)
		}
	case "darwin":
		cmd = exec.Command("open", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	if err := cmd.Start(); err != nil {
		return errs.Internal("无法打开文件管理器", err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// OpenExternal 打开外部链接（仅允许 http/https）。
func (a *App) OpenExternal(rawURL string) error {
	if a.ctx == nil {
		return errs.Internal("应用尚未就绪", nil)
	}
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme == "" {
		return errs.InvalidInput("无效的链接", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errs.InvalidInput("仅允许打开 http/https 链接", nil)
	}
	wailsruntime.BrowserOpenURL(a.ctx, parsed.String())
	return nil
}
