package query

import (
	"crypto/md5"
	"encoding/hex"
	"net"
	"regexp"
	"sort"
	"strings"
	"time"

	"glog/pkg/models"
)

const (
	maxPatternExamples = 10
	maxPatternParams   = 50
)

type clusterAccum struct {
	pattern  models.LogPattern
	paramSet map[string]map[string]bool
}

// Cluster 对查询匹配的日志做模板聚类。
// 最多读取 50,000 条以控制内存；超过部分截断，TotalCount 仍返回实际匹配总数，
// Percentage 以实际读取条数为分母。
func (e *Engine) Cluster(q models.Query) (res *models.PatternClusterResult, err error) {
	defer func() {
		if r := recover(); r != nil {
			res = nil
			err = internalPanic("Cluster", r)
		}
	}()
	if err = e.checkReady(); err != nil {
		return nil, err
	}

	started := time.Now()
	matched, matchErr := e.matchDocs(q)
	if matchErr != nil {
		return nil, matchErr
	}
	totalMatches := int64(matched.GetCardinality())
	if totalMatches == 0 {
		return &models.PatternClusterResult{
			Patterns:   []models.LogPattern{},
			TotalCount: 0,
			Took:       time.Since(started).Milliseconds(),
		}, nil
	}

	ids := matched.ToArray()
	if len(ids) > clusterMaxDocs {
		ids = ids[:clusterMaxDocs]
	}
	entries := e.readEntries(ids)
	if len(entries) == 0 {
		return &models.PatternClusterResult{
			Patterns:   []models.LogPattern{},
			TotalCount: totalMatches,
			Took:       time.Since(started).Milliseconds(),
		}, nil
	}

	accumulators := make(map[string]*clusterAccum)
	for i := range entries {
		entry := &entries[i]
		text := entry.Message
		if strings.TrimSpace(text) == "" {
			text = entry.RawLog
		}
		template, params := generalizeMessage(text)
		acc := accumulators[template]
		if acc == nil {
			acc = &clusterAccum{
				pattern: models.LogPattern{
					ID:         patternID(template),
					Template:   template,
					Examples:   make([]string, 0, maxPatternExamples),
					Parameters: make(map[string][]string),
				},
				paramSet: make(map[string]map[string]bool),
			}
			accumulators[template] = acc
		}

		acc.pattern.Count++
		if ts := entry.Timestamp; !ts.IsZero() {
			if acc.pattern.FirstSeen.IsZero() || ts.Before(acc.pattern.FirstSeen) {
				acc.pattern.FirstSeen = ts
			}
			if acc.pattern.LastSeen.IsZero() || ts.After(acc.pattern.LastSeen) {
				acc.pattern.LastSeen = ts
			}
		}
		if len(acc.pattern.Examples) < maxPatternExamples {
			acc.pattern.Examples = append(acc.pattern.Examples, text)
		}
		mergePatternParams(acc, params)
	}

	readCount := int64(len(entries))
	patterns := make([]models.LogPattern, 0, len(accumulators))
	for _, acc := range accumulators {
		if readCount > 0 {
			acc.pattern.Percentage = float64(acc.pattern.Count) / float64(readCount) * 100
		}
		patterns = append(patterns, acc.pattern)
	}
	sort.Slice(patterns, func(i, j int) bool {
		if patterns[i].Count == patterns[j].Count {
			return patterns[i].Template < patterns[j].Template
		}
		return patterns[i].Count > patterns[j].Count
	})

	return &models.PatternClusterResult{
		Patterns:   patterns,
		TotalCount: totalMatches,
		Took:       time.Since(started).Milliseconds(),
	}, nil
}

func patternID(template string) string {
	sum := md5.Sum([]byte(template))
	return hex.EncodeToString(sum[:])[:12]
}

func mergePatternParams(acc *clusterAccum, params map[string][]string) {
	for key, values := range params {
		if acc.paramSet[key] == nil {
			acc.paramSet[key] = make(map[string]bool)
		}
		for _, value := range values {
			if len(acc.pattern.Parameters[key]) >= maxPatternParams {
				break
			}
			if acc.paramSet[key][value] {
				continue
			}
			acc.paramSet[key][value] = true
			acc.pattern.Parameters[key] = append(acc.pattern.Parameters[key], value)
		}
	}
}

// ---------------------------------------------------------------------------
// 日志泛化
// ---------------------------------------------------------------------------

var (
	reURL = regexp.MustCompile(`(?i)\b(?:https?|ftp)://[^\s<>"']+`)

	reEmail = regexp.MustCompile(`(?i)\b[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}\b`)

	reUUID = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`)

	// IPv6 候选由 net.ParseIP 二次校验，避免误伤 12:30:45 之类时间。
	reIPv6Candidate = regexp.MustCompile(`[0-9A-Fa-f:]{2,39}`)

	reIPv4 = regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`)

	// Windows 盘符路径或至少两段 Unix 路径。
	rePath = regexp.MustCompile(`(?:[A-Za-z]:\\[^\s:*?"|]+|(?:/[\w.@%+\-<>]+){2,}/?)`)

	reDateTime = regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:[.,]\d+)?(?:Z|[+-]\d{2}:?\d{2})?\b`)

	reSyslogTime = regexp.MustCompile(`\b(?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)\s+\d{1,2}\s+\d{2}:\d{2}:\d{2}\b`)

	reDate = regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}\b`)

	reTimeOnly = regexp.MustCompile(`\b\d{2}:\d{2}:\d{2}(?:[.,]\d+)?\b`)

	reHex = regexp.MustCompile(`\b[0-9A-Fa-f]{8,}\b`)

	reNumber = regexp.MustCompile(`\b\d+(?:\.\d+)?\b`)
)

// generalizeMessage 按固定顺序泛化日志文本并提取参数。
// URL/IP/UUID 等先于数字与时间替换，避免参数被拆碎。
func generalizeMessage(text string) (string, map[string][]string) {
	params := make(map[string][]string)
	template := text

	template = replaceURLs(template, params)
	template = replaceCollect(template, reEmail, "<EMAIL>", "EMAIL", params, nil)
	template = replaceCollect(template, reUUID, "<UUID>", "UUID", params, nil)
	template = replaceCollect(template, reIPv6Candidate, "<IP>", "IP", params, func(s string) bool {
		ip := net.ParseIP(s)
		return ip != nil && strings.Contains(s, ":") && ip.To4() == nil
	})
	template = replaceCollect(template, reIPv4, "<IP>", "IP", params, func(s string) bool {
		ip := net.ParseIP(s)
		return ip != nil && ip.To4() != nil
	})
	template = replaceCollect(template, rePath, "<PATH>", "PATH", params, nil)
	template = replaceCollect(template, reDateTime, "<TIME>", "TIME", params, nil)
	template = replaceCollect(template, reSyslogTime, "<TIME>", "TIME", params, nil)
	template = replaceCollect(template, reDate, "<TIME>", "TIME", params, nil)
	template = replaceCollect(template, reTimeOnly, "<TIME>", "TIME", params, nil)
	template = replaceCollect(template, reHex, "<HEX>", "HEX", params, func(s string) bool {
		for _, c := range s {
			if (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
				return true
			}
		}
		return false
	})
	template = replaceCollect(template, reNumber, "<NUM>", "NUM", params, nil)

	return template, params
}

func replaceCollect(text string, re *regexp.Regexp, placeholder, key string, params map[string][]string, accept func(string) bool) string {
	return re.ReplaceAllStringFunc(text, func(match string) string {
		if match == "" {
			return match
		}
		if accept != nil && !accept(match) {
			return match
		}
		addPatternParam(params, key, match)
		return placeholder
	})
}

func replaceURLs(text string, params map[string][]string) string {
	return reURL.ReplaceAllStringFunc(text, func(match string) string {
		trimmed := strings.TrimRight(match, ".,;:!?'\")]}>")
		if trimmed == "" {
			return match
		}
		suffix := match[len(trimmed):]
		addPatternParam(params, "URL", trimmed)
		return "<URL>" + suffix
	})
}

func addPatternParam(params map[string][]string, key, value string) {
	if value == "" {
		return
	}
	list := params[key]
	for _, existing := range list {
		if existing == value {
			return
		}
	}
	if len(list) >= maxPatternParams {
		return
	}
	params[key] = append(list, value)
}
