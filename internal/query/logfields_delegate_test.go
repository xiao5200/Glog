package query

import (
	"testing"

	"glog/internal/logfields"
	"glog/pkg/models"
)

// TestExtractFieldValueDelegatesToLogfields 验证查询侧派生字段提取与
// internal/logfields 完全一致：source_ip/destination_ip/event_type 均返回
// string + bool，且 Channel 优先于 Provider。
func TestExtractFieldValueDelegatesToLogfields(t *testing.T) {
	provider := "Microsoft-Windows-Security-Auditing"
	entry := &models.LogEntry{
		Provider: &provider,
		Fields: map[string]interface{}{
			"Channel":       "Security",
			"SourceIp":      "10.0.0.6",
			"DestinationIp": "10.0.0.9",
		},
	}

	cases := []struct {
		field string
		want  string
	}{
		{"source_ip", "10.0.0.6"},
		{"destination_ip", "10.0.0.9"},
		{"event_type", "Security"}, // Channel 优先于 Provider
	}
	for _, tc := range cases {
		got, ok := extractFieldValue(entry, tc.field)
		if !ok {
			t.Errorf("extractFieldValue(%q) ok = false, want true", tc.field)
			continue
		}
		text, isString := got.(string)
		if !isString {
			t.Errorf("extractFieldValue(%q) 返回 %T, want string", tc.field, got)
			continue
		}
		if text != tc.want {
			t.Errorf("extractFieldValue(%q) = %q, want %q", tc.field, text, tc.want)
		}
	}

	// 与 logfields 的直接调用结果保持逐字段一致。
	if got, ok := logfields.EventType(entry); !ok || got != "Security" {
		t.Errorf("logfields.EventType = (%q, %v), want (Security, true)", got, ok)
	}
	if got, ok := logfields.ExtractIP(entry.Fields, logfields.SourceIPAliases); !ok || got != "10.0.0.6" {
		t.Errorf("logfields.ExtractIP(source) = (%q, %v), want (10.0.0.6, true)", got, ok)
	}
	if got, ok := logfields.ExtractIP(entry.Fields, logfields.DestinationIPAliases); !ok || got != "10.0.0.9" {
		t.Errorf("logfields.ExtractIP(destination) = (%q, %v), want (10.0.0.9, true)", got, ok)
	}

	// 缺失字段仍返回 ok=false，不破坏原有语义。
	empty := &models.LogEntry{}
	for _, field := range []string{"source_ip", "destination_ip", "event_type"} {
		if _, ok := extractFieldValue(empty, field); ok {
			t.Errorf("extractFieldValue(%q) on empty entry ok = true, want false", field)
		}
	}
}
