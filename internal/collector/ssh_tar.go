package collector

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"

	"glog/pkg/models"
)

const (
	// 单个远端压缩包成员解压后的上限，避免 tar/gzip 炸弹耗尽内存与磁盘。
	maxTarFileBytes  = int64(5) << 30  // 5 GiB
	maxTarTotalBytes = int64(20) << 30 // 20 GiB
	maxTarEntries    = 100000
	maxTarPathBytes  = 4096
)

func remoteTarCommand(remotePath, compression string) (string, error) {
	quoted, err := shellQuote(remotePath)
	if err != nil {
		return "", fmt.Errorf("远程路径无效: %w", err)
	}
	var flags string
	switch compression {
	case "gzip":
		flags = "-czf"
	case "none":
		flags = "-cf"
	default:
		return "", fmt.Errorf("不支持的 tar 压缩算法 %q", compression)
	}
	// -- 终止选项解析；路径随后由 shellQuote 包裹，防止以 - 开头的文件名被当作选项。
	return fmt.Sprintf("tar %s - -- %s", flags, quoted), nil
}

// collectCompressedPath 把远端目录/文件打为 tar 流后本地解析。
// 不会把压缩包写入磁盘，也不信任 tar 头中的路径与类型。
func (c *SSHCollector) collectCompressedPath(ctx context.Context, client *ssh.Client, host, compression, remotePath string,
	acc *resultAccumulator, em *emitter) {

	command, err := remoteTarCommand(remotePath, compression)
	if err != nil {
		acc.addErrorf("SSH %s 构建压缩采集命令失败 %s: %v", host, remotePath, err)
		return
	}

	runErr := runSSHStream(ctx, client, command, func(reader io.Reader) error {
		var tarSource io.Reader = reader
		if compression == "gzip" {
			gz, gzipErr := gzip.NewReader(reader)
			if gzipErr != nil {
				return fmt.Errorf("远端 tar.gz 流无效: %w", gzipErr)
			}
			defer gz.Close()
			tarSource = gz
		}

		tr := tar.NewReader(tarSource)
		remaining := maxTarTotalBytes
		entries := 0
		regularFiles := 0

		for {
			if err := ctx.Err(); err != nil {
				return err
			}

			header, nextErr := tr.Next()
			if nextErr == io.EOF {
				break
			}
			if nextErr != nil {
				return fmt.Errorf("解析远端 tar 流失败: %w", nextErr)
			}
			entries++
			if entries > maxTarEntries {
				return fmt.Errorf("远端 tar 成员数量超过上限 %d，已停止", maxTarEntries)
			}

			name, pathErr := safeTarPath(header.Name)
			if pathErr != nil {
				return fmt.Errorf("拒绝不安全的 tar 成员 %q: %w", header.Name, pathErr)
			}

			// 只采集常规文件；符号链接、硬链接、设备文件、目录等一律跳过，
			// 防止后续环节把链接目标当作日志内容或发生路径穿越。
			if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
				continue
			}
			if header.Size < 0 {
				return fmt.Errorf("tar 成员 %q 大小非法: %d", name, header.Size)
			}
			if header.Size > maxTarFileBytes {
				return fmt.Errorf("tar 成员 %q 解压后大小 %d 超过单文件上限 %d",
					name, header.Size, maxTarFileBytes)
			}
			if header.Size > remaining {
				return fmt.Errorf("tar 解压总量超过上限 %d，已在成员 %q 停止", maxTarTotalBytes, name)
			}
			remaining -= header.Size

			regularFiles++
			label := remotePath + ":" + name
			acc.addFile(label)
			if err := c.collectTarEntry(ctx, tr, host, label, acc, em); err != nil {
				return err
			}
			if em.Err() != nil {
				return em.Err()
			}
		}

		if regularFiles == 0 {
			return fmt.Errorf("远端 tar 流中没有常规文件")
		}
		return nil
	})

	if runErr != nil && ctx.Err() == nil && em.Err() == nil {
		acc.addErrorf("SSH %s 压缩采集失败 %s: %v", host, remotePath, runErr)
	}
}

func (c *SSHCollector) collectTarEntry(ctx context.Context, tr *tar.Reader, host, label string,
	acc *resultAccumulator, em *emitter) error {

	br := bufio.NewReaderSize(tr, readBufferSize)
	limit := c.cOptions.lineLimit()
	var offset int64

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		line, consumed, truncated, readErr := readLineLimited(br, limit)
		startOffset := offset
		offset += consumed
		acc.addBytes(consumed)

		if readErr != nil {
			if readErr == io.EOF {
				return nil
			}
			return fmt.Errorf("读取 tar 成员 %s 失败（偏移 %d）: %w", label, startOffset, readErr)
		}
		if truncated {
			acc.addErrorf("SSH %s 压缩包成员 %s 偏移 %d 处存在超长行（超过 %d 字节），已截断",
				host, label, startOffset, limit)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}

		raw := models.RawLine{
			SourceID:   c.cOptions.SourceID,
			SourceHost: c.cOptions.SourceHost,
			FilePath:   label,
			Offset:     startOffset,
			Size:       consumed,
			Text:       string(line),
			Structured: false,
		}
		if err := em.Emit(raw); err != nil {
			return err
		}
		acc.addLine()
	}
}

// safeTarPath 拒绝绝对路径、盘符路径、UNC 路径以及包含 .. 的成员名。
// tar 只承载文件名，不代表本地落盘路径；这里仍按最严格规则检查。
func safeTarPath(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("成员名为空")
	}
	if len(name) > maxTarPathBytes {
		return "", fmt.Errorf("成员名过长（超过 %d 字节）", maxTarPathBytes)
	}
	if strings.ContainsRune(name, 0) || strings.ContainsAny(name, "\r\n") {
		return "", fmt.Errorf("成员名包含控制字符")
	}

	normalized := strings.ReplaceAll(name, `\`, "/")
	if path.IsAbs(normalized) || filepath.IsAbs(name) {
		return "", fmt.Errorf("成员名不能是绝对路径")
	}
	if strings.HasPrefix(normalized, "//") {
		return "", fmt.Errorf("成员名不能是 UNC 路径")
	}
	if len(normalized) >= 2 && normalized[1] == ':' &&
		((normalized[0] >= 'a' && normalized[0] <= 'z') || (normalized[0] >= 'A' && normalized[0] <= 'Z')) {
		return "", fmt.Errorf("成员名不能包含盘符")
	}

	cleaned := path.Clean(normalized)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("成员名包含路径穿越")
	}
	return cleaned, nil
}
