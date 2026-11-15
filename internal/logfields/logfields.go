// Package logfields 提供日志派生字段（事件类型、来源 IP、目的 IP）的统一提取逻辑。
//
// 索引构建（internal/indexer）与查询引擎（internal/query）共用本包，
// 确保写入 bitmap 的值与查询/聚合时解释的值完全一致，避免两套别名规则漂移。
package logfields

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"glog/pkg/models"
)

// maxLookupDepth 限制嵌套字段查找深度，避免异常的自引用 map 造成无限递归。
const maxLookupDepth = 8

// 派生字段别名，按优先级排列。source_ip 的别名刻意不包含任何目的地前缀，
// 避免来源/目的 IP 互相误取；通用键（ip、ipaddress）放在最后，优先级最低。
var (
	SourceIPAliases = []string{
		"source_ip", "src_ip", "sourceip", "srcip", "sourceaddress", "sourcenetworkaddress",
		"srcaddress", "srcaddr", "ipsource",
		"remote_ip", "remote_addr", "remoteaddress", "client_ip", "client_addr",
		"clientaddress", "ipaddress", "ip",
	}
	DestinationIPAliases = []string{
		"destination_ip", "dest_ip", "dst_ip", "destinationaddress", "destaddress",
		"dstaddress", "dstaddr", "destaddr", "destinationnetworkaddress",
		"targetip", "targetaddress", "targetaddr", "serverip", "serveraddress",
	}
	// EventTypeAliases 后两项为 Windows EVTX 常见的事件通道字段（Channel），
	// 让 event_type 优先展示 Security/System/Application 等通道，而不是 Provider 全名。
	EventTypeAliases = []string{"event_type", "eventtype", "channel"}
)

// NormalizeKey 将字段键归一化为小写字母数字，便于匹配
// SourceIp / source_ip / Source IP 等不同命名风格。
func NormalizeKey(key string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, key)
}

// ValueString 把字段值转换为稳定的字符串表示。
func ValueString(v interface{}) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case []byte:
		return string(x)
	case time.Time:
		return x.Format(time.RFC3339Nano)
	case json.Number:
		return x.String()
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(x), 'f', -1, 32)
	case int:
		return strconv.FormatInt(int64(x), 10)
	case int8:
		return strconv.FormatInt(int64(x), 10)
	case int16:
		return strconv.FormatInt(int64(x), 10)
	case int32:
		return strconv.FormatInt(int64(x), 10)
	case int64:
		return strconv.FormatInt(x, 10)
	case uint:
		return strconv.FormatUint(uint64(x), 10)
	case uint8:
		return strconv.FormatUint(uint64(x), 10)
	case uint16:
		return strconv.FormatUint(uint64(x), 10)
	case uint32:
		return strconv.FormatUint(uint64(x), 10)
	case uint64:
		return strconv.FormatUint(x, 10)
	case bool:
		return strconv.FormatBool(x)
	case []string:
		return strings.Join(x, ",")
	case []interface{}:
		parts := make([]string, 0, len(x))
		for _, item := range x {
			if item == nil {
				continue
			}
			parts = append(parts, ValueString(item))
		}
		return strings.Join(parts, ",")
	default:
		return fmt.Sprint(v)
	}
}

// InterfaceMap 把常见的 map 表示统一为 map[string]interface{}。
func InterfaceMap(v interface{}) (map[string]interface{}, bool) {
	switch m := v.(type) {
	case map[string]interface{}:
		return m, true
	case map[string]string:
		out := make(map[string]interface{}, len(m))
		for k, vv := range m {
			out[k] = vv
		}
		return out, true
	}
	return nil, false
}

// LookupAlias 在 fields 中查找指定别名：先匹配顶层键，再递归嵌套 map。
func LookupAlias(fields map[string]interface{}, alias string) (interface{}, bool) {
	target := NormalizeKey(alias)
	if target == "" {
		return nil, false
	}
	return findNormalizedValue(fields, target, 0)
}

// findNormalizedValue 按归一化键查找字段值（顶层优先，随后递归嵌套 map）。
func findNormalizedValue(fields map[string]interface{}, target string, depth int) (interface{}, bool) {
	if fields == nil || target == "" || depth > maxLookupDepth {
		return nil, false
	}
	for key, value := range fields {
		if NormalizeKey(key) == target {
			return value, true
		}
	}
	for _, value := range fields {
		nested, ok := InterfaceMap(value)
		if !ok {
			continue
		}
		if found, ok := findNormalizedValue(nested, target, depth+1); ok {
			return found, true
		}
	}
	return nil, false
}

// ExtractIP 按别名优先级在动态字段中查找 IP 值；空值与占位符 "-" 视为缺失。
// 返回值已去除首尾空白。
func ExtractIP(fields map[string]interface{}, aliases []string) (string, bool) {
	for _, alias := range aliases {
		value, ok := LookupAlias(fields, alias)
		if !ok {
			continue
		}
		text := strings.TrimSpace(ValueString(value))
		if text == "" || text == "-" {
			continue
		}
		return text, true
	}
	return "", false
}

// EventType 依次尝试动态字段 event_type/eventType/Channel、Provider、
// 源文件名与来源，返回非空的事件类型。
func EventType(entry *models.LogEntry) (string, bool) {
	if entry == nil {
		return "", false
	}
	for _, alias := range EventTypeAliases {
		value, ok := LookupAlias(entry.Fields, alias)
		if !ok {
			continue
		}
		if text := strings.TrimSpace(ValueString(value)); text != "" {
			return text, true
		}
	}
	if entry.Provider != nil {
		if text := strings.TrimSpace(*entry.Provider); text != "" {
			return text, true
		}
	}
	if path := strings.TrimSpace(entry.SourceFile); path != "" {
		return filepath.Base(path), true
	}
	if text := strings.TrimSpace(entry.Source); text != "" {
		return text, true
	}
	return "", false
}
