package app

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"glog/configs"
	"glog/internal/collector"
	"glog/pkg/models"
)

type scanRoot struct {
	path     string
	depth    int
	category string
}

type detectedCandidate struct {
	path string
}

// DetectLocalLogs 扫描当前主机常见日志路径，并给出解析模板建议。
func (a *App) DetectLocalLogs() (*models.LocalLogDiscovery, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	return a.discoverLocalLogs(ctx), nil
}

func (a *App) discoverLocalLogs(ctx context.Context) *models.LocalLogDiscovery {
	result := &models.LocalLogDiscovery{
		ScannedAt: time.Now().UTC(),
		Items:     []models.DetectedLog{},
		Errors:    []string{},
	}

	if runtime.GOOS == "windows" {
		result.Items = append(result.Items,
			models.DetectedLog{
				ID:         detectionID("eventlog", "System"),
				Name:       "Windows 系统事件日志",
				Path:       "eventlog://System",
				Type:       "eventlog",
				Channel:    "System",
				Category:   "windows",
				Readable:   true,
				TemplateID: "windows-evtx",
				Format:     "json",
				Preview:    "系统事件通道；普通用户通过 wevtutil 自动导出后解析。",
			},
			models.DetectedLog{
				ID:         detectionID("eventlog", "Application"),
				Name:       "Windows 应用程序事件日志",
				Path:       "eventlog://Application",
				Type:       "eventlog",
				Channel:    "Application",
				Category:   "windows",
				Readable:   true,
				TemplateID: "windows-evtx",
				Format:     "json",
				Preview:    "应用程序事件通道；普通用户通过 wevtutil 自动导出后解析。",
			},
			models.DetectedLog{
				ID:         detectionID("eventlog", "Security"),
				Name:       "Windows 安全事件日志",
				Path:       "eventlog://Security",
				Type:       "eventlog",
				Channel:    "Security",
				Category:   "windows",
				Readable:   true,
				TemplateID: "windows-evtx",
				Format:     "json",
				Preview:    "安全事件通道；通常需要管理员权限。",
			},
		)
	}
	if runtime.GOOS == "linux" {
		result.Items = append(result.Items, models.DetectedLog{
			ID:         detectionID("journal", "systemd"),
			Name:       "systemd journal",
			Path:       "journal://systemd",
			Type:       "journal",
			Channel:    "journal",
			Category:   "system",
			Readable:   true,
			TemplateID: "journald-json",
			Format:     "json",
			Preview:    "systemd journal；通过 journalctl -o json 采集。",
		})
	}

	roots, globs := buildDiscoveryTargets()
	result.Roots = make([]string, 0, len(roots))
	for _, root := range roots {
		result.Roots = append(result.Roots, root.path)
	}

	candidates := make(map[string]struct{})
	limitReached := false
	contextExpired := false

	addCandidate := func(path string) {
		if limitReached {
			return
		}
		path = filepath.Clean(path)
		key := path
		if runtime.GOOS == "windows" {
			key = strings.ToLower(path)
		}
		if _, ok := candidates[key]; ok {
			return
		}
		candidates[key] = struct{}{}
		if len(candidates) >= 400 {
			limitReached = true
		}
	}

	// 先处理显式 glob（CBS、setupapi 等已知高价值文件），避免被应用日志目录的
	// 候选数量上限挤掉；目录遍历作为兜底。
	for _, pattern := range globs {
		if contextExpired || limitReached {
			break
		}
		matchPattern := pattern
		for _, match := range expandGlobSafe(matchPattern) {
			info, err := os.Stat(match)
			if err == nil && !info.IsDir() && isLogCandidate(match) {
				addCandidate(match)
			}
		}
	}

	for _, root := range roots {
		if contextExpired || limitReached {
			break
		}
		info, err := os.Stat(root.path)
		if err != nil || !info.IsDir() {
			continue
		}
		rootCopy := root
		err = filepath.WalkDir(root.path, func(path string, entry fs.DirEntry, err error) error {
			if ctx.Err() != nil {
				contextExpired = true
				return ctx.Err()
			}
			if limitReached {
				return filepath.SkipAll
			}
			if err != nil {
				if len(result.Errors) < 8 {
					result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", path, err))
				}
				if entry != nil && entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if entry == nil {
				return nil
			}
			rel, relErr := filepath.Rel(rootCopy.path, path)
			depth := 0
			if relErr == nil && rel != "." {
				depth = strings.Count(rel, string(filepath.Separator))
			}
			if entry.IsDir() {
				name := strings.ToLower(entry.Name())
				if shouldSkipDir(name) || depth >= rootCopy.depth {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.Type()&fs.ModeSymlink != 0 {
				return nil
			}
			if depth > rootCopy.depth {
				return nil
			}
			if isLogCandidate(entry.Name()) {
				addCandidate(path)
			}
			return nil
		})
		if err != nil && ctx.Err() == nil && len(result.Errors) < 8 {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", root.path, err))
		}
	}

	if contextExpired {
		result.Errors = append(result.Errors, "扫描时间超过上限，结果可能不完整")
	}

	type scoredCandidate struct {
		path  string
		score int
	}
	scored := make([]scoredCandidate, 0, len(candidates))
	for path := range candidates {
		scored = append(scored, scoredCandidate{path: path, score: candidateScore(path)})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return strings.ToLower(scored[i].path) < strings.ToLower(scored[j].path)
	})

	// 优先处理高价值日志；同一目录最多保留 6 个文件，避免应用日志刷屏。
	selected := make([]string, 0, 200)
	perDir := make(map[string]int)
	const detectLimit = 220
	for _, candidate := range scored {
		dir := strings.ToLower(filepath.Dir(candidate.path))
		if perDir[dir] >= 6 {
			continue
		}
		perDir[dir]++
		selected = append(selected, candidate.path)
		if len(selected) >= detectLimit {
			result.Truncated = true
			break
		}
	}

	for _, path := range selected {
		item, ok := a.describeLocalLog(path)
		if !ok {
			continue
		}
		result.Items = append(result.Items, item)
	}
	if limitReached {
		result.Truncated = true
	}

	sortDetectedLogs(result.Items)
	const maxItems = 120
	if len(result.Items) > maxItems {
		result.Items = result.Items[:maxItems]
		result.Truncated = true
	}
	if result.Items == nil {
		result.Items = []models.DetectedLog{}
	}
	if result.Errors == nil {
		result.Errors = []string{}
	}
	return result
}

func (a *App) describeLocalLog(path string) (models.DetectedLog, bool) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() == 0 {
		return models.DetectedLog{}, false
	}
	item := models.DetectedLog{
		ID:        detectionID("file", path),
		Name:      filepath.Base(path),
		Path:      path,
		Type:      "file",
		Category:  detectLogCategory(path),
		SizeBytes: info.Size(),
		ModTime:   info.ModTime().UTC(),
		Readable:  isReadableFile(path),
	}
	if collector.IsEVTX(path) {
		item.Type = "evtx"
		item.TemplateID = "windows-evtx"
		item.Format = "evtx"
		item.Preview = "Windows EVTX 文件；使用原生 EVTX 解析。"
		return item, true
	}
	if !item.Readable {
		return item, true
	}
	samples, previewErr := collector.ReadFilePreview(path, 5)
	if previewErr != nil || len(samples) == 0 {
		if previewErr != nil {
			item.Preview = "无法读取预览：" + previewErr.Error()
		}
		return item, true
	}
	item.Preview = strings.TrimSpace(samples[0])
	if len(item.Preview) > 220 {
		item.Preview = item.Preview[:220] + "…"
	}
	if a.parserEngine != nil {
		if tpl, confidence := a.parserEngine.DetectTemplate(samples); tpl != nil && confidence > 0.2 {
			item.TemplateID = tpl.ID
			item.Format = tpl.Format
		}
	}
	return item, true
}

func buildDiscoveryTargets() ([]scanRoot, []string) {
	home, _ := os.UserHomeDir()
	var roots []scanRoot
	var globs []string

	if runtime.GOOS == "windows" {
		systemRoot := os.Getenv("SystemRoot")
		if systemRoot == "" {
			systemRoot = `C:\Windows`
		}
		roots = append(roots,
			scanRoot{filepath.Join(systemRoot, "Logs"), 3, "system"},
			scanRoot{`C:\inetpub\logs`, 4, "web_server"},
			scanRoot{`C:\Apache24\logs`, 2, "web_server"},
			scanRoot{`C:\nginx\logs`, 2, "web_server"},
			scanRoot{`C:\Redis\logs`, 2, "database"},
			scanRoot{os.Getenv("ProgramData"), 3, "application"},
			scanRoot{os.Getenv("APPDATA"), 3, "application"},
			scanRoot{os.Getenv("LOCALAPPDATA"), 3, "application"},
		)
		if home != "" {
			roots = append(roots, scanRoot{home, 2, "user"})
		}
		globs = append(globs,
			`C:\Program Files\*\logs\*.log`,
			`C:\Program Files (x86)\*\logs\*.log`,
			`C:\ProgramData\*\logs\*.log`,
			`C:\ProgramData\*\log\*.log`,
			`C:\ProgramData\*\Logs\*.log`,
			// 常见 Windows 组件日志，确保不会被大量应用日志挤掉。
			filepath.Join(systemRoot, "Logs", "CBS", "CBS.log"),
			filepath.Join(systemRoot, "Logs", "DISM", "dism.log"),
			filepath.Join(systemRoot, "INF", "setupapi.dev.log"),
			filepath.Join(systemRoot, "PFRO.log"),
		)
	} else {
		roots = append(roots,
			scanRoot{"/var/log", 3, "system"},
			scanRoot{"/var/log/audit", 2, "auth"},
			scanRoot{"/var/log/nginx", 2, "web_server"},
			scanRoot{"/var/log/apache2", 2, "web_server"},
			scanRoot{"/var/log/httpd", 2, "web_server"},
			scanRoot{"/var/log/mysql", 2, "database"},
			scanRoot{"/var/log/postgresql", 2, "database"},
			scanRoot{"/var/log/redis", 2, "database"},
		)
		if home != "" {
			roots = append(roots, scanRoot{home, 2, "user"})
		}
	}

	// 已知常见路径（系统/中间件预设）作为补充。
	for _, paths := range collector.GetCommonLogPaths(runtime.GOOS) {
		for _, path := range paths {
			if strings.ContainsAny(path, "*?[") {
				globs = append(globs, path)
			} else if info, err := os.Stat(path); err == nil && !info.IsDir() {
				roots = append(roots, scanRoot{filepath.Dir(path), 0, detectLogCategory(path)})
			}
		}
	}
	for _, preset := range aPresetPaths(runtime.GOOS) {
		if strings.ContainsAny(preset, "*?[") {
			globs = append(globs, preset)
		} else if info, err := os.Stat(preset); err == nil && !info.IsDir() {
			roots = append(roots, scanRoot{filepath.Dir(preset), 0, detectLogCategory(preset)})
		}
	}

	// 去掉空路径、重复路径。
	seenRoots := map[string]bool{}
	cleanRoots := roots[:0]
	for _, root := range roots {
		if root.path == "" {
			continue
		}
		key := filepath.Clean(root.path)
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if seenRoots[key] {
			continue
		}
		seenRoots[key] = true
		root.path = filepath.Clean(root.path)
		cleanRoots = append(cleanRoots, root)
	}
	seenGlobs := map[string]bool{}
	cleanGlobs := globs[:0]
	for _, pattern := range globs {
		if pattern == "" || seenGlobs[pattern] {
			continue
		}
		seenGlobs[pattern] = true
		cleanGlobs = append(cleanGlobs, pattern)
	}
	return cleanRoots, cleanGlobs
}

func aPresetPaths(goos string) []string {
	var paths []string
	// 从嵌入的中间件预设中提取对应平台路径，避免在发现逻辑中重复维护列表。
	for _, preset := range middlewarePresetValues() {
		if goos == "windows" {
			paths = append(paths, preset.WindowsPaths...)
		} else {
			paths = append(paths, preset.LinuxPaths...)
		}
	}
	return paths
}

func middlewarePresetValues() []models.MiddlewarePreset {
	data, err := configs.FS.ReadFile(configs.PresetsFile)
	if err != nil {
		return nil
	}
	var file struct {
		Presets []models.MiddlewarePreset `yaml:"presets"`
	}
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil
	}
	return file.Presets
}

func expandGlobSafe(pattern string) []string {
	if !strings.ContainsAny(pattern, "*?[") {
		return []string{pattern}
	}
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		out = append(out, match)
	}
	return out
}

func shouldSkipDir(name string) bool {
	switch strings.ToLower(name) {
	case "node_modules", ".git", ".svn", "cache", "code cache", "crashpad", "winsxs", "driverstore", "prefetch", "temp", "tmp":
		return true
	}
	return strings.HasPrefix(name, ".")
}

func isLogCandidate(name string) bool {
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".log"), strings.HasSuffix(lower, ".err"), strings.HasSuffix(lower, ".out"):
		return true
	case strings.HasSuffix(lower, ".log.gz"), strings.HasSuffix(lower, ".err.gz"), strings.HasSuffix(lower, ".out.gz"):
		return true
	case strings.HasSuffix(lower, ".evtx"):
		return true
	case strings.HasSuffix(lower, ".txt"):
		return strings.Contains(lower, "log") || strings.Contains(lower, "history") || strings.Contains(lower, "setupapi")
	}
	switch lower {
	case "syslog", "messages", "auth.log", "secure", "cron", "dmesg", "bash_history", "zsh_history":
		return true
	}
	return false
}

func candidateScore(path string) int {
	lower := strings.ToLower(path)
	score := 0
	switch detectLogCategory(path) {
	case "system":
		score += 100
	case "auth":
		score += 95
	case "web_server":
		score += 90
	case "database":
		score += 85
	case "windows":
		score += 80
	case "application":
		score += 40
	case "user":
		score += 30
	default:
		score += 10
	}
	if containsAny(lower, `windows\logs`, "/var/log", "inetpub", "nginx", "apache", "httpd", "mysql", "postgres", "redis") {
		score += 30
	}
	for _, name := range []string{
		"cbs.log", "dism.log", "setupapi.dev.log", "syslog", "messages", "auth.log", "secure",
		"access.log", "error.log", "nginx.log", "mysql.log", "slow.log", "glog.log",
	} {
		if strings.HasSuffix(lower, name) {
			score += 40
			break
		}
	}
	if strings.Contains(lower, "alibabaprotect") {
		score -= 20
	}
	return score
}

func detectLogCategory(path string) string {
	lower := strings.ToLower(path)
	switch {
	case containsAny(lower, "auth.log", "secure", "sshd", "sudo", "audit"):
		return "auth"
	case containsAny(lower, "nginx", "apache", "httpd", "inetpub", "w3svc", "iis"):
		return "web_server"
	case containsAny(lower, "mysql", "postgres", "redis", "mongo", "elastic", "slow.log"):
		return "database"
	case containsAny(lower, `\windows\logs`, "/var/log", "winevt", "system32"):
		return "system"
	case strings.Contains(lower, "glog"):
		return "application"
	default:
		return "application"
	}
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func detectionID(kind, value string) string {
	sum := sha1.Sum([]byte(kind + "|" + strings.ToLower(value)))
	return "log-" + hex.EncodeToString(sum[:6])
}

func sortDetectedLogs(items []models.DetectedLog) {
	priority := map[string]int{
		"auth": 1, "web_server": 2, "database": 3, "windows": 4, "system": 5, "application": 6, "user": 7, "other": 8,
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		// 事件日志 / journal 置顶，便于一键采集。
		typeRank := func(t string) int {
			switch t {
			case "eventlog", "journal":
				return 0
			case "evtx":
				return 1
			default:
				return 2
			}
		}
		if typeRank(a.Type) != typeRank(b.Type) {
			return typeRank(a.Type) < typeRank(b.Type)
		}
		if a.Readable != b.Readable {
			return a.Readable
		}
		pa, pb := priority[a.Category], priority[b.Category]
		if pa != pb {
			return pa < pb
		}
		return a.Path < b.Path
	})
}
