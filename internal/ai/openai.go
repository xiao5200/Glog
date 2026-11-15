package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"glog/internal/errs"
)

const (
	// 非 2xx 响应最多读取 4KB，避免巨量错误页/敏感内容进入内存与错误信息。
	maxErrorBodyBytes = 4 << 10
	// 正常响应体上限，防止异常提供商返回超大内容耗尽内存。
	maxResponseBodyBytes = 8 << 20
	// 模型列表接口的请求超时。
	modelsRequestTimeout = 30 * time.Second
)

// TestConfig 使用给定配置测试 OpenAI 兼容服务的连通性。
// Provider 为空时按 custom 处理（仍需提供 Endpoint 与 Model）。
func TestConfig(ctx context.Context, cfg Config) error {
	cfg = normalizeConfig(cfg)
	if cfg.Provider == "" {
		cfg.Provider = "custom"
	}
	return newOpenAIProvider(cfg).Test(ctx)
}

// ListModels 查询 OpenAI 兼容服务的可用模型列表。
// Endpoint 支持基础地址、/chat/completions 完整地址或已带 /models 的地址。
func ListModels(ctx context.Context, cfg Config) ([]string, error) {
	cfg = normalizeConfig(cfg)
	if cfg.Provider == "" {
		cfg.Provider = "custom"
	}
	endpoint, err := resolveModelsEndpoint(cfg)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", errs.ErrAIProviderNotConfigured, redactSecrets(err.Error(), cfg.APIKey))
	}
	if ctx == nil {
		ctx = context.Background()
	}
	requestCtx, cancel := context.WithTimeout(ctx, modelsRequestTimeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: 提供商 %s 的模型列表 Endpoint 无效（%s）",
			errs.ErrAIRequestFailed, cfg.Provider, redactSecrets(displayURL(endpoint), cfg.APIKey))
	}
	httpReq.Header.Set("Accept", "application/json")
	if cfg.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}

	resp, err := (&http.Client{}).Do(httpReq)
	if err != nil {
		if ctxErr := requestCtx.Err(); ctxErr != nil {
			if errors.Is(ctxErr, context.DeadlineExceeded) {
				return nil, fmt.Errorf("%w: 获取提供商 %s 的模型列表超时（超过 %s）: %w",
					errs.ErrAIRequestFailed, cfg.Provider, modelsRequestTimeout, ctxErr)
			}
			return nil, fmt.Errorf("%w: 获取提供商 %s 的模型列表已取消: %w", errs.ErrAIRequestFailed, cfg.Provider, ctxErr)
		}
		return nil, fmt.Errorf("%w: 连接提供商 %s 失败（%s）：%s",
			errs.ErrAIRequestFailed, cfg.Provider, redactSecrets(displayURL(endpoint), cfg.APIKey),
			redactSecrets(sanitizeNetworkError(err), cfg.APIKey))
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
		return nil, modelsStatusError(cfg, resp, body)
	}

	body, err := readLimitedBody(resp.Body, maxResponseBodyBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: 读取提供商 %s 的模型列表响应失败：%s",
			errs.ErrAIInvalidResponse, cfg.Provider, redactSecrets(err.Error(), cfg.APIKey))
	}
	names, err := parseModelsResponse(body)
	if err != nil {
		return nil, fmt.Errorf("%w: 提供商 %s 返回的模型列表无法解析：%v",
			errs.ErrAIInvalidResponse, cfg.Provider, err)
	}
	return names, nil
}

func modelsStatusError(cfg Config, resp *http.Response, body []byte) error {
	detail := extractProviderErrorMessage(body)
	if detail == "" {
		detail = string(body)
	}
	detail = truncateRunes(redactSecrets(detail, cfg.APIKey), 300)

	message := fmt.Sprintf("提供商 %s 获取模型列表失败（HTTP %s）",
		cfg.Provider, redactSecrets(sanitizeInline(resp.Status), cfg.APIKey))
	if detail != "" {
		message += "：" + detail
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		message += "（请检查 API Key 或访问权限）"
	case http.StatusNotFound:
		message += "（Endpoint 可能不支持 /models 接口，请确认填写的是 OpenAI 兼容地址，例如 https://api.openai.com/v1）"
	case http.StatusTooManyRequests:
		message += "（请求过于频繁或配额不足）"
	}
	return fmt.Errorf("%w: %s", errs.ErrAIRequestFailed, message)
}

func parseModelsResponse(body []byte) ([]string, error) {
	type modelItem struct {
		ID string `json:"id"`
	}

	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("响应内容为空")
	}

	var candidates []string
	switch trimmed[0] {
	case '{':
		var payload struct {
			Data []modelItem `json:"data"`
		}
		if err := json.Unmarshal(trimmed, &payload); err != nil {
			return nil, fmt.Errorf("响应不是有效的 JSON 对象")
		}
		for _, item := range payload.Data {
			candidates = append(candidates, item.ID)
		}
	case '[':
		var items []modelItem
		if err := json.Unmarshal(trimmed, &items); err == nil {
			for _, item := range items {
				candidates = append(candidates, item.ID)
			}
		} else {
			var names []string
			if err := json.Unmarshal(trimmed, &names); err != nil {
				return nil, fmt.Errorf("期望包含 data[].id 的 JSON 对象或模型 ID 数组")
			}
			candidates = append(candidates, names...)
		}
	default:
		return nil, fmt.Errorf("响应不是 JSON 对象或数组")
	}

	seen := make(map[string]struct{}, len(candidates))
	names := make([]string, 0, len(candidates))
	for _, name := range candidates {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// resolveModelsEndpoint 解析并规范化 OpenAI 兼容的 /models 地址。
func resolveModelsEndpoint(cfg Config) (string, error) {
	endpoint := strings.TrimSpace(cfg.Endpoint)
	if endpoint == "" {
		endpoint = defaultEndpoints[strings.ToLower(strings.TrimSpace(cfg.Provider))]
	}
	if endpoint == "" {
		return "", fmt.Errorf("提供商 %q 必须提供 Endpoint", cfg.Provider)
	}

	endpoint = strings.TrimRight(endpoint, "/")
	lower := strings.ToLower(endpoint)
	if strings.HasSuffix(lower, "/models") {
		return endpoint, nil
	}
	if strings.HasSuffix(lower, "/chat/completions") {
		endpoint = strings.TrimRight(endpoint[:len(endpoint)-len("/chat/completions")], "/")
	}
	return endpoint + "/models", nil
}

var defaultEndpoints = map[string]string{
	"openai":   "https://api.openai.com/v1",
	"deepseek": "https://api.deepseek.com/v1",
	"ollama":   "http://localhost:11434/v1",
}

type openAIProvider struct {
	name        string
	cfg         Config
	endpoint    string // 已规范化为 /chat/completions 完整地址
	endpointErr error
	client      *http.Client
}

func newOpenAIProvider(cfg Config) Provider {
	endpoint, endpointErr := resolveEndpoint(cfg)
	return &openAIProvider{
		name:        cfg.Provider,
		cfg:         cfg,
		endpoint:    endpoint,
		endpointErr: endpointErr,
		client:      &http.Client{},
	}
}

func (p *openAIProvider) Name() string { return p.name }

func (p *openAIProvider) Complete(ctx context.Context, req Request) (string, error) {
	content, err := p.chat(ctx, req)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("%w: 提供商 %s 返回了空内容", errs.ErrAIInvalidResponse, p.name)
	}
	return content, nil
}

// Test 使用极短 prompt 验证连通性，不产生明显费用。
func (p *openAIProvider) Test(ctx context.Context) error {
	_, err := p.chat(ctx, Request{
		System:      "你是一个连通性测试助手。",
		Prompt:      "ping",
		MaxTokens:   8,
		Temperature: 0,
	})
	return err
}

func (p *openAIProvider) chat(ctx context.Context, req Request) (string, error) {
	if p.endpointErr != nil {
		return "", fmt.Errorf("%w: 提供商 %q 配置无效：%v", errs.ErrAIProviderNotConfigured, p.name, p.endpointErr)
	}
	if p.endpoint == "" {
		return "", fmt.Errorf("%w: 提供商 %q 未配置 Endpoint", errs.ErrAIProviderNotConfigured, p.name)
	}
	if strings.TrimSpace(p.cfg.Model) == "" {
		return "", fmt.Errorf("%w: 提供商 %q 未配置模型", errs.ErrAIProviderNotConfigured, p.name)
	}
	if ctx == nil {
		ctx = context.Background()
	}

	timeout := p.cfg.Timeout
	if timeout <= 0 {
		timeout = defaultRequestTimeout
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	payload, err := buildChatRequestBody(p.name, p.cfg.Model, req)
	if err != nil {
		return "", fmt.Errorf("%w: 构建 %s 请求失败：%v", errs.ErrAIRequestFailed, p.name, err)
	}

	httpReq, err := http.NewRequestWithContext(requestCtx, http.MethodPost, p.endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("%w: 提供商 %q 的 Endpoint 无效", errs.ErrAIRequestFailed, p.name)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	if p.cfg.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	}

	resp, err := p.httpClient().Do(httpReq)
	if err != nil {
		if ctxErr := requestCtx.Err(); ctxErr != nil {
			if errors.Is(ctxErr, context.DeadlineExceeded) {
				return "", fmt.Errorf("%w: 提供商 %s 请求超时（超过 %s，%s）: %w",
					errs.ErrAIRequestFailed, p.name, timeout, p.displayEndpoint(), ctxErr)
			}
			return "", fmt.Errorf("%w: 提供商 %s 请求已取消: %w", errs.ErrAIRequestFailed, p.name, ctxErr)
		}
		return "", fmt.Errorf("%w: 连接提供商 %s 失败（%s）：%s",
			errs.ErrAIRequestFailed, p.name, p.displayEndpoint(), p.redactText(sanitizeNetworkError(err)))
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
		return "", p.statusError(resp, body)
	}

	body, err := readLimitedBody(resp.Body, maxResponseBodyBytes)
	if err != nil {
		return "", fmt.Errorf("%w: 读取提供商 %s 响应失败：%s", errs.ErrAIInvalidResponse, p.name, p.redactText(err.Error()))
	}

	var result chatResponsePayload
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("%w: 提供商 %s 返回的响应不是有效 JSON：%v", errs.ErrAIInvalidResponse, p.name, err)
	}
	if detail := extractErrorRaw(result.Error); detail != "" {
		return "", fmt.Errorf("%w: 提供商 %s 返回错误：%s",
			errs.ErrAIRequestFailed, p.name, p.redactText(truncateRunes(sanitizeInline(detail), 300)))
	}
	return extractResponseContent(&result, p.name)
}

func (p *openAIProvider) httpClient() *http.Client {
	if p.client != nil {
		return p.client
	}
	return http.DefaultClient
}

func (p *openAIProvider) statusError(resp *http.Response, body []byte) error {
	detail := extractProviderErrorMessage(body)
	if detail == "" {
		detail = string(body)
	}
	detail = truncateRunes(p.redactText(sanitizeInline(detail)), 300)

	message := fmt.Sprintf("提供商 %s 返回 HTTP %s", p.name, p.redactText(sanitizeInline(resp.Status)))
	if detail != "" {
		message += "：" + detail
	}

	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("%w: %w: %s（请检查 API Key 或访问权限）",
			errs.ErrAIRequestFailed, errs.ErrPermissionDenied, message)
	case http.StatusTooManyRequests:
		return fmt.Errorf("%w: %w: %s（请求过于频繁或配额不足）",
			errs.ErrAIRequestFailed, errs.ErrAIQuotaExceeded, message)
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return fmt.Errorf("%w: %s（请求超时）", errs.ErrAIRequestFailed, message)
	default:
		return fmt.Errorf("%w: %s", errs.ErrAIRequestFailed, message)
	}
}

func (p *openAIProvider) displayEndpoint() string {
	if p.endpoint == "" {
		return "(Endpoint 未配置)"
	}
	return p.redactText(displayURL(p.endpoint))
}

func (p *openAIProvider) redactText(value string) string {
	return redactSecrets(value, p.cfg.APIKey)
}

// redactSecrets 清理错误文本中的 API Key、Bearer 令牌与常见凭据字段。
func redactSecrets(value, apiKey string) string {
	value = sanitizeInline(value)
	if apiKey != "" {
		value = strings.ReplaceAll(value, apiKey, "***")
	}
	value = bearerTokenPattern.ReplaceAllString(value, "Bearer ***")
	value = credentialValuePattern.ReplaceAllString(value, "$1=***")
	value = skTokenPattern.ReplaceAllString(value, "sk-***")
	return value
}

var (
	bearerTokenPattern     = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]+`)
	credentialValuePattern = regexp.MustCompile(`(?i)\b(api[_-]?key|apikey|access[_-]?token|auth[_-]?token|token|secret|password)\b\s*[:=]\s*["']?[^\s"',;}]+`)
	skTokenPattern         = regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{8,}\b`)
)

// resolveEndpoint 解析并规范化 Chat Completions 地址。
func resolveEndpoint(cfg Config) (string, error) {
	endpoint := strings.TrimSpace(cfg.Endpoint)
	if endpoint == "" {
		endpoint = defaultEndpoints[strings.ToLower(strings.TrimSpace(cfg.Provider))]
	}
	if endpoint == "" {
		return "", fmt.Errorf("提供商 %q 必须提供 Endpoint", cfg.Provider)
	}
	return normalizeChatEndpoint(endpoint), nil
}

func normalizeChatEndpoint(endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	endpoint = strings.TrimRight(endpoint, "/")
	if strings.HasSuffix(strings.ToLower(endpoint), "/chat/completions") {
		return endpoint
	}
	return endpoint + "/chat/completions"
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type responseFormat struct {
	Type string `json:"type"`
}

type chatRequestBody struct {
	Model          string          `json:"model"`
	Messages       []chatMessage   `json:"messages"`
	Temperature    float64         `json:"temperature"`
	MaxTokens      int             `json:"max_tokens"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}

func buildChatRequestBody(provider, model string, req Request) ([]byte, error) {
	messages := []chatMessage{
		{Role: "system", Content: req.System},
		{Role: "user", Content: req.Prompt},
	}

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}

	payload := chatRequestBody{
		Model:       model,
		Messages:    messages,
		Temperature: req.Temperature,
		MaxTokens:   maxTokens,
	}
	// Ollama 的 OpenAI 兼容接口对 response_format 支持不一致，按契约不下发。
	if req.JSONMode && !strings.EqualFold(provider, "ollama") {
		payload.ResponseFormat = &responseFormat{Type: "json_object"}
	}
	return json.Marshal(payload)
}

type chatResponseMessage struct {
	Content json.RawMessage `json:"content"`
}

type chatResponseChoice struct {
	Message      chatResponseMessage `json:"message"`
	Text         string              `json:"text"`
	FinishReason string              `json:"finish_reason"`
}

type chatResponsePayload struct {
	Choices  []chatResponseChoice `json:"choices"`
	Message  *chatResponseMessage `json:"message"`
	Response string               `json:"response"`
	Error    json.RawMessage      `json:"error"`
}

type contentPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func extractResponseContent(payload *chatResponsePayload, provider string) (string, error) {
	recognized := false
	candidates := make([]string, 0, 4)

	for _, choice := range payload.Choices {
		recognized = true
		if content := rawContentToString(choice.Message.Content); content != "" {
			candidates = append(candidates, content)
		}
		if choice.Text != "" {
			candidates = append(candidates, choice.Text)
		}
	}
	if payload.Message != nil {
		recognized = true
		if content := rawContentToString(payload.Message.Content); content != "" {
			candidates = append(candidates, content)
		}
	}
	if strings.TrimSpace(payload.Response) != "" {
		recognized = true
		candidates = append(candidates, payload.Response)
	}

	for _, candidate := range candidates {
		if strings.TrimSpace(candidate) != "" {
			return candidate, nil
		}
	}
	if !recognized {
		return "", fmt.Errorf("%w: 提供商 %s 的响应中缺少 choices[].message.content、message.content 或 choices[].text",
			errs.ErrAIInvalidResponse, provider)
	}
	// 已识别到响应结构但内容为空：由 Complete 统一判定为空内容错误，
	// Test 场景下则视为服务器可达。
	return "", nil
}

func rawContentToString(raw json.RawMessage) string {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	var parts []contentPart
	if err := json.Unmarshal(raw, &parts); err == nil {
		var builder strings.Builder
		for _, part := range parts {
			if part.Text == "" {
				continue
			}
			if builder.Len() > 0 {
				builder.WriteByte('\n')
			}
			builder.WriteString(part.Text)
		}
		return builder.String()
	}
	var part contentPart
	if err := json.Unmarshal(raw, &part); err == nil {
		return part.Text
	}
	return ""
}

func extractProviderErrorMessage(body []byte) string {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return ""
	}
	var payload struct {
		Error   json.RawMessage `json:"error"`
		Message string          `json:"message"`
		Detail  string          `json:"detail"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	if len(payload.Error) > 0 {
		if message := extractErrorRaw(payload.Error); message != "" {
			return message
		}
	}
	if strings.TrimSpace(payload.Message) != "" {
		return payload.Message
	}
	return payload.Detail
}

func extractErrorRaw(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	var detail struct {
		Message string          `json:"message"`
		Type    string          `json:"type"`
		Code    json.RawMessage `json:"code"`
	}
	if err := json.Unmarshal(raw, &detail); err != nil {
		return ""
	}
	if strings.TrimSpace(detail.Message) != "" {
		return detail.Message
	}
	if len(detail.Code) > 0 {
		var code string
		if json.Unmarshal(detail.Code, &code) == nil {
			return code
		}
		return string(detail.Code)
	}
	return detail.Type
}

func readLimitedBody(reader io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("响应体超过 %d 字节限制", limit)
	}
	return data, nil
}

// displayURL 去掉查询参数、片段与用户信息，避免 Endpoint 中的凭据进入错误信息。
func displayURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "(Endpoint 无法解析)"
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.User = nil
	return parsed.String()
}

// sanitizeNetworkError 避免 url.Error 携带完整 URL（可能含查询参数凭据）。
func sanitizeNetworkError(err error) string {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		inner := ""
		if urlErr.Err != nil {
			inner = urlErr.Err.Error()
		}
		return fmt.Sprintf("%s %s: %s", urlErr.Op, displayURL(urlErr.URL), sanitizeInline(inner))
	}
	return sanitizeInline(err.Error())
}
