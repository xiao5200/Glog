package parser

import (
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"glog/configs"
	"glog/internal/errs"
	"glog/internal/logx"
	"glog/pkg/models"
)

// compiledTemplate 是模板的运行时表示，仅缓存与解析直接相关且可复用的
// 编译结果。Parse 不会把任何运行时状态写回 models.ParseTemplate。
type compiledTemplate struct {
	format     string
	regex      *regexp.Regexp
	extraction *compiledLevelExtraction
}

func normalizeFormat(format string) string {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		return formatRegex
	}
	return format
}

func normalizeTemplate(tpl *models.ParseTemplate) {
	if tpl == nil {
		return
	}
	tpl.Format = normalizeFormat(tpl.Format)
	if strings.TrimSpace(tpl.Type) == "" {
		tpl.Type = "builtin"
	}
	if strings.TrimSpace(tpl.Source) == "" {
		tpl.Source = "builtin"
	}
	now := time.Now()
	if tpl.CreatedAt.IsZero() {
		tpl.CreatedAt = now
	}
	if tpl.UpdatedAt.IsZero() {
		tpl.UpdatedAt = now
	}
}

func validateTemplate(tpl *models.ParseTemplate) error {
	if tpl == nil {
		return fmt.Errorf("模板为空")
	}
	if strings.TrimSpace(tpl.ID) == "" {
		return fmt.Errorf("模板缺少 id")
	}
	switch tpl.Format {
	case formatRegex, formatJSON, formatSyslog, formatAuto:
	default:
		return fmt.Errorf("不支持的格式 %q", tpl.Format)
	}
	if tpl.Format == formatRegex && strings.TrimSpace(tpl.Regex) == "" {
		return fmt.Errorf("regex 格式必须提供正则表达式")
	}

	var compiled *regexp.Regexp
	if strings.TrimSpace(tpl.Regex) != "" {
		re, err := regexp.Compile(tpl.Regex)
		if err != nil {
			return fmt.Errorf("正则表达式无效: %w", err)
		}
		compiled = re
	}
	if compiled != nil {
		groups := compiled.NumSubexp()
		for name, index := range tpl.FieldMap {
			if strings.TrimSpace(name) == "" {
				return fmt.Errorf("field_map 存在空字段名")
			}
			if index < 0 || index > groups {
				return fmt.Errorf("字段 %q 的捕获组 %d 超出范围（共 %d 个捕获组）", name, index, groups)
			}
		}
	}

	if tpl.MultiLine != nil && tpl.MultiLine.Enabled && strings.TrimSpace(tpl.MultiLine.Pattern) != "" {
		if _, err := regexp.Compile(tpl.MultiLine.Pattern); err != nil {
			return fmt.Errorf("多行起始正则无效: %w", err)
		}
	}
	return nil
}

func buildCompiled(tpl *models.ParseTemplate) (*compiledTemplate, error) {
	if tpl == nil {
		return nil, fmt.Errorf("模板为空")
	}
	format := normalizeFormat(tpl.Format)
	ct := &compiledTemplate{format: format}

	switch format {
	case formatRegex:
		if strings.TrimSpace(tpl.Regex) == "" {
			return nil, fmt.Errorf("regex 格式必须提供正则表达式")
		}
		re, err := regexp.Compile(tpl.Regex)
		if err != nil {
			return nil, fmt.Errorf("正则表达式无效: %w", err)
		}
		ct.regex = re
	case formatAuto:
		if strings.TrimSpace(tpl.Regex) != "" {
			re, err := regexp.Compile(tpl.Regex)
			if err != nil {
				return nil, fmt.Errorf("正则表达式无效: %w", err)
			}
			ct.regex = re
		}
	case formatJSON, formatSyslog:
		// 该两种格式不需要模板正则。
	default:
		return nil, fmt.Errorf("不支持的格式 %q", tpl.Format)
	}

	if tpl.LevelExtraction != nil {
		ct.extraction = compileLevelExtraction(tpl.LevelExtraction)
	}
	return ct, nil
}

func templateCompileKey(tpl *models.ParseTemplate) string {
	if tpl == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(normalizeFormat(tpl.Format))
	b.WriteByte(0)
	b.WriteString(tpl.Regex)
	b.WriteByte(0)
	if le := tpl.LevelExtraction; le != nil {
		b.WriteString("le:")
		b.WriteString(le.Field)
		b.WriteByte(0)
		b.WriteString(string(le.Default))
		b.WriteByte(0)
		for _, rule := range le.Rules {
			fmt.Fprintf(&b, "%v|%s|%s;", rule.Range, rule.Match, rule.Level)
		}
	}
	return b.String()
}

// compile 返回模板的编译结果；同一模板（ID 无关，按格式/正则/级别规则）
// 的编译结果会被缓存。
func (e *Engine) compile(tpl *models.ParseTemplate) (*compiledTemplate, error) {
	key := templateCompileKey(tpl)
	e.mu.RLock()
	cached := e.compiled[key]
	e.mu.RUnlock()
	if cached != nil {
		return cached, nil
	}

	built, err := buildCompiled(tpl)
	if err != nil {
		return nil, err
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	if existing := e.compiled[key]; existing != nil {
		return existing, nil
	}
	e.compiled[key] = built
	return built, nil
}

func (e *Engine) loadBuiltinTemplates() error {
	entries, err := fs.ReadDir(configs.FS, configs.TemplatesDir)
	if err != nil {
		return fmt.Errorf("%w: 读取内置模板目录失败: %v", errs.ErrParseTemplateMissing, err)
	}

	loaded := 0
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), "_") {
			continue
		}
		ext := strings.ToLower(path.Ext(entry.Name()))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		fullPath := path.Join(configs.TemplatesDir, entry.Name())
		data, readErr := fs.ReadFile(configs.FS, fullPath)
		if readErr != nil {
			logx.Warn("读取内置解析模板失败", zap.String("file", fullPath), zap.Error(readErr))
			continue
		}

		var tpl models.ParseTemplate
		if err := yaml.Unmarshal(data, &tpl); err != nil {
			logx.Error("内置解析模板 YAML 无效", zap.String("file", fullPath), zap.Error(err))
			continue
		}
		if strings.TrimSpace(tpl.ID) == "" {
			tpl.ID = strings.TrimSuffix(entry.Name(), path.Ext(entry.Name()))
		}
		tpl.ID = strings.TrimSpace(tpl.ID)
		normalizeTemplate(&tpl)

		if err := validateTemplate(&tpl); err != nil {
			logx.Error("内置解析模板校验失败",
				zap.String("file", fullPath), zap.String("id", tpl.ID), zap.Error(err))
			continue
		}
		if _, err := e.compile(&tpl); err != nil {
			logx.Error("内置解析模板预编译失败",
				zap.String("file", fullPath), zap.String("id", tpl.ID), zap.Error(err))
			continue
		}

		duplicate := false
		e.mu.Lock()
		if _, exists := e.templates[tpl.ID]; exists {
			duplicate = true
		}
		stored := tpl
		e.templates[stored.ID] = &stored
		e.mu.Unlock()

		if duplicate {
			logx.Warn("解析模板 ID 重复，后者覆盖前者",
				zap.String("id", tpl.ID), zap.String("file", fullPath))
		}
		loaded++
	}

	if loaded == 0 {
		return fmt.Errorf("%w: 未加载到任何内置解析模板", errs.ErrParseTemplateMissing)
	}
	return nil
}

func cloneTemplateValue(tpl *models.ParseTemplate) models.ParseTemplate {
	if tpl == nil {
		return models.ParseTemplate{}
	}
	clone := *tpl
	if tpl.FieldMap != nil {
		clone.FieldMap = make(map[string]int, len(tpl.FieldMap))
		for k, v := range tpl.FieldMap {
			clone.FieldMap[k] = v
		}
	}
	if tpl.LevelMap != nil {
		clone.LevelMap = make(map[string]string, len(tpl.LevelMap))
		for k, v := range tpl.LevelMap {
			clone.LevelMap[k] = v
		}
	}
	if tpl.Examples != nil {
		clone.Examples = append([]string(nil), tpl.Examples...)
	}
	if tpl.LevelExtraction != nil {
		le := *tpl.LevelExtraction
		if tpl.LevelExtraction.Rules != nil {
			le.Rules = make([]models.LevelRule, len(tpl.LevelExtraction.Rules))
			for i, rule := range tpl.LevelExtraction.Rules {
				ruleCopy := rule
				if rule.Range != nil {
					ruleCopy.Range = append([]int(nil), rule.Range...)
				}
				le.Rules[i] = ruleCopy
			}
		}
		clone.LevelExtraction = &le
	}
	if tpl.MultiLine != nil {
		ml := *tpl.MultiLine
		clone.MultiLine = &ml
	}
	return clone
}

func cloneTemplatePtr(tpl *models.ParseTemplate) *models.ParseTemplate {
	if tpl == nil {
		return nil
	}
	clone := cloneTemplateValue(tpl)
	return &clone
}
