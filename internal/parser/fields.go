package parser

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"glog/pkg/models"
)

const (
	minInt64Float          = -9223372036854775808.0
	maxInt64ExclusiveFloat = 9223372036854775808.0
)

func lookupCaseInsensitive(values map[string]interface{}, name string) (interface{}, bool) {
	if values == nil || name == "" {
		return nil, false
	}
	if value, ok := values[name]; ok {
		return value, true
	}
	foundKey := ""
	found := false
	for key := range values {
		if strings.EqualFold(key, name) {
			if !found || key < foundKey {
				foundKey = key
				found = true
			}
		}
	}
	if found {
		return values[foundKey], true
	}
	return nil, false
}

// lookupValue 先在解析出的原始字段中查找，再在 LogEntry.Fields 中查找。
func lookupValue(primary, secondary map[string]interface{}, name string) (interface{}, bool) {
	if name == "" {
		return nil, false
	}
	if value, ok := lookupCaseInsensitive(primary, name); ok {
		return value, true
	}
	if value, ok := lookupCaseInsensitive(secondary, name); ok {
		return value, true
	}
	return nil, false
}

func pickValue(values map[string]interface{}, names ...string) (interface{}, bool) {
	for _, name := range names {
		if value, ok := lookupCaseInsensitive(values, name); ok {
			return value, true
		}
	}
	return nil, false
}

func valueToString(value interface{}) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case []byte:
		return string(v)
	case json.Number:
		return v.String()
	case bool:
		return strconv.FormatBool(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 32)
	case int:
		return strconv.Itoa(v)
	case int8:
		return strconv.FormatInt(int64(v), 10)
	case int16:
		return strconv.FormatInt(int64(v), 10)
	case int32:
		return strconv.FormatInt(int64(v), 10)
	case int64:
		return strconv.FormatInt(v, 10)
	case uint:
		return strconv.FormatUint(uint64(v), 10)
	case uint8:
		return strconv.FormatUint(uint64(v), 10)
	case uint16:
		return strconv.FormatUint(uint64(v), 10)
	case uint32:
		return strconv.FormatUint(uint64(v), 10)
	case uint64:
		return strconv.FormatUint(v, 10)
	default:
		if encoded, err := json.Marshal(v); err == nil {
			return string(encoded)
		}
		return fmt.Sprint(v)
	}
}

var numericExactFields = map[string]struct{}{
	"status":                 {},
	"sc_status":              {},
	"sc_substatus":           {},
	"sc_win32_status":        {},
	"substatus":              {},
	"bytes":                  {},
	"body_bytes_sent":        {},
	"bytes_sent":             {},
	"bytes_received":         {},
	"size":                   {},
	"duration":               {},
	"pid":                    {},
	"port":                   {},
	"s_port":                 {},
	"response_time":          {},
	"upstream_time":          {},
	"request_time":           {},
	"time_taken":             {},
	"elapsed":                {},
	"latency":                {},
	"count":                  {},
	"length":                 {},
	"content_length":         {},
	"thread_id":              {},
	"facility":               {},
	"logon_type":             {},
	"severity_number":        {},
	"response_size":          {},
	"request_size":           {},
	"upstream_status":        {},
	"upstream_response_time": {},
}

var numericSuffixes = []string{
	"_bytes", "_size", "_duration", "_time", "_ms", "_us", "_ns",
	"_pid", "_port", "_count", "_length", "_status", "_code",
}

func isNumericFieldName(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return false
	}
	if _, ok := numericExactFields[name]; ok {
		return true
	}
	for _, suffix := range numericSuffixes {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

// numericizeField 将常见数值字段的字符串值转换为 int64/float64，
// 便于索引与查询阶段做数值比较。无法解析时保留原值。
func numericizeField(name string, value interface{}) interface{} {
	if value == nil || !isNumericFieldName(name) {
		return value
	}

	switch v := value.(type) {
	case string:
		text := strings.TrimSpace(v)
		if text == "" {
			return value
		}
		if i, err := strconv.ParseInt(text, 10, 64); err == nil {
			return i
		}
		if f, err := strconv.ParseFloat(text, 64); err == nil && isFiniteFloat(f) {
			return f
		}
		return value
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return i
		}
		if f, err := v.Float64(); err == nil && isFiniteFloat(f) {
			return f
		}
		return value
	case float64:
		if !isFiniteFloat(v) {
			return value
		}
		if v == math.Trunc(v) && v >= minInt64Float && v < maxInt64ExclusiveFloat {
			return int64(v)
		}
		return v
	case float32:
		return numericizeField(name, float64(v))
	case int:
		return int64(v)
	case int8:
		return int64(v)
	case int16:
		return int64(v)
	case int32:
		return int64(v)
	case int64:
		return v
	case uint:
		if uint64(v) <= uint64(math.MaxInt64) {
			return int64(v)
		}
		return v
	case uint8:
		return int64(v)
	case uint16:
		return int64(v)
	case uint32:
		return int64(v)
	case uint64:
		if v <= uint64(math.MaxInt64) {
			return int64(v)
		}
		return v
	default:
		return value
	}
}

func isFiniteFloat(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func addField(entry *models.LogEntry, name string, value interface{}) {
	if entry == nil || strings.TrimSpace(name) == "" {
		return
	}
	if entry.Fields == nil {
		entry.Fields = make(map[string]interface{})
	}
	entry.Fields[name] = numericizeField(name, value)
}

func toInt64Value(value interface{}) (int64, bool) {
	switch v := value.(type) {
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return i, true
		}
		if f, err := v.Float64(); err == nil {
			return floatToInt64(f)
		}
	case string:
		text := strings.TrimSpace(v)
		if text == "" {
			return 0, false
		}
		if i, err := strconv.ParseInt(text, 10, 64); err == nil {
			return i, true
		}
		if f, err := strconv.ParseFloat(text, 64); err == nil {
			return floatToInt64(f)
		}
	case float64:
		return floatToInt64(v)
	case float32:
		return floatToInt64(float64(v))
	case int:
		return int64(v), true
	case int8:
		return int64(v), true
	case int16:
		return int64(v), true
	case int32:
		return int64(v), true
	case int64:
		return v, true
	case uint:
		if uint64(v) <= uint64(math.MaxInt64) {
			return int64(v), true
		}
	case uint8:
		return int64(v), true
	case uint16:
		return int64(v), true
	case uint32:
		return int64(v), true
	case uint64:
		if v <= uint64(math.MaxInt64) {
			return int64(v), true
		}
	}
	return 0, false
}

func floatToInt64(value float64) (int64, bool) {
	if !isFiniteFloat(value) || value != math.Trunc(value) {
		return 0, false
	}
	if value < minInt64Float || value >= maxInt64ExclusiveFloat {
		return 0, false
	}
	return int64(value), true
}

func parseUint32(value interface{}) (uint32, bool) {
	i, ok := toInt64Value(value)
	if !ok || i < 0 || i > int64(^uint32(0)) {
		return 0, false
	}
	return uint32(i), true
}

func hasRegexMeta(text string) bool {
	return strings.ContainsAny(text, "[](){}.*+?|^$\\")
}
