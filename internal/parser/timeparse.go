package parser

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"time"

	"glog/pkg/models"
)

// fallbackTimeLayouts 在模板 TimeFormat 失败后依次尝试。无时区的布局
// 使用本地时区解析；带时区的布局以日志中的时区为准。
var fallbackTimeLayouts = []string{
	time.RFC3339,
	time.RFC3339Nano,
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05.000Z07:00",
	"02/Jan/2006:15:04:05 -0700",
	"Jan _2 15:04:05",
	"2006/01/02 15:04:05",
	"01/02/2006 15:04:05",
	"15:04:05",

	// 常见变体
	"2006-01-02T15:04:05",
	"2006-01-02T15:04:05.000",
	"2006-01-02T15:04:05.000000Z07:00",
	"2006-01-02T15:04:05-07:00",
	"2006-01-02T15:04:05.000-07:00",
	"2006-01-02 15:04:05.000",
	"2006-01-02 15:04:05.000000",
	"2006-01-02 15:04:05,000",
	"2006-01-02T15:04:05,000",
	"2006-01-02 15:04:05 MST",
	"2006-01-02 15:04:05.000 MST",
	"2006-01-02 15:04:05 -0700",
	"2006-01-02 15:04:05.000 -0700",
	"2006-01-02",
	"Jan 2 15:04:05",
	"2 Jan 2006 15:04:05",
	"2 Jan 2006 15:04:05.000",
	"Mon Jan _2 15:04:05 2006",
	"Mon Jan _2 15:04:05.000000 2006",
	"Mon Jan 2 15:04:05 2006",
}

func resolveTimestamp(tpl *models.ParseTemplate, values, fields map[string]interface{}) (time.Time, bool) {
	if tpl == nil {
		return time.Time{}, false
	}
	layout := strings.TrimSpace(tpl.TimeFormat)

	if timeField := strings.TrimSpace(tpl.TimeField); timeField != "" {
		if value, ok := lookupValue(values, fields, timeField); ok {
			if parsed, ok := parseTimeValue(value, layout); ok {
				return parsed, true
			}
		}
	}

	// IIS W3C 等格式把日期与时间拆成两个字段。
	if dateValue, ok := lookupValue(values, fields, "date"); ok {
		if timeValue, ok := lookupValue(values, fields, "time"); ok {
			combined := strings.TrimSpace(valueToString(dateValue)) + " " + strings.TrimSpace(valueToString(timeValue))
			if parsed, ok := parseTimeString(combined, layout); ok {
				return parsed, true
			}
		}
	}

	for _, name := range []string{"@timestamp", "timestamp", "ts", "datetime", "date_time", "eventtime", "event_time", "time"} {
		if value, ok := lookupValue(values, fields, name); ok {
			if parsed, ok := parseTimeValue(value, layout); ok {
				return parsed, true
			}
		}
	}
	return time.Time{}, false
}

func parseTimeValue(value interface{}, layout string) (time.Time, bool) {
	switch v := value.(type) {
	case nil:
		return time.Time{}, false
	case time.Time:
		if v.IsZero() {
			return time.Time{}, false
		}
		return v, true
	case *time.Time:
		if v == nil || v.IsZero() {
			return time.Time{}, false
		}
		return *v, true
	case string:
		return parseTimeString(v, layout)
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return unixFromInt64(i)
		}
		if f, err := v.Float64(); err == nil {
			return unixFromFloat(f)
		}
		return time.Time{}, false
	case float64:
		return unixFromFloat(v)
	case float32:
		return unixFromFloat(float64(v))
	case int:
		return unixFromInt64(int64(v))
	case int8:
		return unixFromInt64(int64(v))
	case int16:
		return unixFromInt64(int64(v))
	case int32:
		return unixFromInt64(int64(v))
	case int64:
		return unixFromInt64(v)
	case uint:
		if uint64(v) <= uint64(math.MaxInt64) {
			return unixFromInt64(int64(v))
		}
	case uint8:
		return unixFromInt64(int64(v))
	case uint16:
		return unixFromInt64(int64(v))
	case uint32:
		return unixFromInt64(int64(v))
	case uint64:
		if v <= uint64(math.MaxInt64) {
			return unixFromInt64(int64(v))
		}
	}

	text := strings.TrimSpace(valueToString(value))
	if text == "" {
		return time.Time{}, false
	}
	return parseTimeString(text, layout)
}

func parseTimeString(text, layout string) (time.Time, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return time.Time{}, false
	}

	if layout != "" {
		if parsed, err := time.ParseInLocation(layout, text, time.Local); err == nil {
			return adjustParsedTime(parsed, layout), true
		}
	}
	for _, candidate := range fallbackTimeLayouts {
		if candidate == layout {
			continue
		}
		if parsed, err := time.ParseInLocation(candidate, text, time.Local); err == nil {
			return adjustParsedTime(parsed, candidate), true
		}
	}

	// 数字时间戳：秒/毫秒/微秒/纳秒按数量级识别。
	if len(text) >= 9 {
		if i, err := strconv.ParseInt(text, 10, 64); err == nil {
			return unixFromInt64(i)
		}
		if f, err := strconv.ParseFloat(text, 64); err == nil {
			return unixFromFloat(f)
		}
	}
	return time.Time{}, false
}

func adjustParsedTime(parsed time.Time, layout string) time.Time {
	// 只有时分秒时使用当天日期（本地时区）。
	if layout == "15:04:05" || layout == "15:04" {
		now := time.Now()
		return time.Date(now.Year(), now.Month(), now.Day(),
			parsed.Hour(), parsed.Minute(), parsed.Second(), parsed.Nanosecond(), time.Local)
	}
	// 无年份布局（如 Jan _2 15:04:05）按当前年份补全。
	if parsed.Year() == 0 {
		now := time.Now()
		return time.Date(now.Year(), parsed.Month(), parsed.Day(),
			parsed.Hour(), parsed.Minute(), parsed.Second(), parsed.Nanosecond(), parsed.Location())
	}
	return parsed
}

func unixFromInt64(value int64) (time.Time, bool) {
	abs := value
	if abs < 0 {
		if abs == math.MinInt64 {
			return time.Unix(0, value), true
		}
		abs = -abs
	}
	switch {
	case abs >= 1e17: // 纳秒
		return time.Unix(0, value), true
	case abs >= 1e14: // 微秒
		seconds := value / 1e6
		nanos := (value % 1e6) * 1e3
		return normalizedUnix(seconds, nanos), true
	case abs >= 1e11: // 毫秒
		seconds := value / 1e3
		nanos := (value % 1e3) * 1e6
		return normalizedUnix(seconds, nanos), true
	default: // 秒
		return time.Unix(value, 0), true
	}
}

func unixFromFloat(value float64) (time.Time, bool) {
	if !isFiniteFloat(value) {
		return time.Time{}, false
	}
	abs := math.Abs(value)
	var seconds int64
	var nanos int64
	switch {
	case abs >= 1e17: // 纳秒
		seconds = int64(value / 1e9)
		nanos = int64(math.Mod(value, 1e9))
	case abs >= 1e14: // 微秒
		seconds = int64(value / 1e6)
		nanos = int64(math.Mod(value, 1e6)) * 1e3
	case abs >= 1e11: // 毫秒
		seconds = int64(value / 1e3)
		nanos = int64(math.Mod(value, 1e3)) * 1e6
	default: // 秒
		seconds = int64(value)
		nanos = int64((value - float64(seconds)) * 1e9)
	}
	return normalizedUnix(seconds, nanos), true
}

func normalizedUnix(seconds, nanos int64) time.Time {
	if nanos < 0 {
		nanos += 1e9
		seconds--
	}
	if nanos >= 1e9 {
		seconds += nanos / 1e9
		nanos %= 1e9
	}
	return time.Unix(seconds, nanos)
}

func fallbackTimestamp(ctx ParseContext) time.Time {
	if !ctx.CollectedAt.IsZero() {
		return ctx.CollectedAt
	}
	return time.Now()
}
