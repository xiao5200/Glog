package app

import (
	"strings"

	"glog/internal/errs"
	"glog/pkg/models"
)

// CreateInvestigation 创建调查工单。
func (a *App) CreateInvestigation(req models.CreateInvestigationRequest) (*models.Investigation, error) {
	if _, _, _, err := a.requireCurrent(req.ProjectID); err != nil {
		return nil, err
	}
	invSvc, _, err := a.requireServices()
	if err != nil {
		return nil, err
	}
	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" {
		return nil, errs.InvalidInput("工单标题不能为空", nil)
	}
	return invSvc.Create(req)
}

// UpdateInvestigation 更新调查工单。
func (a *App) UpdateInvestigation(invID string, updates models.InvestigationUpdates) (*models.Investigation, error) {
	invSvc, _, err := a.requireServices()
	if err != nil {
		return nil, err
	}
	return invSvc.Update(invID, updates)
}

// GetInvestigation 获取调查工单。
func (a *App) GetInvestigation(invID string) (*models.Investigation, error) {
	invSvc, _, err := a.requireServices()
	if err != nil {
		return nil, err
	}
	return invSvc.Get(invID)
}

// ListInvestigations 列出调查工单。
func (a *App) ListInvestigations(projectID string) ([]models.Investigation, error) {
	if projectID == "" {
		if _, _, _, err := a.requireCurrent(""); err != nil {
			return nil, err
		}
	} else if _, _, _, err := a.requireCurrent(projectID); err != nil {
		return nil, err
	}
	invSvc, _, err := a.requireServices()
	if err != nil {
		return nil, err
	}
	list, err := invSvc.List(projectID)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []models.Investigation{}
	}
	return list, nil
}

// CloseInvestigation 结案。
func (a *App) CloseInvestigation(invID string) error {
	invSvc, _, err := a.requireServices()
	if err != nil {
		return err
	}
	return invSvc.Close(invID)
}

// DeleteInvestigation 删除调查工单（关联证据标记一并删除）。
func (a *App) DeleteInvestigation(invID string) error {
	invSvc, _, err := a.requireServices()
	if err != nil {
		return err
	}
	return invSvc.Delete(invID)
}

// MarkEvidence 标记证据。
func (a *App) MarkEvidence(invID string, docID uint64, note string) error {
	invSvc, _, err := a.requireServices()
	if err != nil {
		return err
	}
	return invSvc.MarkEvidence(invID, docID, note)
}

// UnmarkEvidence 取消证据标记。
func (a *App) UnmarkEvidence(invID string, docID uint64) error {
	invSvc, _, err := a.requireServices()
	if err != nil {
		return err
	}
	return invSvc.UnmarkEvidence(invID, docID)
}

// ListEvidence 列出证据。
func (a *App) ListEvidence(invID string) ([]models.EvidenceItem, error) {
	invSvc, _, err := a.requireServices()
	if err != nil {
		return nil, err
	}
	list, err := invSvc.ListEvidence(invID)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []models.EvidenceItem{}
	}
	return list, nil
}

// GetInvestigationTimeline 获取调查时间线。
func (a *App) GetInvestigationTimeline(invID string) ([]models.TimelineEvent, error) {
	invSvc, _, err := a.requireServices()
	if err != nil {
		return nil, err
	}
	list, err := invSvc.Timeline(invID)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []models.TimelineEvent{}
	}
	return list, nil
}

// GetInvestigationRelatedLogs 获取与调查工单相关的日志（证据优先，其次线索命中）。
// limit 默认 100，最大 500。
func (a *App) GetInvestigationRelatedLogs(invID string, limit int) ([]models.LogEntry, error) {
	if _, _, _, err := a.requireCurrent(""); err != nil {
		return nil, err
	}
	invSvc, _, err := a.requireServices()
	if err != nil {
		return nil, err
	}
	logs, err := invSvc.RelatedLogs(invID, limit)
	if err != nil {
		return nil, err
	}
	if logs == nil {
		logs = []models.LogEntry{}
	}
	return logs, nil
}

// ExportEvidence 导出证据包。
func (a *App) ExportEvidence(invID, outputPath string) (string, error) {
	invSvc, _, err := a.requireServices()
	if err != nil {
		return "", err
	}
	return invSvc.ExportEvidence(invID, outputPath)
}
