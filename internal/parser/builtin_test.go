package parser

import (
	"context"
	"strings"
	"testing"
	"time"

	"glog/pkg/models"
)

func TestBuiltinTemplatesParseExamples(t *testing.T) {
	engine, err := NewEngine()
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	templates := engine.Templates()
	if len(templates) < 15 {
		t.Fatalf("expected >=15 builtin templates, got %d", len(templates))
	}

	for i := range templates {
		tpl := templates[i]
		if len(tpl.Examples) == 0 {
			t.Errorf("template %s has no examples", tpl.ID)
			continue
		}
		success := 0
		for _, example := range tpl.Examples {
			entry, err := engine.Parse(example, &tpl, ParseContext{
				SourceID:    "src",
				SourceHost:  "test-host",
				SourceFile:  "test.log",
				CollectedAt: time.Now().UTC(),
			})
			if err != nil {
				t.Logf("template %s example parse failed: %v | %s", tpl.ID, err, truncateForTest(example, 120))
				continue
			}
			if entry == nil {
				t.Logf("template %s returned nil entry", tpl.ID)
				continue
			}
			if strings.TrimSpace(entry.Message) == "" {
				t.Logf("template %s produced empty message", tpl.ID)
				continue
			}
			success++
		}
		if success == 0 {
			t.Errorf("template %s: no example parsed successfully", tpl.ID)
		}
	}
}

func TestNginxLevelAndTime(t *testing.T) {
	engine, err := NewEngine()
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	tpl, ok := engine.Template("nginx-combined")
	if !ok {
		t.Fatal("nginx-combined template missing")
	}
	entry, err := engine.Parse(
		`203.0.113.9 - admin [10/Oct/2024:13:55:38 +0800] "GET /admin HTTP/1.1" 500 42 "-" "sqlmap/1.7"`,
		tpl,
		ParseContext{SourceHost: "web-01", CollectedAt: time.Now()},
	)
	if err != nil {
		t.Fatalf("parse nginx: %v", err)
	}
	if entry.Level != models.LevelError {
		t.Errorf("expected ERROR from status 500, got %s", entry.Level)
	}
	if entry.Fields == nil || entry.Fields["remote_addr"] == nil {
		t.Errorf("expected remote_addr field")
	}
	if entry.Timestamp.IsZero() {
		t.Errorf("expected parsed timestamp")
	}
}

func TestJSONAndSyslogDetection(t *testing.T) {
	engine, err := NewEngine()
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	jsonSamples := []string{
		`{"timestamp":"2024-10-10T14:00:02Z","level":"error","message":"connection refused","host":"web-01","status":502}`,
		`{"timestamp":"2024-10-10T14:00:03Z","level":"info","message":"ok","host":"web-01"}`,
	}
	det := engine.DetectFormat(jsonSamples)
	if det.DetectedFormat != "json" {
		t.Errorf("expected json detection, got %+v", det)
	}
	tpl, confidence := engine.DetectTemplate(jsonSamples)
	if tpl == nil || confidence <= 0 {
		t.Fatalf("expected detected template, got %v confidence=%f", tpl, confidence)
	}
	entry, err := engine.Parse(jsonSamples[0], tpl, ParseContext{CollectedAt: time.Now()})
	if err != nil {
		t.Fatalf("parse json: %v", err)
	}
	if entry.Level != models.LevelError || entry.Message != "connection refused" {
		t.Errorf("unexpected json entry: level=%s message=%s", entry.Level, entry.Message)
	}

	syslogSamples := []string{
		`Oct 10 14:00:01 web-01 sshd[12345]: Failed password for root from 203.0.113.5 port 22 ssh2`,
	}
	if det := engine.DetectFormat(syslogSamples); det.DetectedFormat != "syslog" {
		t.Errorf("expected syslog detection, got %+v", det)
	}
}

func TestPreviewAndAITemplateGeneration(t *testing.T) {
	engine, err := NewEngine()
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	tpl, ok := engine.Template("nginx-combined")
	if !ok {
		t.Fatal("nginx template missing")
	}
	preview := engine.Preview(*tpl, []string{
		`127.0.0.1 - - [10/Oct/2024:13:55:36 +0800] "GET / HTTP/1.1" 200 1 "-" "curl"`,
		`not a matching line`,
	})
	if len(preview.Samples) != 2 {
		t.Fatalf("expected 2 preview samples, got %d", len(preview.Samples))
	}
	if preview.SuccessRate <= 0 || preview.SuccessRate >= 1 {
		t.Errorf("expected partial success rate, got %f", preview.SuccessRate)
	}

	response := "```json\n" + `{
		"name":"Custom App",
		"description":"custom",
		"format":"regex",
		"regex":"^(\\d{4}-\\d{2}-\\d{2}) (\\w+) (.*)$",
		"field_map":{"timestamp":1,"level":2,"message":3},
		"time_format":"2006-01-02",
		"time_field":"timestamp",
		"level_field":"level",
		"level_map":{"INFO":"INFO","ERROR":"ERROR"},
		"examples":["2024-10-10 INFO hello"]
	}` + "\n```"
	generated, err := engine.GenerateTemplateWithAI(context.Background(), []string{"2024-10-10 INFO hello"}, "Custom", "app",
		func(context.Context, string, string) (string, error) { return response, nil })
	if err != nil {
		t.Fatalf("generate template: %v", err)
	}
	if generated.Format != "regex" || generated.Regex == "" || generated.Type != "ai_generated" {
		t.Fatalf("unexpected generated template: %+v", generated)
	}
	entry, err := engine.Parse("2024-10-10 INFO hello", generated, ParseContext{CollectedAt: time.Now()})
	if err != nil {
		t.Fatalf("parse generated template: %v", err)
	}
	if entry.Message != "hello" || entry.Level != models.LevelInfo {
		t.Errorf("unexpected generated parse: %+v", entry)
	}
}

func TestMultilineAssembler(t *testing.T) {
	asm := NewMultilineAssembler(&models.MultiLineConfig{
		Enabled:  true,
		Pattern:  `^\d{4}-\d{2}-\d{2}`,
		MaxLines: 10,
	})
	if out, ready := asm.Add("2024-01-01 first line"); ready || out != "" {
		t.Fatalf("unexpected flush on first line")
	}
	if out, ready := asm.Add("stack trace"); ready || out != "" {
		t.Fatalf("unexpected flush on continuation")
	}
	out, ready := asm.Add("2024-01-02 second line")
	if !ready {
		t.Fatalf("expected flush when new start line arrives")
	}
	if !strings.Contains(out, "stack trace") {
		t.Fatalf("unexpected multiline content: %q", out)
	}
	tail := asm.Flush()
	if !strings.Contains(tail, "second line") {
		t.Fatalf("unexpected tail: %q", tail)
	}
}

func truncateForTest(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
