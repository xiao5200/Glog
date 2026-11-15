package app

import (
	"strings"

	"glog/internal/errs"
	"glog/pkg/models"
)

// Search 执行日志查询。
func (a *App) Search(projectID string, query models.Query) (*models.QueryResult, error) {
	_, store, engine, err := a.requireCurrent(projectID)
	if err != nil {
		return nil, err
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	if query.Limit <= 0 {
		page := 100
		if a.configMgr != nil {
			page = a.configMgr.Config().UI.PageSize
		}
		if page <= 0 {
			page = 100
		}
		query.Limit = page
	}
	if query.Limit > 1000 {
		query.Limit = 1000
	}
	if query.SortOrder == "" {
		query.SortOrder = "desc"
	}
	if query.SortBy == "" {
		query.SortBy = "timestamp"
	}
	_ = store
	return engine.Search(query)
}

// GetLogEntry 获取单条日志。
func (a *App) GetLogEntry(projectID string, docID uint32) (*models.LogEntry, error) {
	_, store, _, err := a.requireCurrent(projectID)
	if err != nil {
		return nil, err
	}
	return store.ReadDoc(docID)
}

// GetLogContext 获取日志上下文。
func (a *App) GetLogContext(projectID string, docID uint32, before, after int) (*models.ContextResult, error) {
	_, _, engine, err := a.requireCurrent(projectID)
	if err != nil {
		return nil, err
	}
	if before < 0 {
		before = 0
	}
	if after < 0 {
		after = 0
	}
	if before == 0 && after == 0 {
		before, after = 10, 10
	}
	if before > 500 {
		before = 500
	}
	if after > 500 {
		after = 500
	}
	return engine.Context(docID, before, after)
}

// Aggregate 聚合查询。
func (a *App) Aggregate(projectID string, req models.AggregateRequest) (*models.AggregateResult, error) {
	_, _, engine, err := a.requireCurrent(projectID)
	if err != nil {
		return nil, err
	}
	if req.TopN <= 0 {
		req.TopN = 10
	}
	if req.TopN > 100 {
		req.TopN = 100
	}
	return engine.Aggregate(req)
}

// ClusterPatterns 日志模式聚类。
func (a *App) ClusterPatterns(projectID string, query models.Query) (*models.PatternClusterResult, error) {
	_, _, engine, err := a.requireCurrent(projectID)
	if err != nil {
		return nil, err
	}
	return engine.Cluster(query)
}

// GetQuerySnapshots 获取查询快照列表。
func (a *App) GetQuerySnapshots(projectID string) ([]models.QuerySnapshot, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	list, err := a.metadata.ListQuerySnapshots(projectID)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []models.QuerySnapshot{}
	}
	return list, nil
}

// SaveQuerySnapshot 保存查询快照。
func (a *App) SaveQuerySnapshot(snapshot models.QuerySnapshot) error {
	if err := a.ready(); err != nil {
		return err
	}
	snapshot.Name = strings.TrimSpace(snapshot.Name)
	if snapshot.Name == "" {
		return errs.InvalidInput("快照名称不能为空", nil)
	}
	if snapshot.ID == "" {
		snapshot.ID = newID()
	}
	if snapshot.ProjectID == "" {
		a.mu.RLock()
		if a.currentProject != nil {
			snapshot.ProjectID = a.currentProject.ID
		}
		a.mu.RUnlock()
	}
	if snapshot.ProjectID == "" {
		return errs.ErrProjectNotOpen
	}
	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = nowUTC()
	}
	return a.metadata.SaveQuerySnapshot(&snapshot)
}

// DeleteQuerySnapshot 删除查询快照。
func (a *App) DeleteQuerySnapshot(snapshotID string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.metadata.DeleteQuerySnapshot(snapshotID)
}
