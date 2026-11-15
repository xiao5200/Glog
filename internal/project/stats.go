package project

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"glog/internal/errs"
	"glog/pkg/models"
)

// RecomputeStats 根据日志源重新汇总项目统计，并更新数据库与 project.meta。
func (s *Service) RecomputeStats(id string) (*models.ProjectStatistics, error) {
	dir, err := s.projectDirChecked(id)
	if err != nil {
		return nil, errs.InvalidInput("项目ID无效", err)
	}
	p, err := s.store.GetProject(id)
	if err != nil {
		return nil, err
	}
	sources, err := s.store.ListLogSources(id)
	if err != nil {
		return nil, errs.Internal("读取项目日志源失败", err)
	}

	stats := &models.ProjectStatistics{
		LevelDistribution: map[string]int64{},
		TopHosts:          []models.HostStat{},
	}
	hostCounts := make(map[string]int64)
	var mergedRange *models.TimeRange

	for _, src := range sources {
		stats.TotalLogs += src.LogCount
		stats.SizeBytes += src.SizeBytes
		mergedRange = mergeTimeRange(mergedRange, src.TimeRange)
		if host := strings.TrimSpace(src.Host); host != "" {
			hostCounts[host] += src.LogCount
		}
	}
	stats.TimeRange = mergedRange
	stats.SourceCount = len(sources)
	stats.LevelDistribution = cloneInt64Map(p.Levels)

	// 可选读取索引 meta 中的级别分布与词条数；结构不匹配时忽略。
	if idxMeta, ok := readIndexMeta(dir); ok {
		levels := idxMeta.LevelCounts
		if len(levels) == 0 {
			levels = idxMeta.LevelDistribution
		}
		if len(levels) > 0 {
			stats.LevelDistribution = cloneInt64Map(levels)
		}
		switch {
		case idxMeta.UniqueTerms > 0:
			stats.UniqueTerms = idxMeta.UniqueTerms
		case idxMeta.TermCount > 0:
			stats.UniqueTerms = idxMeta.TermCount
		case idxMeta.TotalTerms > 0:
			stats.UniqueTerms = idxMeta.TotalTerms
		}
	}
	stats.IndexSizeBytes = directorySize(filepath.Join(dir, "indexes"))

	top := make([]models.HostStat, 0, len(hostCounts))
	for host, count := range hostCounts {
		top = append(top, models.HostStat{Host: host, LogCount: count})
	}
	sort.Slice(top, func(i, j int) bool {
		if top[i].LogCount != top[j].LogCount {
			return top[i].LogCount > top[j].LogCount
		}
		return top[i].Host < top[j].Host
	})
	if len(top) > 10 {
		top = top[:10]
	}
	stats.TopHosts = top

	p.LogCount = stats.TotalLogs
	p.SizeBytes = stats.SizeBytes
	p.TimeRange = stats.TimeRange
	p.Levels = stats.LevelDistribution
	p.UpdatedAt = time.Now().UTC()
	if err := s.store.UpdateProject(p); err != nil {
		return nil, err
	}
	p.Sources = sources
	if err := s.WriteMeta(p); err != nil {
		return nil, errs.Internal("统计已更新，但写入 project.meta 失败", err)
	}
	return stats, nil
}

// RecordCollection 记录一次采集结果：创建或累加日志源，累加项目统计，更新
// 数据库与 project.meta。
func (s *Service) RecordCollection(projectID string, src *models.LogSource, logCount, sizeBytes int64, tr *models.TimeRange, levels map[string]int64) error {
	if s == nil || s.store == nil {
		return errs.Internal("项目服务未初始化", nil)
	}
	if err := validateProjectID(projectID); err != nil {
		return errs.InvalidInput("项目ID无效", err)
	}
	if src == nil {
		return errs.InvalidInput("日志源不能为空", nil)
	}
	if logCount < 0 || sizeBytes < 0 {
		return errs.InvalidInput("日志条数与大小不能为负数", nil)
	}
	if tr != nil && !tr.Start.IsZero() && !tr.End.IsZero() && tr.Start.After(tr.End) {
		return errs.InvalidInput("日志时间范围无效：开始时间晚于结束时间", nil)
	}

	p, err := s.store.GetProject(projectID)
	if err != nil {
		return err
	}

	projectSources, err := s.store.ListLogSources(projectID)
	if err != nil {
		return errs.Internal("读取项目日志源失败", err)
	}

	var existing *models.LogSource
	if id := strings.TrimSpace(src.ID); id != "" {
		for i := range projectSources {
			if projectSources[i].ID == id {
				existing = &projectSources[i]
				break
			}
		}
		if existing == nil {
			// ID 已存在于其他项目时拒绝，避免跨项目数据串写。
			if _, err := s.store.GetLogSource(id); err == nil {
				return errs.InvalidInput("日志源已属于其他项目，不能跨项目更新", nil)
			} else if !errors.Is(err, errs.ErrNotFound) {
				return errs.Internal("检查日志源归属失败", err)
			}
		}
	}

	switch {
	case existing != nil:
		merged := *src
		merged.ID = existing.ID
		merged.CreatedAt = existing.CreatedAt
		merged.LogCount = existing.LogCount + logCount
		merged.SizeBytes = existing.SizeBytes + sizeBytes
		merged.TimeRange = mergeTimeRange(existing.TimeRange, tr)
		if strings.TrimSpace(merged.Type) == "" {
			merged.Type = existing.Type
		}
		if strings.TrimSpace(merged.Host) == "" {
			merged.Host = existing.Host
		}
		if strings.TrimSpace(merged.Path) == "" {
			merged.Path = existing.Path
		}
		if strings.TrimSpace(merged.ParseMode) == "" {
			merged.ParseMode = existing.ParseMode
		}
		if strings.TrimSpace(merged.TemplateID) == "" {
			merged.TemplateID = existing.TemplateID
		}
		if strings.TrimSpace(merged.AIProvider) == "" {
			merged.AIProvider = existing.AIProvider
		}
		if err := s.store.UpdateLogSource(&merged); err != nil {
			return err
		}
		*src = merged
	default:
		if strings.TrimSpace(src.ID) == "" {
			src.ID = uuid.NewString()
		}
		if src.CreatedAt.IsZero() {
			src.CreatedAt = time.Now().UTC()
		}
		if strings.TrimSpace(src.Type) == "" {
			src.Type = "local"
		}
		if strings.TrimSpace(src.ParseMode) == "" {
			src.ParseMode = "builtin"
		}
		src.LogCount = logCount
		src.SizeBytes = sizeBytes
		src.TimeRange = mergeTimeRange(nil, tr)
		if err := s.store.CreateLogSourceForProject(projectID, src); err != nil {
			return err
		}
	}

	p.LogCount += logCount
	p.SizeBytes += sizeBytes
	p.TimeRange = mergeTimeRange(p.TimeRange, tr)
	if p.Levels == nil {
		p.Levels = map[string]int64{}
	}
	for level, count := range levels {
		if strings.TrimSpace(level) == "" || count <= 0 {
			continue
		}
		p.Levels[level] += count
	}
	p.UpdatedAt = time.Now().UTC()

	if err := s.store.UpdateProject(p); err != nil {
		return err
	}

	sources, err := s.store.ListLogSources(projectID)
	if err != nil {
		return errs.Internal("项目统计已更新，但读取日志源失败", err)
	}
	p.Sources = sources
	if err := s.WriteMeta(p); err != nil {
		return errs.Internal("项目统计已更新，但写入 project.meta 失败", err)
	}
	return nil
}

// ListSources 列出项目下的日志源。
func (s *Service) ListSources(projectID string) ([]models.LogSource, error) {
	if s == nil || s.store == nil {
		return nil, errs.Internal("项目服务未初始化", nil)
	}
	if err := validateProjectID(projectID); err != nil {
		return nil, errs.InvalidInput("项目ID无效", err)
	}
	sources, err := s.store.ListLogSources(projectID)
	if err != nil {
		return nil, err
	}
	if sources == nil {
		sources = []models.LogSource{}
	}
	return sources, nil
}

// mergeTimeRange 合并两个时间范围：取最早开始与最晚结束，不修改入参。
func mergeTimeRange(base, add *models.TimeRange) *models.TimeRange {
	if add == nil || (add.Start.IsZero() && add.End.IsZero()) {
		return base
	}
	out := &models.TimeRange{}
	if base != nil {
		out.Start = base.Start
		out.End = base.End
	}
	if !add.Start.IsZero() && (out.Start.IsZero() || add.Start.Before(out.Start)) {
		out.Start = add.Start.UTC()
	}
	if !add.End.IsZero() && (out.End.IsZero() || add.End.After(out.End)) {
		out.End = add.End.UTC()
	}
	if out.Start.IsZero() && out.End.IsZero() {
		return nil
	}
	return out
}

// projectIndexMeta 是 indexes/meta.json 中可选字段的宽松映射。
type projectIndexMeta struct {
	LevelCounts       map[string]int64 `json:"level_counts"`
	LevelDistribution map[string]int64 `json:"level_distribution"`
	UniqueTerms       int64            `json:"unique_terms"`
	TermCount         int64            `json:"term_count"`
	TotalTerms        int64            `json:"total_terms"`
}

// readIndexMeta 尽力读取索引元数据；文件缺失或结构未知时返回 false。
func readIndexMeta(projectDir string) (*projectIndexMeta, bool) {
	for _, name := range []string{"meta.json", "metadata.json"} {
		data, err := os.ReadFile(filepath.Join(projectDir, "indexes", name))
		if err != nil {
			continue
		}
		var meta projectIndexMeta
		if err := json.Unmarshal(data, &meta); err != nil {
			continue
		}
		return &meta, true
	}
	return nil, false
}

// directorySize 统计目录下普通文件大小；目录缺失或读取失败时返回已统计值。
func directorySize(root string) int64 {
	var total int64
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return nil
		}
		if info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	return total
}
