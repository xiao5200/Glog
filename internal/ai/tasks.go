package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"glog/pkg/models"
)

// ---------------------------------------------------------------------------
// 稳健 JSON 辅助类型
// ---------------------------------------------------------------------------

// flexString 接受字符串、数字、布尔和 null，统一转换为字符串。
type flexString string

func (s *flexString) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		*s = ""
		return nil
	}

	var value interface{}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	switch typed := value.(type) {
	case string:
		*s = flexString(typed)
	case json.Number:
		*s = flexString(typed.String())
	case bool:
		*s = flexString(strconv.FormatBool(typed))
	case nil:
		*s = ""
	default:
		return fmt.Errorf("期望字符串，实际为 %T", typed)
	}
	return nil
}

// stringList 接受字符串、数字数组或单个标量。
type stringList []string

func (l *stringList) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		*l = nil
		return nil
	}

	if trimmed[0] != '[' {
		var item flexString
		if err := item.UnmarshalJSON(trimmed); err != nil {
			return err
		}
		*l = []string{string(item)}
		return nil
	}

	var raw []json.RawMessage
	if err := json.Unmarshal(trimmed, &raw); err != nil {
		return err
	}
	result := make([]string, 0, len(raw))
	for _, itemRaw := range raw {
		var item flexString
		if err := item.UnmarshalJSON(itemRaw); err != nil {
			// 忽略无法转换的数组元素，保留其余可用内容。
			continue
		}
		result = append(result, string(item))
	}
	*l = result
	return nil
}

// flexFloat 接受数字或数字字符串。
type flexFloat float64

func (f *flexFloat) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		*f = 0
		return nil
	}

	var value interface{}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	switch typed := value.(type) {
	case json.Number:
		number, err := strconv.ParseFloat(typed.String(), 64)
		if err != nil {
			return fmt.Errorf("无效数值 %q", truncateRunes(typed.String(), 40))
		}
		*f = flexFloat(number)
		return nil
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			*f = 0
			return nil
		}
		number, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return fmt.Errorf("无效数值 %q", truncateRunes(text, 40))
		}
		*f = flexFloat(number)
		return nil
	default:
		return fmt.Errorf("期望数值，实际为 %T", typed)
	}
}

// flexIntMap 用于 field_map：值为整数或数字字符串。
type flexIntMap map[string]int

func (m *flexIntMap) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		*m = nil
		return nil
	}
	if trimmed[0] != '{' {
		return fmt.Errorf("field_map 必须是对象")
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &raw); err != nil {
		return err
	}

	keys := make([]string, 0, len(raw))
	for key := range raw {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	result := make(flexIntMap, len(raw))
	for _, key := range keys {
		var number flexFloat
		if err := number.UnmarshalJSON(raw[key]); err != nil {
			return fmt.Errorf("字段 %q 的值无效：%v", truncateRunes(key, 100), err)
		}
		value := float64(number)
		if math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value {
			return fmt.Errorf("字段 %q 的捕获组索引必须是整数", truncateRunes(key, 100))
		}
		if value < -1e9 || value > 1e9 {
			return fmt.Errorf("字段 %q 的捕获组索引超出范围", truncateRunes(key, 100))
		}
		result[key] = int(value)
	}
	*m = result
	return nil
}

// flexUint32List 接受数字、数字字符串或其数组，非法元素直接忽略。
type flexUint32List []uint32

func (l *flexUint32List) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		*l = nil
		return nil
	}

	if trimmed[0] != '[' {
		if value, ok := parseFlexUint32(trimmed); ok {
			*l = []uint32{value}
		} else {
			*l = nil
		}
		return nil
	}

	var raw []json.RawMessage
	if err := json.Unmarshal(trimmed, &raw); err != nil {
		return err
	}
	result := make([]uint32, 0, len(raw))
	for _, itemRaw := range raw {
		if value, ok := parseFlexUint32(itemRaw); ok {
			result = append(result, value)
		}
	}
	*l = result
	return nil
}

func parseFlexUint32(data []byte) (uint32, bool) {
	var number flexFloat
	if err := number.UnmarshalJSON(data); err != nil {
		return 0, false
	}
	value := float64(number)
	if math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value || value < 0 || value > 4294967295 {
		return 0, false
	}
	return uint32(value), true
}

// ---------------------------------------------------------------------------
// 模板解析
// ---------------------------------------------------------------------------

type templatePayload struct {
	Name        flexString            `json:"name"`
	Description flexString            `json:"description"`
	Format      flexString            `json:"format"`
	Regex       flexString            `json:"regex"`
	FieldMap    flexIntMap            `json:"field_map"`
	TimeFormat  flexString            `json:"time_format"`
	TimeField   flexString            `json:"time_field"`
	LevelField  flexString            `json:"level_field"`
	LevelMap    map[string]flexString `json:"level_map"`
	Examples    stringList            `json:"examples"`
	Category    flexString            `json:"category"`
}

// ParseTemplateResponse 解析并校验 AI 生成的解析模板。
func ParseTemplateResponse(raw string) (*models.ParseTemplate, error) {
	var payload templatePayload
	if err := ParseJSONInto(raw, &payload); err != nil {
		return nil, err
	}

	format := strings.ToLower(strings.TrimSpace(string(payload.Format)))
	if format == "" {
		format = "auto"
	}
	switch format {
	case "regex", "json", "syslog", "auto":
	default:
		return nil, invalidAIResponse("模板 format 必须是 regex/json/syslog/auto 之一，实际为 %q", truncateRunes(format, 50))
	}

	regexText := strings.TrimSpace(string(payload.Regex))
	groupCount := 0
	if regexText != "" {
		compiled, err := regexp.Compile(regexText)
		if err != nil {
			return nil, invalidAIResponse("模板正则不是有效的 RE2 表达式：%v", err)
		}
		groupCount = compiled.NumSubexp()
	} else if format != "json" && format != "syslog" {
		return nil, invalidAIResponse("format 为 %q 时必须提供 regex", format)
	}

	if len(payload.FieldMap) > 0 {
		if regexText == "" {
			return nil, invalidAIResponse("regex 为空时 field_map 必须为空")
		}
		keys := make([]string, 0, len(payload.FieldMap))
		for key := range payload.FieldMap {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			field := strings.TrimSpace(key)
			index := payload.FieldMap[key]
			if field == "" {
				return nil, invalidAIResponse("field_map 包含空字段名")
			}
			if index < 1 {
				return nil, invalidAIResponse("field_map[%q] 的捕获组索引必须从 1 开始，实际为 %d", truncateRunes(field, 100), index)
			}
			if index > groupCount {
				return nil, invalidAIResponse("field_map[%q] 的捕获组索引 %d 超出正则捕获组数量 %d",
					truncateRunes(field, 100), index, groupCount)
			}
		}
	}

	levelMap := make(map[string]string, len(payload.LevelMap))
	for key, value := range payload.LevelMap {
		key = strings.TrimSpace(key)
		valueText := strings.TrimSpace(string(value))
		if key == "" || valueText == "" {
			continue
		}
		if normalized := models.NormalizeLevel(valueText); normalized != models.LevelUnknown {
			valueText = normalized
		} else {
			valueText = strings.ToUpper(valueText)
		}
		levelMap[key] = valueText
	}
	if len(levelMap) == 0 {
		levelMap = nil
	}

	name := truncateRunes(strings.TrimSpace(string(payload.Name)), 200)
	if name == "" {
		name = "AI生成模板"
	}

	examples := make([]string, 0, len(payload.Examples))
	seenExamples := make(map[string]struct{}, len(payload.Examples))
	for _, example := range payload.Examples {
		example = strings.TrimSpace(sanitizeText(example))
		if example == "" {
			continue
		}
		if _, exists := seenExamples[example]; exists {
			continue
		}
		seenExamples[example] = struct{}{}
		examples = append(examples, truncateRunes(example, maxTemplateSampleLen))
		if len(examples) >= 20 {
			break
		}
	}

	now := time.Now().UTC()
	template := &models.ParseTemplate{
		Name:        name,
		Description: truncateRunes(strings.TrimSpace(string(payload.Description)), 2000),
		Type:        "ai_generated",
		Category:    truncateRunes(strings.TrimSpace(string(payload.Category)), 100),
		Format:      format,
		Priority:    50,
		Regex:       regexText,
		FieldMap:    map[string]int(payload.FieldMap),
		TimeFormat:  truncateRunes(strings.TrimSpace(string(payload.TimeFormat)), 200),
		TimeField:   truncateRunes(strings.TrimSpace(string(payload.TimeField)), 256),
		LevelMap:    levelMap,
		LevelField:  truncateRunes(strings.TrimSpace(string(payload.LevelField)), 256),
		Examples:    examples,
		CreatedAt:   now,
		UpdatedAt:   now,
		Source:      "ai",
	}
	return template, nil
}

// ---------------------------------------------------------------------------
// 异常检测解析
// ---------------------------------------------------------------------------

type anomalyItemPayload struct {
	Timestamp flexString `json:"timestamp"`
	Level     flexString `json:"level"`
	Message   flexString `json:"message"`
	Reason    flexString `json:"reason"`
	Score     flexFloat  `json:"score"`
}

type anomalyPayload struct {
	Summary     flexString           `json:"summary"`
	RiskLevel   flexString           `json:"risk_level"`
	Findings    stringList           `json:"findings"`
	Suggestions stringList           `json:"suggestions"`
	Anomalies   []anomalyItemPayload `json:"anomalies"`
}

// ParseAnomalyResponse 解析异常检测结果，容忍缺失字段并归一化风险级别与分数。
func ParseAnomalyResponse(raw string) (*models.AnomalyDetectionResult, error) {
	var payload anomalyPayload
	if err := ParseJSONInto(raw, &payload); err != nil {
		return nil, err
	}

	summary := truncateRunes(strings.TrimSpace(sanitizeText(string(payload.Summary))), 20000)
	findings := sanitizeStringList(payload.Findings, 50, 2000)
	suggestions := sanitizeStringList(payload.Suggestions, 50, 2000)

	anomalies := make([]models.Anomaly, 0, len(payload.Anomalies))
	for _, item := range payload.Anomalies {
		if len(anomalies) >= 500 {
			break
		}
		timestamp := parseFlexibleTime(string(item.Timestamp))
		level := normalizeAILevel(string(item.Level))
		message := truncateRunes(strings.TrimSpace(sanitizeText(string(item.Message))), 2000)
		reason := truncateRunes(strings.TrimSpace(sanitizeText(string(item.Reason))), 2000)
		score := clampFloat(float64(item.Score), 0, 100)

		if timestamp.IsZero() && level == "" && message == "" && reason == "" && score == 0 {
			continue
		}
		anomalies = append(anomalies, models.Anomaly{
			Timestamp: timestamp,
			Level:     level,
			Message:   message,
			Reason:    reason,
			Score:     score,
		})
	}

	rawRisk := strings.TrimSpace(string(payload.RiskLevel))
	riskLevel := normalizeRiskLevel(rawRisk, anomalies)
	if summary == "" && rawRisk == "" && len(findings) == 0 && len(suggestions) == 0 && len(anomalies) == 0 {
		return nil, invalidAIResponse("AI 未返回可识别的异常检测结果")
	}

	return &models.AnomalyDetectionResult{
		Summary:     summary,
		RiskLevel:   riskLevel,
		Findings:    findings,
		Suggestions: suggestions,
		Anomalies:   anomalies,
		GeneratedAt: time.Now().UTC(),
	}, nil
}

// ---------------------------------------------------------------------------
// 单条日志分析解析
// ---------------------------------------------------------------------------

type logAnalysisPayload struct {
	Summary       flexString     `json:"summary"`
	RiskLevel     flexString     `json:"risk_level"`
	Findings      stringList     `json:"findings"`
	Suggestions   stringList     `json:"suggestions"`
	RelatedDocIDs flexUint32List `json:"related_doc_ids"`
}

// ParseLogAnalysisResponse 解析单条日志分析结果。
// 容忍缺失字段，归一化风险级别，并对 related_doc_ids 去重、限量。
func ParseLogAnalysisResponse(raw string) (*models.LogAnalysisResult, error) {
	var payload logAnalysisPayload
	if err := ParseJSONInto(raw, &payload); err != nil {
		return nil, err
	}

	summary := truncateRunes(strings.TrimSpace(sanitizeText(string(payload.Summary))), 20000)
	findings := sanitizeStringList(payload.Findings, 50, 2000)
	suggestions := sanitizeStringList(payload.Suggestions, 50, 2000)

	relatedDocIDs := make([]uint32, 0, len(payload.RelatedDocIDs))
	seenDocIDs := make(map[uint32]struct{}, len(payload.RelatedDocIDs))
	for _, docID := range payload.RelatedDocIDs {
		if _, exists := seenDocIDs[docID]; exists {
			continue
		}
		seenDocIDs[docID] = struct{}{}
		relatedDocIDs = append(relatedDocIDs, docID)
		if len(relatedDocIDs) >= 200 {
			break
		}
	}

	rawRisk := strings.TrimSpace(string(payload.RiskLevel))
	if summary == "" && rawRisk == "" && len(findings) == 0 && len(suggestions) == 0 && len(relatedDocIDs) == 0 {
		return nil, invalidAIResponse("AI 未返回可识别的日志分析结果")
	}

	return &models.LogAnalysisResult{
		Summary:       summary,
		RiskLevel:     normalizeRiskLevel(rawRisk, nil),
		Findings:      findings,
		Suggestions:   suggestions,
		RelatedDocIDs: relatedDocIDs,
		GeneratedAt:   time.Now().UTC(),
	}, nil
}

// ---------------------------------------------------------------------------
// 根因分析解析
// ---------------------------------------------------------------------------

type rootCauseItemPayload struct {
	Cause       flexString `json:"cause"`
	Confidence  flexFloat  `json:"confidence"`
	Evidence    stringList `json:"evidence"`
	Description flexString `json:"description"`
}

type rootCausePayload struct {
	Summary     flexString             `json:"summary"`
	RootCauses  []rootCauseItemPayload `json:"root_causes"`
	Timeline    stringList             `json:"timeline"`
	Suggestions stringList             `json:"suggestions"`
}

// ParseRootCauseResponse 解析根因分析结果，confidence 限定在 0-1。
func ParseRootCauseResponse(raw string) (*models.RootCauseResult, error) {
	var payload rootCausePayload
	if err := ParseJSONInto(raw, &payload); err != nil {
		return nil, err
	}

	summary := truncateRunes(strings.TrimSpace(sanitizeText(string(payload.Summary))), 20000)

	rootCauses := make([]models.RootCause, 0, len(payload.RootCauses))
	for _, item := range payload.RootCauses {
		if len(rootCauses) >= 50 {
			break
		}
		cause := truncateRunes(strings.TrimSpace(sanitizeText(string(item.Cause))), 1000)
		description := truncateRunes(strings.TrimSpace(sanitizeText(string(item.Description))), 4000)
		evidence := sanitizeStringList(item.Evidence, 50, 2000)
		if cause == "" && description == "" && len(evidence) == 0 {
			continue
		}
		rootCauses = append(rootCauses, models.RootCause{
			Cause:       cause,
			Confidence:  clampFloat(float64(item.Confidence), 0, 1),
			Evidence:    evidence,
			Description: description,
		})
	}

	timeline := sanitizeStringList(payload.Timeline, 100, 2000)
	suggestions := sanitizeStringList(payload.Suggestions, 50, 2000)

	if summary == "" && len(rootCauses) == 0 && len(timeline) == 0 && len(suggestions) == 0 {
		return nil, invalidAIResponse("AI 未返回可识别的根因分析结果")
	}

	return &models.RootCauseResult{
		Summary:     summary,
		RootCauses:  rootCauses,
		Timeline:    timeline,
		Suggestions: suggestions,
		GeneratedAt: time.Now().UTC(),
	}, nil
}

// ---------------------------------------------------------------------------
// 自然语言查询解析
// ---------------------------------------------------------------------------

type nlQueryPayload struct {
	Text      flexString     `json:"text"`
	Levels    stringList     `json:"levels"`
	Hosts     stringList     `json:"hosts"`
	Sources   stringList     `json:"sources"`
	EventIDs  flexUint32List `json:"event_ids"`
	StartTime flexString     `json:"start_time"`
	EndTime   flexString     `json:"end_time"`
	SortBy    flexString     `json:"sort_by"`
	SortOrder flexString     `json:"sort_order"`
	Limit     flexString     `json:"limit"`
}

// ParseNLQueryResponse 解析自然语言查询结果，输出可直接交给查询后端的 models.Query。
func ParseNLQueryResponse(raw string) (*models.Query, error) {
	var payload nlQueryPayload
	if err := ParseJSONInto(raw, &payload); err != nil {
		return nil, err
	}

	text := truncateRunes(sanitizeInline(string(payload.Text)), 20000)

	levels := make([]models.LogLevel, 0, len(payload.Levels))
	seenLevels := make(map[string]struct{}, len(payload.Levels))
	for _, rawLevel := range payload.Levels {
		level := models.NormalizeLevel(rawLevel)
		if level == models.LevelUnknown {
			continue
		}
		if _, exists := seenLevels[level]; exists {
			continue
		}
		seenLevels[level] = struct{}{}
		levels = append(levels, level)
	}

	hosts := sanitizeQueryValues(payload.Hosts, 100, 256)
	sources := sanitizeQueryValues(payload.Sources, 100, 256)

	eventIDs := make([]uint32, 0, len(payload.EventIDs))
	seenEventIDs := make(map[uint32]struct{}, len(payload.EventIDs))
	for _, eventID := range payload.EventIDs {
		if _, exists := seenEventIDs[eventID]; exists {
			continue
		}
		seenEventIDs[eventID] = struct{}{}
		eventIDs = append(eventIDs, eventID)
		if len(eventIDs) >= 100 {
			break
		}
	}

	startTime := parseRFC3339Time(string(payload.StartTime))
	endTime := parseRFC3339Time(string(payload.EndTime))
	if startTime != nil && endTime != nil && startTime.After(*endTime) {
		startTime, endTime = endTime, startTime
	}

	sortBy := strings.ToLower(strings.TrimSpace(string(payload.SortBy)))
	switch sortBy {
	case "timestamp", "level", "doc_id":
	default:
		sortBy = ""
	}
	sortOrder := strings.ToLower(strings.TrimSpace(string(payload.SortOrder)))
	switch sortOrder {
	case "asc", "desc":
	default:
		sortOrder = ""
	}

	limit := 100
	if rawLimit := strings.TrimSpace(string(payload.Limit)); rawLimit != "" {
		if value, err := strconv.ParseFloat(rawLimit, 64); err == nil &&
			!math.IsNaN(value) && !math.IsInf(value, 0) {
			// 先按浮点范围收敛，避免超大数值转换为 int 时的实现相关行为。
			switch {
			case value < 1:
				limit = 1
			case value > 1000:
				limit = 1000
			default:
				limit = int(value)
			}
		}
	}

	return &models.Query{
		Text:      text,
		StartTime: startTime,
		EndTime:   endTime,
		Levels:    levels,
		Hosts:     hosts,
		Sources:   sources,
		EventIDs:  eventIDs,
		SortBy:    sortBy,
		SortOrder: sortOrder,
		Limit:     limit,
	}, nil
}

// ---------------------------------------------------------------------------
// 报告解析
// ---------------------------------------------------------------------------

// ParseReportResponse 解析报告响应，支持 Markdown 正文以及 JSON 包裹形式。
func ParseReportResponse(raw string) (*models.ReportResult, error) {
	contentCandidate := strings.TrimSpace(stripOuterMarkdownFence(trimBOMAndSpace(raw)))
	if contentCandidate == "" {
		return nil, invalidAIResponse("AI 返回的报告内容为空")
	}

	if strings.HasPrefix(contentCandidate, "{") {
		var envelope struct {
			Title   flexString `json:"title"`
			Content flexString `json:"content"`
		}
		if err := ParseJSONInto(contentCandidate, &envelope); err == nil {
			content := strings.TrimSpace(stripOuterMarkdownFence(string(envelope.Content)))
			if content == "" {
				return nil, invalidAIResponse("AI 返回的报告内容为空")
			}
			title := truncateRunes(sanitizeInline(string(envelope.Title)), 200)
			if title == "" {
				title = extractMarkdownTitle(content)
			}
			if title == "" {
				title = "日志分析报告"
			}
			return &models.ReportResult{
				Title:       title,
				Content:     content,
				Format:      "markdown",
				GeneratedAt: time.Now().UTC(),
			}, nil
		}
	}

	title := extractMarkdownTitle(contentCandidate)
	if title == "" {
		title = "日志分析报告"
	}
	return &models.ReportResult{
		Title:       title,
		Content:     contentCandidate,
		Format:      "markdown",
		GeneratedAt: time.Now().UTC(),
	}, nil
}

func stripOuterMarkdownFence(text string) string {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "```") {
		return text
	}

	if strings.HasSuffix(text, "```") && len(text) >= 6 {
		body := text[3 : len(text)-3]
		body = strings.TrimLeft(body, " \t")
		if newline := strings.IndexByte(body, '\n'); newline >= 0 {
			firstLine := strings.TrimSpace(strings.TrimSuffix(body[:newline], "\r"))
			if isMarkdownFenceLanguage(firstLine) {
				body = body[newline+1:]
			}
		} else if isMarkdownFenceLanguage(strings.TrimSpace(body)) {
			body = ""
		}
		return strings.TrimSpace(body)
	}

	// 缺少闭合围栏：仅在开头是 markdown 语言标记时去掉该围栏行。
	body := strings.TrimLeft(text[3:], " \t")
	if newline := strings.IndexByte(body, '\n'); newline >= 0 {
		firstLine := strings.TrimSpace(strings.TrimSuffix(body[:newline], "\r"))
		if isMarkdownFenceLanguage(firstLine) {
			return strings.TrimSpace(body[newline+1:])
		}
	}
	return text
}

func isMarkdownFenceLanguage(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "markdown", "md", "text", "plaintext", "txt":
		return true
	default:
		return false
	}
}

func extractMarkdownTitle(content string) string {
	lines := strings.Split(content, "\n")
	inFence := false
	fallback := ""

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}

		level := 0
		for level < len(trimmed) && trimmed[level] == '#' {
			level++
		}
		if level < 1 || level > 6 {
			continue
		}
		rest := trimmed[level:]
		if rest != "" && rest[0] != ' ' && rest[0] != '\t' {
			continue
		}
		title := strings.TrimSpace(strings.TrimRight(rest, "#"))
		if title == "" {
			continue
		}
		title = truncateRunes(sanitizeInline(title), 200)
		if level == 1 {
			return title
		}
		if fallback == "" {
			fallback = title
		}
	}
	return fallback
}

// ---------------------------------------------------------------------------
// 通用归一化/清洗
// ---------------------------------------------------------------------------

func sanitizeStringList(items []string, maxItems, maxLen int) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		value := truncateRunes(strings.TrimSpace(sanitizeText(item)), maxLen)
		if value == "" {
			continue
		}
		result = append(result, value)
		if maxItems > 0 && len(result) >= maxItems {
			break
		}
	}
	return result
}

func sanitizeQueryValues(items []string, maxItems, maxLen int) []string {
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		value := truncateRunes(sanitizeInline(item), maxLen)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
		if maxItems > 0 && len(result) >= maxItems {
			break
		}
	}
	return result
}

func clampFloat(value, minValue, maxValue float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return minValue
	}
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func normalizeAILevel(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if normalized := models.NormalizeLevel(raw); normalized != models.LevelUnknown {
		return normalized
	}
	return truncateRunes(strings.ToUpper(raw), 32)
}

func normalizeRiskLevel(raw string, anomalies []models.Anomaly) string {
	key := strings.ToLower(strings.TrimSpace(raw))
	key = strings.TrimSpace(strings.TrimSuffix(key, "风险"))
	key = strings.TrimSpace(strings.TrimSuffix(key, " risk"))
	switch key {
	case "high", "critical", "severe", "urgent", "严重", "高", "紧急":
		return "high"
	case "medium", "mid", "moderate", "warning", "中", "中等":
		return "medium"
	case "low", "info", "minor", "none", "safe", "低":
		return "low"
	}

	maxScore := 0.0
	for _, anomaly := range anomalies {
		if anomaly.Score > maxScore {
			maxScore = anomaly.Score
		}
	}
	switch {
	case maxScore >= 80:
		return "high"
	case maxScore >= 50:
		return "medium"
	default:
		return "low"
	}
}

func parseFlexibleTime(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" ||
		strings.EqualFold(value, "null") ||
		strings.EqualFold(value, "unknown") ||
		value == "-" {
		return time.Time{}
	}

	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
		"2006/01/02 15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}

	if number, err := strconv.ParseInt(value, 10, 64); err == nil {
		if number <= 0 {
			return time.Time{}
		}
		if number > 100000000000 {
			return time.UnixMilli(number).UTC()
		}
		return time.Unix(number, 0).UTC()
	}
	return time.Time{}
}

func parseRFC3339Time(value string) *time.Time {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "null") {
		return nil
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, value); err == nil {
			parsed = parsed.UTC()
			return &parsed
		}
	}
	return nil
}
