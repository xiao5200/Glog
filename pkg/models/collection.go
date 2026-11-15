package models

import "time"

// ParseConfig 采集时的解析配置。
type ParseConfig struct {
	Mode           string `json:"mode"` // builtin/ai/native_evtx
	TemplateID     string `json:"template_id,omitempty"`
	AIProvider     string `json:"ai_provider,omitempty"`
	SaveAITemplate bool   `json:"save_template"`
	SampleSize     int    `json:"sample_size,omitempty"`
}

// LocalCollectRequest 本地采集请求。
type LocalCollectRequest struct {
	ProjectID  string   `json:"project_id"`
	FilePaths  []string `json:"file_paths"`
	Recursive  bool     `json:"recursive"`
	SourceType string   `json:"source_type"` // file/directory/evtx
	Mode       string   `json:"mode"`        // one_time/continuous

	FilePattern  string   `json:"file_pattern,omitempty"`
	ExcludePaths []string `json:"exclude_paths,omitempty"`

	Concurrency int `json:"concurrency"`
	BufferSize  int `json:"buffer_size"`

	Parse ParseConfig `json:"parse"`
}

// SSHCollectRequest SSH 采集请求。
type SSHCollectRequest struct {
	ProjectID     string   `json:"project_id"`
	HostID        string   `json:"host_id"`
	RemotePaths   []string `json:"remote_paths"`
	Strategy      string   `json:"strategy"` // compress/filter/journalctl/incremental
	FilterPattern string   `json:"filter_pattern,omitempty"`
	Compression   string   `json:"compression"` // gzip/zstd/none
	Concurrency   int      `json:"concurrency"`
	Since         string   `json:"since,omitempty"`
	Until         string   `json:"until,omitempty"`
	Unit          string   `json:"unit,omitempty"`

	Parse ParseConfig `json:"parse"`
}

// SSHLogSearchRequest 远程 Linux 日志查找请求。
type SSHLogSearchRequest struct {
	HostID         string   `json:"host_id"`
	Roots          []string `json:"roots,omitempty"` // 为空默认 /var/log；FullScan=true 时忽略并扫描 /
	FullScan       bool     `json:"full_scan"`
	Keyword        string   `json:"keyword,omitempty"`
	SearchContent  bool     `json:"search_content"`  // true=搜索文件内容；false=按路径匹配
	IncludeRotated bool     `json:"include_rotated"` // 是否包含 .gz/.1 等轮转压缩日志
	MaxResults     int      `json:"max_results,omitempty"`
}

// SSHRemoteLogFile 远程日志检索命中的文件。
type SSHRemoteLogFile struct {
	Path    string    `json:"path"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
}

// SSHLogSearchResult 远程日志检索结果。
type SSHLogSearchResult struct {
	Files     []SSHRemoteLogFile `json:"files"`
	Truncated bool               `json:"truncated"`
	TookMS    int64              `json:"took_ms"`
}

// WindowsEventLogRequest Windows 事件日志采集请求。
type WindowsEventLogRequest struct {
	ProjectID string     `json:"project_id"`
	Channels  []string   `json:"channels"`
	StartTime *time.Time `json:"start_time,omitempty"`
	EndTime   *time.Time `json:"end_time,omitempty"`
	EventIDs  []uint32   `json:"event_ids,omitempty"`
	Levels    []string   `json:"levels,omitempty"`

	Parse ParseConfig `json:"parse"`
}

// SystemdJournalRequest Linux 本地 journalctl 采集请求。
type SystemdJournalRequest struct {
	ProjectID string `json:"project_id"`
	Since     string `json:"since,omitempty"`
	Until     string `json:"until,omitempty"`
	Unit      string `json:"unit,omitempty"`
	Priority  string `json:"priority,omitempty"`
	Lines     int    `json:"lines,omitempty"`
	Follow    bool   `json:"follow"`

	Parse ParseConfig `json:"parse"`
}

// ImportFileRequest 导入文件请求（文本/压缩/evtx 自动识别）。
type ImportFileRequest struct {
	ProjectID string      `json:"project_id"`
	FilePath  string      `json:"file_path"`
	Parse     ParseConfig `json:"parse"`
}

// CollectResult 采集结果。
type CollectResult struct {
	TaskID       string    `json:"task_id"`
	ProjectID    string    `json:"project_id"`
	SourceID     string    `json:"source_id"`
	SourceIDs    []string  `json:"source_ids"`
	FileCount    int       `json:"file_count"`
	TotalBytes   int64     `json:"total_bytes"`
	LogCount     int64     `json:"log_count"`
	FailedCount  int64     `json:"failed_count"`
	Duration     int64     `json:"duration"`
	Status       string    `json:"status"` // success/failed/partial/stopped
	ErrorMessage string    `json:"error_message,omitempty"`
	FinishedAt   time.Time `json:"finished_at"`
}

// TaskInfo 异步任务信息。
type TaskInfo struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"` // local/ssh/evtx/import/analysis/rule
	ProjectID  string         `json:"project_id"`
	SourceID   string         `json:"source_id,omitempty"`
	Status     string         `json:"status"` // pending/running/success/failed/stopped
	Progress   float64        `json:"progress"`
	Message    string         `json:"message"`
	Processed  int64          `json:"processed"`
	Total      int64          `json:"total"`
	Error      string         `json:"error,omitempty"`
	StartedAt  time.Time      `json:"started_at"`
	FinishedAt *time.Time     `json:"finished_at,omitempty"`
	Result     *CollectResult `json:"result,omitempty"`
}

// SSHHost SSH 主机配置（对外视图，不回传密码）。
type SSHHost struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Host        string     `json:"host"`
	Port        int        `json:"port"`
	Username    string     `json:"username"`
	AuthMethod  string     `json:"auth_method"` // password/key
	KeyPath     string     `json:"key_path,omitempty"`
	ProxyJump   string     `json:"proxy_jump,omitempty"`
	HasPassword bool       `json:"has_password"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
}

// SaveSSHHostRequest 保存 SSH 主机（密码明文仅在此结构出现，后端加密存储）。
type SaveSSHHostRequest struct {
	ID         string `json:"id,omitempty"`
	Name       string `json:"name"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Username   string `json:"username"`
	AuthMethod string `json:"auth_method"`
	Password   string `json:"password,omitempty"`
	KeyPath    string `json:"key_path,omitempty"`
	ProxyJump  string `json:"proxy_jump,omitempty"`
}

// FileChangeEvent 文件变化事件。
type FileChangeEvent struct {
	Path      string `json:"path"`
	EventType string `json:"event_type"` // write/create/remove
}

// CollectionProgress 采集进度事件负载。
type CollectionProgress struct {
	TaskID    string  `json:"task_id"`
	Progress  float64 `json:"progress"`
	Message   string  `json:"message"`
	Processed int64   `json:"processed"`
	Total     int64   `json:"total"`
}
