package models

import "time"

// DetectedLog 本机日志发现结果。
type DetectedLog struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Path    string `json:"path"`
	Type    string `json:"type"` // file/evtx/eventlog/journal
	Channel string `json:"channel,omitempty"`

	Category  string    `json:"category"` // auth/web_server/database/windows/application/user/other
	SizeBytes int64     `json:"size_bytes"`
	ModTime   time.Time `json:"mod_time"`

	Readable   bool   `json:"readable"`
	TemplateID string `json:"template_id,omitempty"`
	Format     string `json:"format,omitempty"`
	Preview    string `json:"preview,omitempty"`
}

// LocalLogDiscovery 一次本机日志扫描结果。
type LocalLogDiscovery struct {
	ScannedAt time.Time     `json:"scanned_at"`
	Roots     []string      `json:"roots"`
	Items     []DetectedLog `json:"items"`
	Errors    []string      `json:"errors"`
	Truncated bool          `json:"truncated"`
}
