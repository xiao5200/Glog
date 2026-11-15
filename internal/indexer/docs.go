package indexer

import (
	"bufio"
	"encoding/binary"
	"io"
	"math"
	"os"
)

// saveDocsIndex 写出 indexes/docs.idx：
// magic(8) + version(4) + count(8) + count × {offset uint64, storedLen uint32, rawLen uint32}。
// offset 为日志块起始位置（compressedLen 字段所在位置）。
func (s *Store) saveDocsIndex(path string) (int64, error) {
	return writeIndexAtomic(path, func(w io.Writer) error {
		var hdr [docsHeaderSize]byte
		copy(hdr[:8], []byte(docsMagic))
		binary.LittleEndian.PutUint32(hdr[8:12], formatVersion)
		binary.LittleEndian.PutUint64(hdr[12:20], uint64(len(s.docs)))
		if _, err := w.Write(hdr[:]); err != nil {
			return err
		}
		var rec [docRecordSize]byte
		for i := range s.docs {
			m := &s.docs[i]
			binary.LittleEndian.PutUint64(rec[0:8], m.offset)
			binary.LittleEndian.PutUint32(rec[8:12], m.storedLen)
			binary.LittleEndian.PutUint32(rec[12:16], m.rawLen)
			if _, err := w.Write(rec[:]); err != nil {
				return err
			}
		}
		return nil
	})
}

// loadDocsIndex 加载 docs.idx，并校验块偏移与 logs.dat 当前大小完全一致。
// 若日志文件在最后一次 Finalize 后又被追加，这里会失败，从而触发重建。
func (s *Store) loadDocsIndex(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return corruptedf("%s: 读取文件状态失败: %v", path, err)
	}
	if st.Size() < docsHeaderSize {
		return corruptedf("%s 文件过短（%d 字节）", path, st.Size())
	}
	if err := readMagic(f, docsMagic); err != nil {
		return corruptedf("%s: %v", path, err)
	}
	var head [12]byte
	if _, err := io.ReadFull(f, head[:]); err != nil {
		return corruptedf("%s: 读取文件头失败: %v", path, err)
	}
	if version := binary.LittleEndian.Uint32(head[0:4]); version != formatVersion {
		return corruptedf("%s 格式版本不支持: %d", path, version)
	}
	count := binary.LittleEndian.Uint64(head[4:12])

	dataSize := st.Size() - docsHeaderSize
	if dataSize%docRecordSize != 0 {
		return corruptedf("%s 记录区长度不是 %d 的整数倍", path, docRecordSize)
	}
	if count != uint64(dataSize/docRecordSize) {
		return corruptedf("%s 文档数与文件长度不一致（count=%d）", path, count)
	}
	if count > uint64(math.MaxUint32) || count > uint64(math.MaxInt) {
		return corruptedf("%s 文档数超出支持范围: %d", path, count)
	}

	docs := make([]docMeta, int(count))
	reader := bufio.NewReaderSize(f, indexWriteBufferSize)
	var rec [docRecordSize]byte
	expectedEnd := int64(logsHeaderSize)

	for i := range docs {
		if _, err := io.ReadFull(reader, rec[:]); err != nil {
			return corruptedf("%s: 读取第 %d 条记录失败: %v", path, i, err)
		}
		offset := binary.LittleEndian.Uint64(rec[0:8])
		storedLen := binary.LittleEndian.Uint32(rec[8:12])
		rawLen := binary.LittleEndian.Uint32(rec[12:16])
		if offset != uint64(expectedEnd) {
			return corruptedf("%s: 第 %d 条记录偏移不连续（期望 %d，实际 %d）", path, i, expectedEnd, offset)
		}
		if storedLen == 0 || rawLen == 0 || storedLen > rawLen || rawLen > maxLogBlockSize {
			return corruptedf("%s: 第 %d 条记录块长度非法", path, i)
		}
		blockEnd := expectedEnd + 8 + int64(storedLen)
		if blockEnd > s.logOffset {
			return corruptedf("%s: 第 %d 条记录超出日志文件范围", path, i)
		}
		docs[i] = docMeta{
			offset:    offset,
			storedLen: storedLen,
			rawLen:    rawLen,
		}
		expectedEnd = blockEnd
	}
	if expectedEnd != s.logOffset {
		return corruptedf("%s 落后于 raw/logs.dat（索引结束 %d，日志结束 %d）", path, expectedEnd, s.logOffset)
	}

	s.docs = docs
	return nil
}
