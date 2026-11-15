package query

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"glog/internal/errs"
	"glog/internal/logfields"
	"glog/pkg/models"
)

// docPredicate 是需要在读取文档后逐条判断的谓词。
type docPredicate func(*models.LogEntry) bool

// canonicalFieldName 归一化字段名。host 映射到 source_host，msg 映射到 message；
// fields.xxx 保留点路径大小写（便于匹配动态字段键）。
func canonicalFieldName(raw string) string {
	name := strings.TrimSpace(raw)
	low := strings.ToLower(name)
	switch low {
	case "level":
		return "level"
	case "host", "source_host", "hostname":
		return "source_host"
	case "source":
		return "source"
	case "event_id", "eventid":
		return "event_id"
	case "event_type", "eventtype":
		return "event_type"
	case "source_ip", "src_ip", "sourceip", "srcip":
		return "source_ip"
	case "destination_ip", "dest_ip", "dst_ip", "destinationip", "destip", "dstip":
		return "destination_ip"
	case "message", "msg":
		return "message"
	case "id", "doc_id", "docid":
		return "id"
	case "timestamp", "@timestamp", "time":
		return "@timestamp"
	case "raw", "raw_log", "rawlog":
		return "raw_log"
	case "source_file", "file":
		return "source_file"
	case "provider":
		return "provider"
	case "computer":
		return "computer"
	case "user_id", "userid":
		return "user_id"
	case "tags":
		return "tags"
	}
	if strings.HasPrefix(low, "fields.") {
		// 前缀 "fields." 为 ASCII，name 与 low 的长度一致。
		return "fields." + name[len("fields."):]
	}
	return name
}

// isKnownQueryField 判断 text 查询语法中可作为 field:value 前缀的字段。
// 未列出的字段一律回退为普通关键词，避免把 host:port、MAC、URL 等误判为字段。
func isKnownQueryField(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "level", "host", "source_host", "hostname", "source",
		"event_id", "eventid", "event_type", "eventtype",
		"source_ip", "src_ip", "sourceip", "srcip",
		"destination_ip", "dest_ip", "dst_ip", "destinationip", "destip", "dstip",
		"message", "msg", "id", "doc_id", "docid",
		"timestamp", "@timestamp", "time", "raw", "raw_log", "rawlog",
		"source_file", "file", "provider", "computer", "user_id", "userid", "tags":
		return true
	}
	return false
}

// extractFieldValue 从 LogEntry 提取字段值。ok 为 false 表示字段不存在或为空指针。
func extractFieldValue(entry *models.LogEntry, field string) (interface{}, bool) {
	if entry == nil {
		return nil, false
	}
	switch field {
	case "level":
		return entry.Level, true
	case "source_host":
		return entry.SourceHost, true
	case "source":
		return entry.Source, true
	case "event_id":
		if entry.EventID == nil {
			return nil, false
		}
		return *entry.EventID, true
	case "event_type":
		return extractEventType(entry)
	case "source_ip":
		return extractIPField(entry.Fields, logfields.SourceIPAliases)
	case "destination_ip":
		return extractIPField(entry.Fields, logfields.DestinationIPAliases)
	case "message":
		return entry.Message, true
	case "raw_log":
		return entry.RawLog, true
	case "id":
		return entry.DocID, true
	case "@timestamp":
		return entry.Timestamp, true
	case "source_file":
		return entry.SourceFile, true
	case "provider":
		if entry.Provider == nil {
			return nil, false
		}
		return *entry.Provider, true
	case "computer":
		if entry.Computer == nil {
			return nil, false
		}
		return *entry.Computer, true
	case "user_id":
		if entry.UserID == nil {
			return nil, false
		}
		return *entry.UserID, true
	case "tags":
		if entry.Tags == nil {
			return nil, false
		}
		return entry.Tags, true
	}
	if strings.HasPrefix(strings.ToLower(field), "fields.") {
		path := field[len("fields."):]
		return lookupFieldsPath(entry.Fields, path)
	}
	if entry.Fields != nil {
		if v, ok := entry.Fields[field]; ok {
			return v, true
		}
		for k, v := range entry.Fields {
			if strings.EqualFold(k, field) {
				return v, true
			}
		}
	}
	return nil, false
}

// 派生字段（event_type/source_ip/destination_ip）的别名与提取规则统一由
// internal/logfields 提供：索引构建与查询/聚合共用同一套实现，避免两套规则漂移。

// extractIPField 按别名优先级在动态字段中查找 IP 值；空值与占位符 "-" 视为缺失。
// 成功时返回去除首尾空白的 string。
func extractIPField(fields map[string]interface{}, aliases []string) (interface{}, bool) {
	value, ok := logfields.ExtractIP(fields, aliases)
	if !ok {
		return nil, false
	}
	return value, true
}

// extractEventType 依次尝试动态字段 event_type/eventType/Channel、Provider、
// 源文件名与来源，返回非空的事件类型（string）。
func extractEventType(entry *models.LogEntry) (interface{}, bool) {
	value, ok := logfields.EventType(entry)
	if !ok {
		return nil, false
	}
	return value, true
}

// normalizeFieldKey 将字段键归一化为小写字母数字，便于匹配
// SourceIp / source_ip / Source IP 等不同命名风格。
func normalizeFieldKey(key string) string {
	return logfields.NormalizeKey(key)
}

func lookupFieldsPath(fields map[string]interface{}, path string) (interface{}, bool) {
	if fields == nil || path == "" {
		return nil, false
	}
	parts := strings.Split(path, ".")
	var cur interface{} = fields
	for _, part := range parts {
		m, ok := interfaceMap(cur)
		if !ok {
			return nil, false
		}
		v, ok := mapLookupFold(m, part)
		if !ok {
			return nil, false
		}
		cur = v
	}
	if cur == nil {
		return nil, false
	}
	return cur, true
}

// interfaceMap 统一 map 表示，规则与 internal/logfields 一致。
func interfaceMap(v interface{}) (map[string]interface{}, bool) {
	return logfields.InterfaceMap(v)
}

func mapLookupFold(m map[string]interface{}, key string) (interface{}, bool) {
	if v, ok := m[key]; ok {
		return v, true
	}
	for k, v := range m {
		if strings.EqualFold(k, key) {
			return v, true
		}
	}
	return nil, false
}

// valueToString 把字段值转换为稳定的字符串表示，规则与 internal/logfields 一致。
func valueToString(v interface{}) string {
	return logfields.ValueString(v)
}

func toFloat(v interface{}) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int8:
		return float64(x), true
	case int16:
		return float64(x), true
	case int32:
		return float64(x), true
	case int64:
		return float64(x), true
	case uint:
		return float64(x), true
	case uint8:
		return float64(x), true
	case uint16:
		return float64(x), true
	case uint32:
		return float64(x), true
	case uint64:
		return float64(x), true
	case json.Number:
		f, err := x.Float64()
		return f, err == nil
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return 0, false
		}
		f, err := strconv.ParseFloat(s, 64)
		return f, err == nil
	}
	return 0, false
}

func valueEquals(got, want interface{}) bool {
	switch g := got.(type) {
	case nil:
		return want == nil
	case []string:
		for _, item := range g {
			if valueEquals(item, want) {
				return true
			}
		}
		return false
	case []interface{}:
		for _, item := range g {
			if valueEquals(item, want) {
				return true
			}
		}
		return false
	case time.Time:
		switch w := want.(type) {
		case time.Time:
			return g.Equal(w)
		case string:
			if t, _, err := parseTimeBound(w, false); err == nil {
				return g.Equal(t)
			}
		}
		return false
	}
	if want == nil {
		return false
	}
	if gf, ok := toFloat(got); ok {
		if wf, ok2 := toFloat(want); ok2 {
			return gf == wf
		}
	}
	return strings.EqualFold(valueToString(got), valueToString(want))
}

func valueCompare(got, want interface{}) (int, bool) {
	if got == nil || want == nil {
		return 0, false
	}
	if gt, ok := got.(time.Time); ok {
		var wt time.Time
		switch w := want.(type) {
		case time.Time:
			wt = w
		case string:
			t, _, err := parseTimeBound(w, false)
			if err != nil {
				return 0, false
			}
			wt = t
		default:
			return 0, false
		}
		if gt.Before(wt) {
			return -1, true
		}
		if gt.After(wt) {
			return 1, true
		}
		return 0, true
	}
	if gf, ok := toFloat(got); ok {
		if wf, ok2 := toFloat(want); ok2 {
			if gf < wf {
				return -1, true
			}
			if gf > wf {
				return 1, true
			}
			return 0, true
		}
	}
	gs := strings.ToLower(valueToString(got))
	ws := strings.ToLower(valueToString(want))
	return strings.Compare(gs, ws), true
}

func valueContains(got, want interface{}) bool {
	switch g := got.(type) {
	case []string:
		for _, item := range g {
			if valueContains(item, want) {
				return true
			}
		}
		return false
	case []interface{}:
		for _, item := range g {
			if valueContains(item, want) {
				return true
			}
		}
		return false
	}
	return strings.Contains(strings.ToLower(valueToString(got)), strings.ToLower(valueToString(want)))
}

func containsFold(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}

func hasPrefixFold(s, prefix string) bool {
	return strings.HasPrefix(strings.ToLower(s), strings.ToLower(prefix))
}

// sliceValues 把数组值展开为 []interface{}；isList 表示输入本身是数组。
func sliceValues(v interface{}) ([]interface{}, bool) {
	if v == nil {
		return nil, false
	}
	if _, ok := v.([]byte); ok {
		return []interface{}{v}, false
	}
	switch x := v.(type) {
	case []interface{}:
		return x, true
	case []string:
		out := make([]interface{}, len(x))
		for i, item := range x {
			out[i] = item
		}
		return out, true
	case []int:
		out := make([]interface{}, len(x))
		for i, item := range x {
			out[i] = item
		}
		return out, true
	case []int32:
		out := make([]interface{}, len(x))
		for i, item := range x {
			out[i] = item
		}
		return out, true
	case []int64:
		out := make([]interface{}, len(x))
		for i, item := range x {
			out[i] = item
		}
		return out, true
	case []uint32:
		out := make([]interface{}, len(x))
		for i, item := range x {
			out[i] = item
		}
		return out, true
	case []uint64:
		out := make([]interface{}, len(x))
		for i, item := range x {
			out[i] = item
		}
		return out, true
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
		out := make([]interface{}, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			out[i] = rv.Index(i).Interface()
		}
		return out, true
	}
	return []interface{}{v}, false
}

func valuesFromFilter(v interface{}) []interface{} {
	list, isList := sliceValues(v)
	if isList {
		return list
	}
	return []interface{}{v}
}

func levelPriorityOf(level string) int {
	if p, ok := models.LogLevelPriority[strings.ToUpper(strings.TrimSpace(level))]; ok {
		return p
	}
	return -1
}

func rangeEndpointOpen(raw string) bool {
	s := strings.TrimSpace(raw)
	return s == "" || s == "*"
}

func priorityEndpoint(raw string) (int, bool) {
	s := strings.TrimSpace(raw)
	if p, ok := models.LogLevelPriority[strings.ToUpper(s)]; ok {
		return p, true
	}
	if n, err := strconv.Atoi(s); err == nil {
		return n, true
	}
	return 0, false
}

func levelInPriorityRange(level, startRaw, endRaw string) bool {
	startOpen := rangeEndpointOpen(startRaw)
	endOpen := rangeEndpointOpen(endRaw)
	startP, startOK := priorityEndpoint(startRaw)
	endP, endOK := priorityEndpoint(endRaw)
	if (startOpen || startOK) && (endOpen || endOK) {
		p := levelPriorityOf(level)
		if !startOpen && p < startP {
			return false
		}
		if !endOpen && p > endP {
			return false
		}
		return true
	}
	return stringInRangeFold(level, startRaw, endRaw)
}

func stringInRangeFold(value, startRaw, endRaw string) bool {
	if !rangeEndpointOpen(startRaw) {
		if strings.Compare(strings.ToLower(value), strings.ToLower(strings.TrimSpace(startRaw))) < 0 {
			return false
		}
	}
	if !rangeEndpointOpen(endRaw) {
		if strings.Compare(strings.ToLower(value), strings.ToLower(strings.TrimSpace(endRaw))) > 0 {
			return false
		}
	}
	return true
}

func valueInRange(got interface{}, startRaw, endRaw string) bool {
	if got == nil {
		return false
	}
	if !rangeEndpointOpen(startRaw) {
		c, ok := valueCompare(got, startRaw)
		if !ok || c < 0 {
			return false
		}
	}
	if !rangeEndpointOpen(endRaw) {
		c, ok := valueCompare(got, endRaw)
		if !ok || c > 0 {
			return false
		}
	}
	return true
}

func indexedValueInRange(field, value, startRaw, endRaw string) bool {
	switch field {
	case "level":
		return levelInPriorityRange(value, startRaw, endRaw)
	case "event_id":
		return valueInRange(value, startRaw, endRaw)
	default:
		return stringInRangeFold(value, startRaw, endRaw)
	}
}

// buildFieldFilterPredicate 将 Query.Fields 中无法走索引的过滤器编译为逐条谓词。
func buildFieldFilterPredicate(ff models.FieldFilter) (docPredicate, error) {
	field := canonicalFieldName(ff.Field)
	op := strings.ToLower(strings.TrimSpace(ff.Operator))
	if op == "" {
		op = "eq"
	}
	switch op {
	case "eq", "=", "==", "ne", "!=", "<>", "gt", ">", "gte", ">=",
		"lt", "<", "lte", "<=", "in", "not_in", "nin", "contains",
		"not_contains", "regex":
	default:
		return nil, fmt.Errorf("%w: 不支持的字段操作符 %q", errs.ErrInvalidInput, ff.Operator)
	}

	list, isList := sliceValues(ff.Value)
	if !isList {
		list = []interface{}{ff.Value}
	}

	var re *regexp.Regexp
	if op == "regex" {
		pattern := valueToString(ff.Value)
		compiled, err := regexp.Compile("(?i)" + pattern)
		if err != nil {
			return nil, fmt.Errorf("%w: 字段 %s 的正则表达式无效: %v", errs.ErrInvalidInput, ff.Field, err)
		}
		re = compiled
	}

	cmpWant := ff.Value
	if isList && len(list) > 0 {
		cmpWant = list[0]
	}

	return func(entry *models.LogEntry) bool {
		got, ok := extractFieldValue(entry, field)
		switch op {
		case "eq", "=", "==":
			if !ok {
				return false
			}
			if isList {
				for _, want := range list {
					if valueEquals(got, want) {
						return true
					}
				}
				return false
			}
			return valueEquals(got, ff.Value)
		case "ne", "!=", "<>":
			if !ok {
				return true
			}
			if isList {
				for _, want := range list {
					if valueEquals(got, want) {
						return false
					}
				}
				return true
			}
			return !valueEquals(got, ff.Value)
		case "gt", ">":
			if !ok {
				return false
			}
			c, comparable := valueCompare(got, cmpWant)
			return comparable && c > 0
		case "gte", ">=":
			if !ok {
				return false
			}
			c, comparable := valueCompare(got, cmpWant)
			return comparable && c >= 0
		case "lt", "<":
			if !ok {
				return false
			}
			c, comparable := valueCompare(got, cmpWant)
			return comparable && c < 0
		case "lte", "<=":
			if !ok {
				return false
			}
			c, comparable := valueCompare(got, cmpWant)
			return comparable && c <= 0
		case "in":
			if !ok {
				return false
			}
			for _, want := range list {
				if valueEquals(got, want) {
					return true
				}
			}
			return false
		case "not_in", "nin":
			if !ok {
				return true
			}
			for _, want := range list {
				if valueEquals(got, want) {
					return false
				}
			}
			return true
		case "contains":
			if !ok {
				return false
			}
			return valueContains(got, ff.Value)
		case "not_contains":
			if !ok {
				return true
			}
			return !valueContains(got, ff.Value)
		case "regex":
			if !ok || re == nil {
				return false
			}
			return re.MatchString(valueToString(got))
		}
		return false
	}, nil
}
