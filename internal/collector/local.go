package collector

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"sync"

	"glog/pkg/models"
)

// LocalTextCollector 采集本地文本日志，支持通配符、递归目录、排除规则与 .gz。
type LocalTextCollector struct {
	paths        []string
	recursive    bool
	excludePaths []string
	opts         Options
}

// NewLocalTextCollector 创建本地文本采集器。
func NewLocalTextCollector(paths []string, recursive bool, excludePaths []string, opts Options) *LocalTextCollector {
	return &LocalTextCollector{
		paths:        append([]string(nil), paths...),
		recursive:    recursive,
		excludePaths: append([]string(nil), excludePaths...),
		opts:         opts,
	}
}

// Collect 采集所有匹配文件。单个文件错误写入 Result.Errors，不中断其他文件。
func (c *LocalTextCollector) Collect(ctx context.Context, emit func(models.RawLine) error) (*Result, error) {
	if c == nil {
		return nil, fmt.Errorf("本地文本采集器不能为空")
	}
	ctx = ensureContext(ctx)
	em, err := newEmitter(ctx, emit)
	if err != nil {
		return nil, err
	}
	defer em.Close()

	acc := newResultAccumulator()
	files, expandErrs := expandPaths(ctx, c.paths, c.recursive, c.excludePaths)
	for _, expandErr := range expandErrs {
		acc.addError(expandErr)
	}
	if err := stoppedError(ctx); err != nil {
		return acc.snapshot(), err
	}
	if len(files) == 0 {
		return acc.snapshot(), fmt.Errorf("未找到可采集的日志文件（共 %d 个路径模式）", len(c.paths))
	}

	concurrency := c.opts.concurrency(defaultConcurrency)
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for _, filePath := range files {
		select {
		case <-em.ctx.Done():
			wg.Wait()
			return finalize(ctx, em, acc)
		case sem <- struct{}{}:
		}

		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			defer func() { <-sem }()
			c.collectFile(em.ctx, p, acc, em)
		}(filePath)
	}
	wg.Wait()

	return finalize(ctx, em, acc)
}

func (c *LocalTextCollector) collectFile(ctx context.Context, filePath string, acc *resultAccumulator, em *emitter) {
	info, err := os.Stat(filePath)
	if err != nil {
		acc.addErrorf("检查文件失败 %s: %v", filePath, err)
		return
	}
	if info.IsDir() {
		acc.addErrorf("路径是目录而非文件: %s", filePath)
		return
	}
	if !info.Mode().IsRegular() {
		acc.addErrorf("不是常规文件: %s", filePath)
		return
	}
	if IsEVTX(filePath) {
		acc.addErrorf("EVTX 文件请使用「Windows 事件日志」或「文件导入」采集: %s", filePath)
		return
	}

	rd, closeFn, err := openLocalReader(filePath)
	if err != nil {
		acc.addErrorf("打开文件失败 %s: %v", filePath, err)
		return
	}
	defer func() { _ = closeFn() }()

	acc.addFile(filePath)
	br := bufio.NewReaderSize(rd, readBufferSize)
	limit := c.opts.lineLimit()
	var offset int64

	for {
		if err := ctx.Err(); err != nil {
			return
		}

		line, consumed, truncated, readErr := readLineLimited(br, limit)
		startOffset := offset
		offset += consumed
		acc.addBytes(consumed)

		if readErr != nil {
			if readErr == io.EOF {
				return
			}
			if ctx.Err() != nil {
				return
			}
			acc.addErrorf("读取文件失败 %s（偏移 %d）: %v", filePath, startOffset, readErr)
			return
		}
		if truncated {
			acc.addErrorf("文件 %s 偏移 %d 处存在超长行（超过 %d 字节），已截断",
				filePath, startOffset, limit)
		}
		if ctx.Err() != nil {
			return
		}

		raw := models.RawLine{
			SourceID:   c.opts.SourceID,
			SourceHost: c.opts.SourceHost,
			FilePath:   filePath,
			Offset:     startOffset,
			Size:       consumed,
			Text:       string(line),
			Structured: false,
		}
		if err := em.Emit(raw); err != nil {
			return
		}
		acc.addLine()
	}
}
