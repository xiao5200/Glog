package models

import "time"

// AIProviderInfo AI 提供商信息（不含密钥）。
type AIProviderInfo struct {
	Provider   string    `json:"provider"`
	Name       string    `json:"name"`
	Endpoint   string    `json:"endpoint"`
	Model      string    `json:"model"`
	Enabled    bool      `json:"enabled"`
	Priority   int       `json:"priority"`
	HasKey     bool      `json:"has_key"`
	Configured bool      `json:"configured"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// SaveAIConfigRequest 保存 AI 配置；APIKey 为空表示保持原密钥。
type SaveAIConfigRequest struct {
	Provider   string            `json:"provider"`
	Endpoint   string            `json:"endpoint"`
	Model      string            `json:"model"`
	APIKey     string            `json:"api_key,omitempty"`
	Enabled    bool              `json:"enabled"`
	Priority   int               `json:"priority"`
	Parameters map[string]string `json:"parameters,omitempty"`
}

// AIModelsRequest 识别 OpenAI 兼容服务可用模型；APIKey 为空时使用已保存密钥。
type AIModelsRequest struct {
	Provider string `json:"provider"`
	Endpoint string `json:"endpoint,omitempty"`
	APIKey   string `json:"api_key,omitempty"`
}

// AIModelTestRequest 在保存前测试指定模型连通性；APIKey 为空时使用已保存密钥。
type AIModelTestRequest struct {
	Provider string `json:"provider"`
	Endpoint string `json:"endpoint,omitempty"`
	Model    string `json:"model"`
	APIKey   string `json:"api_key,omitempty"`
}

// LogAnalysisRequest 单条日志 AI 分析请求；ContextBefore/After 为 0 表示仅分析本条。
type LogAnalysisRequest struct {
	ProjectID     string `json:"project_id"`
	DocID         uint32 `json:"doc_id"`
	Provider      string `json:"provider"`
	ContextBefore int    `json:"context_before"`
	ContextAfter  int    `json:"context_after"`
	Question      string `json:"question,omitempty"`
}

// LogAnalysisResult 单条日志 AI 分析结果。
type LogAnalysisResult struct {
	Summary       string    `json:"summary"`
	RiskLevel     string    `json:"risk_level"`
	Findings      []string  `json:"findings"`
	Suggestions   []string  `json:"suggestions"`
	RelatedDocIDs []uint32  `json:"related_doc_ids,omitempty"`
	Analyzed      int       `json:"analyzed"`
	Provider      string    `json:"provider"`
	RawResponse   string    `json:"raw_response,omitempty"`
	GeneratedAt   time.Time `json:"generated_at"`
}

// AnomalyDetectionRequest 异常检测请求。
type AnomalyDetectionRequest struct {
	ProjectID   string   `json:"project_id"`
	Provider    string   `json:"provider"`
	Query       Query    `json:"query"`
	MaxLogs     int      `json:"max_logs"`
	AggregateBy []string `json:"aggregate_by,omitempty"`
}

// AnomalyDetectionResult 异常检测结果。
type AnomalyDetectionResult struct {
	Summary     string               `json:"summary"`
	RiskLevel   string               `json:"risk_level"` // high/medium/low
	Findings    []string             `json:"findings"`
	Suggestions []string             `json:"suggestions"`
	Anomalies   []Anomaly            `json:"anomalies"`
	Aggregates  []AggregateDimension `json:"aggregates,omitempty"`
	Analyzed    int                  `json:"analyzed"`
	Provider    string               `json:"provider"`
	RawResponse string               `json:"raw_response,omitempty"`
	GeneratedAt time.Time            `json:"generated_at"`
}

// AggregateDimension 单个维度的聚合统计结果（Top 20）。
type AggregateDimension struct {
	Field string    `json:"field"`
	Items []TopItem `json:"items"`
	Total int64     `json:"total"`
}

type Anomaly struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	Reason    string    `json:"reason"`
	Score     float64   `json:"score"`
	DocIDs    []uint64  `json:"doc_ids,omitempty"`
}

// RootCauseRequest 根因分析请求。
type RootCauseRequest struct {
	ProjectID string `json:"project_id"`
	Provider  string `json:"provider"`
	Query     Query  `json:"query"`
	Question  string `json:"question"`
	MaxLogs   int    `json:"max_logs"`
}

// RootCauseResult 根因分析结果。
type RootCauseResult struct {
	Summary     string      `json:"summary"`
	RootCauses  []RootCause `json:"root_causes"`
	Timeline    []string    `json:"timeline"`
	Suggestions []string    `json:"suggestions"`
	Analyzed    int         `json:"analyzed"`
	Provider    string      `json:"provider"`
	RawResponse string      `json:"raw_response,omitempty"`
	GeneratedAt time.Time   `json:"generated_at"`
}

type RootCause struct {
	Cause       string   `json:"cause"`
	Confidence  float64  `json:"confidence"`
	Evidence    []string `json:"evidence"`
	Description string   `json:"description"`
}

// ReportRequest 报告生成请求。
type ReportRequest struct {
	ProjectID       string   `json:"project_id"`
	InvestigationID string   `json:"investigation_id,omitempty"`
	Provider        string   `json:"provider"`
	Title           string   `json:"title"`
	Query           Query    `json:"query"`
	MaxLogs         int      `json:"max_logs"`
	AggregateBy     []string `json:"aggregate_by,omitempty"`
	OutputPath      string   `json:"output_path,omitempty"`
}

// ReportResult 报告结果。
type ReportResult struct {
	Title       string               `json:"title"`
	Content     string               `json:"content"` // Markdown
	Format      string               `json:"format"`
	OutputPath  string               `json:"output_path,omitempty"`
	Provider    string               `json:"provider"`
	Aggregates  []AggregateDimension `json:"aggregates,omitempty"`
	GeneratedAt time.Time            `json:"generated_at"`
}
