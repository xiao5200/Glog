package models

import "time"

// LogLevel 是日志级别的字符串别名，保证 JSON/Wails 绑定直通 string。
type LogLevel = string

const (
	LevelDebug    = "DEBUG"
	LevelInfo     = "INFO"
	LevelWarn     = "WARN"
	LevelError    = "ERROR"
	LevelCritical = "CRITICAL"
	LevelFatal    = "FATAL"
	LevelUnknown  = "UNKNOWN"
)

// LogLevelPriority 日志级别优先级映射（越大越严重）。
var LogLevelPriority = map[string]int{
	LevelDebug:    0,
	LevelInfo:     1,
	LevelWarn:     2,
	LevelError:    3,
	LevelCritical: 4,
	LevelFatal:    5,
	LevelUnknown:  -1,
}

// NormalizeLevel 将各种平台的原始级别归一化为标准级别。
func NormalizeLevel(raw string) LogLevel {
	switch normalizeLower(raw) {
	case "debug", "trace", "verbose", "d", "7":
		return LevelDebug
	case "info", "information", "notice", "i", "6", "informational":
		return LevelInfo
	case "warn", "warning", "w", "4":
		return LevelWarn
	case "error", "err", "e", "3":
		return LevelError
	case "critical", "crit", "fatal", "alert", "emerg", "panic", "2", "1", "0":
		return LevelCritical
	default:
		return LevelUnknown
	}
}

func normalizeLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

// LogEntry 标准化日志条目。
type LogEntry struct {
	// DocID 是项目内连续分配的内部编号，查询/证据/上下文均使用它。
	DocID uint64 `json:"doc_id"`
	// ID 为对外字符串编号（DocID 的十进制形式）。
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Level     LogLevel  `json:"level"`
	Message   string    `json:"message"`

	// 来源信息
	Source       string `json:"source"`
	SourceHost   string `json:"source_host"`
	SourceFile   string `json:"source_file"`
	SourceOffset int64  `json:"source_offset"`

	// Windows 专用字段
	EventID  *uint32 `json:"event_id,omitempty"`
	Provider *string `json:"provider,omitempty"`
	Computer *string `json:"computer,omitempty"`
	UserID   *string `json:"user_id,omitempty"`

	// 动态字段
	Fields map[string]interface{} `json:"fields,omitempty"`

	// 原始数据
	RawLog string `json:"raw_log"`

	// 分析标记
	Tags       []string `json:"tags,omitempty"`
	IsAnomaly  bool     `json:"is_anomaly"`
	RiskScore  *float32 `json:"risk_score,omitempty"`
	IsEvidence bool     `json:"is_evidence"`
}

// TimeRange 时间范围。
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// RawLine 采集器输出的原始行，是采集 → 解析流水线的统一输入。
type RawLine struct {
	SourceID   string `json:"source_id"`
	SourceHost string `json:"source_host"`
	FilePath   string `json:"file_path"`
	Offset     int64  `json:"offset"`
	Size       int64  `json:"size"`
	Seq        int64  `json:"seq"`
	Text       string `json:"text"`
	// Structured 为 true 表示 Text 是采集器已标准化的 JSON（如 evtx）。
	Structured  bool      `json:"structured"`
	CollectedAt time.Time `json:"collected_at"`
}

// IndexStatistics 索引统计信息。
type IndexStatistics struct {
	TotalDocuments int64            `json:"total_documents"`
	UniqueTerms    int64            `json:"unique_terms"`
	IndexSizeBytes int64            `json:"index_size_bytes"`
	LogSizeBytes   int64            `json:"log_size_bytes"`
	LevelCounts    map[string]int64 `json:"level_counts"`
	TimeRange      *TimeRange       `json:"time_range,omitempty"`
	CacheHitRate   float64          `json:"cache_hit_rate"`
	LastBuildTime  time.Time        `json:"last_build_time"`
}
