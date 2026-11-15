package project

import (
	"os"
	"path/filepath"
	"testing"

	"glog/internal/storage"
	"glog/pkg/models"
)

func TestProjectCreateUpdateExportImport(t *testing.T) {
	root := t.TempDir()
	store1, err := storage.Open(filepath.Join(root, "one", "metadata.db"))
	if err != nil {
		t.Fatalf("open store1: %v", err)
	}
	defer store1.Close()
	svc1 := New(store1, filepath.Join(root, "one", "projects"))

	p, err := svc1.Create(models.CreateProjectRequest{
		Name:        "导出项目",
		Description: "source",
		Tags:        []string{"a", "b"},
		Priority:    "high",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if p.ID == "" || p.ProjectPath == "" {
		t.Fatalf("unexpected project: %+v", p)
	}
	// 新建项目必须是干净项目：无日志、无来源、无级别分布，raw/indexes 目录为空。
	if p.LogCount != 0 || len(p.Sources) != 0 || len(p.Levels) != 0 {
		t.Fatalf("new project should be clean: %+v", p)
	}
	for _, sub := range []string{"raw", "indexes"} {
		dir := filepath.Join(p.ProjectPath, sub)
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("expected %s directory: %v", sub, err)
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read %s directory: %v", sub, err)
		}
		if len(entries) != 0 {
			t.Fatalf("new project %s directory should be empty, got %d entries", sub, len(entries))
		}
	}

	name := "更新名称"
	updated, err := svc1.Update(p.ID, models.ProjectUpdates{Name: &name})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != name {
		t.Fatalf("unexpected updated name: %s", updated.Name)
	}
	if err := svc1.Archive(p.ID); err != nil {
		t.Fatalf("archive: %v", err)
	}
	archived, err := svc1.Get(p.ID)
	if err != nil || archived.Status != "archived" {
		t.Fatalf("expected archived, got %+v err=%v", archived, err)
	}

	meta, err := svc1.ReadMeta(p.ID)
	if err != nil || meta.Name != name {
		t.Fatalf("read meta: %+v err=%v", meta, err)
	}

	zipPath := filepath.Join(root, "export", "project.zip")
	exported, err := svc1.Export(p.ID, zipPath)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	info, err := os.Stat(exported)
	if err != nil || info.Size() == 0 {
		t.Fatalf("exported file missing: %v", err)
	}

	store2, err := storage.Open(filepath.Join(root, "two", "metadata.db"))
	if err != nil {
		t.Fatalf("open store2: %v", err)
	}
	defer store2.Close()
	svc2 := New(store2, filepath.Join(root, "two", "projects"))
	imported, err := svc2.Import(exported)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if imported.ID != p.ID || imported.Name != name {
		t.Fatalf("unexpected imported project: %+v", imported)
	}
	got, err := svc2.Get(imported.ID)
	if err != nil || got.Name != name {
		t.Fatalf("get imported: %+v err=%v", got, err)
	}
	if _, err := os.Stat(filepath.Join(got.ProjectPath, "project.meta")); err != nil {
		t.Fatalf("imported project.meta missing: %v", err)
	}

	stats, err := svc2.RecomputeStats(imported.ID)
	if err != nil {
		t.Fatalf("recompute stats: %v", err)
	}
	if stats == nil || stats.SourceCount != 0 {
		t.Fatalf("unexpected stats: %+v", stats)
	}

	if err := svc2.Delete(imported.ID); err != nil {
		t.Fatalf("delete imported: %v", err)
	}
	if _, err := os.Stat(got.ProjectPath); !os.IsNotExist(err) {
		t.Fatalf("project directory should be removed, err=%v", err)
	}
}
