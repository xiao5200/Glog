package collector

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"strings"
)

// openLocalReader 打开本地文件，并按 gzip 魔数自动解压。
// 返回的 close 函数同时关闭 gzip reader 和底层文件，可重复调用是安全的。
func openLocalReader(path string) (io.Reader, func() error, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}

	rd, closeGzip, err := maybeGzipReader(f, path)
	if err != nil {
		_ = f.Close()
		return nil, nil, err
	}

	closeFn := func() error {
		gzipErr := closeGzip()
		fileErr := f.Close()
		if gzipErr != nil {
			return gzipErr
		}
		return fileErr
	}
	return rd, closeFn, nil
}

// maybeGzipReader 通过 gzip 魔数识别压缩文件。若文件名以 .gz/.gzip 结尾
// 但内容不是 gzip，则返回可读错误，避免把二进制内容当文本采集。
func maybeGzipReader(r io.Reader, name string) (io.Reader, func() error, error) {
	br := bufio.NewReaderSize(r, readBufferSize)
	magic, err := br.Peek(2)
	if err != nil && err != io.EOF && err != bufio.ErrBufferFull {
		return nil, nil, fmt.Errorf("读取文件头失败 %s: %w", name, err)
	}

	if len(magic) >= 2 && magic[0] == 0x1f && magic[1] == 0x8b {
		gz, err := gzip.NewReader(br)
		if err != nil {
			return nil, nil, fmt.Errorf("解压 gzip 失败 %s: %w", name, err)
		}
		return gz, gz.Close, nil
	}

	if isGzipPath(name) {
		return nil, nil, fmt.Errorf("文件扩展名为 .gz 但内容不是有效的 gzip: %s", name)
	}
	return br, func() error { return nil }, nil
}

func isGzipPath(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".gz") || strings.HasSuffix(lower, ".gzip")
}

// trimLineEnd 去除行尾的 \n 或 \r\n。
func trimLineEnd(b []byte) []byte {
	if n := len(b); n > 0 && b[n-1] == '\n' {
		b = b[:n-1]
	}
	if n := len(b); n > 0 && b[n-1] == '\r' {
		b = b[:n-1]
	}
	return b
}

// readLineLimited 逐行读取，单行最多保留 max 字节。
// 返回的 consumed 是实际从 reader 消耗的字节数（包含换行符和超长被丢弃的部分），
// 用于精确跟踪解压后的 offset。超长行会被截断并返回 truncated=true。
//
// 这里刻意使用 bufio.Reader.ReadSlice 而不是 bufio.Scanner，避免 Scanner
// 默认 64 KiB 的上限，同时把内存占用限制在 max 附近。
func readLineLimited(br *bufio.Reader, max int) (line []byte, consumed int64, truncated bool, err error) {
	if max <= 0 {
		max = defaultBufferSize
	}

	buf := make([]byte, 0, 256)
	for {
		frag, readErr := br.ReadSlice('\n')
		consumed += int64(len(frag))

		if len(buf) <= max {
			remaining := max + 1 - len(buf) // +1 容纳行尾 \n
			if remaining > len(frag) {
				remaining = len(frag)
			}
			if remaining > 0 {
				buf = append(buf, frag[:remaining]...)
			}
			if remaining < len(frag) {
				truncated = true
			}
		} else if len(frag) > 0 {
			truncated = true
		}

		if readErr == bufio.ErrBufferFull {
			continue
		}

		buf = trimLineEnd(buf)
		if len(buf) > max {
			buf = buf[:max]
			truncated = true
		}

		if readErr != nil {
			if readErr == io.EOF {
				// io.EOF 带数据时先把该行返回，下一次调用再返回 EOF。
				if len(buf) == 0 && consumed == 0 {
					return nil, 0, false, io.EOF
				}
				return buf, consumed, truncated, nil
			}
			return buf, consumed, truncated, readErr
		}
		return buf, consumed, truncated, nil
	}
}
