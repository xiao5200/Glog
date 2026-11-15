// Package ai 提供 AI 提供商管理、OpenAI 兼容协议适配、稳健 JSON 解析，
// 以及日志分析任务的提示词构建与响应解析。
//
// 所有 AI 输出均视为不可信数据：调用方应先使用 Parse* 系列函数完成校验，
// 再写入存储或展示给用户。
package ai

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"glog/internal/errs"
)

const (
	defaultRequestTimeout = 60 * time.Second
	// 推理型模型（如 deepseek-flash）会先消耗 reasoning tokens，
	// 2048 可能在生成正文前耗尽预算，导致空内容；默认提高到 8192。
	defaultMaxTokens   = 8192
	defaultTemperature = 0.2
)

// Config AI 提供商配置。
type Config struct {
	Provider string // openai/deepseek/ollama/custom（也接受任意 OpenAI 兼容名称）
	Endpoint string
	Model    string
	APIKey   string
	Enabled  bool
	Priority int // 数字越大越优先
	Timeout  time.Duration
	// Parameters 可选参数：
	//   max_tokens  覆盖默认单次请求最大 token 数
	//   temperature 覆盖默认温度
	Parameters map[string]string
}

// Request 单次补全请求。
type Request struct {
	System      string
	Prompt      string
	MaxTokens   int
	Temperature float64
	JSONMode    bool
}

// Provider AI 提供商。
type Provider interface {
	Name() string
	Complete(ctx context.Context, req Request) (string, error)
	Test(ctx context.Context) error
}

type providerEntry struct {
	cfg      Config
	provider Provider
}

// Manager 管理多个 AI 提供商配置，支持并发访问与热更新。
type Manager struct {
	mu      sync.RWMutex
	entries map[string]*providerEntry
	order   []string // 按 Priority 降序、名称升序排列
}

// NewManager 创建管理器。configs 可以为空，之后通过 SetConfigs 热更新。
func NewManager(configs []Config) *Manager {
	m := &Manager{}
	m.SetConfigs(configs)
	return m
}

// SetConfigs 替换全部配置。重复的 Provider 以最后一条为准。
func (m *Manager) SetConfigs(configs []Config) {
	entries := make(map[string]*providerEntry, len(configs))
	for _, raw := range configs {
		cfg := normalizeConfig(raw)
		if cfg.Provider == "" {
			continue
		}
		entries[cfg.Provider] = &providerEntry{
			cfg:      cfg,
			provider: newOpenAIProvider(cfg),
		}
	}

	order := make([]string, 0, len(entries))
	for name := range entries {
		order = append(order, name)
	}
	sort.Slice(order, func(i, j int) bool {
		left := entries[order[i]].cfg
		right := entries[order[j]].cfg
		if left.Priority != right.Priority {
			return left.Priority > right.Priority
		}
		return order[i] < order[j]
	})

	m.mu.Lock()
	m.entries = entries
	m.order = order
	m.mu.Unlock()
}

// Providers 返回所有已保存的提供商名称（包含未启用的配置），
// 按 Priority 降序、名称升序排列。
func (m *Manager) Providers() []string {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, len(m.order))
	copy(out, m.order)
	return out
}

// HasConfigured 是否至少有一个已启用且配置完整的提供商。
func (m *Manager) HasConfigured() bool {
	if m == nil {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, entry := range m.entries {
		if entry.cfg.Enabled && configIssue(entry.cfg) == "" {
			return true
		}
	}
	return false
}

// SelectedProvider 选择实际使用的提供商。
// explicit 非空时优先使用该提供商（仍需存在、已启用且配置完整）；
// 否则按 Priority 从高到低选择已启用的提供商。
func (m *Manager) SelectedProvider(explicit string) (string, error) {
	if m == nil {
		return "", fmt.Errorf("%w: AI 管理器未初始化", errs.ErrAIProviderNotConfigured)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	name, _, err := m.selectLocked(explicit)
	return name, err
}

// Complete 使用指定的提供商（provider 为空时自动选择）完成一次补全请求。
func (m *Manager) Complete(ctx context.Context, provider, system, prompt string, jsonMode bool) (string, error) {
	if m == nil {
		return "", fmt.Errorf("%w: AI 管理器未初始化", errs.ErrAIProviderNotConfigured)
	}

	m.mu.RLock()
	_, entry, err := m.selectLocked(provider)
	if err != nil {
		m.mu.RUnlock()
		return "", err
	}
	cfg := entry.cfg
	target := entry.provider
	m.mu.RUnlock()

	req := Request{
		System:      system,
		Prompt:      prompt,
		MaxTokens:   intParameter(cfg.Parameters, "max_tokens", defaultMaxTokens),
		Temperature: floatParameter(cfg.Parameters, "temperature", defaultTemperature),
		JSONMode:    jsonMode,
	}
	return target.Complete(ctx, req)
}

// Test 测试指定提供商的连通性（provider 为空时自动选择）。
func (m *Manager) Test(ctx context.Context, provider string) error {
	if m == nil {
		return fmt.Errorf("%w: AI 管理器未初始化", errs.ErrAIProviderNotConfigured)
	}

	m.mu.RLock()
	_, entry, err := m.selectLocked(provider)
	if err != nil {
		m.mu.RUnlock()
		return err
	}
	target := entry.provider
	m.mu.RUnlock()

	return target.Test(ctx)
}

// selectLocked 在已持有读锁的情况下选择提供商。
func (m *Manager) selectLocked(explicit string) (string, *providerEntry, error) {
	explicit = strings.ToLower(strings.TrimSpace(explicit))
	if explicit != "" {
		entry, ok := m.entries[explicit]
		if !ok {
			return "", nil, fmt.Errorf("%w: 未找到提供商 %q", errs.ErrAIProviderNotConfigured, explicit)
		}
		if !entry.cfg.Enabled {
			return "", nil, fmt.Errorf("%w: 提供商 %q 未启用", errs.ErrAIProviderNotConfigured, explicit)
		}
		if issue := configIssue(entry.cfg); issue != "" {
			return "", nil, fmt.Errorf("%w: 提供商 %q 配置不完整：%s", errs.ErrAIProviderNotConfigured, explicit, issue)
		}
		return entry.cfg.Provider, entry, nil
	}

	for _, name := range m.order {
		entry := m.entries[name]
		if entry == nil || !entry.cfg.Enabled {
			continue
		}
		if configIssue(entry.cfg) != "" {
			continue
		}
		return entry.cfg.Provider, entry, nil
	}
	return "", nil, fmt.Errorf("%w: 请先在设置中启用并配置至少一个 AI 提供商", errs.ErrAIProviderNotConfigured)
}

func normalizeConfig(cfg Config) Config {
	cfg.Provider = strings.ToLower(strings.TrimSpace(cfg.Provider))
	cfg.Endpoint = strings.TrimSpace(cfg.Endpoint)
	cfg.Model = strings.TrimSpace(cfg.Model)
	cfg.APIKey = strings.TrimSpace(cfg.APIKey)

	params := make(map[string]string, len(cfg.Parameters))
	for key, value := range cfg.Parameters {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		params[key] = value
	}
	cfg.Parameters = params
	return cfg
}

// configIssue 返回配置缺陷；空字符串表示配置完整。
// 注意：API Key 不是必填项（本地 Ollama/自定义网关可能不需要）。
func configIssue(cfg Config) string {
	if cfg.Provider == "" {
		return "提供商名称为空"
	}
	if _, err := resolveEndpoint(cfg); err != nil {
		return err.Error()
	}
	if cfg.Model == "" {
		return "模型名称未配置"
	}
	return ""
}

func intParameter(params map[string]string, key string, fallback int) int {
	raw, ok := params[key]
	if !ok {
		return fallback
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func floatParameter(params map[string]string, key string, fallback float64) float64 {
	raw, ok := params[key]
	if !ok {
		return fallback
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return fallback
	}
	return value
}
