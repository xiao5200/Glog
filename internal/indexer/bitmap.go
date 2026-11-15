package indexer

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"

	"github.com/RoaringBitmap/roaring"

	"glog/internal/logfields"
	"glog/internal/logx"
	"glog/pkg/models"
)

const (
	fieldLevel         = "level"
	fieldSourceHost    = "source_host"
	fieldSource        = "source"
	fieldEventID       = "event_id"
	fieldEventType     = "event_type"
	fieldSourceIP      = "source_ip"
	fieldDestinationIP = "destination_ip"
)

// addEntryFieldsLocked 把内建字段与派生字段（event_type/source_ip/destination_ip）
// 的值加入 roaring bitmap。空字符串值不建索引，避免产生无意义的空值位图。
// 派生字段的提取规则与查询/聚合共用 internal/logfields，保证 bitmap 值与解释值一致。
func (s *Store) addEntryFieldsLocked(entry *models.LogEntry, docID uint32) {
	s.addFieldValueLocked(fieldLevel, entry.Level, docID)
	if entry.SourceHost != "" {
		s.addFieldValueLocked(fieldSourceHost, entry.SourceHost, docID)
	}
	if entry.Source != "" {
		s.addFieldValueLocked(fieldSource, entry.Source, docID)
	}
	if entry.EventID != nil {
		s.addFieldValueLocked(fieldEventID, strconv.FormatUint(uint64(*entry.EventID), 10), docID)
	}
	if eventType, ok := logfields.EventType(entry); ok {
		s.addFieldValueLocked(fieldEventType, eventType, docID)
	}
	if sourceIP, ok := logfields.ExtractIP(entry.Fields, logfields.SourceIPAliases); ok {
		s.addFieldValueLocked(fieldSourceIP, sourceIP, docID)
	}
	if destinationIP, ok := logfields.ExtractIP(entry.Fields, logfields.DestinationIPAliases); ok {
		s.addFieldValueLocked(fieldDestinationIP, destinationIP, docID)
	}
}

func (s *Store) addFieldValueLocked(field, value string, docID uint32) {
	if field == "" || value == "" {
		return
	}
	if len(field) > math.MaxUint16 || len(value) > math.MaxUint16 {
		// bitmap 文件使用 uint16 长度字段，超长值无法持久化，跳过并记录。
		logx.Warn(fmt.Sprintf("bitmap 字段 %q 的值过长（%d 字节），已跳过索引", field, len(value)))
		return
	}
	fieldValues := s.fields[field]
	if fieldValues == nil {
		fieldValues = make(map[string]*roaring.Bitmap)
		s.fields[field] = fieldValues
	}
	rb := fieldValues[value]
	if rb == nil {
		rb = roaring.New()
		fieldValues[value] = rb
	}
	rb.Add(docID)
}

// FieldBitmap 返回字段等于 value 的 docID 列表（升序新切片）。
func (s *Store) FieldBitmap(field, value string) []uint32 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	fieldValues := s.fields[field]
	if fieldValues == nil {
		return nil
	}
	rb := fieldValues[value]
	if rb == nil || rb.IsEmpty() {
		return nil
	}
	return rb.ToArray()
}

// FieldCounts 统计字段各值在 filter 文档集合中的数量。
// filter 为 nil 表示统计全部文档；返回 ok=false 表示该字段没有任何索引值。
// 与逐值调用 FieldBitmap 相比，该方法直接使用内部 roaring 位图求交集，
// 避免为每个值复制 docID 数组，适合来源/目的 IP 等高基数字段的聚合。
func (s *Store) FieldCounts(field string, filter *roaring.Bitmap) (map[string]int64, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	fieldValues := s.fields[field]
	if len(fieldValues) == 0 {
		return nil, false
	}
	out := make(map[string]int64, len(fieldValues))
	for value, rb := range fieldValues {
		var count int
		if filter == nil {
			count = int(rb.GetCardinality())
		} else {
			count = int(rb.AndCardinality(filter))
		}
		if count > 0 {
			out[value] = int64(count)
		}
	}
	return out, true
}

// FieldValues 返回字段出现过的所有值（字典序）。
func (s *Store) FieldValues(field string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	fieldValues := s.fields[field]
	if len(fieldValues) == 0 {
		return nil
	}
	values := make([]string, 0, len(fieldValues))
	for value := range fieldValues {
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}

// saveBitmapIndex 写出 indexes/bitmap.idx：
//
//	magic(8) + version(4) + fieldCount(4)
//	fieldCount × {nameLen uint16, name, valueCount uint32,
//	              valueCount × {valueLen uint16, value, dataLen uint32, roaring bytes}}
func (s *Store) saveBitmapIndex(path string) (int64, error) {
	fields := make([]string, 0, len(s.fields))
	for field := range s.fields {
		fields = append(fields, field)
	}
	sort.Strings(fields)

	return writeIndexAtomic(path, func(w io.Writer) error {
		var hdr [bitmapHeaderSize]byte
		copy(hdr[:8], []byte(bitmapMagic))
		binary.LittleEndian.PutUint32(hdr[8:12], bitmapFormatVersion)
		binary.LittleEndian.PutUint32(hdr[12:16], uint32(len(fields)))
		if _, err := w.Write(hdr[:]); err != nil {
			return err
		}

		for _, field := range fields {
			if len(field) == 0 || len(field) > math.MaxUint16 {
				return fmt.Errorf("bitmap 字段名长度非法: %d", len(field))
			}
			fieldValues := s.fields[field]
			values := make([]string, 0, len(fieldValues))
			for value := range fieldValues {
				values = append(values, value)
			}
			sort.Strings(values)

			var lenBuf [2]byte
			binary.LittleEndian.PutUint16(lenBuf[:], uint16(len(field)))
			if _, err := w.Write(lenBuf[:]); err != nil {
				return err
			}
			if _, err := io.WriteString(w, field); err != nil {
				return err
			}
			var countBuf [4]byte
			binary.LittleEndian.PutUint32(countBuf[:], uint32(len(values)))
			if _, err := w.Write(countBuf[:]); err != nil {
				return err
			}

			for _, value := range values {
				if len(value) > math.MaxUint16 {
					return fmt.Errorf("bitmap 字段 %q 的值长度非法: %d", field, len(value))
				}
				data, err := fieldValues[value].MarshalBinary()
				if err != nil {
					return fmt.Errorf("序列化 bitmap 失败: %w", err)
				}
				if uint64(len(data)) > uint64(math.MaxUint32) {
					return fmt.Errorf("bitmap 数据过大")
				}

				binary.LittleEndian.PutUint16(lenBuf[:], uint16(len(value)))
				if _, err := w.Write(lenBuf[:]); err != nil {
					return err
				}
				if _, err := io.WriteString(w, value); err != nil {
					return err
				}
				binary.LittleEndian.PutUint32(countBuf[:], uint32(len(data)))
				if _, err := w.Write(countBuf[:]); err != nil {
					return err
				}
				if _, err := w.Write(data); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// loadBitmapIndex 加载 bitmap.idx，并据此重建 levelCounts。
// 校验每个 bitmap 的 docID 范围，以及 level 覆盖全部文档。
func (s *Store) loadBitmapIndex(path string) error {
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
	if fileSize < bitmapHeaderSize {
		return corruptedf("%s 文件过短（%d 字节）", path, fileSize)
	}
	if err := readMagic(f, bitmapMagic); err != nil {
		return corruptedf("%s: %v", path, err)
	}
	var head [8]byte
	if _, err := io.ReadFull(f, head[:]); err != nil {
		return corruptedf("%s: 读取文件头失败: %v", path, err)
	}
	if version := binary.LittleEndian.Uint32(head[0:4]); version != bitmapFormatVersion {
		return corruptedf("%s 格式版本不支持: %d", path, version)
	}
	fieldCount := binary.LittleEndian.Uint32(head[4:8])
	if uint64(fieldCount) > uint64(fileSize-bitmapHeaderSize)/7 || uint64(fieldCount) > uint64(math.MaxInt) {
		return corruptedf("%s 字段数异常: %d", path, fieldCount)
	}

	reader := bufio.NewReaderSize(f, indexWriteBufferSize)
	fields := make(map[string]map[string]*roaring.Bitmap, int(fieldCount))
	docCount := uint64(len(s.docs))

	for fi := uint32(0); fi < fieldCount; fi++ {
		var lenBuf [2]byte
		if _, err := io.ReadFull(reader, lenBuf[:]); err != nil {
			return corruptedf("%s: 读取第 %d 个字段名长度失败: %v", path, fi, err)
		}
		nameLen := binary.LittleEndian.Uint16(lenBuf[:])
		if nameLen == 0 {
			return corruptedf("%s: 第 %d 个字段名为空", path, fi)
		}
		nameBuf := make([]byte, nameLen)
		if _, err := io.ReadFull(reader, nameBuf); err != nil {
			return corruptedf("%s: 读取第 %d 个字段名失败: %v", path, fi, err)
		}
		name := string(nameBuf)
		if _, exists := fields[name]; exists {
			return corruptedf("%s: 字段 %q 重复", path, name)
		}

		var countBuf [4]byte
		if _, err := io.ReadFull(reader, countBuf[:]); err != nil {
			return corruptedf("%s: 读取字段 %q 的值数量失败: %v", path, name, err)
		}
		valueCount := binary.LittleEndian.Uint32(countBuf[:])
		if uint64(valueCount) > uint64(fileSize)/6 || uint64(valueCount) > uint64(math.MaxInt) {
			return corruptedf("%s: 字段 %q 的值数量异常: %d", path, name, valueCount)
		}

		fieldValues := make(map[string]*roaring.Bitmap, int(valueCount))
		for vi := uint32(0); vi < valueCount; vi++ {
			if _, err := io.ReadFull(reader, lenBuf[:]); err != nil {
				return corruptedf("%s: 读取字段 %q 第 %d 个值长度失败: %v", path, name, vi, err)
			}
			valueLen := binary.LittleEndian.Uint16(lenBuf[:])
			valueBuf := make([]byte, valueLen)
			if _, err := io.ReadFull(reader, valueBuf); err != nil {
				return corruptedf("%s: 读取字段 %q 第 %d 个值失败: %v", path, name, vi, err)
			}
			value := string(valueBuf)
			if _, exists := fieldValues[value]; exists {
				return corruptedf("%s: 字段 %q 的值 %q 重复", path, name, value)
			}

			if _, err := io.ReadFull(reader, countBuf[:]); err != nil {
				return corruptedf("%s: 读取字段 %q 第 %d 个 bitmap 长度失败: %v", path, name, vi, err)
			}
			dataLen := binary.LittleEndian.Uint32(countBuf[:])
			if dataLen == 0 || uint64(dataLen) > uint64(fileSize) {
				return corruptedf("%s: 字段 %q 第 %d 个 bitmap 长度非法", path, name, vi)
			}
			data := make([]byte, dataLen)
			if _, err := io.ReadFull(reader, data); err != nil {
				return corruptedf("%s: 读取字段 %q 第 %d 个 bitmap 失败: %v", path, name, vi, err)
			}
			rb := roaring.New()
			if err := rb.UnmarshalBinary(data); err != nil {
				return corruptedf("%s: 字段 %q 第 %d 个 bitmap 反序列化失败: %v", path, name, vi, err)
			}
			if rb.GetCardinality() > docCount {
				return corruptedf("%s: 字段 %q 的 bitmap 基数超过文档数", path, name)
			}
			if !rb.IsEmpty() && uint64(rb.Maximum()) >= docCount {
				return corruptedf("%s: 字段 %q 的 bitmap 含越界 docID", path, name)
			}
			fieldValues[value] = rb
		}
		fields[name] = fieldValues
	}

	// 文件应恰好读完，不允许尾随垃圾。
	var extra [1]byte
	if n, err := reader.Read(extra[:]); n != 0 {
		return corruptedf("%s 存在多余数据", path)
	} else if err != nil && err != io.EOF {
		return corruptedf("%s: 读取尾部失败: %v", path, err)
	}

	levelCounts := make(map[string]int64)
	levelField, ok := fields[fieldLevel]
	if ok {
		var total uint64
		for value, rb := range levelField {
			card := rb.GetCardinality()
			levelCounts[value] = int64(card)
			total += card
		}
		if total != docCount {
			return corruptedf("%s: level bitmap 覆盖 %d 条文档，与文档总数 %d 不一致", path, total, docCount)
		}
	} else if docCount != 0 {
		return corruptedf("%s: 缺少 level 字段索引", path)
	}

	s.fields = fields
	s.levelCounts = levelCounts
	return nil
}
