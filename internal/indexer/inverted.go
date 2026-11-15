package indexer

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/RoaringBitmap/roaring"
)

// Postings 返回词项的 posting 列表。
//
// 参数 term 应为 Tokenize 输出的词项（小写）。为容错，未命中时会再尝试
// 小写形式。返回值是索引内部切片的只读视图，不复制：
//   - 调用方不得修改返回切片/元素，也不得对其 append 后回写；
//   - 内部只会向同一 posting 切片尾部追加更大的 docID，从不改写已有元素，
//     因此查询持有的旧视图在并发 Append 下仍然安全（快照语义）。
func (s *Store) Postings(term string) []uint32 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if ids, ok := s.terms[term]; ok {
		return ids
	}
	lower := strings.ToLower(term)
	if lower != term {
		return s.terms[lower]
	}
	return nil
}

// PostingsPrefix 返回所有以 prefix 开头词项的 postings 并集，
// docID 升序去重。prefix 会先转小写（与 Tokenize 一致）。
// 空前缀返回全部文档 ID。
func (s *Store) PostingsPrefix(prefix string) []uint32 {
	prefix = strings.ToLower(prefix)
	if utf8.RuneCountInString(prefix) > maxTokenRunes {
		return nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if prefix == "" {
		ids := make([]uint32, len(s.docs))
		for i := range ids {
			ids[i] = uint32(i)
		}
		return ids
	}

	rb := roaring.New()
	for term, ids := range s.terms {
		if len(ids) == 0 || !strings.HasPrefix(term, prefix) {
			continue
		}
		// posting 内 docID 严格升序，满足 AddMany 对输入有序的要求。
		rb.AddMany(ids)
	}
	if rb.IsEmpty() {
		return nil
	}
	return rb.ToArray()
}

// postingEncodedLen 返回 posting 编码长度，并顺带校验 docID 严格升序。
// 编码：uint32 个数 + 对 (首个 docID, 后续差值) 的 uvarint 序列。
func postingEncodedLen(ids []uint32) (int, error) {
	if len(ids) == 0 {
		return 0, fmt.Errorf("posting 为空")
	}
	n := 4
	var prev uint32
	for i, id := range ids {
		if i == 0 {
			n += uvarintLen(uint64(id))
		} else {
			if id <= prev {
				return 0, fmt.Errorf("posting 未按 docID 严格升序")
			}
			n += uvarintLen(uint64(id - prev))
		}
		prev = id
	}
	return n, nil
}

// appendPosting 编码 posting 到 dst；调用前必须已通过 postingEncodedLen 校验。
func appendPosting(dst []byte, ids []uint32) []byte {
	var cnt [4]byte
	binary.LittleEndian.PutUint32(cnt[:], uint32(len(ids)))
	dst = append(dst, cnt[:]...)
	var prev uint32
	for i, id := range ids {
		if i == 0 {
			dst = appendUvarint(dst, uint64(id))
		} else {
			dst = appendUvarint(dst, uint64(id-prev))
		}
		prev = id
	}
	return dst
}

// saveInvertedIndex 写出 indexes/inverted.idx：
//
//	magic(8) + version(4) + docCount(8) + termCount(8)
//	termCount × {termLen uint16, term, postingOffset uint64, postingLen uint32, df uint32}
//	postings 区（按词项字典序连续排列）
//
// postingOffset 是相对于文件起始的绝对偏移；postingLen 是编码后长度。
func (s *Store) saveInvertedIndex(path string) (int64, error) {
	terms := make([]string, 0, len(s.terms))
	for term := range s.terms {
		terms = append(terms, term)
	}
	sort.Strings(terms)

	offsets := make([]uint64, len(terms))
	lengths := make([]uint32, len(terms))

	var dictSize int64
	for _, term := range terms {
		if len(term) == 0 || len(term) > math.MaxUint16 {
			return 0, fmt.Errorf("词项长度非法: %d", len(term))
		}
		dictSize += int64(2 + len(term) + 8 + 4 + 4)
	}

	nextOffset := uint64(invertedHeaderSize) + uint64(dictSize)
	for i, term := range terms {
		ids := s.terms[term]
		if uint64(len(ids)) > uint64(math.MaxUint32) {
			return 0, fmt.Errorf("词项 %q 的文档频率超过 uint32 上限", term)
		}
		plen, err := postingEncodedLen(ids)
		if err != nil {
			return 0, fmt.Errorf("词项 %q: %w", term, err)
		}
		if uint64(plen) > uint64(math.MaxUint32) {
			return 0, fmt.Errorf("词项 %q 的 posting 编码长度超过 uint32 上限", term)
		}
		if nextOffset < uint64(invertedHeaderSize) || nextOffset > uint64(math.MaxInt64) {
			return 0, fmt.Errorf("倒排索引文件大小超过支持范围")
		}
		offsets[i] = nextOffset
		lengths[i] = uint32(plen)
		nextOffset += uint64(plen)
	}
	if nextOffset > uint64(math.MaxInt64) {
		return 0, fmt.Errorf("倒排索引文件大小超过支持范围")
	}

	return writeIndexAtomic(path, func(w io.Writer) error {
		var hdr [invertedHeaderSize]byte
		copy(hdr[:8], []byte(invertedMagic))
		binary.LittleEndian.PutUint32(hdr[8:12], formatVersion)
		binary.LittleEndian.PutUint64(hdr[12:20], uint64(len(s.docs)))
		binary.LittleEndian.PutUint64(hdr[20:28], uint64(len(terms)))
		if _, err := w.Write(hdr[:]); err != nil {
			return err
		}

		var fixed [16]byte
		for i, term := range terms {
			var lenBuf [2]byte
			binary.LittleEndian.PutUint16(lenBuf[:], uint16(len(term)))
			if _, err := w.Write(lenBuf[:]); err != nil {
				return err
			}
			if _, err := io.WriteString(w, term); err != nil {
				return err
			}
			binary.LittleEndian.PutUint64(fixed[0:8], offsets[i])
			binary.LittleEndian.PutUint32(fixed[8:12], lengths[i])
			binary.LittleEndian.PutUint32(fixed[12:16], uint32(len(s.terms[term])))
			if _, err := w.Write(fixed[:]); err != nil {
				return err
			}
		}

		var buf []byte
		for i, term := range terms {
			ids := s.terms[term]
			buf = buf[:0]
			buf = appendPosting(buf, ids)
			if uint32(len(buf)) != lengths[i] {
				return fmt.Errorf("词项 %q 的 posting 编码长度不一致", term)
			}
			if _, err := w.Write(buf); err != nil {
				return err
			}
		}
		return nil
	})
}

type termEntry struct {
	term   string
	offset uint64
	length uint32
	df     uint32
}

// loadInvertedIndex 加载并解码 inverted.idx，重建词项 -> 升序 docID 列表。
func (s *Store) loadInvertedIndex(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return corruptedf("%s: 读取文件状态失败: %v", path, err)
	}
	fileSize := st.Size()
	if fileSize < invertedHeaderSize {
		return corruptedf("%s 文件过短（%d 字节）", path, fileSize)
	}
	if err := readMagic(f, invertedMagic); err != nil {
		return corruptedf("%s: %v", path, err)
	}
	var head [20]byte
	if _, err := io.ReadFull(f, head[:]); err != nil {
		return corruptedf("%s: 读取文件头失败: %v", path, err)
	}
	if version := binary.LittleEndian.Uint32(head[0:4]); version != formatVersion {
		return corruptedf("%s 格式版本不支持: %d", path, version)
	}
	docCount := binary.LittleEndian.Uint64(head[4:12])
	termCount := binary.LittleEndian.Uint64(head[12:20])
	if docCount != uint64(len(s.docs)) {
		return corruptedf("%s 文档数与 docs.idx 不一致（%d != %d）", path, docCount, len(s.docs))
	}
	// 词典项最小 19 字节（termLen 2 + term 1 + offset 8 + len 4 + df 4），
	// 用于在解析前限制 termCount，防止损坏文件触发超大 map 分配。
	if termCount > uint64(fileSize-invertedHeaderSize)/19 {
		return corruptedf("%s 词项数异常: %d", path, termCount)
	}
	if termCount > uint64(math.MaxInt) {
		return corruptedf("%s 词项数超出支持范围: %d", path, termCount)
	}

	reader := bufio.NewReaderSize(f, indexWriteBufferSize)
	dictPos := int64(invertedHeaderSize)
	entries := make([]termEntry, 0, int(termCount))

	for i := uint64(0); i < termCount; i++ {
		var lenBuf [2]byte
		if _, err := io.ReadFull(reader, lenBuf[:]); err != nil {
			return corruptedf("%s: 读取第 %d 个词项长度失败: %v", path, i, err)
		}
		termLen := binary.LittleEndian.Uint16(lenBuf[:])
		if termLen == 0 {
			return corruptedf("%s: 第 %d 个词项长度为 0", path, i)
		}
		termBytes := make([]byte, termLen)
		if _, err := io.ReadFull(reader, termBytes); err != nil {
			return corruptedf("%s: 读取第 %d 个词项失败: %v", path, i, err)
		}
		var meta [16]byte
		if _, err := io.ReadFull(reader, meta[:]); err != nil {
			return corruptedf("%s: 读取第 %d 个词项元数据失败: %v", path, i, err)
		}
		entries = append(entries, termEntry{
			term:   string(termBytes),
			offset: binary.LittleEndian.Uint64(meta[0:8]),
			length: binary.LittleEndian.Uint32(meta[8:12]),
			df:     binary.LittleEndian.Uint32(meta[12:16]),
		})
		dictPos += int64(2+int(termLen)) + 16
		if dictPos > fileSize {
			return corruptedf("%s: 词典区越界", path)
		}
	}

	// 写入端按词项字典序连续写 postings；这里严格校验偏移连续，
	// 同时可发现尾随垃圾数据与重叠。
	expectedOffset := uint64(dictPos)
	for i := range entries {
		e := &entries[i]
		if e.length < 4 {
			return corruptedf("%s: 词项 %q 的 posting 长度非法", path, e.term)
		}
		if e.offset != expectedOffset {
			return corruptedf("%s: 词项 %q 的 posting 偏移不连续", path, e.term)
		}
		if e.offset > uint64(fileSize) || uint64(e.length) > uint64(fileSize)-e.offset {
			return corruptedf("%s: 词项 %q 的 posting 越界", path, e.term)
		}
		if e.df == 0 || uint64(e.df) > uint64(len(s.docs)) {
			return corruptedf("%s: 词项 %q 的 df 非法", path, e.term)
		}
		expectedOffset += uint64(e.length)
	}
	if expectedOffset != uint64(fileSize) {
		return corruptedf("%s 存在多余数据或 posting 区不完整", path)
	}

	terms := make(map[string][]uint32, int(termCount))
	var scratch []byte
	for i := range entries {
		e := &entries[i]
		if _, exists := terms[e.term]; exists {
			return corruptedf("%s: 词项 %q 重复", path, e.term)
		}
		if int(e.length) > cap(scratch) {
			scratch = make([]byte, int(e.length))
		}
		buf := scratch[:int(e.length)]
		if _, err := f.ReadAt(buf, int64(e.offset)); err != nil {
			return corruptedf("%s: 读取词项 %q 的 posting 失败: %v", path, e.term, err)
		}
		ids, err := decodePosting(buf, e.df, uint32(len(s.docs)))
		if err != nil {
			return corruptedf("%s: 词项 %q 的 posting 损坏: %v", path, e.term, err)
		}
		terms[e.term] = ids
	}
	if uint64(len(terms)) != termCount {
		return corruptedf("%s: 词项数不一致", path)
	}

	s.terms = terms
	return nil
}

// decodePosting 解码 uint32 个数 + delta-uvarint docID 列表。
func decodePosting(buf []byte, df uint32, docCount uint32) ([]uint32, error) {
	if len(buf) < 4 {
		return nil, fmt.Errorf("长度不足")
	}
	count := binary.LittleEndian.Uint32(buf[0:4])
	if count != df {
		return nil, fmt.Errorf("记录个数 %d 与 df %d 不一致", count, df)
	}
	if df == 0 {
		return nil, fmt.Errorf("df 为 0")
	}
	// 每个 delta 至少占 1 字节，先用 posting 长度限制 df，
	// 避免损坏文件声称超大 df 时分配巨量内存。
	if uint64(df) > uint64(len(buf)-4) {
		return nil, fmt.Errorf("df %d 与 posting 长度不匹配", df)
	}
	ids := make([]uint32, 0, df)
	pos := 4
	var acc uint64
	for i := uint32(0); i < df; i++ {
		delta, n := binary.Uvarint(buf[pos:])
		if n <= 0 {
			return nil, fmt.Errorf("uvarint 解码失败")
		}
		pos += n
		if delta > uint64(math.MaxUint32) || acc > uint64(math.MaxUint32)-delta {
			return nil, fmt.Errorf("docID 溢出")
		}
		acc += delta
		id := uint32(acc)
		if id >= docCount {
			return nil, fmt.Errorf("docID %d 越界", id)
		}
		if i > 0 && id <= ids[len(ids)-1] {
			return nil, fmt.Errorf("docID 未严格升序")
		}
		ids = append(ids, id)
	}
	if pos != len(buf) {
		return nil, fmt.Errorf("posting 存在多余数据")
	}
	return ids, nil
}
