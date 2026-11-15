package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"glog/internal/errs"
)

// AIConfigRecord AI 提供商配置记录。APIKeyEncrypted 为密文。
// Parameters 保持调用方传入的原始字符串（不在此层解析 JSON）。
type AIConfigRecord struct {
	Provider        string    `json:"provider"`
	APIKeyEncrypted string    `json:"api_key_encrypted,omitempty"`
	Endpoint        string    `json:"endpoint"`
	Model           string    `json:"model"`
	Enabled         bool      `json:"enabled"`
	Priority        int       `json:"priority"`
	Parameters      string    `json:"parameters"`
	UpdatedAt       time.Time `json:"updated_at"`
}

const aiConfigSelectColumns = `provider, COALESCE(api_key, ''), COALESCE(endpoint, ''),
	COALESCE(model, ''), COALESCE(enabled, 0), COALESCE(priority, 0),
	COALESCE(parameters, ''), updated_at`

func scanAIConfig(row rowScanner) (*AIConfigRecord, error) {
	var (
		rec        AIConfigRecord
		enabled    int
		updatedRaw string
	)
	err := row.Scan(
		&rec.Provider, &rec.APIKeyEncrypted, &rec.Endpoint, &rec.Model,
		&enabled, &rec.Priority, &rec.Parameters, &updatedRaw,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("AI配置不存在: %w", errs.ErrNotFound)
		}
		return nil, fmt.Errorf("读取AI配置失败: %w", err)
	}
	rec.Enabled = enabled != 0
	if rec.UpdatedAt, err = scanTime(updatedRaw); err != nil {
		return nil, fmt.Errorf("解析AI配置更新时间失败: %w", err)
	}
	return &rec, nil
}

// SaveAIConfigRecord 保存 AI 配置（按 provider 插入或更新）。
// 更新时 APIKeyEncrypted 为空表示保留原密钥。
func (s *MetadataStore) SaveAIConfigRecord(rec *AIConfigRecord) error {
	if rec == nil {
		return invalidInput("AI配置不能为空")
	}
	provider := strings.TrimSpace(rec.Provider)
	if provider == "" {
		return invalidInput("AI提供商不能为空")
	}
	updatedAt := rec.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = nowUTC()
	}
	enabled := 0
	if rec.Enabled {
		enabled = 1
	}

	_, err := s.db.Exec(
		`INSERT INTO ai_configs
			(provider, api_key, endpoint, model, enabled, priority, parameters, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(provider) DO UPDATE SET
			api_key = CASE
				WHEN excluded.api_key IS NULL OR excluded.api_key = ''
				THEN ai_configs.api_key
				ELSE excluded.api_key
			END,
			endpoint = excluded.endpoint,
			model = excluded.model,
			enabled = excluded.enabled,
			priority = excluded.priority,
			parameters = excluded.parameters,
			updated_at = excluded.updated_at`,
		provider, nullIfEmpty(rec.APIKeyEncrypted), rec.Endpoint, rec.Model,
		enabled, rec.Priority, rec.Parameters, formatTime(updatedAt),
	)
	if err != nil {
		return fmt.Errorf("保存AI配置失败: %w", err)
	}
	rec.Provider = provider
	rec.UpdatedAt = updatedAt
	return nil
}

// GetAIConfigRecord 按提供商读取 AI 配置。
func (s *MetadataStore) GetAIConfigRecord(provider string) (*AIConfigRecord, error) {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return nil, invalidInput("AI提供商不能为空")
	}
	row := s.db.QueryRow("SELECT "+aiConfigSelectColumns+" FROM ai_configs WHERE provider = ?", provider)
	return scanAIConfig(row)
}

// ListAIConfigRecords 列出全部 AI 配置（按优先级倒序）。
func (s *MetadataStore) ListAIConfigRecords() ([]AIConfigRecord, error) {
	rows, err := s.db.Query(
		"SELECT " + aiConfigSelectColumns + ` FROM ai_configs
		 ORDER BY priority DESC, provider COLLATE NOCASE ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("查询AI配置列表失败: %w", err)
	}
	defer rows.Close()

	out := make([]AIConfigRecord, 0, 8)
	for rows.Next() {
		rec, err := scanAIConfig(rows)
		if err != nil {
			return nil, fmt.Errorf("读取AI配置列表失败: %w", err)
		}
		out = append(out, *rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历AI配置列表失败: %w", err)
	}
	return out, nil
}
