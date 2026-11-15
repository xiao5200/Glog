package query

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"glog/internal/errs"
	"glog/pkg/models"
)

// NodeType 是查询 AST 节点类型。
type NodeType int

const (
	NodeAND NodeType = iota
	NodeOR
	NodeNOT
	NodeTERM
	NodeFIELD
	NodeRANGE
	NodeWILDCARD
	NodePHRASE
	NodeTIME
	NodeALL
)

func (t NodeType) String() string {
	switch t {
	case NodeAND:
		return "AND"
	case NodeOR:
		return "OR"
	case NodeNOT:
		return "NOT"
	case NodeTERM:
		return "TERM"
	case NodeFIELD:
		return "FIELD"
	case NodeRANGE:
		return "RANGE"
	case NodeWILDCARD:
		return "WILDCARD"
	case NodePHRASE:
		return "PHRASE"
	case NodeTIME:
		return "TIME"
	case NodeALL:
		return "ALL"
	default:
		return "UNKNOWN"
	}
}

// ASTNode 是查询语法树节点。
type ASTNode struct {
	Type       NodeType   `json:"type"`
	Value      string     `json:"value,omitempty"`
	Field      string     `json:"field,omitempty"`
	Wildcard   bool       `json:"wildcard,omitempty"`
	RangeStart string     `json:"range_start,omitempty"`
	RangeEnd   string     `json:"range_end,omitempty"`
	TimeStart  *time.Time `json:"time_start,omitempty"`
	TimeEnd    *time.Time `json:"time_end,omitempty"`
	Children   []*ASTNode `json:"children,omitempty"`
	Pos        int        `json:"pos"`
}

// ParsedQuery 是 Parse 的结果，供执行器与测试/调试使用。
type ParsedQuery struct {
	Text    string               `json:"text"`
	AST     *ASTNode             `json:"ast,omitempty"`
	Terms   []string             `json:"terms,omitempty"`
	Filters []models.FieldFilter `json:"filters,omitempty"`
}

// Parse 解析查询文本。空查询返回匹配全集的 AST。
// 非法语法返回包装 errs.ErrQuerySyntaxError 的错误，并包含出错位置。
func Parse(text string) (res *ParsedQuery, err error) {
	defer func() {
		if r := recover(); r != nil {
			res = nil
			err = internalPanic("Parse", r)
		}
	}()

	if strings.TrimSpace(text) == "" {
		return &ParsedQuery{
			Text: text,
			AST:  &ASTNode{Type: NodeALL},
		}, nil
	}

	tokens, lexErr := lexQuery(text)
	if lexErr != nil {
		return nil, lexErr
	}
	p := &parser{tokens: tokens}
	root, parseErr := p.parseOr()
	if parseErr != nil {
		return nil, parseErr
	}
	if t := p.peek(); t.kind != tokenEOF {
		if t.kind == tokenRParen {
			return nil, syntaxError(t.pos, "多余的右括号")
		}
		return nil, syntaxError(t.pos, "无法识别的查询内容")
	}
	pq := &ParsedQuery{Text: text, AST: root}
	collectQueryMeta(root, pq)
	return pq, nil
}

func syntaxError(pos int, msg string) error {
	if pos < 0 {
		pos = 0
	}
	return fmt.Errorf("%w: %s（位置 %d）", errs.ErrQuerySyntaxError, msg, pos+1)
}

// ---------------------------------------------------------------------------
// 词法分析
// ---------------------------------------------------------------------------

type tokenKind uint8

const (
	tokenEOF tokenKind = iota
	tokenWord
	tokenPhrase
	tokenLParen
	tokenRParen
)

type lexToken struct {
	kind   tokenKind
	text   string
	pos    int
	quoted bool
}

func lexQuery(input string) ([]lexToken, error) {
	tokens := make([]lexToken, 0, 16)
	i := 0
	for i < len(input) {
		c := input[i]
		if isSpaceByte(c) {
			i++
			continue
		}
		switch c {
		case '(':
			tokens = append(tokens, lexToken{kind: tokenLParen, pos: i})
			i++
		case ')':
			tokens = append(tokens, lexToken{kind: tokenRParen, pos: i})
			i++
		case '"':
			start := i
			value, next, err := scanQuoted(input, i)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, lexToken{kind: tokenPhrase, text: value, pos: start, quoted: true})
			i = next
		default:
			tok, next, err := scanWord(input, i)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, tok)
			i = next
		}
	}
	tokens = append(tokens, lexToken{kind: tokenEOF, pos: len(input)})
	return tokens, nil
}

// scanQuoted 扫描以 " 开头的短语，返回去掉引号后的内容与下一个位置。
func scanQuoted(input string, start int) (string, int, error) {
	var b strings.Builder
	i := start + 1
	for i < len(input) {
		ch := input[i]
		if ch == '\\' {
			if i+1 < len(input) && (input[i+1] == '"' || input[i+1] == '\\') {
				b.WriteByte(input[i+1])
				i += 2
				continue
			}
			b.WriteByte(ch)
			i++
			continue
		}
		if ch == '"' {
			return b.String(), i + 1, nil
		}
		b.WriteByte(ch)
		i++
	}
	return "", i, syntaxError(start, "引号未闭合")
}

// scanWord 扫描一个裸词。词内允许出现 [ ... ] 范围（可包含空格）与内嵌引号。
func scanWord(input string, start int) (lexToken, int, error) {
	var b strings.Builder
	i := start
	quoted := false
	inBracket := false
	bracketStart := -1
	for i < len(input) {
		c := input[i]
		if inBracket {
			b.WriteByte(c)
			if c == ']' {
				inBracket = false
				bracketStart = -1
			}
			i++
			continue
		}
		if c == '[' {
			inBracket = true
			bracketStart = i
			b.WriteByte(c)
			i++
			continue
		}
		if c == '"' {
			quotePos := i
			quoted = true
			value, next, err := scanQuoted(input, i)
			if err != nil {
				return lexToken{}, i, syntaxError(quotePos, "引号未闭合")
			}
			b.WriteString(value)
			i = next
			continue
		}
		if isSpaceByte(c) || c == '(' || c == ')' {
			break
		}
		b.WriteByte(c)
		i++
	}
	if inBracket {
		return lexToken{}, i, syntaxError(bracketStart, "范围括号未闭合")
	}
	return lexToken{kind: tokenWord, text: b.String(), pos: start, quoted: quoted}, i, nil
}

func isSpaceByte(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\r', '\v', '\f':
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// 语法分析（优先级：NOT > AND > OR；相邻表达式为隐式 AND）
// ---------------------------------------------------------------------------

type parser struct {
	tokens []lexToken
	pos    int
}

func (p *parser) peek() lexToken {
	if p.pos < 0 || p.pos >= len(p.tokens) {
		return lexToken{kind: tokenEOF}
	}
	return p.tokens[p.pos]
}

func (p *parser) next() lexToken {
	t := p.peek()
	if t.kind != tokenEOF {
		p.pos++
	}
	return t
}

func (p *parser) parseOr() (*ASTNode, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		if t.kind != tokenWord || !strings.EqualFold(t.text, "OR") {
			break
		}
		op := p.next()
		if isExprBoundary(p.peek()) {
			return nil, syntaxError(op.pos, "OR 后缺少表达式")
		}
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = &ASTNode{Type: NodeOR, Children: []*ASTNode{left, right}, Pos: op.pos}
	}
	return left, nil
}

func (p *parser) parseAnd() (*ASTNode, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		if t.kind == tokenEOF || t.kind == tokenRParen {
			break
		}
		if t.kind == tokenWord && strings.EqualFold(t.text, "OR") {
			break
		}
		if t.kind == tokenWord && strings.EqualFold(t.text, "AND") {
			op := p.next()
			if isExprBoundary(p.peek()) {
				return nil, syntaxError(op.pos, "AND 后缺少表达式")
			}
			right, err := p.parseUnary()
			if err != nil {
				return nil, err
			}
			left = &ASTNode{Type: NodeAND, Children: []*ASTNode{left, right}, Pos: op.pos}
			continue
		}
		// 相邻表达式：隐式 AND。
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = &ASTNode{Type: NodeAND, Children: []*ASTNode{left, right}, Pos: t.pos}
	}
	return left, nil
}

func (p *parser) parseUnary() (*ASTNode, error) {
	t := p.peek()
	if t.kind == tokenWord && strings.EqualFold(t.text, "NOT") {
		op := p.next()
		if isExprBoundary(p.peek()) {
			return nil, syntaxError(op.pos, "NOT 后缺少表达式")
		}
		child, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &ASTNode{Type: NodeNOT, Children: []*ASTNode{child}, Pos: op.pos}, nil
	}
	return p.parsePrimary()
}

func (p *parser) parsePrimary() (*ASTNode, error) {
	t := p.peek()
	switch t.kind {
	case tokenEOF:
		return nil, syntaxError(t.pos, "缺少查询表达式")
	case tokenRParen:
		return nil, syntaxError(t.pos, "多余的右括号")
	case tokenLParen:
		open := p.next()
		if nt := p.peek(); nt.kind == tokenRParen || nt.kind == tokenEOF {
			return nil, syntaxError(open.pos, "括号内缺少表达式")
		}
		expr, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if closeTok := p.peek(); closeTok.kind != tokenRParen {
			return nil, syntaxError(open.pos, "缺少右括号")
		}
		p.next()
		return expr, nil
	case tokenPhrase:
		p.next()
		if t.text == "" {
			return nil, syntaxError(t.pos, "短语不能为空")
		}
		return &ASTNode{Type: NodePHRASE, Value: t.text, Pos: t.pos}, nil
	case tokenWord:
		p.next()
		if strings.EqualFold(t.text, "AND") || strings.EqualFold(t.text, "OR") {
			return nil, syntaxError(t.pos, "意外的逻辑运算符")
		}
		return p.classifyWord(t)
	default:
		return nil, syntaxError(t.pos, "无法识别的查询内容")
	}
}

func isExprBoundary(t lexToken) bool {
	if t.kind == tokenEOF || t.kind == tokenRParen {
		return true
	}
	if t.kind == tokenWord && (strings.EqualFold(t.text, "AND") || strings.EqualFold(t.text, "OR")) {
		return true
	}
	return false
}

func (p *parser) classifyWord(tok lexToken) (*ASTNode, error) {
	word := tok.text
	if word == "" {
		return nil, syntaxError(tok.pos, "空查询词")
	}
	if field, value, ok := splitFieldExpr(word); ok {
		return p.buildFieldNode(field, value, tok)
	}
	if !tok.quoted && strings.Contains(word, "*") {
		if word == "*" {
			return &ASTNode{Type: NodeALL, Pos: tok.pos}, nil
		}
		if strings.HasSuffix(word, "*") && strings.Count(word, "*") == 1 {
			prefix := strings.TrimSuffix(word, "*")
			if prefix == "" {
				return &ASTNode{Type: NodeALL, Pos: tok.pos}, nil
			}
			return &ASTNode{Type: NodeWILDCARD, Value: prefix, Pos: tok.pos}, nil
		}
		return nil, syntaxError(tok.pos, "通配符仅支持后缀形式（如 error*）")
	}
	return &ASTNode{Type: NodeTERM, Value: word, Pos: tok.pos}, nil
}

// splitFieldExpr 判断 token 是否为 field:value。仅识别已知字段与 fields.xxx，
// 其余带冒号的词按普通关键词处理（例如 URL、IPv6、MAC、host:port）。
func splitFieldExpr(word string) (string, string, bool) {
	idx := strings.IndexByte(word, ':')
	if idx <= 0 {
		return "", "", false
	}
	field := word[:idx]
	value := word[idx+1:]
	// 仅已知字段才按字段查询解析。未知前缀（http、IPv6、盘符、host:port 等）
	// 保持为普通关键词，因此不能让值中的 "://" / "::" 影响已知字段的解析。
	if isKnownQueryField(field) || strings.HasPrefix(strings.ToLower(field), "fields.") {
		return field, value, true
	}
	return "", "", false
}

func isRangeValue(value string) bool {
	return len(value) >= 2 && value[0] == '[' && value[len(value)-1] == ']'
}

func (p *parser) buildFieldNode(field, value string, tok lexToken) (*ASTNode, error) {
	canonical := canonicalFieldName(field)

	if canonical == "@timestamp" {
		if isRangeValue(value) {
			return p.buildTimeRangeNode(value, tok)
		}
		if strings.HasPrefix(value, "[") || strings.HasSuffix(value, "]") {
			return nil, syntaxError(tok.pos, "范围格式错误，应为 [开始 TO 结束]")
		}
		if value == "" {
			return nil, syntaxError(tok.pos, "时间字段值不能为空")
		}
		start, end, err := parseTimestampEquality(value)
		if err != nil {
			return nil, syntaxError(tok.pos, "无效的时间值: "+err.Error())
		}
		return &ASTNode{
			Type:      NodeTIME,
			Field:     canonical,
			TimeStart: &start,
			TimeEnd:   &end,
			Pos:       tok.pos,
		}, nil
	}

	if strings.HasPrefix(value, "[") || strings.HasSuffix(value, "]") {
		if !isRangeValue(value) {
			return nil, syntaxError(tok.pos, "范围格式错误，应为 [开始 TO 结束]")
		}
		start, end, ok := splitRangeInner(value)
		if !ok {
			return nil, syntaxError(tok.pos, "范围格式错误，应为 [开始 TO 结束]")
		}
		return &ASTNode{
			Type:       NodeRANGE,
			Field:      canonical,
			RangeStart: start,
			RangeEnd:   end,
			Pos:        tok.pos,
		}, nil
	}

	if value == "" {
		return nil, syntaxError(tok.pos, "字段值不能为空")
	}

	wildcard := false
	if !tok.quoted && strings.Contains(value, "*") {
		if strings.HasSuffix(value, "*") && strings.Count(value, "*") == 1 {
			wildcard = true
			value = strings.TrimSuffix(value, "*")
		} else {
			return nil, syntaxError(tok.pos, "通配符仅支持后缀形式")
		}
	}

	return &ASTNode{
		Type:     NodeFIELD,
		Field:    canonical,
		Value:    value,
		Wildcard: wildcard,
		Pos:      tok.pos,
	}, nil
}

// splitRangeInner 在 [ ... ] 中按独立单词 TO（大小写不敏感、两侧需空白）切分。
func splitRangeInner(value string) (string, string, bool) {
	if len(value) < 2 || value[0] != '[' || value[len(value)-1] != ']' {
		return "", "", false
	}
	inner := value[1 : len(value)-1]
	for i := 0; i+2 <= len(inner); i++ {
		if i == 0 || !isSpaceByte(inner[i-1]) {
			continue
		}
		if inner[i] != 't' && inner[i] != 'T' {
			continue
		}
		if inner[i+1] != 'o' && inner[i+1] != 'O' {
			continue
		}
		if i+2 >= len(inner) || !isSpaceByte(inner[i+2]) {
			continue
		}
		left := strings.TrimSpace(inner[:i])
		right := strings.TrimSpace(inner[i+2:])
		return left, right, true
	}
	return "", "", false
}

func (p *parser) buildTimeRangeNode(value string, tok lexToken) (*ASTNode, error) {
	startRaw, endRaw, ok := splitRangeInner(value)
	if !ok {
		return nil, syntaxError(tok.pos, "时间范围格式应为 [开始 TO 结束]")
	}

	var startPtr *time.Time
	var endPtr *time.Time

	if !rangeEndpointOpen(startRaw) {
		t, _, err := parseTimeBound(startRaw, false)
		if err != nil {
			return nil, syntaxError(tok.pos, "无效的时间起点: "+err.Error())
		}
		startPtr = &t
	}
	if !rangeEndpointOpen(endRaw) {
		t, _, err := parseTimeBound(endRaw, true)
		if err != nil {
			return nil, syntaxError(tok.pos, "无效的时间终点: "+err.Error())
		}
		endPtr = &t
	}
	if startPtr != nil && endPtr != nil && startPtr.After(*endPtr) {
		return nil, syntaxError(tok.pos, "时间范围起点晚于终点")
	}
	if startPtr == nil && endPtr == nil {
		return &ASTNode{Type: NodeALL, Pos: tok.pos}, nil
	}
	return &ASTNode{
		Type:      NodeTIME,
		Field:     "@timestamp",
		TimeStart: startPtr,
		TimeEnd:   endPtr,
		Pos:       tok.pos,
	}, nil
}

// parseTimestampEquality 解析 @timestamp:value。纯日期匹配当天 00:00:00 到 23:59:59.999999999。
func parseTimestampEquality(raw string) (time.Time, time.Time, error) {
	t, dateOnly, err := parseTimeBound(raw, false)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if dateOnly {
		end := t.AddDate(0, 0, 1).Add(-time.Nanosecond)
		return t, end, nil
	}
	return t, t, nil
}

// parseTimeBound 解析时间边界。dateOnly 为 true 表示仅日期。
// 仅日期作为终点时扩展到当天最后一纳秒；无时区时间按本地时区解释。
func parseTimeBound(raw string, isEnd bool) (time.Time, bool, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return time.Time{}, false, fmt.Errorf("时间为空")
	}
	if strings.EqualFold(s, "now") {
		return time.Now(), false, nil
	}

	dateLayouts := []string{"2006-01-02", "2006-1-2", "2006/01/02", "20060102"}
	for _, layout := range dateLayouts {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			start := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
			if isEnd {
				return start.AddDate(0, 0, 1).Add(-time.Nanosecond), true, nil
			}
			return start, true, nil
		}
	}

	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, false, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, false, nil
	}

	localLayouts := []string{
		"2006-01-02T15:04:05.999999999",
		"2006-01-02T15:04:05",
		"2006-1-2T15:04:05",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
		"2006-1-2 15:04:05",
		"2006-01-02 15:04",
		"2006/01/02 15:04:05",
	}
	for _, layout := range localLayouts {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, false, nil
		}
	}

	if isAllDigits(s) {
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			switch len(s) {
			case 10:
				return time.Unix(n, 0), false, nil
			case 13:
				return time.UnixMilli(n), false, nil
			}
		}
	}
	return time.Time{}, false, fmt.Errorf("无法解析时间 %q", raw)
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// 调试用元数据提取
// ---------------------------------------------------------------------------

func collectQueryMeta(n *ASTNode, pq *ParsedQuery) {
	if n == nil || pq == nil {
		return
	}
	switch n.Type {
	case NodeTERM, NodeWILDCARD, NodePHRASE:
		pq.Terms = append(pq.Terms, n.Value)
	case NodeFIELD:
		op := "eq"
		if n.Wildcard {
			op = "prefix"
		}
		pq.Filters = append(pq.Filters, models.FieldFilter{
			Field:    n.Field,
			Operator: op,
			Value:    n.Value,
		})
	case NodeRANGE:
		pq.Filters = append(pq.Filters, models.FieldFilter{
			Field:    n.Field,
			Operator: "range",
			Value:    []string{n.RangeStart, n.RangeEnd},
		})
	}
	for _, child := range n.Children {
		collectQueryMeta(child, pq)
	}
}
