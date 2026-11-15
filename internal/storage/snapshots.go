package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"glog/internal/errs"
	"glog/pkg/models"
)

const snapshotSelectColumns = `id, project_id, name, COALESCE(description, ''),
	query_json, COALESCE(result_count, 0), created_at`

func scanSnapshot(row rowScanner) (*models.QuerySnapshot, error) {
	var (
		s            models.QuerySnapshot
		queryRaw     string
		createdAtRaw string
	)
	err := row.Scan(&s.ID, &s.ProjectID, &s.Name, &s.Description, &queryRaw, &s.ResultCount, &createdAtRaw)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("查询快照不存在: %w", errs.ErrNotFound)
		}
		return nil, fmt.Errorf("读取查询快照失败: %w", err)
	}
	if err := decodeJSONString(queryRaw, &s.Query); err != nil {
		return nil, fmt.Errorf("解析查询快照条件失败: %w", err)
	}
	if s.CreatedAt, err = scanTime(createdAtRaw); err != nil {
		return nil, fmt.Errorf("解析查询快照创建时间失败: %w", err)
	}
	return &s, nil
}

// SaveQuerySnapshot 保存查询快照（按 ID 插入或更新）。
func (s *MetadataStore) SaveQuerySnapshot(q *models.QuerySnapshot) error {
	if q == nil {
		return invalidInput("查询快照不能为空")
	}
	if strings.TrimSpace(q.ProjectID) == "" {
		return invalidInput("查询快照所属项目ID不能为空")
	}
	if strings.TrimSpace(q.Name) == "" {
		return invalidInput("查询快照名称不能为空")
	}
	if strings.TrimSpace(q.ID) == "" {
		q.ID = generateID()
	}

	queryJSON, err := marshalJSON(q.Query)
	if err != nil {
		return fmt.Errorf("序列化查询快照条件失败: %w", err)
	}
	createdAt := q.CreatedAt
	if createdAt.IsZero() {
		createdAt = nowUTC()
	}

	_, err = s.db.Exec(
		`INSERT INTO query_snapshots
			(id, project_id, name, description, query_json, result_count, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
			project_id = excluded.project_id,
			name = excluded.name,
			description = excluded.description,
			query_json = excluded.query_json,
			result_count = excluded.result_count`,
		q.ID, q.ProjectID, q.Name, q.Description, queryJSON, q.ResultCount, formatTime(createdAt),
	)
	if err != nil {
		if isForeignKeyViolation(err) {
			return fmt.Errorf("查询快照所属项目不存在（ID=%s）: %w", q.ProjectID, errs.ErrNotFound)
		}
		return fmt.Errorf("保存查询快照失败: %w", err)
	}
	q.CreatedAt = createdAt
	return nil
}

// ListQuerySnapshots 列出项目的查询快照（按创建时间倒序）。
func (s *MetadataStore) ListQuerySnapshots(projectID string) ([]models.QuerySnapshot, error) {
	if strings.TrimSpace(projectID) == "" {
		return nil, invalidInput("项目ID不能为空")
	}
	rows, err := s.db.Query(
		"SELECT "+snapshotSelectColumns+
			" FROM query_snapshots WHERE project_id = ? ORDER BY created_at DESC, id ASC",
		projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("查询快照列表查询失败: %w", err)
	}
	defer rows.Close()

	out := make([]models.QuerySnapshot, 0, 8)
	for rows.Next() {
		snapshot, err := scanSnapshot(rows)
		if err != nil {
			return nil, fmt.Errorf("读取查询快照列表失败: %w", err)
		}
		out = append(out, *snapshot)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历查询快照列表失败: %w", err)
	}
	return out, nil
}

// DeleteQuerySnapshot 删除查询快照。
func (s *MetadataStore) DeleteQuerySnapshot(id string) error {
	if strings.TrimSpace(id) == "" {
		return invalidInput("查询快照ID不能为空")
	}
	res, err := s.db.Exec("DELETE FROM query_snapshots WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("删除查询快照失败: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("读取查询快照删除结果失败: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("查询快照不存在（ID=%s）: %w", id, errs.ErrNotFound)
	}
	return nil
}
