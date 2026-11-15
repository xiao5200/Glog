package threat

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"glog/internal/errs"
	"glog/internal/logx"
	"glog/pkg/models"
)

// RunDetection 对项目运行检测规则。ruleIDs 为空时运行全部已启用规则；
// 指定 ruleIDs 时即使规则被禁用也会运行。单条规则失败不会中断其他规则。
func (s *Service) RunDetection(projectID string, ruleIDs []string) (*models.DetectionResult, error) {
	return s.runDetection(projectID, ruleIDs, true)
}

func (s *Service) runDetection(projectID string, ruleIDs []string, updateStats bool) (*models.DetectionResult, error) {
	_ = projectID
	if s.reader == nil {
		return nil, errs.Internal("日志读取器未配置", nil)
	}

	started := time.Now().UTC()
	rules, err := s.selectRules(ruleIDs)
	if err != nil {
		return nil, err
	}

	result := &models.DetectionResult{
		TaskID:    uuid.NewString(),
		Results:   make([]models.RuleRunResult, 0, len(rules)),
		StartedAt: started,
	}

	for _, rule := range rules {
		run := models.RuleRunResult{
			RuleID:    rule.id,
			RuleName:  rule.name,
			Severity:  rule.severity,
			Matches:   make([]models.DetectionMatch, 0),
			Summaries: make([]models.GroupSummary, 0),
		}
		if rule.err != nil {
			run.Error = rule.err.Error()
			result.Results = append(result.Results, run)
			continue
		}

		matches, summaries, count, scanned, detectErr := s.detectRule(rule)
		if detectErr != nil {
			run.Error = detectErr.Error()
			result.Results = append(result.Results, run)
			continue
		}

		run.Count = count
		run.Matches = matches
		run.Summaries = summaries
		result.Scanned += int64(scanned)
		result.TotalMatches += count

		if updateStats && count > 0 && s.store != nil && rule.persisted {
			if err := s.store.UpdateDetectionRuleMatch(rule.id, time.Now().UTC()); err != nil {
				logx.Warn(fmt.Sprintf("更新规则 %s 匹配统计失败: %v", rule.id, err))
			}
		}
		result.Results = append(result.Results, run)
	}

	result.FinishedAt = time.Now().UTC()
	result.Took = result.FinishedAt.Sub(started).Milliseconds()
	return result, nil
}

func (s *Service) detectRule(rule resolvedRule) ([]models.DetectionMatch, []models.GroupSummary, int64, int, error) {
	def := rule.def
	if def == nil {
		return nil, nil, 0, 0, fmt.Errorf("规则 %s 定义为空", rule.id)
	}
	if def.condition == nil {
		return nil, nil, 0, 0, fmt.Errorf("规则 %s 缺少有效条件", rule.id)
	}

	query := buildDetectionQuery(def)
	res, err := s.reader.Search(query)
	if err != nil {
		return nil, nil, 0, 0, fmt.Errorf("搜索候选日志失败: %w", err)
	}
	if res == nil {
		res = &models.QueryResult{}
	}

	hits := res.Hits
	if len(hits) > detectionCandidateLimit {
		logx.Warn(fmt.Sprintf("规则 %s 返回候选 %d 条，已截断到 %d 条", rule.id, len(hits), detectionCandidateLimit))
		hits = hits[:detectionCandidateLimit]
	}
	if res.Total > int64(len(hits)) {
		logx.Warn(fmt.Sprintf("规则 %s 的候选结果不完整: total=%d returned=%d", rule.id, res.Total, len(hits)))
	}

	candidates := make([]*models.LogEntry, 0, len(hits))
	seen := make(map[uint64]struct{}, len(hits))
	for i := range hits {
		entry := &hits[i]
		if _, ok := seen[entry.DocID]; ok {
			continue
		}
		seen[entry.DocID] = struct{}{}
		candidates = append(candidates, entry)
	}
	scanned := len(candidates)

	matched := make([]*models.LogEntry, 0, len(candidates)/2+1)
	for _, entry := range candidates {
		if def.matches(entry) {
			matched = append(matched, entry)
		}
	}
	if len(matched) == 0 {
		return []models.DetectionMatch{}, []models.GroupSummary{}, 0, scanned, nil
	}

	groups := make(map[string][]*models.LogEntry, 8)
	keys := make([]string, 0, 8)
	for _, entry := range matched {
		key := ruleGroupKey(def, entry)
		if _, ok := groups[key]; !ok {
			keys = append(keys, key)
		}
		groups[key] = append(groups[key], entry)
	}
	sort.Strings(keys)

	type matchRef struct {
		entry *models.LogEntry
		group string
	}
	refs := make([]matchRef, 0, len(matched))
	summaries := make([]models.GroupSummary, 0, len(keys))

	for _, key := range keys {
		group := groups[key]
		sort.SliceStable(group, func(i, j int) bool {
			if group[i].Timestamp.Equal(group[j].Timestamp) {
				return group[i].DocID < group[j].DocID
			}
			return group[i].Timestamp.Before(group[j].Timestamp)
		})

		qualified := qualifiedIndices(def.condition, group)
		summary := models.GroupSummary{GroupKey: key, Count: int64(len(group))}
		for _, index := range qualified {
			refs = append(refs, matchRef{entry: group[index], group: key})
			if len(summary.DocIDs) < detectionMatchLimit {
				summary.DocIDs = append(summary.DocIDs, group[index].DocID)
			}
		}
		summaries = append(summaries, summary)
	}

	sort.SliceStable(refs, func(i, j int) bool {
		if refs[i].entry.Timestamp.Equal(refs[j].entry.Timestamp) {
			if refs[i].entry.DocID == refs[j].entry.DocID {
				return refs[i].group < refs[j].group
			}
			return refs[i].entry.DocID < refs[j].entry.DocID
		}
		return refs[i].entry.Timestamp.Before(refs[j].entry.Timestamp)
	})

	total := int64(len(refs))
	if len(refs) > detectionMatchLimit {
		refs = refs[:detectionMatchLimit]
	}

	tags := def.Actions.Tags
	if len(tags) == 0 {
		tags = def.Tags
	}
	matches := make([]models.DetectionMatch, 0, len(refs))
	for _, ref := range refs {
		entry := ref.entry
		matches = append(matches, models.DetectionMatch{
			RuleID:     rule.id,
			RuleName:   rule.name,
			Severity:   rule.severity,
			DocID:      entry.DocID,
			Timestamp:  entry.Timestamp,
			Message:    truncateText(entry.Message, 500),
			Level:      entry.Level,
			SourceHost: entry.SourceHost,
			GroupKey:   ref.group,
			Tags:       copyStringSlice(tags),
			RiskScore:  def.Actions.RiskScore,
		})
	}
	return matches, summaries, total, scanned, nil
}

func buildDetectionQuery(def *ruleDefinition) models.Query {
	query := models.Query{
		Offset:    0,
		Limit:     detectionCandidateLimit,
		SortBy:    "timestamp",
		SortOrder: "asc",
	}
	for i := range def.Detection.Filter {
		clause := &def.Detection.Filter[i]
		if clause.Op != "eq" && clause.Op != "one_of" {
			continue
		}
		values := clause.stringValues()
		switch strings.ToLower(strings.TrimSpace(clause.Field)) {
		case "level":
			for _, value := range values {
				value = strings.TrimSpace(value)
				if value == "" {
					continue
				}
				level := models.NormalizeLevel(value)
				if level == models.LevelUnknown {
					level = strings.ToUpper(value)
				}
				query.Levels = appendUniqueFold(query.Levels, level)
			}
		case "event_id":
			for _, value := range values {
				parsed, err := strconv.ParseUint(strings.TrimSpace(value), 10, 32)
				if err != nil {
					continue
				}
				query.EventIDs = appendUniqueUint32(query.EventIDs, uint32(parsed))
			}
		case "source_host", "host":
			for _, value := range values {
				value = strings.TrimSpace(value)
				if value != "" {
					query.Hosts = appendUniqueFold(query.Hosts, value)
				}
			}
		case "source":
			for _, value := range values {
				value = strings.TrimSpace(value)
				if value != "" {
					query.Sources = appendUniqueFold(query.Sources, value)
				}
			}
		}
	}
	return query
}

func qualifiedIndices(condition *countCondition, entries []*models.LogEntry) []int {
	if condition == nil || len(entries) == 0 {
		return nil
	}
	if condition.Window <= 0 {
		if !condition.evaluate(int64(len(entries))) {
			return nil
		}
		out := make([]int, len(entries))
		for i := range out {
			out[i] = i
		}
		return out
	}

	type interval struct {
		start int
		end   int
	}
	intervals := make([]interval, 0, 8)
	left := 0
	for right := 0; right < len(entries); right++ {
		cutoff := entries[right].Timestamp.Add(-condition.Window)
		for left <= right && entries[left].Timestamp.Before(cutoff) {
			left++
		}

		switch condition.Op {
		case "<":
			for left <= right && int64(right-left+1) >= condition.Threshold {
				left++
			}
		case "<=", "==":
			for left <= right && int64(right-left+1) > condition.Threshold {
				left++
			}
		}

		if left > right {
			continue
		}
		count := int64(right - left + 1)
		if !condition.evaluate(count) {
			continue
		}
		if len(intervals) > 0 && left <= intervals[len(intervals)-1].end+1 {
			if right > intervals[len(intervals)-1].end {
				intervals[len(intervals)-1].end = right
			}
			continue
		}
		intervals = append(intervals, interval{start: left, end: right})
	}

	if len(intervals) == 0 {
		return nil
	}
	indices := make([]int, 0)
	for _, interval := range intervals {
		for i := interval.start; i <= interval.end; i++ {
			indices = append(indices, i)
		}
	}
	return indices
}

func ruleGroupKey(def *ruleDefinition, entry *models.LogEntry) string {
	if def == nil || len(def.Detection.GroupBy) == 0 {
		return ""
	}
	parts := make([]string, len(def.Detection.GroupBy))
	for i, field := range def.Detection.GroupBy {
		parts[i] = resolveRuleField(entry, field).str
	}
	return strings.Join(parts, "|")
}

func (r *ruleDefinition) matches(entry *models.LogEntry) bool {
	if r == nil || entry == nil {
		return false
	}
	for i := range r.Detection.Filter {
		if !matchFilterClause(&r.Detection.Filter[i], entry) {
			return false
		}
	}
	return true
}

type ruleFieldValue struct {
	present bool
	raw     interface{}
	str     string
	num     float64
	isNum   bool
}

func matchFilterClause(clause *filterClause, entry *models.LogEntry) bool {
	field := resolveRuleField(entry, clause.Field)
	if clause.Op == "exists" {
		expected := true
		raws := clause.rawValues()
		if len(raws) == 1 {
			if boolean, ok := raws[0].(bool); ok {
				expected = boolean
			}
		}
		return field.present == expected
	}
	if !field.present {
		return false
	}

	values := clause.stringValues()
	if len(values) == 0 {
		return false
	}
	results := make([]bool, 0, len(values))
	for i, value := range values {
		var matched bool
		switch clause.Op {
		case "eq", "one_of":
			matched = ruleFieldEquals(field, value)
		case "ne":
			matched = !ruleFieldEquals(field, value)
		case "contains":
			matched = strings.Contains(strings.ToLower(field.str), strings.ToLower(value))
		case "prefix":
			matched = strings.HasPrefix(strings.ToLower(field.str), strings.ToLower(value))
		case "suffix":
			matched = strings.HasSuffix(strings.ToLower(field.str), strings.ToLower(value))
		case "regex":
			if i < len(clause.regexps) && clause.regexps[i] != nil {
				matched = clause.regexps[i].MatchString(field.str)
			}
		case "gt", "lt", "gte", "lte":
			threshold, ok := parseNumericValue(value)
			if ok && field.isNum {
				switch clause.Op {
				case "gt":
					matched = field.num > threshold
				case "lt":
					matched = field.num < threshold
				case "gte":
					matched = field.num >= threshold
				case "lte":
					matched = field.num <= threshold
				}
			}
		}
		results = append(results, matched)
	}

	if clause.Match == "all" {
		for _, matched := range results {
			if !matched {
				return false
			}
		}
		return true
	}
	for _, matched := range results {
		if matched {
			return true
		}
	}
	return false
}

func ruleFieldEquals(field ruleFieldValue, expected string) bool {
	if !field.present {
		return false
	}
	if field.isNum {
		if number, ok := parseNumericValue(expected); ok {
			return field.num == number
		}
	}
	return strings.EqualFold(field.str, expected)
}

func resolveRuleField(entry *models.LogEntry, field string) ruleFieldValue {
	if entry == nil {
		return ruleFieldValue{}
	}
	field = strings.TrimSpace(field)
	lower := strings.ToLower(field)
	switch lower {
	case "level":
		return stringRuleField(entry.Level)
	case "message":
		return stringRuleField(entry.Message)
	case "raw_log", "raw":
		return stringRuleField(entry.RawLog)
	case "source_host", "host":
		return stringRuleField(entry.SourceHost)
	case "source":
		return stringRuleField(entry.Source)
	case "source_file", "file", "file_path":
		return stringRuleField(entry.SourceFile)
	case "event_id":
		if entry.EventID != nil {
			return numberRuleField(uint64(*entry.EventID))
		}
		if value, ok := lookupFieldCI(entry.Fields, "event_id"); ok {
			return newRuleFieldValue(value)
		}
		return ruleFieldValue{}
	case "provider":
		if entry.Provider != nil {
			return stringRuleField(*entry.Provider)
		}
		if value, ok := lookupFieldCI(entry.Fields, "provider"); ok {
			return newRuleFieldValue(value)
		}
		return ruleFieldValue{}
	case "computer":
		if entry.Computer != nil {
			return stringRuleField(*entry.Computer)
		}
		if value, ok := lookupFieldCI(entry.Fields, "computer"); ok {
			return newRuleFieldValue(value)
		}
		return ruleFieldValue{}
	case "user_id", "user":
		if entry.UserID != nil {
			return stringRuleField(*entry.UserID)
		}
		if value, ok := lookupFieldCI(entry.Fields, "user_id"); ok {
			return newRuleFieldValue(value)
		}
		return ruleFieldValue{}
	case "timestamp":
		return stringRuleField(entry.Timestamp.Format(time.RFC3339Nano))
	case "doc_id", "docid":
		return numberRuleField(entry.DocID)
	case "id":
		if strings.TrimSpace(entry.ID) != "" {
			return stringRuleField(entry.ID)
		}
		return numberRuleField(entry.DocID)
	case "fields":
		return newRuleFieldValue(entry.Fields)
	}

	if strings.HasPrefix(lower, "fields.") {
		remainder := field[len("fields."):]
		if value, ok := lookupPath(entry.Fields, remainder); ok {
			return newRuleFieldValue(value)
		}
		return ruleFieldValue{}
	}
	if value, ok := lookupPath(entry.Fields, field); ok {
		return newRuleFieldValue(value)
	}
	return ruleFieldValue{}
}

func stringRuleField(value string) ruleFieldValue {
	return ruleFieldValue{present: true, raw: value, str: value}
}

func numberRuleField(value uint64) ruleFieldValue {
	text := strconv.FormatUint(value, 10)
	number, _ := strconv.ParseFloat(text, 64)
	return ruleFieldValue{present: true, raw: value, str: text, num: number, isNum: true}
}

func newRuleFieldValue(value interface{}) ruleFieldValue {
	if value == nil {
		return ruleFieldValue{present: true}
	}
	out := ruleFieldValue{present: true, raw: value}
	switch typed := value.(type) {
	case string:
		out.str = typed
		out.num, out.isNum = parseNumericValue(typed)
	case json.Number:
		out.str = typed.String()
		out.num, out.isNum = parseNumericValue(out.str)
	case float64:
		out.num, out.isNum = typed, true
		out.str = strconv.FormatFloat(typed, 'f', -1, 64)
	case float32:
		out.num, out.isNum = float64(typed), true
		out.str = strconv.FormatFloat(float64(typed), 'f', -1, 64)
	case int:
		out.str = strconv.Itoa(typed)
		out.num, out.isNum = float64(typed), true
	case int8:
		out.str = strconv.FormatInt(int64(typed), 10)
		out.num, out.isNum = float64(typed), true
	case int16:
		out.str = strconv.FormatInt(int64(typed), 10)
		out.num, out.isNum = float64(typed), true
	case int32:
		out.str = strconv.FormatInt(int64(typed), 10)
		out.num, out.isNum = float64(typed), true
	case int64:
		out.str = strconv.FormatInt(typed, 10)
		out.num, out.isNum = float64(typed), true
	case uint:
		out.str = strconv.FormatUint(uint64(typed), 10)
		out.num, out.isNum = float64(typed), true
	case uint8:
		out.str = strconv.FormatUint(uint64(typed), 10)
		out.num, out.isNum = float64(typed), true
	case uint16:
		out.str = strconv.FormatUint(uint64(typed), 10)
		out.num, out.isNum = float64(typed), true
	case uint32:
		out.str = strconv.FormatUint(uint64(typed), 10)
		out.num, out.isNum = float64(typed), true
	case uint64:
		out.str = strconv.FormatUint(typed, 10)
		out.num, out.isNum = float64(typed), true
	case bool:
		out.str = strconv.FormatBool(typed)
	case []byte:
		out.str = string(typed)
	default:
		if data, err := json.Marshal(value); err == nil {
			out.str = string(data)
		} else {
			out.str = fmt.Sprint(value)
		}
	}
	return out
}

func lookupFieldCI(fields map[string]interface{}, key string) (interface{}, bool) {
	if fields == nil || key == "" {
		return nil, false
	}
	if value, ok := fields[key]; ok {
		return value, true
	}
	keys := make([]string, 0, len(fields))
	for existing := range fields {
		keys = append(keys, existing)
	}
	sort.Strings(keys)
	for _, existing := range keys {
		if strings.EqualFold(existing, key) {
			return fields[existing], true
		}
	}
	return nil, false
}

func lookupPath(fields map[string]interface{}, path string) (interface{}, bool) {
	path = strings.TrimSpace(path)
	if fields == nil || path == "" {
		return nil, false
	}
	if value, ok := lookupFieldCI(fields, path); ok {
		return value, true
	}
	index := strings.IndexByte(path, '.')
	if index <= 0 || index == len(path)-1 {
		return nil, false
	}
	head, tail := path[:index], path[index+1:]
	value, ok := lookupFieldCI(fields, head)
	if !ok {
		return nil, false
	}
	switch nested := value.(type) {
	case map[string]interface{}:
		return lookupPath(nested, tail)
	case map[string]string:
		return lookupStringPath(nested, tail)
	default:
		return nil, false
	}
}

func lookupStringPath(fields map[string]string, path string) (interface{}, bool) {
	if fields == nil || path == "" {
		return nil, false
	}
	if value, ok := fields[path]; ok {
		return value, true
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if strings.EqualFold(key, path) {
			return fields[key], true
		}
	}
	return nil, false
}

func parseNumericValue(value string) (float64, bool) {
	number, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
		return 0, false
	}
	return number, true
}

func appendUniqueFold(values []string, value string) []string {
	for _, existing := range values {
		if strings.EqualFold(existing, value) {
			return values
		}
	}
	return append(values, value)
}

func appendUniqueUint32(values []uint32, value uint32) []uint32 {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
