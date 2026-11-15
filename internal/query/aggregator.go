package query

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/RoaringBitmap/roaring"

	"glog/internal/errs"
	"glog/pkg/models"
)

// Aggregate 对查询条件匹配的文档做时间序列、字段分布与 TopN 聚合。
func (e *Engine) Aggregate(req models.AggregateRequest) (res *models.AggregateResult, err error) {
	defer func() {
		if r := recover(); r != nil {
			res = nil
			err = internalPanic("Aggregate", r)
		}
	}()
	if err = e.checkReady(); err != nil {
		return nil, err
	}

	started := time.Now()
	matched, matchErr := e.matchDocs(req.Query)
	if matchErr != nil {
		return nil, matchErr
	}
	total := int64(matched.GetCardinality())
	out := &models.AggregateResult{Total: total}
	if total == 0 {
		out.Took = time.Since(started).Milliseconds()
		return out, nil
	}

	bucket, bucketErr := chooseTimeBucket(req)
	if bucketErr != nil {
		return nil, bucketErr
	}
	if bucket != "" {
		if bucket == "auto" {
			bucket = e.autoTimeBucket(matched)
		}
		out.TimeSeries = e.buildTimeSeries(matched, bucket)
	}

	out.Distribution = e.buildDistribution(req.AggregateBy, matched, total)
	out.TopItems = buildTopItems(out.Distribution, normalizeTopN(req.TopN))
	out.Took = time.Since(started).Milliseconds()
	return out, nil
}

func chooseTimeBucket(req models.AggregateRequest) (string, error) {
	tb := strings.ToLower(strings.TrimSpace(req.TimeBucket))
	if tb != "" {
		switch tb {
		case "auto":
			return "auto", nil
		case "minute", "hour", "day":
			return tb, nil
		default:
			return "", fmt.Errorf("%w: 不支持的时间桶 %q", errs.ErrInvalidInput, req.TimeBucket)
		}
	}
	for _, raw := range req.AggregateBy {
		unit := strings.ToLower(strings.TrimSpace(raw))
		switch unit {
		case "minute", "hour", "day":
			return unit, nil
		}
	}
	return "", nil
}

func (e *Engine) autoTimeBucket(matched *roaring.Bitmap) string {
	minTS, maxTS, ok := e.timeBounds(matched)
	if !ok {
		return "minute"
	}
	span := maxTS.Sub(minTS)
	if span < 0 {
		return "day"
	}
	if span <= 2*time.Hour {
		return "minute"
	}
	if span <= 72*time.Hour {
		return "hour"
	}
	return "day"
}

func (e *Engine) timeBounds(matched *roaring.Bitmap) (time.Time, time.Time, bool) {
	if matched == nil || matched.IsEmpty() {
		return time.Time{}, time.Time{}, false
	}
	ids := matched.ToArray()
	var minTS time.Time
	var maxTS time.Time
	first := true
	for _, id := range ids {
		ts := e.idx.Timestamp(id)
		if first {
			minTS = ts
			maxTS = ts
			first = false
			continue
		}
		if ts.Before(minTS) {
			minTS = ts
		}
		if ts.After(maxTS) {
			maxTS = ts
		}
	}
	return minTS, maxTS, !first
}

func (e *Engine) buildTimeSeries(matched *roaring.Bitmap, bucket string) []models.TimeSeriesPoint {
	counts := make(map[time.Time]int64)
	for _, id := range matched.ToArray() {
		start := bucketStart(e.idx.Timestamp(id), bucket)
		counts[start]++
	}
	keys := make([]time.Time, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].Before(keys[j]) })
	points := make([]models.TimeSeriesPoint, 0, len(keys))
	for _, key := range keys {
		points = append(points, models.TimeSeriesPoint{Timestamp: key, Count: counts[key]})
	}
	return points
}

// bucketStart 以 UTC 对齐时间桶，避免本地时区 DST 造成的桶边界歧义。
func bucketStart(ts time.Time, bucket string) time.Time {
	t := ts.UTC()
	switch bucket {
	case "minute":
		return t.Truncate(time.Minute)
	case "hour":
		return t.Truncate(time.Hour)
	default:
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	}
}

func (e *Engine) buildDistribution(aggBy []string, matched *roaring.Bitmap, total int64) []models.DistributionItem {
	counts := make(map[string]int64)
	seen := make(map[string]bool)
	var docFields []string

	for _, raw := range aggBy {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		switch strings.ToLower(name) {
		case "minute", "hour", "day":
			continue
		}
		field := canonicalFieldName(name)
		if seen[field] {
			continue
		}
		seen[field] = true

		switch field {
		case "level", "source_host", "source", "event_id",
			"event_type", "source_ip", "destination_ip":
			if !e.countIndexedField(field, matched, counts) {
				docFields = append(docFields, field)
			}
		default:
			docFields = append(docFields, field)
		}
	}

	if len(docFields) > 0 {
		e.countDocumentFields(docFields, matched, counts)
	}

	items := make([]models.DistributionItem, 0, len(counts))
	for key, count := range counts {
		if count <= 0 {
			continue
		}
		percentage := 0.0
		if total > 0 {
			percentage = float64(count) / float64(total) * 100
		}
		items = append(items, models.DistributionItem{
			Key:        key,
			Count:      count,
			Percentage: percentage,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count == items[j].Count {
			return items[i].Key < items[j].Key
		}
		return items[i].Count > items[j].Count
	})
	return items
}

// fieldCountsIndex 是可选的索引能力：直接在内部位图上统计各字段值的交集基数。
// indexer.Store 实现该接口；其它 Index 实现没有此能力时回退到 FieldValues + FieldBitmap。
type fieldCountsIndex interface {
	FieldCounts(field string, filter *roaring.Bitmap) (map[string]int64, bool)
}

// countIndexedField 使用索引与候选交集计数，不逐条读取。
// 返回 false 表示索引没有暴露该字段值，调用方应退化为逐条读取。
func (e *Engine) countIndexedField(field string, matched *roaring.Bitmap, counts map[string]int64) bool {
	if fc, ok := e.idx.(fieldCountsIndex); ok {
		values, indexed := fc.FieldCounts(field, matched)
		if !indexed {
			return false
		}
		for value, count := range values {
			counts[value] += count
		}
		return true
	}
	values := e.idx.FieldValues(field)
	if len(values) == 0 {
		return false
	}
	for _, value := range values {
		count := bitmapFromIDs(e.idx.FieldBitmap(field, value)).AndCardinality(matched)
		if count > 0 {
			counts[value] += int64(count)
		}
	}
	return true
}

// countDocumentFields 用于 fields.xxx 等无法通过 bitmap 计数的字段。
func (e *Engine) countDocumentFields(fields []string, matched *roaring.Bitmap, counts map[string]int64) {
	if matched == nil || matched.IsEmpty() || len(fields) == 0 {
		return
	}
	ids := matched.ToArray()
	batchCount := (len(ids) + scanBatchSize - 1) / scanBatchSize
	partials := make([]map[string]int64, batchCount)
	parallelFor(batchCount, docReadWorkers, func(batch int) {
		start := batch * scanBatchSize
		end := start + scanBatchSize
		if end > len(ids) {
			end = len(ids)
		}
		docs := e.readDocsAligned(ids[start:end])
		local := make(map[string]int64)
		for _, doc := range docs {
			if doc == nil {
				continue
			}
			for _, field := range fields {
				value, ok := extractFieldValue(doc, field)
				if !ok {
					continue
				}
				key := valueToString(value)
				if key == "" {
					continue
				}
				local[key]++
			}
		}
		partials[batch] = local
	})
	for _, partial := range partials {
		for key, count := range partial {
			counts[key] += count
		}
	}
}

func normalizeTopN(topN int) int {
	if topN <= 0 {
		return 10
	}
	return topN
}

func buildTopItems(distribution []models.DistributionItem, topN int) []models.TopItem {
	if topN <= 0 {
		topN = 10
	}
	if topN > len(distribution) {
		topN = len(distribution)
	}
	items := make([]models.TopItem, 0, topN)
	for i := 0; i < topN; i++ {
		items = append(items, models.TopItem{
			Key:   distribution[i].Key,
			Count: distribution[i].Count,
		})
	}
	return items
}
