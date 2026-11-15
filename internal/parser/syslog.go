package parser

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"glog/internal/errs"
	"glog/pkg/models"
)

// RFC5424: <PRI>VERSION TIMESTAMP HOST APP PROCID MSGID SD MSG
var rfc5424Re = regexp.MustCompile(`^<(\d{1,3})>(\d+) (\S+) (\S+) (\S+) (\S+) (\S+) (.*)$`)

// RFC3164: [<PRI>]Mon DD HH:MM:SS HOST TAG[PID]: MSG
var rfc3164Re = regexp.MustCompile(`^(?:<(\d{1,3})>)?([A-Z][a-z]{2}) +(\d{1,2}) (\d{2}:\d{2}:\d{2}) (\S+) ([^:\s]+?)(?:\[(\d+)\])?: ?(.*)$`)

func (e *Engine) parseSyslogLine(line string, tpl *models.ParseTemplate, entry *models.LogEntry) (map[string]interface{}, error) {
	rfc5424 := rfc5424Re.FindStringSubmatch(line)
	rfc3164 := rfc3164Re.FindStringSubmatch(line)

	// 内置模板按 ID 区分 RFC 版本，避免同一行同时命中两个 syslog 模板。
	if tpl != nil {
		switch tpl.ID {
		case "syslog-rfc5424":
			if rfc5424 == nil {
				return nil, fmt.Errorf("%w: 不是 RFC5424 syslog 格式", errs.ErrParseFormatUnknown)
			}
		case "syslog-rfc3164":
			if rfc3164 == nil {
				return nil, fmt.Errorf("%w: 不是 RFC3164 syslog 格式", errs.ErrParseFormatUnknown)
			}
		}
	}

	if rfc5424 != nil {
		return parseRFC5424(rfc5424, entry), nil
	}
	if rfc3164 != nil {
		return parseRFC3164(rfc3164, entry), nil
	}
	return nil, fmt.Errorf("%w: 不是 syslog 格式", errs.ErrParseFormatUnknown)
}

func parseRFC5424(matches []string, entry *models.LogEntry) map[string]interface{} {
	values := make(map[string]interface{})
	pri, priOK := parsePRI(matches[1])
	version := matches[2]
	timestamp := matches[3]
	host := strings.TrimSpace(matches[4])
	app := strings.TrimSpace(matches[5])
	procid := strings.TrimSpace(matches[6])
	msgid := strings.TrimSpace(matches[7])
	structuredData, message := splitStructuredData(matches[8])

	if host != "" && host != "-" {
		entry.SourceHost = host
		values["host"] = host
	}
	if timestamp != "" && timestamp != "-" {
		values["timestamp"] = timestamp
		if parsed, ok := parseTimeString(timestamp, time.RFC3339Nano); ok {
			entry.Timestamp = parsed
		}
	}

	entry.Message = message
	values["message"] = message
	values["syslog_version"] = version
	if app != "" && app != "-" {
		values["app_name"] = app
		addField(entry, "app_name", app)
		addField(entry, "program", app)
	}
	if procid != "" && procid != "-" {
		values["procid"] = procid
		addField(entry, "pid", procid)
	}
	if msgid != "" && msgid != "-" {
		values["msgid"] = msgid
		addField(entry, "msgid", msgid)
	}
	if structuredData != "" {
		values["structured_data"] = structuredData
		addField(entry, "structured_data", structuredData)
	}
	if priOK {
		values["pri"] = pri
		entry.Level = levelFromPRI(pri)
		addField(entry, "facility", pri/8)
	}
	return values
}

func parseRFC3164(matches []string, entry *models.LogEntry) map[string]interface{} {
	values := make(map[string]interface{})
	pri, priOK := parsePRI(matches[1])
	timestampText := matches[2] + " " + matches[3] + " " + matches[4]
	host := strings.TrimSpace(matches[5])
	program := strings.TrimSpace(matches[6])
	pid := strings.TrimSpace(matches[7])
	message := matches[8]

	if host != "" {
		entry.SourceHost = host
		values["host"] = host
	}
	values["timestamp"] = timestampText
	if parsed, ok := parseTimeString(timestampText, "Jan _2 15:04:05"); ok {
		entry.Timestamp = parsed
	}

	entry.Message = message
	values["message"] = message
	if program != "" {
		values["program"] = program
		addField(entry, "program", program)
	}
	if pid != "" {
		values["pid"] = pid
		addField(entry, "pid", pid)
	}
	if priOK {
		values["pri"] = pri
		entry.Level = levelFromPRI(pri)
		addField(entry, "facility", pri/8)
	}
	return values
}

func parsePRI(raw string) (int, bool) {
	if raw == "" {
		return 0, false
	}
	pri, err := strconv.Atoi(raw)
	if err != nil || pri < 0 || pri > 191 {
		return 0, false
	}
	return pri, true
}

func levelFromPRI(pri int) models.LogLevel {
	// PRI = facility*8 + severity，级别映射使用其中的 severity 部分。
	severity := pri % 8
	switch {
	case severity >= 0 && severity <= 3:
		return models.LevelError
	case severity == 4:
		return models.LevelWarn
	case severity >= 5 && severity <= 6:
		return models.LevelInfo
	default:
		return models.LevelDebug
	}
}

// splitStructuredData 从 RFC5424 的 SD 字段中分出结构化数据与消息。
// SD 可能包含引号、转义字符和空格，因此不能简单按空格切分。
func splitStructuredData(rest string) (structuredData, message string) {
	rest = strings.TrimSpace(rest)
	if rest == "" || rest == "-" {
		return "", ""
	}
	if strings.HasPrefix(rest, "- ") {
		return "", strings.TrimSpace(rest[2:])
	}
	if !strings.HasPrefix(rest, "[") {
		return "", rest
	}

	depth := 0
	inQuote := false
	escaped := false
	for i := 0; i < len(rest); i++ {
		if escaped {
			escaped = false
			continue
		}
		switch rest[i] {
		case '\\':
			if inQuote {
				escaped = true
			}
		case '"':
			inQuote = !inQuote
		case '[':
			if !inQuote {
				depth++
			}
		case ']':
			if !inQuote {
				depth--
				if depth == 0 {
					return rest[:i+1], strings.TrimSpace(rest[i+1:])
				}
			}
		}
	}
	return rest, ""
}
