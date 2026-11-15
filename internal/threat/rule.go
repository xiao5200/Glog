package threat

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"glog/internal/errs"
	"glog/pkg/models"
)

type mitreAttack struct {
	Tactics    []string `yaml:"tactics"`
	Techniques []string `yaml:"techniques"`
}

type ruleAlert struct {
	Title   string `yaml:"title"`
	Message string `yaml:"message"`
}

type ruleActions struct {
	Tags      []string   `yaml:"tags"`
	RiskScore int        `yaml:"risk_score"`
	Alert     *ruleAlert `yaml:"alert"`
}

type filterClause struct {
	Field   string      `yaml:"field"`
	Op      string      `yaml:"op"`
	Value   interface{} `yaml:"value"`
	Match   string      `yaml:"match"`
	regexps []*regexp.Regexp
}

// filterClauseList 允许 filter 写成列表或单个映射。
type filterClauseList []filterClause

func (l *filterClauseList) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.SequenceNode:
		var list []filterClause
		if err := node.Decode(&list); err != nil {
			return err
		}
		*l = list
		return nil
	case yaml.MappingNode:
		var one filterClause
		if err := node.Decode(&one); err != nil {
			return err
		}
		*l = filterClauseList{one}
		return nil
	case yaml.ScalarNode:
		if node.Tag == "!!null" {
			*l = nil
			return nil
		}
		return fmt.Errorf("filter 必须是列表或映射")
	default:
		return fmt.Errorf("filter 必须是列表或映射")
	}
}

type ruleDetection struct {
	Filter    filterClauseList `yaml:"filter"`
	GroupBy   []string         `yaml:"group_by"`
	Condition string           `yaml:"condition"`
}

type ruleDefinition struct {
	ID          string        `yaml:"id"`
	Name        string        `yaml:"name"`
	Description string        `yaml:"description"`
	Version     string        `yaml:"version"`
	Author      string        `yaml:"author"`
	Severity    string        `yaml:"severity"`
	Enabled     *bool         `yaml:"enabled"`
	Tags        []string      `yaml:"tags"`
	MITREAttack mitreAttack   `yaml:"mitre_attack"`
	Detection   ruleDetection `yaml:"detection"`
	Actions     ruleActions   `yaml:"actions"`
	Examples    []string      `yaml:"examples"`

	enabled    bool
	condition  *countCondition
	sourcePath string
	sourceName string
	builtin    bool
}

var supportedFilterOps = map[string]struct{}{
	"eq":       {},
	"ne":       {},
	"contains": {},
	"prefix":   {},
	"suffix":   {},
	"regex":    {},
	"one_of":   {},
	"gt":       {},
	"lt":       {},
	"gte":      {},
	"lte":      {},
	"exists":   {},
}

var conditionPattern = regexp.MustCompile(`(?i)^count\s*(>=|<=|==|=|>|<)\s*(\d+)\s*(?:within\s+(\d+)\s*([smhd]))?$`)

type countCondition struct {
	Op        string
	Threshold int64
	Window    time.Duration
}

type resolvedRule struct {
	id        string
	name      string
	severity  string
	def       *ruleDefinition
	meta      *models.DetectionRule
	persisted bool
	err       error
}

// SeedBuiltinRules 将内置 YAML 规则写入存储。已存在的规则仅更新元数据，
// 保留用户设置的 enabled 状态与匹配统计。
func (s *Service) SeedBuiltinRules() error {
	if s.store == nil {
		return errs.Internal("检测规则存储未配置", nil)
	}
	defs, err := loadBuiltinRules()
	if err != nil {
		return err
	}
	if len(defs) == 0 {
		return fmt.Errorf("未找到内置检测规则")
	}

	existing, err := s.store.ListDetectionRules()
	if err != nil {
		return fmt.Errorf("读取现有检测规则失败: %w", err)
	}
	existingByID := make(map[string]models.DetectionRule, len(existing))
	for _, rule := range existing {
		existingByID[rule.ID] = rule
	}

	var failures []string
	for _, def := range defs {
		rule := &models.DetectionRule{
			ID:              def.ID,
			Name:            def.Name,
			Description:     def.Description,
			Severity:        def.Severity,
			Enabled:         def.enabled,
			Tags:            copyStringSlice(def.Tags),
			MITRETactics:    copyStringSlice(def.MITREAttack.Tactics),
			MITRETechniques: copyStringSlice(def.MITREAttack.Techniques),
			FilePath:        def.sourcePath,
			Builtin:         true,
		}
		if old, ok := existingByID[def.ID]; ok {
			rule.Enabled = old.Enabled
			rule.MatchCount = old.MatchCount
			rule.LastMatchedAt = old.LastMatchedAt
		}
		if err := s.store.UpsertDetectionRule(rule); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", def.ID, err))
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("播种内置规则部分失败: %s", strings.Join(failures, "; "))
	}
	return nil
}

// ListRules 列出检测规则。
func (s *Service) ListRules() ([]models.DetectionRule, error) {
	if s.store == nil {
		return nil, errs.Internal("检测规则存储未配置", nil)
	}
	rules, err := s.store.ListDetectionRules()
	if err != nil {
		return nil, fmt.Errorf("获取检测规则失败: %w", err)
	}
	if rules == nil {
		rules = []models.DetectionRule{}
	}
	return rules, nil
}

// SetRuleEnabled 启用或禁用检测规则。
func (s *Service) SetRuleEnabled(id string, enabled bool) error {
	if s.store == nil {
		return errs.Internal("检测规则存储未配置", nil)
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return errs.InvalidInput("规则ID不能为空", errs.ErrInvalidInput)
	}
	if err := s.store.SetDetectionRuleEnabled(id, enabled); err != nil {
		return fmt.Errorf("更新规则启用状态失败: %w", err)
	}
	return nil
}

func parseRuleDefinition(data []byte) (*ruleDefinition, error) {
	var def ruleDefinition
	if err := yaml.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("YAML解析失败: %w", err)
	}
	if err := def.prepare(); err != nil {
		return nil, err
	}
	return &def, nil
}

func (r *ruleDefinition) prepare() error {
	r.ID = strings.TrimSpace(r.ID)
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		return fmt.Errorf("规则名称不能为空")
	}
	r.Severity = normalizeSeverity(r.Severity)
	r.enabled = true
	if r.Enabled != nil {
		r.enabled = *r.Enabled
	}

	groupBy := make([]string, 0, len(r.Detection.GroupBy))
	for _, field := range r.Detection.GroupBy {
		field = strings.TrimSpace(field)
		if field != "" {
			groupBy = append(groupBy, field)
		}
	}
	r.Detection.GroupBy = groupBy

	for i := range r.Detection.Filter {
		clause := &r.Detection.Filter[i]
		clause.Field = strings.TrimSpace(clause.Field)
		if clause.Field == "" {
			return fmt.Errorf("filter[%d].field 不能为空", i)
		}
		clause.Op = strings.ToLower(strings.TrimSpace(clause.Op))
		if clause.Op == "" {
			clause.Op = "eq"
		}
		if _, ok := supportedFilterOps[clause.Op]; !ok {
			return fmt.Errorf("filter[%d].op 不支持: %s", i, clause.Op)
		}
		clause.Match = strings.ToLower(strings.TrimSpace(clause.Match))
		if clause.Match == "" {
			clause.Match = "any"
		}
		if clause.Match != "any" && clause.Match != "all" {
			return fmt.Errorf("filter[%d].match 必须是 any 或 all", i)
		}
		if clause.Op == "regex" {
			for _, pattern := range clause.stringValues() {
				re, err := regexp.Compile(pattern)
				if err != nil {
					return fmt.Errorf("filter[%d] 正则无效: %w", i, err)
				}
				clause.regexps = append(clause.regexps, re)
			}
		}
	}

	condition, err := parseCountCondition(r.Detection.Condition)
	if err != nil {
		return fmt.Errorf("condition 无效: %w", err)
	}
	r.condition = condition

	if r.Actions.RiskScore < 0 || r.Actions.RiskScore > 100 {
		return fmt.Errorf("actions.risk_score 必须在 0 到 100 之间")
	}
	return nil
}

func parseCountCondition(expression string) (*countCondition, error) {
	expr := strings.TrimSpace(expression)
	if expr == "" {
		return &countCondition{Op: ">", Threshold: 0}, nil
	}
	matches := conditionPattern.FindStringSubmatch(expr)
	if matches == nil {
		return nil, fmt.Errorf("无法解析 %q（示例: count > 10 within 5m）", expression)
	}
	threshold, err := strconv.ParseInt(matches[2], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("计数阈值无效: %w", err)
	}
	op := matches[1]
	if op == "=" {
		op = "=="
	}
	condition := &countCondition{Op: op, Threshold: threshold}
	if matches[3] != "" {
		amount, err := strconv.ParseInt(matches[3], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("时间窗数值无效: %w", err)
		}
		var unit time.Duration
		switch strings.ToLower(matches[4]) {
		case "s":
			unit = time.Second
		case "m":
			unit = time.Minute
		case "h":
			unit = time.Hour
		case "d":
			unit = 24 * time.Hour
		default:
			return nil, fmt.Errorf("不支持的时间窗单位: %s", matches[4])
		}
		const maxInt64 = int64(^uint64(0) >> 1)
		if amount > maxInt64/int64(unit) {
			return nil, fmt.Errorf("时间窗过大")
		}
		condition.Window = time.Duration(amount) * unit
	}
	return condition, nil
}

func (c *countCondition) evaluate(count int64) bool {
	switch c.Op {
	case ">":
		return count > c.Threshold
	case ">=":
		return count >= c.Threshold
	case "<":
		return count < c.Threshold
	case "<=":
		return count <= c.Threshold
	case "==":
		return count == c.Threshold
	default:
		return false
	}
}

func (c *filterClause) rawValues() []interface{} {
	switch value := c.Value.(type) {
	case nil:
		return nil
	case []interface{}:
		return value
	case []string:
		out := make([]interface{}, 0, len(value))
		for _, item := range value {
			out = append(out, item)
		}
		return out
	default:
		return []interface{}{value}
	}
}

func (c *filterClause) stringValues() []string {
	raws := c.rawValues()
	out := make([]string, 0, len(raws))
	for _, raw := range raws {
		if raw == nil {
			continue
		}
		switch value := raw.(type) {
		case string:
			out = append(out, value)
		default:
			out = append(out, fmt.Sprint(value))
		}
	}
	return out
}

func (s *Service) selectRules(ruleIDs []string) ([]resolvedRule, error) {
	builtinDefs, err := loadBuiltinRules()
	if err != nil {
		return nil, err
	}
	builtinByID := make(map[string]*ruleDefinition, len(builtinDefs))
	for _, def := range builtinDefs {
		if def.ID != "" {
			builtinByID[def.ID] = def
		}
	}

	var stored []models.DetectionRule
	if s.store != nil {
		stored, err = s.store.ListDetectionRules()
		if err != nil {
			return nil, fmt.Errorf("获取检测规则失败: %w", err)
		}
	}
	storedByID := make(map[string]models.DetectionRule, len(stored))
	for _, rule := range stored {
		if strings.TrimSpace(rule.ID) != "" {
			storedByID[rule.ID] = rule
		}
	}

	ids := make([]string, 0, len(ruleIDs))
	seenIDs := make(map[string]struct{}, len(ruleIDs))
	appendID := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		if _, ok := seenIDs[id]; ok {
			return
		}
		seenIDs[id] = struct{}{}
		ids = append(ids, id)
	}

	if len(ruleIDs) > 0 {
		for _, id := range ruleIDs {
			appendID(id)
		}
	} else if len(stored) > 0 {
		for _, rule := range stored {
			if rule.Enabled {
				appendID(rule.ID)
			}
		}
	} else {
		for _, def := range builtinDefs {
			if def.enabled {
				appendID(def.ID)
			}
		}
	}
	sort.Strings(ids)

	resolved := make([]resolvedRule, 0, len(ids))
	for _, id := range ids {
		storedRule, hasStored := storedByID[id]
		def := builtinByID[id]
		var defErr error
		if def == nil && hasStored {
			def, defErr = loadRuleDefinitionFile(storedRule.FilePath, id)
		}

		item := resolvedRule{id: id, persisted: hasStored}
		if hasStored {
			meta := storedRule
			item.meta = &meta
			item.name = strings.TrimSpace(storedRule.Name)
			item.severity = strings.TrimSpace(storedRule.Severity)
		}
		if def != nil {
			item.def = def
			if item.name == "" {
				item.name = def.Name
			}
			if item.severity == "" {
				item.severity = def.Severity
			}
		} else if !hasStored {
			item.err = fmt.Errorf("规则不存在: %s", id)
		} else if defErr != nil {
			item.err = fmt.Errorf("加载规则 %s 失败: %w", id, defErr)
		} else {
			item.err = fmt.Errorf("规则 %s 缺少可加载的YAML定义", id)
		}
		if item.severity == "" {
			item.severity = "medium"
		}
		if item.name == "" {
			item.name = id
		}
		resolved = append(resolved, item)
	}
	return resolved, nil
}

func loadRuleDefinitionFile(path, fallbackID string) (*ruleDefinition, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("规则文件路径为空")
	}
	if strings.HasPrefix(path, "configs/") || strings.HasPrefix(path, "builtin:") {
		return nil, fmt.Errorf("内置规则必须通过规则ID加载")
	}
	extension := strings.ToLower(filepath.Ext(path))
	if extension != ".yaml" && extension != ".yml" {
		return nil, fmt.Errorf("规则文件必须是 .yaml 或 .yml")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("读取规则文件失败: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("规则文件不能是符号链接")
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("规则路径不是普通文件")
	}
	if info.Size() > maxRuleFileBytes {
		return nil, fmt.Errorf("规则文件超过大小限制（%d 字节）", maxRuleFileBytes)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取规则文件失败: %w", err)
	}
	def, err := parseRuleDefinition(data)
	if err != nil {
		return nil, err
	}
	if def.ID == "" {
		def.ID = fallbackID
	}
	def.sourcePath = path
	def.sourceName = filepath.Base(path)
	return def, nil
}

func normalizeSeverity(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "critical", "crit", "severe":
		return "critical"
	case "high", "major":
		return "high"
	case "low", "info", "informational", "minor":
		return "low"
	case "medium", "med", "moderate", "":
		return "medium"
	default:
		return "medium"
	}
}

func copyStringSlice(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string(nil), values...)
}
