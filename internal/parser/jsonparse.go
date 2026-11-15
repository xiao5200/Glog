package parser

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"glog/internal/errs"
	"glog/pkg/models"
)

const maxJSONFlattenDepth = 32

func decodeJSONObject(line string) (map[string]interface{}, error) {
	text := strings.TrimSpace(line)
	text = strings.TrimSpace(strings.TrimPrefix(text, "\ufeff"))
	if text == "" || text[0] != '{' {
		return nil, fmt.Errorf("不是 JSON 对象")
	}

	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.UseNumber()
	var object map[string]interface{}
	if err := decoder.Decode(&object); err != nil {
		return nil, err
	}
	if object == nil {
		return nil, fmt.Errorf("不是 JSON 对象")
	}

	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("JSON 对象之后存在多余内容")
		}
		return nil, err
	}
	return object, nil
}

func (e *Engine) parseJSONLine(line string, tpl *models.ParseTemplate, entry *models.LogEntry) (map[string]interface{}, error) {
	object, err := decodeJSONObject(line)
	if err != nil {
		return nil, fmt.Errorf("%w: JSON 解析失败: %v", errs.ErrParseFormatUnknown, err)
	}
	if err := validateJSONTemplate(tpl, object); err != nil {
		return nil, fmt.Errorf("%w: %v", errs.ErrParseFormatUnknown, err)
	}

	values := make(map[string]interface{})
	consumed := make(map[string]bool)
	take := func(candidates ...string) (string, interface{}, bool) {
		key, value, ok := pickJSONKey(object, candidates...)
		if !ok {
			return "", nil, false
		}
		consumed[key] = true
		values[key] = value
		return key, value, true
	}

	take("timestamp", "@timestamp", "ts", "time", "datetime", "date_time", "eventtime", "event_time")
	take("level", "severity", "lvl", "loglevel", "log_level", "levelname")

	if _, value, ok := take("message", "msg", "log"); ok {
		entry.Message = valueToString(value)
	}
	if _, value, ok := take("host", "hostname", "source_host", "sourceHost"); ok {
		if host := strings.TrimSpace(valueToString(value)); host != "" {
			entry.SourceHost = host
		}
	}
	if key, value, ok := take("event_id", "eventid"); ok {
		if eventID, parsed := parseUint32(value); parsed {
			entry.EventID = &eventID
		} else {
			// 无法转换为数字时保留原始值，避免信息丢失。
			addField(entry, key, value)
		}
	}
	if _, value, ok := take("provider"); ok {
		if text := strings.TrimSpace(valueToString(value)); text != "" {
			provider := text
			entry.Provider = &provider
		}
	}
	if _, value, ok := take("computer"); ok {
		if text := strings.TrimSpace(valueToString(value)); text != "" {
			computer := text
			entry.Computer = &computer
		}
	}
	if _, value, ok := take("user_id", "userid", "user"); ok {
		if text := strings.TrimSpace(valueToString(value)); text != "" {
			userID := text
			entry.UserID = &userID
		}
	}

	// 其余顶层字段进入 Fields。嵌套对象同时保留原值并按点号展平，
	// 例如 EventData 与 EventData.TargetUserName；采集器 evtx 标准化
	// JSON 的 fields 对象则直接展平到 Fields。
	flat := make(map[string]interface{})
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		if consumed[key] {
			continue
		}
		value := object[key]

		if strings.EqualFold(strings.TrimSpace(key), "fields") {
			if container, ok := value.(map[string]interface{}); ok {
				flattenJSONFields("", container, flat, 0)
				continue
			}
		}

		if nested, ok := value.(map[string]interface{}); ok {
			addField(entry, key, value) // 保留原始嵌套对象
			flattenJSONFields(key, nested, flat, 0)
			continue
		}
		addField(entry, key, value)
	}

	for key, value := range flat {
		addField(entry, key, value)
	}
	return values, nil
}

// validateJSONTemplate 对内置 JSON 模板做轻量特征校验，使 json-generic、
// docker-json、windows-evtx 在自动匹配时能按内容区分，而不是所有 JSON
// 都命中优先级最高的模板。用户自定义模板不受影响。
func validateJSONTemplate(tpl *models.ParseTemplate, object map[string]interface{}) error {
	if tpl == nil {
		return nil
	}
	switch tpl.ID {
	case "docker-json":
		if !jsonHasKey(object, "log") {
			return fmt.Errorf("docker-json 模板需要 log 字段")
		}
		if !jsonHasKey(object, "stream") && !jsonHasKey(object, "time") {
			return fmt.Errorf("docker-json 模板需要 stream 或 time 字段")
		}
	case "windows-evtx":
		if !jsonHasKey(object, "event_id") && !jsonHasKey(object, "eventid") {
			return fmt.Errorf("windows-evtx 模板需要 event_id 字段")
		}
		if !jsonHasKey(object, "provider") && !jsonHasKey(object, "computer") {
			return fmt.Errorf("windows-evtx 模板需要 provider 或 computer 字段")
		}
	}
	return nil
}

func jsonHasKey(object map[string]interface{}, name string) bool {
	_, _, ok := pickJSONKey(object, name)
	return ok
}

func pickJSONKey(object map[string]interface{}, candidates ...string) (string, interface{}, bool) {
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, candidate := range candidates {
		for _, key := range keys {
			if strings.EqualFold(key, candidate) {
				return key, object[key], true
			}
		}
	}
	return "", nil, false
}

func flattenJSONFields(prefix string, object map[string]interface{}, out map[string]interface{}, depth int) {
	if out == nil {
		return
	}
	if depth > maxJSONFlattenDepth {
		if prefix != "" {
			out[prefix] = object
		}
		return
	}

	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		value := object[key]
		fieldName := key
		if prefix != "" {
			fieldName = prefix + "." + key
		}
		if nested, ok := value.(map[string]interface{}); ok {
			flattenJSONFields(fieldName, nested, out, depth+1)
			continue
		}
		out[fieldName] = numericizeField(key, value)
	}
}
