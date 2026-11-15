package threat

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"glog/configs"
)

// loadBuiltinRules 从嵌入资源加载内置 YAML 规则。以下划线开头的文件会被跳过。
func loadBuiltinRules() ([]*ruleDefinition, error) {
	entries, err := configs.FS.ReadDir(configs.RulesDir)
	if err != nil {
		return nil, fmt.Errorf("读取内置规则目录失败: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, "_") {
			continue
		}
		extension := strings.ToLower(path.Ext(name))
		if extension != ".yaml" && extension != ".yml" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	defs := make([]*ruleDefinition, 0, len(names))
	seen := make(map[string]string, len(names))
	for _, name := range names {
		data, err := configs.FS.ReadFile(path.Join(configs.RulesDir, name))
		if err != nil {
			return nil, fmt.Errorf("读取内置规则 %s 失败: %w", name, err)
		}
		def, err := parseRuleDefinition(data)
		if err != nil {
			return nil, fmt.Errorf("加载内置规则 %s 失败: %w", name, err)
		}
		if def.ID == "" {
			def.ID = builtinRuleID(name)
		}
		if def.ID == "" {
			return nil, fmt.Errorf("内置规则 %s 缺少稳定的规则ID", name)
		}
		if previous, ok := seen[def.ID]; ok {
			return nil, fmt.Errorf("内置规则ID重复: %s（%s 与 %s）", def.ID, previous, name)
		}
		seen[def.ID] = name
		def.sourcePath = configs.RulesDir + "/" + name
		def.sourceName = name
		def.builtin = true
		defs = append(defs, def)
	}
	return defs, nil
}

func builtinRuleID(filename string) string {
	base := strings.TrimSuffix(filename, path.Ext(filename))
	base = strings.ToLower(base)
	var b strings.Builder
	lastDash := false
	for _, r := range base {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_':
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return ""
	}
	return "builtin-" + out
}
