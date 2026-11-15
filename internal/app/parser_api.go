package app

import (
	"context"
	"regexp"
	"strings"
	"time"

	"glog/internal/errs"
	"glog/pkg/models"
)

// GetTemplates 获取解析模板。
func (a *App) GetTemplates(filter models.TemplateFilter) ([]models.ParseTemplate, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	if filter.Type == "" {
		filter.Type = "all"
	}
	list, err := a.metadata.ListTemplates(filter)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []models.ParseTemplate{}
	}
	return list, nil
}

// GetTemplate 获取模板详情。
func (a *App) GetTemplate(templateID string) (*models.ParseTemplate, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	if templateID == "" {
		return nil, errs.InvalidInput("模板 ID 不能为空", nil)
	}
	tpl, err := a.metadata.GetTemplate(templateID)
	if err == nil && tpl != nil {
		return tpl, nil
	}
	if builtin, ok := a.parserEngine.Template(templateID); ok {
		return builtin, nil
	}
	return nil, errs.ErrParseTemplateMissing
}

// SaveTemplate 保存自定义或 AI 模板。
func (a *App) SaveTemplate(tpl models.ParseTemplate) error {
	if err := a.ready(); err != nil {
		return err
	}
	tpl.Name = strings.TrimSpace(tpl.Name)
	if tpl.Name == "" {
		return errs.InvalidInput("模板名称不能为空", nil)
	}
	if tpl.Format == "" {
		tpl.Format = "regex"
	}
	switch tpl.Format {
	case "regex", "json", "syslog", "auto":
	default:
		return errs.InvalidInput("不支持的模板格式: "+tpl.Format, nil)
	}
	if tpl.Priority <= 0 {
		tpl.Priority = 60
	}
	if tpl.Regex != "" {
		re, err := regexp.Compile(tpl.Regex)
		if err != nil {
			return errs.InvalidInput("正则表达式无效: "+err.Error(), err)
		}
		maxIndex := re.NumSubexp()
		for field, idx := range tpl.FieldMap {
			if idx < 0 || idx > maxIndex {
				return errs.InvalidInput("字段映射超出捕获组范围: "+field, nil)
			}
		}
	}
	if tpl.ID == "" {
		tpl.ID = "custom-" + newID()
		tpl.Type = "custom"
		tpl.Source = "user"
	}
	if existing, err := a.metadata.GetTemplate(tpl.ID); err == nil && existing != nil {
		if existing.Type == "builtin" {
			return errs.InvalidInput("内置模板不可覆盖，请另存为自定义模板", nil)
		}
	}
	if tpl.Type == "" {
		tpl.Type = "custom"
	}
	if tpl.Type == "builtin" {
		return errs.InvalidInput("内置模板不可通过界面保存", nil)
	}
	if tpl.Source == "" {
		tpl.Source = "user"
	}
	if tpl.CreatedAt.IsZero() {
		tpl.CreatedAt = nowUTC()
	}
	tpl.UpdatedAt = nowUTC()
	return a.metadata.SaveTemplate(&tpl)
}

// DeleteTemplate 删除自定义模板。
func (a *App) DeleteTemplate(templateID string) error {
	if err := a.ready(); err != nil {
		return err
	}
	tpl, err := a.metadata.GetTemplate(templateID)
	if err != nil {
		return err
	}
	if tpl.Type == "builtin" {
		return errs.InvalidInput("内置模板不可删除", nil)
	}
	return a.metadata.DeleteTemplate(templateID)
}

// DetectFormat 自动检测日志格式。
func (a *App) DetectFormat(samples []string) (*models.FormatDetection, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	if len(samples) == 0 {
		return nil, errs.InvalidInput("样本不能为空", nil)
	}
	result := a.parserEngine.DetectFormat(samples)
	return &result, nil
}

// PreviewParse 预览解析结果。
func (a *App) PreviewParse(req models.PreviewParseRequest) (*models.PreviewResult, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	if len(req.Samples) == 0 {
		return nil, errs.InvalidInput("样本不能为空", nil)
	}
	if req.Template.Format == "regex" && req.Template.Regex != "" {
		if _, err := regexp.Compile(req.Template.Regex); err != nil {
			return nil, errs.InvalidInput("正则表达式无效: "+err.Error(), err)
		}
	}
	result := a.parserEngine.Preview(req.Template, req.Samples)
	return &result, nil
}

// GenerateTemplateByAI 使用 AI 生成解析模板。
func (a *App) GenerateTemplateByAI(req models.AITemplateRequest) (*models.ParseTemplate, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	if len(req.Samples) == 0 {
		return nil, errs.InvalidInput("样本不能为空", nil)
	}
	if a.aiMgr == nil {
		return nil, errs.ErrAIProviderNotConfigured
	}
	provider, err := a.aiMgr.SelectedProvider(req.Provider)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(req.TemplateName)
	if name == "" {
		name = "AI 生成模板 " + time.Now().Format("01-02 15:04")
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	complete := func(callCtx context.Context, system, prompt string) (string, error) {
		return a.aiMgr.Complete(callCtx, provider, system, prompt, true)
	}
	tpl, err := a.parserEngine.GenerateTemplateWithAI(ctx, req.Samples, name, req.Category, complete)
	if err != nil {
		return nil, err
	}
	tpl.Type = "ai_generated"
	tpl.Source = "ai"
	return tpl, nil
}
