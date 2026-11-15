package parser

import (
	"regexp"
	"strings"
	"sync"

	"go.uber.org/zap"

	"glog/internal/logx"
	"glog/pkg/models"
)

// MultilineAssembler 按模板配置合并多行日志。Add 返回 (待处理日志, ready)：
// ready 为 true 时 flush 非空，调用方应立即解析该条合并日志。
//
// Match=after（默认）表示匹配起始正则的行开始新日志；Match=before 表示
// 匹配行归属上一条日志，作为上一条日志的结束行一起刷新（简化实现，见
// Add 中注释）。TimeoutMs 不启动定时器，调用方在空闲超时后应主动调用
// Flush。
type MultilineAssembler struct {
	mu      sync.Mutex
	enabled bool
	startRe *regexp.Regexp
	negate  bool
	// matchBefore 为 true 时起始行归属上一块。
	matchBefore bool
	maxLines    int
	buffer      []string
}

// NewMultilineAssembler 根据配置创建多行合并器。配置无效（未启用、缺少
// 正则或正则非法）时退化为逐行直通，不 panic。
func NewMultilineAssembler(cfg *models.MultiLineConfig) *MultilineAssembler {
	assembler := &MultilineAssembler{}
	if cfg == nil || !cfg.Enabled {
		return assembler
	}

	assembler.maxLines = cfg.MaxLines
	assembler.negate = cfg.Negate
	assembler.matchBefore = strings.EqualFold(strings.TrimSpace(cfg.Match), "before")

	pattern := strings.TrimSpace(cfg.Pattern)
	if pattern == "" {
		return assembler
	}
	startRe, err := regexp.Compile(pattern)
	if err != nil {
		logx.Warn("多行起始正则无效，已退化为逐行模式",
			zap.String("pattern", pattern), zap.Error(err))
		return assembler
	}
	assembler.startRe = startRe
	assembler.enabled = true
	return assembler
}

// Add 添加一行。缓冲区尚未形成完整日志时返回 ("", false)。
func (a *MultilineAssembler) Add(line string) (flush string, ready bool) {
	if a == nil {
		return line, true
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.enabled || a.startRe == nil {
		return line, true
	}

	isStart := a.startRe.MatchString(line)
	if a.negate {
		isStart = !isStart
	}

	if isStart && len(a.buffer) > 0 {
		if a.matchBefore {
			// 简化语义：before 模式下匹配到起始行时，当前行仍属于上一块，
			// 将当前行追加到上一块后立即刷新。
			a.buffer = append(a.buffer, line)
			return a.flushLocked(), true
		}
		// after 模式：先用新起始行之前的内容刷新上一块，再开始新块。
		flushed := a.flushLocked()
		a.buffer = append(a.buffer, line)
		return flushed, true
	}

	a.buffer = append(a.buffer, line)
	if a.maxLines > 0 && len(a.buffer) >= a.maxLines {
		return a.flushLocked(), true
	}
	return "", false
}

// Flush 强制返回并清空缓冲区中的剩余内容。
func (a *MultilineAssembler) Flush() string {
	if a == nil {
		return ""
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.flushLocked()
}

func (a *MultilineAssembler) flushLocked() string {
	if len(a.buffer) == 0 {
		return ""
	}
	text := strings.Join(a.buffer, "\n")
	a.buffer = nil
	return text
}
