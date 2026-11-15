package models

import "time"

// IOCItem 威胁指标。
type IOCItem struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"` // ip/domain/md5/sha1/sha256/url/file_path/email
	Value       string    `json:"value"`
	ThreatLevel string    `json:"threat_level"` // low/medium/high/critical
	Description string    `json:"description"`
	Source      string    `json:"source"`
	CreatedAt   time.Time `json:"created_at"`
}

// IOCImportResult IOC 导入结果。
type IOCImportResult struct {
	Imported int       `json:"imported"`
	Skipped  int       `json:"skipped"`
	Errors   []string  `json:"errors"`
	Items    []IOCItem `json:"items"`
	Format   string    `json:"format"`
}

// IOCSearchRequest IOC 检索请求。
type IOCSearchRequest struct {
	ProjectID string   `json:"project_id"`
	Values    []string `json:"values"`
	Query     *Query   `json:"query,omitempty"`
}

// IOCSearchResult IOC 检索结果。
type IOCSearchResult struct {
	Matches []IOCMatch `json:"matches"`
	Total   int64      `json:"total"`
	Scanned int64      `json:"scanned"`
	Took    int64      `json:"took"`
}

type IOCMatch struct {
	IOC          IOCItem   `json:"ioc"`
	DocID        uint64    `json:"doc_id"`
	Entry        *LogEntry `json:"entry,omitempty"`
	MatchedField string    `json:"matched_field"`
}

// DetectionRule 检测规则（对外视图）。
type DetectionRule struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Description     string     `json:"description"`
	Severity        string     `json:"severity"`
	Enabled         bool       `json:"enabled"`
	Tags            []string   `json:"tags,omitempty"`
	MITRETactics    []string   `json:"mitre_tactics,omitempty"`
	MITRETechniques []string   `json:"mitre_techniques,omitempty"`
	MatchCount      int64      `json:"match_count"`
	LastMatchedAt   *time.Time `json:"last_matched_at,omitempty"`
	FilePath        string     `json:"file_path,omitempty"`
	Builtin         bool       `json:"builtin"`
}

// RunDetectionRequest 运行检测请求。
type RunDetectionRequest struct {
	ProjectID string   `json:"project_id"`
	RuleIDs   []string `json:"rule_ids"`
}

// DetectionResult 检测结果。
type DetectionResult struct {
	TaskID       string          `json:"task_id"`
	Results      []RuleRunResult `json:"results"`
	TotalMatches int64           `json:"total_matches"`
	Scanned      int64           `json:"scanned"`
	Took         int64           `json:"took"`
	StartedAt    time.Time       `json:"started_at"`
	FinishedAt   time.Time       `json:"finished_at"`
}

// RuleRunResult 单规则运行结果。
type RuleRunResult struct {
	RuleID    string           `json:"rule_id"`
	RuleName  string           `json:"rule_name"`
	Severity  string           `json:"severity"`
	Count     int64            `json:"count"`
	Matches   []DetectionMatch `json:"matches"`
	Error     string           `json:"error,omitempty"`
	Summaries []GroupSummary   `json:"summaries,omitempty"`
}

type GroupSummary struct {
	GroupKey string   `json:"group_key"`
	Count    int64    `json:"count"`
	DocIDs   []uint64 `json:"doc_ids,omitempty"`
}

// DetectionMatch 单条命中。
type DetectionMatch struct {
	RuleID     string    `json:"rule_id"`
	RuleName   string    `json:"rule_name"`
	Severity   string    `json:"severity"`
	DocID      uint64    `json:"doc_id"`
	Timestamp  time.Time `json:"timestamp"`
	Message    string    `json:"message"`
	Level      string    `json:"level"`
	SourceHost string    `json:"source_host"`
	GroupKey   string    `json:"group_key,omitempty"`
	Tags       []string  `json:"tags,omitempty"`
	RiskScore  int       `json:"risk_score"`
}

// ForensicsRequest 取证包请求。
type ForensicsRequest struct {
	ProjectID       string   `json:"project_id"`
	InvestigationID string   `json:"investigation_id,omitempty"`
	OutputPath      string   `json:"output_path"`
	DocIDs          []uint64 `json:"doc_ids,omitempty"`
	IOCValues       []string `json:"ioc_values,omitempty"`
	Query           *Query   `json:"query,omitempty"`
	IncludeEntries  bool     `json:"include_entries"`
	IncludeEvidence bool     `json:"include_evidence"`
}

// ForensicsResult 取证包结果。
type ForensicsResult struct {
	Path        string    `json:"path"`
	Files       []string  `json:"files"`
	EntryCount  int       `json:"entry_count"`
	Bytes       int64     `json:"bytes"`
	SHA256      string    `json:"sha256"`
	GeneratedAt time.Time `json:"generated_at"`
}
