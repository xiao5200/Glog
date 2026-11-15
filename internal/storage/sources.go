package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"glog/internal/errs"
	"glog/pkg/models"
)

const logSourceSelectColumns = `id, COALESCE(type, 'local'), COALESCE(host, ''),
	COALESCE(path, ''), COALESCE(parse_mode, 'builtin'), COALESCE(template_id, ''),
	COALESCE(log_count, 0), COALESCE(size_bytes, 0), time_range_start, time_range_end,
	created_at, COALESCE(ai_provider, '')`

func scanLogSource(row rowScanner) (*models.LogSource, error) {
	var (
		src            models.LogSource
		trStart, trEnd sql.NullString
		createdAt      string
	)
	err := row.Scan(
		&src.ID, &src.Type, &src.Host, &src.Path, &src.ParseMode, &src.TemplateID,
		&src.LogCount, &src.SizeBytes, &trStart, &trEnd, &createdAt, &src.AIProvider,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("日志源不存在: %w", errs.ErrNotFound)
		}
		return nil, fmt.Errorf("读取日志源记录失败: %w", err)
	}

	if src.CreatedAt, err = scanTime(createdAt); err != nil {
		return nil, fmt.Errorf("解析日志源创建时间失败: %w", err)
	}
	start, err := scanTimePtr(trStart)
	if err != nil {
		return nil, fmt.Errorf("解析日志源时间范围失败: %w", err)
	}
	end, err := scanTimePtr(trEnd)
	if err != nil {
		return nil, fmt.Errorf("解析日志源时间范围失败: %w", err)
	}
	if start != nil || end != nil {
		src.TimeRange = &models.TimeRange{}
		if start != nil {
			src.TimeRange.Start = *start
		}
		if end != nil {
			src.TimeRange.End = *end
		}
	}
	return &src, nil
}

// insertLogSourceTx 在给定执行器（事务或 DB）中插入日志源。
func insertLogSourceTx(ex execer, src *models.LogSource, projectID string) error {
	if src == nil {
		return invalidInput("日志源不能为空")
	}
	if strings.TrimSpace(projectID) == "" {
		return invalidInput("日志源所属项目ID不能为空")
	}
	if strings.TrimSpace(src.ID) == "" {
		src.ID = generateID()
	}
	typ := strings.TrimSpace(src.Type)
	if typ == "" {
		typ = "local"
	}
	parseMode := strings.TrimSpace(src.ParseMode)
	if parseMode == "" {
		parseMode = "builtin"
	}
	createdAt := src.CreatedAt
	if createdAt.IsZero() {
		createdAt = nowUTC()
	}
	src.Type = typ
	src.ParseMode = parseMode
	src.CreatedAt = createdAt

	var trStart, trEnd any
	if src.TimeRange != nil {
		trStart = timeArg(src.TimeRange.Start)
		trEnd = timeArg(src.TimeRange.End)
	}

	_, err := ex.Exec(
		`INSERT INTO log_sources
			(id, project_id, type, host, path, parse_mode, template_id,
			 log_count, size_bytes, time_range_start, time_range_end, created_at, ai_provider)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		src.ID, projectID, typ, src.Host, src.Path, parseMode, src.TemplateID,
		src.LogCount, src.SizeBytes, trStart, trEnd, formatTime(createdAt), src.AIProvider,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return alreadyExists("日志源", src.ID)
		}
		if isForeignKeyViolation(err) {
			return fmt.Errorf("日志源所属项目不存在（ID=%s）: %w", projectID, errs.ErrNotFound)
		}
		return fmt.Errorf("保存日志源失败: %w", err)
	}
	return nil
}

// CreateLogSource 新增日志源。
//
// 注意：models.LogSource 不包含 ProjectID 字段，因此该接口无法独立确定所属项目。
// 新增日志源请使用 CreateLogSourceForProject，或通过 CreateProject 携带 Sources 写入。
// 若传入的 src.ID 已存在，本方法会按该 ID 更新日志源（保持原所属项目不变）。
func (s *MetadataStore) CreateLogSource(src *models.LogSource) error {
	if src == nil {
		return invalidInput("日志源不能为空")
	}
	if strings.TrimSpace(src.ID) == "" {
		src.ID = generateID()
	}

	_, err := s.GetLogSource(src.ID)
	switch {
	case err == nil:
		return s.UpdateLogSource(src)
	case !errors.Is(err, errs.ErrNotFound):
		return err
	default:
		return fmt.Errorf("无法确定日志源所属项目（LogSource 不含 project_id），请使用 CreateLogSourceForProject: %w", errs.ErrInvalidInput)
	}
}

// CreateLogSourceForProject 新增日志源并显式指定所属项目。
func (s *MetadataStore) CreateLogSourceForProject(projectID string, src *models.LogSource) error {
	if src == nil {
		return invalidInput("日志源不能为空")
	}
	if strings.TrimSpace(projectID) == "" {
		return invalidInput("日志源所属项目ID不能为空")
	}
	return insertLogSourceTx(s.db, src, projectID)
}

// UpdateLogSource 全量更新日志源的可变字段（不修改所属项目与创建时间）。
func (s *MetadataStore) UpdateLogSource(src *models.LogSource) error {
	if src == nil {
		return invalidInput("日志源不能为空")
	}
	id := strings.TrimSpace(src.ID)
	if id == "" {
		return invalidInput("日志源ID不能为空")
	}

	var trStart, trEnd any
	if src.TimeRange != nil {
		trStart = timeArg(src.TimeRange.Start)
		trEnd = timeArg(src.TimeRange.End)
	}

	res, err := s.db.Exec(
		`UPDATE log_sources SET type = ?, host = ?, path = ?, parse_mode = ?,
			template_id = ?, log_count = ?, size_bytes = ?,
			time_range_start = ?, time_range_end = ?, ai_provider = ?
		 WHERE id = ?`,
		src.Type, src.Host, src.Path, src.ParseMode, src.TemplateID,
		src.LogCount, src.SizeBytes, trStart, trEnd, src.AIProvider, id,
	)
	if err != nil {
		return fmt.Errorf("更新日志源失败: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("读取日志源更新结果失败: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("日志源不存在（ID=%s）: %w", id, errs.ErrNotFound)
	}
	return nil
}

// GetLogSource 按 ID 读取日志源。
func (s *MetadataStore) GetLogSource(id string) (*models.LogSource, error) {
	if strings.TrimSpace(id) == "" {
		return nil, invalidInput("日志源ID不能为空")
	}
	row := s.db.QueryRow("SELECT "+logSourceSelectColumns+" FROM log_sources WHERE id = ?", id)
	return scanLogSource(row)
}

// ListLogSources 按创建时间列出项目下所有日志源。
func (s *MetadataStore) ListLogSources(projectID string) ([]models.LogSource, error) {
	if strings.TrimSpace(projectID) == "" {
		return nil, invalidInput("项目ID不能为空")
	}
	rows, err := s.db.Query(
		"SELECT "+logSourceSelectColumns+" FROM log_sources WHERE project_id = ? ORDER BY created_at ASC, id ASC",
		projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("查询日志源列表失败: %w", err)
	}
	defer rows.Close()

	out := make([]models.LogSource, 0, 8)
	for rows.Next() {
		src, err := scanLogSource(rows)
		if err != nil {
			return nil, fmt.Errorf("读取日志源列表失败: %w", err)
		}
		out = append(out, *src)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历日志源列表失败: %w", err)
	}
	return out, nil
}

// DeleteLogSources 删除项目下全部日志源。
func (s *MetadataStore) DeleteLogSources(projectID string) error {
	if strings.TrimSpace(projectID) == "" {
		return invalidInput("项目ID不能为空")
	}
	if _, err := s.db.Exec("DELETE FROM log_sources WHERE project_id = ?", projectID); err != nil {
		return fmt.Errorf("删除日志源失败: %w", err)
	}
	return nil
}
