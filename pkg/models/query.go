package models

import "time"

// FieldFilter 字段过滤器。
type FieldFilter struct {
	Field    string      `json:"field"`
	Operator string      `json:"operator"` // eq/ne/gt/lt/gte/lte/in/contains/regex/not_contains
	Value    interface{} `json:"value"`
}

// Query 查询模型。
type Query struct {
	Text string `json:"text,omitempty"`

	StartTime *time.Time `json:"start_time,omitempty"`
	EndTime   *time.Time `json:"end_time,omitempty"`

	Levels   []LogLevel    `json:"levels,omitempty"`
	Hosts    []string      `json:"hosts,omitempty"`
	Sources  []string      `json:"sources,omitempty"`
	EventIDs []uint32      `json:"event_ids,omitempty"`
	Fields   []FieldFilter `json:"field_filters,omitempty"`

	SortBy    string `json:"sort_by,omitempty"`    // timestamp/level/doc_id
	SortOrder string `json:"sort_order,omitempty"` // asc/desc

	Offset int `json:"offset"`
	Limit  int `json:"limit"`
}

// QueryResult 查询结果。
type QueryResult struct {
	Total   int64      `json:"total"`
	Hits    []LogEntry `json:"hits"`
	Took    int64      `json:"took"` // 毫秒
	HasMore bool       `json:"has_more"`
	QueryID string     `json:"query_id,omitempty"`
}

// ContextResult 上下文查询结果。
type ContextResult struct {
	Entries     []LogEntry `json:"entries"`
	AnchorIndex int        `json:"anchor_index"`
}

// QuerySnapshot 查询快照。
type QuerySnapshot struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Query       Query     `json:"query"`
	ResultCount int64     `json:"result_count"`
	CreatedAt   time.Time `json:"created_at"`
}

// AggregateRequest 聚合请求。
type AggregateRequest struct {
	Query       Query    `json:"query"`
	AggregateBy []string `json:"aggregate_by"` // level/source_host/source/event_id/hour/day
	TopN        int      `json:"top_n"`
	TimeBucket  string   `json:"time_bucket,omitempty"` // minute/hour/day/auto
}

// AggregateResult 聚合结果。
type AggregateResult struct {
	TimeSeries   []TimeSeriesPoint  `json:"time_series,omitempty"`
	Distribution []DistributionItem `json:"distribution,omitempty"`
	TopItems     []TopItem          `json:"top_items,omitempty"`
	Total        int64              `json:"total"`
	Took         int64              `json:"took"`
}

type TimeSeriesPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Count     int64     `json:"count"`
}

type DistributionItem struct {
	Key        string  `json:"key"`
	Count      int64   `json:"count"`
	Percentage float64 `json:"percentage"`
}

type TopItem struct {
	Key   string `json:"key"`
	Count int64  `json:"count"`
}

// PatternClusterResult 模式聚类结果。
type PatternClusterResult struct {
	Patterns   []LogPattern `json:"patterns"`
	TotalCount int64        `json:"total_count"`
	Took       int64        `json:"took"`
}

// LogPattern 日志模式。
type LogPattern struct {
	ID         string              `json:"id"`
	Template   string              `json:"template"`
	Count      int64               `json:"count"`
	Percentage float64             `json:"percentage"`
	FirstSeen  time.Time           `json:"first_seen"`
	LastSeen   time.Time           `json:"last_seen"`
	Examples   []string            `json:"examples"`
	Parameters map[string][]string `json:"parameters"`
}
