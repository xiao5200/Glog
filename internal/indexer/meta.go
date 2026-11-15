package indexer

import (
	"encoding/json"
	"io"
	"maps"
	"os"
	"time"

	"glog/pkg/models"
)

// sourceRange 记录某个日志源的 docID 起始位置与数量。
// 假设同一 source 的日志在导入时基本连续；交错来源时 StartDoc 取首次出现位置。
type sourceRange struct {
	StartDoc uint32 `json:"start_doc"`
	Count    uint64 `json:"count"`
}

// indexMeta 对应 indexes/meta.json。
type indexMeta struct {
	Version      uint32                 `json:"version"`
	DocCount     uint64                 `json:"doc_count"`
	TermCount    uint64                 `json:"term_count"`
	LogSize      int64                  `json:"log_size"`
	IndexSize    int64                  `json:"index_size"`
	TimeRange    *models.TimeRange      `json:"time_range,omitempty"`
	LevelCounts  map[string]int64       `json:"level_counts"`
	BuiltTime    time.Time              `json:"built_time"`
	SourceRanges map[string]sourceRange `json:"source_ranges,omitempty"`
}

// saveMetaIndex 最后写入 meta.json，作为一次 Finalize 的提交记录。
// indexSize 为 docs/inverted/time/bitmap/bloom 五个索引文件大小之和
// （不含 meta.json 自身；Stats 中的 IndexSizeBytes 直接统计目录实际大小）。
func (s *Store) saveMetaIndex(path string, indexSize int64) (int64, error) {
	built := time.Now().UTC()
	meta := indexMeta{
		Version:      metaVersion,
		DocCount:     uint64(len(s.docs)),
		TermCount:    uint64(len(s.terms)),
		LogSize:      s.logOffset,
		IndexSize:    indexSize,
		LevelCounts:  make(map[string]int64, len(s.levelCounts)),
		BuiltTime:    built,
		SourceRanges: make(map[string]sourceRange, len(s.sourceRanges)),
	}
	for level, count := range s.levelCounts {
		meta.LevelCounts[level] = count
	}
	for source, r := range s.sourceRanges {
		meta.SourceRanges[source] = r
	}
	if s.hasTime {
		meta.TimeRange = &models.TimeRange{
			Start: nsToTime(s.timeMin),
			End:   nsToTime(s.timeMax),
		}
	}

	data, err := json.MarshalIndent(&meta, "", "  ")
	if err != nil {
		return 0, err
	}
	data = append(data, '\n')

	size, err := writeIndexAtomic(path, func(w io.Writer) error {
		_, werr := w.Write(data)
		return werr
	})
	if err != nil {
		return 0, err
	}
	s.builtTime = built
	return size, nil
}

// loadMetaIndex 加载 meta.json，并校验其计数、大小、时间范围与级别统计
// 与已加载的其它索引一致。任意不一致都视为索引不可信，触发重建。
func (s *Store) loadMetaIndex(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var meta indexMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return corruptedf("%s: 解析失败: %v", path, err)
	}
	if meta.Version != metaVersion {
		return corruptedf("%s 版本不支持: %d", path, meta.Version)
	}
	if meta.DocCount != uint64(len(s.docs)) {
		return corruptedf("%s doc_count 不一致（%d != %d）", path, meta.DocCount, len(s.docs))
	}
	if meta.TermCount != uint64(len(s.terms)) {
		return corruptedf("%s term_count 不一致（%d != %d）", path, meta.TermCount, len(s.terms))
	}
	if meta.LogSize != s.logOffset {
		return corruptedf("%s log_size 不一致（%d != %d）", path, meta.LogSize, s.logOffset)
	}
	if s.hasTime {
		if meta.TimeRange == nil ||
			meta.TimeRange.Start.UnixNano() != s.timeMin ||
			meta.TimeRange.End.UnixNano() != s.timeMax {
			return corruptedf("%s time_range 不一致", path)
		}
	} else if meta.TimeRange != nil {
		return corruptedf("%s 不应包含 time_range", path)
	}
	if !maps.Equal(meta.LevelCounts, s.levelCounts) {
		return corruptedf("%s level_counts 不一致", path)
	}

	ranges := make(map[string]sourceRange, len(meta.SourceRanges))
	for source, r := range meta.SourceRanges {
		if source == "" || r.Count == 0 {
			return corruptedf("%s source_ranges 含非法项", path)
		}
		if uint64(r.StartDoc) >= uint64(len(s.docs)) {
			return corruptedf("%s source_ranges 起始 docID 越界", path)
		}
		ranges[source] = r
	}
	s.sourceRanges = ranges
	s.builtTime = meta.BuiltTime
	return nil
}
