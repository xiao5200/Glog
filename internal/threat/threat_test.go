package threat

import (
	"os"
	"path/filepath"
	"testing"

	"glog/internal/storage"
)

func TestSeedBuiltinRulesAndIOCImport(t *testing.T) {
	root := t.TempDir()
	store, err := storage.Open(filepath.Join(root, "metadata.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	svc := NewService(store, nil)
	if err := svc.SeedBuiltinRules(); err != nil {
		t.Fatalf("seed rules: %v", err)
	}
	rules, err := svc.ListRules()
	if err != nil {
		t.Fatalf("list rules: %v", err)
	}
	if len(rules) < 10 {
		t.Fatalf("expected >=10 builtin rules, got %d", len(rules))
	}
	found := false
	for _, rule := range rules {
		if rule.ID == "builtin-ssh-brute" {
			found = true
			if !rule.Enabled || rule.Severity != "high" {
				t.Fatalf("unexpected ssh rule: %+v", rule)
			}
		}
	}
	if !found {
		t.Fatal("builtin-ssh-brute rule missing")
	}
	if err := svc.SetRuleEnabled("builtin-ssh-brute", false); err != nil {
		t.Fatalf("disable rule: %v", err)
	}
	rules, _ = svc.ListRules()
	for _, rule := range rules {
		if rule.ID == "builtin-ssh-brute" && rule.Enabled {
			t.Fatal("rule should be disabled")
		}
	}

	csvPath := filepath.Join(root, "ioc.csv")
	csv := "type,value,threat_level,description\nip,203.0.113.5,high,test\ndomain,evil.example,medium,test\nurl,https://evil.example/x,low,test\n"
	if err := os.WriteFile(csvPath, []byte(csv), 0o644); err != nil {
		t.Fatalf("write csv: %v", err)
	}
	result, err := svc.ImportIOC(csvPath)
	if err != nil {
		t.Fatalf("import ioc: %v", err)
	}
	if result.Imported != 3 {
		t.Fatalf("expected 3 imported IOCs, got %+v", result)
	}
	iocs, err := svc.ListIOCs()
	if err != nil || len(iocs) != 3 {
		t.Fatalf("list iocs: %d err=%v", len(iocs), err)
	}

	jsonPath := filepath.Join(root, "ioc.json")
	if err := os.WriteFile(jsonPath, []byte(`[{"type":"sha256","value":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","threat_level":"critical"}]`), 0o644); err != nil {
		t.Fatalf("write json: %v", err)
	}
	jsonResult, err := svc.ImportIOC(jsonPath)
	if err != nil || jsonResult.Imported != 1 {
		t.Fatalf("import json ioc: %+v err=%v", jsonResult, err)
	}
}
