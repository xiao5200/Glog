package collector

import (
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"glog/pkg/models"
)

func writeGzip(t *testing.T, path, content string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create gzip: %v", err)
	}
	gw := gzip.NewWriter(f)
	if _, err := gw.Write([]byte(content)); err != nil {
		t.Fatalf("write gzip: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close file: %v", err)
	}
}

func TestLocalPathsPreviewAndCollect(t *testing.T) {
	dir := t.TempDir()
	plain := filepath.Join(dir, "app.log")
	plainContent := "line one\nline two\nline three\n"
	if err := os.WriteFile(plain, []byte(plainContent), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}
	gzPath := filepath.Join(dir, "old.log.gz")
	writeGzip(t, gzPath, "gz one\ngz two\n")
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "nested.log"), []byte("nested\n"), 0o644); err != nil {
		t.Fatalf("write nested: %v", err)
	}

	if ok, err := ValidatePath(plain); err != nil || !ok {
		t.Fatalf("validate path: ok=%v err=%v", ok, err)
	}
	if IsEVTX(plain) {
		t.Fatal("plain log should not be evtx")
	}
	if !IsEVTX("Security.evtx") {
		t.Fatal("expected evtx detection")
	}

	glob, err := ExpandPaths([]string{filepath.Join(dir, "*.log")}, false, nil)
	if err != nil {
		t.Fatalf("expand glob: %v", err)
	}
	if len(glob) != 1 || glob[0] != plain {
		t.Fatalf("unexpected glob result: %v", glob)
	}
	all, err := ExpandPaths([]string{dir}, true, []string{gzPath})
	if err != nil {
		t.Fatalf("expand recursive: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 files after exclusion, got %v", all)
	}

	preview, err := ReadFilePreview(gzPath, 2)
	if err != nil {
		t.Fatalf("preview gzip: %v", err)
	}
	if len(preview) != 2 || preview[1] != "gz two" {
		t.Fatalf("unexpected gzip preview: %v", preview)
	}
	entries, err := BrowseDirectory(dir)
	if err != nil {
		t.Fatalf("browse: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("unexpected browse entries: %v", entries)
	}

	var lines []models.RawLine
	col := NewLocalTextCollector([]string{plain}, false, nil, Options{
		ProjectID: "p", SourceID: "s", SourceHost: "h", Concurrency: 1, BufferSize: 4096,
	})
	result, err := col.Collect(context.Background(), func(line models.RawLine) error {
		lines = append(lines, line)
		return nil
	})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(lines) != 3 || len(result.Files) != 1 {
		t.Fatalf("unexpected collect result: lines=%d files=%v", len(lines), result.Files)
	}
	if lines[0].Text != "line one" || lines[2].Text != "line three" {
		t.Fatalf("unexpected lines: %+v", lines)
	}
	if lines[0].SourceID != "s" || lines[0].SourceHost != "h" {
		t.Fatalf("unexpected line source: %+v", lines[0])
	}

	common := GetCommonLogPaths("linux")
	if len(common) == 0 {
		t.Fatal("expected common log paths")
	}
}

func TestExclusionAndStopped(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.log", "b.log"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x\n"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	files, err := ExpandPaths([]string{dir}, true, []string{"*b.log"})
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if len(files) != 1 || !strings.HasSuffix(files[0], "a.log") {
		t.Fatalf("unexpected exclusion result: %v", files)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	col := NewLocalTextCollector([]string{filepath.Join(dir, "a.log")}, false, nil, Options{})
	_, err = col.Collect(ctx, func(models.RawLine) error { return nil })
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "cancel") {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}
