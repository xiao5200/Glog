package ai

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"unicode"

	"glog/internal/errs"
)

// ExtractJSON 从 AI 返回文本中提取候选 JSON 字符串。
//
// 处理步骤：
//  1. 去除 UTF-8 BOM 与首尾空白；
//  2. 若存在 Markdown 代码围栏（```json / ```），取围栏内内容；
//  3. 定位第一个 '{' 或 '['，按括号配对找到完整的 JSON 值，忽略字符串内
//     的括号与转义字符，丢弃前后说明文字。
//
// 若未找到 JSON 结构，则返回清理后的原文，交由调用方完成最终校验。
func ExtractJSON(raw string) string {
	text := trimBOMAndSpace(raw)
	if text == "" {
		return ""
	}
	text = stripCodeFences(text)
	text = trimBOMAndSpace(text)
	if value, ok := findJSONValue(text); ok {
		return value
	}
	return text
}

// ParseJSONInto 安全解析 AI 返回的 JSON 到 target。
//
// 会先提取 JSON 片段，再尝试修复尾逗号后重新解析。
// 任何失败均返回包装了 errs.ErrAIInvalidResponse 的可读错误，且不会泄漏原始日志内容。
func ParseJSONInto(raw string, target interface{}) error {
	if target == nil {
		return invalidAIResponse("解析目标不能为空")
	}
	candidate := ExtractJSON(raw)
	if candidate == "" {
		return invalidAIResponse("AI 返回内容为空")
	}

	data := []byte(candidate)
	err := safeUnmarshal(data, target)
	if err == nil {
		return nil
	}

	repaired := removeTrailingCommas(candidate)
	if repaired != candidate {
		if repairErr := safeUnmarshal([]byte(repaired), target); repairErr == nil {
			return nil
		} else {
			err = repairErr
		}
	}
	return fmt.Errorf("%w: JSON 解析失败：%v", errs.ErrAIInvalidResponse, err)
}

func invalidAIResponse(format string, args ...interface{}) error {
	return fmt.Errorf("%w: %s", errs.ErrAIInvalidResponse, fmt.Sprintf(format, args...))
}

func stripCodeFences(text string) string {
	start := strings.Index(text, "```")
	if start < 0 {
		return text
	}
	rest := text[start+3:]
	if end := strings.Index(rest, "```"); end >= 0 {
		rest = rest[:end]
	}
	rest = strings.TrimLeft(rest, " \t")

	if newline := strings.IndexByte(rest, '\n'); newline >= 0 {
		firstLine := strings.TrimSpace(strings.TrimSuffix(rest[:newline], "\r"))
		if isFenceLanguage(firstLine) {
			rest = rest[newline+1:]
		}
	} else {
		trimmed := strings.TrimSpace(rest)
		lower := strings.ToLower(trimmed)
		for _, language := range []string{"json", "jsonc", "json5", "javascript", "js", "text", "plaintext", "markdown"} {
			if strings.HasPrefix(lower, language) {
				rest = trimmed[len(language):]
				break
			}
		}
	}
	return strings.TrimSpace(rest)
}

func isFenceLanguage(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "json", "jsonc", "json5", "javascript", "js", "text", "plaintext", "markdown":
		return true
	default:
		return false
	}
}

// findJSONValue 找到第一个完整且括号配对的 JSON 对象/数组。
func findJSONValue(text string) (string, bool) {
	start := -1
	for i := 0; i < len(text); i++ {
		if text[i] == '{' || text[i] == '[' {
			start = i
			break
		}
	}
	if start < 0 {
		return "", false
	}

	stack := []byte{text[start]}
	inString := false
	escaped := false

	for i := start + 1; i < len(text); i++ {
		current := text[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch current {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}

		switch current {
		case '"':
			inString = true
		case '{', '[':
			stack = append(stack, current)
		case '}', ']':
			expected := byte('}')
			if stack[len(stack)-1] == '[' {
				expected = ']'
			}
			if current != expected {
				// 前面的说明文字中可能混有不成对的括号，忽略不匹配的闭合符。
				continue
			}
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				return text[start : i+1], true
			}
		}
	}

	// 括号不完整时退化为“第一个起始符到最后一个对应闭合符”，
	// 让后续修复/解析给出明确错误。
	closing := byte('}')
	if text[start] == '[' {
		closing = ']'
	}
	if last := strings.LastIndexByte(text, closing); last > start {
		return text[start : last+1], true
	}
	return "", false
}

// removeTrailingCommas 删除对象/数组末尾的多余逗号，忽略字符串内容。
func removeTrailingCommas(text string) string {
	var builder strings.Builder
	builder.Grow(len(text))

	inString := false
	escaped := false
	for i := 0; i < len(text); i++ {
		current := text[i]

		if inString {
			builder.WriteByte(current)
			if escaped {
				escaped = false
				continue
			}
			if current == '\\' {
				escaped = true
				continue
			}
			if current == '"' {
				inString = false
			}
			continue
		}

		if current == '"' {
			inString = true
			builder.WriteByte(current)
			continue
		}

		if current == ',' {
			next := i + 1
			for next < len(text) &&
				(text[next] == ' ' || text[next] == '\t' || text[next] == '\r' || text[next] == '\n') {
				next++
			}
			if next < len(text) && (text[next] == '}' || text[next] == ']') {
				continue
			}
		}
		builder.WriteByte(current)
	}
	return builder.String()
}

// safeUnmarshal 先解析到新值，成功后才写回 target，避免失败时留下半解析状态。
func safeUnmarshal(data []byte, target interface{}) error {
	value := reflect.ValueOf(target)
	if value.Kind() != reflect.Ptr || value.IsNil() {
		return fmt.Errorf("解析目标必须是非空指针")
	}
	fresh := reflect.New(value.Elem().Type())
	if err := json.Unmarshal(data, fresh.Interface()); err != nil {
		return err
	}
	value.Elem().Set(fresh.Elem())
	return nil
}

// trimBOMAndSpace 去除首尾空白与 UTF-8 BOM。
func trimBOMAndSpace(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, string(rune(0xFEFF)))
	return strings.TrimSpace(value)
}

// truncateRunes 按字符数截断，避免破坏 UTF-8。
func truncateRunes(value string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	if max == 1 {
		return "…"
	}
	return string(runes[:max-1]) + "…"
}

// sanitizeText 清除非法的 UTF-8 与控制字符（保留换行、制表符）。
func sanitizeText(value string) string {
	value = strings.ToValidUTF8(value, "�")
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return r
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, value)
}

// sanitizeInline 将文本压成单行，用于日志行、错误信息和标题。
func sanitizeInline(value string) string {
	value = sanitizeText(value)
	value = strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ", "\t", " ").Replace(value)
	return strings.TrimSpace(value)
}
