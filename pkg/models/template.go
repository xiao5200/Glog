package models

import "time"

// ParseTemplate 日志解析模板。
type ParseTemplate struct {
	ID          string `json:"id" yaml:"id"`
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description" yaml:"description"`
	Type        string `json:"type" yaml:"type"`         // builtin/custom/ai_generated
	Category    string `json:"category" yaml:"category"` // web_server/database/middleware...

	// Format 解析方式：regex/json/syslog/auto（默认 regex）。
	Format string `json:"format" yaml:"format"`
	// Priority 用于多模板匹配时选择更具体的模板，越大越优先。
	Priority int `json:"priority" yaml:"priority"`

	Regex    string         `json:"regex" yaml:"regex"`
	FieldMap map[string]int `json:"field_map" yaml:"field_map"`

	TimeFormat string `json:"time_format" yaml:"time_format"`
	TimeField  string `json:"time_field" yaml:"time_field"`

	LevelMap   map[string]string `json:"level_map" yaml:"level_map"`
	LevelField string            `json:"level_field" yaml:"level_field"`

	LevelExtraction *LevelExtraction `json:"level_extraction,omitempty" yaml:"level_extraction,omitempty"`
	MultiLine       *MultiLineConfig `json:"multiline,omitempty" yaml:"multiline,omitempty"`

	Examples []string `json:"examples,omitempty" yaml:"examples,omitempty"`

	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	UsageCount int64     `json:"usage_count"`
	Source     string    `json:"source"` // builtin/user/ai
}

// LevelExtraction 动态级别提取规则。
type LevelExtraction struct {
	Field   string      `json:"field" yaml:"field"`
	Rules   []LevelRule `json:"rules" yaml:"rules"`
	Default LogLevel    `json:"default" yaml:"default"`
}

// LevelRule 单条级别规则。
type LevelRule struct {
	Range []int    `json:"range,omitempty" yaml:"range,omitempty"`
	Match string   `json:"match,omitempty" yaml:"match,omitempty"`
	Level LogLevel `json:"level" yaml:"level"`
}

// MultiLineConfig 多行日志配置。
type MultiLineConfig struct {
	Enabled   bool   `json:"enabled" yaml:"enabled"`
	Pattern   string `json:"pattern" yaml:"pattern"`
	Negate    bool   `json:"negate" yaml:"negate"`
	Match     string `json:"match" yaml:"match"` // after/before
	MaxLines  int    `json:"max_lines" yaml:"max_lines"`
	TimeoutMs int    `json:"timeout_ms" yaml:"timeout_ms"`
}

// TemplateFilter 模板筛选器。
type TemplateFilter struct {
	Type     string `json:"type"` // builtin/custom/ai_generated/all
	Category string `json:"category"`
	Search   string `json:"search"`
}

// PreviewParseRequest 预览解析请求。
type PreviewParseRequest struct {
	Template ParseTemplate `json:"template"`
	Samples  []string      `json:"samples"`
}

// ParsedSample 单条解析样本。
type ParsedSample struct {
	RawLog  string    `json:"raw_log"`
	Parsed  *LogEntry `json:"parsed,omitempty"`
	Success bool      `json:"success"`
	Error   string    `json:"error,omitempty"`
}

// PreviewResult 预览解析结果。
type PreviewResult struct {
	Samples     []ParsedSample `json:"samples"`
	SuccessRate float64        `json:"success_rate"`
}

// FormatDetection 格式检测结果。
type FormatDetection struct {
	DetectedFormat     string   `json:"detected_format"`
	Confidence         float64  `json:"confidence"`
	SuggestedTemplates []string `json:"suggested_templates"`
}

// AITemplateRequest AI 生成模板请求。
type AITemplateRequest struct {
	Samples      []string `json:"samples"`
	Provider     string   `json:"provider"`
	TemplateName string   `json:"template_name"`
	Category     string   `json:"category"`
}
