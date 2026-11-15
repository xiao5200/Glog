package storage

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"glog/internal/errs"
	"glog/pkg/models"
)

func TestMetadataStoreLifecycle(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "metadata.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer store.Close()

	now := time.Now().UTC()
	project := &models.Project{
		ID:          "proj-1",
		Name:        "测试项目",
		Description: "desc",
		CreatedAt:   now,
		UpdatedAt:   now,
		Status:      "active",
		ProjectPath: t.TempDir(),
		Levels:      map[string]int64{"INFO": 2, "ERROR": 1},
		Tags:        []string{"test"},
		Priority:    "high",
	}
	if err := store.CreateProject(project); err != nil {
		t.Fatalf("create project: %v", err)
	}
	got, err := store.GetProject(project.ID)
	if err != nil {
		t.Fatalf("get project: %v", err)
	}
	if got.Name != project.Name || got.Levels["ERROR"] != 1 {
		t.Fatalf("unexpected project: %+v", got)
	}
	if _, err := store.GetProject("missing"); !errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
	list, err := store.ListProjects(models.ProjectFilter{Status: "active"})
	if err != nil || len(list) != 1 {
		t.Fatalf("list projects: len=%d err=%v", len(list), err)
	}

	project.Name = "更新项目"
	project.UpdatedAt = now.Add(time.Minute)
	if err := store.UpdateProject(project); err != nil {
		t.Fatalf("update project: %v", err)
	}
	got, _ = store.GetProject(project.ID)
	if got.Name != "更新项目" {
		t.Fatalf("update not applied: %+v", got)
	}

	template := &models.ParseTemplate{
		ID: "tpl-1", Name: "模板", Type: "custom", Category: "test",
		Format: "regex", Regex: `^(\w+) (.*)$`,
		FieldMap:  map[string]int{"level": 1, "message": 2},
		LevelMap:  map[string]string{"ERR": "ERROR"},
		CreatedAt: now, UpdatedAt: now, Source: "user",
	}
	if err := store.SaveTemplate(template); err != nil {
		t.Fatalf("save template: %v", err)
	}
	tpl, err := store.GetTemplate("tpl-1")
	if err != nil || tpl.FieldMap["message"] != 2 {
		t.Fatalf("get template: %+v err=%v", tpl, err)
	}
	templates, err := store.ListTemplates(models.TemplateFilter{Type: "custom"})
	if err != nil || len(templates) != 1 {
		t.Fatalf("list templates: len=%d err=%v", len(templates), err)
	}
	if err := store.IncrementTemplateUsage("tpl-1"); err != nil {
		t.Fatalf("increment usage: %v", err)
	}

	if err := store.SaveSSHHost(&models.SSHHost{
		ID: "ssh-1", Name: "host", Host: "127.0.0.1", Port: 22,
		Username: "root", AuthMethod: "password", HasPassword: true,
	}, "encrypted-secret"); err != nil {
		t.Fatalf("save ssh host: %v", err)
	}
	host, enc, err := store.GetSSHHost("ssh-1")
	if err != nil || host.Username != "root" || enc != "encrypted-secret" {
		t.Fatalf("get ssh host: %+v %q err=%v", host, enc, err)
	}
	hosts, err := store.ListSSHHosts()
	if err != nil || len(hosts) != 1 {
		t.Fatalf("list ssh hosts: %d %v", len(hosts), err)
	}
	if err := store.TouchSSHHost("ssh-1"); err != nil {
		t.Fatalf("touch ssh host: %v", err)
	}

	inv := &models.Investigation{
		ID: "inv-1", ProjectID: project.ID, Title: "调查",
		ClueType: "ip", ClueValue: "1.2.3.4", Status: "open",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := store.CreateInvestigation(inv); err != nil {
		t.Fatalf("create investigation: %v", err)
	}
	mark := &models.EvidenceMark{
		ID: "ev-1", InvestigationID: inv.ID, DocID: 7,
		LogEntryID: "7", MarkedAt: now, Note: "note",
	}
	if err := store.AddEvidence(mark); err != nil {
		t.Fatalf("add evidence: %v", err)
	}
	// 幂等。
	if err := store.AddEvidence(mark); err != nil {
		t.Fatalf("add evidence idempotent: %v", err)
	}
	evidence, err := store.ListEvidence(inv.ID)
	if err != nil || len(evidence) != 1 {
		t.Fatalf("list evidence: len=%d err=%v", len(evidence), err)
	}
	gotInv, err := store.GetInvestigation(inv.ID)
	if err != nil || len(gotInv.EvidenceIDs) != 1 {
		t.Fatalf("investigation evidence ids: %+v err=%v", gotInv, err)
	}
	if err := store.RemoveEvidence(inv.ID, 7); err != nil {
		t.Fatalf("remove evidence: %v", err)
	}
	evidence, _ = store.ListEvidence(inv.ID)
	if len(evidence) != 0 {
		t.Fatalf("expected evidence removed")
	}

	snapshot := &models.QuerySnapshot{
		ID: "snap-1", ProjectID: project.ID, Name: "快照",
		Query: models.Query{Text: "error"}, ResultCount: 3, CreatedAt: now,
	}
	if err := store.SaveQuerySnapshot(snapshot); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}
	snapshots, err := store.ListQuerySnapshots(project.ID)
	if err != nil || len(snapshots) != 1 {
		t.Fatalf("list snapshots: %d %v", len(snapshots), err)
	}

	aiRec := &AIConfigRecord{
		Provider: "ollama", Endpoint: "http://localhost:11434/v1",
		Model: "llama3", Enabled: true, Priority: 10,
		Parameters: "{}", UpdatedAt: now,
	}
	if err := store.SaveAIConfigRecord(aiRec); err != nil {
		t.Fatalf("save ai config: %v", err)
	}
	rec, err := store.GetAIConfigRecord("ollama")
	if err != nil || rec.Model != "llama3" {
		t.Fatalf("get ai config: %+v err=%v", rec, err)
	}
	recs, err := store.ListAIConfigRecords()
	if err != nil || len(recs) != 1 {
		t.Fatalf("list ai configs: %d %v", len(recs), err)
	}

	rule := &models.DetectionRule{
		ID: "rule-1", Name: "规则", Severity: "high", Enabled: true,
		Tags: []string{"test"}, MITRETactics: []string{"Credential Access"},
	}
	if err := store.UpsertDetectionRule(rule); err != nil {
		t.Fatalf("upsert rule: %v", err)
	}
	if err := store.SetDetectionRuleEnabled("rule-1", false); err != nil {
		t.Fatalf("disable rule: %v", err)
	}
	rules, err := store.ListDetectionRules()
	if err != nil || len(rules) != 1 || rules[0].Enabled {
		t.Fatalf("list rules: %+v err=%v", rules, err)
	}
	if err := store.UpdateDetectionRuleMatch("rule-1", now); err != nil {
		t.Fatalf("update rule match: %v", err)
	}

	ioc := &models.IOCItem{
		ID: "ioc-1", Type: "ip", Value: "203.0.113.5",
		ThreatLevel: "high", CreatedAt: now,
	}
	if err := store.SaveIOC(ioc); err != nil {
		t.Fatalf("save ioc: %v", err)
	}
	found, err := store.FindIOCByValue("203.0.113.5")
	if err != nil || found.ID != "ioc-1" {
		t.Fatalf("find ioc: %+v err=%v", found, err)
	}
	iocs, err := store.ListIOCs()
	if err != nil || len(iocs) != 1 {
		t.Fatalf("list iocs: %d %v", len(iocs), err)
	}

	if err := store.SetSetting("k", "v"); err != nil {
		t.Fatalf("set setting: %v", err)
	}
	v, err := store.GetSetting("k")
	if err != nil || v != "v" {
		t.Fatalf("get setting: %q err=%v", v, err)
	}

	if err := store.DeleteProject(project.ID); err != nil {
		t.Fatalf("delete project: %v", err)
	}
	if _, err := store.GetProject(project.ID); !errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("expected project deleted, got %v", err)
	}
}
