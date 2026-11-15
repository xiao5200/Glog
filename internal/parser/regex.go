package parser

import (
	"fmt"
	"sort"
	"strings"

	"glog/internal/errs"
	"glog/pkg/models"
)

func (e *Engine) parseRegexLine(ct *compiledTemplate, line string, tpl *models.ParseTemplate, entry *models.LogEntry) (map[string]interface{}, error) {
	if tpl == nil {
		return nil, parseFailedError("", "模板为空")
	}
	if ct == nil || ct.regex == nil {
		return nil, parseFailedError(tpl.ID, "正则表达式未编译")
	}

	matches := ct.regex.FindStringSubmatch(line)
	if matches == nil {
		return nil, fmt.Errorf("%w: 模板 %q 的正则不匹配该日志行", errs.ErrParseFailed, tpl.ID)
	}

	values := make(map[string]interface{}, len(tpl.FieldMap))
	names := make([]string, 0, len(tpl.FieldMap))
	for name := range tpl.FieldMap {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		index := tpl.FieldMap[name]
		if index < 0 || index >= len(matches) {
			return nil, parseFailedError(tpl.ID,
				fmt.Sprintf("字段 %q 的捕获组 %d 超出范围（共 %d 个捕获组）", name, index, len(matches)-1))
		}
		values[name] = strings.TrimSpace(matches[index])
	}

	applyRegexSpecialFields(entry, values)
	return values, nil
}
