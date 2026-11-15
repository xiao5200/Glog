// Package query 实现 GLog 的查询引擎：查询语法解析、bitmap 执行、
// 分页排序、上下文、聚合与日志模式聚类。
//
// 本包只依赖 Index 只读接口，不直接依赖 internal/indexer。
package query

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/RoaringBitmap/roaring"

	"glog/internal/errs"
	"glog/pkg/models"
)

// Index 是查询引擎依赖的索引只读接口。
// indexer.Store 以结构类型实现该接口后可直接传入 NewEngine。
type Index interface {
	DocCount() uint32
	ReadDoc(docID uint32) (*models.LogEntry, error)
	ReadDocs(docIDs []uint32) ([]*models.LogEntry, error)
	AllDocIDs() []uint32
	Postings(term string) []uint32
	PostingsPrefix(prefix string) []uint32
	TimeRange(start, end time.Time) []uint32
	FieldBitmap(field, value string) []uint32
	FieldValues(field string) []string
	Timestamp(docID uint32) time.Time
	LevelCounts() map[string]int64
	Stats() models.IndexStatistics
	Tokenize(text string) []string
}

const (
	defaultLimit = 100
	// maxLimit 是引擎层单次查询的硬上限。威胁检测会以 200000 作为候选
	// 上限（detectionCandidateLimit），因此这里需与其对齐；面向 UI 的
	// internal/app/query_api.go 会自行把分页请求 clamp 到 1000。
	maxLimit        = 200000
	defaultContext  = 10
	maxContext      = 500
	docReadWorkers  = 8
	scanBatchSize   = 256
	clusterMaxDocs  = 50000
	clusterReadSize = 512
)

// Engine 是查询引擎实例。零值 Engine（或传入 nil Index）不会 panic，
// 公开方法会返回明确的“索引未构建”错误。
type Engine struct {
	idx Index
}

// NewEngine 创建查询引擎。idx 为 nil 时引擎仍可创建，但查询方法会返回错误。
func NewEngine(idx Index) *Engine {
	return &Engine{idx: idx}
}

func (e *Engine) checkReady() error {
	if e == nil || e.idx == nil {
		return fmt.Errorf("查询引擎未初始化: %w", errs.ErrIndexNotBuilt)
	}
	return nil
}

func internalPanic(op string, r interface{}) error {
	return errs.Internal("查询引擎内部错误（"+op+"）", fmt.Errorf("%v", r))
}

// 时间开区间使用的边界。日志时间正常位于 1970~9999 年之间。
var (
	minTimeBound = time.Time{}
	maxTimeBound = time.Date(9999, 12, 31, 23, 59, 59, 999999999, time.UTC)
)

func bitmapFromIDs(ids []uint32) *roaring.Bitmap {
	bm := roaring.New()
	bm.AddMany(ids)
	return bm
}

// intersectBitmaps 返回 base 与所有 bms 的交集。base/bms 不会被修改。
func intersectBitmaps(base *roaring.Bitmap, bms []*roaring.Bitmap) *roaring.Bitmap {
	cur := roaring.New()
	if base != nil {
		cur = base.Clone()
	}
	if len(bms) == 0 {
		return cur
	}
	sorted := make([]*roaring.Bitmap, 0, len(bms))
	for _, bm := range bms {
		if bm != nil {
			sorted = append(sorted, bm)
		}
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].GetCardinality() < sorted[j].GetCardinality()
	})
	for _, bm := range sorted {
		if bm.IsEmpty() {
			return roaring.New()
		}
		cur.And(bm)
		if cur.IsEmpty() {
			return cur
		}
	}
	return cur
}

// parallelFor 以固定并发度处理 [0,n)，结果按下标由调用方写入独立槽位。
// 单个任务 panic 只会终止其所在 worker 的剩余任务，不会让进程崩溃。
func parallelFor(n, workers int, fn func(i int)) {
	if n <= 0 {
		return
	}
	if workers <= 0 {
		workers = 1
	}
	if workers > n {
		workers = n
	}
	chunk := (n + workers - 1) / workers
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		start := w * chunk
		if start >= n {
			break
		}
		end := start + chunk
		if end > n {
			end = n
		}
		wg.Add(1)
		go func(start, end int) {
			defer wg.Done()
			defer func() { _ = recover() }()
			for i := start; i < end; i++ {
				fn(i)
			}
		}(start, end)
	}
	wg.Wait()
}
