package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"glog/pkg/models"
)

func TestManagerCompleteAndTest(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["model"] != "test-model" {
			t.Errorf("unexpected model: %v", body["model"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices":[{"message":{"role":"assistant","content":"{\"ok\":true}"}}]
		}`))
	}))
	defer server.Close()

	mgr := NewManager([]Config{{
		Provider: "custom",
		Endpoint: server.URL + "/v1",
		Model:    "test-model",
		APIKey:   "sk-secret-123",
		Enabled:  true,
		Priority: 100,
		Timeout:  2 * time.Second,
	}})

	provider, err := mgr.SelectedProvider("")
	if err != nil {
		t.Fatalf("select provider: %v", err)
	}
	if provider != "custom" {
		t.Fatalf("unexpected provider %s", provider)
	}
	content, err := mgr.Complete(context.Background(), "custom", "system", "prompt", true)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if !strings.Contains(content, `"ok":true`) {
		t.Fatalf("unexpected content: %s", content)
	}
	if gotAuth != "Bearer sk-secret-123" {
		t.Fatalf("unexpected auth header: %q", gotAuth)
	}
	if err := mgr.Test(context.Background(), "custom"); err != nil {
		t.Fatalf("test: %v", err)
	}
}

func TestManagerErrorsDoNotLeakKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid key sk-secret-123"}}`))
	}))
	defer server.Close()

	mgr := NewManager([]Config{{
		Provider: "custom",
		Endpoint: server.URL + "/v1",
		Model:    "test-model",
		APIKey:   "sk-secret-123",
		Enabled:  true,
	}})
	_, err := mgr.Complete(context.Background(), "custom", "s", "p", false)
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "sk-secret-123") {
		t.Fatalf("API key leaked in error: %v", err)
	}

	if _, err := mgr.Complete(context.Background(), "not-configured", "s", "p", false); err == nil {
		t.Fatal("expected unconfigured provider error")
	}
}

func TestParseJSONInto(t *testing.T) {
	type payload struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}
	var target payload
	raw := "说明文字\n```json\n{\"name\":\"glog\",\"count\":3,}\n```\n结束"
	if err := ParseJSONInto(raw, &target); err != nil {
		t.Fatalf("parse fenced json: %v", err)
	}
	if target.Name != "glog" || target.Count != 3 {
		t.Fatalf("unexpected target: %+v", target)
	}
	if err := ParseJSONInto("not json", &target); err == nil {
		t.Fatal("expected invalid JSON error")
	}
}

func TestParseAnomalyResponse(t *testing.T) {
	raw := `{
		"summary":"发现异常登录",
		"risk_level":"HIGH",
		"findings":["短时间大量失败"],
		"suggestions":["封禁来源 IP"],
		"anomalies":[
			{"timestamp":"2024-10-10T14:00:00Z","level":"ERROR","message":"failed","reason":"bruteforce","score":120}
		]
	}`
	res, err := ParseAnomalyResponse(raw)
	if err != nil {
		t.Fatalf("parse anomaly: %v", err)
	}
	if res.RiskLevel != "high" {
		t.Fatalf("unexpected risk level: %s", res.RiskLevel)
	}
	if len(res.Anomalies) != 1 || res.Anomalies[0].Score > 100 {
		t.Fatalf("unexpected anomalies: %+v", res.Anomalies)
	}
}

func TestParseNLQueryResponse(t *testing.T) {
	raw := `{"text":"level:ERROR","levels":["ERROR","error"],"start_time":"2024-01-01T00:00:00Z","limit":5000}`
	query, err := ParseNLQueryResponse(raw)
	if err != nil {
		t.Fatalf("parse nl query: %v", err)
	}
	if query.Text != "level:ERROR" || query.Limit != 1000 {
		t.Fatalf("unexpected query: %+v", query)
	}
	if len(query.Levels) != 1 || query.Levels[0] != models.LevelError {
		t.Fatalf("unexpected levels: %+v", query.Levels)
	}
	if query.StartTime == nil || !query.StartTime.Equal(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected start time: %v", query.StartTime)
	}
}

func TestPromptBuilders(t *testing.T) {
	entries := []models.LogEntry{{
		Timestamp:  time.Date(2024, 10, 10, 14, 0, 0, 0, time.UTC),
		Level:      models.LevelError,
		Message:    "connection refused",
		SourceHost: "web-01",
	}}
	system, user := BuildAnomalyPrompt(entries, models.IndexStatistics{TotalDocuments: 1})
	if system == "" || !strings.Contains(user, "connection refused") {
		t.Fatalf("unexpected anomaly prompt: %q / %q", system, user)
	}
	system, user = BuildRootCausePrompt(entries, "为什么失败")
	if !strings.Contains(system, "JSON") || !strings.Contains(user, "为什么失败") {
		t.Fatalf("unexpected root cause prompt")
	}
	system, user = BuildNLQueryPrompt("找错误", "项目摘要")
	if !strings.Contains(system, "Query") || user == "" {
		t.Fatalf("unexpected nl prompt")
	}
	formatted := FormatEntriesForPrompt(entries, 10, 100)
	if !strings.Contains(formatted, "ERROR") || !strings.Contains(formatted, "web-01") {
		t.Fatalf("unexpected formatted entries: %q", formatted)
	}
	system, user = BuildReportPrompt("标题", entries, nil)
	if system == "" || !strings.Contains(user, "标题") {
		t.Fatalf("unexpected report prompt")
	}
}

func ExampleExtractJSON() {
	fmt.Println(ExtractJSON("prefix {\"a\":1} suffix"))
	// Output: {"a":1}
}
