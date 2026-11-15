package parser

import (
	"math"
	"regexp"
	"strings"
	"time"

	"glog/pkg/models"
)

var accessCombinedRe = regexp.MustCompile(`^\S+ \S+ \S+ \[[^\]]+\] "\S+ \S+ \S+" \d{3} \S+ "[^"]*" "[^"]*"`)

var genericLevelRe = regexp.MustCompile(`(?i)(?:^|[\s\[\]()<>])(trace|debug|info|warn|warning|error|err|critical|fatal)(?:$|[\s\[\]()<>:])`)

// DetectFormat 统计样本中各类格式的命中率并返回置信度与建议模板。
// nginx 与 apache 的 combined 日志在文本上不可区分，因此统一返回
// access_combined，并同时建议两个模板。
func (e *Engine) DetectFormat(samples []string) models.FormatDetection {
	result := models.FormatDetection{
		DetectedFormat:     "unknown",
		Confidence:         0,
		SuggestedTemplates: []string{},
	}

	lines := nonEmptySamples(samples)
	if len(lines) == 0 {
		return result
	}

	jsonCount := 0
	dockerCount := 0
	evtxCount := 0
	for _, line := range lines {
		object, err := decodeJSONObject(strings.TrimRight(line, "\r\n"))
		if err != nil {
			continue
		}
		jsonCount++
		if jsonHasKey(object, "log") && (jsonHasKey(object, "stream") || jsonHasKey(object, "time")) {
			dockerCount++
		}
		if (jsonHasKey(object, "event_id") || jsonHasKey(object, "eventid")) &&
			(jsonHasKey(object, "provider") || jsonHasKey(object, "computer")) {
			evtxCount++
		}
	}

	syslog3164Count := 0
	syslog5424Count := 0
	accessCount := 0
	genericCount := 0
	for _, line := range lines {
		text := strings.TrimRight(line, "\r\n")
		if rfc5424Re.MatchString(text) {
			syslog5424Count++
		} else if rfc3164Re.MatchString(text) {
			syslog3164Count++
		}
		if accessCombinedRe.MatchString(text) {
			accessCount++
		}
		if genericLevelRe.MatchString(text) {
			genericCount++
		}
	}

	type formatScore struct {
		format string
		hits   int
	}
	scores := []formatScore{
		{format: "json", hits: jsonCount},
		{format: "syslog", hits: syslog3164Count + syslog5424Count},
		{format: "access_combined", hits: accessCount},
		{format: "generic", hits: genericCount},
	}
	best := scores[0]
	for _, score := range scores[1:] {
		if score.hits > best.hits {
			best = score
		}
	}
	added := make(map[string]bool)
	add := func(id string) {
		if id == "" || added[id] || !e.hasTemplate(id) {
			return
		}
		added[id] = true
		result.SuggestedTemplates = append(result.SuggestedTemplates, id)
	}

	if best.hits == 0 {
		// 不属于上述通用类别（如 IIS W3C、Redis）时，回退到模板匹配，
		// 避免内置模板明明可解析却返回 unknown。
		if tpl, rate := e.bestTemplate(lines); tpl != nil && rate >= 0.5 {
			result.DetectedFormat = formatRegex
			result.Confidence = rate
			add(tpl.ID)
		}
		return result
	}

	result.DetectedFormat = best.format
	result.Confidence = math.Min(1, float64(best.hits)/float64(len(lines)))

	switch best.format {
	case "json":
		if evtxCount > 0 {
			add("windows-evtx")
		}
		if dockerCount > 0 {
			add("docker-json")
		}
		add("json-generic")
	case "syslog":
		if syslog5424Count > 0 {
			add("syslog-rfc5424")
		}
		if syslog3164Count > 0 {
			add("syslog-rfc3164")
		}
		for _, line := range lines {
			if strings.Contains(line, "sshd[") {
				add("sshd")
				break
			}
		}
	case "access_combined":
		add("nginx-combined")
		add("apache-combined")
	case "generic":
		if tpl, rate := e.bestTemplate(lines); tpl != nil && rate >= 0.5 {
			add(tpl.ID)
		}
	}

	return result
}

// DetectTemplate 尝试所有内置模板，筛选成功率不低于 80% 的模板，按成功率
// 优先、Priority 次之选择最佳模板。没有模板达标时返回 (nil, 0)。
func (e *Engine) DetectTemplate(samples []string) (*models.ParseTemplate, float64) {
	if e == nil {
		return nil, 0
	}
	lines := nonEmptySamples(samples)
	if len(lines) == 0 {
		return nil, 0
	}
	best, rate := e.bestTemplate(lines)
	if best == nil || rate < 0.8 {
		return nil, 0
	}
	return cloneTemplatePtr(best), rate
}

// bestTemplate 返回内部模板指针与成功率。调用方不得修改返回的模板。
func (e *Engine) bestTemplate(samples []string) (*models.ParseTemplate, float64) {
	if e == nil {
		return nil, 0
	}
	lines := nonEmptySamples(samples)
	if len(lines) == 0 {
		return nil, 0
	}

	ctx := ParseContext{SourceID: "detect", CollectedAt: time.Now()}
	var best *models.ParseTemplate
	bestRate := -1.0
	for _, tpl := range e.snapshotTemplates() {
		success := 0
		for _, line := range lines {
			if _, err := e.Parse(line, tpl, ctx); err == nil {
				success++
			}
		}
		rate := float64(success) / float64(len(lines))
		if rate > bestRate+1e-9 ||
			(math.Abs(rate-bestRate) <= 1e-9 && (best == nil || tpl.Priority > best.Priority)) {
			best = tpl
			bestRate = rate
		}
	}
	if best == nil {
		return nil, 0
	}
	return best, bestRate
}

func nonEmptySamples(samples []string) []string {
	lines := make([]string, 0, len(samples))
	for _, sample := range samples {
		if strings.TrimSpace(sample) == "" {
			continue
		}
		lines = append(lines, sample)
	}
	return lines
}
