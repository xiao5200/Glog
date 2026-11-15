package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"glog/internal/errs"
	"glog/pkg/models"
)

const templateSelectColumns = `id, name, COALESCE(description, ''), COALESCE(type, 'custom'),
	COALESCE(category, ''), COALESCE(format, 'regex'), COALESCE(priority, 0), COALESCE(regex, ''),
	COALESCE(field_map, '{}'), COALESCE(time_format, ''), COALESCE(time_field, ''),
	COALESCE(level_map, '{}'), COALESCE(level_field, ''), level_extraction, multiline_config,
	examples, created_at, updated_at, COALESCE(usage_count, 0), COALESCE(source, 'user')`

func scanTemplate(row rowScanner) (*models.ParseTemplate, error) {
	var (
		t                        models.ParseTemplate
		fieldMapRaw, levelMapRaw string
		levelExtRaw, multiRaw    sql.NullString
		examplesRaw              sql.NullString
		createdAt, updatedAt     string
	)
	err := row.Scan(
		&t.ID, &t.Name, &t.Description, &t.Type, &t.Category, &t.Format, &t.Priority, &t.Regex,
		&fieldMapRaw, &t.TimeFormat, &t.TimeField, &levelMapRaw, &t.LevelField,
		&levelExtRaw, &multiRaw, &examplesRaw, &createdAt, &updatedAt, &t.UsageCount, &t.Source,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("解析模板不存在: %w", errs.ErrNotFound)
		}
		return nil, fmt.Errorf("读取解析模板失败: %w", err)
	}

	if t.CreatedAt, err = scanTime(createdAt); err != nil {
		return nil, fmt.Errorf("解析模板创建时间失败: %w", err)
	}
	if t.UpdatedAt, err = scanTime(updatedAt); err != nil {
		return nil, fmt.Errorf("解析模板更新时间失败: %w", err)
	}
	if err := decodeJSONString(fieldMapRaw, &t.FieldMap); err != nil {
		return nil, fmt.Errorf("解析模板字段映射失败: %w", err)
	}
	if t.FieldMap == nil {
		t.FieldMap = map[string]int{}
	}
	if err := decodeJSONString(levelMapRaw, &t.LevelMap); err != nil {
		return nil, fmt.Errorf("解析模板级别映射失败: %w", err)
	}
	if t.LevelMap == nil {
		t.LevelMap = map[string]string{}
	}
	levelExt, err := decodeJSONPtr[models.LevelExtraction](levelExtRaw)
	if err != nil {
		return nil, fmt.Errorf("解析模板级别提取规则失败: %w", err)
	}
	t.LevelExtraction = levelExt
	multi, err := decodeJSONPtr[models.MultiLineConfig](multiRaw)
	if err != nil {
		return nil, fmt.Errorf("解析模板多行配置失败: %w", err)
	}
	t.MultiLine = multi
	if err := decodeJSON(examplesRaw, &t.Examples); err != nil {
		return nil, fmt.Errorf("解析模板示例失败: %w", err)
	}
	return &t, nil
}

// SaveTemplate 保存模板（按 ID 插入或更新）。
// 更新已有模板时保留创建时间与使用次数。
func (s *MetadataStore) SaveTemplate(t *models.ParseTemplate) error {
	if t == nil {
		return invalidInput("解析模板不能为空")
	}
	if strings.TrimSpace(t.Name) == "" {
		return invalidInput("解析模板名称不能为空")
	}
	if strings.TrimSpace(t.ID) == "" {
		t.ID = generateID()
	}

	now := nowUTC()
	createdAt := t.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}
	updatedAt := t.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = now
	}
	typ := strings.TrimSpace(t.Type)
	if typ == "" {
		typ = "custom"
	}
	source := strings.TrimSpace(t.Source)
	if source == "" {
		source = "user"
	}
	format := strings.TrimSpace(t.Format)
	if format == "" {
		format = "regex"
	}
	usageCount := t.UsageCount
	if usageCount < 0 {
		usageCount = 0
	}

	fieldMapJSON, err := marshalJSON(nonNilIntMap(t.FieldMap))
	if err != nil {
		return fmt.Errorf("序列化模板字段映射失败: %w", err)
	}
	levelMapJSON, err := marshalJSON(nonNilStringMap(t.LevelMap))
	if err != nil {
		return fmt.Errorf("序列化模板级别映射失败: %w", err)
	}
	levelExtArg, err := marshalJSONArg(t.LevelExtraction)
	if err != nil {
		return fmt.Errorf("序列化模板级别提取规则失败: %w", err)
	}
	multiArg, err := marshalJSONArg(t.MultiLine)
	if err != nil {
		return fmt.Errorf("序列化模板多行配置失败: %w", err)
	}
	examplesArg, err := marshalJSONArg(t.Examples)
	if err != nil {
		return fmt.Errorf("序列化模板示例失败: %w", err)
	}

	_, err = s.db.Exec(
		`INSERT INTO parse_templates
			(id, name, description, type, category, format, priority, regex, field_map,
			 time_format, time_field, level_map, level_field, level_extraction, multiline_config,
			 examples, created_at, updated_at, usage_count, source)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			description = excluded.description,
			type = excluded.type,
			category = excluded.category,
			format = excluded.format,
			priority = excluded.priority,
			regex = excluded.regex,
			field_map = excluded.field_map,
			time_format = excluded.time_format,
			time_field = excluded.time_field,
			level_map = excluded.level_map,
			level_field = excluded.level_field,
			level_extraction = excluded.level_extraction,
			multiline_config = excluded.multiline_config,
			examples = excluded.examples,
			updated_at = excluded.updated_at,
			source = excluded.source`,
		t.ID, t.Name, t.Description, typ, t.Category, format, t.Priority, t.Regex, fieldMapJSON,
		t.TimeFormat, t.TimeField, levelMapJSON, t.LevelField, levelExtArg, multiArg,
		examplesArg, formatTime(createdAt), formatTime(updatedAt), usageCount, source,
	)
	if err != nil {
		return fmt.Errorf("保存解析模板失败: %w", err)
	}
	return nil
}

// GetTemplate 按 ID 读取模板。
func (s *MetadataStore) GetTemplate(id string) (*models.ParseTemplate, error) {
	if strings.TrimSpace(id) == "" {
		return nil, invalidInput("解析模板ID不能为空")
	}
	row := s.db.QueryRow("SELECT "+templateSelectColumns+" FROM parse_templates WHERE id = ?", id)
	return scanTemplate(row)
}

// ListTemplates 按类型/分类/关键词筛选模板。
func (s *MetadataStore) ListTemplates(f models.TemplateFilter) ([]models.ParseTemplate, error) {
	var (
		where []string
		args  []any
	)

	typ := strings.ToLower(strings.TrimSpace(f.Type))
	switch typ {
	case "", "all":
		// 不过滤
	default:
		where = append(where, "type = ?")
		args = append(args, typ)
	}

	if category := strings.TrimSpace(f.Category); category != "" {
		where = append(where, "category = ?")
		args = append(args, category)
	}

	if search := strings.TrimSpace(f.Search); search != "" {
		pattern := likeContains(search)
		where = append(where,
			"(name LIKE ? ESCAPE '!' OR COALESCE(description, '') LIKE ? ESCAPE '!')")
		args = append(args, pattern, pattern)
	}

	query := "SELECT " + templateSelectColumns + " FROM parse_templates"
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY priority DESC, usage_count DESC, name COLLATE NOCASE ASC, id ASC"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("查询解析模板列表失败: %w", err)
	}
	defer rows.Close()

	out := make([]models.ParseTemplate, 0, 16)
	for rows.Next() {
		t, err := scanTemplate(rows)
		if err != nil {
			return nil, fmt.Errorf("读取解析模板列表失败: %w", err)
		}
		out = append(out, *t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历解析模板列表失败: %w", err)
	}
	return out, nil
}

// DeleteTemplate 删除模板。
func (s *MetadataStore) DeleteTemplate(id string) error {
	if strings.TrimSpace(id) == "" {
		return invalidInput("解析模板ID不能为空")
	}
	res, err := s.db.Exec("DELETE FROM parse_templates WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("删除解析模板失败: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("读取模板删除结果失败: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("解析模板不存在（ID=%s）: %w", id, errs.ErrNotFound)
	}
	return nil
}

// IncrementTemplateUsage 增加模板使用次数。
func (s *MetadataStore) IncrementTemplateUsage(id string) error {
	if strings.TrimSpace(id) == "" {
		return invalidInput("解析模板ID不能为空")
	}
	res, err := s.db.Exec(
		"UPDATE parse_templates SET usage_count = usage_count + 1, updated_at = ? WHERE id = ?",
		formatTime(nowUTC()), id,
	)
	if err != nil {
		return fmt.Errorf("更新模板使用次数失败: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("读取模板更新结果失败: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("解析模板不存在（ID=%s）: %w", id, errs.ErrNotFound)
	}
	return nil
}

// CountTemplates 返回模板总数。
func (s *MetadataStore) CountTemplates() (int64, error) {
	var count int64
	if err := s.db.QueryRow("SELECT COUNT(*) FROM parse_templates").Scan(&count); err != nil {
		return 0, fmt.Errorf("统计解析模板数量失败: %w", err)
	}
	return count, nil
}

func nonNilIntMap(v map[string]int) map[string]int {
	if v == nil {
		return map[string]int{}
	}
	return v
}

func nonNilStringMap(v map[string]string) map[string]string {
	if v == nil {
		return map[string]string{}
	}
	return v
}
