// Package collector 负责把本地文本日志、Windows EVTX、systemd journal
// 以及 SSH 远程日志转换为统一的 models.RawLine 流。
//
// 设计约束：
//   - Collect 必须能响应 context 取消；
//   - emit 回调可能不是线程安全的，所有实现都通过 emitter 串行调用；
//   - 单个文件失败只记录到 Result.Errors，不中断其他文件；
//   - 不使用 shell 拼接本地命令，SSH 侧所有用户输入均做 shell 转义。
package collector

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"glog/pkg/models"
)

// ErrStopped 表示采集因 context 取消或超时而提前停止。
var ErrStopped = errors.New("采集已停止")

const (
	defaultConcurrency = 4
	defaultBufferSize  = 4 << 20  // 单行默认上限 4 MiB
	minBufferSize      = 4 << 10  // 单行下限 4 KiB
	maxBufferSize      = 64 << 20 // 单行上限 64 MiB
	readBufferSize     = 64 << 10 // bufio 读取块大小
	maxResultErrors    = 200      // Result.Errors 最多保留的条数
)

// Options 采集器通用选项。
type Options struct {
	ProjectID   string
	SourceID    string
	SourceHost  string
	Concurrency int
	// BufferSize 是单行最大字节数，<=0 时使用 4 MiB。
	BufferSize int
}

// Result 一次采集的汇总结果。
type Result struct {
	Files      []string `json:"files"`
	TotalBytes int64    `json:"total_bytes"`
	Lines      int64    `json:"lines"`
	Errors     []string `json:"errors"`
}

// Collector 采集器统一接口。
type Collector interface {
	Collect(ctx context.Context, emit func(models.RawLine) error) (*Result, error)
}

func (o Options) concurrency(def int) int {
	n := o.Concurrency
	if n <= 0 {
		n = def
	}
	if n < 1 {
		n = 1
	}
	if n > 64 {
		n = 64
	}
	return n
}

func (o Options) lineLimit() int {
	n := o.BufferSize
	if n <= 0 {
		return defaultBufferSize
	}
	if n < minBufferSize {
		return minBufferSize
	}
	if n > maxBufferSize {
		return maxBufferSize
	}
	return n
}

// resultAccumulator 汇总多个并发文件的结果。
type resultAccumulator struct {
	totalBytes atomic.Int64
	lines      atomic.Int64

	mu         sync.Mutex
	files      []string
	errs       []string
	suppressed int
}

// newResultAccumulator 创建并发安全的采集结果累加器。
func newResultAccumulator() *resultAccumulator {
	return &resultAccumulator{
		files: make([]string, 0, 8),
		errs:  make([]string, 0, 8),
	}
}

func (a *resultAccumulator) addFile(path string) {
	if path == "" {
		return
	}
	a.mu.Lock()
	a.files = append(a.files, path)
	a.mu.Unlock()
}

func (a *resultAccumulator) addBytes(n int64) {
	if n > 0 {
		a.totalBytes.Add(n)
	}
}

func (a *resultAccumulator) addLine() {
	a.lines.Add(1)
}

func (a *resultAccumulator) addError(err error) {
	if err == nil {
		return
	}
	a.addErrorString(err.Error())
}

func (a *resultAccumulator) addErrorf(format string, args ...interface{}) {
	a.addErrorString(fmt.Sprintf(format, args...))
}

func (a *resultAccumulator) addErrorString(msg string) {
	if msg == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.errs) >= maxResultErrors {
		a.suppressed++
		return
	}
	a.errs = append(a.errs, msg)
}

func (a *resultAccumulator) snapshot() *Result {
	a.mu.Lock()
	defer a.mu.Unlock()

	res := &Result{
		Files:      make([]string, len(a.files)),
		TotalBytes: a.totalBytes.Load(),
		Lines:      a.lines.Load(),
		Errors:     make([]string, len(a.errs)),
	}
	copy(res.Files, a.files)
	copy(res.Errors, a.errs)
	if a.suppressed > 0 {
		res.Errors = append(res.Errors,
			fmt.Sprintf("另有 %d 条错误未显示（超过上限 %d）", a.suppressed, maxResultErrors))
	}
	return res
}

// emitter 串行化 emit 调用，并分配全局递增的 Seq。
// 任一 emit 返回错误后，取消派生 context，让所有 worker 尽快退出。
type emitter struct {
	mu     sync.Mutex
	parent context.Context
	ctx    context.Context
	cancel context.CancelFunc
	emit   func(models.RawLine) error
	seq    atomic.Int64
	err    error
}

func newEmitter(parent context.Context, emit func(models.RawLine) error) (*emitter, error) {
	if emit == nil {
		return nil, fmt.Errorf("采集回调 emit 不能为空")
	}
	parent = ensureContext(parent)
	ctx, cancel := context.WithCancel(parent)
	return &emitter{
		parent: parent,
		ctx:    ctx,
		cancel: cancel,
		emit:   emit,
	}, nil
}

func (e *emitter) Emit(line models.RawLine) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.err != nil {
		return e.err
	}
	if line.CollectedAt.IsZero() {
		line.CollectedAt = time.Now()
	}
	line.Seq = e.seq.Add(1)
	if err := e.emit(line); err != nil {
		e.err = fmt.Errorf("写入采集结果失败: %w", err)
		e.cancel()
		return e.err
	}
	return nil
}

func (e *emitter) Err() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.err
}

func (e *emitter) Close() {
	if e.cancel != nil {
		e.cancel()
	}
}

func ensureContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func stoppedError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%w: %v", ErrStopped, err)
	}
	return nil
}

// finalize 统一处理采集结束后的状态：emit 错误优先，其次 context 取消，
// 最后在没有读取到任何日志且存在错误时返回总体错误。
func finalize(ctx context.Context, em *emitter, acc *resultAccumulator) (*Result, error) {
	res := acc.snapshot()
	if err := em.Err(); err != nil {
		return res, err
	}
	if err := stoppedError(ctx); err != nil {
		return res, err
	}
	if res.Lines == 0 && len(res.Errors) > 0 {
		return res, fmt.Errorf("采集未成功读取任何日志: %s", res.Errors[0])
	}
	return res, nil
}
