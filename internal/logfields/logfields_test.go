package logfields

import (
	"testing"

	"glog/pkg/models"
)

func TestNormalizeKey(t *testing.T) {
	cases := map[string]string{
		"Source IP":      "sourceip",
		"destination-ip": "destinationip",
		"EventType":      "eventtype",
		"":               "",
	}
	for raw, want := range cases {
		if got := NormalizeKey(raw); got != want {
			t.Fatalf("NormalizeKey(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestExtractIP(t *testing.T) {
	nested := map[string]interface{}{
		"EventData": map[string]interface{}{
			"Source IP":      "10.1.0.6",
			"Destination-IP": "10.1.0.9",
		},
		"DestinationIp": "10.0.0.9",
	}
	if got, ok := ExtractIP(nested, SourceIPAliases); !ok || got != "10.1.0.6" {
		t.Fatalf("source ip = %q, %v; want 10.1.0.6, true", got, ok)
	}
	if got, ok := ExtractIP(nested, DestinationIPAliases); !ok || got != "10.0.0.9" {
		t.Fatalf("destination ip = %q, %v; want 10.0.0.9, true", got, ok)
	}

	if got, ok := ExtractIP(map[string]interface{}{"RemoteAddr": "192.0.2.10"}, SourceIPAliases); !ok || got != "192.0.2.10" {
		t.Fatalf("remote_addr alias = %q, %v", got, ok)
	}
	if _, ok := ExtractIP(map[string]interface{}{"ClientIp": " - "}, SourceIPAliases); ok {
		t.Fatal("placeholder '-' should be treated as missing")
	}
	if _, ok := ExtractIP(map[string]interface{}{"DestinationIp": "10.0.0.9"}, SourceIPAliases); ok {
		t.Fatal("source IP aliases must not pick destination values")
	}
	if _, ok := ExtractIP(nil, SourceIPAliases); ok {
		t.Fatal("nil fields should not yield a value")
	}
}

func TestEventType(t *testing.T) {
	provider := "Microsoft-Windows-Security-Auditing"
	cases := []struct {
		name  string
		entry *models.LogEntry
		want  string
		ok    bool
	}{
		{
			name: "channel 优先于 provider",
			entry: &models.LogEntry{
				Provider: &provider,
				Fields:   map[string]interface{}{"Channel": "Security"},
			},
			want: "Security",
			ok:   true,
		},
		{
			name: "event_type 优先于 channel",
			entry: &models.LogEntry{
				Provider: &provider,
				Fields:   map[string]interface{}{"Channel": "System", "EventType": "Security"},
			},
			want: "Security",
			ok:   true,
		},
		{
			name:  "provider 回退",
			entry: &models.LogEntry{Provider: &provider},
			want:  provider,
			ok:    true,
		},
		{
			name:  "源文件名回退",
			entry: &models.LogEntry{SourceFile: "C:/logs/Application.evtx"},
			want:  "Application.evtx",
			ok:    true,
		},
		{
			name:  "来源回退",
			entry: &models.LogEntry{Source: "custom-source"},
			want:  "custom-source",
			ok:    true,
		},
		{
			name:  "空通道与空 provider 视为缺失",
			entry: &models.LogEntry{Fields: map[string]interface{}{"Channel": "  "}},
			want:  "",
			ok:    false,
		},
		{
			name:  "nil entry",
			entry: nil,
			want:  "",
			ok:    false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := EventType(tc.entry)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("EventType() = %q, %v; want %q, %v", got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestValueString(t *testing.T) {
	cases := []struct {
		value interface{}
		want  string
	}{
		{"text", "text"},
		{int64(4625), "4625"},
		{uint32(7), "7"},
		{float64(1.5), "1.5"},
		{true, "true"},
		{[]interface{}{"a", nil, int64(2)}, "a,2"},
	}
	for _, tc := range cases {
		if got := ValueString(tc.value); got != tc.want {
			t.Fatalf("ValueString(%v) = %q, want %q", tc.value, got, tc.want)
		}
	}
}
