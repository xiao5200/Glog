// Package configs 嵌入内置模板、规则与中间件预设，保证单可执行文件分发。
package configs

import "embed"

//go:embed templates/builtin/*.yaml rules/builtin/*.yaml middleware_presets.yaml
var FS embed.FS

const (
	TemplatesDir = "templates/builtin"
	RulesDir     = "rules/builtin"
	PresetsFile  = "middleware_presets.yaml"
)
