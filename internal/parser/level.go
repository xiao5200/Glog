package parser

import (
	"regexp"
	"strings"

	"glog/pkg/models"
)

type compiledLevelRule struct {
	hasRange   bool
	min        int
	max        int
	matchLower string
	matchRe    *regexp.Regexp
	level      models.LogLevel
}

func (rule compiledLevelRule) matches(text string) bool {
	if rule.hasRange {
		value, ok := toInt64Value(strings.TrimSpace(text))
		if !ok {
			return false
		}
		return value >= int64(rule.min) && value <= int64(rule.max)
	}
	if rule.matchRe != nil {
		return rule.matchRe.MatchString(text)
	}
	if rule.matchLower != "" {
		return strings.Contains(strings.ToLower(text), rule.matchLower)
	}
	return false
}

type compiledLevelExtraction struct {
	field string
	rules []compiledLevelRule
	def   models.LogLevel
}

func compileLevelExtraction(extraction *models.LevelExtraction) *compiledLevelExtraction {
	if extraction == nil {
		return nil
	}
	compiled := &compiledLevelExtraction{
		field: strings.TrimSpace(extraction.Field),
		def:   normalizeLevelValue(extraction.Default),
	}
	for _, rule := range extraction.Rules {
		level := normalizeLevelValue(rule.Level)
		if strings.TrimSpace(rule.Level) == "" {
			continue
		}

		item := compiledLevelRule{level: level}
		if len(rule.Range) >= 2 {
			item.hasRange = true
			item.min = rule.Range[0]
			item.max = rule.Range[1]
			if item.min > item.max {
				item.min, item.max = item.max, item.min
			}
		}

		match := strings.TrimSpace(rule.Match)
		if match != "" {
			if hasRegexMeta(match) {
				if re, err := regexp.Compile(match); err == nil {
					item.matchRe = re
				} else {
					// 非法正则按大小写不敏感子串匹配处理，避免解析时 panic。
					item.matchLower = strings.ToLower(match)
				}
			} else {
				item.matchLower = strings.ToLower(match)
			}
		}

		if !item.hasRange && item.matchRe == nil && item.matchLower == "" {
			continue
		}
		compiled.rules = append(compiled.rules, item)
	}
	return compiled
}

func normalizeLevelValue(raw string) models.LogLevel {
	text := strings.TrimSpace(raw)
	if text == "" {
		return models.LevelUnknown
	}
	if level := models.NormalizeLevel(text); level != models.LevelUnknown {
		return level
	}
	// 保留自定义级别（大写），避免 LevelMap 中的非标准值被静默丢弃。
	return models.LogLevel(strings.ToUpper(text))
}

func lookupLevelMap(levelMap map[string]string, raw string) (string, bool) {
	if len(levelMap) == 0 {
		return "", false
	}
	if value, ok := levelMap[raw]; ok {
		return value, true
	}
	for key, value := range levelMap {
		if strings.EqualFold(key, raw) {
			return value, true
		}
	}
	return "", false
}

// resolveLevel 按 LevelField/LevelMap、NormalizeLevel、LevelExtraction、
// Default 的顺序确定级别，全部失败时返回 UNKNOWN。
func resolveLevel(tpl *models.ParseTemplate, ct *compiledTemplate, values, fields map[string]interface{}) models.LogLevel {
	if tpl == nil {
		return models.LevelUnknown
	}

	var raw interface{}
	hasRaw := false
	if levelField := strings.TrimSpace(tpl.LevelField); levelField != "" {
		raw, hasRaw = lookupValue(values, fields, levelField)
	}
	if !hasRaw {
		for _, name := range []string{"level", "lvl", "severity"} {
			if value, ok := lookupValue(values, fields, name); ok {
				raw = value
				hasRaw = true
				break
			}
		}
	}

	if hasRaw {
		text := strings.TrimSpace(valueToString(raw))
		if text != "" {
			if mapped, ok := lookupLevelMap(tpl.LevelMap, text); ok {
				level := normalizeLevelValue(mapped)
				if level != models.LevelUnknown {
					return level
				}
			}
			if level := models.NormalizeLevel(text); level != models.LevelUnknown {
				return level
			}
		}
	}

	if ct != nil && ct.extraction != nil {
		extraction := ct.extraction
		text := ""
		if extraction.field != "" {
			if value, ok := lookupValue(values, fields, extraction.field); ok {
				text = valueToString(value)
			}
		} else if hasRaw {
			text = valueToString(raw)
		}
		for _, rule := range extraction.rules {
			if rule.matches(text) {
				return rule.level
			}
		}
		if extraction.def != "" && extraction.def != models.LevelUnknown {
			return extraction.def
		}
	}

	return models.LevelUnknown
}
