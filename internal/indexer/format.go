package indexer

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"time"

	"glog/internal/errs"
)

// 持久化格式版本。
// logs.dat 与 docs/inverted/time/bloom 索引共用 formatVersion=1；
// bitmap.idx 使用独立版本：v2 起索引包含 event_type/source_ip/destination_ip
// 派生字段。旧项目加载 v1 bitmap 时版本校验失败，Open 会从 logs.dat 整库重建，
// 从而保证派生字段覆盖全部历史日志。
const (
	formatVersion       uint32 = 1
	bitmapFormatVersion uint32 = 2
	metaVersion         uint32 = 1
)

// 各文件头长度（字节）。
const (
	logsHeaderSize     = 12 // magic(8) + version(4)
	docsHeaderSize     = 20 // magic(8) + version(4) + count(8)
	invertedHeaderSize = 28 // magic(8) + version(4) + docCount(8) + termCount(8)
	timeHeaderSize     = 20 // magic(8) + version(4) + count(8)
	bitmapHeaderSize   = 16 // magic(8) + version(4) + fieldCount(4)
	bloomHeaderSize    = 16 // magic(8) + version(4) + bucketCount(4)
)

// 定长记录大小（字节）。
const (
	docRecordSize  = 16 // offset(8) + storedLen(4) + rawLen(4)
	timeRecordSize = 12 // ts(8) + docID(4)
)

// magic 字符串（均为 8 字节）。
const (
	logsMagic     = "GLOGSEG1"
	docsMagic     = "GLOGDOC1"
	invertedMagic = "GLOGINV1"
	timeMagic     = "GLOGTIM1"
	bitmapMagic   = "GLOGBMP1"
	bloomMagic    = "GLOGBLM1"
)

// 写入与容量约束。
const (
	// indexWriteBufferSize 用于索引文件与日志文件写入的缓冲区大小。
	indexWriteBufferSize = 1 << 20

	// maxLogBlockSize 单条日志未压缩 JSON 的上限（256 MiB），
	// 防止损坏的块头导致巨量内存分配，也保证块头的 uint32 长度字段安全。
	maxLogBlockSize = 256 << 20

	// maxTokenRunes Tokenize 允许保留的最大 token 长度（按 rune 计）。
	maxTokenRunes = 64

	// Bloom 默认参数：每个小时桶 1<<20 bit（128 KiB），3 个哈希函数。
	defaultBloomBits      uint32 = 1 << 20
	defaultBloomHashCount uint8  = 3

	// 加载时的防御性上限。
	maxBloomBits      uint32 = 1 << 24
	maxBloomHashCount uint8  = 32
	maxBloomBuckets          = 720
	maxBloomFileBytes int64  = 512 << 20
)

// nsPerHour 一个小时的纳秒数。
const nsPerHour int64 = int64(time.Hour)

// docMeta 是常驻内存的文档元数据（每文档 24 字节，对齐后）。
// 只保存定位压缩块所需的信息与时间戳，不保存原始日志内容，
// 日志正文按需从 logs.dat 解码，避免同一条日志在内存中保留多份。
type docMeta struct {
	offset    uint64 // 日志块起始位置（compressedLen 字段所在位置）
	storedLen uint32 // 落盘长度：压缩后长度；未压缩时等于 rawLen
	rawLen    uint32 // 未压缩 JSON 长度
	ts        int64  // Unix 纳秒时间戳（索引内统一用 UTC 瞬时值比较）
}

// corruptedf 构造带 errs.ErrIndexCorrupted 哨兵的错误。
func corruptedf(format string, args ...interface{}) error {
	return fmt.Errorf("%w: %s", errs.ErrIndexCorrupted, fmt.Sprintf(format, args...))
}

// countingWriter 统计实际写入字节数，用于回填索引文件大小。
type countingWriter struct {
	w io.Writer
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

// writeIndexAtomic 将索引写入 path+".tmp"，成功后再 rename 覆盖正式文件。
// 返回实际写入字节数。任何一步失败都会删除临时文件，尽量保留旧文件可用。
func writeIndexAtomic(path string, write func(io.Writer) error) (int64, error) {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, err
	}
	bw := bufio.NewWriterSize(f, indexWriteBufferSize)
	cw := &countingWriter{w: bw}

	writeErr := write(cw)
	if writeErr == nil {
		writeErr = bw.Flush()
	}
	if writeErr == nil {
		// Finalize 是检查点操作，落盘到稳定存储，避免索引与日志文件
		// 在系统崩溃后处于新旧混合状态。
		writeErr = f.Sync()
	}
	closeErr := f.Close()
	if writeErr != nil {
		_ = os.Remove(tmp)
		return 0, writeErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return 0, closeErr
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return 0, err
	}
	return cw.n, nil
}

// readMagic 读取并校验 8 字节 magic。
func readMagic(r io.Reader, want string) error {
	var buf [8]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return err
	}
	if string(buf[:]) != want {
		return fmt.Errorf("magic 不匹配：期望 %q，实际 %q", want, string(buf[:]))
	}
	return nil
}

// appendUvarint 以 uvarint 追加 v 到 dst。
func appendUvarint(dst []byte, v uint64) []byte {
	var buf [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(buf[:], v)
	return append(dst, buf[:n]...)
}

// uvarintLen 返回 v 的 uvarint 编码长度。
func uvarintLen(v uint64) int {
	var buf [binary.MaxVarintLen64]byte
	return binary.PutUvarint(buf[:], v)
}

// nsToTime 将纳秒时间戳还原为 UTC 时间。
func nsToTime(ns int64) time.Time {
	return time.Unix(0, ns).UTC()
}

// hourStart 返回 ns 所在小时桶的起始纳秒（按时间轴向下取整，支持 1970 年之前）。
func hourStart(ns int64) int64 {
	q := ns / nsPerHour
	if ns < 0 && ns%nsPerHour != 0 {
		q--
	}
	return q * nsPerHour
}
