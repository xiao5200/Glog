package threat

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"glog/internal/errs"
	"glog/internal/logx"
	"glog/pkg/models"
)

var (
	emailValuePattern  = regexp.MustCompile(`^[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}$`)
	domainValuePattern = regexp.MustCompile(`(?i)^(?:[a-z0-9](?:[a-z0-9\-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)
)

const (
	iocTypeIP       = "ip"
	iocTypeDomain   = "domain"
	iocTypeMD5      = "md5"
	iocTypeSHA1     = "sha1"
	iocTypeSHA256   = "sha256"
	iocTypeURL      = "url"
	iocTypeEmail    = "email"
	iocTypeFilePath = "file_path"
	iocTypeUnknown  = "unknown"
)

type rawIOCEntry struct {
	Type        string
	Value       string
	ThreatLevel string
	Description string
	Source      string
	Location    string
}

// ImportIOC 从 JSON/CSV/TXT 文件导入 IOC。单个条目的格式或保存错误会逐条收集，
// 最多保留 100 条；只有文件不可读或整体格式无法识别时才返回错误。
func (s *Service) ImportIOC(filePath string) (*models.IOCImportResult, error) {
	if s.store == nil {
		return nil, errs.Internal("IOC存储未配置", nil)
	}
	path := strings.TrimSpace(filePath)
	if path == "" {
		return nil, errs.InvalidInput("IOC文件路径不能为空", errs.ErrInvalidInput)
	}

	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("读取IOC文件失败: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, errs.InvalidInput("IOC文件不能是符号链接", errs.ErrInvalidInput)
	}
	if !info.Mode().IsRegular() {
		return nil, errs.InvalidInput("IOC路径不是普通文件", errs.ErrInvalidInput)
	}
	if info.Size() > maxIOCFileBytes {
		return nil, errs.InvalidInput(fmt.Sprintf("IOC文件超过大小限制（%d 字节）", maxIOCFileBytes), errs.ErrInvalidInput)
	}

	format := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
	switch format {
	case "json", "csv", "txt":
	default:
		return nil, errs.InvalidInput("不支持的IOC文件格式，仅支持 .json/.csv/.txt", errs.ErrInvalidInput)
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("打开IOC文件失败: %w", err)
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxIOCFileBytes+1))
	if err != nil {
		return nil, fmt.Errorf("读取IOC文件内容失败: %w", err)
	}
	if int64(len(data)) > maxIOCFileBytes {
		return nil, errs.InvalidInput(fmt.Sprintf("IOC文件超过大小限制（%d 字节）", maxIOCFileBytes), errs.ErrInvalidInput)
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})

	collector := &iocErrorCollector{limit: maxIOCErrors, items: make([]string, 0, 8)}
	var entries []rawIOCEntry
	switch format {
	case "json":
		entries, err = parseIOCJSON(data, collector)
	case "csv":
		entries, err = parseIOCCSV(data, collector)
	case "txt":
		entries, err = parseIOCTxt(data, collector)
	}
	if err != nil {
		return nil, errs.InvalidInput("IOC文件格式无法识别", err)
	}

	result := &models.IOCImportResult{
		Errors: make([]string, 0, len(collector.items)),
		Items:  make([]models.IOCItem, 0, len(entries)),
		Format: format,
	}
	seen := make(map[string]struct{}, len(entries))
	now := time.Now().UTC()
	for _, entry := range entries {
		value := normalizeIOCValue(entry.Value)
		if value == "" {
			collector.add("%s: IOC值为空", entry.Location)
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			result.Skipped++
			continue
		}
		seen[key] = struct{}{}

		iocType, ok := normalizeIOCTypeName(entry.Type)
		if !ok {
			iocType = detectIOCType(value)
			if iocType == iocTypeUnknown {
				collector.add("%s: 无法识别IOC类型: %s", entry.Location, truncateText(value, 120))
				continue
			}
		}

		existing, findErr := s.store.FindIOCByValue(value)
		if findErr != nil {
			if !errors.Is(findErr, errs.ErrNotFound) {
				logx.Warn(fmt.Sprintf("检查重复IOC %q 失败，继续导入: %v", truncateText(value, 80), findErr))
			}
		} else if existing != nil {
			result.Skipped++
			continue
		}

		item := models.IOCItem{
			ID:          uuid.NewString(),
			Type:        iocType,
			Value:       value,
			ThreatLevel: normalizeThreatLevel(entry.ThreatLevel),
			Description: strings.TrimSpace(entry.Description),
			Source:      strings.TrimSpace(entry.Source),
			CreatedAt:   now,
		}
		if err := s.store.SaveIOC(&item); err != nil {
			collector.add("%s: 保存IOC失败: %v", entry.Location, err)
			continue
		}
		result.Imported++
		result.Items = append(result.Items, item)
	}

	result.Errors = collector.items
	return result, nil
}

// ListIOCs 列出全部 IOC。
func (s *Service) ListIOCs() ([]models.IOCItem, error) {
	if s.store == nil {
		return nil, errs.Internal("IOC存储未配置", nil)
	}
	items, err := s.store.ListIOCs()
	if err != nil {
		return nil, fmt.Errorf("获取IOC列表失败: %w", err)
	}
	if items == nil {
		items = []models.IOCItem{}
	}
	return items, nil
}

// DeleteIOC 删除指定 IOC。
func (s *Service) DeleteIOC(id string) error {
	if s.store == nil {
		return errs.Internal("IOC存储未配置", nil)
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return errs.InvalidInput("IOC ID不能为空", errs.ErrInvalidInput)
	}
	if err := s.store.DeleteIOC(id); err != nil {
		return fmt.Errorf("删除IOC失败: %w", err)
	}
	return nil
}

// SearchIOC 在项目中检索给定 IOC 值。values 为空时检索已保存的全部 IOC。
// baseQuery 的文本与时间条件会被合并，并保留其余过滤条件；每个 IOC 最多取 1000 条候选，
// 结果按 IOC+DocID 去重，最多返回 2000 条。
func (s *Service) SearchIOC(projectID string, values []string, baseQuery *models.Query) (*models.IOCSearchResult, error) {
	_ = projectID
	if s.reader == nil {
		return nil, errs.Internal("日志读取器未配置", nil)
	}
	started := time.Now()
	iocs, err := s.resolveIOCs(values)
	if err != nil {
		return nil, err
	}

	result := &models.IOCSearchResult{Matches: make([]models.IOCMatch, 0, 16)}
	seenMatches := make(map[string]struct{}, 64)

searchLoop:
	for _, ioc := range iocs {
		query := buildIOCQuery(baseQuery, ioc.Value)
		res, err := s.reader.Search(query)
		if err != nil {
			return nil, fmt.Errorf("检索IOC %q 失败: %w", truncateText(ioc.Value, 80), err)
		}
		if res == nil {
			continue
		}
		hits := res.Hits
		if len(hits) > iocCandidateLimit {
			hits = hits[:iocCandidateLimit]
		}
		result.Scanned += int64(len(hits))

		lowerValue := strings.ToLower(ioc.Value)
		for i := range hits {
			hit := &hits[i]
			docID := hit.DocID
			entry := hit
			if docID <= uint64(^uint32(0)) {
				if read, readErr := s.reader.ReadDoc(uint32(docID)); readErr == nil && read != nil {
					entry = read
					if read.DocID != 0 {
						docID = read.DocID
					}
				}
			}

			matchedField, matched := matchIOCEntryValue(entry, lowerValue)
			if !matched {
				continue
			}
			key := lowerValue + "|" + strconv.FormatUint(docID, 10)
			if _, ok := seenMatches[key]; ok {
				continue
			}
			seenMatches[key] = struct{}{}
			result.Matches = append(result.Matches, models.IOCMatch{
				IOC:          ioc,
				DocID:        docID,
				Entry:        entry,
				MatchedField: matchedField,
			})
			if len(result.Matches) >= iocResultLimit {
				break searchLoop
			}
		}
	}

	result.Total = int64(len(result.Matches))
	result.Took = time.Since(started).Milliseconds()
	return result, nil
}

func (s *Service) resolveIOCs(values []string) ([]models.IOCItem, error) {
	out := make([]models.IOCItem, 0, len(values))
	seen := make(map[string]struct{}, len(values))

	if len(values) == 0 {
		if s.store == nil {
			return nil, errs.Internal("IOC存储未配置", nil)
		}
		items, err := s.store.ListIOCs()
		if err != nil {
			return nil, fmt.Errorf("获取IOC列表失败: %w", err)
		}
		for _, item := range items {
			value := normalizeIOCValue(item.Value)
			if value == "" {
				continue
			}
			key := strings.ToLower(value)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, item)
		}
		return out, nil
	}

	for _, raw := range values {
		value := normalizeIOCValue(raw)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		if s.store != nil {
			if item, err := s.store.FindIOCByValue(value); err == nil && item != nil {
				copied := *item
				if copied.Value == "" {
					copied.Value = value
				}
				if copied.Type == "" {
					copied.Type = detectIOCType(value)
				}
				if copied.ThreatLevel == "" {
					copied.ThreatLevel = "medium"
				}
				out = append(out, copied)
				continue
			}
		}
		out = append(out, models.IOCItem{
			Type:        detectIOCType(value),
			Value:       value,
			ThreatLevel: "medium",
		})
	}
	return out, nil
}

func buildIOCQuery(base *models.Query, value string) models.Query {
	var query models.Query
	if base != nil {
		query = *base
	}
	if strings.TrimSpace(query.Text) == "" {
		query.Text = value
	} else {
		query.Text = strings.TrimSpace(query.Text) + " AND " + quoteQueryValue(value)
	}
	query.Offset = 0
	query.Limit = iocCandidateLimit
	return query
}

func quoteQueryValue(value string) string {
	if !strings.ContainsAny(value, " \t\r\n\"()[]{}:") {
		return value
	}
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}

func matchIOCEntryValue(entry *models.LogEntry, lowerValue string) (string, bool) {
	if entry == nil || lowerValue == "" {
		return "", false
	}
	if strings.Contains(strings.ToLower(entry.Message), lowerValue) {
		return "message", true
	}
	if strings.Contains(strings.ToLower(entry.RawLog), lowerValue) {
		return "raw", true
	}
	if path, ok := fieldsContainValue(entry.Fields, lowerValue); ok {
		if path == "" {
			return "fields", true
		}
		return "fields." + path, true
	}
	return "", false
}

func fieldsContainValue(fields map[string]interface{}, lowerValue string) (string, bool) {
	if len(fields) == 0 {
		return "", false
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if path, ok := valueContainsText(fields[key], lowerValue); ok {
			if path == "" {
				return key, true
			}
			return key + "." + path, true
		}
	}
	return "", false
}

func valueContainsText(value interface{}, lowerValue string) (string, bool) {
	switch v := value.(type) {
	case nil:
		return "", false
	case string:
		if strings.Contains(strings.ToLower(v), lowerValue) {
			return "", true
		}
	case []interface{}:
		for i, item := range v {
			if path, ok := valueContainsText(item, lowerValue); ok {
				index := strconv.Itoa(i)
				if path == "" {
					return index, true
				}
				return index + "." + path, true
			}
		}
	case []string:
		for i, item := range v {
			if strings.Contains(strings.ToLower(item), lowerValue) {
				return strconv.Itoa(i), true
			}
		}
	case map[string]interface{}:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if path, ok := valueContainsText(v[key], lowerValue); ok {
				if path == "" {
					return key, true
				}
				return key + "." + path, true
			}
		}
	case map[string]string:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if strings.Contains(strings.ToLower(v[key]), lowerValue) {
				return key, true
			}
		}
	case json.Number:
		if strings.Contains(strings.ToLower(v.String()), lowerValue) {
			return "", true
		}
	default:
		if strings.Contains(strings.ToLower(fmt.Sprint(v)), lowerValue) {
			return "", true
		}
	}
	return "", false
}

func detectIOCType(value string) string {
	value = normalizeIOCValue(value)
	if value == "" {
		return iocTypeUnknown
	}
	if isURLValue(value) {
		return iocTypeURL
	}
	if emailValuePattern.MatchString(value) {
		return iocTypeEmail
	}
	if ip := net.ParseIP(value); ip != nil {
		return iocTypeIP
	}
	switch {
	case len(value) == 64 && isHexString(value):
		return iocTypeSHA256
	case len(value) == 40 && isHexString(value):
		return iocTypeSHA1
	case len(value) == 32 && isHexString(value):
		return iocTypeMD5
	}
	if domainValuePattern.MatchString(value) {
		return iocTypeDomain
	}
	if strings.ContainsAny(value, `/\`) {
		return iocTypeFilePath
	}
	return iocTypeUnknown
}

func normalizeIOCTypeName(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "ip", "ipv4", "ipv6", "ip_address", "ipaddress", "ip-address":
		return iocTypeIP, true
	case "domain", "hostname", "fqdn":
		return iocTypeDomain, true
	case "md5":
		return iocTypeMD5, true
	case "sha1", "sha-1":
		return iocTypeSHA1, true
	case "sha256", "sha-256":
		return iocTypeSHA256, true
	case "url", "uri":
		return iocTypeURL, true
	case "email", "mail", "email_address", "email-address":
		return iocTypeEmail, true
	case "file_path", "filepath", "file-path", "path", "file":
		return iocTypeFilePath, true
	default:
		return "", false
	}
}

func normalizeThreatLevel(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "low", "info", "informational", "minor":
		return "low"
	case "medium", "med", "moderate", "":
		return "medium"
	case "high", "major":
		return "high"
	case "critical", "crit", "severe":
		return "critical"
	default:
		return "medium"
	}
}

func normalizeIOCValue(raw string) string {
	value := strings.TrimSpace(strings.TrimPrefix(raw, "\ufeff"))
	if len(value) >= 2 {
		first, last := value[0], value[len(value)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			value = strings.TrimSpace(value[1 : len(value)-1])
		}
	}
	return value
}

func isURLValue(value string) bool {
	if !strings.Contains(value, "://") {
		return false
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return false
	}
	return parsed.Scheme != "" && (parsed.Host != "" || parsed.Opaque != "")
}

func isHexString(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return false
		}
	}
	return true
}

func parseIOCJSON(data []byte, collector *iocErrorCollector) ([]rawIOCEntry, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, nil
	}

	var raws []json.RawMessage
	switch trimmed[0] {
	case '[':
		if err := json.Unmarshal(trimmed, &raws); err != nil {
			return nil, fmt.Errorf("JSON数组解析失败: %w", err)
		}
	case '{':
		var object map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &object); err != nil {
			return nil, fmt.Errorf("JSON对象解析失败: %w", err)
		}
		if iocsRaw, ok := object["iocs"]; ok {
			if err := json.Unmarshal(iocsRaw, &raws); err != nil {
				return nil, fmt.Errorf("iocs 字段必须是数组: %w", err)
			}
		} else if _, ok := object["value"]; ok {
			raws = []json.RawMessage{json.RawMessage(trimmed)}
		} else {
			return nil, fmt.Errorf("JSON中缺少 iocs 数组或 value 字段")
		}
	default:
		return nil, fmt.Errorf("JSON根节点必须是数组或对象")
	}

	out := make([]rawIOCEntry, 0, len(raws))
	for i, raw := range raws {
		location := fmt.Sprintf("第 %d 项", i+1)
		item := bytes.TrimSpace(raw)
		if len(item) == 0 || bytes.Equal(item, []byte("null")) {
			collector.add("%s: 空条目", location)
			continue
		}

		entry := rawIOCEntry{Location: location}
		if item[0] == '"' {
			if err := json.Unmarshal(item, &entry.Value); err != nil {
				collector.add("%s: IOC值不是合法字符串: %v", location, err)
				continue
			}
		} else if item[0] == '{' {
			var obj struct {
				Type        string          `json:"type"`
				Value       json.RawMessage `json:"value"`
				ThreatLevel string          `json:"threat_level"`
				Description string          `json:"description"`
				Source      string          `json:"source"`
			}
			if err := json.Unmarshal(item, &obj); err != nil {
				collector.add("%s: 解析IOC对象失败: %v", location, err)
				continue
			}
			entry.Type = obj.Type
			entry.ThreatLevel = obj.ThreatLevel
			entry.Description = obj.Description
			entry.Source = obj.Source
			value := bytes.TrimSpace(obj.Value)
			if len(value) == 0 || bytes.Equal(value, []byte("null")) {
				collector.add("%s: 缺少 value 字段", location)
				continue
			}
			if value[0] == '"' {
				if err := json.Unmarshal(value, &entry.Value); err != nil {
					collector.add("%s: value 不是合法字符串: %v", location, err)
					continue
				}
			} else {
				var scalar interface{}
				if err := json.Unmarshal(value, &scalar); err != nil {
					collector.add("%s: value 无法解析: %v", location, err)
					continue
				}
				entry.Value = fmt.Sprint(scalar)
			}
		} else {
			collector.add("%s: 条目必须是字符串或对象", location)
			continue
		}

		if strings.TrimSpace(entry.Value) == "" {
			collector.add("%s: IOC值为空", location)
			continue
		}
		out = append(out, entry)
	}
	return out, nil
}

func parseIOCCSV(data []byte, collector *iocErrorCollector) ([]rawIOCEntry, error) {
	reader := csv.NewReader(bytes.NewReader(data))
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true
	records, err := reader.ReadAll()
	if err != nil {
		if len(records) == 0 {
			return nil, fmt.Errorf("CSV解析失败: %w", err)
		}
		collector.add("CSV在第 %d 行附近解析失败: %v", len(records)+1, err)
	}
	if len(records) == 0 {
		return nil, nil
	}

	columnIndex := make(map[string]int, 4)
	hasHeader := false
	for i, cell := range records[0] {
		key := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(cell, "\ufeff")))
		switch key {
		case "value", "ioc", "indicator":
			if _, exists := columnIndex["value"]; !exists {
				columnIndex["value"] = i
				hasHeader = true
			}
		case "type", "ioc_type", "indicator_type":
			if _, exists := columnIndex["type"]; !exists {
				columnIndex["type"] = i
			}
		case "threat_level", "threatlevel", "level", "severity":
			if _, exists := columnIndex["threat_level"]; !exists {
				columnIndex["threat_level"] = i
			}
		case "description", "desc":
			if _, exists := columnIndex["description"]; !exists {
				columnIndex["description"] = i
			}
		case "source":
			if _, exists := columnIndex["source"]; !exists {
				columnIndex["source"] = i
			}
		}
	}
	if !hasHeader {
		columnIndex["value"] = 0
	}

	start := 0
	if hasHeader {
		start = 1
	}
	out := make([]rawIOCEntry, 0, len(records)-start)
	for i := start; i < len(records); i++ {
		row := records[i]
		if rowIsEmpty(row) {
			continue
		}
		entry := rawIOCEntry{Location: fmt.Sprintf("第 %d 行", i+1)}
		entry.Value = csvCell(row, columnIndex["value"])
		entry.Type = csvCell(row, columnIndex["type"])
		entry.ThreatLevel = csvCell(row, columnIndex["threat_level"])
		entry.Description = csvCell(row, columnIndex["description"])
		entry.Source = csvCell(row, columnIndex["source"])

		if !hasHeader && entry.Type == "" && len(row) > 1 {
			if _, ok := normalizeIOCTypeName(row[1]); ok {
				entry.Type = row[1]
			}
		}
		if strings.TrimSpace(entry.Value) == "" {
			collector.add("%s: IOC值为空", entry.Location)
			continue
		}
		out = append(out, entry)
	}
	return out, nil
}

func parseIOCTxt(data []byte, collector *iocErrorCollector) ([]rawIOCEntry, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	out := make([]rawIOCEntry, 0, 64)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "\ufeff"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		entry := rawIOCEntry{Location: fmt.Sprintf("第 %d 行", lineNumber), Value: line}
		if comma := strings.Index(line, ","); comma > 0 {
			left := strings.TrimSpace(line[:comma])
			right := strings.TrimSpace(line[comma+1:])
			if _, ok := normalizeIOCTypeName(right); ok {
				entry.Value = left
				entry.Type = right
			}
		}
		if strings.TrimSpace(entry.Value) == "" {
			collector.add("%s: IOC值为空", entry.Location)
			continue
		}
		out = append(out, entry)
	}
	if err := scanner.Err(); err != nil {
		collector.add("读取文本行失败: %v", err)
	}
	return out, nil
}

func csvCell(row []string, index int) string {
	if index < 0 || index >= len(row) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(row[index], "\ufeff"))
}

func rowIsEmpty(row []string) bool {
	for _, cell := range row {
		if strings.TrimSpace(cell) != "" {
			return false
		}
	}
	return true
}

type iocErrorCollector struct {
	items []string
	limit int
}

func (c *iocErrorCollector) add(format string, args ...interface{}) {
	if c == nil || len(c.items) >= c.limit {
		return
	}
	c.items = append(c.items, fmt.Sprintf(format, args...))
}

func truncateText(value string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max]) + "…"
}
