package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"glog/internal/errs"
	"glog/pkg/models"
)

const investigationSelectColumns = `id, project_id, title, COALESCE(description, ''),
	COALESCE(clue_type, ''), COALESCE(clue_value, ''), COALESCE(status, 'open'),
	COALESCE(related_log_count, 0), time_range_start, time_range_end, ai_analysis,
	COALESCE(tags, '[]'), risk_score, created_at, updated_at, closed_at,
	COALESCE(assigned_to, '')`

func scanInvestigation(row rowScanner) (*models.Investigation, error) {
	var (
		inv            models.Investigation
		trStart, trEnd sql.NullString
		aiRaw, tagsRaw sql.NullString
		riskScore      sql.NullFloat64
		createdAt      string
		updatedAt      string
		closedAt       sql.NullString
	)
	err := row.Scan(
		&inv.ID, &inv.ProjectID, &inv.Title, &inv.Description, &inv.ClueType, &inv.ClueValue,
		&inv.Status, &inv.RelatedLogCount, &trStart, &trEnd, &aiRaw, &tagsRaw, &riskScore,
		&createdAt, &updatedAt, &closedAt, &inv.AssignedTo,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("调查工单不存在: %w", errs.ErrNotFound)
		}
		return nil, fmt.Errorf("读取调查工单失败: %w", err)
	}

	if inv.CreatedAt, err = scanTime(createdAt); err != nil {
		return nil, fmt.Errorf("解析调查工单创建时间失败: %w", err)
	}
	if inv.UpdatedAt, err = scanTime(updatedAt); err != nil {
		return nil, fmt.Errorf("解析调查工单更新时间失败: %w", err)
	}
	start, err := scanTimePtr(trStart)
	if err != nil {
		return nil, fmt.Errorf("解析调查工单时间范围失败: %w", err)
	}
	end, err := scanTimePtr(trEnd)
	if err != nil {
		return nil, fmt.Errorf("解析调查工单时间范围失败: %w", err)
	}
	if start != nil || end != nil {
		inv.TimeRange = &models.TimeRange{}
		if start != nil {
			inv.TimeRange.Start = *start
		}
		if end != nil {
			inv.TimeRange.End = *end
		}
	}
	analysis, err := decodeJSONPtr[models.AIAnalysisResult](aiRaw)
	if err != nil {
		return nil, fmt.Errorf("解析调查工单AI分析结果失败: %w", err)
	}
	inv.AIAnalysis = analysis
	if err := decodeJSON(tagsRaw, &inv.Tags); err != nil {
		return nil, fmt.Errorf("解析调查工单标签失败: %w", err)
	}
	if inv.Tags == nil {
		inv.Tags = []string{}
	}
	if riskScore.Valid {
		v := float32(riskScore.Float64)
		inv.RiskScore = &v
	}
	closed, err := scanTimePtr(closedAt)
	if err != nil {
		return nil, fmt.Errorf("解析调查工单结案时间失败: %w", err)
	}
	inv.ClosedAt = closed
	inv.EvidenceIDs = []string{}
	return &inv, nil
}

// CreateInvestigation 创建调查工单。
// 若 inv.EvidenceIDs 非空，会同时写入证据表，并以证据表为准维护 evidence_ids 列。
func (s *MetadataStore) CreateInvestigation(inv *models.Investigation) error {
	if inv == nil {
		return invalidInput("调查工单不能为空")
	}
	if strings.TrimSpace(inv.ProjectID) == "" {
		return invalidInput("调查工单所属项目ID不能为空")
	}
	if strings.TrimSpace(inv.Title) == "" {
		return invalidInput("调查工单标题不能为空")
	}
	if strings.TrimSpace(inv.ID) == "" {
		inv.ID = generateID()
	}

	now := nowUTC()
	createdAt := inv.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}
	updatedAt := inv.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = createdAt
	}
	status := strings.TrimSpace(inv.Status)
	if status == "" {
		status = "open"
	}
	closedAt := inv.ClosedAt
	if status == "closed" && (closedAt == nil || closedAt.IsZero()) {
		closedAt = &now
	}

	tagsJSON, err := marshalJSON(nonNilStrings(inv.Tags))
	if err != nil {
		return fmt.Errorf("序列化调查工单标签失败: %w", err)
	}
	aiArg, err := marshalJSONArg(inv.AIAnalysis)
	if err != nil {
		return fmt.Errorf("序列化调查工单AI分析结果失败: %w", err)
	}
	riskArg := float32Arg(inv.RiskScore)

	var trStart, trEnd any
	if inv.TimeRange != nil {
		trStart = timeArg(inv.TimeRange.Start)
		trEnd = timeArg(inv.TimeRange.End)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("开启调查工单事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.Exec(
		`INSERT INTO investigations
			(id, project_id, title, description, clue_type, clue_value, status,
			 related_log_count, time_range_start, time_range_end, ai_analysis, evidence_ids,
			 tags, risk_score, created_at, updated_at, closed_at, assigned_to)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '[]', ?, ?, ?, ?, ?, ?)`,
		inv.ID, inv.ProjectID, inv.Title, inv.Description, inv.ClueType, inv.ClueValue,
		status, inv.RelatedLogCount, trStart, trEnd, aiArg, tagsJSON, riskArg,
		formatTime(createdAt), formatTime(updatedAt), timePtrArg(closedAt), inv.AssignedTo,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return alreadyExists("调查工单", inv.ID)
		}
		if isForeignKeyViolation(err) {
			return fmt.Errorf("调查工单所属项目不存在（ID=%s）: %w", inv.ProjectID, errs.ErrNotFound)
		}
		return fmt.Errorf("保存调查工单失败: %w", err)
	}

	evidenceIDs := uniqueNonEmptyStrings(inv.EvidenceIDs)
	for _, logEntryID := range evidenceIDs {
		docID, _ := strconv.ParseUint(logEntryID, 10, 64)
		_, err := tx.Exec(
			`INSERT INTO evidence_marks
				(id, investigation_id, doc_id, log_entry_id, marked_at, note)
			 VALUES (?, ?, ?, ?, ?, '')
			 ON CONFLICT(investigation_id, log_entry_id) DO NOTHING`,
			generateID(), inv.ID, docID, logEntryID, formatTime(now),
		)
		if err != nil {
			return fmt.Errorf("保存初始证据标记失败: %w", err)
		}
	}
	if err := recomputeEvidenceIDsTx(tx, inv.ID); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交调查工单事务失败: %w", err)
	}

	inv.CreatedAt = createdAt
	inv.UpdatedAt = updatedAt
	inv.Status = status
	inv.ClosedAt = closedAt
	inv.EvidenceIDs = evidenceIDs
	return nil
}

// GetInvestigation 按 ID 读取调查工单，证据ID从证据表计算。
func (s *MetadataStore) GetInvestigation(id string) (*models.Investigation, error) {
	if strings.TrimSpace(id) == "" {
		return nil, invalidInput("调查工单ID不能为空")
	}
	row := s.db.QueryRow("SELECT "+investigationSelectColumns+" FROM investigations WHERE id = ?", id)
	inv, err := scanInvestigation(row)
	if err != nil {
		return nil, err
	}
	ids, err := s.loadEvidenceIDs(id)
	if err != nil {
		return nil, err
	}
	inv.EvidenceIDs = ids
	return inv, nil
}

// UpdateInvestigation 更新调查工单。证据ID不通过本方法修改。
func (s *MetadataStore) UpdateInvestigation(inv *models.Investigation) error {
	if inv == nil {
		return invalidInput("调查工单不能为空")
	}
	id := strings.TrimSpace(inv.ID)
	if id == "" {
		return invalidInput("调查工单ID不能为空")
	}
	if strings.TrimSpace(inv.Title) == "" {
		return invalidInput("调查工单标题不能为空")
	}

	now := nowUTC()
	updatedAt := inv.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = now
	}
	status := strings.TrimSpace(inv.Status)
	closedAt := inv.ClosedAt
	switch status {
	case "closed":
		if closedAt == nil || closedAt.IsZero() {
			closedAt = &now
		}
	case "open":
		closedAt = nil
	}

	tagsJSON, err := marshalJSON(nonNilStrings(inv.Tags))
	if err != nil {
		return fmt.Errorf("序列化调查工单标签失败: %w", err)
	}
	aiArg, err := marshalJSONArg(inv.AIAnalysis)
	if err != nil {
		return fmt.Errorf("序列化调查工单AI分析结果失败: %w", err)
	}
	riskArg := float32Arg(inv.RiskScore)

	var trStart, trEnd any
	if inv.TimeRange != nil {
		trStart = timeArg(inv.TimeRange.Start)
		trEnd = timeArg(inv.TimeRange.End)
	}

	res, err := s.db.Exec(
		`UPDATE investigations SET
			title = ?, description = ?, clue_type = ?, clue_value = ?,
			status = COALESCE(NULLIF(?, ''), status),
			related_log_count = ?, time_range_start = ?, time_range_end = ?,
			ai_analysis = ?, tags = ?, risk_score = ?, updated_at = ?,
			closed_at = ?, assigned_to = ?
		 WHERE id = ?`,
		inv.Title, inv.Description, inv.ClueType, inv.ClueValue, status,
		inv.RelatedLogCount, trStart, trEnd, aiArg, tagsJSON, riskArg,
		formatTime(updatedAt), timePtrArg(closedAt), inv.AssignedTo, id,
	)
	if err != nil {
		return fmt.Errorf("更新调查工单失败: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("读取调查工单更新结果失败: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("调查工单不存在（ID=%s）: %w", id, errs.ErrNotFound)
	}
	return nil
}

// ListInvestigations 列出项目下的调查工单（按创建时间倒序）。
func (s *MetadataStore) ListInvestigations(projectID string) ([]models.Investigation, error) {
	if strings.TrimSpace(projectID) == "" {
		return nil, invalidInput("项目ID不能为空")
	}
	rows, err := s.db.Query(
		"SELECT "+investigationSelectColumns+
			" FROM investigations WHERE project_id = ? ORDER BY created_at DESC, id ASC",
		projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("查询调查工单列表失败: %w", err)
	}

	out := make([]models.Investigation, 0, 8)
	for rows.Next() {
		inv, err := scanInvestigation(rows)
		if err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("读取调查工单列表失败: %w", err)
		}
		out = append(out, *inv)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("遍历调查工单列表失败: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("关闭调查工单结果集失败: %w", err)
	}

	if len(out) == 0 {
		return out, nil
	}

	evidenceByInv := make(map[string][]string, len(out))
	evRows, err := s.db.Query(
		`SELECT investigation_id, log_entry_id FROM evidence_marks
		 WHERE investigation_id IN (SELECT id FROM investigations WHERE project_id = ?)
		 ORDER BY marked_at ASC, id ASC`,
		projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("查询调查工单证据失败: %w", err)
	}
	for evRows.Next() {
		var invID, logEntryID string
		if err := evRows.Scan(&invID, &logEntryID); err != nil {
			_ = evRows.Close()
			return nil, fmt.Errorf("读取调查工单证据失败: %w", err)
		}
		evidenceByInv[invID] = append(evidenceByInv[invID], logEntryID)
	}
	if err := evRows.Err(); err != nil {
		_ = evRows.Close()
		return nil, fmt.Errorf("遍历调查工单证据失败: %w", err)
	}
	if err := evRows.Close(); err != nil {
		return nil, fmt.Errorf("关闭调查工单证据结果集失败: %w", err)
	}

	for i := range out {
		if ids, ok := evidenceByInv[out[i].ID]; ok {
			out[i].EvidenceIDs = ids
		}
	}
	return out, nil
}

// DeleteInvestigation 删除调查工单；证据标记由外键级联删除。
func (s *MetadataStore) DeleteInvestigation(id string) error {
	if strings.TrimSpace(id) == "" {
		return invalidInput("调查工单ID不能为空")
	}
	res, err := s.db.Exec("DELETE FROM investigations WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("删除调查工单失败: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("读取调查工单删除结果失败: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("调查工单不存在（ID=%s）: %w", id, errs.ErrNotFound)
	}
	return nil
}

// AddEvidence 添加证据标记。同一工单对同一日志重复标记时幂等返回。
func (s *MetadataStore) AddEvidence(m *models.EvidenceMark) error {
	if m == nil {
		return invalidInput("证据标记不能为空")
	}
	invID := strings.TrimSpace(m.InvestigationID)
	if invID == "" {
		return invalidInput("证据标记所属调查工单ID不能为空")
	}

	logEntryID := strings.TrimSpace(m.LogEntryID)
	if logEntryID == "" {
		logEntryID = strconv.FormatUint(m.DocID, 10)
	}
	docID := m.DocID
	if docID == 0 {
		if parsed, err := strconv.ParseUint(logEntryID, 10, 64); err == nil {
			docID = parsed
		}
	}
	markedAt := m.MarkedAt
	if markedAt.IsZero() {
		markedAt = nowUTC()
	}
	markID := strings.TrimSpace(m.ID)
	if markID == "" {
		markID = generateID()
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("开启证据标记事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := ensureInvestigationTx(tx, invID); err != nil {
		return err
	}

	res, err := tx.Exec(
		`INSERT INTO evidence_marks
			(id, investigation_id, doc_id, log_entry_id, marked_at, note)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(investigation_id, log_entry_id) DO NOTHING`,
		markID, invID, docID, logEntryID, formatTime(markedAt), m.Note,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return alreadyExists("证据标记", markID)
		}
		if isForeignKeyViolation(err) {
			return fmt.Errorf("调查工单不存在（ID=%s）: %w", invID, errs.ErrNotFound)
		}
		return fmt.Errorf("保存证据标记失败: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("读取证据标记结果失败: %w", err)
	}
	if affected > 0 {
		if err := recomputeEvidenceIDsTx(tx, invID); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交证据标记事务失败: %w", err)
	}

	m.ID = markID
	m.InvestigationID = invID
	m.LogEntryID = logEntryID
	m.DocID = docID
	m.MarkedAt = markedAt
	return nil
}

// RemoveEvidence 按工单与文档ID移除证据标记。
func (s *MetadataStore) RemoveEvidence(invID string, docID uint64) error {
	invID = strings.TrimSpace(invID)
	if invID == "" {
		return invalidInput("调查工单ID不能为空")
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("开启证据移除事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := ensureInvestigationTx(tx, invID); err != nil {
		return err
	}

	res, err := tx.Exec(
		`DELETE FROM evidence_marks
		 WHERE investigation_id = ? AND (doc_id = ? OR log_entry_id = ?)`,
		invID, docID, strconv.FormatUint(docID, 10),
	)
	if err != nil {
		return fmt.Errorf("删除证据标记失败: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("读取证据删除结果失败: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("证据标记不存在（工单=%s，文档=%d）: %w", invID, docID, errs.ErrNotFound)
	}
	if err := recomputeEvidenceIDsTx(tx, invID); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交证据移除事务失败: %w", err)
	}
	return nil
}

// ListEvidence 列出调查工单下的证据标记。
func (s *MetadataStore) ListEvidence(invID string) ([]models.EvidenceMark, error) {
	invID = strings.TrimSpace(invID)
	if invID == "" {
		return nil, invalidInput("调查工单ID不能为空")
	}

	var exists int
	err := s.db.QueryRow("SELECT 1 FROM investigations WHERE id = ?", invID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("调查工单不存在（ID=%s）: %w", invID, errs.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("检查调查工单失败: %w", err)
	}

	rows, err := s.db.Query(
		`SELECT id, investigation_id, doc_id, log_entry_id, marked_at, COALESCE(note, '')
		 FROM evidence_marks WHERE investigation_id = ?
		 ORDER BY marked_at ASC, id ASC`,
		invID,
	)
	if err != nil {
		return nil, fmt.Errorf("查询证据标记失败: %w", err)
	}
	defer rows.Close()

	out := make([]models.EvidenceMark, 0, 16)
	for rows.Next() {
		var (
			m         models.EvidenceMark
			markedRaw string
		)
		if err := rows.Scan(&m.ID, &m.InvestigationID, &m.DocID, &m.LogEntryID, &markedRaw, &m.Note); err != nil {
			return nil, fmt.Errorf("读取证据标记失败: %w", err)
		}
		if m.MarkedAt, err = scanTime(markedRaw); err != nil {
			return nil, fmt.Errorf("解析证据标记时间失败: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历证据标记失败: %w", err)
	}
	return out, nil
}

// ensureInvestigationTx 确认调查工单存在。
func ensureInvestigationTx(tx *sql.Tx, invID string) error {
	var one int
	err := tx.QueryRow("SELECT 1 FROM investigations WHERE id = ?", invID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("调查工单不存在（ID=%s）: %w", invID, errs.ErrNotFound)
	}
	if err != nil {
		return fmt.Errorf("检查调查工单失败: %w", err)
	}
	return nil
}

// loadEvidenceIDs 从证据表计算并返回工单的证据日志ID列表。
func (s *MetadataStore) loadEvidenceIDs(invID string) ([]string, error) {
	rows, err := s.db.Query(
		`SELECT log_entry_id FROM evidence_marks WHERE investigation_id = ?
		 ORDER BY marked_at ASC, id ASC`,
		invID,
	)
	if err != nil {
		return nil, fmt.Errorf("查询证据ID失败: %w", err)
	}
	defer rows.Close()

	ids := make([]string, 0, 8)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("读取证据ID失败: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历证据ID失败: %w", err)
	}
	return ids, nil
}

// recomputeEvidenceIDsTx 根据证据表重算 investigations.evidence_ids，保持两者一致。
func recomputeEvidenceIDsTx(tx *sql.Tx, invID string) error {
	rows, err := tx.Query(
		`SELECT log_entry_id FROM evidence_marks WHERE investigation_id = ?
		 ORDER BY marked_at ASC, id ASC`,
		invID,
	)
	if err != nil {
		return fmt.Errorf("查询证据ID失败: %w", err)
	}

	ids := make([]string, 0, 8)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return fmt.Errorf("读取证据ID失败: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("遍历证据ID失败: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("关闭证据结果集失败: %w", err)
	}

	data, err := marshalJSON(ids)
	if err != nil {
		return fmt.Errorf("序列化证据ID失败: %w", err)
	}
	if _, err := tx.Exec("UPDATE investigations SET evidence_ids = ? WHERE id = ?", data, invID); err != nil {
		return fmt.Errorf("更新调查工单证据ID失败: %w", err)
	}
	return nil
}

func uniqueNonEmptyStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func float32Arg(v *float32) any {
	if v == nil {
		return nil
	}
	return float64(*v)
}
