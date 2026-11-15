package models

import "time"

// SystemInfo 系统信息。
type SystemInfo struct {
	OS           string `json:"os"`
	Arch         string `json:"arch"`
	Version      string `json:"version"`
	DataPath     string `json:"data_path"`
	ProjectsPath string `json:"projects_path"`
	TempPath     string `json:"temp_path"`
	LogPath      string `json:"log_path"`
	DBPath       string `json:"db_path"`
	Executable   string `json:"executable"`
}

// AppConfig 全局配置（同时用于前端读写）。
type AppConfig struct {
	General     GeneralConfig     `json:"general" toml:"general"`
	Performance PerformanceConfig `json:"performance" toml:"performance"`
	Collector   CollectorConfig   `json:"collector" toml:"collector"`
	Indexer     IndexerConfig     `json:"indexer" toml:"indexer"`
	Parser      ParserConfig      `json:"parser" toml:"parser"`
	AI          AIConfigSection   `json:"ai" toml:"ai"`
	UI          UIConfig          `json:"ui" toml:"ui"`
}

type GeneralConfig struct {
	Version  string `json:"version" toml:"version"`
	DataPath string `json:"data_path" toml:"data_path"`
	TempPath string `json:"temp_path" toml:"temp_path"`
	LogPath  string `json:"log_path" toml:"log_path"`
}

type PerformanceConfig struct {
	MaxGoroutines        int `json:"max_goroutines" toml:"max_goroutines"`
	ParserWorkers        int `json:"parser_workers" toml:"parser_workers"`
	IndexerWorkers       int `json:"indexer_workers" toml:"indexer_workers"`
	AITaskWorkers        int `json:"ai_task_workers" toml:"ai_task_workers"`
	MaxMemoryMB          int `json:"max_memory_mb" toml:"max_memory_mb"`
	CacheSizeMB          int `json:"cache_size_mb" toml:"cache_size_mb"`
	LRUCacheItems        int `json:"lru_cache_items" toml:"lru_cache_items"`
	QueryTimeoutSeconds  int `json:"query_timeout_seconds" toml:"query_timeout_seconds"`
	QueryCacheTTLMinutes int `json:"query_cache_ttl_minutes" toml:"query_cache_ttl_minutes"`
}

type CollectorConfig struct {
	SSHTimeoutSeconds int    `json:"ssh_timeout_seconds" toml:"ssh_timeout_seconds"`
	SSHConcurrency    int    `json:"ssh_concurrency" toml:"ssh_concurrency"`
	SSHCompression    string `json:"ssh_compression" toml:"ssh_compression"`
	LocalBufferSizeKB int    `json:"local_buffer_size_kb" toml:"local_buffer_size_kb"`
	LocalBatchSize    int    `json:"local_batch_size" toml:"local_batch_size"`
}

type IndexerConfig struct {
	IndexFlushIntervalSeconds int    `json:"index_flush_interval_seconds" toml:"index_flush_interval_seconds"`
	IndexBatchSize            int    `json:"index_batch_size" toml:"index_batch_size"`
	CompressionAlgorithm      string `json:"compression_algorithm" toml:"compression_algorithm"`
	BloomFilterSize           int    `json:"bloom_filter_size" toml:"bloom_filter_size"`
	BloomFilterHashFunctions  int    `json:"bloom_filter_hash_functions" toml:"bloom_filter_hash_functions"`
}

type ParserConfig struct {
	MultiLineTimeoutMs int    `json:"multiline_timeout_ms" toml:"multiline_timeout_ms"`
	MultiLineMaxLines  int    `json:"multiline_max_lines" toml:"multiline_max_lines"`
	DefaultTimeFormat  string `json:"default_time_format" toml:"default_time_format"`
}

type AIConfigSection struct {
	DefaultProvider       string `json:"default_provider" toml:"default_provider"`
	DefaultSampleSize     int    `json:"default_sample_size" toml:"default_sample_size"`
	MaxSampleSize         int    `json:"max_sample_size" toml:"max_sample_size"`
	RequestTimeoutSeconds int    `json:"request_timeout_seconds" toml:"request_timeout_seconds"`
	MaxLogsPerRequest     int    `json:"max_logs_per_request" toml:"max_logs_per_request"`
}

type UIConfig struct {
	Theme    string `json:"theme" toml:"theme"`
	Language string `json:"language" toml:"language"`
	PageSize int    `json:"log_page_size" toml:"log_page_size"`
	Overscan int    `json:"virtual_scroll_overscan" toml:"virtual_scroll_overscan"`
}

// DashboardInfo 应用首页汇总。
type DashboardInfo struct {
	ProjectCount int        `json:"project_count"`
	OpenProject  string     `json:"open_project,omitempty"`
	RecentTasks  []TaskInfo `json:"recent_tasks"`
	GeneratedAt  time.Time  `json:"generated_at"`
}

// HostPort 当前主机的监听端口。
type HostPort struct {
	Protocol     string `json:"protocol"` // TCP / UDP
	LocalAddress string `json:"local_address"`
	LocalPort    int    `json:"local_port"`
	Process      string `json:"process,omitempty"`
	PID          int    `json:"pid,omitempty"`
	Exposed      bool   `json:"exposed"` // 绑定 0.0.0.0 / ::，可能对外暴露
}

// HostConnection 当前主机的网络连接（重点为外联）。
type HostConnection struct {
	Protocol      string `json:"protocol"`
	LocalAddress  string `json:"local_address"`
	LocalPort     int    `json:"local_port"`
	RemoteAddress string `json:"remote_address"`
	RemotePort    int    `json:"remote_port"`
	State         string `json:"state"`
	Process       string `json:"process,omitempty"`
	PID           int    `json:"pid,omitempty"`
	External      bool   `json:"external"` // 远端为公网/非内网地址
}

// HostNetworkSnapshot 当前主机网络暴露面快照。
type HostNetworkSnapshot struct {
	GeneratedAt time.Time        `json:"generated_at"`
	Hostname    string           `json:"hostname"`
	Platform    string           `json:"platform"`
	Arch        string           `json:"arch"`
	Ports       []HostPort       `json:"ports"`
	Connections []HostConnection `json:"connections"`
	ExternalIPs []string         `json:"external_ips"`
	Errors      []string         `json:"errors"`
	Truncated   bool             `json:"truncated"`
	TotalPorts  int              `json:"total_ports"`
	TotalConns  int              `json:"total_connections"`
}
