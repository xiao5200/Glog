package collector

import (
	"fmt"
	"strings"
)

const (
	maxRemotePathLength  = 4096
	maxOptionValueLength = 4096
)

// shellQuote 把参数安全地包进单引号，供远端 POSIX shell 使用。
// 单引号内部通过 '\” 结束/转义/重新开启单引号。
// 拒绝 NUL、换行及所有 C0 控制字符，避免命令分隔与日志注入。
func shellQuote(arg string) (string, error) {
	if strings.ContainsRune(arg, 0) {
		return "", fmt.Errorf("参数包含 NUL 字符")
	}
	for _, r := range arg {
		if r < 0x20 || r == 0x7f {
			return "", fmt.Errorf("参数包含控制字符")
		}
	}
	escaped := strings.ReplaceAll(arg, "'", `'\''`)
	return "'" + escaped + "'", nil
}

// validateRemotePath 校验用户提供的远端路径。不做任何 shell 拼接前检查之外的
// 路径归一化，避免改变用户语义；控制字符、空路径与超长路径直接拒绝。
func validateRemotePath(remotePath string) error {
	if strings.TrimSpace(remotePath) == "" {
		return fmt.Errorf("远程路径不能为空")
	}
	if len(remotePath) > maxRemotePathLength {
		return fmt.Errorf("远程路径过长（超过 %d 字节）", maxRemotePathLength)
	}
	if strings.ContainsRune(remotePath, 0) {
		return fmt.Errorf("远程路径包含 NUL 字符")
	}
	for _, r := range remotePath {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("远程路径包含控制字符")
		}
	}
	return nil
}

// validateOptionValue 校验 journalctl / tar 等命令的选项值。
func validateOptionValue(value string) error {
	if len(value) > maxOptionValueLength {
		return fmt.Errorf("参数过长（超过 %d 字节）", maxOptionValueLength)
	}
	if strings.ContainsRune(value, 0) {
		return fmt.Errorf("参数包含 NUL 字符")
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("参数包含控制字符")
		}
	}
	return nil
}
