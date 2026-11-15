package parser

import (
	"strings"

	"glog/pkg/models"
)

func newEntry(ctx ParseContext) *models.LogEntry {
	return &models.LogEntry{
		Source:       ctx.SourceID,
		SourceHost:   ctx.SourceHost,
		SourceFile:   ctx.SourceFile,
		SourceOffset: ctx.Offset,
	}
}

// parseAs 使用单一格式解析并完成统一收尾。line 保留原始内容（含可能的
// 行尾换行）用于 RawLog，匹配时使用去除行尾换行的文本。
func (e *Engine) parseAs(format, line string, tpl *models.ParseTemplate, ct *compiledTemplate, ctx ParseContext) (*models.LogEntry, error) {
	entry := newEntry(ctx)
	text := strings.TrimRight(line, "\r\n")

	var (
		values map[string]interface{}
		err    error
	)
	switch format {
	case formatJSON:
		values, err = e.parseJSONLine(text, tpl, entry)
	case formatSyslog:
		values, err = e.parseSyslogLine(text, tpl, entry)
	case formatRegex:
		values, err = e.parseRegexLine(ct, text, tpl, entry)
	default:
		return nil, formatUnknownError(format)
	}
	if err != nil {
		return nil, err
	}
	if values == nil {
		values = map[string]interface{}{}
	}

	e.finalizeEntry(entry, tpl, ct, values, ctx, line)
	return entry, nil
}

func (e *Engine) finalizeEntry(entry *models.LogEntry, tpl *models.ParseTemplate, ct *compiledTemplate, values map[string]interface{}, ctx ParseContext, rawLine string) {
	if entry == nil {
		return
	}
	entry.RawLog = rawLine
	if strings.TrimSpace(entry.SourceHost) == "" {
		entry.SourceHost = ctx.SourceHost
	}

	if timestamp, ok := resolveTimestamp(tpl, values, entry.Fields); ok {
		entry.Timestamp = timestamp
	} else if entry.Timestamp.IsZero() {
		entry.Timestamp = fallbackTimestamp(ctx)
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = fallbackTimestamp(ctx)
	}

	level := resolveLevel(tpl, ct, values, entry.Fields)
	switch {
	case level != models.LevelUnknown:
		entry.Level = level
	case entry.Level == "" || entry.Level == models.LevelUnknown:
		// syslog PRI 已在解析阶段给出级别时保留该级别。
		entry.Level = models.LevelUnknown
	}

	if strings.TrimSpace(entry.Message) == "" {
		entry.Message = rawLine
	}
	if len(entry.Fields) == 0 {
		entry.Fields = nil
	}
}

type fieldKind int

const (
	fieldOther fieldKind = iota
	fieldTime
	fieldLevel
	fieldMessage
	fieldHost
	fieldEventID
	fieldProvider
	fieldComputer
	fieldUser
)

func classifyField(name string) fieldKind {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "timestamp", "time", "ts", "@timestamp", "datetime", "date_time", "eventtime", "event_time":
		return fieldTime
	case "level", "lvl", "severity", "loglevel", "log_level", "levelname":
		return fieldLevel
	case "message", "msg", "log":
		return fieldMessage
	case "host", "hostname", "source_host", "srchost":
		return fieldHost
	case "event_id", "eventid":
		return fieldEventID
	case "provider", "provider_name", "providername":
		return fieldProvider
	case "computer", "computername", "computer_name":
		return fieldComputer
	case "user_id", "userid", "user":
		return fieldUser
	default:
		return fieldOther
	}
}

// applyRegexSpecialFields 将 FieldMap 捕获值中的公共字段映射到 LogEntry，
// 其余字段进入 Fields。Regex 模板中的 log 字段不作为 message，避免与
// 普通业务字段冲突（JSON 解析才将 log 视为消息）。
func applyRegexSpecialFields(entry *models.LogEntry, values map[string]interface{}) {
	if entry == nil {
		return
	}

	if value, ok := pickValue(values, "message", "msg"); ok {
		entry.Message = valueToString(value)
	}
	if value, ok := pickValue(values, "host", "source_host", "hostname"); ok {
		if host := strings.TrimSpace(valueToString(value)); host != "" {
			entry.SourceHost = host
		}
	}
	if value, ok := pickValue(values, "event_id", "eventid"); ok {
		if eventID, parsed := parseUint32(value); parsed {
			entry.EventID = &eventID
		} else {
			addField(entry, "event_id", value)
		}
	}
	if value, ok := pickValue(values, "provider"); ok {
		if text := strings.TrimSpace(valueToString(value)); text != "" {
			provider := text
			entry.Provider = &provider
		}
	}
	if value, ok := pickValue(values, "computer"); ok {
		if text := strings.TrimSpace(valueToString(value)); text != "" {
			computer := text
			entry.Computer = &computer
		}
	}
	if value, ok := pickValue(values, "user_id", "userid", "user"); ok {
		if text := strings.TrimSpace(valueToString(value)); text != "" {
			userID := text
			entry.UserID = &userID
		}
	}

	for name, value := range values {
		if isConsumedRegexField(name) {
			continue
		}
		addField(entry, name, value)
	}
}

func isConsumedRegexField(name string) bool {
	kind := classifyField(name)
	if kind == fieldOther {
		return false
	}
	if kind == fieldMessage && strings.EqualFold(strings.TrimSpace(name), "log") {
		return false
	}
	return true
}
