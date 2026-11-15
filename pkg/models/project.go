package models

import "time"

// Project 项目模型。
type Project struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Status      string    `json:"status"` // active/archived

	ProjectPath string `json:"project_path"`

	LogCount  int64            `json:"log_count"`
	SizeBytes int64            `json:"size_bytes"`
	TimeRange *TimeRange       `json:"time_range,omitempty"`
	Sources   []LogSource      `json:"sources"`
	Levels    map[string]int64 `json:"level_distribution"`
	Tags      []string         `json:"tags,omitempty"`
	Priority  string           `json:"priority"` // high/medium/low
}

// LogSource 日志源。
type LogSource struct {
	ID         string     `json:"id"`
	Type       string     `json:"type"` // local/ssh/evtx/import
	Host       string     `json:"host"`
	Path       string     `json:"path"`
	ParseMode  string     `json:"parse_mode"` // builtin/ai/native_evtx
	TemplateID string     `json:"template_id"`
	LogCount   int64      `json:"log_count"`
	SizeBytes  int64      `json:"size_bytes"`
	TimeRange  *TimeRange `json:"time_range,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	AIProvider string     `json:"ai_provider,omitempty"`
}

// CreateProjectRequest 创建项目请求。
type CreateProjectRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Priority    string   `json:"priority"`
}

// ProjectFilter 项目筛选器。
type ProjectFilter struct {
	Status   string   `json:"status"` // active/archived/all
	Tags     []string `json:"tags"`
	SortBy   string   `json:"sort_by"` // created_at/updated_at/log_count/size/name
	SortDesc bool     `json:"sort_desc"`
	Search   string   `json:"search"`
}

// ProjectUpdates 项目更新（nil 表示不修改）。
type ProjectUpdates struct {
	Name        *string   `json:"name"`
	Description *string   `json:"description"`
	Tags        *[]string `json:"tags"`
	Priority    *string   `json:"priority"`
	Status      *string   `json:"status"`
}

// HostStat 主机统计。
type HostStat struct {
	Host     string `json:"host"`
	LogCount int64  `json:"log_count"`
}

// ProjectStatistics 项目统计。
type ProjectStatistics struct {
	TotalLogs         int64            `json:"total_logs"`
	SizeBytes         int64            `json:"size_bytes"`
	TimeRange         *TimeRange       `json:"time_range,omitempty"`
	LevelDistribution map[string]int64 `json:"level_distribution"`
	TopHosts          []HostStat       `json:"top_hosts"`
	SourceCount       int              `json:"source_count"`
	IndexSizeBytes    int64            `json:"index_size_bytes"`
	UniqueTerms       int64            `json:"unique_terms"`
}

// FileInfo 本地文件信息。
type FileInfo struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	IsDir   bool      `json:"is_dir"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
}

// MiddlewarePreset 中间件预设。
type MiddlewarePreset struct {
	Name         string   `json:"name" yaml:"name"`
	Description  string   `json:"description" yaml:"description"`
	Category     string   `json:"category" yaml:"category"`
	WindowsPaths []string `json:"windows_paths" yaml:"windows_paths"`
	LinuxPaths   []string `json:"linux_paths" yaml:"linux_paths"`
	TemplateID   string   `json:"template_id" yaml:"template_id"`
	MultiLine    bool     `json:"multiline" yaml:"multiline"`
}
