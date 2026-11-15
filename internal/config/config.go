package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"

	"glog/pkg/models"
)

const AppVersion = "1.0.0"

// Manager 管理全局配置与数据目录。
type Manager struct {
	mu         sync.RWMutex
	cfg        models.AppConfig
	dataDir    string
	configPath string
}

// DefaultConfig 返回默认配置。
func DefaultConfig() models.AppConfig {
	return models.AppConfig{
		General: models.GeneralConfig{
			Version:  AppVersion,
			DataPath: "./data",
			TempPath: "./temp",
			LogPath:  "./logs",
		},
		Performance: models.PerformanceConfig{
			MaxGoroutines:        100,
			ParserWorkers:        8,
			IndexerWorkers:       4,
			AITaskWorkers:        3,
			MaxMemoryMB:          2048,
			CacheSizeMB:          512,
			LRUCacheItems:        1000,
			QueryTimeoutSeconds:  30,
			QueryCacheTTLMinutes: 10,
		},
		Collector: models.CollectorConfig{
			SSHTimeoutSeconds: 30,
			SSHConcurrency:    8,
			SSHCompression:    "gzip",
			LocalBufferSizeKB: 64,
			LocalBatchSize:    10000,
		},
		Indexer: models.IndexerConfig{
			IndexFlushIntervalSeconds: 10,
			IndexBatchSize:            1000,
			CompressionAlgorithm:      "lz4",
			BloomFilterSize:           1_000_000,
			BloomFilterHashFunctions:  3,
		},
		Parser: models.ParserConfig{
			MultiLineTimeoutMs: 5000,
			MultiLineMaxLines:  500,
			DefaultTimeFormat:  "2006-01-02T15:04:05Z07:00",
		},
		AI: models.AIConfigSection{
			DefaultProvider:       "ollama",
			DefaultSampleSize:     40,
			MaxSampleSize:         100,
			RequestTimeoutSeconds: 60,
			MaxLogsPerRequest:     200,
		},
		UI: models.UIConfig{
			Theme:    "dark",
			Language: "zh-CN",
			PageSize: 100,
			Overscan: 10,
		},
	}
}

// ResolveDataDir 解析数据目录：优先程序目录旁 data/，不可写则回退用户目录。
func ResolveDataDir() (string, error) {
	if env := os.Getenv("GLOG_DATA_DIR"); env != "" {
		if err := ensureWritable(env); err != nil {
			return "", err
		}
		abs, _ := filepath.Abs(env)
		return abs, nil
	}

	exe, err := os.Executable()
	if err == nil {
		exe, _ = filepath.Abs(exe)
		lowerExe := strings.ToLower(exe)
		// wails dev 使用临时目录构建；开发模式优先当前工作目录，避免数据随临时目录丢失。
		if strings.Contains(lowerExe, "go-build") || strings.Contains(lowerExe, "\\temp\\") || strings.Contains(lowerExe, "/tmp/") {
			if cwd, cwdErr := os.Getwd(); cwdErr == nil {
				devDir := filepath.Join(cwd, "data")
				if ensureWritable(devDir) == nil {
					return devDir, nil
				}
			}
		}
		dir := filepath.Join(filepath.Dir(exe), "data")
		if ensureWritable(dir) == nil {
			return dir, nil
		}
	}

	// 回退到用户配置目录
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("无法确定数据目录: %w", err)
	}
	dir := filepath.Join(base, "GLog")
	if err := ensureWritable(dir); err != nil {
		return "", err
	}
	return dir, nil
}

func ensureWritable(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".write-test-*")
	if err != nil {
		return err
	}
	name := f.Name()
	_ = f.Close()
	return os.Remove(name)
}

// Load 在默认数据目录加载配置。
func Load() (*Manager, error) {
	dir, err := ResolveDataDir()
	if err != nil {
		return nil, err
	}
	return LoadAt(dir)
}

// LoadAt 在指定数据目录加载（便于测试）。
func LoadAt(dataDir string) (*Manager, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	m := &Manager{
		cfg:        DefaultConfig(),
		dataDir:    dataDir,
		configPath: filepath.Join(dataDir, "config.toml"),
	}

	data, err := os.ReadFile(m.configPath)
	switch {
	case err == nil:
		cfg := DefaultConfig()
		if err := toml.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("解析配置文件失败: %w", err)
		}
		// 补齐关键默认值
		applyFallbacks(&cfg)
		m.cfg = cfg
	case errors.Is(err, os.ErrNotExist):
		applyFallbacks(&m.cfg)
		if err := m.Save(); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	return m, nil
}

func applyFallbacks(cfg *models.AppConfig) {
	def := DefaultConfig()
	if cfg.General.Version == "" {
		cfg.General.Version = def.General.Version
	}
	if cfg.Performance.ParserWorkers <= 0 {
		cfg.Performance.ParserWorkers = def.Performance.ParserWorkers
	}
	if cfg.Performance.IndexerWorkers <= 0 {
		cfg.Performance.IndexerWorkers = def.Performance.IndexerWorkers
	}
	if cfg.Performance.QueryTimeoutSeconds <= 0 {
		cfg.Performance.QueryTimeoutSeconds = def.Performance.QueryTimeoutSeconds
	}
	if cfg.Collector.SSHConcurrency <= 0 {
		cfg.Collector.SSHConcurrency = def.Collector.SSHConcurrency
	}
	if cfg.Collector.LocalBufferSizeKB <= 0 {
		cfg.Collector.LocalBufferSizeKB = def.Collector.LocalBufferSizeKB
	}
	if cfg.Indexer.IndexBatchSize <= 0 {
		cfg.Indexer.IndexBatchSize = def.Indexer.IndexBatchSize
	}
	if cfg.Parser.MultiLineMaxLines <= 0 {
		cfg.Parser.MultiLineMaxLines = def.Parser.MultiLineMaxLines
	}
	if cfg.AI.DefaultSampleSize <= 0 {
		cfg.AI.DefaultSampleSize = def.AI.DefaultSampleSize
	}
	if cfg.AI.MaxLogsPerRequest <= 0 {
		cfg.AI.MaxLogsPerRequest = def.AI.MaxLogsPerRequest
	}
	if cfg.UI.PageSize <= 0 {
		cfg.UI.PageSize = def.UI.PageSize
	}
	if cfg.UI.Overscan <= 0 {
		cfg.UI.Overscan = def.UI.Overscan
	}
}

// Config 返回配置副本。
func (m *Manager) Config() models.AppConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg
}

// Update 更新并持久化配置。
func (m *Manager) Update(cfg models.AppConfig) error {
	applyFallbacks(&cfg)
	m.mu.Lock()
	m.cfg = cfg
	m.mu.Unlock()
	return m.Save()
}

// Save 写入 config.toml。
func (m *Manager) Save() error {
	m.mu.RLock()
	cfg := m.cfg
	m.mu.RUnlock()

	f, err := os.CreateTemp(filepath.Dir(m.configPath), ".config-*.toml")
	if err != nil {
		return err
	}
	tmp := f.Name()
	enc := toml.NewEncoder(f)
	err = enc.Encode(cfg)
	closeErr := f.Close()
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	if err := os.Rename(tmp, m.configPath); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// DataDir 数据根目录。
func (m *Manager) DataDir() string { return m.dataDir }

// ConfigPath 配置文件路径。
func (m *Manager) ConfigPath() string { return m.configPath }

// ProjectsDir 项目根目录。
func (m *Manager) ProjectsDir() string { return filepath.Join(m.dataDir, "projects") }

// TempDir 临时目录。
func (m *Manager) TempDir() string { return filepath.Join(m.dataDir, "temp") }

// LogDir 日志目录。
func (m *Manager) LogDir() string { return filepath.Join(m.dataDir, "logs") }

// DBPath 元数据库路径。
func (m *Manager) DBPath() string { return filepath.Join(m.dataDir, "metadata.db") }

// EnsureDirs 创建必要目录。
func (m *Manager) EnsureDirs() error {
	for _, dir := range []string{m.ProjectsDir(), m.TempDir(), m.LogDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}
