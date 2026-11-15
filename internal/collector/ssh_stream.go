package collector

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
)

// runSSHStream 在独立 session 中执行远端命令，并把 stdout 交给 consume 处理。
// 每个文件调用一次，避免复用 SSH session。
func runSSHStream(ctx context.Context, client *ssh.Client, command string, consume func(io.Reader) error) error {
	_, err := runSSHStreamDetailed(ctx, client, command, consume)
	return err
}

// runSSHStreamDetailed 与 runSSHStream 行为一致，额外返回远端 stderr 文本。
// 供需要区分"命令成功但 stderr 有告警/错误"的调用方使用（如远程日志检索），
// 普通采集流程仍然只关心错误。
func runSSHStreamDetailed(ctx context.Context, client *ssh.Client, command string, consume func(io.Reader) error) (string, error) {
	if client == nil {
		return "", fmt.Errorf("SSH 客户端未初始化")
	}
	if consume == nil {
		return "", fmt.Errorf("SSH 输出处理函数不能为空")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}

	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("创建 SSH 会话失败: %w", err)
	}
	defer session.Close()

	stdout, err := session.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("获取远程命令输出失败: %w", err)
	}
	stderr := newLimitedBuffer(64 << 10)
	session.Stderr = stderr

	if err := session.Start(command); err != nil {
		return "", fmt.Errorf("启动远程命令失败: %w", err)
	}

	done := make(chan struct{})
	var killOnce sync.Once
	kill := func() {
		killOnce.Do(func() {
			_ = session.Signal(ssh.SIGKILL)
			_ = session.Close()
		})
	}

	go func() {
		select {
		case <-ctx.Done():
			kill()
		case <-done:
		}
	}()

	consumeErr := consume(stdout)

	if consumeErr != nil {
		// 消费端提前退出时关闭 session，避免远端继续写导致 Wait 卡住。
		kill()
		_ = session.Wait()
		close(done)
		if ctx.Err() != nil {
			return strings.TrimSpace(stderr.String()), ctx.Err()
		}
		return strings.TrimSpace(stderr.String()), consumeErr
	}

	// tar.Reader/gzip.Reader 可能在 tar 结束标记处提前停止读取，但远端仍在写
	// gzip trailer 或后续数据。必须先排空 stdout，否则 session.Wait 会因远端
	// 写满通道缓冲区而永久阻塞。
	_, _ = io.Copy(io.Discard, stdout)

	// 保持 watcher 存活到 Wait 返回，确保远端命令卡住时 ctx 取消能将其杀死。
	waitErr := session.Wait()
	close(done)
	if ctx.Err() != nil {
		return strings.TrimSpace(stderr.String()), ctx.Err()
	}
	if waitErr != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return detail, fmt.Errorf("%w（远程 stderr: %s）", waitErr, truncateString(detail, 2048))
		}
		return "", waitErr
	}
	return strings.TrimSpace(stderr.String()), nil
}
