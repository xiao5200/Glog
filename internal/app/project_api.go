package app

import (
	"strings"

	"glog/internal/errs"
	"glog/pkg/models"
)

// CreateProject 创建项目。
func (a *App) CreateProject(req models.CreateProjectRequest) (*models.Project, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return nil, errs.InvalidInput("项目名称不能为空", nil)
	}
	if req.Priority == "" {
		req.Priority = "medium"
	}
	return a.projectsSvc.Create(req)
}

// ListProjects 列出项目。
func (a *App) ListProjects(filter models.ProjectFilter) ([]models.Project, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.projectsSvc.List(filter)
}

// GetProject 获取项目详情。
func (a *App) GetProject(projectID string) (*models.Project, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.projectsSvc.Get(projectID)
}

// UpdateProject 更新项目。
func (a *App) UpdateProject(projectID string, updates models.ProjectUpdates) (*models.Project, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	p, err := a.projectsSvc.Update(projectID, updates)
	if err != nil {
		return nil, err
	}

	a.mu.Lock()
	if a.currentProject != nil && a.currentProject.ID == projectID {
		a.currentProject = p
	}
	a.mu.Unlock()
	return p, nil
}

// DeleteProject 删除项目（若为当前项目会先关闭）。
func (a *App) DeleteProject(projectID string) error {
	if err := a.ready(); err != nil {
		return err
	}
	a.mu.RLock()
	isCurrent := a.currentProject != nil && a.currentProject.ID == projectID
	a.mu.RUnlock()
	if isCurrent {
		if err := a.CloseProject(); err != nil {
			return err
		}
	}
	return a.projectsSvc.Delete(projectID)
}

// ArchiveProject 归档项目。
func (a *App) ArchiveProject(projectID string) error {
	if err := a.ready(); err != nil {
		return err
	}
	if err := a.projectsSvc.Archive(projectID); err != nil {
		return err
	}
	a.mu.Lock()
	if a.currentProject != nil && a.currentProject.ID == projectID {
		a.refreshCurrentProjectLocked()
	}
	a.mu.Unlock()
	return nil
}

// ExportProject 导出项目包，返回实际生成的 zip 路径。
func (a *App) ExportProject(projectID, outputPath string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.projectsSvc.Export(projectID, outputPath)
}

// ImportProject 导入项目包。
func (a *App) ImportProject(packagePath string) (*models.Project, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(packagePath) == "" {
		return nil, errs.InvalidInput("项目包路径不能为空", nil)
	}
	return a.projectsSvc.Import(packagePath)
}

// GetProjectStatistics 获取项目统计。
func (a *App) GetProjectStatistics(projectID string) (*models.ProjectStatistics, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.projectsSvc.RecomputeStats(projectID)
}

// GetProjectSources 获取项目日志源。
func (a *App) GetProjectSources(projectID string) ([]models.LogSource, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.projectsSvc.ListSources(projectID)
}
