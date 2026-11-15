package indexer

import (
	"bufio"
	"encoding/binary"
	"io"
	"os"
	"sort"
	"time"
)

// ensureTimeOrderLocked 在需要按时间查询或持久化时重建排序视图。
// timeOrder 只保存 docID，时间戳从 docMeta 读取，避免重复存储。
func (s *Store) ensureTimeOrderLocked() {
	if !s.timeDirty {
		return
	}
	order := make([]uint32, len(s.docs))
	for i := range order {
		order[i] = uint32(i)
	}
	sort.Slice(order, func(i, j int) bool {
		ti := s.docs[order[i]].ts
		tj := s.docs[order[j]].ts
		if ti != tj {
			return ti < tj
		}
		return order[i] < order[j]
	})
	s.timeOrder = order
	s.timeDirty = false
}

// TimeRange 返回时间戳落在 [start, end] 内的 docID（含端点），
// 按时间升序排列。返回新切片，调用方可自由修改。
func (s *Store) TimeRange(start, end time.Time) []uint32 {
	startNs := start.UnixNano()
	endNs := end.UnixNano()
	if startNs > endNs {
		return nil
	}

	s.mu.RLock()
	if !s.timeDirty {
		result := s.timeRangeLocked(startNs, endNs)
		s.mu.RUnlock()
		return result
	}
	s.mu.RUnlock()

	// 排序需要写锁。排序完成后 timeDirty=false，后续查询恢复只读路径。
	s.mu.Lock()
	s.ensureTimeOrderLocked()
	result := s.timeRangeLocked(startNs, endNs)
	s.mu.Unlock()
	return result
}

func (s *Store) timeRangeLocked(startNs, endNs int64) []uint32 {
	n := len(s.timeOrder)
	lo := sort.Search(n, func(i int) bool {
		return s.docs[s.timeOrder[i]].ts >= startNs
	})
	var out []uint32
	for i := lo; i < n; i++ {
		docID := s.timeOrder[i]
		if s.docs[docID].ts > endNs {
			break
		}
		out = append(out, docID)
	}
	return out
}

// Timestamp 返回文档的时间戳（UTC）。docID 越界返回零值 time.Time。
func (s *Store) Timestamp(docID uint32) time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if uint64(docID) >= uint64(len(s.docs)) {
		return time.Time{}
	}
	return nsToTime(s.docs[docID].ts)
}

// saveTimeIndex 写出 indexes/time.idx：
// magic(8) + version(4) + count(8) + count × {ts int64(ns), docID uint32}。
func (s *Store) saveTimeIndex(path string) (int64, error) {
	s.ensureTimeOrderLocked()
	return writeIndexAtomic(path, func(w io.Writer) error {
		var hdr [timeHeaderSize]byte
		copy(hdr[:8], []byte(timeMagic))
		binary.LittleEndian.PutUint32(hdr[8:12], formatVersion)
		binary.LittleEndian.PutUint64(hdr[12:20], uint64(len(s.timeOrder)))
		if _, err := w.Write(hdr[:]); err != nil {
			return err
		}
		var rec [timeRecordSize]byte
		for _, docID := range s.timeOrder {
			binary.LittleEndian.PutUint64(rec[0:8], uint64(s.docs[docID].ts))
			binary.LittleEndian.PutUint32(rec[8:12], docID)
			if _, err := w.Write(rec[:]); err != nil {
				return err
			}
		}
		return nil
	})
}

// loadTimeIndex 加载 time.idx，并把时间戳回填到 docs 元数据。
// 同时校验时间戳非递减、docID 覆盖 [0, count) 且不重复。
func (s *Store) loadTimeIndex(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return corruptedf("%s: 读取文件状态失败: %v", path, err)
	}
	if st.Size() < timeHeaderSize {
		return corruptedf("%s 文件过短（%d 字节）", path, st.Size())
	}
	if err := readMagic(f, timeMagic); err != nil {
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
	if count != uint64(len(s.docs)) {
		return corruptedf("%s 文档数与 docs.idx 不一致（%d != %d）", path, count, len(s.docs))
	}
	if st.Size() != timeHeaderSize+int64(count)*timeRecordSize {
		return corruptedf("%s 文件长度与文档数不一致", path)
	}

	order := make([]uint32, len(s.docs))
	seen := make([]uint64, (len(s.docs)+63)/64)
	reader := bufio.NewReaderSize(f, indexWriteBufferSize)
	var rec [timeRecordSize]byte
	var prevTS int64
	havePrev := false

	for i := range order {
		if _, err := io.ReadFull(reader, rec[:]); err != nil {
			return corruptedf("%s: 读取第 %d 条记录失败: %v", path, i, err)
		}
		ts := int64(binary.LittleEndian.Uint64(rec[0:8]))
		docID := binary.LittleEndian.Uint32(rec[8:12])
		if uint64(docID) >= uint64(len(s.docs)) {
			return corruptedf("%s: 第 %d 条记录 docID 越界", path, i)
		}
		word := docID >> 6
		bit := uint64(1) << (docID & 63)
		if seen[word]&bit != 0 {
			return corruptedf("%s: 第 %d 条记录 docID 重复", path, i)
		}
		seen[word] |= bit
		if havePrev && ts < prevTS {
			return corruptedf("%s: 时间戳未按升序排列（第 %d 条）", path, i)
		}
		prevTS = ts
		havePrev = true

		s.docs[docID].ts = ts
		order[i] = docID
	}

	s.timeOrder = order
	s.timeDirty = false
	if len(order) == 0 {
		s.timeMin = 0
		s.timeMax = 0
		s.hasTime = false
	} else {
		s.timeMin = s.docs[order[0]].ts
		s.timeMax = s.docs[order[len(order)-1]].ts
		s.hasTime = true
	}
	return nil
}
