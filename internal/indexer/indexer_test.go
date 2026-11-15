package indexer

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/RoaringBitmap/roaring"

	"glog/pkg/models"
)

func hasToken(tokens []string, want string) bool {
	for _, token := range tokens {
		if token == want {
			return true
		}
	}
	return false
}

func TestTokenizer(t *testing.T) {
	tokens := Tokenize("Failed password for root from 192.168.1.100")
	for _, want := range []string{"failed", "password", "root", "192.168.1.100", "192", "168"} {
		if !hasToken(tokens, want) {
			t.Errorf("expected token %q in %v", want, tokens)
		}
	}

	cjk := Tokenize("用户登录失败")
	for _, want := range []string{"用", "用户", "户", "户登", "登录", "失败"} {
		if !hasToken(cjk, want) {
			t.Errorf("expected CJK token %q in %v", want, cjk)
		}
	}

	if tokens := Tokenize(""); tokens != nil {
		t.Errorf("expected nil tokens for empty input, got %v", tokens)
	}
}

func TestAppendReadAndStats(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ts := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	entries := []*models.LogEntry{
		{Timestamp: ts, Level: models.LevelInfo, Message: "started", SourceHost: "host-a", Source: "src"},
		{Timestamp: ts.Add(time.Minute), Level: models.LevelError, Message: "failed", SourceHost: "host-b", Source: "src"},
	}
	if err := store.Append(entries); err != nil {
		t.Fatalf("append: %v", err)
	}
	if entries[0].DocID != 0 || entries[0].ID != "0" || entries[1].DocID != 1 {
		t.Fatalf("unexpected doc ids: %+v", entries)
	}
	if store.DocCount() != 2 {
		t.Fatalf("unexpected doc count: %d", store.DocCount())
	}
	got, err := store.ReadDoc(1)
	if err != nil || got.Message != "failed" || got.Level != models.LevelError {
		t.Fatalf("read doc: %+v err=%v", got, err)
	}
	if len(store.Postings("failed")) != 1 || store.Postings("failed")[0] != 1 {
		t.Fatalf("unexpected postings: %v", store.Postings("failed"))
	}
	if len(store.PostingsPrefix("fail")) != 1 {
		t.Fatalf("unexpected prefix postings: %v", store.PostingsPrefix("fail"))
	}
	rangeIDs := store.TimeRange(ts, ts.Add(30*time.Second))
	if len(rangeIDs) != 1 || rangeIDs[0] != 0 {
		t.Fatalf("unexpected time range: %v", rangeIDs)
	}
	if ids := store.FieldBitmap("level", models.LevelError); len(ids) != 1 || ids[0] != 1 {
		t.Fatalf("unexpected level bitmap: %v", ids)
	}
	if values := store.FieldValues("source_host"); len(values) != 2 {
		t.Fatalf("unexpected field values: %v", values)
	}
	fullCounts, ok := store.FieldCounts("level", nil)
	if !ok || fullCounts[models.LevelInfo] != 1 || fullCounts[models.LevelError] != 1 {
		t.Fatalf("unexpected full field counts: %v ok=%v", fullCounts, ok)
	}
	filter := roaring.New()
	filter.Add(1)
	filteredCounts, ok := store.FieldCounts("level", filter)
	if !ok || filteredCounts[models.LevelError] != 1 || len(filteredCounts) != 1 {
		t.Fatalf("unexpected filtered field counts: %v ok=%v", filteredCounts, ok)
	}
	if _, ok := store.FieldCounts("missing_field", nil); ok {
		t.Fatal("missing field should report ok=false")
	}
	if counts := store.LevelCounts(); counts[models.LevelError] != 1 || counts[models.LevelInfo] != 1 {
		t.Fatalf("unexpected level counts: %v", counts)
	}
	stats := store.Stats()
	if stats.TotalDocuments != 2 || stats.TimeRange == nil {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if err := store.Finalize(); err != nil {
		t.Fatalf("finalize: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "raw", "logs.dat")); err != nil {
		t.Fatalf("logs.dat missing: %v", err)
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()
	got, err = reopened.ReadDoc(1)
	if err != nil || got.Message != "failed" {
		t.Fatalf("read after reopen: %+v err=%v", got, err)
	}
	if reopened.DocCount() != 2 {
		t.Fatalf("unexpected reopened doc count: %d", reopened.DocCount())
	}
}

// assertDerivedBitmaps 校验派生字段（event_type/source_ip/destination_ip）的
// bitmap 与取值集合。测试数据约定：
//   - doc0：Channel=Security（优先于 Provider），SourceIp/DestinationIp 均为顶层键；
//   - doc1：无 Channel，回退 Provider；SourceIp 位于嵌套 EventData 中；
//   - doc2：无 Provider，回退源文件名。
func assertDerivedBitmaps(t *testing.T, store *Store, provider string) {
	t.Helper()

	bitmapCases := []struct {
		field string
		value string
		want  []uint32
	}{
		{"event_type", "Security", []uint32{0}},
		{"event_type", provider, []uint32{1}},
		{"event_type", "System.evtx", []uint32{2}},
		{"source_ip", "10.0.0.6", []uint32{0}},
		{"source_ip", "10.0.0.7", []uint32{1}},
		{"destination_ip", "10.0.0.9", []uint32{0}},
	}
	for _, tc := range bitmapCases {
		if got := store.FieldBitmap(tc.field, tc.value); !slices.Equal(got, tc.want) {
			t.Errorf("FieldBitmap(%q, %q) = %v, want %v", tc.field, tc.value, got, tc.want)
		}
	}

	valuesCases := []struct {
		field string
		want  []string
	}{
		{"event_type", []string{provider, "Security", "System.evtx"}},
		{"source_ip", []string{"10.0.0.6", "10.0.0.7"}},
		{"destination_ip", []string{"10.0.0.9"}},
	}
	for _, tc := range valuesCases {
		if got := store.FieldValues(tc.field); !slices.Equal(got, tc.want) {
			t.Errorf("FieldValues(%q) = %v, want %v", tc.field, got, tc.want)
		}
	}

	// 无 IP 的文档不应产生空值位图。
	if got := store.FieldBitmap("source_ip", ""); len(got) != 0 {
		t.Errorf("source_ip 不应索引空值，got %v", got)
	}
}

func TestDerivedFieldBitmaps(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	provider := "Microsoft-Windows-Security-Auditing"
	ts := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	entries := []*models.LogEntry{
		{
			Timestamp: ts,
			Level:     models.LevelInfo,
			Message:   "channel wins",
			Provider:  &provider,
			Fields: map[string]interface{}{
				"Channel":       "Security",
				"SourceIp":      "10.0.0.6",
				"DestinationIp": "10.0.0.9",
			},
		},
		{
			Timestamp: ts.Add(time.Minute),
			Level:     models.LevelInfo,
			Message:   "provider fallback",
			Provider:  &provider,
			Fields: map[string]interface{}{
				"EventData": map[string]interface{}{"SourceIp": "10.0.0.7"},
			},
		},
		{
			Timestamp:  ts.Add(2 * time.Minute),
			Level:      models.LevelWarn,
			Message:    "file fallback",
			SourceFile: "C:/logs/System.evtx",
		},
	}
	if err := store.Append(entries); err != nil {
		t.Fatalf("append: %v", err)
	}
	assertDerivedBitmaps(t, store, provider)

	if err := store.Finalize(); err != nil {
		t.Fatalf("finalize: %v", err)
	}
	builtBefore := store.Stats().LastBuildTime
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	reopened, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()
	// 重新打开应走 bitmap v2 加载路径而不是重建。
	assertDerivedBitmaps(t, reopened, provider)
	if got := reopened.Stats().LastBuildTime; !got.Equal(builtBefore) {
		t.Fatalf("bitmap v2 索引应直接加载而非重建：built time %v -> %v", builtBefore, got)
	}
}

func TestBitmapIndexVersionMismatchTriggersRebuild(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	provider := "Microsoft-Windows-Security-Auditing"
	entry := &models.LogEntry{
		Timestamp: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
		Level:     models.LevelInfo,
		Message:   "versioned",
		Provider:  &provider,
		Fields: map[string]interface{}{
			"Channel":  "Security",
			"SourceIp": "10.0.0.6",
		},
	}
	if err := store.Append([]*models.LogEntry{entry}); err != nil {
		t.Fatalf("append: %v", err)
	}
	if err := store.Finalize(); err != nil {
		t.Fatalf("finalize: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	bitmapPath := filepath.Join(dir, "indexes", "bitmap.idx")
	data, err := os.ReadFile(bitmapPath)
	if err != nil {
		t.Fatalf("read bitmap.idx: %v", err)
	}
	if len(data) < bitmapHeaderSize {
		t.Fatalf("bitmap.idx 过短: %d", len(data))
	}
	if got := binary.LittleEndian.Uint32(data[8:12]); got != bitmapFormatVersion {
		t.Fatalf("finalize 应写入 bitmapFormatVersion=%d, got %d", bitmapFormatVersion, got)
	}
	// 模拟旧项目：把版本改回 1，Open 应检测到不兼容并整库重建。
	binary.LittleEndian.PutUint32(data[8:12], 1)
	if err := os.WriteFile(bitmapPath, data, 0o644); err != nil {
		t.Fatalf("write bitmap.idx: %v", err)
	}

	reopened, err := Open(dir)
	if err != nil {
		t.Fatalf("open with stale bitmap version should rebuild: %v", err)
	}
	defer reopened.Close()
	if ids := reopened.FieldBitmap("event_type", "Security"); !slices.Equal(ids, []uint32{0}) {
		t.Fatalf("重建后 event_type bitmap = %v, want [0]", ids)
	}
	rewritten, err := os.ReadFile(bitmapPath)
	if err != nil {
		t.Fatalf("read rebuilt bitmap.idx: %v", err)
	}
	if got := binary.LittleEndian.Uint32(rewritten[8:12]); got != bitmapFormatVersion {
		t.Fatalf("重建后 bitmap.idx 版本 = %d, want %d", got, bitmapFormatVersion)
	}
}
