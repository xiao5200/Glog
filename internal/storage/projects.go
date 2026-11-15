package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"glog/internal/errs"
	"glog/pkg/models"
)

const projectSelectColumns = `id, name, COALESCE(description, ''), created_at, updated_at,
	COALESCE(status, 'active'), COALESCE(project_path, ''), COALESCE(log_count, 0),
	COALESCE(size_bytes, 0), time_range_start, time_range_end, tags,
	COALESCE(priority, 'medium'), level_distribution`

// scanProject 从一行结果读取项目。
func scanProject(row rowScanner) (*models.Project, error) {
	var (
		p                    models.Project
		createdAt, updatedAt string
		trStart, trEnd       sql.NullString
		tagsRaw, levelsRaw   sql.NullString
	)
	err := row.Scan(
		&p.ID, &p.Name, &p.Description, &createdAt, &updatedAt, &p.Status, &p.ProjectPath,
		&p.LogCount, &p.SizeBytes, &trStart, &trEnd, &tagsRaw, &p.Priority, &levelsRaw,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("项目不存在: %w", errs.ErrNotFound)
		}
		return nil, fmt.Errorf("读取项目记录失败: %w", err)
	}

	if p.CreatedAt, err = scanTime(createdAt); err != nil {
		return nil, fmt.Errorf("解析项目创建时间失败: %w", err)
	}
	if p.UpdatedAt, err = scanTime(updatedAt); err != nil {
		return nil, fmt.Errorf("解析项目更新时间失败: %w", err)
	}
	start, err := scanTimePtr(trStart)
	if err != nil {
		return nil, fmt.Errorf("解析项目时间范围失败: %w", err)
	}
	end, err := scanTimePtr(trEnd)
	if err != nil {
		return nil, fmt.Errorf("解析项目时间范围失败: %w", err)
	}
	if start != nil || end != nil {
		p.TimeRange = &models.TimeRange{}
		if start != nil {
			p.TimeRange.Start = *start
		}
		if end != nil {
			p.TimeRange.End = *end
		}
	}
	if err := decodeJSON(tagsRaw, &p.Tags); err != nil {
		return nil, fmt.Errorf("解析项目标签失败: %w", err)
	}
	if p.Tags == nil {
		p.Tags = []string{}
	}
	if err := decodeJSON(levelsRaw, &p.Levels); err != nil {
		return nil, fmt.Errorf("解析项目级别分布失败: %w", err)
	}
	if p.Levels == nil {
		p.Levels = map[string]int64{}
	}
	return &p, nil
}

// CreateProject 插入项目。若 p.Sources 非空，会在同一事务内一并写入日志源。
func (s *MetadataStore) CreateProject(p *models.Project) error {
	if p == nil {
		return invalidInput("项目不能为空")
	}
	id := strings.TrimSpace(p.ID)
	name := strings.TrimSpace(p.Name)
	if id == "" {
		return invalidInput("项目ID不能为空")
	}
	if name == "" {
		return invalidInput("项目名称不能为空")
	}

	now := nowUTC()
	createdAt := p.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}
	updatedAt := p.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = createdAt
	}
	status := strings.TrimSpace(p.Status)
	if status == "" {
		status = "active"
	}
	priority := strings.TrimSpace(p.Priority)
	if priority == "" {
		priority = "medium"
	}

	tagsJSON, err := marshalJSON(nonNilStrings(p.Tags))
	if err != nil {
		return fmt.Errorf("序列化项目标签失败: %w", err)
	}
	levelsJSON, err := marshalJSON(nonNilInt64Map(p.Levels))
	if err != nil {
		return fmt.Errorf("序列化项目级别分布失败: %w", err)
	}

	var trStart, trEnd any
	if p.TimeRange != nil {
		trStart = timeArg(p.TimeRange.Start)
		trEnd = timeArg(p.TimeRange.End)
	}

	p.CreatedAt = createdAt
	p.UpdatedAt = updatedAt
	p.Status = status
	p.Priority = priority

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("开启项目事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.Exec(
		`INSERT INTO projects
			(id, name, description, created_at, updated_at, status, project_path,
			 log_count, size_bytes, time_range_start, time_range_end, tags, priority, level_distribution)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, name, p.Description, formatTime(createdAt), formatTime(updatedAt), status, p.ProjectPath,
		p.LogCount, p.SizeBytes, trStart, trEnd, tagsJSON, priority, levelsJSON,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return alreadyExists("项目", id)
		}
		return fmt.Errorf("保存项目失败: %w", err)
	}

	for i := range p.Sources {
		if err := insertLogSourceTx(tx, &p.Sources[i], id); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交项目事务失败: %w", err)
	}
	return nil
}

// GetProject 按 ID 读取项目。日志源需通过 ListLogSources 单独读取。
func (s *MetadataStore) GetProject(id string) (*models.Project, error) {
	if strings.TrimSpace(id) == "" {
		return nil, invalidInput("项目ID不能为空")
	}
	row := s.db.QueryRow("SELECT "+projectSelectColumns+" FROM projects WHERE id = ?", id)
	p, err := scanProject(row)
	if err != nil {
		return nil, err
	}
	return p, nil
}

// UpdateProject 全量更新项目的可变字段。Sources 不在此方法中处理。
func (s *MetadataStore) UpdateProject(p *models.Project) error {
	if p == nil {
		return invalidInput("项目不能为空")
	}
	id := strings.TrimSpace(p.ID)
	name := strings.TrimSpace(p.Name)
	if id == "" {
		return invalidInput("项目ID不能为空")
	}
	if name == "" {
		return invalidInput("项目名称不能为空")
	}

	updatedAt := p.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = nowUTC()
	}
	status := strings.TrimSpace(p.Status)
	priority := strings.TrimSpace(p.Priority)

	tagsJSON, err := marshalJSON(nonNilStrings(p.Tags))
	if err != nil {
		return fmt.Errorf("序列化项目标签失败: %w", err)
	}
	levelsJSON, err := marshalJSON(nonNilInt64Map(p.Levels))
	if err != nil {
		return fmt.Errorf("序列化项目级别分布失败: %w", err)
	}

	var trStart, trEnd any
	if p.TimeRange != nil {
		trStart = timeArg(p.TimeRange.Start)
		trEnd = timeArg(p.TimeRange.End)
	}

	res, err := s.db.Exec(
		`UPDATE projects SET
			name = ?, description = ?, updated_at = ?,
			status = COALESCE(NULLIF(?, ''), status),
			project_path = COALESCE(NULLIF(?, ''), project_path),
			log_count = ?, size_bytes = ?,
			time_range_start = ?, time_range_end = ?, tags = ?,
			priority = COALESCE(NULLIF(?, ''), priority), level_distribution = ?
		 WHERE id = ?`,
		name, p.Description, formatTime(updatedAt), status, p.ProjectPath,
		p.LogCount, p.SizeBytes, trStart, trEnd, tagsJSON, priority, levelsJSON, id,
	)
	if err != nil {
		return fmt.Errorf("更新项目失败: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("读取项目更新结果失败: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("项目不存在（ID=%s）: %w", id, errs.ErrNotFound)
	}
	return nil
}

// DeleteProject 删除项目；log_sources/investigations/query_snapshots 由外键级联删除。
func (s *MetadataStore) DeleteProject(id string) error {
	if strings.TrimSpace(id) == "" {
		return invalidInput("项目ID不能为空")
	}
	res, err := s.db.Exec("DELETE FROM projects WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("删除项目失败: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("读取项目删除结果失败: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("项目不存在（ID=%s）: %w", id, errs.ErrNotFound)
	}
	return nil
}

// ListProjects 按筛选条件列出项目。
func (s *MetadataStore) ListProjects(f models.ProjectFilter) ([]models.Project, error) {
	var (
		where []string
		args  []any
	)

	status := strings.ToLower(strings.TrimSpace(f.Status))
	switch status {
	case "", "all":
		// 不过滤
	case "active", "archived":
		where = append(where, "status = ?")
		args = append(args, status)
	default:
		return nil, invalidInput("无效的项目状态筛选 %q（可选 active/archived/all）", f.Status)
	}

	if search := strings.TrimSpace(f.Search); search != "" {
		pattern := likeContains(search)
		where = append(where,
			"(name LIKE ? ESCAPE '!' OR COALESCE(description, '') LIKE ? ESCAPE '!')")
		args = append(args, pattern, pattern)
	}

	sortColumn, sortDir, err := projectOrderClause(f)
	if err != nil {
		return nil, err
	}

	query := "SELECT " + projectSelectColumns + " FROM projects"
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY " + sortColumn + " " + sortDir + ", id ASC"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("查询项目列表失败: %w", err)
	}
	defer rows.Close()

	out := make([]models.Project, 0, 16)
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, fmt.Errorf("读取项目列表失败: %w", err)
		}
		out = append(out, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历项目列表失败: %w", err)
	}

	if len(f.Tags) > 0 {
		out = filterProjectsByTags(out, f.Tags)
	}
	return out, nil
}

// projectOrderClause 返回白名单内的排序字段与方向。
func projectOrderClause(f models.ProjectFilter) (string, string, error) {
	sortBy := strings.ToLower(strings.TrimSpace(f.SortBy))
	if sortBy == "" {
		return "updated_at", "DESC", nil
	}

	dir := "ASC"
	if f.SortDesc {
		dir = "DESC"
	}

	switch sortBy {
	case "created_at":
		return "created_at", dir, nil
	case "updated_at":
		return "updated_at", dir, nil
	case "log_count":
		return "log_count", dir, nil
	case "size_bytes", "size":
		return "size_bytes", dir, nil
	case "name":
		return "name COLLATE NOCASE", dir, nil
	default:
		return "", "", invalidInput("无效的排序字段 %q（可选 created_at/updated_at/log_count/size_bytes/name）", f.SortBy)
	}
}

// filterProjectsByTags 实现“标签任一匹配”。
func filterProjectsByTags(list []models.Project, filterTags []string) []models.Project {
	wanted := make(map[string]struct{}, len(filterTags))
	for _, tag := range filterTags {
		tag = strings.TrimSpace(tag)
		if tag != "" {
			wanted[tag] = struct{}{}
		}
	}
	if len(wanted) == 0 {
		return list
	}

	out := make([]models.Project, 0, len(list))
	for _, p := range list {
		for _, tag := range p.Tags {
			if _, ok := wanted[tag]; ok {
				out = append(out, p)
				break
			}
		}
	}
	return out
}
