package collector

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"glog/internal/errs"
	"glog/pkg/models"
)

// JournalOptions systemd journal 采集选项。
type JournalOptions struct {
	Since    string
	Until    string
	Unit     string
	Priority string
	Lines    int
	Follow   bool
}

// JournalCollector 通过 journalctl 采集 systemd journal，仅 Linux 可用。
type JournalCollector struct {
	opts  JournalOptions
	cOpts Options
}

// NewJournalCollector 创建 journal 采集器。
func NewJournalCollector(opts JournalOptions, cOptions Options) *JournalCollector {
	return &JournalCollector{opts: opts, cOpts: cOptions}
}

// Collect 执行 journalctl 并逐行输出 --output json 的 JSON 行。
func (c *JournalCollector) Collect(ctx context.Context, emit func(models.RawLine) error) (*Result, error) {
	if c == nil {
		return nil, fmt.Errorf("journal 采集器不能为空")
	}
	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("%w: journalctl 仅支持 Linux", errs.ErrUnsupportedOS)
	}
	ctx = ensureContext(ctx)
	em, err := newEmitter(ctx, emit)
	if err != nil {
		return nil, err
	}
	defer em.Close()

	args, err := buildJournalArgs(c.opts)
	if err != nil {
		return nil, err
	}

	acc := newResultAccumulator()
	label := journalLabel(c.opts)

	cmd := exec.CommandContext(ctx, "journalctl", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return acc.snapshot(), fmt.Errorf("获取 journalctl 输出管道失败: %w", err)
	}
	stderr := newLimitedBuffer(64 << 10)
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		return acc.snapshot(), fmt.Errorf("启动 journalctl 失败（请确认系统使用 systemd 且已安装 journalctl）: %w", err)
	}
	acc.addFile(label)

	waiting := true
	defer func() {
		if waiting && cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()

	br := bufio.NewReaderSize(stdout, readBufferSize)
	limit := c.cOpts.lineLimit()
	var offset int64
	var readErr error

	for {
		if ctx.Err() != nil {
			_ = cmd.Process.Kill()
			break
		}

		line, consumed, truncated, lineErr := readLineLimited(br, limit)
		startOffset := offset
		offset += consumed
		acc.addBytes(consumed)

		if lineErr != nil {
			if !errors.Is(lineErr, io.EOF) {
				readErr = lineErr
				_ = cmd.Process.Kill()
			}
			break
		}

		if truncated {
			// 截断后的 JSON 无法解析，丢弃该行但保留字节 offset。
			acc.addErrorf("journalctl 输出偏移 %d 处存在超长行（超过 %d 字节），已跳过",
				startOffset, limit)
			continue
		}
		if ctx.Err() != nil {
			_ = cmd.Process.Kill()
			break
		}

		raw := models.RawLine{
			SourceID:   c.cOpts.SourceID,
			SourceHost: c.cOpts.SourceHost,
			FilePath:   label,
			Offset:     startOffset,
			Size:       consumed,
			Text:       string(line),
			Structured: true,
		}
		if err := em.Emit(raw); err != nil {
			_ = cmd.Process.Kill()
			break
		}
		acc.addLine()
	}

	waitErr := cmd.Wait()
	waiting = false

	res := acc.snapshot()
	if err := em.Err(); err != nil {
		return res, err
	}
	if err := stoppedError(ctx); err != nil {
		return res, err
	}
	if readErr != nil {
		return res, fmt.Errorf("读取 journalctl 输出失败: %w", readErr)
	}
	if waitErr != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			detail = "，stderr: " + truncateString(detail, 2048)
		}
		// 已读取到部分日志时作为部分失败处理，全部失败才返回总体错误。
		if res.Lines == 0 {
			return res, fmt.Errorf("journalctl 执行失败: %w%s", waitErr, detail)
		}
		res.Errors = append(res.Errors, fmt.Sprintf("journalctl 执行失败: %v%s", waitErr, detail))
	}
	return res, nil
}

func buildJournalArgs(opts JournalOptions) ([]string, error) {
	if opts.Lines < 0 {
		return nil, fmt.Errorf("journalctl --lines 不能为负数")
	}
	for flag, value := range map[string]string{
		"--since":    opts.Since,
		"--until":    opts.Until,
		"--unit":     opts.Unit,
		"--priority": opts.Priority,
	} {
		if value == "" {
			continue
		}
		if err := validateOptionValue(value); err != nil {
			return nil, fmt.Errorf("%s 参数无效: %w", flag, err)
		}
	}

	args := []string{"--no-pager"}
	if opts.Since != "" {
		args = append(args, "--since", opts.Since)
	}
	if opts.Until != "" {
		args = append(args, "--until", opts.Until)
	}
	if opts.Unit != "" {
		args = append(args, "--unit", opts.Unit)
	}
	if opts.Priority != "" {
		args = append(args, "--priority", opts.Priority)
	}
	if opts.Lines > 0 {
		args = append(args, "--lines", strconv.Itoa(opts.Lines))
	}
	if opts.Follow {
		args = append(args, "--follow")
	}
	args = append(args, "--output", "json")
	return args, nil
}

func journalLabel(opts JournalOptions) string {
	unit := strings.TrimSpace(opts.Unit)
	if unit != "" {
		return "journalctl:" + unit
	}
	return "journalctl:local"
}
