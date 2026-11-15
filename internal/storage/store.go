// Package storage 提供 GLog 的 SQLite 元数据存储。
//
// 数据库使用 modernc.org/sqlite（纯 Go 实现），启用 WAL、busy_timeout 与
// 外键约束。所有 SQL 均使用参数绑定，时间统一以 UTC RFC3339Nano 文本保存。
package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite" // 注册 "sqlite" 驱动

	"glog/internal/errs"
)

// schemaVersion 当前数据库结构版本，写入 PRAGMA user_version。
const schemaVersion = 1

const (
	defaultBusyTimeoutMs = 5000
	dbMaxOpenConns       = 10
	dbMaxIdleConns       = 5
)

// MetadataStore 封装 SQLite 元数据库。
type MetadataStore struct {
	db *sql.DB
}

// schemaStatements 为幂等建表/建索引脚本。
var schemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS projects (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'active',
		project_path TEXT NOT NULL DEFAULT '',
		log_count INTEGER NOT NULL DEFAULT 0,
		size_bytes INTEGER NOT NULL DEFAULT 0,
		time_range_start TEXT,
		time_range_end TEXT,
		tags TEXT,
		priority TEXT NOT NULL DEFAULT 'medium',
		level_distribution TEXT
	)`,
	`CREATE INDEX IF NOT EXISTS idx_projects_status ON projects(status)`,
	`CREATE INDEX IF NOT EXISTS idx_projects_created_at ON projects(created_at DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_projects_updated_at ON projects(updated_at DESC)`,

	`CREATE TABLE IF NOT EXISTS log_sources (
		id TEXT PRIMARY KEY,
		project_id TEXT NOT NULL,
		type TEXT NOT NULL DEFAULT 'local',
		host TEXT,
		path TEXT NOT NULL DEFAULT '',
		parse_mode TEXT NOT NULL DEFAULT 'builtin',
		template_id TEXT,
		log_count INTEGER NOT NULL DEFAULT 0,
		size_bytes INTEGER NOT NULL DEFAULT 0,
		time_range_start TEXT,
		time_range_end TEXT,
		created_at TEXT NOT NULL,
		ai_provider TEXT,
		metadata TEXT,
		FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
	)`,
	`CREATE INDEX IF NOT EXISTS idx_log_sources_project ON log_sources(project_id)`,
	`CREATE INDEX IF NOT EXISTS idx_log_sources_type ON log_sources(type)`,

	`CREATE TABLE IF NOT EXISTS parse_templates (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT,
		type TEXT NOT NULL DEFAULT 'custom',
		category TEXT,
		format TEXT NOT NULL DEFAULT 'regex',
		priority INTEGER NOT NULL DEFAULT 0,
		regex TEXT NOT NULL DEFAULT '',
		field_map TEXT NOT NULL DEFAULT '{}',
		time_format TEXT,
		time_field TEXT,
		level_map TEXT,
		level_field TEXT,
		level_extraction TEXT,
		multiline_config TEXT,
		examples TEXT,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		usage_count INTEGER NOT NULL DEFAULT 0,
		source TEXT NOT NULL DEFAULT 'user'
	)`,
	`CREATE INDEX IF NOT EXISTS idx_parse_templates_type ON parse_templates(type)`,
	`CREATE INDEX IF NOT EXISTS idx_parse_templates_category ON parse_templates(category)`,
	`CREATE INDEX IF NOT EXISTS idx_parse_templates_usage ON parse_templates(usage_count DESC)`,

	`CREATE TABLE IF NOT EXISTS ssh_hosts (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		host TEXT NOT NULL,
		port INTEGER NOT NULL DEFAULT 22,
		username TEXT NOT NULL,
		auth_method TEXT NOT NULL DEFAULT 'password',
		password_encrypted TEXT,
		key_path TEXT,
		proxy_jump TEXT,
		last_used_at TEXT,
		created_at TEXT NOT NULL,
		metadata TEXT
	)`,
	`CREATE INDEX IF NOT EXISTS idx_ssh_hosts_last_used ON ssh_hosts(last_used_at DESC)`,

	`CREATE TABLE IF NOT EXISTS investigations (
		id TEXT PRIMARY KEY,
		project_id TEXT NOT NULL,
		title TEXT NOT NULL,
		description TEXT,
		clue_type TEXT NOT NULL DEFAULT '',
		clue_value TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'open',
		related_log_count INTEGER NOT NULL DEFAULT 0,
		time_range_start TEXT,
		time_range_end TEXT,
		ai_analysis TEXT,
		evidence_ids TEXT,
		tags TEXT,
		risk_score REAL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		closed_at TEXT,
		assigned_to TEXT,
		FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
	)`,
	`CREATE INDEX IF NOT EXISTS idx_investigations_project ON investigations(project_id)`,
	`CREATE INDEX IF NOT EXISTS idx_investigations_status ON investigations(status)`,
	`CREATE INDEX IF NOT EXISTS idx_investigations_clue_type ON investigations(clue_type)`,
	`CREATE INDEX IF NOT EXISTS idx_investigations_created_at ON investigations(created_at DESC)`,

	`CREATE TABLE IF NOT EXISTS query_snapshots (
		id TEXT PRIMARY KEY,
		project_id TEXT NOT NULL,
		name TEXT NOT NULL,
		description TEXT,
		query_json TEXT NOT NULL,
		result_count INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL,
		FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
	)`,
	`CREATE INDEX IF NOT EXISTS idx_query_snapshots_project ON query_snapshots(project_id)`,
	`CREATE INDEX IF NOT EXISTS idx_query_snapshots_created_at ON query_snapshots(created_at DESC)`,

	`CREATE TABLE IF NOT EXISTS ai_configs (
		provider TEXT PRIMARY KEY,
		api_key TEXT,
		endpoint TEXT,
		model TEXT,
		enabled INTEGER NOT NULL DEFAULT 0,
		priority INTEGER NOT NULL DEFAULT 0,
		parameters TEXT,
		updated_at TEXT NOT NULL
	)`,

	`CREATE TABLE IF NOT EXISTS detection_rules (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT,
		rule_file_path TEXT NOT NULL DEFAULT '',
		enabled INTEGER NOT NULL DEFAULT 1,
		severity TEXT NOT NULL DEFAULT 'medium',
		tags TEXT,
		mitre_tactics TEXT,
		mitre_techniques TEXT,
		match_count INTEGER NOT NULL DEFAULT 0,
		last_matched_at TEXT,
		builtin INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_detection_rules_enabled ON detection_rules(enabled)`,
	`CREATE INDEX IF NOT EXISTS idx_detection_rules_severity ON detection_rules(severity)`,

	`CREATE TABLE IF NOT EXISTS ioc_indicators (
		id TEXT PRIMARY KEY,
		ioc_type TEXT NOT NULL,
		ioc_value TEXT NOT NULL,
		threat_level TEXT NOT NULL DEFAULT 'medium',
		description TEXT,
		source TEXT,
		created_at TEXT NOT NULL,
		metadata TEXT
	)`,
	`CREATE INDEX IF NOT EXISTS idx_ioc_type ON ioc_indicators(ioc_type)`,
	`CREATE INDEX IF NOT EXISTS idx_ioc_value ON ioc_indicators(ioc_value)`,
	`CREATE INDEX IF NOT EXISTS idx_ioc_threat_level ON ioc_indicators(threat_level)`,

	`CREATE TABLE IF NOT EXISTS evidence_marks (
		id TEXT PRIMARY KEY,
		investigation_id TEXT NOT NULL,
		doc_id INTEGER NOT NULL DEFAULT 0,
		log_entry_id TEXT NOT NULL,
		marked_at TEXT NOT NULL,
		note TEXT,
		UNIQUE (investigation_id, log_entry_id),
		FOREIGN KEY (investigation_id) REFERENCES investigations(id) ON DELETE CASCADE
	)`,
	`CREATE INDEX IF NOT EXISTS idx_evidence_investigation ON evidence_marks(investigation_id)`,
	`CREATE INDEX IF NOT EXISTS idx_evidence_log_entry ON evidence_marks(log_entry_id)`,
	`CREATE INDEX IF NOT EXISTS idx_evidence_doc ON evidence_marks(investigation_id, doc_id)`,

	`CREATE TABLE IF NOT EXISTS settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	)`,
}

// Open 打开（必要时创建）元数据库并完成初始化。
func Open(dbPath string) (*MetadataStore, error) {
	if strings.TrimSpace(dbPath) == "" {
		return nil, fmt.Errorf("数据库路径不能为空: %w", errs.ErrInvalidInput)
	}

	inMemory := dbPath == ":memory:" || strings.Contains(dbPath, ":memory:")
	if !inMemory && !strings.HasPrefix(dbPath, "file:") {
		dir := filepath.Dir(dbPath)
		if dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, fmt.Errorf("创建数据库目录失败: %w", err)
			}
		}
	}

	dsn := buildDSN(dbPath, defaultBusyTimeoutMs)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}

	if inMemory {
		// 普通 :memory: 数据库在不同连接间不共享，限制为单连接。
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
	} else {
		db.SetMaxOpenConns(dbMaxOpenConns)
		db.SetMaxIdleConns(dbMaxIdleConns)
	}

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("连接数据库失败: %w", err)
	}

	// 再次显式设置当前连接的 PRAGMA，DSN 负责新连接。
	if _, err := db.Exec("PRAGMA busy_timeout = " + fmt.Sprintf("%d", defaultBusyTimeoutMs)); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("设置 busy_timeout 失败: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("启用外键约束失败: %w", err)
	}

	var journalMode string
	if err := db.QueryRow("PRAGMA journal_mode = WAL").Scan(&journalMode); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("启用 WAL 模式失败: %w", err)
	}
	if !inMemory && !strings.EqualFold(journalMode, "wal") {
		_ = db.Close()
		return nil, fmt.Errorf("无法启用 WAL 模式（当前为 %q），请检查数据库所在磁盘是否支持", journalMode)
	}

	store := &MetadataStore{db: db}
	if err := store.initSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

// buildDSN 组装 modernc.org/sqlite 的连接参数。
// 每个新连接都会自动应用这些 PRAGMA，避免连接池中的新连接丢失设置。
func buildDSN(dbPath string, busyTimeoutMs int) string {
	sep := "?"
	if strings.Contains(dbPath, "?") {
		sep = "&"
	}
	return dbPath + sep +
		"_journal_mode=WAL" +
		"&_busy_timeout=" + fmt.Sprintf("%d", busyTimeoutMs) +
		"&_foreign_keys=1"
}

func (s *MetadataStore) initSchema() error {
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("读取数据库版本失败: %w", err)
	}
	if version > schemaVersion {
		return fmt.Errorf("数据库结构版本过新（当前 %d，程序支持 %d），请升级程序", version, schemaVersion)
	}

	for i, stmt := range schemaStatements {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("初始化数据库结构失败（第 %d 条语句）: %w", i+1, err)
		}
	}

	if version != schemaVersion {
		if _, err := s.db.Exec(fmt.Sprintf("PRAGMA user_version = %d", schemaVersion)); err != nil {
			return fmt.Errorf("更新数据库版本失败: %w", err)
		}
	}
	return nil
}

// Close 关闭数据库。
func (s *MetadataStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// DB 返回底层 *sql.DB，供需要事务编排的调用方使用。
func (s *MetadataStore) DB() *sql.DB {
	if s == nil {
		return nil
	}
	return s.db
}

// GetSetting 读取键值设置。
func (s *MetadataStore) GetSetting(key string) (string, error) {
	if strings.TrimSpace(key) == "" {
		return "", invalidInput("设置键不能为空")
	}
	var value string
	err := s.db.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("设置项不存在（key=%s）: %w", key, errs.ErrNotFound)
	}
	if err != nil {
		return "", fmt.Errorf("读取设置项失败: %w", err)
	}
	return value, nil
}

// SetSetting 写入（插入或更新）键值设置。
func (s *MetadataStore) SetSetting(key, value string) error {
	if strings.TrimSpace(key) == "" {
		return invalidInput("设置键不能为空")
	}
	_, err := s.db.Exec(
		`INSERT INTO settings (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value,
	)
	if err != nil {
		return fmt.Errorf("保存设置项失败: %w", err)
	}
	return nil
}
