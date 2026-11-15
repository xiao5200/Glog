package collector

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/0xrawsec/golang-evtx/evtx"

	"glog/pkg/models"
)

const (
	maxEVTXMessageBytes = 64 << 10
	evtxHeaderMinSize   = 4096
)

// EVTXCollector 解析 Windows EVTX 事件日志。纯 Go 解析，非 Windows 也可用。
type EVTXCollector struct {
	paths []string
	opts  Options

	mu       sync.RWMutex
	start    *time.Time
	end      *time.Time
	eventIDs map[uint32]struct{}
	levels   map[string]struct{}
}

// NewEVTXCollector 创建 EVTX 采集器。
func NewEVTXCollector(paths []string, opts Options) *EVTXCollector {
	return &EVTXCollector{
		paths: append([]string(nil), paths...),
		opts:  opts,
	}
}

// SetFilter 设置可选的 EVTX 事件过滤条件。start/end 为闭区间；
// levels 接受 Information/Warning/Error/Critical/Verbose（大小写不敏感）。
func (c *EVTXCollector) SetFilter(start, end *time.Time, eventIDs []uint32, levels []string) {
	if c == nil {
		return
	}

	var startCopy, endCopy *time.Time
	if start != nil {
		v := *start
		startCopy = &v
	}
	if end != nil {
		v := *end
		endCopy = &v
	}

	idSet := make(map[uint32]struct{}, len(eventIDs))
	for _, id := range eventIDs {
		idSet[id] = struct{}{}
	}
	levelSet := make(map[string]struct{}, len(levels))
	for _, level := range levels {
		if strings.TrimSpace(level) == "" {
			continue
		}
		levelSet[normalizeEVTXLevel(level)] = struct{}{}
	}

	c.mu.Lock()
	c.start = startCopy
	c.end = endCopy
	c.eventIDs = idSet
	c.levels = levelSet
	c.mu.Unlock()
}

// Collect 逐个解析 EVTX 文件并输出标准化 JSON 行。
func (c *EVTXCollector) Collect(ctx context.Context, emit func(models.RawLine) error) (*Result, error) {
	if c == nil {
		return nil, fmt.Errorf("EVTX 采集器不能为空")
	}
	ctx = ensureContext(ctx)
	em, err := newEmitter(ctx, emit)
	if err != nil {
		return nil, err
	}
	defer em.Close()

	acc := newResultAccumulator()
	files, expandErrs := expandPaths(ctx, c.paths, true, nil)
	for _, expandErr := range expandErrs {
		acc.addError(expandErr)
	}
	if err := stoppedError(ctx); err != nil {
		return acc.snapshot(), err
	}

	evtxFiles := make([]string, 0, len(files))
	for _, filePath := range files {
		if IsEVTX(filePath) {
			evtxFiles = append(evtxFiles, filePath)
		}
	}
	if len(evtxFiles) == 0 {
		return acc.snapshot(), fmt.Errorf("未找到可解析的 EVTX 文件（共 %d 个路径模式）", len(c.paths))
	}

	concurrency := c.opts.concurrency(defaultConcurrency)
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for _, filePath := range evtxFiles {
		select {
		case <-em.ctx.Done():
			wg.Wait()
			return finalize(ctx, em, acc)
		case sem <- struct{}{}:
		}

		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			defer func() { <-sem }()
			c.collectFile(em.ctx, p, acc, em)
		}(filePath)
	}
	wg.Wait()

	return finalize(ctx, em, acc)
}

func (c *EVTXCollector) collectFile(ctx context.Context, filePath string, acc *resultAccumulator, em *emitter) {
	info, err := os.Stat(filePath)
	if err != nil {
		acc.addErrorf("检查 EVTX 文件失败 %s: %v", filePath, err)
		return
	}
	if !info.Mode().IsRegular() {
		acc.addErrorf("不是常规 EVTX 文件: %s", filePath)
		return
	}

	ef, err := openEVTX(filePath)
	if err != nil {
		acc.addErrorf("打开 EVTX 文件失败 %s: %v", filePath, err)
		return
	}

	// 注意：golang-evtx 的 FastEvents 在内部 goroutine 中解析，遇到损坏数据
	// 可能 panic，调用方无法 recover。因此只在正常读完（channel 关闭）后关闭
	// 文件；ctx 取消时直接返回，避免关闭正在被库 goroutine 使用的文件句柄。
	events := ef.FastEvents()
	finished := false
	defer func() {
		if finished {
			_ = ef.Close()
		}
	}()

	acc.addFile(filePath)
	fallbackTime := info.ModTime()
	if fallbackTime.IsZero() {
		fallbackTime = time.Now()
	}
	handle := func(ev *evtx.GoEvtxMap) {
		if ev == nil {
			return
		}
		text, eventTime, eventID, level, convErr := c.normalizeEvent(ev, fallbackTime)
		if convErr != nil {
			acc.addErrorf("转换 EVTX 事件失败 %s: %v", filePath, convErr)
			return
		}
		if !c.matchFilter(eventTime, eventID, level) {
			return
		}
		raw := models.RawLine{
			SourceID:   c.opts.SourceID,
			SourceHost: c.opts.SourceHost,
			FilePath:   filePath,
			Offset:     eventID,
			Size:       int64(len(text)),
			Text:       text,
			Structured: true,
		}
		if err := em.Emit(raw); err != nil {
			return
		}
		acc.addLine()
		acc.addBytes(int64(len(text)))
	}

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-events:
			if !ok {
				finished = true
				return
			}
			handle(ev)
			if em.Err() != nil {
				return
			}
		}
	}
}

type evtxRecord struct {
	Timestamp string                 `json:"timestamp"`
	Level     string                 `json:"level"`
	EventID   int64                  `json:"event_id"`
	Provider  string                 `json:"provider"`
	Channel   string                 `json:"channel,omitempty"`
	Computer  string                 `json:"computer"`
	UserID    string                 `json:"user_id"`
	Message   string                 `json:"message"`
	Fields    map[string]interface{} `json:"fields"`
}

func (c *EVTXCollector) normalizeEvent(ev *evtx.GoEvtxMap, fallback time.Time) (string, time.Time, int64, string, error) {
	eventTime, timeOK := evtxTime(ev,
		"/Event/System/TimeCreated/SystemTime",
		"/Event/System/TimeCreated/#attributes/SystemTime",
	)
	if !timeOK || eventTime.IsZero() {
		eventTime = fallback
	}

	eventID, _ := evtxInt(ev,
		"/Event/System/EventID",
		"/Event/System/EventID/Value",
	)
	provider := evtxString(ev,
		"/Event/System/Provider/Name",
		"/Event/System/Provider/#attributes/Name",
	)
	// Channel（Security/System/Application 等）供 event_type 优先使用，
	// 比 Provider 全名更适合作事件类型。
	channel := evtxString(ev, "/Event/System/Channel")
	computer := evtxString(ev, "/Event/System/Computer")
	userID := evtxString(ev,
		"/Event/System/Security/UserID",
		"/Event/System/Security/#attributes/UserID",
	)
	rawLevel := evtxString(ev,
		"/Event/System/Level",
		"/Event/System/Level/Value",
	)
	level := normalizeEVTXLevel(rawLevel)

	fields := evtxEventData(ev)
	message := strings.TrimSpace(evtxString(ev, "/Event/RenderingInfo/Message"))
	if message == "" {
		message = formatEVTXFields(fields)
	}
	if message == "" {
		message = fmt.Sprintf("EventID=%d Provider=%s", eventID, provider)
	}
	message = truncateString(message, maxEVTXMessageBytes)

	record := evtxRecord{
		Timestamp: eventTime.UTC().Format(time.RFC3339Nano),
		Level:     level,
		EventID:   eventID,
		Provider:  provider,
		Channel:   channel,
		Computer:  computer,
		UserID:    userID,
		Message:   message,
		Fields:    fields,
	}

	data, err := json.Marshal(record)
	if err != nil {
		// 极端情况下字段里可能存在无法 JSON 序列化的值，降级为字符串。
		record.Fields = stringifyEVTXFields(record.Fields)
		data, err = json.Marshal(record)
	}
	if err != nil {
		return "", time.Time{}, 0, "", fmt.Errorf("序列化事件失败: %w", err)
	}
	return string(data), eventTime, eventID, level, nil
}

func (c *EVTXCollector) matchFilter(eventTime time.Time, eventID int64, level string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.start != nil && eventTime.Before(*c.start) {
		return false
	}
	if c.end != nil && eventTime.After(*c.end) {
		return false
	}
	if len(c.eventIDs) > 0 {
		if eventID < 0 || eventID > int64(^uint32(0)) {
			return false
		}
		if _, ok := c.eventIDs[uint32(eventID)]; !ok {
			return false
		}
	}
	if len(c.levels) > 0 {
		if _, ok := c.levels[level]; !ok {
			return false
		}
	}
	return true
}

func openEVTX(filePath string) (*evtx.File, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, err
	}
	if info.Size() < evtxHeaderMinSize {
		return nil, fmt.Errorf("文件过小，不是有效的 EVTX 文件（%d 字节）", info.Size())
	}

	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	var magic [8]byte
	_, readErr := io.ReadFull(f, magic[:])
	_ = f.Close()
	if readErr != nil {
		return nil, fmt.Errorf("读取文件头失败: %w", readErr)
	}
	if string(magic[:]) != evtxMagic {
		return nil, fmt.Errorf("EVTX 魔数不正确")
	}

	var file evtx.File
	var openErr error
	func() {
		defer func() {
			if r := recover(); r != nil {
				openErr = fmt.Errorf("EVTX 文件头解析异常: %v", r)
			}
		}()

		file, openErr = evtx.Open(filePath)
		if errors.Is(openErr, evtx.ErrDirtyFile) {
			_ = file.Close()
			file, openErr = evtx.OpenDirty(filePath)
		}
	}()
	if openErr != nil {
		// golang-evtx 在文件头校验失败时不会关闭已打开的文件句柄。
		_ = file.Close()
		return nil, openErr
	}

	if err := validateEVTXLayout(filePath, &file, info.Size()); err != nil {
		_ = file.Close()
		return nil, err
	}
	return &file, nil
}

// validateEVTXLayout 预检查 chunk 数量、边界与魔数，尽量降低底层库在
// FastEvents 中因损坏文件 panic 的概率。
func validateEVTXLayout(filePath string, file *evtx.File, fileSize int64) error {
	base := int64(file.Header.ChunkDataOffset)
	chunkCount := int(file.Header.ChunkCount)
	if chunkCount == 0 {
		return nil
	}

	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	var magic [8]byte
	for i := 0; i < chunkCount; i++ {
		offset := base + int64(i)*evtx.ChunkSize
		if offset < 0 || offset+evtx.ChunkSize > fileSize {
			return fmt.Errorf("第 %d 个 chunk 越界（文件可能不完整）", i)
		}
		if _, err := f.ReadAt(magic[:], offset); err != nil {
			return fmt.Errorf("读取第 %d 个 chunk 头失败: %w", i, err)
		}
		if string(magic[:]) != "ElfChnk\x00" {
			return fmt.Errorf("第 %d 个 chunk 魔数不正确", i)
		}
	}
	return nil
}

func evtxString(ev *evtx.GoEvtxMap, paths ...string) string {
	if ev == nil {
		return ""
	}
	for _, p := range paths {
		gp := evtx.Path(p)
		if value, err := ev.GetString(&gp); err == nil {
			return strings.TrimSpace(value)
		}
		if value, err := ev.Get(&gp); err == nil && value != nil {
			switch v := (*value).(type) {
			case string:
				return strings.TrimSpace(v)
			case evtx.GoEvtxMap:
				if s, ok := v["Value"].(string); ok {
					return strings.TrimSpace(s)
				}
			default:
				if v != nil {
					return strings.TrimSpace(fmt.Sprint(v))
				}
			}
		}
	}
	return ""
}

func evtxInt(ev *evtx.GoEvtxMap, paths ...string) (int64, bool) {
	if ev == nil {
		return 0, false
	}
	for _, p := range paths {
		gp := evtx.Path(p)
		if value, err := ev.GetInt(&gp); err == nil {
			return value, true
		}
		if s := evtxString(ev, p); s != "" {
			if value, err := strconv.ParseInt(s, 0, 64); err == nil {
				return value, true
			}
		}
	}
	return 0, false
}

func evtxTime(ev *evtx.GoEvtxMap, paths ...string) (time.Time, bool) {
	if ev == nil {
		return time.Time{}, false
	}
	for _, p := range paths {
		gp := evtx.Path(p)
		if value, err := ev.GetTime(&gp); err == nil && !value.IsZero() {
			return value, true
		}
		if s := evtxString(ev, p); s != "" {
			if value, err := time.Parse(time.RFC3339Nano, s); err == nil {
				return value, true
			}
		}
	}
	return time.Time{}, false
}

func evtxEventData(ev *evtx.GoEvtxMap) map[string]interface{} {
	result := make(map[string]interface{})
	if ev == nil {
		return result
	}
	for _, p := range []string{"/Event/EventData", "/Event/UserData"} {
		gp := evtx.Path(p)
		mp, err := ev.GetMap(&gp)
		if err != nil || mp == nil {
			continue
		}
		for key, value := range *mp {
			if _, exists := result[key]; !exists {
				result[key] = normalizeEVTXValue(value)
			}
		}
	}
	return result
}

func normalizeEVTXValue(value interface{}) interface{} {
	switch v := value.(type) {
	case nil:
		return nil
	case evtx.GoEvtxMap:
		out := make(map[string]interface{}, len(v))
		for key, item := range v {
			out[key] = normalizeEVTXValue(item)
		}
		return out
	case map[string]interface{}:
		out := make(map[string]interface{}, len(v))
		for key, item := range v {
			out[key] = normalizeEVTXValue(item)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(v))
		for i, item := range v {
			out[i] = normalizeEVTXValue(item)
		}
		return out
	case time.Time:
		return v.UTC().Format(time.RFC3339Nano)
	case []byte:
		return base64.StdEncoding.EncodeToString(v)
	default:
		return v
	}
}

func stringifyEVTXFields(fields map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(fields))
	for key, value := range fields {
		out[key] = fmt.Sprint(value)
	}
	return out
}

func formatEVTXFields(fields map[string]interface{}) string {
	if len(fields) == 0 {
		return ""
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+evtxValueString(fields[key]))
	}
	return strings.Join(parts, " ")
}

func evtxValueString(value interface{}) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprint(v)
		}
		return string(data)
	}
}

// normalizeEVTXLevel 把 Windows 事件级别（0-5）或文本级别归一化为设计约定值。
func normalizeEVTXLevel(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "critical", "crit", "1":
		return "Critical"
	case "error", "err", "2":
		return "Error"
	case "warning", "warn", "3":
		return "Warning"
	case "information", "info", "4", "0", "logalways", "":
		return "Information"
	case "verbose", "5":
		return "Verbose"
	}

	if n, err := strconv.Atoi(value); err == nil {
		switch n {
		case 1:
			return "Critical"
		case 2:
			return "Error"
		case 3:
			return "Warning"
		case 5:
			return "Verbose"
		default:
			return "Information"
		}
	}
	return "Information"
}
