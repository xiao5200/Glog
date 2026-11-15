package app

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"glog/pkg/models"
)

func newTestApp(t *testing.T) *App {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("GLOG_DATA_DIR", dir)

	a := NewApp()
	a.Startup(context.Background())
	if a.startupErr != nil {
		t.Fatalf("startup failed: %v", a.startupErr)
	}
	// 测试环境没有 Wails 前端，事件推送替换为空实现。
	a.taskMgr.emit = func(string, interface{}) {}
	t.Cleanup(func() {
		a.Shutdown(context.Background())
	})
	return a
}

func waitTask(t *testing.T, a *App, taskID string, timeout time.Duration) models.TaskInfo {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		tasks, err := a.ListTasks()
		if err != nil {
			t.Fatalf("list tasks: %v", err)
		}
		for _, task := range tasks {
			if task.ID != taskID {
				continue
			}
			switch task.Status {
			case "success", "failed", "stopped":
				return task
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("task %s did not finish in %s", taskID, timeout)
	return models.TaskInfo{}
}

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestGLogEndToEnd(t *testing.T) {
	a := newTestApp(t)

	project, err := a.CreateProject(models.CreateProjectRequest{
		Name:        "集成测试项目",
		Description: "e2e",
		Tags:        []string{"test"},
		Priority:    "high",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if project.LogCount != 0 || len(project.Sources) != 0 {
		t.Fatalf("new project should be clean: %+v", project)
	}
	if err := a.OpenProject(project.ID); err != nil {
		t.Fatalf("open project: %v", err)
	}
	empty, err := a.Search(project.ID, models.Query{Limit: 10})
	if err != nil {
		t.Fatalf("search fresh project: %v", err)
	}
	if empty.Total != 0 || len(empty.Hits) != 0 {
		t.Fatalf("fresh project should have no logs: %+v", empty)
	}

	workDir := t.TempDir()
	nginxLog := writeFile(t, workDir, "access.log", strings.Join([]string{
		`127.0.0.1 - - [10/Oct/2024:13:55:36 +0800] "GET /index.html HTTP/1.1" 200 612 "-" "Mozilla/5.0"`,
		`203.0.113.9 - admin [10/Oct/2024:13:55:37 +0800] "POST /login HTTP/1.1" 401 128 "https://example.com/" "curl/8.0"`,
		`203.0.113.9 - admin [10/Oct/2024:13:55:38 +0800] "GET /admin HTTP/1.1" 500 42 "-" "sqlmap/1.7"`,
	}, "\n"))

	task, err := a.StartLocalCollection(models.LocalCollectRequest{
		ProjectID: project.ID,
		FilePaths: []string{nginxLog},
		Parse:     models.ParseConfig{Mode: "builtin", TemplateID: "nginx-combined"},
	})
	if err != nil {
		t.Fatalf("start local collection: %v", err)
	}
	done := waitTask(t, a, task.ID, 15*time.Second)
	if done.Status != "success" {
		t.Fatalf("collection status=%s error=%s", done.Status, done.Error)
	}
	if done.Result == nil || done.Result.LogCount != 3 {
		t.Fatalf("unexpected collect result: %+v", done.Result)
	}

	res, err := a.Search(project.ID, models.Query{Text: "GET", Limit: 20})
	if err != nil {
		t.Fatalf("search GET: %v", err)
	}
	if res.Total != 2 {
		t.Fatalf("expected 2 GET logs, got %d", res.Total)
	}

	res, err = a.Search(project.ID, models.Query{Levels: []string{"ERROR"}, Limit: 20})
	if err != nil {
		t.Fatalf("search level: %v", err)
	}
	if res.Total < 1 {
		t.Fatalf("expected ERROR log, got %d", res.Total)
	}

	agg, err := a.Aggregate(project.ID, models.AggregateRequest{
		Query:       models.Query{},
		AggregateBy: []string{"level"},
		TopN:        10,
	})
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if agg.Total != 3 {
		t.Fatalf("aggregate total=%d", agg.Total)
	}

	cluster, err := a.ClusterPatterns(project.ID, models.Query{})
	if err != nil {
		t.Fatalf("cluster: %v", err)
	}
	if cluster.TotalCount == 0 || len(cluster.Patterns) == 0 {
		t.Fatalf("unexpected cluster result: %+v", cluster)
	}

	if _, err := a.GetLogContext(project.ID, 0, 1, 1); err != nil {
		t.Fatalf("context: %v", err)
	}

	// 追加 SSH 日志用于规则检测与 IOC 检索。
	sshLog := writeFile(t, workDir, "auth.log", strings.Join(func() []string {
		lines := make([]string, 0, 12)
		for i := 0; i < 12; i++ {
			lines = append(lines, fmt.Sprintf(
				`Oct 10 14:0%d:%02d web-01 sshd[%d]: Failed password for root from 203.0.113.5 port %d ssh2`,
				i/60, i%60, 1000+i, 55000+i,
			))
		}
		return lines
	}(), "\n"))
	task, err = a.StartLocalCollection(models.LocalCollectRequest{
		ProjectID: project.ID,
		FilePaths: []string{sshLog},
		Parse:     models.ParseConfig{Mode: "builtin", TemplateID: "sshd"},
	})
	if err != nil {
		t.Fatalf("start ssh log collection: %v", err)
	}
	waitDone := waitTask(t, a, task.ID, 15*time.Second)
	if waitDone.Status != "success" {
		t.Fatalf("ssh log collection status=%s error=%s", waitDone.Status, waitDone.Error)
	}

	stats, err := a.GetProjectStatistics(project.ID)
	if err != nil {
		t.Fatalf("project stats: %v", err)
	}
	if stats.TotalLogs != 15 {
		t.Fatalf("expected 15 logs, got %d", stats.TotalLogs)
	}

	detection, err := a.RunDetection(models.RunDetectionRequest{
		ProjectID: project.ID,
		RuleIDs:   []string{"builtin-ssh-brute"},
	})
	if err != nil {
		t.Fatalf("run detection: %v", err)
	}
	if detection.TotalMatches < 1 {
		t.Fatalf("expected ssh brute force detection, got %+v", detection)
	}

	iocCSV := writeFile(t, workDir, "ioc.csv", "type,value,threat_level,description\nip,203.0.113.5,high,e2e\n")
	imported, err := a.ImportIOC(iocCSV)
	if err != nil {
		t.Fatalf("import ioc: %v", err)
	}
	if imported.Imported < 1 {
		t.Fatalf("expected imported IOC, got %+v", imported)
	}
	iocResult, err := a.SearchIOC(models.IOCSearchRequest{
		ProjectID: project.ID,
		Values:    []string{"203.0.113.5"},
	})
	if err != nil {
		t.Fatalf("search ioc: %v", err)
	}
	if iocResult.Total < 1 {
		t.Fatalf("expected IOC matches, got %+v", iocResult)
	}

	inv, err := a.CreateInvestigation(models.CreateInvestigationRequest{
		ProjectID:   project.ID,
		Title:       "SSH 暴破调查",
		Description: "e2e",
		ClueType:    "ip",
		ClueValue:   "203.0.113.5",
		Tags:        []string{"brute_force"},
		DocIDs:      []uint64{0},
	})
	if err != nil {
		t.Fatalf("create investigation: %v", err)
	}
	evidence, err := a.ListEvidence(inv.ID)
	if err != nil {
		t.Fatalf("list evidence: %v", err)
	}
	if len(evidence) < 1 {
		t.Fatalf("expected evidence, got %d", len(evidence))
	}
	timeline, err := a.GetInvestigationTimeline(inv.ID)
	if err != nil {
		t.Fatalf("timeline: %v", err)
	}
	if len(timeline) < 1 {
		t.Fatalf("expected timeline events")
	}

	// 编辑线索与时间范围、清空风险评分。
	riskScore := float32(55)
	if _, err := a.UpdateInvestigation(inv.ID, models.InvestigationUpdates{RiskScore: &riskScore}); err != nil {
		t.Fatalf("set risk score: %v", err)
	}
	clueType := "ip"
	clueValue := "203.0.113.9"
	timeStart := time.Date(2024, 10, 10, 0, 0, 0, 0, time.UTC)
	timeEnd := time.Date(2024, 10, 11, 0, 0, 0, 0, time.UTC)
	clearRisk := true
	updated, err := a.UpdateInvestigation(inv.ID, models.InvestigationUpdates{
		ClueType:       &clueType,
		ClueValue:      &clueValue,
		TimeRange:      &models.TimeRange{Start: timeStart, End: timeEnd},
		ClearRiskScore: &clearRisk,
	})
	if err != nil {
		t.Fatalf("update investigation: %v", err)
	}
	if updated.ClueType != clueType || updated.ClueValue != clueValue || updated.TimeRange == nil || updated.RiskScore != nil {
		t.Fatalf("unexpected updated investigation: %+v", updated)
	}
	clearTimeRange := true
	cleared, err := a.UpdateInvestigation(inv.ID, models.InvestigationUpdates{ClearTimeRange: &clearTimeRange})
	if err != nil {
		t.Fatalf("clear time range: %v", err)
	}
	if cleared.TimeRange != nil {
		t.Fatalf("expected cleared time range, got %+v", cleared.TimeRange)
	}
	if _, err := a.UpdateInvestigation(inv.ID, models.InvestigationUpdates{
		TimeRange: &models.TimeRange{Start: timeStart, End: timeEnd},
	}); err != nil {
		t.Fatalf("restore time range: %v", err)
	}

	related, err := a.GetInvestigationRelatedLogs(inv.ID, 10)
	if err != nil {
		t.Fatalf("related logs: %v", err)
	}
	if len(related) < 1 {
		t.Fatalf("expected related logs, got %d", len(related))
	}

	evidencePath := filepath.Join(workDir, "evidence.zip")
	exportedPath, err := a.ExportEvidence(inv.ID, evidencePath)
	if err != nil {
		t.Fatalf("export evidence: %v", err)
	}
	if fi, err := os.Stat(exportedPath); err != nil || fi.Size() == 0 {
		t.Fatalf("evidence zip missing: %v", err)
	}

	zipEntries := readZipEntries(t, exportedPath)
	for _, name := range []string{
		"manifest.json", "investigation.json", "evidence.json",
		"timeline.json", "logs.jsonl", "README.txt",
	} {
		if _, ok := zipEntries[name]; !ok {
			t.Fatalf("evidence zip missing %s", name)
		}
	}
	if !strings.Contains(string(zipEntries["README.txt"]), "SSH 暴破调查") {
		t.Fatalf("README.txt missing investigation title")
	}
	logsData := bytes.TrimSpace(zipEntries["logs.jsonl"])
	if len(logsData) == 0 {
		t.Fatalf("logs.jsonl should not be empty when related logs exist")
	}
	for i, line := range strings.Split(string(logsData), "\n") {
		var entry models.LogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("logs.jsonl line %d invalid: %v", i+1, err)
		}
	}
	var manifest struct {
		Version string            `json:"version"`
		SHA256  map[string]string `json:"sha256"`
	}
	if err := json.Unmarshal(zipEntries["manifest.json"], &manifest); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	if manifest.Version != "1.1" {
		t.Fatalf("unexpected manifest version: %s", manifest.Version)
	}
	if _, ok := manifest.SHA256["manifest.json"]; ok {
		t.Fatalf("manifest sha256 should not include manifest.json itself")
	}
	for _, name := range []string{
		"investigation.json", "evidence.json", "timeline.json", "logs.jsonl", "README.txt",
	} {
		if manifest.SHA256[name] != sha256HexForTest(zipEntries[name]) {
			t.Fatalf("manifest sha256 mismatch for %s", name)
		}
	}

	// 创建并删除临时工单，确认删除后无法读取。
	throwaway, err := a.CreateInvestigation(models.CreateInvestigationRequest{
		ProjectID: project.ID,
		Title:     "待删除工单",
	})
	if err != nil {
		t.Fatalf("create throwaway investigation: %v", err)
	}
	if err := a.DeleteInvestigation(throwaway.ID); err != nil {
		t.Fatalf("delete investigation: %v", err)
	}
	if _, err := a.GetInvestigation(throwaway.ID); err == nil {
		t.Fatalf("expected error getting deleted investigation")
	}

	forensics, err := a.GenerateForensicsPackage(models.ForensicsRequest{
		ProjectID:       project.ID,
		InvestigationID: inv.ID,
		OutputPath:      filepath.Join(workDir, "forensics.zip"),
		IOCValues:       []string{"203.0.113.5"},
		IncludeEntries:  true,
		IncludeEvidence: true,
	})
	if err != nil {
		t.Fatalf("forensics: %v", err)
	}
	if forensics.Path == "" || forensics.SHA256 == "" || forensics.EntryCount == 0 {
		t.Fatalf("unexpected forensics result: %+v", forensics)
	}

	projectExport := filepath.Join(workDir, "project.zip")
	exportPath, err := a.ExportProject(project.ID, projectExport)
	if err != nil {
		t.Fatalf("export project: %v", err)
	}
	if fi, err := os.Stat(exportPath); err != nil || fi.Size() == 0 {
		t.Fatalf("project export missing: %v", err)
	}

	templates, err := a.GetTemplates(models.TemplateFilter{Type: "all"})
	if err != nil {
		t.Fatalf("templates: %v", err)
	}
	if len(templates) < 15 {
		t.Fatalf("expected builtin templates, got %d", len(templates))
	}
	presets, err := a.GetMiddlewarePresets()
	if err != nil {
		t.Fatalf("presets: %v", err)
	}
	if len(presets) < 8 {
		t.Fatalf("expected presets, got %d", len(presets))
	}

	if err := a.CloseProject(); err != nil {
		t.Fatalf("close project: %v", err)
	}
	if err := a.OpenProject(project.ID); err != nil {
		t.Fatalf("reopen project: %v", err)
	}
	reopened, err := a.Search(project.ID, models.Query{Text: "Failed password", Limit: 20})
	if err != nil {
		t.Fatalf("search after reopen: %v", err)
	}
	if reopened.Total < 12 {
		t.Fatalf("expected persisted logs after reopen, got %d", reopened.Total)
	}

	if runtime.GOOS != "linux" {
		if _, err := a.StartJournalCollection(models.SystemdJournalRequest{ProjectID: project.ID}); err == nil {
			t.Fatalf("expected unsupported OS error for journalctl")
		}
	}

	if err := a.TestAIConnection("openai"); err == nil {
		t.Fatalf("expected unconfigured AI provider error")
	}
}

func readZipEntries(t *testing.T, path string) map[string][]byte {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open evidence zip: %v", err)
	}
	defer zr.Close()
	entries := make(map[string][]byte, len(zr.File))
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open zip entry %s: %v", f.Name, err)
		}
		data, readErr := io.ReadAll(rc)
		closeErr := rc.Close()
		if readErr != nil {
			t.Fatalf("read zip entry %s: %v", f.Name, readErr)
		}
		if closeErr != nil {
			t.Fatalf("close zip entry %s: %v", f.Name, closeErr)
		}
		entries[f.Name] = data
	}
	return entries
}

func sha256HexForTest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
