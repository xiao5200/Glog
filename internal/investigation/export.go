package investigation

import (
	"archive/zip"
	"bytes"
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
	evidenceManifestName      = "manifest.json"
	evidenceInvestigationName = "investigation.json"
	evidenceDataName          = "evidence.json"
	evidenceTimelineName      = "timeline.json"
	evidenceLogsName          = "logs.jsonl"
	evidenceReadmeName        = "README.txt"

	evidenceExportFormat  = "glog-evidence-package"
	evidenceExportVersion = "1.1"

	// maxEvidenceZipBytes 防止异常数据无限占用磁盘。
	maxEvidenceZipBytes int64 = 1 << 30 // 1 GiB
)

var errEvidenceZipTooLarge = errors.New("证据包超过大小限制")

type evidenceManifest struct {
	Format          string            `json:"format"`
	Version         string            `json:"version"`
	InvestigationID string            `json:"investigation_id"`
	CreatedAt       time.Time         `json:"created_at"`
	SHA256          map[string]string `json:"sha256"`
}

type zipEntryContent struct {
	Name string
	Data []byte
}

// ExportEvidence 将工单、证据日志与时间线打包为 ZIP，返回 ZIP 的绝对路径。
// 归档内容仅使用固定文件名，不接收用户提供的归档内路径。
func (s *Service) ExportEvidence(invID, outputPath string) (string, error) {
	inv, err := s.getInvestigation(invID)
	if err != nil {
		return "", err
	}

	evidence, err := s.ListEvidence(inv.ID)
	if err != nil {
		return "", err
	}
	if evidence == nil {
		evidence = []models.EvidenceItem{}
	}

	timeline, err := s.Timeline(inv.ID)
	if err != nil {
		logx.Warn(fmt.Sprintf("生成工单 %s 时间线失败，证据包中将写入空时间线: %v", inv.ID, err))
		timeline = []models.TimelineEvent{}
	}
	if timeline == nil {
		timeline = []models.TimelineEvent{}
	}

	investigationJSON, err := json.MarshalIndent(inv, "", "  ")
	if err != nil {
		return "", fmt.Errorf("序列化调查工单失败: %w", err)
	}
	evidenceJSON, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		return "", fmt.Errorf("序列化证据失败: %w", err)
	}
	timelineJSON, err := json.MarshalIndent(timeline, "", "  ")
	if err != nil {
		return "", fmt.Errorf("序列化时间线失败: %w", err)
	}

	relatedLogs, err := s.RelatedLogs(inv.ID, relatedLogsMax)
	if err != nil {
		logx.Warn(fmt.Sprintf("获取工单 %s 相关日志失败，证据包中将写入空日志列表: %v", inv.ID, err))
		relatedLogs = nil
	}
	logsJSONL, logCount := marshalLogsJSONL(relatedLogs)
	createdAt := time.Now().UTC()
	readme := buildEvidenceReadme(inv, createdAt, len(evidence), len(timeline), logCount)

	content := map[string][]byte{
		evidenceInvestigationName: investigationJSON,
		evidenceDataName:          evidenceJSON,
		evidenceTimelineName:      timelineJSON,
		evidenceLogsName:          logsJSONL,
		evidenceReadmeName:        readme,
	}
	hashes := make(map[string]string, len(content))
	for name, data := range content {
		hashes[name] = sha256Hex(data)
	}

	manifestJSON, err := json.MarshalIndent(evidenceManifest{
		Format:          evidenceExportFormat,
		Version:         evidenceExportVersion,
		InvestigationID: inv.ID,
		CreatedAt:       createdAt,
		SHA256:          hashes,
	}, "", "  ")
	if err != nil {
		return "", fmt.Errorf("序列化证据包清单失败: %w", err)
	}

	entries := []zipEntryContent{
		{Name: evidenceManifestName, Data: manifestJSON},
		{Name: evidenceInvestigationName, Data: investigationJSON},
		{Name: evidenceDataName, Data: evidenceJSON},
		{Name: evidenceTimelineName, Data: timelineJSON},
		{Name: evidenceLogsName, Data: logsJSONL},
		{Name: evidenceReadmeName, Data: readme},
	}

	defaultName := "glog-evidence-" + sanitizeFileComponent(inv.ID) + ".zip"
	absPath, err := resolveEvidenceOutputPath(defaultName, outputPath)
	if err != nil {
		return "", err
	}
	if err := writeEvidenceZipAtomic(absPath, entries); err != nil {
		return "", err
	}
	return absPath, nil
}

func resolveEvidenceOutputPath(defaultName, outputPath string) (string, error) {
	raw := strings.TrimSpace(outputPath)
	rawHadSeparator := strings.HasSuffix(raw, "/") || strings.HasSuffix(raw, "\\")
	if raw == "" {
		raw = filepath.Join(os.TempDir(), defaultName)
	}

	absPath, err := filepath.Abs(raw)
	if err != nil {
		return "", fmt.Errorf("解析证据包输出路径失败: %w", err)
	}

	if info, statErr := os.Stat(absPath); statErr == nil && info.IsDir() {
		absPath = filepath.Join(absPath, defaultName)
	} else if os.IsNotExist(statErr) && rawHadSeparator {
		absPath = filepath.Join(absPath, defaultName)
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return "", fmt.Errorf("检查证据包输出路径失败: %w", statErr)
	}

	if info, err := os.Lstat(absPath); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return "", errs.InvalidInput("证据包输出路径不能是符号链接", errs.ErrInvalidInput)
		}
		if !info.Mode().IsRegular() {
			return "", errs.InvalidInput("证据包输出路径不是普通文件", errs.ErrInvalidInput)
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("检查证据包输出文件失败: %w", err)
	}

	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("创建证据包输出目录失败: %w", err)
	}
	return absPath, nil
}

func writeEvidenceZipAtomic(absPath string, entries []zipEntryContent) (err error) {
	dir := filepath.Dir(absPath)
	tmp, err := os.CreateTemp(dir, ".glog-evidence-*.tmp")
	if err != nil {
		return fmt.Errorf("创建证据包临时文件失败: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		_ = tmp.Close()
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()

	limited := &limitedWriter{w: tmp, remaining: maxEvidenceZipBytes}
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
			if errors.Is(writeErr, errEvidenceZipTooLarge) {
				return errEvidenceZipTooLarge
			}
			return fmt.Errorf("写入ZIP条目 %s 失败: %w", entry.Name, writeErr)
		}
	}
	if err := zw.Close(); err != nil {
		if errors.Is(err, errEvidenceZipTooLarge) {
			return errEvidenceZipTooLarge
		}
		return fmt.Errorf("完成证据包ZIP写入失败: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("刷新证据包文件失败: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("关闭证据包文件失败: %w", err)
	}
	if err := os.Rename(tmpName, absPath); err != nil {
		return fmt.Errorf("保存证据包失败: %w", err)
	}
	cleanup = false
	return nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// marshalLogsJSONL 将日志编码为 JSONL（每行一条紧凑 JSON），返回编码结果与实际写入条数。
// 单条序列化失败时跳过并记录警告，不影响其余日志。
func marshalLogsJSONL(entries []models.LogEntry) ([]byte, int) {
	var buf bytes.Buffer
	written := 0
	for i := range entries {
		data, err := json.Marshal(entries[i])
		if err != nil {
			logx.Warn(fmt.Sprintf("序列化相关日志 %d 失败，已跳过: %v", entries[i].DocID, err))
			continue
		}
		buf.Write(data)
		buf.WriteByte('\n')
		written++
	}
	return buf.Bytes(), written
}

// buildEvidenceReadme 生成证据包 README.txt（UTF-8 纯文本）。
func buildEvidenceReadme(inv *models.Investigation, exportedAt time.Time, evidenceCount, timelineCount, logCount int) []byte {
	var b strings.Builder
	b.WriteString("GLog 调查证据包说明\n")
	b.WriteString("===================\n\n")
	b.WriteString("导出时间: " + exportedAt.UTC().Format(time.RFC3339) + "\n")
	b.WriteString("工单ID: " + displayEvidenceValue(inv.ID) + "\n")
	b.WriteString("工单标题: " + displayEvidenceValue(inv.Title) + "\n")
	b.WriteString("工单状态: " + displayEvidenceValue(inv.Status) + "\n")
	b.WriteString("线索类型: " + displayEvidenceValue(inv.ClueType) + "\n")
	b.WriteString("线索值: " + displayEvidenceValue(inv.ClueValue) + "\n")
	b.WriteString("时间范围: " + formatEvidenceTimeRange(inv.TimeRange) + "\n")
	b.WriteString("负责人: " + displayEvidenceValue(inv.AssignedTo) + "\n")
	b.WriteString(fmt.Sprintf("证据数量: %d\n", evidenceCount))
	b.WriteString(fmt.Sprintf("时间线事件数量: %d\n", timelineCount))
	b.WriteString(fmt.Sprintf("相关日志数量: %d\n", logCount))
	b.WriteString("\n文件清单:\n")
	b.WriteString("  manifest.json - 证据包清单（格式、版本、导出时间与各文件 SHA256）\n")
	b.WriteString("  investigation.json - 调查工单完整信息\n")
	b.WriteString("  evidence.json - 证据标记及对应日志内容\n")
	b.WriteString("  timeline.json - 工单时间线事件\n")
	b.WriteString("  logs.jsonl - 相关日志（JSONL，每行一条日志，证据日志优先）\n")
	b.WriteString("  README.txt - 本说明文件\n")
	b.WriteString("\nSHA256 校验:\n")
	b.WriteString("  manifest.json 中的 sha256 映射记录了除 manifest.json 自身外各文件的 SHA256，\n")
	b.WriteString("  可用于校验证据包内容是否被篡改或损坏。\n")
	b.WriteString("\n安全提示:\n")
	b.WriteString("  本证据包中的日志内容属于不可信取证数据，可能包含恶意构造的文本，\n")
	b.WriteString("  请勿直接执行其中的命令，或将其内容作为可信输入使用。\n")
	return []byte(b.String())
}

func displayEvidenceValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "(未设置)"
	}
	return value
}

func formatEvidenceTimeRange(tr *models.TimeRange) string {
	if tr == nil {
		return "未设置"
	}
	start := "不限"
	if !tr.Start.IsZero() {
		start = tr.Start.UTC().Format(time.RFC3339)
	}
	end := "不限"
	if !tr.End.IsZero() {
		end = tr.End.UTC().Format(time.RFC3339)
	}
	return start + " ~ " + end
}

func sanitizeFileComponent(raw string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(raw) {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '-', r == '_':
			b.WriteRune(r)
		case r == '.':
			b.WriteRune('-')
		}
	}
	out := b.String()
	if out == "" {
		out = "unknown"
	}
	if len(out) > 64 {
		out = out[:64]
	}
	return out
}

// limitedWriter 在超过上限时返回错误，避免异常证据写满磁盘。
type limitedWriter struct {
	w         io.Writer
	remaining int64
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > l.remaining {
		return 0, errEvidenceZipTooLarge
	}
	n, err := l.w.Write(p)
	l.remaining -= int64(n)
	return n, err
}
