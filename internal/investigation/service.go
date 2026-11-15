// Package investigation 实现调查工单、证据标记、时间线与证据包导出。
//
// 本包通过 Store/Reader 接口与持久化和日志检索解耦，不直接依赖
// internal/storage、internal/indexer 或 internal/query。
package investigation

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"glog/internal/errs"
	"glog/internal/logx"
	"glog/pkg/models"
)

// maxDocIDUint64 是 Reader.ReadDoc 的 uint32 参数可表示的最大值。
const maxDocIDUint64 uint64 = 1<<32 - 1

// timelineLimit、timelineSearchLimit 限制证据包/时间线的规模，避免误导出巨量数据。
const (
	timelineLimit       = 500
	timelineSearchLimit = 200
	relatedLogsDefault  = 100
	relatedLogsMax      = 500
)

// Store 是调查工单与证据标记的持久化接口。
type Store interface {
	CreateInvestigation(inv *models.Investigation) error
	GetInvestigation(id string) (*models.Investigation, error)
	UpdateInvestigation(inv *models.Investigation) error
	DeleteInvestigation(id string) error
	ListInvestigations(projectID string) ([]models.Investigation, error)
	AddEvidence(m *models.EvidenceMark) error
	RemoveEvidence(invID string, docID uint64) error
	ListEvidence(invID string) ([]models.EvidenceMark, error)
}

// Reader 是日志检索接口，通常由 query 服务实现。
type Reader interface {
	Search(q models.Query) (*models.QueryResult, error)
	ReadDoc(docID uint32) (*models.LogEntry, error)
}

// Service 调查工单服务。
type Service struct {
	store  Store
	reader Reader
}

// NewService 创建调查工单服务。store 必须非空；reader 在需要读取日志时使用。
func NewService(store Store, reader Reader) *Service {
	return &Service{store: store, reader: reader}
}

// Create 创建调查工单。Title 与 ProjectID 必填，ClueType 为空时默认为 message。
// 若请求携带 DocIDs，则在工单创建后尽力逐个标记为证据；单条读取/标记失败只记录警告，
// 不影响工单创建结果。
func (s *Service) Create(req models.CreateInvestigationRequest) (*models.Investigation, error) {
	if s.store == nil {
		return nil, errs.Internal("调查工单存储未配置", nil)
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		return nil, errs.InvalidInput("工单标题不能为空", errs.ErrInvalidInput)
	}
	projectID := strings.TrimSpace(req.ProjectID)
	if projectID == "" {
		return nil, errs.InvalidInput("项目ID不能为空", errs.ErrInvalidInput)
	}
	clueType, err := normalizeClueType(req.ClueType)
	if err != nil {
		return nil, err
	}
	if err := validateRiskScore(req.RiskScore); err != nil {
		return nil, err
	}
	if err := validateInvestigationTimeRange(req.TimeRange); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	inv := &models.Investigation{
		ID:              uuid.NewString(),
		ProjectID:       projectID,
		Title:           title,
		Description:     req.Description,
		ClueType:        clueType,
		ClueValue:       strings.TrimSpace(req.ClueValue),
		Status:          "open",
		RelatedLogCount: 0,
		TimeRange:       cloneTimeRange(req.TimeRange),
		EvidenceIDs:     []string{},
		Tags:            copyStrings(req.Tags),
		RiskScore:       cloneFloat32(req.RiskScore),
		CreatedAt:       now,
		UpdatedAt:       now,
		AssignedTo:      strings.TrimSpace(req.AssignedTo),
	}

	if err := s.store.CreateInvestigation(inv); err != nil {
		return nil, fmt.Errorf("创建调查工单失败: %w", err)
	}

	if len(req.DocIDs) > 0 {
		seen := make(map[uint64]struct{}, len(req.DocIDs))
		for _, docID := range req.DocIDs {
			if _, ok := seen[docID]; ok {
				continue
			}
			seen[docID] = struct{}{}
			if err := s.MarkEvidence(inv.ID, docID, ""); err != nil {
				logx.Warn(fmt.Sprintf("创建工单 %s 时标记证据 %d 失败: %v", inv.ID, docID, err))
			}
		}
	}

	// 重新读取以返回包含证据数量/ID 的最新工单；读取失败时仍返回已创建的工单。
	if latest, err := s.store.GetInvestigation(inv.ID); err == nil && latest != nil {
		inv = latest
	}
	return inv, nil
}

// Update 按非空指针字段更新调查工单。状态变更时同步 ClosedAt。
func (s *Service) Update(id string, u models.InvestigationUpdates) (*models.Investigation, error) {
	inv, err := s.getInvestigation(id)
	if err != nil {
		return nil, err
	}

	changed := false
	if u.Title != nil {
		title := strings.TrimSpace(*u.Title)
		if title == "" {
			return nil, errs.InvalidInput("工单标题不能为空", errs.ErrInvalidInput)
		}
		if inv.Title != title {
			inv.Title = title
			changed = true
		}
	}
	if u.Description != nil {
		if inv.Description != *u.Description {
			inv.Description = *u.Description
			changed = true
		}
	}
	if u.Status != nil {
		status, err := normalizeStatus(*u.Status)
		if err != nil {
			return nil, err
		}
		if status != inv.Status {
			inv.Status = status
			changed = true
		}
		now := time.Now().UTC()
		if status == "closed" {
			if inv.ClosedAt == nil || inv.ClosedAt.IsZero() {
				inv.ClosedAt = &now
				changed = true
			}
		} else if inv.ClosedAt != nil {
			inv.ClosedAt = nil
			changed = true
		}
	}
	if u.Tags != nil {
		inv.Tags = copyStrings(*u.Tags)
		changed = true
	}
	if u.ClueType != nil {
		clueType, err := normalizeClueType(*u.ClueType)
		if err != nil {
			return nil, err
		}
		if inv.ClueType != clueType {
			inv.ClueType = clueType
			changed = true
		}
	}
	if u.ClueValue != nil {
		clueValue := strings.TrimSpace(*u.ClueValue)
		if inv.ClueValue != clueValue {
			inv.ClueValue = clueValue
			changed = true
		}
	}
	if u.ClearTimeRange != nil && *u.ClearTimeRange {
		if inv.TimeRange != nil {
			inv.TimeRange = nil
			changed = true
		}
	} else if u.TimeRange != nil {
		timeRange := cloneTimeRange(u.TimeRange)
		if err := validateInvestigationTimeRange(timeRange); err != nil {
			return nil, err
		}
		if !sameTimeRange(inv.TimeRange, timeRange) {
			inv.TimeRange = timeRange
			changed = true
		}
	}
	if u.ClearRiskScore != nil && *u.ClearRiskScore {
		if inv.RiskScore != nil {
			inv.RiskScore = nil
			changed = true
		}
	} else if u.RiskScore != nil {
		if err := validateRiskScore(u.RiskScore); err != nil {
			return nil, err
		}
		inv.RiskScore = cloneFloat32(u.RiskScore)
		changed = true
	}
	if u.AssignedTo != nil {
		assigned := strings.TrimSpace(*u.AssignedTo)
		if inv.AssignedTo != assigned {
			inv.AssignedTo = assigned
			changed = true
		}
	}

	if !changed {
		return inv, nil
	}

	inv.UpdatedAt = time.Now().UTC()
	if err := s.store.UpdateInvestigation(inv); err != nil {
		return nil, fmt.Errorf("更新调查工单失败: %w", err)
	}
	return inv, nil
}

// Get 获取调查工单详情。
func (s *Service) Get(id string) (*models.Investigation, error) {
	return s.getInvestigation(id)
}

// List 列出项目下的调查工单。
func (s *Service) List(projectID string) ([]models.Investigation, error) {
	if s.store == nil {
		return nil, errs.Internal("调查工单存储未配置", nil)
	}
	items, err := s.store.ListInvestigations(strings.TrimSpace(projectID))
	if err != nil {
		return nil, fmt.Errorf("获取调查工单列表失败: %w", err)
	}
	if items == nil {
		items = []models.Investigation{}
	}
	return items, nil
}

// Close 结案：状态置为 closed 并记录结案时间。
func (s *Service) Close(id string) error {
	inv, err := s.getInvestigation(id)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	inv.Status = "closed"
	if inv.ClosedAt == nil || inv.ClosedAt.IsZero() {
		inv.ClosedAt = &now
	}
	inv.UpdatedAt = now
	if err := s.store.UpdateInvestigation(inv); err != nil {
		return fmt.Errorf("结案失败: %w", err)
	}
	return nil
}

// Delete 删除调查工单。关联证据标记由存储层随工单级联删除。
func (s *Service) Delete(id string) error {
	if s.store == nil {
		return errs.Internal("调查工单存储未配置", nil)
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return errs.InvalidInput("调查工单ID不能为空", errs.ErrInvalidInput)
	}
	if err := s.store.DeleteInvestigation(id); err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return errs.NotFound("调查工单不存在", err)
		}
		return fmt.Errorf("删除调查工单失败: %w", err)
	}
	return nil
}

// MarkEvidence 将日志标记为工单证据。日志必须可读；同一日志重复标记是幂等的。
func (s *Service) MarkEvidence(invID string, docID uint64, note string) error {
	if s.store == nil {
		return errs.Internal("调查工单存储未配置", nil)
	}
	if s.reader == nil {
		return errs.Internal("日志读取器未配置", nil)
	}
	inv, err := s.getInvestigation(invID)
	if err != nil {
		return err
	}
	if docID > maxDocIDUint64 {
		return errs.NotFound("日志不存在或不可读", errs.ErrNotFound)
	}

	entry, readErr := s.reader.ReadDoc(uint32(docID))
	if readErr != nil || entry == nil {
		if readErr != nil {
			return fmt.Errorf("日志 %d 不存在或不可读: %w: %w", docID, errs.ErrNotFound, readErr)
		}
		return fmt.Errorf("日志 %d 不存在或不可读: %w", docID, errs.ErrNotFound)
	}

	now := time.Now().UTC()
	docIDStr := strconv.FormatUint(docID, 10)
	mark := &models.EvidenceMark{
		ID:              uuid.NewString(),
		InvestigationID: inv.ID,
		DocID:           docID,
		LogEntryID:      logEntryIDString(entry, docID),
		MarkedAt:        now,
		Note:            strings.TrimSpace(note),
	}
	if err := s.store.AddEvidence(mark); err != nil {
		return fmt.Errorf("保存证据标记失败: %w", err)
	}

	present := false
	for _, id := range inv.EvidenceIDs {
		if id == docIDStr {
			present = true
			break
		}
	}
	if present {
		return nil
	}

	inv.EvidenceIDs = append(inv.EvidenceIDs, docIDStr)
	inv.RelatedLogCount = int64(len(inv.EvidenceIDs))
	inv.UpdatedAt = now
	if err := s.store.UpdateInvestigation(inv); err != nil {
		return fmt.Errorf("更新工单证据列表失败: %w", err)
	}
	return nil
}

// UnmarkEvidence 取消日志的证据标记，并同步工单证据列表与关联日志数。
func (s *Service) UnmarkEvidence(invID string, docID uint64) error {
	if s.store == nil {
		return errs.Internal("调查工单存储未配置", nil)
	}
	inv, err := s.getInvestigation(invID)
	if err != nil {
		return err
	}
	if err := s.store.RemoveEvidence(inv.ID, docID); err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return errs.NotFound("证据标记不存在", err)
		}
		return fmt.Errorf("移除证据标记失败: %w", err)
	}

	docIDStr := strconv.FormatUint(docID, 10)
	ids := make([]string, 0, len(inv.EvidenceIDs))
	for _, id := range inv.EvidenceIDs {
		if id != docIDStr {
			ids = append(ids, id)
		}
	}
	inv.EvidenceIDs = ids
	inv.RelatedLogCount = int64(len(ids))
	inv.UpdatedAt = time.Now().UTC()
	if err := s.store.UpdateInvestigation(inv); err != nil {
		return fmt.Errorf("更新工单证据列表失败: %w", err)
	}
	return nil
}

// ListEvidence 列出证据标记并尽力填充日志内容；单条日志读取失败时 Entry 为 nil。
func (s *Service) ListEvidence(invID string) ([]models.EvidenceItem, error) {
	if s.store == nil {
		return nil, errs.Internal("调查工单存储未配置", nil)
	}
	id := strings.TrimSpace(invID)
	if id == "" {
		return nil, errs.InvalidInput("调查工单ID不能为空", errs.ErrInvalidInput)
	}
	marks, err := s.store.ListEvidence(id)
	if err != nil {
		return nil, fmt.Errorf("获取证据列表失败: %w", err)
	}
	items := make([]models.EvidenceItem, 0, len(marks))
	for i := range marks {
		mark := marks[i]
		item := models.EvidenceItem{Mark: mark}
		if s.reader != nil && mark.DocID <= maxDocIDUint64 {
			entry, err := s.reader.ReadDoc(uint32(mark.DocID))
			if err != nil {
				logx.Warn(fmt.Sprintf("读取证据日志 %d 失败: %v", mark.DocID, err))
			} else {
				item.Entry = entry
			}
		}
		items = append(items, item)
	}
	return items, nil
}

// Timeline 生成工单时间线。优先使用证据日志；没有可读证据时按线索值检索。
func (s *Service) Timeline(invID string) ([]models.TimelineEvent, error) {
	inv, err := s.getInvestigation(invID)
	if err != nil {
		return nil, err
	}

	entries, err := s.evidenceEntries(inv.ID)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		q, ok := clueQuery(inv, timelineSearchLimit, "asc")
		if !ok {
			return []models.TimelineEvent{}, nil
		}
		if s.reader == nil {
			return nil, errs.Internal("日志读取器未配置", nil)
		}
		res, err := s.reader.Search(q)
		if err != nil {
			return nil, fmt.Errorf("按线索检索时间线日志失败: %w", err)
		}
		if res != nil {
			entries = append(entries, res.Hits...)
		}
	}

	entries = dedupeEntries(entries)
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Timestamp.Before(entries[j].Timestamp)
	})
	if len(entries) > timelineLimit {
		entries = entries[:timelineLimit]
	}

	events := make([]models.TimelineEvent, 0, len(entries))
	for i := range entries {
		entry := &entries[i]
		source := strings.TrimSpace(entry.SourceHost)
		if source == "" {
			source = strings.TrimSpace(entry.Source)
		}
		severity := strings.TrimSpace(entry.Level)
		if severity == "" {
			severity = models.LevelUnknown
		}
		events = append(events, models.TimelineEvent{
			Timestamp:   entry.Timestamp,
			Event:       timelineEventText(entry),
			Source:      source,
			Severity:    severity,
			LogEntryIDs: []string{logEntryIDString(entry, entry.DocID)},
		})
	}
	return events, nil
}

// RelatedLogs 返回与工单相关的日志。优先使用证据日志，否则按线索值检索。
// limit 默认 100，最大 500。
func (s *Service) RelatedLogs(invID string, limit int) ([]models.LogEntry, error) {
	if limit <= 0 {
		limit = relatedLogsDefault
	}
	if limit > relatedLogsMax {
		limit = relatedLogsMax
	}

	inv, err := s.getInvestigation(invID)
	if err != nil {
		return nil, err
	}

	evidence, err := s.evidenceEntries(inv.ID)
	if err != nil {
		return nil, err
	}
	out := make([]models.LogEntry, 0, limit)
	seen := make(map[uint64]struct{}, len(evidence))
	for i := range evidence {
		entry := evidence[i]
		if _, ok := seen[entry.DocID]; ok {
			continue
		}
		seen[entry.DocID] = struct{}{}
		out = append(out, entry)
		if len(out) >= limit {
			return out, nil
		}
	}
	if len(out) >= limit {
		return out, nil
	}

	q, ok := clueQuery(inv, limit, "desc")
	if !ok {
		return out, nil
	}
	if s.reader == nil {
		if len(out) > 0 {
			return out, nil
		}
		return nil, errs.Internal("日志读取器未配置", nil)
	}
	res, err := s.reader.Search(q)
	if err != nil {
		return nil, fmt.Errorf("按线索检索相关日志失败: %w", err)
	}
	if res == nil {
		return out, nil
	}
	for i := range res.Hits {
		if len(out) >= limit {
			break
		}
		hit := res.Hits[i]
		if _, ok := seen[hit.DocID]; ok {
			continue
		}
		seen[hit.DocID] = struct{}{}
		out = append(out, hit)
	}
	return out, nil
}

// SetAIAnalysis 保存 AI 分析结果。GeneratedAt 为空时补当前时间；风险等级在工单尚无
// 风险评分时映射为 high=90/medium=60/low=30。本方法不改变工单状态（closed 保持不变）。
func (s *Service) SetAIAnalysis(invID string, res *models.AIAnalysisResult) error {
	if res == nil {
		return errs.InvalidInput("AI分析结果不能为空", errs.ErrInvalidInput)
	}
	inv, err := s.getInvestigation(invID)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	copied := *res
	if copied.GeneratedAt.IsZero() {
		copied.GeneratedAt = now
	}
	copied.Timeline = append([]models.TimelineEvent(nil), res.Timeline...)
	copied.AttackPhases = append([]models.AttackPhase(nil), res.AttackPhases...)
	copied.Findings = copyStrings(res.Findings)
	copied.Suggestions = copyStrings(res.Suggestions)
	inv.AIAnalysis = &copied

	if inv.RiskScore == nil {
		switch strings.ToLower(strings.TrimSpace(res.RiskLevel)) {
		case "high":
			inv.RiskScore = float32Ptr(90)
		case "medium":
			inv.RiskScore = float32Ptr(60)
		case "low":
			inv.RiskScore = float32Ptr(30)
		}
	}

	inv.UpdatedAt = now
	if err := s.store.UpdateInvestigation(inv); err != nil {
		return fmt.Errorf("保存AI分析结果失败: %w", err)
	}
	return nil
}

// getInvestigation 读取并校验工单存在。
func (s *Service) getInvestigation(id string) (*models.Investigation, error) {
	if s.store == nil {
		return nil, errs.Internal("调查工单存储未配置", nil)
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errs.InvalidInput("调查工单ID不能为空", errs.ErrInvalidInput)
	}
	inv, err := s.store.GetInvestigation(id)
	if err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return nil, errs.NotFound("调查工单不存在", err)
		}
		return nil, fmt.Errorf("获取调查工单失败: %w", err)
	}
	if inv == nil {
		return nil, errs.NotFound("调查工单不存在", errs.ErrNotFound)
	}
	return inv, nil
}

// evidenceEntries 读取工单证据对应的日志，单条失败只记录警告。
func (s *Service) evidenceEntries(invID string) ([]models.LogEntry, error) {
	if s.store == nil {
		return nil, errs.Internal("调查工单存储未配置", nil)
	}
	marks, err := s.store.ListEvidence(invID)
	if err != nil {
		return nil, fmt.Errorf("获取工单证据失败: %w", err)
	}
	if s.reader == nil {
		return []models.LogEntry{}, nil
	}
	out := make([]models.LogEntry, 0, len(marks))
	seen := make(map[uint64]struct{}, len(marks))
	for _, mark := range marks {
		if mark.DocID > maxDocIDUint64 {
			continue
		}
		if _, ok := seen[mark.DocID]; ok {
			continue
		}
		entry, err := s.reader.ReadDoc(uint32(mark.DocID))
		if err != nil {
			logx.Warn(fmt.Sprintf("读取证据日志 %d 失败: %v", mark.DocID, err))
			continue
		}
		if entry == nil {
			continue
		}
		seen[mark.DocID] = struct{}{}
		out = append(out, *entry)
	}
	return out, nil
}

func normalizeStatus(raw string) (string, error) {
	status := strings.ToLower(strings.TrimSpace(raw))
	switch status {
	case "open", "analyzing", "closed":
		return status, nil
	default:
		return "", errs.InvalidInput("工单状态必须是 open、analyzing 或 closed", errs.ErrInvalidInput)
	}
}

// normalizeClueType 校验并规范化线索类型。空值默认为 message；未知类型返回输入错误。
func normalizeClueType(raw string) (string, error) {
	clueType := strings.ToLower(strings.TrimSpace(raw))
	if clueType == "" {
		return "message", nil
	}
	switch clueType {
	case "ip", "user", "pid", "file", "domain", "event_id", "message", "log_entry":
		return clueType, nil
	default:
		return "", errs.InvalidInput("线索类型必须是 ip、user、pid、file、domain、event_id、message 或 log_entry", errs.ErrInvalidInput)
	}
}

// validateInvestigationTimeRange 校验时间范围：允许只设置一端，但不能两端都是零值；
// 两端都设置时要求开始时间不晚于结束时间。
func validateInvestigationTimeRange(tr *models.TimeRange) error {
	if tr == nil {
		return nil
	}
	startZero := tr.Start.IsZero()
	endZero := tr.End.IsZero()
	if startZero && endZero {
		return errs.InvalidInput("时间范围不能同时为空", errs.ErrInvalidInput)
	}
	if !startZero && !endZero && tr.Start.After(tr.End) {
		return errs.InvalidInput("时间范围开始时间不能晚于结束时间", errs.ErrInvalidInput)
	}
	return nil
}

// sameTimeRange 判断两个时间范围是否等价（nil 与零值字段均按未设置处理）。
func sameTimeRange(a, b *models.TimeRange) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Start.Equal(b.Start) && a.End.Equal(b.End)
}

// clueQuery 根据工单线索构造日志检索条件。
// 线索值为空或线索类型为 log_entry（仅表示证据）时返回 false；
// event_id 线索优先按结构化事件ID检索，无法解析时回退为文本检索。
func clueQuery(inv *models.Investigation, limit int, sortOrder string) (models.Query, bool) {
	if inv == nil {
		return models.Query{}, false
	}
	clue := strings.TrimSpace(inv.ClueValue)
	if clue == "" {
		return models.Query{}, false
	}
	clueType := strings.ToLower(strings.TrimSpace(inv.ClueType))
	if clueType == "log_entry" {
		return models.Query{}, false
	}

	q := models.Query{
		Text:      clue,
		Limit:     limit,
		SortBy:    "timestamp",
		SortOrder: sortOrder,
	}
	if clueType == "event_id" {
		if eventID, err := strconv.ParseUint(clue, 10, 32); err == nil {
			q.EventIDs = []uint32{uint32(eventID)}
			q.Text = ""
		}
	}
	if inv.TimeRange != nil {
		if !inv.TimeRange.Start.IsZero() {
			start := inv.TimeRange.Start
			q.StartTime = &start
		}
		if !inv.TimeRange.End.IsZero() {
			end := inv.TimeRange.End
			q.EndTime = &end
		}
	}
	return q, true
}

func validateRiskScore(score *float32) error {
	if score == nil {
		return nil
	}
	if *score < 0 || *score > 100 {
		return errs.InvalidInput("风险评分必须在 0 到 100 之间", errs.ErrInvalidInput)
	}
	return nil
}

func timelineEventText(entry *models.LogEntry) string {
	level := strings.TrimSpace(entry.Level)
	if level == "" {
		level = models.LevelUnknown
	}
	message := strings.Join(strings.Fields(entry.Message), " ")
	message = truncateRunes(message, 200)
	if message == "" {
		return "[" + level + "]"
	}
	return "[" + level + "] " + message
}

func logEntryIDString(entry *models.LogEntry, fallbackDocID uint64) string {
	if entry != nil {
		if id := strings.TrimSpace(entry.ID); id != "" {
			return id
		}
		if entry.DocID != 0 {
			return strconv.FormatUint(entry.DocID, 10)
		}
	}
	return strconv.FormatUint(fallbackDocID, 10)
}

func dedupeEntries(entries []models.LogEntry) []models.LogEntry {
	out := make([]models.LogEntry, 0, len(entries))
	seen := make(map[uint64]struct{}, len(entries))
	for i := range entries {
		if _, ok := seen[entries[i].DocID]; ok {
			continue
		}
		seen[entries[i].DocID] = struct{}{}
		out = append(out, entries[i])
	}
	return out
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}

func cloneTimeRange(tr *models.TimeRange) *models.TimeRange {
	if tr == nil {
		return nil
	}
	copied := *tr
	return &copied
}

func cloneFloat32(v *float32) *float32 {
	if v == nil {
		return nil
	}
	copied := *v
	return &copied
}

func float32Ptr(v float32) *float32 {
	return &v
}

func copyStrings(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string(nil), values...)
}
