// Package parser 实现 GLog 的日志解析引擎：内置模板加载、正则/JSON/syslog
// 解析、格式与模板检测、预览以及 AI 模板生成。
package parser

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"glog/internal/errs"
	"glog/pkg/models"
)

// 解析格式常量。ParseTemplate.Format 为空时按 regex 处理。
const (
	formatRegex  = "regex"
	formatJSON   = "json"
	formatSyslog = "syslog"
	formatAuto   = "auto"
)

// ParseContext 携带解析一条日志时的来源上下文。
// ID/DocID 由索引层分配，解析器不写入。
type ParseContext struct {
	SourceID    string
	SourceHost  string
	SourceFile  string
	Offset      int64
	CollectedAt time.Time
}

// Engine 是线程安全的解析引擎。模板与已编译正则均缓存在内部 map 中，
// Parse 不会修改传入的模板。
type Engine struct {
	mu        sync.RWMutex
	templates map[string]*models.ParseTemplate
	compiled  map[string]*compiledTemplate
}

// NewEngine 从 configs.FS 加载全部内置模板并预编译。
func NewEngine() (*Engine, error) {
	e := &Engine{
		templates: make(map[string]*models.ParseTemplate),
		compiled:  make(map[string]*compiledTemplate),
	}
	if err := e.loadBuiltinTemplates(); err != nil {
		return nil, err
	}
	return e, nil
}

// Templates 返回全部模板的副本，按 Priority 降序、ID 升序排列。
func (e *Engine) Templates() []models.ParseTemplate {
	if e == nil {
		return nil
	}
	e.mu.RLock()
	list := make([]models.ParseTemplate, 0, len(e.templates))
	for _, tpl := range e.templates {
		list = append(list, cloneTemplateValue(tpl))
	}
	e.mu.RUnlock()

	sort.Slice(list, func(i, j int) bool {
		if list[i].Priority != list[j].Priority {
			return list[i].Priority > list[j].Priority
		}
		return list[i].ID < list[j].ID
	})
	return list
}

// Template 按 ID 获取模板副本。
func (e *Engine) Template(id string) (*models.ParseTemplate, bool) {
	if e == nil {
		return nil, false
	}
	e.mu.RLock()
	tpl, ok := e.templates[id]
	e.mu.RUnlock()
	if !ok || tpl == nil {
		return nil, false
	}
	return cloneTemplatePtr(tpl), true
}

// snapshotTemplates 返回内部模板指针的快照（调用方不得修改）。
func (e *Engine) snapshotTemplates() []*models.ParseTemplate {
	e.mu.RLock()
	list := make([]*models.ParseTemplate, 0, len(e.templates))
	for _, tpl := range e.templates {
		list = append(list, tpl)
	}
	e.mu.RUnlock()

	sort.Slice(list, func(i, j int) bool {
		if list[i].Priority != list[j].Priority {
			return list[i].Priority > list[j].Priority
		}
		return list[i].ID < list[j].ID
	})
	return list
}

func (e *Engine) hasTemplate(id string) bool {
	if e == nil {
		return false
	}
	e.mu.RLock()
	_, ok := e.templates[id]
	e.mu.RUnlock()
	return ok
}

// Parse 使用指定模板解析一行日志。返回的 LogEntry 已填充时间、级别、
// 消息、原始日志和来源信息；ID 与 DocID 留空由索引层分配。
func (e *Engine) Parse(line string, tpl *models.ParseTemplate, ctx ParseContext) (*models.LogEntry, error) {
	if e == nil {
		return nil, fmt.Errorf("%w: 解析引擎未初始化", errs.ErrParseFailed)
	}
	if tpl == nil {
		return nil, fmt.Errorf("%w: 解析模板为空", errs.ErrParseTemplateMissing)
	}

	ct, err := e.compile(tpl)
	if err != nil {
		return nil, fmt.Errorf("%w: 模板 %q 编译失败: %v", errs.ErrParseFailed, tpl.ID, err)
	}

	format := normalizeFormat(tpl.Format)
	if format == formatAuto {
		var lastErr error
		// auto 依次尝试 JSON、syslog、模板正则。
		for _, candidate := range []string{formatJSON, formatSyslog, formatRegex} {
			if candidate == formatRegex && strings.TrimSpace(tpl.Regex) == "" {
				continue
			}
			entry, attemptErr := e.parseAs(candidate, line, tpl, ct, ctx)
			if attemptErr == nil {
				return entry, nil
			}
			lastErr = attemptErr
		}
		if lastErr == nil {
			lastErr = fmt.Errorf("auto 格式没有可用的解析方式")
		}
		return nil, fmt.Errorf("%w: 模板 %q: %v", errs.ErrParseFormatUnknown, tpl.ID, lastErr)
	}

	return e.parseAs(format, line, tpl, ct, ctx)
}

// ParseWithBest 自动选择最合适的模板解析。模板均未匹配时依次回退到
// syslog 与 JSON 通用解析；两者都不成功则返回 ErrParseFormatUnknown。
func (e *Engine) ParseWithBest(line string, ctx ParseContext) (*models.LogEntry, *models.ParseTemplate, error) {
	if e == nil {
		return nil, nil, fmt.Errorf("%w: 解析引擎未初始化", errs.ErrParseFailed)
	}
	if strings.TrimSpace(line) == "" {
		return nil, nil, fmt.Errorf("%w: 空日志行", errs.ErrParseFailed)
	}

	var lastErr error
	for _, tpl := range e.snapshotTemplates() {
		entry, err := e.Parse(line, tpl, ctx)
		if err == nil {
			return entry, cloneTemplatePtr(tpl), nil
		}
		lastErr = err
	}

	// 无模板可用（或全部未匹配）时先尝试 syslog。
	syslogTemplate := &models.ParseTemplate{Name: "syslog", Format: formatSyslog}
	if entry, err := e.Parse(line, syslogTemplate, ctx); err == nil {
		var matched *models.ParseTemplate
		trimmed := strings.TrimRight(line, "\r\n")
		switch {
		case rfc5424Re.MatchString(trimmed):
			if tpl, ok := e.Template("syslog-rfc5424"); ok {
				matched = tpl
			}
		case rfc3164Re.MatchString(trimmed):
			if tpl, ok := e.Template("syslog-rfc3164"); ok {
				matched = tpl
			}
		}
		return entry, matched, nil
	} else {
		lastErr = err
	}

	// 最后尝试通用 JSON 解析（例如模板集合被裁剪时）。
	jsonTemplate := &models.ParseTemplate{Name: "json", Format: formatJSON}
	if entry, err := e.Parse(line, jsonTemplate, ctx); err == nil {
		if tpl, ok := e.Template("json-generic"); ok {
			return entry, tpl, nil
		}
		return entry, nil, nil
	}

	if lastErr != nil {
		return nil, nil, fmt.Errorf("%w: 无模板匹配（最后错误: %v）", errs.ErrParseFormatUnknown, lastErr)
	}
	return nil, nil, fmt.Errorf("%w: 无可用解析模板", errs.ErrParseFormatUnknown)
}
