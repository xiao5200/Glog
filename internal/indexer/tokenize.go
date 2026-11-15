package indexer

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// 额外整段识别的模式。均在已经小写化的文本上匹配。
var (
	// IPv4：匹配后还会校验每个 octet <= 255。
	ipv4Re = regexp.MustCompile(`\b\d{1,3}(?:\.\d{1,3}){3}\b`)
	// 域名：标签 + 至少一个点 + 字母 TLD。宽松匹配，同时也能命中 auth.log
	// 这类带点标识符，作为额外 token 不影响普通分词结果。
	domainRe = regexp.MustCompile(`\b(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}\b`)
	// 文件路径：/ 或 \ 分隔的连续非空段，可带 Windows 盘符或前导分隔符。
	pathRe = regexp.MustCompile(`[\\/]?(?:[^\\/\s"'<>|?*,;()\[\]{}]+[\\/])+[^\\/\s"'<>|?*,;()\[\]{}]+`)
)

// Tokenize 是查询端与索引端共用的稳定分词器：
//
//  1. 文本转小写；
//  2. 按 Unicode 字母/数字/下划线切分为普通 token；
//  3. CJK（Han/Hiragana/Katakana/Hangul）连续段额外输出每个字符与相邻双字 bigram；
//  4. 额外整体识别 IPv4、含点域名、文件路径（/ 或 \ 分隔）作为 token；
//  5. 丢弃长度超过 64 个 rune 的 token；
//  6. 去重并按字节序排序。
//
// 示例："Failed password for root from 192.168.1.100" 会产生
// failed/password/for/root/from/192.168.1.100/192/168/1/100 等 token。
func Tokenize(text string) []string {
	if text == "" {
		return nil
	}
	lower := strings.ToLower(text)
	runes := []rune(lower)

	var tokens []string
	add := func(token string) {
		if token == "" {
			return
		}
		if utf8.RuneCountInString(token) > maxTokenRunes {
			return
		}
		tokens = append(tokens, token)
	}

	// 普通分词 + CJK 单字/双字。
	for i := 0; i < len(runes); {
		if !isTokenRune(runes[i]) {
			i++
			continue
		}
		j := i
		for j < len(runes) && isTokenRune(runes[j]) {
			j++
		}
		emitTokenRun(runes[i:j], add)
		i = j
	}

	for _, loc := range ipv4Re.FindAllStringIndex(lower, -1) {
		candidate := lower[loc[0]:loc[1]]
		if validIPv4(candidate) {
			add(candidate)
		}
	}
	for _, match := range domainRe.FindAllString(lower, -1) {
		add(match)
	}
	for _, match := range pathRe.FindAllString(lower, -1) {
		add(match)
	}

	if len(tokens) == 0 {
		return nil
	}
	sort.Strings(tokens)
	out := tokens[:0]
	for i, token := range tokens {
		if i == 0 || token != tokens[i-1] {
			out = append(out, token)
		}
	}
	return out
}

func isTokenRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

func isCJKRune(r rune) bool {
	return unicode.Is(unicode.Han, r) ||
		unicode.Is(unicode.Hiragana, r) ||
		unicode.Is(unicode.Katakana, r) ||
		unicode.Is(unicode.Hangul, r)
}

// emitTokenRun 处理一个由 token rune 组成的连续段：
// 非 CJK 段整体作为一个 token；CJK 段输出单字和相邻双字 bigram。
func emitTokenRun(runes []rune, add func(string)) {
	for i := 0; i < len(runes); {
		if isCJKRune(runes[i]) {
			j := i
			for j < len(runes) && isCJKRune(runes[j]) {
				j++
			}
			for k := i; k < j; k++ {
				add(string(runes[k]))
				if k+1 < j {
					add(string(runes[k : k+2]))
				}
			}
			i = j
			continue
		}
		j := i
		for j < len(runes) && !isCJKRune(runes[j]) {
			j++
		}
		add(string(runes[i:j]))
		i = j
	}
}

func validIPv4(candidate string) bool {
	parts := strings.Split(candidate, ".")
	if len(parts) != 4 {
		return false
	}
	for _, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 || n > 255 {
			return false
		}
	}
	return true
}
