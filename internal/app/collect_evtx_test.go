package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExplicitEVTXPaths(t *testing.T) {
	dir := t.TempDir()
	evtxA := filepath.Join(dir, "Setup.evtx")
	evtxB := filepath.Join(dir, "System.EVTX")
	text := filepath.Join(dir, "app.log")
	for _, path := range []string{evtxA, evtxB, text} {
		if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	paths, ok := explicitEVTXPaths([]string{evtxA, " " + evtxB + " "})
	if !ok || len(paths) != 2 {
		t.Fatalf("expected two EVTX paths, got ok=%v paths=%v", ok, paths)
	}
	if paths[0] != evtxA || paths[1] != evtxB {
		t.Fatalf("unexpected paths: %v", paths)
	}

	if _, ok := explicitEVTXPaths([]string{evtxA, text}); ok {
		t.Fatal("mixed EVTX and text paths must not be treated as EVTX-only")
	}
	if _, ok := explicitEVTXPaths([]string{filepath.Join(dir, "*.evtx")}); ok {
		t.Fatal("wildcard paths must not be treated as explicit EVTX")
	}
	if _, ok := explicitEVTXPaths(nil); ok {
		t.Fatal("empty path list must not be treated as EVTX-only")
	}
}
