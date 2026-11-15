package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"glog/internal/errs"
	"glog/pkg/models"
)

const detectionRuleSelectColumns = `id, name, COALESCE(description, ''), COALESCE(severity, 'medium'),
	COALESCE(enabled, 1), COALESCE(tags, '[]'), COALESCE(mitre_tactics, '[]'),
	COALESCE(mitre_techniques, '[]'), COALESCE(match_count, 0), last_matched_at,
	COALESCE(rule_file_path, ''), COALESCE(builtin, 0)`

func scanDetectionRule(row rowScanner) (*models.DetectionRule, error) {
	var (
		rule             models.DetectionRule
		enabled, builtin int
		tagsRaw          sql.NullString
		tacticsRaw       sql.NullString
		techniquesRaw    sql.NullString
		lastMatched      sql.NullString
	)
	err := row.Scan(
		&rule.ID, &rule.Name, &rule.Description, &rule.Severity, &enabled,
		&tagsRaw, &tacticsRaw, &techniquesRaw, &rule.MatchCount, &lastMatched,
		&rule.FilePath, &builtin,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("检测规则不存在: %w", errs.ErrNotFound)
		}
		return nil, fmt.Errorf("读取检测规则失败: %w", err)
	}
	rule.Enabled = enabled != 0
	rule.Builtin = builtin != 0
	if err := decodeJSON(tagsRaw, &rule.Tags); err != nil {
		return nil, fmt.Errorf("解析检测规则标签失败: %w", err)
	}
	if err := decodeJSON(tacticsRaw, &rule.MITRETactics); err != nil {
		return nil, fmt.Errorf("解析检测规则MITRE战术失败: %w", err)
	}
	if err := decodeJSON(techniquesRaw, &rule.MITRETechniques); err != nil {
		return nil, fmt.Errorf("解析检测规则MITRE技术失败: %w", err)
	}
	matchedAt, err := scanTimePtr(lastMatched)
	if err != nil {
		return nil, fmt.Errorf("解析检测规则最后匹配时间失败: %w", err)
	}
	rule.LastMatchedAt = matchedAt
	return &rule, nil
}

// UpsertDetectionRule 保存检测规则（按 ID 插入或更新）。
// 更新已有规则时保留启用状态、匹配计数与最后匹配时间（请使用
// SetDetectionRuleEnabled 修改启用状态）。
func (s *MetadataStore) UpsertDetectionRule(rule *models.DetectionRule) error {
	if rule == nil {
		return invalidInput("检测规则不能为空")
	}
	if strings.TrimSpace(rule.Name) == "" {
		return invalidInput("检测规则名称不能为空")
	}
	if strings.TrimSpace(rule.ID) == "" {
		rule.ID = generateID()
	}
	severity := strings.TrimSpace(rule.Severity)
	if severity == "" {
		severity = "medium"
	}
	enabled := 0
	if rule.Enabled {
		enabled = 1
	}
	builtin := 0
	if rule.Builtin {
		builtin = 1
	}

	now := nowUTC()
	tagsJSON, err := marshalJSONArg(rule.Tags)
	if err != nil {
		return fmt.Errorf("序列化检测规则标签失败: %w", err)
	}
	tacticsJSON, err := marshalJSONArg(rule.MITRETactics)
	if err != nil {
		return fmt.Errorf("序列化检测规则MITRE战术失败: %w", err)
	}
	techniquesJSON, err := marshalJSONArg(rule.MITRETechniques)
	if err != nil {
		return fmt.Errorf("序列化检测规则MITRE技术失败: %w", err)
	}
	matchCount := rule.MatchCount
	if matchCount < 0 {
		matchCount = 0
	}

	_, err = s.db.Exec(
		`INSERT INTO detection_rules
			(id, name, description, rule_file_path, enabled, severity, tags,
			 mitre_tactics, mitre_techniques, match_count, last_matched_at,
			 builtin, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			description = excluded.description,
			rule_file_path = excluded.rule_file_path,
			severity = excluded.severity,
			tags = excluded.tags,
			mitre_tactics = excluded.mitre_tactics,
			mitre_techniques = excluded.mitre_techniques,
			builtin = excluded.builtin,
			updated_at = excluded.updated_at`,
		rule.ID, rule.Name, rule.Description, rule.FilePath, enabled, severity,
		tagsJSON, tacticsJSON, techniquesJSON, matchCount, timePtrArg(rule.LastMatchedAt),
		builtin, formatTime(now), formatTime(now),
	)
	if err != nil {
		return fmt.Errorf("保存检测规则失败: %w", err)
	}
	return nil
}

// ListDetectionRules 列出全部检测规则。
func (s *MetadataStore) ListDetectionRules() ([]models.DetectionRule, error) {
	rows, err := s.db.Query(
		"SELECT " + detectionRuleSelectColumns +
			" FROM detection_rules ORDER BY name COLLATE NOCASE ASC, id ASC",
	)
	if err != nil {
		return nil, fmt.Errorf("查询检测规则列表失败: %w", err)
	}
	defer rows.Close()

	out := make([]models.DetectionRule, 0, 16)
	for rows.Next() {
		rule, err := scanDetectionRule(rows)
		if err != nil {
			return nil, fmt.Errorf("读取检测规则列表失败: %w", err)
		}
		out = append(out, *rule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历检测规则列表失败: %w", err)
	}
	return out, nil
}

// SetDetectionRuleEnabled 启用/禁用检测规则。
func (s *MetadataStore) SetDetectionRuleEnabled(id string, enabled bool) error {
	if strings.TrimSpace(id) == "" {
		return invalidInput("检测规则ID不能为空")
	}
	value := 0
	if enabled {
		value = 1
	}
	res, err := s.db.Exec(
		"UPDATE detection_rules SET enabled = ?, updated_at = ? WHERE id = ?",
		value, formatTime(nowUTC()), id,
	)
	if err != nil {
		return fmt.Errorf("更新检测规则启用状态失败: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("读取检测规则更新结果失败: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("检测规则不存在（ID=%s）: %w", id, errs.ErrNotFound)
	}
	return nil
}

// UpdateDetectionRuleMatch 记录一次规则命中。
func (s *MetadataStore) UpdateDetectionRuleMatch(id string, matchedAt time.Time) error {
	if strings.TrimSpace(id) == "" {
		return invalidInput("检测规则ID不能为空")
	}
	if matchedAt.IsZero() {
		matchedAt = nowUTC()
	}
	res, err := s.db.Exec(
		`UPDATE detection_rules SET match_count = match_count + 1,
			last_matched_at = ?, updated_at = ? WHERE id = ?`,
		formatTime(matchedAt), formatTime(nowUTC()), id,
	)
	if err != nil {
		return fmt.Errorf("更新检测规则匹配信息失败: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("读取检测规则更新结果失败: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("检测规则不存在（ID=%s）: %w", id, errs.ErrNotFound)
	}
	return nil
}

const iocSelectColumns = `id, COALESCE(ioc_type, ''), COALESCE(ioc_value, ''),
	COALESCE(threat_level, 'medium'), COALESCE(description, ''), COALESCE(source, ''),
	created_at`

func scanIOC(row rowScanner) (*models.IOCItem, error) {
	var (
		item         models.IOCItem
		createdAtRaw string
	)
	err := row.Scan(
		&item.ID, &item.Type, &item.Value, &item.ThreatLevel,
		&item.Description, &item.Source, &createdAtRaw,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("IOC指标不存在: %w", errs.ErrNotFound)
		}
		return nil, fmt.Errorf("读取IOC指标失败: %w", err)
	}
	if item.CreatedAt, err = scanTime(createdAtRaw); err != nil {
		return nil, fmt.Errorf("解析IOC创建时间失败: %w", err)
	}
	return &item, nil
}

// SaveIOC 保存 IOC 指标（按 ID 插入或更新）。
func (s *MetadataStore) SaveIOC(item *models.IOCItem) error {
	if item == nil {
		return invalidInput("IOC指标不能为空")
	}
	if strings.TrimSpace(item.Type) == "" {
		return invalidInput("IOC类型不能为空")
	}
	if strings.TrimSpace(item.Value) == "" {
		return invalidInput("IOC值不能为空")
	}
	if strings.TrimSpace(item.ID) == "" {
		item.ID = generateID()
	}
	threatLevel := strings.TrimSpace(item.ThreatLevel)
	if threatLevel == "" {
		threatLevel = "medium"
	}
	createdAt := item.CreatedAt
	if createdAt.IsZero() {
		createdAt = nowUTC()
	}

	_, err := s.db.Exec(
		`INSERT INTO ioc_indicators
			(id, ioc_type, ioc_value, threat_level, description, source, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
			ioc_type = excluded.ioc_type,
			ioc_value = excluded.ioc_value,
			threat_level = excluded.threat_level,
			description = excluded.description,
			source = excluded.source`,
		item.ID, strings.TrimSpace(item.Type), strings.TrimSpace(item.Value), threatLevel,
		item.Description, item.Source, formatTime(createdAt),
	)
	if err != nil {
		return fmt.Errorf("保存IOC指标失败: %w", err)
	}
	item.CreatedAt = createdAt
	return nil
}

// ListIOCs 列出全部 IOC 指标（按创建时间倒序）。
func (s *MetadataStore) ListIOCs() ([]models.IOCItem, error) {
	rows, err := s.db.Query(
		"SELECT " + iocSelectColumns + " FROM ioc_indicators ORDER BY created_at DESC, id ASC",
	)
	if err != nil {
		return nil, fmt.Errorf("查询IOC列表失败: %w", err)
	}
	defer rows.Close()

	out := make([]models.IOCItem, 0, 32)
	for rows.Next() {
		item, err := scanIOC(rows)
		if err != nil {
			return nil, fmt.Errorf("读取IOC列表失败: %w", err)
		}
		out = append(out, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历IOC列表失败: %w", err)
	}
	return out, nil
}

// DeleteIOC 删除 IOC 指标。
func (s *MetadataStore) DeleteIOC(id string) error {
	if strings.TrimSpace(id) == "" {
		return invalidInput("IOC指标ID不能为空")
	}
	res, err := s.db.Exec("DELETE FROM ioc_indicators WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("删除IOC指标失败: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("读取IOC删除结果失败: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("IOC指标不存在（ID=%s）: %w", id, errs.ErrNotFound)
	}
	return nil
}

// FindIOCByValue 按值查找 IOC 指标（ASCII 大小写不敏感）。
func (s *MetadataStore) FindIOCByValue(value string) (*models.IOCItem, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, invalidInput("IOC值不能为空")
	}
	row := s.db.QueryRow(
		"SELECT "+iocSelectColumns+" FROM ioc_indicators WHERE ioc_value = ? COLLATE NOCASE ORDER BY created_at ASC LIMIT 1",
		value,
	)
	item, err := scanIOC(row)
	if err != nil {
		return nil, err
	}
	return item, nil
}
