package query

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/RoaringBitmap/roaring"

	"glog/pkg/models"
)

// Search 执行查询并返回分页结果。
func (e *Engine) Search(q models.Query) (res *models.QueryResult, err error) {
	defer func() {
		if r := recover(); r != nil {
			res = nil
			err = internalPanic("Search", r)
		}
	}()
	if err = e.checkReady(); err != nil {
		return nil, err
	}

	started := time.Now()
	matched, matchErr := e.matchDocs(q)
	if matchErr != nil {
		return nil, matchErr
	}
	total := int64(matched.GetCardinality())

	limit := q.Limit
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	offset := q.Offset
	if offset < 0 {
		offset = 0
	}

	if total == 0 || int64(offset) >= total {
		return &models.QueryResult{
			Total:   total,
			Hits:    []models.LogEntry{},
			Took:    time.Since(started).Milliseconds(),
			HasMore: false,
		}, nil
	}

	ids := e.sortMatchedIDs(matched, q.SortBy, q.SortOrder)
	if offset >= len(ids) {
		return &models.QueryResult{
			Total:   total,
			Hits:    []models.LogEntry{},
			Took:    time.Since(started).Milliseconds(),
			HasMore: false,
		}, nil
	}
	end := offset + limit
	if end > len(ids) {
		end = len(ids)
	}
	pageIDs := ids[offset:end]
	hits := e.readPage(pageIDs)
	return &models.QueryResult{
		Total:   total,
		Hits:    hits,
		Took:    time.Since(started).Milliseconds(),
		HasMore: int64(end) < total,
	}, nil
}

// matchDocs 合并 Text AST 与 Query 结构化筛选，返回匹配文档 bitmap。
// 无法走索引的条件（短语、fields.* 等）在候选集上逐条读取过滤。
func (e *Engine) matchDocs(q models.Query) (*roaring.Bitmap, error) {
	pq, err := Parse(q.Text)
	if err != nil {
		return nil, err
	}

	all := e.allDocsBitmap()
	universe := all.Clone()

	if q.StartTime != nil || q.EndTime != nil {
		universe = e.evalTimeBounds(q.StartTime, q.EndTime, universe)
		if universe.IsEmpty() {
			return universe, nil
		}
	}

	var conditions []*roaring.Bitmap
	if len(q.Levels) > 0 {
		bm := roaring.New()
		for _, level := range q.Levels {
			bm.Or(e.fieldValueBitmap("level", level))
		}
		conditions = append(conditions, bm)
	}
	if len(q.Hosts) > 0 {
		bm := roaring.New()
		for _, host := range q.Hosts {
			bm.Or(e.fieldValueBitmap("source_host", host))
		}
		conditions = append(conditions, bm)
	}
	if len(q.Sources) > 0 {
		bm := roaring.New()
		for _, source := range q.Sources {
			bm.Or(e.fieldValueBitmap("source", source))
		}
		conditions = append(conditions, bm)
	}
	if len(q.EventIDs) > 0 {
		bm := roaring.New()
		for _, eventID := range q.EventIDs {
			bm.Or(e.fieldValueBitmap("event_id", strconv.FormatUint(uint64(eventID), 10)))
		}
		conditions = append(conditions, bm)
	}
	universe = intersectBitmaps(universe, conditions)
	if universe.IsEmpty() {
		return universe, nil
	}

	var filterBitmaps []*roaring.Bitmap
	var residual []models.FieldFilter
	for _, ff := range q.Fields {
		if strings.TrimSpace(ff.Field) == "" {
			continue
		}
		bm, handled := e.fieldFilterBitmap(ff, all)
		if handled {
			filterBitmaps = append(filterBitmaps, bm)
		} else {
			residual = append(residual, ff)
		}
	}
	universe = intersectBitmaps(universe, filterBitmaps)
	if universe.IsEmpty() {
		return universe, nil
	}

	if pq.AST != nil {
		universe = e.evalNode(pq.AST, universe)
		if universe.IsEmpty() {
			return universe, nil
		}
	}

	if len(residual) > 0 {
		preds := make([]docPredicate, 0, len(residual))
		for _, ff := range residual {
			pred, predErr := buildFieldFilterPredicate(ff)
			if predErr != nil {
				return nil, predErr
			}
			preds = append(preds, pred)
		}
		combined := func(entry *models.LogEntry) bool {
			for _, pred := range preds {
				if !pred(entry) {
					return false
				}
			}
			return true
		}
		universe = e.scanPredicate(universe, combined)
	}
	return universe, nil
}

func (e *Engine) allDocsBitmap() *roaring.Bitmap {
	return bitmapFromIDs(e.idx.AllDocIDs())
}

// fieldFilterBitmap 对可用索引的 Query.Fields 过滤器直接生成 bitmap。
// 返回 handled=false 表示需要退化为逐条谓词过滤。
func (e *Engine) fieldFilterBitmap(ff models.FieldFilter, all *roaring.Bitmap) (*roaring.Bitmap, bool) {
	field := canonicalFieldName(ff.Field)
	op := strings.ToLower(strings.TrimSpace(ff.Operator))
	if op == "" {
		op = "eq"
	}
	values := valuesFromFilter(ff.Value)

	switch field {
	case "level", "source_host", "source", "event_id",
		"event_type", "source_ip", "destination_ip":
		switch op {
		case "eq", "=", "==", "in":
			return e.fieldValuesBitmapUnion(field, values), true
		case "ne", "!=", "<>", "not_in", "nin":
			eq := e.fieldValuesBitmapUnion(field, values)
			ne := all.Clone()
			ne.AndNot(eq)
			return ne, true
		}
	case "id":
		switch op {
		case "eq", "=", "==", "in":
			return e.idBitmap(values, all), true
		case "ne", "!=", "<>", "not_in", "nin":
			eq := e.idBitmap(values, all)
			ne := all.Clone()
			ne.AndNot(eq)
			return ne, true
		}
	}
	return nil, false
}

func (e *Engine) fieldValuesBitmapUnion(field string, values []interface{}) *roaring.Bitmap {
	out := roaring.New()
	for _, value := range values {
		out.Or(e.fieldValueBitmap(field, valueToString(value)))
	}
	return out
}

func (e *Engine) idBitmap(values []interface{}, all *roaring.Bitmap) *roaring.Bitmap {
	out := roaring.New()
	for _, value := range values {
		s := strings.TrimSpace(valueToString(value))
		id, err := strconv.ParseUint(s, 10, 32)
		if err != nil {
			continue
		}
		out.Add(uint32(id))
	}
	if all != nil {
		out.And(all)
	}
	return out
}

// ---------------------------------------------------------------------------
// AST 执行
// ---------------------------------------------------------------------------

func (e *Engine) evalNode(n *ASTNode, universe *roaring.Bitmap) *roaring.Bitmap {
	if universe == nil {
		universe = roaring.New()
	}
	if n == nil {
		return universe.Clone()
	}
	if universe.IsEmpty() {
		return roaring.New()
	}

	switch n.Type {
	case NodeALL:
		return universe.Clone()
	case NodeAND:
		return e.evalAnd(n.Children, universe)
	case NodeOR:
		out := roaring.New()
		for _, child := range n.Children {
			out.Or(e.evalNode(child, universe))
		}
		return out
	case NodeNOT:
		if len(n.Children) == 0 {
			return universe.Clone()
		}
		child := e.evalNode(n.Children[0], universe)
		out := universe.Clone()
		out.AndNot(child)
		return out
	case NodeTERM:
		return e.evalTerm(n.Value, universe)
	case NodePHRASE:
		return e.evalPhrase(n.Value, universe)
	case NodeWILDCARD:
		return e.evalWildcard(n.Value, universe)
	case NodeFIELD:
		return e.evalFieldNode(n, universe)
	case NodeRANGE:
		return e.evalRangeNode(n, universe)
	case NodeTIME:
		return e.evalTimeNode(n, universe)
	default:
		return universe.Clone()
	}
}

// evalAnd 先执行可用索引的廉价条件（按结果基数升序求交），
// 再在缩小后的候选集上执行 NOT/OR/短语/动态字段等复杂条件。
func (e *Engine) evalAnd(children []*ASTNode, universe *roaring.Bitmap) *roaring.Bitmap {
	if len(children) == 0 {
		return universe.Clone()
	}
	cheap := make([]*roaring.Bitmap, 0, len(children))
	complex := make([]*ASTNode, 0, len(children))
	for _, child := range children {
		if isCheapIndexNode(child) {
			bm := e.evalNode(child, universe)
			if bm.IsEmpty() {
				return bm
			}
			cheap = append(cheap, bm)
		} else {
			complex = append(complex, child)
		}
	}
	cur := intersectBitmaps(universe, cheap)
	for _, child := range complex {
		if cur.IsEmpty() {
			break
		}
		cur.And(e.evalNode(child, cur))
	}
	return cur
}

func isCheapIndexNode(n *ASTNode) bool {
	if n == nil {
		return true
	}
	switch n.Type {
	case NodeTERM, NodeWILDCARD, NodeTIME, NodeALL:
		return true
	case NodeFIELD:
		switch canonicalFieldName(n.Field) {
		case "level", "source_host", "source", "event_id", "id", "@timestamp",
			"event_type", "source_ip", "destination_ip":
			return true
		}
	case NodeRANGE:
		switch canonicalFieldName(n.Field) {
		case "level", "source_host", "source", "event_id", "@timestamp":
			return true
		}
	}
	return false
}

func (e *Engine) evalTerm(text string, universe *roaring.Bitmap) *roaring.Bitmap {
	tokens := dedupeStrings(e.idx.Tokenize(text))
	if len(tokens) == 0 {
		return roaring.New()
	}
	return e.intersectTokens(tokens, universe)
}

func (e *Engine) evalWildcard(prefix string, universe *roaring.Bitmap) *roaring.Bitmap {
	if strings.TrimSpace(prefix) == "" {
		return universe.Clone()
	}
	return e.prefixPostingsBitmap(prefix, universe)
}

func (e *Engine) evalPhrase(phrase string, universe *roaring.Bitmap) *roaring.Bitmap {
	candidate := e.candidateForText(phrase, universe)
	needle := strings.ToLower(phrase)
	pred := func(entry *models.LogEntry) bool {
		if strings.Contains(strings.ToLower(entry.Message), needle) {
			return true
		}
		return strings.Contains(strings.ToLower(entry.RawLog), needle)
	}
	return e.scanPredicate(candidate, pred)
}

func (e *Engine) evalFieldNode(n *ASTNode, universe *roaring.Bitmap) *roaring.Bitmap {
	if n == nil {
		return universe.Clone()
	}
	field := canonicalFieldName(n.Field)
	switch field {
	case "level", "source_host", "source", "event_id",
		"event_type", "source_ip", "destination_ip":
		return e.evalIndexedFieldNode(field, n.Value, n.Wildcard, universe)
	case "id":
		if n.Wildcard {
			prefix := n.Value
			pred := func(entry *models.LogEntry) bool {
				if entry == nil {
					return false
				}
				return hasPrefixFold(strconv.FormatUint(entry.DocID, 10), prefix)
			}
			return e.scanPredicate(universe, pred)
		}
		id, err := strconv.ParseUint(strings.TrimSpace(n.Value), 10, 32)
		if err != nil {
			return roaring.New()
		}
		bm := roaring.New()
		bm.Add(uint32(id))
		bm.And(universe)
		return bm
	case "@timestamp":
		start, end, err := parseTimestampEquality(n.Value)
		if err != nil {
			return roaring.New()
		}
		return e.evalTimeBounds(&start, &end, universe)
	case "message", "raw_log":
		return e.evalContainsField(field, n.Value, n.Wildcard, universe)
	}

	if n.Wildcard {
		prefix := n.Value
		pred := func(entry *models.LogEntry) bool {
			value, ok := extractFieldValue(entry, field)
			if !ok {
				return false
			}
			return hasPrefixFold(valueToString(value), prefix)
		}
		return e.scanPredicate(universe, pred)
	}

	want := n.Value
	pred := func(entry *models.LogEntry) bool {
		value, ok := extractFieldValue(entry, field)
		if !ok {
			return false
		}
		return valueEquals(value, want)
	}
	return e.scanPredicate(universe, pred)
}

func (e *Engine) evalIndexedFieldNode(field, value string, wildcard bool, universe *roaring.Bitmap) *roaring.Bitmap {
	if wildcard {
		if bm, ok := e.fieldValuePrefixBitmap(field, value); ok {
			bm.And(universe)
			return bm
		}
		prefix := value
		pred := func(entry *models.LogEntry) bool {
			fieldValue, ok := extractFieldValue(entry, field)
			if !ok {
				return false
			}
			return hasPrefixFold(valueToString(fieldValue), prefix)
		}
		return e.scanPredicate(universe, pred)
	}
	bm := e.fieldValueBitmap(field, value)
	bm.And(universe)
	return bm
}

func (e *Engine) evalContainsField(field, value string, prefix bool, universe *roaring.Bitmap) *roaring.Bitmap {
	var candidate *roaring.Bitmap
	if prefix {
		candidate = e.prefixPostingsBitmap(value, universe)
	} else {
		candidate = e.candidateForText(value, universe)
	}
	pred := func(entry *models.LogEntry) bool {
		got, ok := extractFieldValue(entry, field)
		if !ok {
			return false
		}
		s := valueToString(got)
		if prefix {
			return hasPrefixFold(s, value)
		}
		return containsFold(s, value)
	}
	return e.scanPredicate(candidate, pred)
}

func (e *Engine) evalRangeNode(n *ASTNode, universe *roaring.Bitmap) *roaring.Bitmap {
	if n == nil {
		return universe.Clone()
	}
	field := canonicalFieldName(n.Field)

	if field == "@timestamp" {
		var startPtr *time.Time
		var endPtr *time.Time
		if !rangeEndpointOpen(n.RangeStart) {
			t, _, err := parseTimeBound(n.RangeStart, false)
			if err != nil {
				return roaring.New()
			}
			startPtr = &t
		}
		if !rangeEndpointOpen(n.RangeEnd) {
			t, _, err := parseTimeBound(n.RangeEnd, true)
			if err != nil {
				return roaring.New()
			}
			endPtr = &t
		}
		return e.evalTimeBounds(startPtr, endPtr, universe)
	}

	switch field {
	case "level", "source_host", "source", "event_id":
		if bm, ok := e.rangeValueBitmap(field, n.RangeStart, n.RangeEnd); ok {
			bm.And(universe)
			return bm
		}
	}

	startRaw, endRaw := n.RangeStart, n.RangeEnd
	pred := func(entry *models.LogEntry) bool {
		value, ok := extractFieldValue(entry, field)
		if !ok {
			return false
		}
		if field == "level" || field == "source_host" || field == "source" || field == "event_id" {
			return indexedValueInRange(field, valueToString(value), startRaw, endRaw)
		}
		return valueInRange(value, startRaw, endRaw)
	}
	return e.scanPredicate(universe, pred)
}

func (e *Engine) rangeValueBitmap(field, startRaw, endRaw string) (*roaring.Bitmap, bool) {
	values := e.idx.FieldValues(field)
	if len(values) == 0 {
		return nil, false
	}
	out := roaring.New()
	for _, value := range values {
		if !indexedValueInRange(field, value, startRaw, endRaw) {
			continue
		}
		out.Or(bitmapFromIDs(e.idx.FieldBitmap(field, value)))
	}
	return out, true
}

func (e *Engine) evalTimeNode(n *ASTNode, universe *roaring.Bitmap) *roaring.Bitmap {
	return e.evalTimeBounds(n.TimeStart, n.TimeEnd, universe)
}

func (e *Engine) evalTimeBounds(start, end *time.Time, universe *roaring.Bitmap) *roaring.Bitmap {
	if start == nil && end == nil {
		return universe.Clone()
	}
	startTime := minTimeBound
	endTime := maxTimeBound
	if start != nil {
		startTime = *start
	}
	if end != nil {
		endTime = *end
	}
	if startTime.After(endTime) {
		return roaring.New()
	}
	bm := bitmapFromIDs(e.idx.TimeRange(startTime, endTime))
	bm.And(universe)
	return bm
}

// ---------------------------------------------------------------------------
// 字段 bitmap / 词项 bitmap
// ---------------------------------------------------------------------------

// fieldValueBitmap 以大小写不敏感方式解析枚举字段值。
// level 直接使用大写值；其余字段通过 FieldValues + FieldBitmap 合并大小写变体。
func (e *Engine) fieldValueBitmap(field, value string) *roaring.Bitmap {
	field = canonicalFieldName(field)
	directValue := value
	if field == "level" {
		directValue = strings.ToUpper(strings.TrimSpace(value))
	}
	out := bitmapFromIDs(e.idx.FieldBitmap(field, directValue))
	for _, candidate := range e.idx.FieldValues(field) {
		if strings.EqualFold(candidate, value) {
			out.Or(bitmapFromIDs(e.idx.FieldBitmap(field, candidate)))
		}
	}
	return out
}

func (e *Engine) fieldValuePrefixBitmap(field, prefix string) (*roaring.Bitmap, bool) {
	values := e.idx.FieldValues(field)
	if len(values) == 0 {
		return nil, false
	}
	out := roaring.New()
	for _, value := range values {
		if hasPrefixFold(value, prefix) {
			out.Or(bitmapFromIDs(e.idx.FieldBitmap(field, value)))
		}
	}
	return out, true
}

func (e *Engine) intersectTokens(tokens []string, universe *roaring.Bitmap) *roaring.Bitmap {
	if len(tokens) == 0 {
		return roaring.New()
	}
	if universe == nil {
		universe = roaring.New()
	}
	postings := make([][]uint32, 0, len(tokens))
	for _, token := range tokens {
		ids := e.idx.Postings(token)
		if len(ids) == 0 {
			return roaring.New()
		}
		postings = append(postings, ids)
	}
	sort.Slice(postings, func(i, j int) bool {
		return len(postings[i]) < len(postings[j])
	})
	out := bitmapFromIDs(postings[0])
	out.And(universe)
	for _, ids := range postings[1:] {
		if out.IsEmpty() {
			return out
		}
		out.And(bitmapFromIDs(ids))
	}
	return out
}

func (e *Engine) candidateForText(text string, universe *roaring.Bitmap) *roaring.Bitmap {
	tokens := dedupeStrings(e.idx.Tokenize(text))
	if len(tokens) == 0 {
		return universe.Clone()
	}
	return e.intersectTokens(tokens, universe)
}

func (e *Engine) prefixPostingsBitmap(prefix string, universe *roaring.Bitmap) *roaring.Bitmap {
	if universe == nil || universe.IsEmpty() {
		return roaring.New()
	}
	if strings.TrimSpace(prefix) == "" {
		return universe.Clone()
	}
	tokens := dedupeStrings(e.idx.Tokenize(prefix))
	if len(tokens) == 0 {
		ids := e.idx.PostingsPrefix(prefix)
		if len(ids) == 0 {
			if lower := strings.ToLower(prefix); lower != prefix {
				ids = e.idx.PostingsPrefix(lower)
			}
		}
		bm := bitmapFromIDs(ids)
		bm.And(universe)
		return bm
	}

	var bm *roaring.Bitmap
	if len(tokens) == 1 {
		bm = bitmapFromIDs(e.idx.PostingsPrefix(tokens[0]))
	} else {
		bm = e.intersectTokens(tokens[:len(tokens)-1], universe)
		bm.And(bitmapFromIDs(e.idx.PostingsPrefix(tokens[len(tokens)-1])))
	}
	bm.And(universe)
	return bm
}

func dedupeStrings(values []string) []string {
	if len(values) <= 1 {
		return values
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

// ---------------------------------------------------------------------------
// 排序
// ---------------------------------------------------------------------------

func (e *Engine) sortMatchedIDs(bm *roaring.Bitmap, sortBy, sortOrder string) []uint32 {
	ids := bm.ToArray()
	if len(ids) <= 1 {
		return ids
	}
	asc := sortOrderAscending(sortOrder)
	switch strings.ToLower(strings.TrimSpace(sortBy)) {
	case "", "timestamp", "time":
		return e.sortByTimestamp(ids, asc)
	case "level":
		return e.sortByLevel(ids, bm, asc)
	case "doc_id", "docid", "id":
		if asc {
			return ids
		}
		out := make([]uint32, len(ids))
		for i := range ids {
			out[len(ids)-1-i] = ids[i]
		}
		return out
	default:
		// 其他字段回退 timestamp desc。
		return e.sortByTimestamp(ids, false)
	}
}

func sortOrderAscending(order string) bool {
	switch strings.ToLower(strings.TrimSpace(order)) {
	case "asc", "ascending":
		return true
	default:
		return false
	}
}

func (e *Engine) sortByTimestamp(ids []uint32, asc bool) []uint32 {
	type item struct {
		id uint32
		ts time.Time
	}
	items := make([]item, len(ids))
	for i, id := range ids {
		items[i] = item{id: id, ts: e.idx.Timestamp(id)}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].ts.Equal(items[j].ts) {
			return items[i].id < items[j].id
		}
		if asc {
			return items[i].ts.Before(items[j].ts)
		}
		return items[i].ts.After(items[j].ts)
	})
	out := make([]uint32, len(items))
	for i, it := range items {
		out[i] = it.id
	}
	return out
}

// sortByLevel 优先使用 level 的 bitmap 分桶，避免为排序读取全部文档。
// 当索引未暴露 FieldValues("level") 时退化为批量读取文档获取级别。
func (e *Engine) sortByLevel(ids []uint32, bm *roaring.Bitmap, asc bool) []uint32 {
	values := e.idx.FieldValues("level")
	if len(values) == 0 {
		return e.sortByLevelFromDocs(ids, asc)
	}

	buckets := make(map[int]*roaring.Bitmap)
	covered := roaring.New()
	for _, value := range values {
		part := bitmapFromIDs(e.idx.FieldBitmap("level", value))
		part.And(bm)
		if part.IsEmpty() {
			continue
		}
		part.AndNot(covered)
		if part.IsEmpty() {
			continue
		}
		covered.Or(part)
		priority := levelPriorityOf(value)
		bucket := buckets[priority]
		if bucket == nil {
			bucket = roaring.New()
			buckets[priority] = bucket
		}
		bucket.Or(part)
	}
	if covered.GetCardinality() < bm.GetCardinality() {
		rest := bm.Clone()
		rest.AndNot(covered)
		if !rest.IsEmpty() {
			priority := levelPriorityOf("")
			bucket := buckets[priority]
			if bucket == nil {
				bucket = roaring.New()
				buckets[priority] = bucket
			}
			bucket.Or(rest)
		}
	}

	priorities := make([]int, 0, len(buckets))
	for priority := range buckets {
		priorities = append(priorities, priority)
	}
	sort.Ints(priorities)
	if !asc {
		for i, j := 0, len(priorities)-1; i < j; i, j = i+1, j-1 {
			priorities[i], priorities[j] = priorities[j], priorities[i]
		}
	}
	out := make([]uint32, 0, len(ids))
	for _, priority := range priorities {
		out = append(out, buckets[priority].ToArray()...)
	}
	return out
}

func (e *Engine) sortByLevelFromDocs(ids []uint32, asc bool) []uint32 {
	priorities := make([]int, len(ids))
	for i := range priorities {
		priorities[i] = -1
	}
	batchCount := (len(ids) + scanBatchSize - 1) / scanBatchSize
	parallelFor(batchCount, docReadWorkers, func(batch int) {
		start := batch * scanBatchSize
		end := start + scanBatchSize
		if end > len(ids) {
			end = len(ids)
		}
		batchIDs := ids[start:end]
		docs := e.readDocsAligned(batchIDs)
		for i, doc := range docs {
			if doc == nil {
				continue
			}
			priorities[start+i] = levelPriorityOf(doc.Level)
		}
	})

	type item struct {
		id       uint32
		priority int
	}
	items := make([]item, len(ids))
	for i, id := range ids {
		items[i] = item{id: id, priority: priorities[i]}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].priority == items[j].priority {
			return items[i].id < items[j].id
		}
		if asc {
			return items[i].priority < items[j].priority
		}
		return items[i].priority > items[j].priority
	})
	out := make([]uint32, len(items))
	for i, it := range items {
		out[i] = it.id
	}
	return out
}

// ---------------------------------------------------------------------------
// 上下文
// ---------------------------------------------------------------------------

// Context 返回 docID 前后按 docID 连续的日志，按时间升序排列。
// before/after 默认 10，最大 500；docID 越界返回空结果且 AnchorIndex=-1。
func (e *Engine) Context(docID uint32, before, after int) (res *models.ContextResult, err error) {
	defer func() {
		if r := recover(); r != nil {
			res = nil
			err = internalPanic("Context", r)
		}
	}()
	if err = e.checkReady(); err != nil {
		return nil, err
	}
	if before <= 0 {
		before = defaultContext
	}
	if before > maxContext {
		before = maxContext
	}
	if after <= 0 {
		after = defaultContext
	}
	if after > maxContext {
		after = maxContext
	}

	docCount := e.idx.DocCount()
	if docCount == 0 || docID >= docCount {
		return &models.ContextResult{Entries: []models.LogEntry{}, AnchorIndex: -1}, nil
	}

	start := int64(docID) - int64(before)
	if start < 0 {
		start = 0
	}
	end := int64(docID) + int64(after)
	if end >= int64(docCount) {
		end = int64(docCount) - 1
	}
	if end < start {
		return &models.ContextResult{Entries: []models.LogEntry{}, AnchorIndex: -1}, nil
	}

	ids := make([]uint32, 0, end-start+1)
	for id := start; id <= end; id++ {
		ids = append(ids, uint32(id))
	}
	docs := e.readDocsAligned(ids)
	entries := make([]models.LogEntry, 0, len(ids))
	for i, doc := range docs {
		if doc == nil || i >= len(ids) {
			continue
		}
		doc.DocID = uint64(ids[i])
		doc.ID = strconv.FormatUint(uint64(ids[i]), 10)
		entries = append(entries, *doc)
	}

	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Timestamp.Equal(entries[j].Timestamp) {
			return entries[i].DocID < entries[j].DocID
		}
		return entries[i].Timestamp.Before(entries[j].Timestamp)
	})

	anchorIndex := -1
	for i := range entries {
		if entries[i].DocID == uint64(docID) {
			anchorIndex = i
			break
		}
	}
	return &models.ContextResult{Entries: entries, AnchorIndex: anchorIndex}, nil
}

// ---------------------------------------------------------------------------
// 文档读取
// ---------------------------------------------------------------------------

// scanPredicate 在候选 bitmap 上分批并发读取文档并执行谓词。
// 读取失败的文档被跳过；结果保持为 bitmap（与顺序无关）。
func (e *Engine) scanPredicate(candidate *roaring.Bitmap, pred docPredicate) *roaring.Bitmap {
	if candidate == nil || candidate.IsEmpty() {
		return roaring.New()
	}
	ids := candidate.ToArray()
	batchCount := (len(ids) + scanBatchSize - 1) / scanBatchSize
	results := make([]*roaring.Bitmap, batchCount)
	parallelFor(batchCount, docReadWorkers, func(batch int) {
		start := batch * scanBatchSize
		end := start + scanBatchSize
		if end > len(ids) {
			end = len(ids)
		}
		batchIDs := ids[start:end]
		docs := e.readDocsAligned(batchIDs)
		local := roaring.New()
		for i, doc := range docs {
			if doc == nil || i >= len(batchIDs) {
				continue
			}
			id := batchIDs[i]
			doc.DocID = uint64(id)
			doc.ID = strconv.FormatUint(uint64(id), 10)
			if pred(doc) {
				local.Add(id)
			}
		}
		results[batch] = local
	})

	out := roaring.New()
	for _, result := range results {
		if result != nil {
			out.Or(result)
		}
	}
	return out
}

func (e *Engine) readPage(ids []uint32) []models.LogEntry {
	if len(ids) == 0 {
		return []models.LogEntry{}
	}
	docs := e.readDocsAligned(ids)
	entries := make([]models.LogEntry, 0, len(ids))
	for i, doc := range docs {
		if doc == nil || i >= len(ids) {
			continue
		}
		doc.DocID = uint64(ids[i])
		doc.ID = strconv.FormatUint(uint64(ids[i]), 10)
		entries = append(entries, *doc)
	}
	return entries
}

// readDocsAligned 优先使用批量 ReadDocs；失败或长度不符时退化为并发 ReadDoc。
// 返回切片长度始终等于 ids，失败位置为 nil。
func (e *Engine) readDocsAligned(ids []uint32) []*models.LogEntry {
	out := make([]*models.LogEntry, len(ids))
	if len(ids) == 0 {
		return out
	}
	docs, err := e.idx.ReadDocs(ids)
	if err == nil && len(docs) == len(ids) {
		copy(out, docs)
		return out
	}
	return e.readDocsParallel(ids)
}

func (e *Engine) readDocsParallel(ids []uint32) []*models.LogEntry {
	out := make([]*models.LogEntry, len(ids))
	parallelFor(len(ids), docReadWorkers, func(i int) {
		doc, err := e.idx.ReadDoc(ids[i])
		if err == nil {
			out[i] = doc
		}
	})
	return out
}

// readEntries 按输入顺序返回成功读取的日志条目，读取失败项被跳过。
func (e *Engine) readEntries(ids []uint32) []models.LogEntry {
	if len(ids) == 0 {
		return []models.LogEntry{}
	}
	batchCount := (len(ids) + clusterReadSize - 1) / clusterReadSize
	results := make([][]models.LogEntry, batchCount)
	parallelFor(batchCount, docReadWorkers, func(batch int) {
		start := batch * clusterReadSize
		end := start + clusterReadSize
		if end > len(ids) {
			end = len(ids)
		}
		batchIDs := ids[start:end]
		docs := e.readDocsAligned(batchIDs)
		entries := make([]models.LogEntry, 0, len(batchIDs))
		for i, doc := range docs {
			if doc == nil || i >= len(batchIDs) {
				continue
			}
			id := batchIDs[i]
			doc.DocID = uint64(id)
			doc.ID = strconv.FormatUint(uint64(id), 10)
			entries = append(entries, *doc)
		}
		results[batch] = entries
	})

	total := 0
	for _, entries := range results {
		total += len(entries)
	}
	out := make([]models.LogEntry, 0, total)
	for _, entries := range results {
		out = append(out, entries...)
	}
	return out
}
