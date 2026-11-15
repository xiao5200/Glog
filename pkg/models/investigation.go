package models

import "time"

// Investigation 调查工单。
type Investigation struct {
	ID          string `json:"id"`
	ProjectID   string `json:"project_id"`
	Title       string `json:"title"`
	Description string `json:"description"`

	ClueType  string `json:"clue_type"` // ip/user/pid/file/domain/event_id/message
	ClueValue string `json:"clue_value"`

	Status string `json:"status"` // open/analyzing/closed

	RelatedLogCount int64      `json:"related_log_count"`
	TimeRange       *TimeRange `json:"time_range,omitempty"`

	AIAnalysis *AIAnalysisResult `json:"ai_analysis,omitempty"`

	EvidenceIDs []string `json:"evidence_ids"`

	Tags      []string `json:"tags,omitempty"`
	RiskScore *float32 `json:"risk_score,omitempty"`

	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	ClosedAt  *time.Time `json:"closed_at,omitempty"`

	AssignedTo string `json:"assigned_to,omitempty"`
}

// AIAnalysisResult AI 分析结果。
type AIAnalysisResult struct {
	Timeline     []TimelineEvent `json:"timeline"`
	AttackPhases []AttackPhase   `json:"attack_phases"`
	RiskLevel    string          `json:"risk_level"`
	Findings     []string        `json:"findings"`
	Suggestions  []string        `json:"suggestions"`
	GeneratedAt  time.Time       `json:"generated_at"`
}

type TimelineEvent struct {
	Timestamp   time.Time `json:"timestamp"`
	Event       string    `json:"event"`
	Source      string    `json:"source"`
	Severity    string    `json:"severity"`
	LogEntryIDs []string  `json:"log_entry_ids"`
}

type AttackPhase struct {
	Phase       string   `json:"phase"`
	Description string   `json:"description"`
	Evidence    []string `json:"evidence"`
}

// EvidenceMark 证据标记。
type EvidenceMark struct {
	ID              string    `json:"id"`
	InvestigationID string    `json:"investigation_id"`
	DocID           uint64    `json:"doc_id"`
	LogEntryID      string    `json:"log_entry_id"`
	MarkedAt        time.Time `json:"marked_at"`
	Note            string    `json:"note"`
}

// EvidenceItem 证据 + 日志内容。
type EvidenceItem struct {
	Mark  EvidenceMark `json:"mark"`
	Entry *LogEntry    `json:"entry,omitempty"`
}

// CreateInvestigationRequest 创建调查工单。
type CreateInvestigationRequest struct {
	ProjectID   string     `json:"project_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	ClueType    string     `json:"clue_type"`
	ClueValue   string     `json:"clue_value"`
	Tags        []string   `json:"tags"`
	RiskScore   *float32   `json:"risk_score,omitempty"`
	TimeRange   *TimeRange `json:"time_range,omitempty"`
	AssignedTo  string     `json:"assigned_to,omitempty"`
	// DocIDs 可选：创建时直接标记为证据。
	DocIDs []uint64 `json:"doc_ids,omitempty"`
}

// InvestigationUpdates 更新调查工单。仅非 nil 指针字段生效。
type InvestigationUpdates struct {
	Title       *string   `json:"title"`
	Description *string   `json:"description"`
	Status      *string   `json:"status"`
	Tags        *[]string `json:"tags"`
	RiskScore   *float32  `json:"risk_score"`
	AssignedTo  *string   `json:"assigned_to"`

	// ClueType/ClueValue 更新线索类型与线索值；ClueType 会被规范化，空串视为 message。
	ClueType  *string `json:"clue_type"`
	ClueValue *string `json:"clue_value"`

	// TimeRange 设置新的时间范围；ClearTimeRange 为 true 时优先清空时间范围。
	TimeRange      *TimeRange `json:"time_range,omitempty"`
	ClearTimeRange *bool      `json:"clear_time_range,omitempty"`

	// ClearRiskScore 为 true 时清空风险评分，优先于 RiskScore。
	ClearRiskScore *bool `json:"clear_risk_score,omitempty"`
}
