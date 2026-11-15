package app

import (
	"strings"

	"glog/internal/errs"
	"glog/pkg/models"
)

// ImportIOC 导入 IOC 文件。
func (a *App) ImportIOC(filePath string) (*models.IOCImportResult, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	_, threatSvc, err := a.requireServices()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(filePath) == "" {
		return nil, errs.InvalidInput("IOC 文件路径不能为空", nil)
	}
	return threatSvc.ImportIOC(filePath)
}

// ListIOCs 列出 IOC。
func (a *App) ListIOCs() ([]models.IOCItem, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	list, err := a.metadata.ListIOCs()
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []models.IOCItem{}
	}
	return list, nil
}

// DeleteIOC 删除 IOC。
func (a *App) DeleteIOC(iocID string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.metadata.DeleteIOC(iocID)
}

// SearchIOC 检索 IOC。
func (a *App) SearchIOC(req models.IOCSearchRequest) (*models.IOCSearchResult, error) {
	if _, _, _, err := a.requireCurrent(req.ProjectID); err != nil {
		return nil, err
	}
	_, threatSvc, err := a.requireServices()
	if err != nil {
		return nil, err
	}
	if len(req.Values) == 0 {
		return nil, errs.InvalidInput("IOC 值不能为空", nil)
	}
	return threatSvc.SearchIOC(req.ProjectID, req.Values, req.Query)
}

// GetDetectionRules 获取检测规则。
func (a *App) GetDetectionRules() ([]models.DetectionRule, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	list, err := a.metadata.ListDetectionRules()
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []models.DetectionRule{}
	}
	return list, nil
}

// EnableRule 启用检测规则。
func (a *App) EnableRule(ruleID string) error {
	if err := a.ready(); err != nil {
		return err
	}
	if _, threatSvc, err := a.requireServices(); err == nil {
		return threatSvc.SetRuleEnabled(ruleID, true)
	}
	return a.metadata.SetDetectionRuleEnabled(ruleID, true)
}

// DisableRule 禁用检测规则。
func (a *App) DisableRule(ruleID string) error {
	if err := a.ready(); err != nil {
		return err
	}
	if _, threatSvc, err := a.requireServices(); err == nil {
		return threatSvc.SetRuleEnabled(ruleID, false)
	}
	return a.metadata.SetDetectionRuleEnabled(ruleID, false)
}

// RunDetection 运行检测规则。
func (a *App) RunDetection(req models.RunDetectionRequest) (*models.DetectionResult, error) {
	if _, _, _, err := a.requireCurrent(req.ProjectID); err != nil {
		return nil, err
	}
	_, threatSvc, err := a.requireServices()
	if err != nil {
		return nil, err
	}
	return threatSvc.RunDetection(req.ProjectID, req.RuleIDs)
}

// GenerateForensicsPackage 生成取证包。
func (a *App) GenerateForensicsPackage(req models.ForensicsRequest) (*models.ForensicsResult, error) {
	if _, _, _, err := a.requireCurrent(req.ProjectID); err != nil {
		return nil, err
	}
	invSvc, threatSvc, err := a.requireServices()
	if err != nil {
		return nil, err
	}

	var inv *models.Investigation
	var evidence []models.EvidenceItem
	if req.InvestigationID != "" {
		inv, err = invSvc.Get(req.InvestigationID)
		if err != nil {
			return nil, err
		}
		evidence, err = invSvc.ListEvidence(req.InvestigationID)
		if err != nil {
			return nil, err
		}
	}
	return threatSvc.GenerateForensics(req, inv, evidence)
}
