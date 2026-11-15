package parser

import (
	"fmt"
	"time"

	"glog/pkg/models"
)

// Preview 逐条解析样本并返回解析结果与成功率。单条解析异常会被 recover，
// 不会中断整个预览。
func (e *Engine) Preview(tpl models.ParseTemplate, samples []string) models.PreviewResult {
	result := models.PreviewResult{
		Samples:     make([]models.ParsedSample, 0, len(samples)),
		SuccessRate: 0,
	}
	if len(samples) == 0 {
		return result
	}

	ctx := ParseContext{SourceID: "preview", CollectedAt: time.Now()}
	success := 0
	for _, sample := range samples {
		parsed := e.previewSample(&tpl, sample, ctx)
		if parsed.Success {
			success++
		}
		result.Samples = append(result.Samples, parsed)
	}
	result.SuccessRate = float64(success) / float64(len(samples))
	return result
}

func (e *Engine) previewSample(tpl *models.ParseTemplate, sample string, ctx ParseContext) (parsed models.ParsedSample) {
	parsed = models.ParsedSample{RawLog: sample}
	defer func() {
		if recovered := recover(); recovered != nil {
			parsed.Parsed = nil
			parsed.Success = false
			parsed.Error = fmt.Sprintf("解析发生异常: %v", recovered)
		}
	}()

	entry, err := e.Parse(sample, tpl, ctx)
	if err != nil {
		parsed.Error = err.Error()
		return parsed
	}
	parsed.Parsed = entry
	parsed.Success = true
	return parsed
}
