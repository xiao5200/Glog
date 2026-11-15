package collector

import (
	"bytes"
	"context"
	"sync"
	"unicode/utf8"
)

// limitedBuffer 只保留前 limit 字节，避免远端 stderr 无限制占用内存。
type limitedBuffer struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func newLimitedBuffer(limit int) *limitedBuffer {
	if limit <= 0 {
		limit = 64 << 10
	}
	return &limitedBuffer{limit: limit}
}

func (w *limitedBuffer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	remaining := w.limit - w.buf.Len()
	switch {
	case remaining <= 0:
		if len(p) > 0 {
			w.truncated = true
		}
	case len(p) <= remaining:
		_, _ = w.buf.Write(p)
	default:
		_, _ = w.buf.Write(p[:remaining])
		w.truncated = true
	}
	return len(p), nil
}

func (w *limitedBuffer) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	s := w.buf.String()
	if w.truncated {
		s += "..."
	}
	return s
}

// truncateString 截断字符串并尽量不破坏 UTF-8 字符。
func truncateString(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	cut := max - 3
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "..."
}

// checkContext 是一个便于在循环中调用的辅助函数。
func checkContext(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}
