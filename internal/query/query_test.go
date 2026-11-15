// 同包测试：便于直接验证 extractFieldValue、canonicalFieldName 等未导出实现。
package query

import (
	"testing"
	"time"

	"glog/internal/indexer"
	"glog/pkg/models"
)

type testIndex struct {
	*indexer.Store
}

func (i testIndex) Tokenize(text string) []string { return indexer.Tokenize(text) }

func buildTestIndex(t *testing.T) (string, *indexer.Store, *Engine) {
	t.Helper()
	dir := t.TempDir()
	store, err := indexer.Open(dir)
	if err != nil {
		t.Fatalf("open indexer: %v", err)
	}
	eventID := uint32(4625)
	entries := []*models.LogEntry{
		{
			Timestamp:  mustTime(t, "2024-01-01T10:00:00Z"),
			Level:      models.LevelInfo,
			Message:    "GET /index.html 200",
			Source:     "src-1",
			SourceHost: "web-01",
			Fields:     map[string]interface{}{"status": int64(200), "remote_addr": "192.0.2.10"},
		},
		{
			Timestamp:  mustTime(t, "2024-01-01T10:01:00Z"),
			Level:      models.LevelError,
			Message:    "connection refused to db-01",
			Source:     "src-1",
			SourceHost: "web-02",
			Fields:     map[string]interface{}{"status": int64(500)},
		},
		{
			Timestamp:  mustTime(t, "2024-01-01T10:02:00Z"),
			Level:      models.LevelWarn,
			Message:    "slow query 1200ms",
			Source:     "src-2",
			SourceHost: "web-01",
			Fields:     map[string]interface{}{"status": int64(200)},
		},
		{
			Timestamp:  mustTime(t, "2024-01-01T10:03:00Z"),
			Level:      models.LevelInfo,
			Message:    "用户登录失败 from 10.0.0.5",
			Source:     "src-2",
			SourceHost: "web-03",
			Fields:     map[string]interface{}{"remote_addr": "10.0.0.5"},
		},
		{
			Timestamp:  mustTime(t, "2024-01-01T10:04:00Z"),
			Level:      models.LevelDebug,
			Message:    "health check ok",
			Source:     "src-1",
			SourceHost: "web-01",
			EventID:    &eventID,
		},
	}
	if err := store.Append(entries); err != nil {
		t.Fatalf("append: %v", err)
	}
	if err := store.Finalize(); err != nil {
		t.Fatalf("finalize: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return dir, store, NewEngine(testIndex{Store: store})
}

func mustTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("parse time: %v", err)
	}
	return parsed
}

func TestSearchBooleanAndFilters(t *testing.T) {
	_, _, engine := buildTestIndex(t)

	cases := []struct {
		name  string
		query models.Query
		total int64
	}{
		{"implicit AND", models.Query{Text: "connection refused", Limit: 10}, 1},
		{"OR", models.Query{Text: "refused OR slow", Limit: 10}, 2},
		{"NOT", models.Query{Text: "NOT health", Limit: 10}, 4},
		{"wildcard", models.Query{Text: "connect*", Limit: 10}, 1},
		{"phrase", models.Query{Text: `"connection refused"`, Limit: 10}, 1},
		{"level", models.Query{Levels: []string{"ERROR"}, Limit: 10}, 1},
		{"host", models.Query{Hosts: []string{"web-01"}, Limit: 10}, 3},
		{"source", models.Query{Sources: []string{"src-2"}, Limit: 10}, 2},
		{"event id", models.Query{EventIDs: []uint32{4625}, Limit: 10}, 1},
		{"field range", models.Query{
			Fields: []models.FieldFilter{{Field: "fields.status", Operator: "gte", Value: float64(500)}},
			Limit:  10,
		}, 1},
		{"cjk", models.Query{Text: "登录失败", Limit: 10}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := engine.Search(tc.query)
			if err != nil {
				t.Fatalf("search: %v", err)
			}
			if res.Total != tc.total {
				t.Fatalf("expected total %d, got %d (hits=%d)", tc.total, res.Total, len(res.Hits))
			}
		})
	}
}

func TestSearchSortPaginationAndContext(t *testing.T) {
	_, _, engine := buildTestIndex(t)

	asc, err := engine.Search(models.Query{SortBy: "timestamp", SortOrder: "asc", Limit: 2})
	if err != nil {
		t.Fatalf("search asc: %v", err)
	}
	if asc.Total != 5 || len(asc.Hits) != 2 || !asc.HasMore {
		t.Fatalf("unexpected page: %+v", asc)
	}
	if asc.Hits[0].Message != "GET /index.html 200" {
		t.Fatalf("unexpected first entry: %s", asc.Hits[0].Message)
	}

	page2, err := engine.Search(models.Query{SortBy: "timestamp", SortOrder: "asc", Offset: 2, Limit: 2})
	if err != nil {
		t.Fatalf("search page2: %v", err)
	}
	if len(page2.Hits) != 2 || page2.Hits[0].Message != "slow query 1200ms" {
		t.Fatalf("unexpected page2: %+v", page2.Hits)
	}

	ctx, err := engine.Context(2, 1, 1)
	if err != nil {
		t.Fatalf("context: %v", err)
	}
	if len(ctx.Entries) != 3 || ctx.AnchorIndex != 1 {
		t.Fatalf("unexpected context: len=%d anchor=%d", len(ctx.Entries), ctx.AnchorIndex)
	}
}

func TestAggregateAndCluster(t *testing.T) {
	_, _, engine := buildTestIndex(t)

	agg, err := engine.Aggregate(models.AggregateRequest{
		Query:       models.Query{},
		AggregateBy: []string{"level", "source_host"},
		TopN:        10,
		TimeBucket:  "hour",
	})
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if agg.Total != 5 {
		t.Fatalf("expected total 5, got %d", agg.Total)
	}
	if len(agg.Distribution) == 0 || len(agg.TimeSeries) == 0 {
		t.Fatalf("expected distribution and time series, got %+v", agg)
	}
	var infoCount int64
	for _, item := range agg.Distribution {
		if item.Key == models.LevelInfo {
			infoCount = item.Count
		}
	}
	if infoCount != 2 {
		t.Fatalf("expected 2 INFO logs, got %d", infoCount)
	}

	cluster, err := engine.Cluster(models.Query{})
	if err != nil {
		t.Fatalf("cluster: %v", err)
	}
	if cluster.TotalCount != 5 || len(cluster.Patterns) == 0 {
		t.Fatalf("unexpected cluster: %+v", cluster)
	}
}

// buildDerivedFieldIndex 构造覆盖来源/目的 IP、事件类型与事件 ID 的测试索引。
func buildDerivedFieldIndex(t *testing.T) *Engine {
	t.Helper()
	dir := t.TempDir()
	store, err := indexer.Open(dir)
	if err != nil {
		t.Fatalf("open indexer: %v", err)
	}
	provider := "Microsoft-Windows-Sysmon"
	eventID := uint32(4625)
	entries := []*models.LogEntry{
		{
			Timestamp:  mustTime(t, "2024-02-01T09:00:00Z"),
			Level:      models.LevelWarn,
			Message:    "failed logon",
			Source:     "evtx",
			SourceFile: "C:/logs/Security.evtx",
			EventID:    &eventID,
			Fields: map[string]interface{}{
				"IpAddress":     "10.0.0.5",
				"SourceIp":      "10.0.0.6",
				"DestinationIp": "10.0.0.9",
				"EventType":     "Security",
				"EventID":       4625,
			},
		},
		{
			Timestamp:  mustTime(t, "2024-02-01T09:01:00Z"),
			Level:      models.LevelInfo,
			Message:    "allowed logon",
			Source:     "evtx",
			SourceFile: "C:/logs/Security.evtx",
			Fields: map[string]interface{}{
				"EventData": map[string]interface{}{
					"SourceIp":      "10.0.0.6",
					"DestinationIp": "10.0.0.10",
					"EventType":     "Security",
				},
				"IpAddress": "10.0.0.5",
			},
		},
		{
			Timestamp: mustTime(t, "2024-02-01T09:02:00Z"),
			Level:     models.LevelInfo,
			Message:   "network connection",
			Source:    "sysmon",
			Provider:  &provider,
			Fields: map[string]interface{}{
				"RemoteIp":           "10.0.0.7",
				"DestinationAddress": "10.0.0.11",
			},
		},
		{
			Timestamp:  mustTime(t, "2024-02-01T09:03:00Z"),
			Level:      models.LevelDebug,
			Message:    "client disconnected",
			Source:     "app",
			SourceFile: "C:/logs/Application.evtx",
			Fields:     map[string]interface{}{"ClientIp": "-"},
		},
		{
			Timestamp: mustTime(t, "2024-02-01T09:04:00Z"),
			Level:     models.LevelInfo,
			Message:   "collector heartbeat",
			Source:    "custom-source",
		},
		{
			Timestamp: mustTime(t, "2024-02-01T09:05:00Z"),
			Level:     models.LevelInfo,
			Message:   "no origin info",
		},
	}
	if err := store.Append(entries); err != nil {
		t.Fatalf("append: %v", err)
	}
	if err := store.Finalize(); err != nil {
		t.Fatalf("finalize: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return NewEngine(testIndex{Store: store})
}

func distributionCounts(items []models.DistributionItem) map[string]int64 {
	out := make(map[string]int64, len(items))
	for _, item := range items {
		out[item.Key] = item.Count
	}
	return out
}

func TestCanonicalDerivedFieldAliases(t *testing.T) {
	cases := map[string]string{
		"SOURCE_IP":      "source_ip",
		"src_ip":         "source_ip",
		"SourceIP":       "source_ip",
		"srcip":          "source_ip",
		"DESTINATION_IP": "destination_ip",
		"dest_ip":        "destination_ip",
		"DstIp":          "destination_ip",
		"Event_Type":     "event_type",
		"EVENTTYPE":      "event_type",
	}
	for raw, want := range cases {
		if got := canonicalFieldName(raw); got != want {
			t.Fatalf("canonicalFieldName(%q) = %q, want %q", raw, got, want)
		}
	}
	for _, name := range []string{
		"source_ip", "src_ip", "sourceip", "srcip",
		"destination_ip", "dest_ip", "dst_ip", "destinationip", "destip", "dstip",
		"event_type", "eventtype",
	} {
		if !isKnownQueryField(name) {
			t.Fatalf("isKnownQueryField(%q) = false, want true", name)
		}
	}
}

func TestExtractDerivedFieldValues(t *testing.T) {
	provider := "Microsoft-Windows-Security-Auditing"
	cases := []struct {
		name  string
		entry *models.LogEntry
		field string
		want  interface{}
		ok    bool
	}{
		{
			name: "source_ip 优先于通用 IpAddress",
			entry: &models.LogEntry{Fields: map[string]interface{}{
				"IpAddress":     "10.0.0.5",
				"SourceIp":      "10.0.0.6",
				"DestinationIp": "10.0.0.9",
			}},
			field: "source_ip",
			want:  "10.0.0.6",
			ok:    true,
		},
		{
			name: "destination_ip 取目的地址",
			entry: &models.LogEntry{Fields: map[string]interface{}{
				"IpAddress":     "10.0.0.5",
				"SourceIp":      "10.0.0.6",
				"DestinationIp": "10.0.0.9",
			}},
			field: "destination_ip",
			want:  "10.0.0.9",
			ok:    true,
		},
		{
			name: "source_ip 不误取目的地址",
			entry: &models.LogEntry{Fields: map[string]interface{}{
				"DestinationIp":      "10.0.0.9",
				"DestinationAddress": "10.0.0.11",
			}},
			field: "source_ip",
			want:  nil,
			ok:    false,
		},
		{
			name: "嵌套 EventData 与键归一化",
			entry: &models.LogEntry{Fields: map[string]interface{}{
				"EventData": map[string]interface{}{
					"Source IP":      "10.1.0.6",
					"Destination-IP": "10.1.0.9",
					"EventType":      "Security",
				},
			}},
			field: "source_ip",
			want:  "10.1.0.6",
			ok:    true,
		},
		{
			name:  "remote_ip 别名",
			entry: &models.LogEntry{Fields: map[string]interface{}{"remote_ip": "10.0.0.7"}},
			field: "source_ip",
			want:  "10.0.0.7",
			ok:    true,
		},
		{
			name:  "空占位符视为缺失",
			entry: &models.LogEntry{Fields: map[string]interface{}{"SourceIp": "-"}},
			field: "source_ip",
			want:  nil,
			ok:    false,
		},
		{
			name:  "缺失字段返回 false",
			entry: &models.LogEntry{},
			field: "destination_ip",
			want:  nil,
			ok:    false,
		},
		{
			name: "event_type 优先动态字段",
			entry: &models.LogEntry{
				Provider: &provider,
				Fields:   map[string]interface{}{"event_type": "Security"},
			},
			field: "event_type",
			want:  "Security",
			ok:    true,
		},
		{
			name:  "event_type 回退 Provider",
			entry: &models.LogEntry{Provider: &provider},
			field: "event_type",
			want:  provider,
			ok:    true,
		},
		{
			name:  "event_type 回退源文件名",
			entry: &models.LogEntry{SourceFile: "C:/logs/Security.evtx"},
			field: "event_type",
			want:  "Security.evtx",
			ok:    true,
		},
		{
			name:  "event_type 回退来源",
			entry: &models.LogEntry{Source: "custom-source"},
			field: "event_type",
			want:  "custom-source",
			ok:    true,
		},
		{
			name:  "event_type 全部为空",
			entry: &models.LogEntry{Fields: map[string]interface{}{"EventType": "  "}},
			field: "event_type",
			want:  nil,
			ok:    false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := extractFieldValue(tc.entry, tc.field)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v (value=%v)", ok, tc.ok, got)
			}
			if ok && valueToString(got) != valueToString(tc.want) {
				t.Fatalf("value = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAggregateDerivedFields(t *testing.T) {
	engine := buildDerivedFieldIndex(t)

	all := engine.allDocsBitmap()
	for _, field := range []string{"event_type", "source_ip", "destination_ip"} {
		values := engine.idx.FieldValues(field)
		if len(values) == 0 {
			t.Fatalf("expected bitmap index to expose values for %q", field)
		}
		indexed := make(map[string]int64)
		if !engine.countIndexedField(field, all, indexed) {
			t.Fatalf("expected aggregation for %q to use the index path", field)
		}
	}

	sourceAgg, err := engine.Aggregate(models.AggregateRequest{
		AggregateBy: []string{"source_ip"},
		TopN:        10,
	})
	if err != nil {
		t.Fatalf("aggregate source_ip: %v", err)
	}
	sourceCounts := distributionCounts(sourceAgg.Distribution)
	if sourceCounts["10.0.0.6"] != 2 || sourceCounts["10.0.0.7"] != 1 || len(sourceCounts) != 2 {
		t.Fatalf("unexpected source_ip distribution: %v", sourceCounts)
	}
	for _, unexpected := range []string{"10.0.0.5", "10.0.0.9", "10.0.0.10", "10.0.0.11", "-"} {
		if _, exists := sourceCounts[unexpected]; exists {
			t.Fatalf("source_ip unexpectedly bucketed %q: %v", unexpected, sourceCounts)
		}
	}

	destAgg, err := engine.Aggregate(models.AggregateRequest{
		AggregateBy: []string{"destination_ip"},
		TopN:        10,
	})
	if err != nil {
		t.Fatalf("aggregate destination_ip: %v", err)
	}
	destCounts := distributionCounts(destAgg.Distribution)
	for _, want := range []string{"10.0.0.9", "10.0.0.10", "10.0.0.11"} {
		if destCounts[want] != 1 {
			t.Fatalf("expected destination %q count 1, got %v", want, destCounts)
		}
	}
	if len(destCounts) != 3 {
		t.Fatalf("unexpected destination_ip distribution: %v", destCounts)
	}

	typeAgg, err := engine.Aggregate(models.AggregateRequest{
		AggregateBy: []string{"event_type"},
		TopN:        10,
	})
	if err != nil {
		t.Fatalf("aggregate event_type: %v", err)
	}
	typeCounts := distributionCounts(typeAgg.Distribution)
	if typeCounts["Security"] != 2 {
		t.Fatalf("expected 2 Security events, got %v", typeCounts)
	}
	if typeCounts["Microsoft-Windows-Sysmon"] != 1 {
		t.Fatalf("expected Provider fallback bucket, got %v", typeCounts)
	}
	if typeCounts["Application.evtx"] != 1 {
		t.Fatalf("expected SourceFile fallback bucket, got %v", typeCounts)
	}
	if typeCounts["custom-source"] != 1 {
		t.Fatalf("expected Source fallback bucket, got %v", typeCounts)
	}
	if len(typeCounts) != 4 {
		t.Fatalf("unexpected event_type distribution: %v", typeCounts)
	}

	eventAgg, err := engine.Aggregate(models.AggregateRequest{
		AggregateBy: []string{"event_id"},
		TopN:        10,
	})
	if err != nil {
		t.Fatalf("aggregate event_id: %v", err)
	}
	eventCounts := distributionCounts(eventAgg.Distribution)
	if eventCounts["4625"] != 1 || len(eventCounts) != 1 {
		t.Fatalf("unexpected event_id distribution: %v", eventCounts)
	}

	// 忽略大小写/下划线的别名聚合应落到同一组索引桶。
	aliasAgg, err := engine.Aggregate(models.AggregateRequest{
		AggregateBy: []string{"srcip", "DESTINATION_IP", "eventtype"},
		TopN:        10,
	})
	if err != nil {
		t.Fatalf("aggregate derived field aliases: %v", err)
	}
	aliasCounts := distributionCounts(aliasAgg.Distribution)
	if aliasCounts["10.0.0.6"] != 2 || aliasCounts["10.0.0.7"] != 1 {
		t.Fatalf("unexpected srcip alias distribution: %v", aliasCounts)
	}
	if aliasCounts["10.0.0.9"] != 1 || aliasCounts["10.0.0.10"] != 1 || aliasCounts["10.0.0.11"] != 1 {
		t.Fatalf("unexpected DESTINATION_IP alias distribution: %v", aliasCounts)
	}
	if aliasCounts["Security"] != 2 || aliasCounts["custom-source"] != 1 {
		t.Fatalf("unexpected eventtype alias distribution: %v", aliasCounts)
	}
	if len(aliasCounts) != 9 {
		t.Fatalf("unexpected merged alias distribution: %v", aliasCounts)
	}

	// 过滤条件走 bitmap 索引后，聚合维度仍在索引路径上计数。
	filteredAgg, err := engine.Aggregate(models.AggregateRequest{
		Query: models.Query{
			Fields: []models.FieldFilter{{Field: "source_ip", Operator: "eq", Value: "10.0.0.6"}},
		},
		AggregateBy: []string{"destination_ip"},
		TopN:        10,
	})
	if err != nil {
		t.Fatalf("aggregate filtered destination_ip: %v", err)
	}
	if filteredAgg.Total != 2 {
		t.Fatalf("expected 2 matched docs, got %d", filteredAgg.Total)
	}
	filteredCounts := distributionCounts(filteredAgg.Distribution)
	if filteredCounts["10.0.0.9"] != 1 || filteredCounts["10.0.0.10"] != 1 || len(filteredCounts) != 2 {
		t.Fatalf("unexpected filtered destination_ip distribution: %v", filteredCounts)
	}
}

// TestRemoteAddrAliasIndexedAsSourceIP 覆盖 logfields 新增的 remote_addr 别名：
// 基础测试数据中的 remote_addr 现在应作为 source_ip 参与索引、过滤与聚合。
func TestRemoteAddrAliasIndexedAsSourceIP(t *testing.T) {
	_, _, engine := buildTestIndex(t)

	res, err := engine.Search(models.Query{
		Fields: []models.FieldFilter{{Field: "source_ip", Operator: "eq", Value: "10.0.0.5"}},
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("search source_ip from remote_addr: %v", err)
	}
	if res.Total != 1 {
		t.Fatalf("expected 1 doc for remote_addr source_ip, got %d", res.Total)
	}

	agg, err := engine.Aggregate(models.AggregateRequest{
		AggregateBy: []string{"source_ip"},
		TopN:        10,
	})
	if err != nil {
		t.Fatalf("aggregate source_ip from remote_addr: %v", err)
	}
	sourceCounts := distributionCounts(agg.Distribution)
	if sourceCounts["192.0.2.10"] != 1 || sourceCounts["10.0.0.5"] != 1 || len(sourceCounts) != 2 {
		t.Fatalf("unexpected remote_addr source_ip distribution: %v", sourceCounts)
	}
}

// TestAggregateDerivedFieldFallbackWithoutIndexValues 验证派生字段没有任何索引值时
// 聚合仍回退逐条扫描且结果为空（而不是报错或漏算）。
func TestAggregateDerivedFieldFallbackWithoutIndexValues(t *testing.T) {
	_, _, engine := buildTestIndex(t)

	if values := engine.idx.FieldValues("destination_ip"); len(values) != 0 {
		t.Fatalf("expected no destination_ip index values, got %v", values)
	}
	agg, err := engine.Aggregate(models.AggregateRequest{
		AggregateBy: []string{"destination_ip"},
		TopN:        10,
	})
	if err != nil {
		t.Fatalf("aggregate destination_ip fallback: %v", err)
	}
	if agg.Total != 5 {
		t.Fatalf("expected total 5, got %d", agg.Total)
	}
	if len(agg.Distribution) != 0 {
		t.Fatalf("expected empty destination_ip distribution, got %v", distributionCounts(agg.Distribution))
	}
}

// TestDerivedFieldIndexPathCoverage 验证三个派生字段的过滤器/字段节点被判定为可走索引，
// 范围节点则保持扫描路径。
func TestDerivedFieldIndexPathCoverage(t *testing.T) {
	engine := buildDerivedFieldIndex(t)

	all := engine.allDocsBitmap()
	for _, field := range []string{"event_type", "source_ip", "destination_ip"} {
		if values := engine.idx.FieldValues(field); len(values) == 0 {
			t.Fatalf("expected bitmap index to expose values for %q", field)
		}
		if !isCheapIndexNode(&ASTNode{Type: NodeFIELD, Field: field, Value: "x"}) {
			t.Fatalf("expected field node %q to be a cheap index node", field)
		}
		if isCheapIndexNode(&ASTNode{Type: NodeRANGE, Field: field, RangeStart: "a", RangeEnd: "z"}) {
			t.Fatalf("range node on %q must stay on the scan path", field)
		}
		for _, op := range []string{"eq", "=", "==", "in", "ne", "!=", "<>", "not_in", "nin"} {
			value := interface{}("x")
			if op == "in" || op == "not_in" || op == "nin" {
				value = []string{"x", "y"}
			}
			ff := models.FieldFilter{Field: field, Operator: op, Value: value}
			if _, handled := engine.fieldFilterBitmap(ff, all); !handled {
				t.Fatalf("filter %s %s should be handled by the bitmap index", field, op)
			}
		}
	}
}

func TestSearchDerivedFields(t *testing.T) {
	engine := buildDerivedFieldIndex(t)

	filterCases := []struct {
		name   string
		filter models.FieldFilter
		total  int64
	}{
		{"source_ip eq", models.FieldFilter{Field: "source_ip", Operator: "eq", Value: "10.0.0.6"}, 2},
		{"source_ip in", models.FieldFilter{Field: "source_ip", Operator: "in", Value: []string{"10.0.0.6", "10.0.0.7"}}, 3},
		{"source_ip ne", models.FieldFilter{Field: "source_ip", Operator: "ne", Value: "10.0.0.6"}, 4},
		{"source_ip not_in", models.FieldFilter{Field: "source_ip", Operator: "not_in", Value: []string{"10.0.0.6", "10.0.0.7"}}, 3},
		{"destination_ip eq", models.FieldFilter{Field: "destination_ip", Operator: "eq", Value: "10.0.0.10"}, 1},
		{"destination_ip in", models.FieldFilter{Field: "destination_ip", Operator: "in", Value: []string{"10.0.0.9", "10.0.0.11"}}, 2},
		{"destination_ip ne", models.FieldFilter{Field: "destination_ip", Operator: "ne", Value: "10.0.0.9"}, 5},
		{"event_type eq", models.FieldFilter{Field: "event_type", Operator: "eq", Value: "Security"}, 2},
		{"event_type in", models.FieldFilter{Field: "event_type", Operator: "in", Value: []string{"Security", "custom-source"}}, 3},
		{"event_type ne", models.FieldFilter{Field: "event_type", Operator: "ne", Value: "Security"}, 4},
		{"srcip alias eq", models.FieldFilter{Field: "srcip", Operator: "=", Value: "10.0.0.6"}, 2},
		{"DESTINATION_IP alias ne", models.FieldFilter{Field: "DESTINATION_IP", Operator: "<>", Value: "10.0.0.9"}, 5},
		{"eventtype alias eq", models.FieldFilter{Field: "eventtype", Operator: "==", Value: "Security"}, 2},
		{"srcip alias not_in", models.FieldFilter{Field: "srcip", Operator: "nin", Value: []string{"10.0.0.6"}}, 4},
	}
	for _, tc := range filterCases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := engine.Search(models.Query{Fields: []models.FieldFilter{tc.filter}, Limit: 10})
			if err != nil {
				t.Fatalf("search %+v: %v", tc.filter, err)
			}
			if res.Total != tc.total {
				t.Fatalf("expected %d docs for %+v, got %d", tc.total, tc.filter, res.Total)
			}
		})
	}

	textCases := []struct {
		name  string
		text  string
		total int64
	}{
		{"source_ip text", "source_ip:10.0.0.6", 2},
		{"source_ip uppercase field", "SOURCE_IP:10.0.0.6", 2},
		{"srcip alias", "srcip:10.0.0.7", 1},
		{"source_ip wildcard", "source_ip:10.0.0.*", 3},
		{"destination_ip text", "destination_ip:10.0.0.10", 1},
		{"DESTINATION_IP alias text", "DESTINATION_IP:10.0.0.9", 1},
		{"dstip wildcard alias", "dstip:10.0.0.1*", 2},
		{"event_type text", "event_type:Security", 2},
		{"eventtype lowercase value", "eventtype:security", 2},
		{"event_type wildcard", "event_type:secur*", 2},
		{"event_type provider fallback", "event_type:Microsoft-Windows-Sysmon", 1},
		{"implicit AND", "source_ip:10.0.0.6 event_type:Security", 2},
	}
	for _, tc := range textCases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := engine.Search(models.Query{Text: tc.text, Limit: 10})
			if err != nil {
				t.Fatalf("search %q: %v", tc.text, err)
			}
			if res.Total != tc.total {
				t.Fatalf("expected %d docs for %q, got %d", tc.total, tc.text, res.Total)
			}
		})
	}
}

func TestPersistenceAfterReopen(t *testing.T) {
	dir, store, _ := buildTestIndex(t)
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	reopened, err := indexer.Open(dir)
	if err != nil {
		t.Fatalf("reopen indexer: %v", err)
	}
	defer reopened.Close()
	engine := NewEngine(testIndex{Store: reopened})
	res, err := engine.Search(models.Query{Text: "connection refused", Limit: 10})
	if err != nil {
		t.Fatalf("search after reopen: %v", err)
	}
	if res.Total != 1 {
		t.Fatalf("expected persisted search result, got %d", res.Total)
	}
}
