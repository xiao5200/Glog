package collector

import (
	"errors"
	"strings"
	"testing"
	"time"

	"glog/internal/errs"
	"glog/pkg/models"
)

func TestBuildSSHLogFindCommandDefaults(t *testing.T) {
	cmd, err := buildSSHLogFindCommand(SSHLogSearchOptions{})
	if err != nil {
		t.Fatalf("build command: %v", err)
	}
	if !strings.HasPrefix(cmd, "find '/var/log' -xdev ") {
		t.Fatalf("unexpected command prefix: %s", cmd)
	}
	for _, want := range []string{
		"-type f -readable",
		"-path '/var/log/*'",
		"-path '/var/adm/*'",
		"-path '*/log/*'",
		"-path '*/logs/*'",
		"-name '*.log'",
		"-name '*.log.*'",
		"-name '*.out'",
		"-name '*.err'",
		"-name 'messages*'",
		"-name 'syslog*'",
		"-name 'secure*'",
		"-name 'auth.log*'",
		"-name 'cron*'",
		"-name 'maillog*'",
		"-name 'debug*'",
		"-name 'warn*'",
		"-name '*audit*'",
		"-name 'btmp'",
		"-name 'wtmp'",
		"-name 'lastlog'",
		"-name 'tallylog'",
		"-name 'faillog'",
		"-name '*.journal'",
		"-path '*/journal/*'",
		"-name '*.pcap*'",
		"-name '*.db'",
		"-name '*.sqlite*'",
		"-name '*.zip'",
		"-name '*.xz'",
		"-name '*.bz2'",
		"-name '*.zst'",
		"-name '*.7z'",
		"-name '*.dat'",
		"-name 'hardware-summary'",
		"-name 'status'",
		// 默认不包含轮转日志。
		"-name '*.gz'",
		"-name '*.[0-9]'",
		"-name '*.[0-9][0-9]'",
		`-printf '%T@\t%s\t%p\0'`,
	} {
		if !strings.Contains(cmd, want) {
			t.Fatalf("command missing %q:\n%s", want, cmd)
		}
	}
	if strings.Contains(cmd, "-prune") || strings.Contains(cmd, "'/proc'") {
		t.Fatalf("default command should not prune full-scan paths: %s", cmd)
	}
	if strings.Contains(cmd, "xargs") || strings.Contains(cmd, "grep") {
		t.Fatalf("scan command should not run grep: %s", cmd)
	}
	if !strings.HasSuffix(cmd, `-printf '%T@\t%s\t%p\0'`) {
		t.Fatalf("scan command should end with -printf: %s", cmd)
	}
}

func TestBuildSSHLogFindCommandFullScan(t *testing.T) {
	cmd, err := buildSSHLogFindCommand(SSHLogSearchOptions{
		Roots:          []string{"/tmp", "/home"},
		FullScan:       true,
		IncludeRotated: true,
	})
	if err != nil {
		t.Fatalf("build command: %v", err)
	}
	if !strings.HasPrefix(cmd, "find '/' -xdev ") {
		t.Fatalf("full scan should ignore roots and scan /: %s", cmd)
	}
	for _, want := range []string{
		"\\( -path '/proc' -o -path '/sys' -o -path '/dev' -o -path '/run' -o -path '/snap' -o -path '/boot'",
		"-path '/var/lib/docker'",
		"-path '/var/lib/containers'",
		"-path '/var/lib/kubelet'",
		"-path '/var/lib/systemd'",
		"-path '/var/cache'",
		"-path '/usr/share'",
		"-path '/usr/lib'",
		"\\) -prune -o -type f -readable",
	} {
		if !strings.Contains(cmd, want) {
			t.Fatalf("full scan command missing %q:\n%s", want, cmd)
		}
	}
	if strings.Contains(cmd, "/tmp") || strings.Contains(cmd, "/home") {
		t.Fatalf("full scan should ignore user roots: %s", cmd)
	}
	if strings.Contains(cmd, "-name '*.gz'") {
		t.Fatalf("IncludeRotated should not exclude *.gz: %s", cmd)
	}
	if !strings.Contains(cmd, "-name '*.xz'") {
		t.Fatalf("base archive exclusions should remain: %s", cmd)
	}
}

func TestBuildSSHLogFindCommandRoots(t *testing.T) {
	cmd, err := buildSSHLogFindCommand(SSHLogSearchOptions{
		Roots: []string{"/var/log", " /var/log/ ", "/var/adm", "/var/log/."},
	})
	if err != nil {
		t.Fatalf("build command: %v", err)
	}
	if !strings.HasPrefix(cmd, "find '/var/log' '/var/adm' -xdev ") {
		t.Fatalf("roots should be trimmed and deduplicated: %s", cmd)
	}
	if strings.Count(cmd, "'/var/log' ") != 1 {
		t.Fatalf("expected a single /var/log root: %s", cmd)
	}

	if _, err := buildSSHLogFindCommand(SSHLogSearchOptions{Roots: []string{"relative/log"}}); err == nil {
		t.Fatal("expected error for relative root")
	} else if !errors.Is(err, errs.ErrInvalidInput) {
		t.Fatalf("expected invalid input error, got %v", err)
	}
	if _, err := buildSSHLogFindCommand(SSHLogSearchOptions{Roots: []string{"/var/log\nbad"}}); err == nil {
		t.Fatal("expected error for control character in root")
	}
}

func TestBuildSSHLogFindCommandContentKeyword(t *testing.T) {
	cmd, err := buildSSHLogFindCommand(SSHLogSearchOptions{
		Keyword:       "a'b c",
		SearchContent: true,
	})
	if err != nil {
		t.Fatalf("build command: %v", err)
	}
	const suffix = `-print0 | xargs -0 -r grep -I -l -F -Z -e 'a'\''b c' 2>/dev/null`
	if !strings.HasSuffix(cmd, suffix) {
		t.Fatalf("content command should end with grep pipeline:\nwant suffix: %s\ngot: %s", suffix, cmd)
	}
	if strings.Contains(cmd, "-printf") {
		t.Fatalf("content command should not use -printf: %s", cmd)
	}

	cmd, err = buildSSHLogFindCommand(SSHLogSearchOptions{
		Keyword:       "-n",
		SearchContent: true,
	})
	if err != nil {
		t.Fatalf("build command for dash keyword: %v", err)
	}
	if !strings.Contains(cmd, "-e '-n'") {
		t.Fatalf("dash keyword should be quoted after -e: %s", cmd)
	}
}

func TestBuildSSHLogFindCommandRotatedToggle(t *testing.T) {
	without, err := buildSSHLogFindCommand(SSHLogSearchOptions{IncludeRotated: false})
	if err != nil {
		t.Fatalf("build command: %v", err)
	}
	for _, want := range []string{"-name '*.gz'", "-name '*.[0-9]'", "-name '*.[0-9][0-9]'", "-name '*.old'", "-name '*.bak'"} {
		if !strings.Contains(without, want) {
			t.Fatalf("rotated exclusion missing %q", want)
		}
	}

	with, err := buildSSHLogFindCommand(SSHLogSearchOptions{IncludeRotated: true})
	if err != nil {
		t.Fatalf("build command: %v", err)
	}
	for _, unwanted := range []string{"-name '*.gz'", "-name '*.[0-9]'", "-name '*.old'"} {
		if strings.Contains(with, unwanted) {
			t.Fatalf("IncludeRotated command should not contain %q: %s", unwanted, with)
		}
	}
	if !strings.Contains(with, "-name '*.zip'") {
		t.Fatalf("base exclusions should remain: %s", with)
	}
}

func TestBuildSSHLogFindCommandPathKeyword(t *testing.T) {
	cmd, err := buildSSHLogFindCommand(SSHLogSearchOptions{Keyword: "needle"})
	if err != nil {
		t.Fatalf("build command: %v", err)
	}
	if !strings.Contains(cmd, "-path '*needle*'") {
		t.Fatalf("path keyword filter missing: %s", cmd)
	}
	if strings.Contains(cmd, "xargs") {
		t.Fatalf("path keyword search should not run grep: %s", cmd)
	}

	cmd, err = buildSSHLogFindCommand(SSHLogSearchOptions{Keyword: "a*b?[c]"})
	if err != nil {
		t.Fatalf("build command: %v", err)
	}
	if !strings.Contains(cmd, "-path '*a[*]b[?][[]c]*'") {
		t.Fatalf("glob metacharacters should be escaped: %s", cmd)
	}

	empty, err := buildSSHLogFindCommand(SSHLogSearchOptions{})
	if err != nil {
		t.Fatalf("build command: %v", err)
	}
	// 默认条件里只有 5 个静态 -path（/var/log、/var/adm、*/log/*、*/logs/*、*/journal/*），
	// 空关键字不应额外追加路径过滤。
	if got := strings.Count(empty, "-path '"); got != 5 {
		t.Fatalf("empty keyword should not add a path filter, got %d -path predicates: %s", got, empty)
	}
}

func TestBuildSSHLogFindCommandKeywordValidation(t *testing.T) {
	if _, err := buildSSHLogFindCommand(SSHLogSearchOptions{SearchContent: true}); err == nil {
		t.Fatal("expected error for empty content keyword")
	} else if !errors.Is(err, errs.ErrInvalidInput) {
		t.Fatalf("expected invalid input error, got %v", err)
	}
	if _, err := buildSSHLogFindCommand(SSHLogSearchOptions{
		SearchContent: true,
		Keyword:       strings.Repeat("a", maxOptionValueLength+1),
	}); err == nil {
		t.Fatal("expected error for oversized keyword")
	}
	if _, err := buildSSHLogFindCommand(SSHLogSearchOptions{Keyword: "bad\nkeyword"}); err == nil {
		t.Fatal("expected error for control character in keyword")
	}
}

func TestParseSSHLogFindRecord(t *testing.T) {
	file, ok := parseSSHLogFindRecord("1700000000.123456789\t42\t/var/log/app.log")
	if !ok {
		t.Fatal("expected record to parse")
	}
	wantTime := time.Unix(1700000000, 123456789).UTC()
	if !file.ModTime.Equal(wantTime) {
		t.Fatalf("unexpected mod time: %v (want %v)", file.ModTime, wantTime)
	}
	if file.Size != 42 || file.Path != "/var/log/app.log" {
		t.Fatalf("unexpected file: %+v", file)
	}

	// 路径中的空格与制表符必须保留。
	file, ok = parseSSHLogFindRecord("1700000000\t7\t/var/log/my app/a b.log")
	if !ok || file.Path != "/var/log/my app/a b.log" {
		t.Fatalf("space in path not preserved: %+v ok=%v", file, ok)
	}
	file, ok = parseSSHLogFindRecord("1700000000.5\t7\t/var/log/a\tb.log")
	if !ok || file.Path != "/var/log/a\tb.log" {
		t.Fatalf("tab in path not preserved: %+v ok=%v", file, ok)
	}

	// 无小数部分的时间戳。
	file, ok = parseSSHLogFindRecord("1700000001\t1\t/var/log/x.log")
	if !ok || !file.ModTime.Equal(time.Unix(1700000001, 0).UTC()) {
		t.Fatalf("integer mod time parse failed: %+v ok=%v", file, ok)
	}

	for _, bad := range []string{
		"",
		"1700000000\t42",
		"not-a-time\t42\t/var/log/a.log",
		"1700000000\tNaN\t/var/log/a.log",
		"1700000000\t-1\t/var/log/a.log",
		"1700000000\t42\t",
	} {
		if _, ok := parseSSHLogFindRecord(bad); ok {
			t.Fatalf("record %q should be rejected", bad)
		}
	}
}

func TestReadSSHLogScanResults(t *testing.T) {
	input := strings.Join([]string{
		"1700000001.5\t10\t/var/log/a.log",
		"",
		"malformed-record",
		"1700000002.25\t20\t/var/log/dir with space/b.log",
		"1700000003.0\t30\t/var/log/tab\tname.log",
		"1700x\t30\t/var/log/bad.log",
	}, "\x00") + "\x00"

	files, truncated, err := readSSHLogScanResults(strings.NewReader(input), 10)
	if err != nil {
		t.Fatalf("read scan results: %v", err)
	}
	if truncated {
		t.Fatal("did not expect truncation")
	}
	if len(files) != 3 {
		t.Fatalf("expected 3 valid records, got %d: %+v", len(files), files)
	}
	if files[1].Path != "/var/log/dir with space/b.log" {
		t.Fatalf("space path mismatch: %+v", files[1])
	}
	if files[2].Path != "/var/log/tab\tname.log" || files[2].Size != 30 {
		t.Fatalf("tab path mismatch: %+v", files[2])
	}
}

func TestReadSSHLogScanResultsLimit(t *testing.T) {
	input := strings.Join([]string{
		"1700000001\t1\t/var/log/a.log",
		"1700000002\t2\t/var/log/b.log",
		"1700000003\t3\t/var/log/c.log",
	}, "\x00") + "\x00"

	files, truncated, err := readSSHLogScanResults(strings.NewReader(input), 2)
	if !errors.Is(err, errSSHLogSearchLimit) {
		t.Fatalf("expected limit sentinel, got %v", err)
	}
	if !truncated || len(files) != 2 {
		t.Fatalf("expected 2 truncated results, got %d truncated=%v", len(files), truncated)
	}

	// 末尾缺少 NUL 的残片不应被当作完整记录。
	files, truncated, err = readSSHLogScanResults(strings.NewReader("1700000001\t1\t/var/log/incomplete.log"), 10)
	if err != nil || truncated || len(files) != 0 {
		t.Fatalf("partial trailing record should be dropped: files=%d truncated=%v err=%v", len(files), truncated, err)
	}
}

func TestReadSSHLogContentPaths(t *testing.T) {
	input := "/var/log/a.log\x00/var/log/b b.log\x00"
	paths, truncated, err := readSSHLogContentPaths(strings.NewReader(input), 10)
	if err != nil || truncated {
		t.Fatalf("read content paths: paths=%v truncated=%v err=%v", paths, truncated, err)
	}
	if len(paths) != 2 || paths[1] != "/var/log/b b.log" {
		t.Fatalf("unexpected paths: %v", paths)
	}

	paths, truncated, err = readSSHLogContentPaths(strings.NewReader(input), 1)
	if !errors.Is(err, errSSHLogSearchLimit) || !truncated || len(paths) != 1 {
		t.Fatalf("expected truncated single path: paths=%v truncated=%v err=%v", paths, truncated, err)
	}

	paths, truncated, err = readSSHLogContentPaths(strings.NewReader(""), 10)
	if err != nil || truncated || len(paths) != 0 {
		t.Fatalf("empty content output should be empty: %v %v %v", paths, truncated, err)
	}
}

func TestNormalizeSSHLogSearchMaxResults(t *testing.T) {
	cases := []struct{ in, want int }{
		{0, 200},
		{-5, 200},
		{1, 1},
		{500, 500},
		{5000, 2000},
	}
	for _, c := range cases {
		if got := normalizeSSHLogSearchMaxResults(c.in); got != c.want {
			t.Fatalf("normalize(%d)=%d, want %d", c.in, got, c.want)
		}
	}
}

func TestSortSSHLogFiles(t *testing.T) {
	base := time.Unix(1700000000, 0).UTC()
	files := []models.SSHRemoteLogFile{
		{Path: "/var/log/b.log", ModTime: base.Add(2 * time.Second)},
		{Path: "/var/log/a.log", ModTime: base.Add(2 * time.Second)},
		{Path: "/var/log/old.log", ModTime: base},
		{Path: "/var/log/newest.log", ModTime: base.Add(3 * time.Second)},
	}
	sortSSHLogFiles(files)

	want := []string{"/var/log/newest.log", "/var/log/a.log", "/var/log/b.log", "/var/log/old.log"}
	for i, path := range want {
		if files[i].Path != path {
			t.Fatalf("sorted[%d]=%s, want %s (all: %+v)", i, files[i].Path, path, files)
		}
	}
}
