package indexer

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/RoaringBitmap/roaring"
	"github.com/pierrec/lz4/v4"

	"glog/internal/errs"
	"glog/internal/logx"
	"glog/pkg/models"
)

const (
	logWriteBufferSize = 1 << 20
	logFlushBatch      = 1000

	rawDirName   = "raw"
	indexDirName = "indexes"
	logsFileName = "logs.dat"

	docsFileName     = "docs.idx"
	invertedFileName = "inverted.idx"
	timeFileName     = "time.idx"
	bitmapFileName   = "bitmap.idx"
	bloomFileName    = "bloom.idx"
	metaFileName     = "meta.json"
)

var errStoreClosed = errors.New("日志存储已关闭")

// Store 是单个项目的日志存储与内存索引。
//
// 内存策略（按 1000 万条设计）：
//   - 日志正文只保存在 raw/logs.dat，内存中不保留 JSON 副本；
//     docMeta 每文档约 24 字节，ReadDoc 时才解压解码。
//   - 倒排索引为 map[string][]uint32，posting 中每个 docID 4 字节；
//     词项字符串在 map 中只保存一份。
//   - 时间索引常驻的只是按时间排序的 docID（每文档 4 字节），
//     时间戳保存在 docMeta 中，避免重复保存 (ts, docID) 对。
//   - 字段 bitmap 使用 roaring，按实际基数压缩。
//   - Bloom 每小时桶 128 KiB，最多 720 个桶，约 90 MiB 上限。
//
// 并发策略：进程内使用 mu 保护全部可变状态。Append/Finalize/Rebuild/Close
// 使用写锁；查询类方法使用读锁。Postings 返回内部切片的只读视图
// （见其注释），其余结果均返回副本。
type Store struct {
	mu sync.RWMutex

	projectDir string
	rawDir     string
	indexDir   string
	logPath    string

	file      *os.File
	logWriter *bufio.Writer
	logOffset int64 // 日志文件逻辑大小，包含已写入 buffer 的数据
	closed    bool

	// 日志块压缩缓冲复用，避免逐条分配。容量随最大单条日志增长。
	compressBuf []byte
	// compressor 复用 LZ4 压缩状态（含哈希表），只在写锁内使用。
	compressor lz4.Compressor

	docs []docMeta

	// terms: 词项 -> 升序 docID 列表。posting 只追加，不修改既有元素。
	terms map[string][]uint32

	// timeOrder 是按时间戳（相同则按 docID）升序排列的 docID。
	// Append 只置 dirty，真正需要时间查询/Finalize 时再重建。
	timeOrder []uint32
	timeDirty bool
	timeMin   int64
	timeMax   int64
	hasTime   bool

	fields      map[string]map[string]*roaring.Bitmap
	levelCounts map[string]int64

	bloomBuckets map[int64]*bloomBucket

	sourceRanges map[string]sourceRange
	builtTime    time.Time
}

// Open 打开（或创建）项目目录下的日志存储。
//
// 目录结构：<projectDir>/raw/logs.dat 与 <projectDir>/indexes/*。
// indexes 存在且完整时全量加载；缺失、损坏或落后于 logs.dat 时自动从
// logs.dat 逐条重建并写回索引。日志文件自身损坏时返回 errs.ErrIndexCorrupted。
func Open(projectDir string) (*Store, error) {
	if strings.TrimSpace(projectDir) == "" {
		return nil, fmt.Errorf("%w: 项目目录不能为空", errs.ErrInvalidInput)
	}
	projectDir = filepath.Clean(projectDir)
	rawDir := filepath.Join(projectDir, rawDirName)
	indexDir := filepath.Join(projectDir, indexDirName)
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建 raw 目录失败: %w", err)
	}
	if err := os.MkdirAll(indexDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建 indexes 目录失败: %w", err)
	}
	cleanupTempFiles(indexDir)

	logPath := filepath.Join(rawDir, logsFileName)
	f, err := os.OpenFile(logPath, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, fmt.Errorf("打开日志文件失败: %w", err)
	}
	success := false
	defer func() {
		if !success {
			_ = f.Close()
		}
	}()

	logSize, err := initLogFile(f)
	if err != nil {
		return nil, err
	}

	s := newStore(projectDir, rawDir, indexDir, logPath, f, logSize)

	if err := s.loadIndexes(); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			logx.Info(fmt.Sprintf("索引文件缺失，正在从 raw/logs.dat 重建: %v", err))
		} else {
			logx.Warn(fmt.Sprintf("索引加载失败，正在从 raw/logs.dat 重建: %v", err))
		}
		if err := s.rebuildLocked(); err != nil {
			return nil, err
		}
		if err := s.finalizeLocked(); err != nil {
			return nil, err
		}
	}
	if err := s.openLogWriterLocked(); err != nil {
		return nil, err
	}
	success = true
	return s, nil
}

// initLogFile 校验或初始化 logs.dat 文件头，返回当前文件大小。
func initLogFile(f *os.File) (int64, error) {
	st, err := f.Stat()
	if err != nil {
		return 0, fmt.Errorf("读取日志文件状态失败: %w", err)
	}
	size := st.Size()
	if size == 0 {
		var hdr [logsHeaderSize]byte
		copy(hdr[:8], []byte(logsMagic))
		binary.LittleEndian.PutUint32(hdr[8:12], formatVersion)
		if _, err := f.WriteAt(hdr[:], 0); err != nil {
			return 0, fmt.Errorf("写入日志文件头失败: %w", err)
		}
		return int64(logsHeaderSize), nil
	}
	if size < logsHeaderSize {
		return 0, corruptedf("raw/logs.dat 文件过短（%d 字节）", size)
	}
	var hdr [logsHeaderSize]byte
	if _, err := f.ReadAt(hdr[:], 0); err != nil {
		return 0, corruptedf("读取 raw/logs.dat 文件头失败: %v", err)
	}
	if string(hdr[:8]) != logsMagic {
		return 0, corruptedf("raw/logs.dat magic 不正确: %q", string(hdr[:8]))
	}
	if version := binary.LittleEndian.Uint32(hdr[8:12]); version != formatVersion {
		return 0, corruptedf("raw/logs.dat 格式版本不支持: %d", version)
	}
	return size, nil
}

func newStore(projectDir, rawDir, indexDir, logPath string, f *os.File, logSize int64) *Store {
	return &Store{
		projectDir:   projectDir,
		rawDir:       rawDir,
		indexDir:     indexDir,
		logPath:      logPath,
		file:         f,
		logOffset:    logSize,
		docs:         make([]docMeta, 0, 1024),
		terms:        make(map[string][]uint32),
		fields:       make(map[string]map[string]*roaring.Bitmap, 4),
		levelCounts:  make(map[string]int64),
		bloomBuckets: make(map[int64]*bloomBucket),
		sourceRanges: make(map[string]sourceRange),
	}
}

// openLogWriterLocked 将文件定位到末尾并创建缓冲写入器。已存在时直接返回。
func (s *Store) openLogWriterLocked() error {
	if s.logWriter != nil {
		return nil
	}
	if _, err := s.file.Seek(s.logOffset, io.SeekStart); err != nil {
		return fmt.Errorf("定位日志文件末尾失败: %w", err)
	}
	s.logWriter = bufio.NewWriterSize(s.file, logWriteBufferSize)
	return nil
}

// loadIndexes 按依赖顺序加载全部索引文件。任一文件缺失/损坏都会返回错误，
// 由 Open 决定是否从日志重建。
func (s *Store) loadIndexes() error {
	if err := s.loadDocsIndex(filepath.Join(s.indexDir, docsFileName)); err != nil {
		return err
	}
	if err := s.loadTimeIndex(filepath.Join(s.indexDir, timeFileName)); err != nil {
		return err
	}
	if err := s.loadInvertedIndex(filepath.Join(s.indexDir, invertedFileName)); err != nil {
		return err
	}
	if err := s.loadBitmapIndex(filepath.Join(s.indexDir, bitmapFileName)); err != nil {
		return err
	}
	if err := s.loadBloomIndex(filepath.Join(s.indexDir, bloomFileName)); err != nil {
		return err
	}
	return s.loadMetaIndex(filepath.Join(s.indexDir, metaFileName))
}

// Append 追加一批日志。
//
// 每条 entry 会被就地补全：DocID 从当前文档数继续递增，ID 为 DocID 十进制字符串，
// 零值 Timestamp 使用 time.Now()，空 Level 使用 UNKNOWN，nil Fields 初始化为空 map。
// 日志按 1000 条一批刷新内部 bufio.Writer；Append 返回时缓冲区已刷新。
//
// 出错时可能已持久化本批次中的前若干条日志并更新了内存索引；调用方应停止
// 继续写入并重新 Open（会自动重建索引，截断不完整的日志尾部）。
func (s *Store) Append(entries []*models.LogEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return errStoreClosed
	}
	if len(entries) == 0 {
		return nil
	}
	if err := s.openLogWriterLocked(); err != nil {
		return err
	}

	pending := 0
	skipped := 0
	for _, entry := range entries {
		if entry == nil {
			skipped++
			continue
		}
		if err := s.appendOneLocked(entry); err != nil {
			// 尽力把已经写入缓冲器的数据刷出，便于后续日志重建定位完整块。
			_ = s.flushLogLocked()
			return err
		}
		pending++
		if pending >= logFlushBatch {
			if err := s.flushLogLocked(); err != nil {
				return err
			}
			pending = 0
		}
	}
	if err := s.flushLogLocked(); err != nil {
		return err
	}
	if skipped > 0 {
		logx.Warn(fmt.Sprintf("Append 跳过 %d 条空日志", skipped))
	}
	return nil
}

// appendOneLocked 写入一条日志块，并把 (ts, docID) 追加到内存索引。
func (s *Store) appendOneLocked(entry *models.LogEntry) error {
	if uint64(len(s.docs)) >= uint64(math.MaxUint32) {
		return fmt.Errorf("%w: 文档数量超过 uint32 上限", errs.ErrInvalidInput)
	}
	docID := uint32(len(s.docs))
	entry.DocID = uint64(docID)
	entry.ID = strconv.FormatUint(uint64(docID), 10)
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}
	if entry.Level == "" {
		entry.Level = models.LevelUnknown
	}
	if entry.Fields == nil {
		entry.Fields = make(map[string]interface{})
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("%w: 序列化日志（docID=%d）失败: %v", errs.ErrInvalidInput, docID, err)
	}
	if len(data) == 0 || len(data) > maxLogBlockSize {
		return fmt.Errorf("%w: 单条日志过大（docID=%d，%d 字节）", errs.ErrInvalidInput, docID, len(data))
	}

	blockOffset := s.logOffset
	storedLen, rawLen, err := s.writeLogBlockLocked(data)
	if err != nil {
		return err
	}

	ts := entry.Timestamp.UnixNano()
	tokens := Tokenize(entry.Message)
	if len(tokens) == 0 && entry.Message == "" && entry.RawLog != "" {
		// Message 为空时退回原始日志文本，避免未解析日志无法被关键词检索。
		tokens = Tokenize(entry.RawLog)
	}

	s.docs = append(s.docs, docMeta{
		offset:    uint64(blockOffset),
		storedLen: storedLen,
		rawLen:    rawLen,
		ts:        ts,
	})
	s.indexEntryLocked(docID, entry, tokens)
	return nil
}

// writeLogBlockLocked 压缩并写入一个日志块，返回落盘长度与原始长度。
// 压缩不划算或压缩失败时按未压缩写入（此时 compressedLen == uncompressedLen）。
func (s *Store) writeLogBlockLocked(raw []byte) (storedLen, rawLen uint32, err error) {
	bound := lz4.CompressBlockBound(len(raw))
	if cap(s.compressBuf) < bound {
		s.compressBuf = make([]byte, bound)
	}
	dst := s.compressBuf[:bound]

	payload := raw
	if n, cerr := s.compressor.CompressBlock(raw, dst); cerr == nil && n > 0 && n < len(raw) {
		payload = dst[:n]
	}
	if len(payload) > math.MaxUint32 || len(raw) > math.MaxUint32 {
		return 0, 0, fmt.Errorf("%w: 日志块长度超过 uint32 上限", errs.ErrInvalidInput)
	}

	var hdr [8]byte
	binary.LittleEndian.PutUint32(hdr[0:4], uint32(len(payload)))
	binary.LittleEndian.PutUint32(hdr[4:8], uint32(len(raw)))
	if _, werr := s.logWriter.Write(hdr[:]); werr != nil {
		return 0, 0, fmt.Errorf("写入日志块头失败: %w", werr)
	}
	if _, werr := s.logWriter.Write(payload); werr != nil {
		return 0, 0, fmt.Errorf("写入日志块失败: %w", werr)
	}
	s.logOffset += int64(8 + len(payload))
	return uint32(len(payload)), uint32(len(raw)), nil
}

// indexEntryLocked 把已落盘的日志加入全部内存索引。
func (s *Store) indexEntryLocked(docID uint32, entry *models.LogEntry, tokens []string) {
	for _, term := range tokens {
		s.terms[term] = append(s.terms[term], docID)
	}

	ts := entry.Timestamp.UnixNano()
	s.bloomAddLocked(ts, tokens)
	s.addEntryFieldsLocked(entry, docID)
	s.levelCounts[entry.Level]++

	if entry.Source != "" {
		r := s.sourceRanges[entry.Source]
		if r.Count == 0 {
			r.StartDoc = docID
		}
		r.Count++
		s.sourceRanges[entry.Source] = r
	}

	if !s.hasTime {
		s.timeMin = ts
		s.timeMax = ts
		s.hasTime = true
	} else {
		if ts < s.timeMin {
			s.timeMin = ts
		}
		if ts > s.timeMax {
			s.timeMax = ts
		}
	}
	s.timeDirty = true
}

// flushLogLocked 刷新日志写入缓冲。
func (s *Store) flushLogLocked() error {
	if s.logWriter == nil {
		return nil
	}
	if err := s.logWriter.Flush(); err != nil {
		return fmt.Errorf("刷新日志写入缓冲失败: %w", err)
	}
	return nil
}

// Flush 刷新日志写入缓冲（不写索引文件）。
func (s *Store) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errStoreClosed
	}
	return s.flushLogLocked()
}

// Finalize 将内存索引原子地全量写入 indexes 目录，并刷新 logs.dat。
// 可重复调用（幂等）。
func (s *Store) Finalize() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errStoreClosed
	}
	return s.finalizeLocked()
}

func (s *Store) finalizeLocked() error {
	if err := s.flushLogLocked(); err != nil {
		return err
	}
	if s.file != nil {
		if err := s.file.Sync(); err != nil {
			return fmt.Errorf("同步日志文件失败: %w", err)
		}
	}

	docsSize, err := s.saveDocsIndex(filepath.Join(s.indexDir, docsFileName))
	if err != nil {
		return fmt.Errorf("写入 %s 失败: %w", docsFileName, err)
	}
	invertedSize, err := s.saveInvertedIndex(filepath.Join(s.indexDir, invertedFileName))
	if err != nil {
		return fmt.Errorf("写入 %s 失败: %w", invertedFileName, err)
	}
	timeSize, err := s.saveTimeIndex(filepath.Join(s.indexDir, timeFileName))
	if err != nil {
		return fmt.Errorf("写入 %s 失败: %w", timeFileName, err)
	}
	bitmapSize, err := s.saveBitmapIndex(filepath.Join(s.indexDir, bitmapFileName))
	if err != nil {
		return fmt.Errorf("写入 %s 失败: %w", bitmapFileName, err)
	}
	bloomSize, err := s.saveBloomIndex(filepath.Join(s.indexDir, bloomFileName))
	if err != nil {
		return fmt.Errorf("写入 %s 失败: %w", bloomFileName, err)
	}
	indexSize := docsSize + invertedSize + timeSize + bitmapSize + bloomSize
	if _, err := s.saveMetaIndex(filepath.Join(s.indexDir, metaFileName), indexSize); err != nil {
		return fmt.Errorf("写入 %s 失败: %w", metaFileName, err)
	}
	return nil
}

// Rebuild 丢弃内存索引，从 raw/logs.dat 全量重放并立即持久化。
func (s *Store) Rebuild() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errStoreClosed
	}
	if err := s.rebuildLocked(); err != nil {
		return err
	}
	return s.finalizeLocked()
}

// rebuildLocked 从 logs.dat 顺序解码日志并重建内存索引。
// 末尾不完整的块（通常是写入过程中崩溃留下的）会被截断；中间损坏返回错误。
func (s *Store) rebuildLocked() error {
	if s.logWriter != nil {
		if err := s.logWriter.Flush(); err != nil {
			return fmt.Errorf("刷新日志写入缓冲失败: %w", err)
		}
		s.logWriter = nil
	}

	var hdr [logsHeaderSize]byte
	if _, err := s.file.ReadAt(hdr[:], 0); err != nil {
		return corruptedf("读取 raw/logs.dat 文件头失败: %v", err)
	}
	if string(hdr[:8]) != logsMagic {
		return corruptedf("raw/logs.dat magic 不正确")
	}
	if version := binary.LittleEndian.Uint32(hdr[8:12]); version != formatVersion {
		return corruptedf("raw/logs.dat 格式版本不支持: %d", version)
	}

	st, err := s.file.Stat()
	if err != nil {
		return fmt.Errorf("读取日志文件状态失败: %w", err)
	}
	fileSize := st.Size()
	if fileSize < logsHeaderSize {
		return corruptedf("raw/logs.dat 文件过短（%d 字节）", fileSize)
	}

	s.resetIndexesLocked()

	section := io.NewSectionReader(s.file, int64(logsHeaderSize), fileSize-int64(logsHeaderSize))
	reader := bufio.NewReaderSize(section, indexWriteBufferSize)

	pos := int64(logsHeaderSize)
	docID := uint32(0)
	truncated := false

	for pos < fileSize {
		blockStart := pos
		var blockHdr [8]byte
		if _, rerr := io.ReadFull(reader, blockHdr[:]); rerr != nil {
			if rerr == io.EOF || rerr == io.ErrUnexpectedEOF {
				truncated = true
				break
			}
			return fmt.Errorf("读取日志块头失败: %w", rerr)
		}
		storedLen := binary.LittleEndian.Uint32(blockHdr[0:4])
		rawLen := binary.LittleEndian.Uint32(blockHdr[4:8])
		if storedLen == 0 || rawLen == 0 || storedLen > rawLen || rawLen > maxLogBlockSize {
			return corruptedf("日志块头非法（offset=%d, storedLen=%d, rawLen=%d）", blockStart, storedLen, rawLen)
		}
		blockEnd := blockStart + 8 + int64(storedLen)
		if blockEnd > fileSize {
			truncated = true
			break
		}
		payload := make([]byte, storedLen)
		if _, rerr := io.ReadFull(reader, payload); rerr != nil {
			if rerr == io.EOF || rerr == io.ErrUnexpectedEOF {
				truncated = true
				break
			}
			return fmt.Errorf("读取日志块数据失败: %w", rerr)
		}

		raw := decodeLogPayload(payload, rawLen)
		var entry models.LogEntry
		if jerr := json.Unmarshal(raw, &entry); jerr != nil {
			return corruptedf("日志块解析失败（offset=%d）: %v", blockStart, jerr)
		}
		if uint64(docID) >= uint64(math.MaxUint32) {
			return corruptedf("日志数量超过 uint32 上限")
		}
		normalizeEntry(&entry, docID)
		ts := entry.Timestamp.UnixNano()
		tokens := Tokenize(entry.Message)
		if len(tokens) == 0 && entry.Message == "" && entry.RawLog != "" {
			tokens = Tokenize(entry.RawLog)
		}

		s.docs = append(s.docs, docMeta{
			offset:    uint64(blockStart),
			storedLen: storedLen,
			rawLen:    rawLen,
			ts:        ts,
		})
		s.indexEntryLocked(docID, &entry, tokens)
		docID++
		pos = blockEnd
	}

	if truncated {
		logx.Warn(fmt.Sprintf("raw/logs.dat 末尾存在不完整日志块，已截断至偏移 %d", pos))
		if terr := s.file.Truncate(pos); terr != nil {
			return fmt.Errorf("截断不完整日志块失败: %w", terr)
		}
	}
	s.logOffset = pos
	return s.openLogWriterLocked()
}

// resetIndexesLocked 清空全部内存索引状态，但保留文件句柄与压缩缓冲。
func (s *Store) resetIndexesLocked() {
	s.docs = make([]docMeta, 0, 1024)
	s.terms = make(map[string][]uint32)
	s.timeOrder = nil
	s.timeDirty = false
	s.timeMin = 0
	s.timeMax = 0
	s.hasTime = false
	s.fields = make(map[string]map[string]*roaring.Bitmap, 4)
	s.levelCounts = make(map[string]int64)
	s.bloomBuckets = make(map[int64]*bloomBucket)
	s.sourceRanges = make(map[string]sourceRange)
	s.builtTime = time.Time{}
}

// decodeLogPayload 解压日志块。block 格式约定 compressedLen == uncompressedLen
// 表示未压缩；UncompressBlock 失败时同样按未压缩数据处理（由随后的 JSON 解码
// 决定数据是否真的可用）。
func decodeLogPayload(payload []byte, rawLen uint32) []byte {
	if len(payload) == int(rawLen) {
		return payload
	}
	if int(rawLen) > maxLogBlockSize {
		return payload
	}
	dst := make([]byte, int(rawLen))
	if n, err := lz4.UncompressBlock(payload, dst); err == nil && n == int(rawLen) {
		return dst
	}
	return payload
}

// normalizeEntry 在读取/重放时规范化内部标识。
func normalizeEntry(entry *models.LogEntry, docID uint32) {
	entry.DocID = uint64(docID)
	entry.ID = strconv.FormatUint(uint64(docID), 10)
	if entry.Level == "" {
		entry.Level = models.LevelUnknown
	}
	if entry.Fields == nil {
		entry.Fields = make(map[string]interface{})
	}
}

// DocCount 返回当前文档总数。
func (s *Store) DocCount() uint32 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return uint32(len(s.docs))
}

// AllDocIDs 返回 [0, DocCount) 的新切片（调用方可自由修改）。
func (s *Store) AllDocIDs() []uint32 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]uint32, len(s.docs))
	for i := range ids {
		ids[i] = uint32(i)
	}
	return ids
}

// ReadDoc 读取并解码单条日志。docID 越界返回 errs.ErrNotFound；
// IO 失败、解压失败或 JSON 损坏返回包装后的 errs.ErrIndexCorrupted。
func (s *Store) ReadDoc(docID uint32) (*models.LogEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return nil, errStoreClosed
	}
	return s.readDocLocked(docID)
}

func (s *Store) readDocLocked(docID uint32) (*models.LogEntry, error) {
	if uint64(docID) >= uint64(len(s.docs)) {
		return nil, fmt.Errorf("%w: 文档 %d 不存在", errs.ErrNotFound, docID)
	}
	m := &s.docs[docID]
	if m.offset > uint64(math.MaxInt64)-8 {
		return nil, corruptedf("文档 %d 的偏移非法", docID)
	}
	if m.storedLen == 0 || m.rawLen == 0 || m.storedLen > m.rawLen || m.rawLen > maxLogBlockSize {
		return nil, corruptedf("文档 %d 的块长度非法", docID)
	}
	payload := make([]byte, m.storedLen)
	if _, err := s.file.ReadAt(payload, int64(m.offset)+8); err != nil {
		return nil, corruptedf("读取文档 %d 失败: %v", docID, err)
	}
	raw := decodeLogPayload(payload, m.rawLen)
	var entry models.LogEntry
	if err := json.Unmarshal(raw, &entry); err != nil {
		return nil, corruptedf("解析文档 %d 失败: %v", docID, err)
	}
	normalizeEntry(&entry, docID)
	return &entry, nil
}

// ReadDocs 按输入顺序批量读取日志。任一 docID 失败即返回错误。
func (s *Store) ReadDocs(docIDs []uint32) ([]*models.LogEntry, error) {
	if len(docIDs) == 0 {
		return []*models.LogEntry{}, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return nil, errStoreClosed
	}
	out := make([]*models.LogEntry, len(docIDs))
	for i, docID := range docIDs {
		entry, err := s.readDocLocked(docID)
		if err != nil {
			return nil, err
		}
		out[i] = entry
	}
	return out, nil
}

// LevelCounts 返回级别计数的副本。
func (s *Store) LevelCounts() map[string]int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]int64, len(s.levelCounts))
	for k, v := range s.levelCounts {
		out[k] = v
	}
	return out
}

// Stats 从内存统计与 indexes/logs.dat 文件大小生成索引统计。
// CacheHitRate 由上层查询缓存负责，这里固定返回 0。
func (s *Store) Stats() models.IndexStatistics {
	s.mu.RLock()
	logSize := s.logOffset
	if s.file != nil {
		if info, err := s.file.Stat(); err == nil {
			logSize = info.Size()
		}
	}
	stat := models.IndexStatistics{
		TotalDocuments: int64(len(s.docs)),
		UniqueTerms:    int64(len(s.terms)),
		LogSizeBytes:   logSize,
		LevelCounts:    make(map[string]int64, len(s.levelCounts)),
		CacheHitRate:   0,
		LastBuildTime:  s.builtTime,
	}
	for k, v := range s.levelCounts {
		stat.LevelCounts[k] = v
	}
	if s.hasTime {
		stat.TimeRange = &models.TimeRange{
			Start: nsToTime(s.timeMin),
			End:   nsToTime(s.timeMax),
		}
	}
	s.mu.RUnlock()

	stat.IndexSizeBytes = dirSize(s.indexDir)
	return stat
}

// Close 刷新并关闭日志文件句柄。可重复调用。
// 若还希望把内存索引落盘，应在 Close 之前调用 Finalize。
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true

	var firstErr error
	if s.logWriter != nil {
		if err := s.logWriter.Flush(); err != nil {
			firstErr = err
		}
		s.logWriter = nil
	}
	if s.file != nil {
		if firstErr == nil {
			if err := s.file.Sync(); err != nil {
				firstErr = err
			}
		}
		if err := s.file.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		s.file = nil
	}
	return firstErr
}

// cleanupTempFiles 删除上次 Finalize 崩溃遗留的 *.tmp 文件。
func cleanupTempFiles(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tmp") {
			continue
		}
		_ = os.Remove(filepath.Join(dir, entry.Name()))
	}
}

// dirSize 统计目录下普通文件（忽略 .tmp）的总字节数。
func dirSize(dir string) int64 {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	var total int64
	for _, entry := range entries {
		if entry.IsDir() || strings.HasSuffix(entry.Name(), ".tmp") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		total += info.Size()
	}
	return total
}
