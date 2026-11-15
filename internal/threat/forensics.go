package threat

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"glog/internal/errs"
	"glog/internal/logx"
	"glog/pkg/models"
)

const (
	forensicsFormat  = "glog-forensics-package"
	forensicsVersion = "1.0"

	forensicsManifestName      = "manifest.json"
	forensicsInvestigationName = "investigation.json"
	forensicsEvidenceName      = "evidence.json"
	forensicsEntriesName       = "entries.jsonl"
	forensicsIOCMatchesName    = "ioc-matches.json"
	forensicsRuleResultsName   = "rule-results.json"
	forensicsReadmeName        = "README.txt"
)

var errForensicsZipTooLarge = errors.New("取证包超过大小限制")

type forensicsManifest struct {
	Format          string            `json:"format"`
	Version         string            `json:"version"`
	ProjectID       string            `json:"project_id"`
	InvestigationID string            `json:"investigation_id,omitempty"`
	GeneratedAt     time.Time         `json:"generated_at"`
	SHA256          map[string]string `json:"sha256"`
}

type forensicsZipEntry struct {
	Name string
	Data []byte
}

type forensicsEntryLine struct {
	Raw    string           `json:"raw"`
	Parsed *models.LogEntry `json:"parsed"`
}

// GenerateForensics 生成取证包 ZIP。归档内文件名固定，写入采用临时文件+rename 原子完成，
// 并在完成后计算整个 ZIP 的 SHA256。
func (s *Service) GenerateForensics(req models.ForensicsRequest, inv *models.Investigation, evidence []models.EvidenceItem) (*models.ForensicsResult, error) {
	generatedAt := time.Now().UTC()
	projectID := strings.TrimSpace(req.ProjectID)
	if projectID == "" && inv != nil {
		projectID = strings.TrimSpace(inv.ProjectID)
	}
	investigationID := strings.TrimSpace(req.InvestigationID)
	if inv != nil && strings.TrimSpace(inv.ID) != "" {
		investigationID = inv.ID
	}

	files := make(map[string][]byte)
	fileOrder := make([]string, 0, 8)

	if inv != nil {
		data, err := json.MarshalIndent(inv, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("序列化调查工单失败: %w", err)
		}
		files[forensicsInvestigationName] = data
		fileOrder = append(fileOrder, forensicsInvestigationName)
	}

	if evidence == nil {
		evidence = []models.EvidenceItem{}
	}
	evidenceJSON, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("序列化证据失败: %w", err)
	}
	files[forensicsEvidenceName] = evidenceJSON
	fileOrder = append(fileOrder, forensicsEvidenceName)

	entryCount := 0
	needEntries := req.IncludeEntries || len(req.DocIDs) > 0 || req.Query != nil
	if needEntries {
		entries, err := s.collectForensicsEntries(req)
		if err != nil {
			return nil, err
		}
		entriesJSONL := encodeForensicsEntries(entries)
		files[forensicsEntriesName] = entriesJSONL
		fileOrder = append(fileOrder, forensicsEntriesName)
		entryCount = len(entries)
	}

	if len(req.IOCValues) > 0 {
		var payload interface{}
		matches, err := s.SearchIOC(projectID, req.IOCValues, req.Query)
		if err != nil {
			logx.Warn(fmt.Sprintf("取证包IOC检索失败: %v", err))
			payload = map[string]string{"error": err.Error()}
		} else {
			payload = matches
		}
		data, marshalErr := json.MarshalIndent(payload, "", "  ")
		if marshalErr != nil {
			return nil, fmt.Errorf("序列化IOC检索结果失败: %w", marshalErr)
		}
		files[forensicsIOCMatchesName] = data
		fileOrder = append(fileOrder, forensicsIOCMatchesName)
	}

	if req.Query != nil {
		var payload interface{}
		detection, err := s.runDetection(projectID, nil, false)
		if err != nil {
			logx.Warn(fmt.Sprintf("取证包规则检测失败: %v", err))
			payload = map[string]string{"error": err.Error()}
		} else {
			payload = detection
		}
		data, marshalErr := json.MarshalIndent(payload, "", "  ")
		if marshalErr != nil {
			return nil, fmt.Errorf("序列化规则检测结果失败: %w", marshalErr)
		}
		files[forensicsRuleResultsName] = data
		fileOrder = append(fileOrder, forensicsRuleResultsName)
	}

	readme := buildForensicsReadme(projectID, investigationID, generatedAt, entryCount, fileOrder)
	files[forensicsReadmeName] = []byte(readme)
	fileOrder = append(fileOrder, forensicsReadmeName)

	hashes := make(map[string]string, len(files))
	for name, data := range files {
		hashes[name] = sha256HexBytes(data)
	}
	manifestJSON, err := json.MarshalIndent(forensicsManifest{
		Format:          forensicsFormat,
		Version:         forensicsVersion,
		ProjectID:       projectID,
		InvestigationID: investigationID,
		GeneratedAt:     generatedAt,
		SHA256:          hashes,
	}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("序列化取证包清单失败: %w", err)
	}

	zipEntries := make([]forensicsZipEntry, 0, len(fileOrder)+1)
	zipEntries = append(zipEntries, forensicsZipEntry{Name: forensicsManifestName, Data: manifestJSON})
	for _, name := range fileOrder {
		zipEntries = append(zipEntries, forensicsZipEntry{Name: name, Data: files[name]})
	}
	allNames := make([]string, 0, len(zipEntries))
	for _, entry := range zipEntries {
		allNames = append(allNames, entry.Name)
	}

	identifier := investigationID
	if identifier == "" {
		identifier = projectID
	}
	if identifier == "" {
		identifier = generatedAt.Format("20060102-150405")
	}
	defaultName := "glog-forensics-" + sanitizeForensicsComponent(identifier) + ".zip"
	absPath, err := resolveForensicsOutputPath(defaultName, req.OutputPath)
	if err != nil {
		return nil, err
	}
	if err := writeForensicsZipAtomic(absPath, zipEntries); err != nil {
		return nil, err
	}

	hash, size, err := hashFileSHA256(absPath)
	if err != nil {
		return nil, fmt.Errorf("计算取证包SHA256失败: %w", err)
	}
	return &models.ForensicsResult{
		Path:        absPath,
		Files:       allNames,
		EntryCount:  entryCount,
		Bytes:       size,
		SHA256:      hash,
		GeneratedAt: generatedAt,
	}, nil
}

func (s *Service) collectForensicsEntries(req models.ForensicsRequest) ([]*models.LogEntry, error) {
	if s.reader == nil {
		return nil, errs.Internal("日志读取器未配置", nil)
	}

	out := make([]*models.LogEntry, 0, 64)
	seen := make(map[uint64]struct{}, 64)
	add := func(entry *models.LogEntry) bool {
		if entry == nil {
			return len(out) < maxForensicsEntries
		}
		if _, ok := seen[entry.DocID]; ok {
			return true
		}
		seen[entry.DocID] = struct{}{}
		out = append(out, entry)
		return len(out) < maxForensicsEntries
	}

	if len(req.DocIDs) > 0 {
		requested := make([]uint32, 0, len(req.DocIDs))
		requestedSet := make(map[uint32]struct{}, len(req.DocIDs))
		for _, docID := range req.DocIDs {
			if len(requested) >= maxForensicsEntries {
				logx.Warn(fmt.Sprintf("取证包请求的日志ID超过上限 %d，已截断", maxForensicsEntries))
				break
			}
			if docID > uint64(^uint32(0)) {
				logx.Warn(fmt.Sprintf("取证包跳过超出范围的日志ID: %d", docID))
				continue
			}
			converted := uint32(docID)
			if _, ok := requestedSet[converted]; ok {
				continue
			}
			requestedSet[converted] = struct{}{}
			requested = append(requested, converted)
		}

		returned := make(map[uint32]struct{}, len(requested))
		entries, err := s.reader.ReadDocs(requested)
		if err != nil {
			logx.Warn(fmt.Sprintf("批量读取取证日志失败，改为逐条读取: %v", err))
		} else {
			for _, entry := range entries {
				if entry == nil {
					continue
				}
				if entry.DocID <= uint64(^uint32(0)) {
					returned[uint32(entry.DocID)] = struct{}{}
				}
				if !add(entry) {
					break
				}
			}
		}

		if len(out) < maxForensicsEntries {
			for _, docID := range requested {
				if _, ok := returned[docID]; ok {
					continue
				}
				entry, readErr := s.reader.ReadDoc(docID)
				if readErr != nil {
					logx.Warn(fmt.Sprintf("读取取证日志 %d 失败: %v", docID, readErr))
					continue
				}
				if !add(entry) {
					break
				}
			}
		}
	}

	if req.Query != nil && len(out) < maxForensicsEntries {
		query := *req.Query
		query.Offset = 0
		remaining := maxForensicsEntries - len(out)
		limit := remaining
		if query.Limit > 0 && query.Limit < limit {
			limit = query.Limit
		}
		query.Limit = limit
		res, err := s.reader.Search(query)
		if err != nil {
			logx.Warn(fmt.Sprintf("取证包查询日志失败: %v", err))
		} else if res != nil {
			for i := range res.Hits {
				if !add(&res.Hits[i]) {
					break
				}
			}
		}
	} else if req.IncludeEntries && len(req.DocIDs) == 0 && len(out) == 0 {
		res, err := s.reader.Search(models.Query{Limit: maxForensicsEntries, SortBy: "timestamp", SortOrder: "asc"})
		if err != nil {
			logx.Warn(fmt.Sprintf("取证包读取日志失败: %v", err))
		} else if res != nil {
			for i := range res.Hits {
				if !add(&res.Hits[i]) {
					break
				}
			}
		}
	}

	return out, nil
}

func encodeForensicsEntries(entries []*models.LogEntry) []byte {
	if len(entries) == 0 {
		return []byte{}
	}
	var builder strings.Builder
	encoder := json.NewEncoder(&builder)
	encoder.SetEscapeHTML(false)
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		_ = encoder.Encode(forensicsEntryLine{Raw: entry.RawLog, Parsed: entry})
	}
	return []byte(builder.String())
}

func buildForensicsReadme(projectID, investigationID string, generatedAt time.Time, entryCount int, fileNames []string) string {
	var builder strings.Builder
	builder.WriteString("GLog 取证包\n")
	builder.WriteString("====================\n")
	builder.WriteString(fmt.Sprintf("格式: %s\n", forensicsFormat))
	builder.WriteString(fmt.Sprintf("版本: %s\n", forensicsVersion))
	builder.WriteString(fmt.Sprintf("项目ID: %s\n", projectID))
	if investigationID != "" {
		builder.WriteString(fmt.Sprintf("调查工单ID: %s\n", investigationID))
	}
	builder.WriteString(fmt.Sprintf("生成时间(UTC): %s\n", generatedAt.Format(time.RFC3339)))
	builder.WriteString(fmt.Sprintf("日志条目数: %d\n\n", entryCount))
	builder.WriteString("文件清单:\n")
	for _, name := range fileNames {
		builder.WriteString("- " + name + "\n")
	}
	builder.WriteString("- " + forensicsManifestName + "（包含各文件 SHA256）\n\n")
	builder.WriteString("说明: 本包由 GLog 本地生成，未包含任何可执行文件。请妥善保管取证数据。\n")
	return builder.String()
}

func resolveForensicsOutputPath(defaultName, outputPath string) (string, error) {
	raw := strings.TrimSpace(outputPath)
	rawHadSeparator := strings.HasSuffix(raw, "/") || strings.HasSuffix(raw, "\\")
	if raw == "" {
		raw = filepath.Join(os.TempDir(), defaultName)
	}

	absPath, err := filepath.Abs(raw)
	if err != nil {
		return "", fmt.Errorf("解析取证包输出路径失败: %w", err)
	}

	if info, statErr := os.Stat(absPath); statErr == nil && info.IsDir() {
		absPath = filepath.Join(absPath, defaultName)
	} else if os.IsNotExist(statErr) && rawHadSeparator {
		absPath = filepath.Join(absPath, defaultName)
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return "", fmt.Errorf("检查取证包输出路径失败: %w", statErr)
	}

	if info, err := os.Lstat(absPath); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return "", errs.InvalidInput("取证包输出路径不能是符号链接", errs.ErrInvalidInput)
		}
		if !info.Mode().IsRegular() {
			return "", errs.InvalidInput("取证包输出路径不是普通文件", errs.ErrInvalidInput)
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("检查取证包输出文件失败: %w", err)
	}

	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("创建取证包输出目录失败: %w", err)
	}
	return absPath, nil
}

func writeForensicsZipAtomic(absPath string, entries []forensicsZipEntry) (err error) {
	dir := filepath.Dir(absPath)
	tmp, err := os.CreateTemp(dir, ".glog-forensics-*.tmp")
	if err != nil {
		return fmt.Errorf("创建取证包临时文件失败: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		_ = tmp.Close()
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()

	limited := &forensicsLimitedWriter{w: tmp, remaining: maxForensicsZipBytes}
	zw := zip.NewWriter(limited)
	modified := time.Now().UTC()
	for _, entry := range entries {
		header := &zip.FileHeader{
			Name:     entry.Name,
			Method:   zip.Deflate,
			Modified: modified,
		}
		writer, createErr := zw.CreateHeader(header)
		if createErr != nil {
			_ = zw.Close()
			return fmt.Errorf("创建ZIP条目 %s 失败: %w", entry.Name, createErr)
		}
		if _, writeErr := writer.Write(entry.Data); writeErr != nil {
			_ = zw.Close()
			if errors.Is(writeErr, errForensicsZipTooLarge) {
				return errForensicsZipTooLarge
			}
			return fmt.Errorf("写入ZIP条目 %s 失败: %w", entry.Name, writeErr)
		}
	}
	if err := zw.Close(); err != nil {
		if errors.Is(err, errForensicsZipTooLarge) {
			return errForensicsZipTooLarge
		}
		return fmt.Errorf("完成取证包ZIP写入失败: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("刷新取证包文件失败: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("关闭取证包文件失败: %w", err)
	}
	if err := os.Rename(tmpName, absPath); err != nil {
		return fmt.Errorf("保存取证包失败: %w", err)
	}
	cleanup = false
	return nil
}

func hashFileSHA256(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()

	hasher := sha256.New()
	size, err := io.Copy(hasher, file)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hasher.Sum(nil)), size, nil
}

func sha256HexBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func sanitizeForensicsComponent(raw string) string {
	var builder strings.Builder
	for _, r := range strings.TrimSpace(raw) {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '-', r == '_':
			builder.WriteRune(r)
		case r == '.':
			builder.WriteRune('-')
		}
	}
	out := builder.String()
	if out == "" {
		out = "unknown"
	}
	if len(out) > 64 {
		out = out[:64]
	}
	return out
}

type forensicsLimitedWriter struct {
	w         io.Writer
	remaining int64
}

func (l *forensicsLimitedWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > l.remaining {
		return 0, errForensicsZipTooLarge
	}
	n, err := l.w.Write(p)
	l.remaining -= int64(n)
	return n, err
}
