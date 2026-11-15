package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	stdruntime "runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"glog/configs"
	"glog/internal/collector"
	"glog/internal/errs"
	"glog/internal/indexer"
	"glog/internal/logx"
	"glog/internal/parser"
	"glog/pkg/models"
)

const batchFlushSize = 1000

type entryFilter struct {
	start       *time.Time
	end         *time.Time
	eventIDs    map[uint32]bool
	levels      map[string]bool
	filePattern *regexp.Regexp
}

func buildEntryFilter(start, end *time.Time, eventIDs []uint32, levels []string, filePattern string) (entryFilter, error) {
	f := entryFilter{start: start, end: end}
	if len(eventIDs) > 0 {
		f.eventIDs = make(map[uint32]bool, len(eventIDs))
		for _, id := range eventIDs {
			f.eventIDs[id] = true
		}
	}
	if len(levels) > 0 {
		f.levels = make(map[string]bool, len(levels))
		for _, level := range levels {
			if level == "" {
				continue
			}
			f.levels[models.NormalizeLevel(level)] = true
			f.levels[strings.ToUpper(level)] = true
		}
	}
	if strings.TrimSpace(filePattern) != "" {
		re, err := regexp.Compile(filePattern)
		if err != nil {
			return f, errs.InvalidInput("文件名过滤正则无效: "+err.Error(), err)
		}
		f.filePattern = re
	}
	return f, nil
}

func (f entryFilter) matchFile(path string) bool {
	if f.filePattern == nil {
		return true
	}
	return f.filePattern.MatchString(path)
}

func (f entryFilter) matchEntry(entry *models.LogEntry) bool {
	if entry == nil {
		return false
	}
	if f.filePattern != nil && !f.filePattern.MatchString(entry.SourceFile) {
		return false
	}
	if f.start != nil && entry.Timestamp.Before(*f.start) {
		return false
	}
	if f.end != nil && entry.Timestamp.After(*f.end) {
		return false
	}
	if f.eventIDs != nil {
		if entry.EventID == nil || !f.eventIDs[*entry.EventID] {
			return false
		}
	}
	if f.levels != nil {
		if !f.levels[entry.Level] && !f.levels[models.NormalizeLevel(entry.Level)] {
			return false
		}
	}
	return true
}

// StartLocalCollection 启动本地日志采集任务。
func (a *App) StartLocalCollection(req models.LocalCollectRequest) (models.TaskInfo, error) {
	if err := a.ready(); err != nil {
		return models.TaskInfo{}, err
	}
	if _, _, _, err := a.requireCurrent(req.ProjectID); err != nil {
		return models.TaskInfo{}, err
	}
	if len(req.FilePaths) == 0 {
		return models.TaskInfo{}, errs.InvalidInput("至少选择一个文件或目录", nil)
	}
	filter, err := buildEntryFilter(nil, nil, nil, nil, req.FilePattern)
	if err != nil {
		return models.TaskInfo{}, err
	}

	cfg := a.configMgr.Config()
	concurrency := req.Concurrency
	if concurrency <= 0 {
		concurrency = 4
	}
	bufferSize := req.BufferSize
	if bufferSize <= 0 {
		bufferSize = cfg.Collector.LocalBufferSizeKB * 1024
	}
	if bufferSize <= 0 {
		bufferSize = 64 * 1024
	}

	sourceID := newID()
	sourceHost := hostname()
	opts := collector.Options{
		ProjectID:   req.ProjectID,
		SourceID:    sourceID,
		SourceHost:  sourceHost,
		Concurrency: concurrency,
		BufferSize:  bufferSize,
	}

	// 显式 EVTX 文件自动切换为原生 EVTX 解析，避免在本地文件采集中报错。
	if evtxPaths, ok := explicitEVTXPaths(req.FilePaths); ok {
		var unreadable []string
		for _, path := range evtxPaths {
			if !isReadableFile(path) {
				unreadable = append(unreadable, path)
			}
		}
		if len(unreadable) > 0 {
			return models.TaskInfo{}, errs.InvalidInput(
				"以下 EVTX 文件当前用户不可读，请在「Windows 事件日志」中选择对应通道采集（系统会自动使用 wevtutil 导出），或以管理员身份运行："+
					strings.Join(unreadable, "; "), nil)
		}
		parseCfg := req.Parse
		parseCfg.Mode = "native_evtx"
		parseCfg.TemplateID = "windows-evtx"
		evtxCol := collector.NewEVTXCollector(evtxPaths, opts)
		sourcePath := truncateForSource(strings.Join(evtxPaths, "; "))
		info := a.taskMgr.Start(req.ProjectID, "evtx", sourceID, "准备解析 EVTX 文件…", func(ctx context.Context, report taskReporter) (*models.CollectResult, error) {
			return a.runIngestion(ctx, req.ProjectID, sourceID, "local", sourceHost, sourcePath, evtxCol, parseCfg, filter, report)
		})
		return info, nil
	}

	col := collector.NewLocalTextCollector(req.FilePaths, req.Recursive, req.ExcludePaths, opts)
	sourcePath := truncateForSource(strings.Join(req.FilePaths, "; "))

	info := a.taskMgr.Start(req.ProjectID, "local", sourceID, "准备采集…", func(ctx context.Context, report taskReporter) (*models.CollectResult, error) {
		return a.runIngestion(ctx, req.ProjectID, sourceID, "local", sourceHost, sourcePath, col, req.Parse, filter, report)
	})
	return info, nil
}

// explicitEVTXPaths 判断路径列表是否全部为显式 EVTX 文件路径（不含通配符）。
func explicitEVTXPaths(paths []string) ([]string, bool) {
	out := make([]string, 0, len(paths))
	for _, raw := range paths {
		path := strings.TrimSpace(raw)
		if path == "" {
			continue
		}
		if strings.ContainsAny(path, "*?[") || !collector.IsEVTX(path) {
			return nil, false
		}
		out = append(out, path)
	}
	return out, len(out) > 0
}

// StartWindowsEventCollection 启动 Windows 事件日志采集任务。
func (a *App) StartWindowsEventCollection(req models.WindowsEventLogRequest) (models.TaskInfo, error) {
	if err := a.ready(); err != nil {
		return models.TaskInfo{}, err
	}
	if _, _, _, err := a.requireCurrent(req.ProjectID); err != nil {
		return models.TaskInfo{}, err
	}
	if len(req.Channels) == 0 {
		return models.TaskInfo{}, errs.InvalidInput("至少选择一个事件日志通道", nil)
	}
	pathMap, err := collector.DefaultEventLogPaths(req.Channels)
	if err != nil {
		return models.TaskInfo{}, err
	}
	paths := make([]string, 0, len(pathMap))
	channels := make([]string, 0, len(pathMap))
	var exportDir string
	for _, channel := range req.Channels {
		p, ok := pathMap[channel]
		if !ok || p == "" {
			continue
		}
		channels = append(channels, channel)
		if isReadableFile(p) || stdruntime.GOOS != "windows" {
			paths = append(paths, p)
			continue
		}
		// 普通用户通常无法直接读取 winevt 文件；通过 wevtutil 导出到临时目录后解析。
		if exportDir == "" {
			exportDir, err = os.MkdirTemp(a.configMgr.TempDir(), "evtx-")
			if err != nil {
				paths = append(paths, p)
				continue
			}
		}
		dst := filepath.Join(exportDir, sanitizeChannelName(channel)+".evtx")
		if err := exportEventChannel(channel, dst); err != nil {
			logx.Warn("wevtutil 导出事件日志失败", zap.String("channel", channel), zap.Error(err))
			paths = append(paths, p)
			continue
		}
		paths = append(paths, dst)
	}
	if len(paths) == 0 {
		return models.TaskInfo{}, errs.InvalidInput("未找到可用的事件日志文件", nil)
	}

	filter, err := buildEntryFilter(req.StartTime, req.EndTime, req.EventIDs, req.Levels, "")
	if err != nil {
		return models.TaskInfo{}, err
	}

	cfg := a.configMgr.Config()
	sourceID := newID()
	sourceHost := hostname()
	opts := collector.Options{
		ProjectID:   req.ProjectID,
		SourceID:    sourceID,
		SourceHost:  sourceHost,
		Concurrency: 1,
		BufferSize:  cfg.Collector.LocalBufferSizeKB * 1024,
	}
	col := collector.NewEVTXCollector(paths, opts)
	parseCfg := req.Parse
	if parseCfg.Mode == "" {
		parseCfg.Mode = "native_evtx"
	}
	if parseCfg.Mode == "native_evtx" || parseCfg.TemplateID == "" {
		parseCfg.TemplateID = "windows-evtx"
	}
	sourcePath := "事件日志: " + strings.Join(channels, ", ")

	info := a.taskMgr.Start(req.ProjectID, "evtx", sourceID, "准备解析事件日志…", func(ctx context.Context, report taskReporter) (*models.CollectResult, error) {
		if exportDir != "" {
			defer os.RemoveAll(exportDir)
		}
		return a.runIngestion(ctx, req.ProjectID, sourceID, "evtx", sourceHost, sourcePath, col, parseCfg, filter, report)
	})
	return info, nil
}

func isReadableFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

func sanitizeChannelName(channel string) string {
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_")
	name := strings.TrimSpace(replacer.Replace(channel))
	if name == "" {
		name = "channel"
	}
	return name
}

// exportEventChannel 使用 Windows 自带 wevtutil 导出通道，规避普通用户读取 winevt 文件的权限限制。
func exportEventChannel(channel, dst string) error {
	if channel == "" {
		return errs.InvalidInput("通道名不能为空", nil)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "wevtutil", "epl", channel, dst, "/ow:true")
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if len(msg) > 300 {
			msg = msg[:300] + "…"
		}
		return fmt.Errorf("wevtutil 导出失败: %v %s", err, msg)
	}
	if info, statErr := os.Stat(dst); statErr != nil || info.Size() == 0 {
		return fmt.Errorf("wevtutil 未生成有效文件: %v", statErr)
	}
	return nil
}

// StartSSHCollection 启动 SSH 远程采集任务。
func (a *App) StartSSHCollection(req models.SSHCollectRequest) (models.TaskInfo, error) {
	if err := a.ready(); err != nil {
		return models.TaskInfo{}, err
	}
	if _, _, _, err := a.requireCurrent(req.ProjectID); err != nil {
		return models.TaskInfo{}, err
	}
	if req.HostID == "" {
		return models.TaskInfo{}, errs.InvalidInput("请选择 SSH 主机", nil)
	}
	if len(req.RemotePaths) == 0 {
		return models.TaskInfo{}, errs.InvalidInput("至少填写一个远程路径", nil)
	}

	cfg := a.configMgr.Config()
	opts, host, err := a.sshOptionsFromHost(req.HostID, time.Duration(cfg.Collector.SSHTimeoutSeconds)*time.Second)
	if err != nil {
		return models.TaskInfo{}, err
	}

	filter, err := buildEntryFilter(nil, nil, nil, nil, "")
	if err != nil {
		return models.TaskInfo{}, err
	}

	concurrency := req.Concurrency
	if concurrency <= 0 {
		concurrency = cfg.Collector.SSHConcurrency
	}
	if concurrency <= 0 {
		concurrency = 4
	}
	sourceID := newID()
	sourceHost := host.Name
	if sourceHost == "" {
		sourceHost = host.Host
	}
	opts.Compression = req.Compression
	opts.Concurrency = concurrency
	opts.Strategy = req.Strategy
	opts.FilterPattern = req.FilterPattern
	opts.RemotePaths = req.RemotePaths
	opts.Since = req.Since
	opts.Until = req.Until
	opts.Unit = req.Unit
	copts := collector.Options{
		ProjectID:   req.ProjectID,
		SourceID:    sourceID,
		SourceHost:  sourceHost,
		Concurrency: concurrency,
		BufferSize:  256 * 1024,
	}
	col := collector.NewSSHCollector(opts, copts)
	sourcePath := truncateForSource(sourceHost + ": " + strings.Join(req.RemotePaths, ", "))

	info := a.taskMgr.Start(req.ProjectID, "ssh", sourceID, "正在连接 SSH 主机…", func(ctx context.Context, report taskReporter) (*models.CollectResult, error) {
		result, err := a.runIngestion(ctx, req.ProjectID, sourceID, "ssh", sourceHost, sourcePath, col, req.Parse, filter, report)
		_ = a.metadata.TouchSSHHost(req.HostID)
		return result, err
	})
	return info, nil
}

// StartJournalCollection 启动 Linux 本地 journalctl 采集任务。
func (a *App) StartJournalCollection(req models.SystemdJournalRequest) (models.TaskInfo, error) {
	if err := a.ready(); err != nil {
		return models.TaskInfo{}, err
	}
	if _, _, _, err := a.requireCurrent(req.ProjectID); err != nil {
		return models.TaskInfo{}, err
	}
	if stdruntime.GOOS != "linux" {
		return models.TaskInfo{}, errs.ErrUnsupportedOS
	}

	sourceID := newID()
	sourceHost := hostname()
	copts := collector.Options{
		ProjectID:   req.ProjectID,
		SourceID:    sourceID,
		SourceHost:  sourceHost,
		Concurrency: 1,
		BufferSize:  256 * 1024,
	}
	jopts := collector.JournalOptions{
		Since:    req.Since,
		Until:    req.Until,
		Unit:     req.Unit,
		Priority: req.Priority,
		Lines:    req.Lines,
		Follow:   req.Follow,
	}
	col := collector.NewJournalCollector(jopts, copts)
	parseCfg := req.Parse
	if parseCfg.Mode == "" {
		parseCfg.Mode = "builtin"
	}
	if parseCfg.TemplateID == "" && parseCfg.Mode == "builtin" {
		parseCfg.TemplateID = "journald-json"
	}
	filter, err := buildEntryFilter(nil, nil, nil, nil, "")
	if err != nil {
		return models.TaskInfo{}, err
	}
	sourcePath := "journalctl"
	if req.Unit != "" {
		sourcePath += " --unit " + req.Unit
	}

	info := a.taskMgr.Start(req.ProjectID, "journal", sourceID, "正在读取 systemd 日志…", func(ctx context.Context, report taskReporter) (*models.CollectResult, error) {
		return a.runIngestion(ctx, req.ProjectID, sourceID, "journal", sourceHost, sourcePath, col, parseCfg, filter, report)
	})
	return info, nil
}

// ImportFile 导入单个日志文件（文本/gzip/evtx 自动识别）。
func (a *App) ImportFile(req models.ImportFileRequest) (models.TaskInfo, error) {
	if err := a.ready(); err != nil {
		return models.TaskInfo{}, err
	}
	if _, _, _, err := a.requireCurrent(req.ProjectID); err != nil {
		return models.TaskInfo{}, err
	}
	if strings.TrimSpace(req.FilePath) == "" {
		return models.TaskInfo{}, errs.InvalidInput("文件路径不能为空", nil)
	}
	ok, _ := collector.ValidatePath(req.FilePath)
	if !ok {
		return models.TaskInfo{}, errs.ErrFileNotFound
	}

	sourceID := newID()
	sourceHost := hostname()
	cfg := a.configMgr.Config()
	opts := collector.Options{
		ProjectID:   req.ProjectID,
		SourceID:    sourceID,
		SourceHost:  sourceHost,
		Concurrency: 1,
		BufferSize:  cfg.Collector.LocalBufferSizeKB * 1024,
	}
	var col collector.Collector
	if collector.IsEVTX(req.FilePath) {
		col = collector.NewEVTXCollector([]string{req.FilePath}, opts)
	} else {
		col = collector.NewLocalTextCollector([]string{req.FilePath}, false, nil, opts)
	}
	filter, err := buildEntryFilter(nil, nil, nil, nil, "")
	if err != nil {
		return models.TaskInfo{}, err
	}
	sourcePath := truncateForSource(req.FilePath)
	info := a.taskMgr.Start(req.ProjectID, "import", sourceID, "准备导入…", func(ctx context.Context, report taskReporter) (*models.CollectResult, error) {
		return a.runIngestion(ctx, req.ProjectID, sourceID, "import", sourceHost, sourcePath, col, req.Parse, filter, report)
	})
	return info, nil
}

// StopCollection 停止采集任务。
func (a *App) StopCollection(taskID string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.taskMgr.Stop(taskID)
}

// ListTasks 列出任务。
func (a *App) ListTasks() ([]models.TaskInfo, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.taskMgr.List(), nil
}

// GetLocalCommonLogPaths 返回当前系统常见日志路径。
func (a *App) GetLocalCommonLogPaths() (map[string][]string, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return collector.GetCommonLogPaths(stdruntime.GOOS), nil
}

// BrowseLocalDirectory 浏览本地目录。
func (a *App) BrowseLocalDirectory(path string) ([]models.FileInfo, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(path) == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, errs.InvalidInput("路径不能为空", err)
		}
		path = home
	}
	return collector.BrowseDirectory(path)
}

// GetLogFilePreview 预览日志文件前 N 行。
func (a *App) GetLogFilePreview(path string, lines int) ([]string, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return collector.ReadFilePreview(path, lines)
}

// ValidateLocalPath 校验路径是否可读。
func (a *App) ValidateLocalPath(path string) bool {
	ok, _ := collector.ValidatePath(path)
	return ok
}

// GetMiddlewarePresets 返回内置中间件路径预设。
func (a *App) GetMiddlewarePresets() ([]models.MiddlewarePreset, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	data, err := configs.FS.ReadFile(configs.PresetsFile)
	if err != nil {
		return nil, errs.Internal("读取中间件预设失败", err)
	}
	var file struct {
		Presets []models.MiddlewarePreset `yaml:"presets"`
	}
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, errs.Internal("解析中间件预设失败", err)
	}
	sort.SliceStable(file.Presets, func(i, j int) bool {
		if file.Presets[i].Category == file.Presets[j].Category {
			return file.Presets[i].Name < file.Presets[j].Name
		}
		return file.Presets[i].Category < file.Presets[j].Category
	})
	if file.Presets == nil {
		file.Presets = []models.MiddlewarePreset{}
	}
	return file.Presets, nil
}

// GetSSHHosts 获取 SSH 主机列表。
func (a *App) GetSSHHosts() ([]models.SSHHost, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	list, err := a.metadata.ListSSHHosts()
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []models.SSHHost{}
	}
	return list, nil
}

// SaveSSHHost 保存 SSH 主机。
func (a *App) SaveSSHHost(req models.SaveSSHHostRequest) error {
	if err := a.ready(); err != nil {
		return err
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Host = strings.TrimSpace(req.Host)
	req.Username = strings.TrimSpace(req.Username)
	if req.Host == "" || req.Username == "" {
		return errs.InvalidInput("主机地址与用户名不能为空", nil)
	}
	if req.Name == "" {
		req.Name = req.Host
	}
	if req.Port <= 0 {
		req.Port = 22
	}
	if req.Port > 65535 {
		return errs.InvalidInput("端口超出范围", nil)
	}
	if req.AuthMethod == "" {
		req.AuthMethod = "password"
	}
	if req.AuthMethod != "password" && req.AuthMethod != "key" {
		return errs.InvalidInput("不支持的认证方式", nil)
	}

	var existingHost *models.SSHHost
	var encPwd string
	if req.ID != "" {
		host, enc, err := a.metadata.GetSSHHost(req.ID)
		if err == nil {
			existingHost = host
			encPwd = enc
		}
	}
	if req.Password != "" {
		enc, err := a.cipher.Encrypt(req.Password)
		if err != nil {
			return errs.Internal("加密 SSH 密码失败", err)
		}
		encPwd = enc
	}
	if req.AuthMethod == "password" && encPwd == "" {
		return errs.InvalidInput("密码认证需要填写密码", nil)
	}
	if req.AuthMethod == "key" && strings.TrimSpace(req.KeyPath) == "" {
		return errs.InvalidInput("密钥认证需要填写密钥路径", nil)
	}

	id := req.ID
	if id == "" {
		id = newID()
	}
	host := models.SSHHost{
		ID:          id,
		Name:        req.Name,
		Host:        req.Host,
		Port:        req.Port,
		Username:    req.Username,
		AuthMethod:  req.AuthMethod,
		KeyPath:     strings.TrimSpace(req.KeyPath),
		ProxyJump:   strings.TrimSpace(req.ProxyJump),
		HasPassword: encPwd != "",
	}
	if existingHost != nil {
		host.LastUsedAt = existingHost.LastUsedAt
	}
	return a.metadata.SaveSSHHost(&host, encPwd)
}

// DeleteSSHHost 删除 SSH 主机。
func (a *App) DeleteSSHHost(hostID string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.metadata.DeleteSSHHost(hostID)
}

// TestSSHConnection 测试 SSH 连接。
func (a *App) TestSSHConnection(hostID string) error {
	if err := a.ready(); err != nil {
		return err
	}
	cfg := a.configMgr.Config()
	opts, _, err := a.sshOptionsFromHost(hostID, time.Duration(cfg.Collector.SSHTimeoutSeconds)*time.Second)
	if err != nil {
		return err
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 30 * time.Second
	}
	return collector.TestSSHConnection(opts)
}

// sshOptionsFromHost 读取元数据中的 SSH 主机配置并解密密码。
// StartSSHCollection、TestSSHConnection 与 SearchSSHLogs 共用，保持行为一致。
func (a *App) sshOptionsFromHost(hostID string, timeout time.Duration) (collector.SSHOptions, *models.SSHHost, error) {
	host, encPwd, err := a.metadata.GetSSHHost(hostID)
	if err != nil {
		return collector.SSHOptions{}, nil, err
	}
	password := ""
	if encPwd != "" {
		password, err = a.cipher.Decrypt(encPwd)
		if err != nil {
			return collector.SSHOptions{}, nil, errs.Internal("解密 SSH 密码失败", err)
		}
	}
	return collector.SSHOptions{
		Host:      host.Host,
		Port:      host.Port,
		Username:  host.Username,
		Password:  password,
		KeyPath:   host.KeyPath,
		ProxyJump: host.ProxyJump,
		Timeout:   timeout,
	}, host, nil
}

// SearchSSHLogs 在远程 Linux 主机上检索日志文件，供前端"一键采集系统日志 /
// 关键词查找 / 全盘检索 / 选择后拉取"使用。检索为只读操作，不要求已打开项目。
func (a *App) SearchSSHLogs(req models.SSHLogSearchRequest) (*models.SSHLogSearchResult, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	hostID := strings.TrimSpace(req.HostID)
	if hostID == "" {
		return nil, errs.InvalidInput("请选择 SSH 主机", nil)
	}

	cfg := a.configMgr.Config()
	opts, host, err := a.sshOptionsFromHost(hostID, time.Duration(cfg.Collector.SSHTimeoutSeconds)*time.Second)
	if err != nil {
		return nil, err
	}

	// 普通扫描 2 分钟；全盘扫描范围大，放宽到 5 分钟。
	timeout := 2 * time.Minute
	if req.FullScan {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	startedAt := time.Now()
	files, truncated, err := collector.SearchSSHLogs(ctx, opts, collector.SSHLogSearchOptions{
		Roots:          req.Roots,
		FullScan:       req.FullScan,
		Keyword:        req.Keyword,
		SearchContent:  req.SearchContent,
		IncludeRotated: req.IncludeRotated,
		MaxResults:     req.MaxResults,
	})
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			limit := "2 分钟"
			if req.FullScan {
				limit = "5 分钟"
			}
			return nil, errs.New(errs.CodeSSHFailed,
				fmt.Sprintf("远程日志检索超时（超过 %s），可缩小检索目录或关键词范围后重试", limit), err)
		}
		if ctx.Err() == context.Canceled {
			return nil, errs.New(errs.CodeSSHFailed, "远程日志检索已取消", err)
		}
		if errors.Is(err, errs.ErrInvalidInput) {
			return nil, errs.New(errs.CodeInvalidInput, "检索参数无效", err)
		}
		label := host.Host
		if host.Name != "" {
			label = host.Name
		}
		if errors.Is(err, errs.ErrSSHConnectionFailed) {
			return nil, errs.New(errs.CodeSSHFailed,
				fmt.Sprintf("连接 SSH 主机 %s 失败", label), err)
		}
		return nil, errs.New(errs.CodeSSHFailed,
			fmt.Sprintf("在 %s 上检索远程日志失败", label), err)
	}

	if files == nil {
		files = []models.SSHRemoteLogFile{}
	}
	return &models.SSHLogSearchResult{
		Files:     files,
		Truncated: truncated,
		TookMS:    time.Since(startedAt).Milliseconds(),
	}, nil
}

func truncateForSource(s string) string {
	const max = 500
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}

// ---------- 采集流水线 ----------

type ingestion struct {
	app        *App
	ctx        context.Context
	projectID  string
	sourceID   string
	sourceHost string
	sourceType string
	parseCfg   models.ParseConfig
	filter     entryFilter
	report     taskReporter
	store      *indexer.Store

	mu         sync.Mutex
	tpl        *models.ParseTemplate
	buffered   []models.RawLine
	samples    []string
	assemblers map[string]*parser.MultilineAssembler
	batch      []*models.LogEntry

	processed   int64
	parsedCount int64
	failedCount int64
	bytes       int64
	levels      map[string]int64
	first       time.Time
	last        time.Time
	startedAt   time.Time
}

func (a *App) runIngestion(
	ctx context.Context,
	projectID, sourceID, sourceType, sourceHost, sourcePath string,
	col collector.Collector,
	parseCfg models.ParseConfig,
	filter entryFilter,
	report taskReporter,
) (*models.CollectResult, error) {
	a.mu.RLock()
	if a.currentProject == nil || a.indexStore == nil || a.currentProject.ID != projectID {
		a.mu.RUnlock()
		return nil, errs.ErrProjectNotOpen
	}
	store := a.indexStore
	a.mu.RUnlock()

	a.collectMu.Lock()
	defer a.collectMu.Unlock()

	startedAt := time.Now()
	ing := &ingestion{
		app:        a,
		ctx:        ctx,
		projectID:  projectID,
		sourceID:   sourceID,
		sourceHost: sourceHost,
		sourceType: sourceType,
		parseCfg:   parseCfg,
		filter:     filter,
		report:     report,
		store:      store,
		levels:     map[string]int64{},
		assemblers: map[string]*parser.MultilineAssembler{},
		startedAt:  startedAt,
	}
	if report != nil {
		report(0, 0, "正在读取日志…", 0.03)
	}

	colResult, colErr := col.Collect(ctx, ing.feed)

	ing.mu.Lock()
	if ing.tpl == nil && len(ing.samples) > 0 {
		if err := ing.resolveTemplateLocked(); err == nil {
			buffered := ing.buffered
			ing.buffered = nil
			for _, raw := range buffered {
				if err := ing.processRawLocked(raw); err != nil {
					logx.Warn("处理缓冲日志失败", zap.Error(err))
					break
				}
			}
		} else {
			logx.Warn("无法识别日志格式", zap.Error(err))
		}
	}
	ing.flushAssemblersLocked()
	finalizeErr := ing.flushBatchLocked()
	ing.mu.Unlock()

	if finalizeErr != nil {
		return nil, finalizeErr
	}
	if err := store.Finalize(); err != nil {
		return nil, errs.Internal("写入索引失败", err)
	}

	totalBytes := ing.bytes
	fileCount := 0
	var collectErrors []string
	if colResult != nil {
		fileCount = len(colResult.Files)
		if colResult.TotalBytes > 0 {
			totalBytes = colResult.TotalBytes
		}
		collectErrors = append(collectErrors, colResult.Errors...)
	}
	if colErr != nil && ctx.Err() == nil {
		collectErrors = append(collectErrors, colErr.Error())
	}

	if ing.parsedCount > 0 {
		var tr *models.TimeRange
		if !ing.first.IsZero() && !ing.last.IsZero() {
			tr = &models.TimeRange{Start: ing.first, End: ing.last}
		}
		src := &models.LogSource{
			ID:         sourceID,
			Type:       sourceType,
			Host:       sourceHost,
			Path:       sourcePath,
			ParseMode:  parseCfg.Mode,
			TemplateID: "",
			LogCount:   ing.parsedCount,
			SizeBytes:  totalBytes,
			TimeRange:  tr,
			CreatedAt:  nowUTC(),
		}
		if ing.tpl != nil {
			src.TemplateID = ing.tpl.ID
		}
		if parseCfg.Mode == "ai" {
			src.AIProvider = parseCfg.AIProvider
		}
		if err := a.projectsSvc.RecordCollection(projectID, src, ing.parsedCount, totalBytes, tr, ing.levels); err != nil {
			logx.Warn("更新项目统计失败", zap.Error(err))
			collectErrors = append(collectErrors, "更新项目统计失败: "+err.Error())
		} else {
			if fresh, err := a.projectsSvc.Get(projectID); err == nil {
				a.mu.Lock()
				if a.currentProject != nil && a.currentProject.ID == projectID {
					a.currentProject = fresh
				}
				a.mu.Unlock()
			}
		}
	}

	status := "success"
	if ctx.Err() != nil {
		status = "stopped"
	} else if len(collectErrors) > 0 {
		if ing.parsedCount > 0 {
			status = "partial"
		} else {
			status = "failed"
		}
	} else if ing.parsedCount == 0 {
		status = "failed"
		collectErrors = append(collectErrors, "未解析到任何日志")
	}

	result := &models.CollectResult{
		ProjectID:   projectID,
		SourceID:    sourceID,
		SourceIDs:   []string{sourceID},
		FileCount:   fileCount,
		TotalBytes:  totalBytes,
		LogCount:    ing.parsedCount,
		FailedCount: ing.failedCount,
		Duration:    time.Since(startedAt).Milliseconds(),
		Status:      status,
		FinishedAt:  nowUTC(),
	}
	if len(collectErrors) > 0 {
		const maxErr = 5
		shown := collectErrors
		if len(shown) > maxErr {
			shown = shown[:maxErr]
		}
		result.ErrorMessage = strings.Join(shown, "；")
	}
	if report != nil {
		report(ing.processed, 0, "正在写入索引…", 0.98)
	}
	if status == "failed" {
		if result.ErrorMessage == "" {
			result.ErrorMessage = "未解析到任何日志"
		}
		return result, errs.New(errs.CodeParseFailed, "采集失败: "+result.ErrorMessage, nil)
	}
	return result, nil
}

func (ing *ingestion) feed(raw models.RawLine) error {
	ing.mu.Lock()
	defer ing.mu.Unlock()
	return ing.feedLocked(raw)
}

func (ing *ingestion) feedLocked(raw models.RawLine) error {
	ing.processed++
	size := raw.Size
	if size <= 0 {
		size = int64(len(raw.Text)) + 1
	}
	ing.bytes += size
	if ing.filter.filePattern != nil && !ing.filter.matchFile(raw.FilePath) {
		return nil
	}
	if ing.tpl == nil {
		ing.buffered = append(ing.buffered, raw)
		sample := raw.Text
		if len(sample) > 4000 {
			sample = sample[:4000]
		}
		ing.samples = append(ing.samples, sample)
		sampleSize := ing.parseCfg.SampleSize
		if sampleSize <= 0 {
			sampleSize = 40
		}
		if len(ing.samples) >= sampleSize {
			if err := ing.resolveTemplateLocked(); err != nil {
				return err
			}
			buffered := ing.buffered
			ing.buffered = nil
			for _, bufferedRaw := range buffered {
				if err := ing.processRawLocked(bufferedRaw); err != nil {
					return err
				}
			}
		}
	} else {
		if err := ing.processRawLocked(raw); err != nil {
			return err
		}
	}

	if ing.processed%500 == 0 && ing.report != nil {
		progress := 0.05 + 0.85*(1-1/(1+float64(ing.processed)/5000))
		if progress > 0.9 {
			progress = 0.9
		}
		ing.report(ing.processed, 0, fmt.Sprintf("已处理 %d 行，解析 %d 条", ing.processed, ing.parsedCount), progress)
	}
	return nil
}

func (ing *ingestion) processRawLocked(raw models.RawLine) error {
	if ing.tpl == nil {
		return nil
	}
	if ing.tpl.MultiLine != nil && ing.tpl.MultiLine.Enabled {
		key := raw.FilePath
		if key == "" {
			key = "_"
		}
		asm := ing.assemblers[key]
		if asm == nil {
			asm = parser.NewMultilineAssembler(ing.tpl.MultiLine)
			ing.assemblers[key] = asm
		}
		text, ready := asm.Add(raw.Text)
		if !ready {
			return nil
		}
		return ing.parseEntryLocked(text, raw)
	}
	return ing.parseEntryLocked(raw.Text, raw)
}

// autoTemplateFallback 判断当前采集是否使用自动模板识别（未显式指定模板）。
// native_evtx/ai 模式不参与逐行回退，避免绕过专用解析器或 AI 模板。
func (ing *ingestion) autoTemplateFallback() bool {
	mode := ing.parseCfg.Mode
	if mode == "" {
		mode = "builtin"
	}
	return mode == "builtin" && ing.parseCfg.TemplateID == ""
}

func (ing *ingestion) parseEntryLocked(text string, raw models.RawLine) error {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	parseCtx := parser.ParseContext{
		SourceID:    ing.sourceID,
		SourceHost:  ing.sourceHost,
		SourceFile:  raw.FilePath,
		Offset:      raw.Offset,
		CollectedAt: raw.CollectedAt,
	}
	entry, err := ing.app.parserEngine.Parse(text, ing.tpl, parseCtx)
	if err != nil && ing.autoTemplateFallback() {
		// 自动模板模式常用于目录/多文件采集：单一模板可能只匹配其中一种格式
		// （如 syslog + Nginx 混合日志），失败时逐行回退到全部内置模板。
		if fallback, _, fallbackErr := ing.app.parserEngine.ParseWithBest(text, parseCtx); fallbackErr == nil {
			entry, err = fallback, nil
		}
	}
	if err != nil {
		ing.failedCount++
		return nil
	}
	if !ing.filter.matchEntry(entry) {
		return nil
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = raw.CollectedAt
		if entry.Timestamp.IsZero() {
			entry.Timestamp = nowUTC()
		}
	}
	entry.Source = ing.sourceID
	if entry.SourceHost == "" {
		entry.SourceHost = ing.sourceHost
	}
	if entry.SourceFile == "" {
		entry.SourceFile = raw.FilePath
	}
	if entry.Level == "" {
		entry.Level = models.LevelUnknown
	}
	if ing.first.IsZero() || entry.Timestamp.Before(ing.first) {
		ing.first = entry.Timestamp
	}
	if ing.last.IsZero() || entry.Timestamp.After(ing.last) {
		ing.last = entry.Timestamp
	}
	ing.levels[entry.Level]++
	ing.batch = append(ing.batch, entry)
	if len(ing.batch) >= batchFlushSize {
		return ing.flushBatchLocked()
	}
	return nil
}

func (ing *ingestion) flushBatchLocked() error {
	if len(ing.batch) == 0 {
		return nil
	}
	entries := ing.batch
	ing.batch = nil
	if err := ing.store.Append(entries); err != nil {
		return errs.Internal("写入日志存储失败", err)
	}
	ing.parsedCount += int64(len(entries))
	if ing.report != nil {
		ing.report(ing.processed, 0, fmt.Sprintf("已解析 %d 条日志", ing.parsedCount), 0.5)
	}
	return nil
}

func (ing *ingestion) flushAssemblersLocked() {
	for key, asm := range ing.assemblers {
		text := asm.Flush()
		if strings.TrimSpace(text) == "" {
			continue
		}
		raw := models.RawLine{FilePath: key, CollectedAt: nowUTC()}
		if err := ing.parseEntryLocked(text, raw); err != nil {
			logx.Warn("刷新多行日志失败", zap.String("file", key), zap.Error(err))
		}
	}
}

func (ing *ingestion) resolveTemplateLocked() error {
	if ing.tpl != nil {
		return nil
	}
	mode := ing.parseCfg.Mode
	if mode == "" {
		mode = "builtin"
	}

	if mode == "native_evtx" || ing.parseCfg.TemplateID == "windows-evtx" {
		tpl, err := ing.app.templateByID("windows-evtx")
		if err == nil {
			ing.tpl = tpl
			return nil
		}
	}

	if mode == "ai" && ing.app.aiMgr != nil && len(ing.samples) > 0 {
		if provider, err := ing.app.aiMgr.SelectedProvider(ing.parseCfg.AIProvider); err == nil {
			ctx := ing.ctx
			if ctx == nil {
				ctx = context.Background()
			}
			complete := func(callCtx context.Context, system, prompt string) (string, error) {
				return ing.app.aiMgr.Complete(callCtx, provider, system, prompt, true)
			}
			tpl, genErr := ing.app.parserEngine.GenerateTemplateWithAI(ctx, ing.samples, "AI 采集模板", "", complete)
			if genErr == nil && tpl != nil {
				if tpl.ID == "" {
					tpl.ID = "ai-" + newID()
				}
				tpl.Type = "ai_generated"
				tpl.Source = "ai"
				tpl.CreatedAt = nowUTC()
				tpl.UpdatedAt = nowUTC()
				if ing.parseCfg.SaveAITemplate {
					if err := ing.app.metadata.SaveTemplate(tpl); err != nil {
						logx.Warn("保存 AI 模板失败", zap.Error(err))
					}
				}
				ing.tpl = tpl
				ing.parseCfg.TemplateID = tpl.ID
				logx.Info("AI 模板已生成", zap.String("template", tpl.ID))
				return nil
			}
			logx.Warn("AI 生成模板失败，回退自动检测", zap.Error(genErr))
		} else {
			logx.Warn("AI 提供商不可用，回退自动检测", zap.Error(err))
		}
	}

	if ing.parseCfg.TemplateID != "" {
		tpl, err := ing.app.templateByID(ing.parseCfg.TemplateID)
		if err != nil {
			return err
		}
		ing.tpl = tpl
		return nil
	}

	if len(ing.samples) == 0 {
		return errs.ErrParseFormatUnknown
	}
	if tpl, confidence := ing.app.parserEngine.DetectTemplate(ing.samples); tpl != nil {
		ing.tpl = tpl
		logx.Info("自动识别模板", zap.String("template", tpl.ID), zap.Float64("confidence", confidence))
		return nil
	}
	for _, fallback := range []string{"json-generic", "syslog-rfc3164"} {
		if tpl, err := ing.app.templateByID(fallback); err == nil {
			ing.tpl = tpl
			return nil
		}
	}
	return errs.ErrParseFormatUnknown
}

func (a *App) templateByID(id string) (*models.ParseTemplate, error) {
	if id == "" {
		return nil, errs.ErrParseTemplateMissing
	}
	if tpl, err := a.metadata.GetTemplate(id); err == nil && tpl != nil {
		return tpl, nil
	}
	if tpl, ok := a.parserEngine.Template(id); ok {
		return tpl, nil
	}
	return nil, errs.ErrParseTemplateMissing
}
