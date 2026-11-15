package collector

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"glog/internal/errs"
	"glog/pkg/models"
)

const (
	defaultSSHLogSearchRoot       = "/var/log"
	defaultSSHLogSearchMaxResults = 200
	maxSSHLogSearchMaxResults     = 2000
	maxSSHLogSearchRoots          = 64
	// maxNULRecordBytes 限制单条 NUL 分隔记录，防御异常/恶意远端输出耗尽内存。
	maxNULRecordBytes = 1 << 20
)

// errSSHLogSearchLimit 表示已收集到上限数量的结果，用于提前终止远端命令。
var errSSHLogSearchLimit = errors.New("SSH 日志检索结果已达上限")

// SSHLogSearchOptions 远程 Linux 日志查找选项。
type SSHLogSearchOptions struct {
	Roots          []string
	FullScan       bool
	Keyword        string
	SearchContent  bool
	IncludeRotated bool
	MaxResults     int
}

var sshLogFullScanPrunePaths = []string{
	"/proc", "/sys", "/dev", "/run", "/snap", "/boot",
	"/var/lib/docker", "/var/lib/containers", "/var/lib/kubelet",
	"/var/lib/systemd", "/var/cache", "/usr/share", "/usr/lib",
}

// sshLogIncludePredicates 只匹配日志类文件。
var sshLogIncludePredicates = []string{
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
	"-name 'boot*'",
	"-name '*audit*'",
}

// sshLogExcludePredicates 排除二进制、数据库与归档文件。
var sshLogExcludePredicates = []string{
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
	// Debian/Ubuntu installer 在 /var/log/installer 下存放的安装数据库与摘要，
	// 不是持续写入的日志，避免一键采集把它们当作日志解析。
	"-name '*.dat'",
	"-name 'hardware-summary'",
	"-name 'status'",
}

// sshLogRotatedExcludePredicates 在未要求包含轮转日志时追加的排除项。
var sshLogRotatedExcludePredicates = []string{
	"-name '*.gz'",
	"-name '*.[0-9]'",
	"-name '*.[0-9][0-9]'",
	"-name '*.old'",
	"-name '*.bak'",
}

// buildSSHLogFindCommand 生成只读的 GNU find 检索命令。
// 扫描模式输出 NUL 分隔的 mtime\tsize\tpath；内容检索模式把路径交给
// xargs + grep -l -Z，同样以 NUL 分隔输出。
func buildSSHLogFindCommand(search SSHLogSearchOptions) (string, error) {
	roots, err := resolveSSHLogSearchRoots(search)
	if err != nil {
		return "", err
	}
	if err := validateSSHLogSearchKeyword(search); err != nil {
		return "", err
	}

	parts := make([]string, 0, 24)
	parts = append(parts, "find")
	for _, root := range roots {
		quoted, quoteErr := shellQuote(root)
		if quoteErr != nil {
			return "", fmt.Errorf("%w: 检索目录无效 %q: %w", errs.ErrInvalidInput, root, quoteErr)
		}
		parts = append(parts, quoted)
	}
	// 始终不跨文件系统，避免全盘扫描挂载的网络盘/移动介质。
	parts = append(parts, "-xdev")

	if search.FullScan {
		prune := make([]string, 0, len(sshLogFullScanPrunePaths))
		for _, prunePath := range sshLogFullScanPrunePaths {
			quoted, quoteErr := shellQuote(prunePath)
			if quoteErr != nil {
				return "", fmt.Errorf("内置扫描排除路径无效 %q: %w", prunePath, quoteErr)
			}
			prune = append(prune, "-path "+quoted)
		}
		parts = append(parts, "\\( "+joinFindOr(prune)+" \\)", "-prune", "-o")
	}

	// -readable 是 GNU find 扩展：提前过滤当前 SSH 用户读不了的文件
	// （如非 root 下的 /var/log/secure、audit.log），避免前端选中后才失败。
	parts = append(parts,
		"-type", "f", "-readable",
		"\\( "+joinFindOr(sshLogIncludePredicates)+" \\)",
		"!", "\\( "+joinFindOr(sshLogExcludePredicatesFor(search))+" \\)",
	)

	if !search.SearchContent && search.Keyword != "" {
		quoted, quoteErr := shellQuote(findPathGlob(search.Keyword))
		if quoteErr != nil {
			return "", fmt.Errorf("%w: 检索关键字无效: %w", errs.ErrInvalidInput, quoteErr)
		}
		parts = append(parts, "-path", quoted)
	}

	if search.SearchContent {
		quoted, quoteErr := shellQuote(search.Keyword)
		if quoteErr != nil {
			return "", fmt.Errorf("%w: 检索关键字无效: %w", errs.ErrInvalidInput, quoteErr)
		}
		parts = append(parts,
			"-print0", "|", "xargs", "-0", "-r",
			"grep", "-I", "-l", "-F", "-Z", "-e", quoted, "2>/dev/null",
		)
	} else {
		parts = append(parts, `-printf '%T@\t%s\t%p\0'`)
	}
	return strings.Join(parts, " "), nil
}

func joinFindOr(predicates []string) string {
	return strings.Join(predicates, " -o ")
}

func sshLogExcludePredicatesFor(search SSHLogSearchOptions) []string {
	predicates := make([]string, 0, len(sshLogExcludePredicates)+len(sshLogRotatedExcludePredicates))
	predicates = append(predicates, sshLogExcludePredicates...)
	if !search.IncludeRotated {
		predicates = append(predicates, sshLogRotatedExcludePredicates...)
	}
	return predicates
}

// resolveSSHLogSearchRoots 去重并校验检索根目录；FullScan 时忽略用户目录固定扫描 /。
func resolveSSHLogSearchRoots(search SSHLogSearchOptions) ([]string, error) {
	if search.FullScan {
		return []string{"/"}, nil
	}

	roots := make([]string, 0, len(search.Roots))
	seen := make(map[string]struct{}, len(search.Roots))
	for _, raw := range search.Roots {
		root := strings.TrimSpace(raw)
		if root == "" {
			continue
		}
		if err := validateRemotePath(root); err != nil {
			return nil, fmt.Errorf("%w: 检索目录无效 %q: %w", errs.ErrInvalidInput, raw, err)
		}
		if !path.IsAbs(root) {
			return nil, fmt.Errorf("%w: 检索目录必须是绝对路径: %q", errs.ErrInvalidInput, raw)
		}
		cleaned := path.Clean(root)
		if _, ok := seen[cleaned]; ok {
			continue
		}
		seen[cleaned] = struct{}{}
		roots = append(roots, cleaned)
	}
	if len(roots) == 0 {
		return []string{defaultSSHLogSearchRoot}, nil
	}
	if len(roots) > maxSSHLogSearchRoots {
		return nil, fmt.Errorf("%w: 检索目录过多（%d 个，上限 %d）",
			errs.ErrInvalidInput, len(roots), maxSSHLogSearchRoots)
	}
	return roots, nil
}

func validateSSHLogSearchKeyword(search SSHLogSearchOptions) error {
	if search.SearchContent && search.Keyword == "" {
		return fmt.Errorf("%w: 内容检索关键字不能为空", errs.ErrInvalidInput)
	}
	if err := validateOptionValue(search.Keyword); err != nil {
		return fmt.Errorf("%w: 检索关键字无效: %w", errs.ErrInvalidInput, err)
	}
	return nil
}

// findPathGlob 把关键字转换为 fnmatch 模式，转义 * ? [ \，实现"路径包含关键字"的字面匹配。
func findPathGlob(keyword string) string {
	var b strings.Builder
	b.Grow(len(keyword) + 8)
	b.WriteByte('*')
	for _, r := range keyword {
		switch r {
		case '*', '?':
			b.WriteByte('[')
			b.WriteRune(r)
			b.WriteByte(']')
		case '[':
			b.WriteString("[[]")
		case '\\':
			b.WriteString(`\\`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('*')
	return b.String()
}

func normalizeSSHLogSearchMaxResults(n int) int {
	if n <= 0 {
		return defaultSSHLogSearchMaxResults
	}
	if n > maxSSHLogSearchMaxResults {
		return maxSSHLogSearchMaxResults
	}
	return n
}

// readNULRecords 逐条读取 NUL 分隔记录。末尾缺少 NUL 的残片会被丢弃，
// 避免把远端中断产生的半条路径当成完整结果。
func readNULRecords(reader io.Reader, handle func(string) error) error {
	if reader == nil {
		return fmt.Errorf("读取远程输出失败: reader 为空")
	}
	if handle == nil {
		return fmt.Errorf("读取远程输出失败: 处理函数为空")
	}

	br := bufio.NewReaderSize(reader, readBufferSize)
	pending := make([]byte, 0, 256)
	for {
		chunk, err := br.ReadSlice(0)
		if len(chunk) > 0 {
			if chunk[len(chunk)-1] == 0 {
				record := chunk[:len(chunk)-1]
				if len(pending) > 0 {
					if len(pending)+len(record) > maxNULRecordBytes {
						return fmt.Errorf("远程输出单条记录超过上限 %d 字节", maxNULRecordBytes)
					}
					pending = append(pending, record...)
					record = pending
					pending = nil
				} else if len(record) > maxNULRecordBytes {
					return fmt.Errorf("远程输出单条记录超过上限 %d 字节", maxNULRecordBytes)
				}
				if handleErr := handle(string(record)); handleErr != nil {
					return handleErr
				}
			} else {
				if len(pending)+len(chunk) > maxNULRecordBytes {
					return fmt.Errorf("远程输出单条记录超过上限 %d 字节", maxNULRecordBytes)
				}
				pending = append(pending, chunk...)
			}
		}
		if err == nil {
			continue
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
}

// parseSSHLogFindRecord 解析 -printf '%T@\t%s\t%p\0' 的单条记录。
// 使用 SplitN(…, 3) 保留路径中的制表符。
func parseSSHLogFindRecord(record string) (models.SSHRemoteLogFile, bool) {
	parts := strings.SplitN(record, "\t", 3)
	if len(parts) != 3 {
		return models.SSHRemoteLogFile{}, false
	}
	modTime, err := parseFindModTime(parts[0])
	if err != nil {
		return models.SSHRemoteLogFile{}, false
	}
	size, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
	if err != nil || size < 0 {
		return models.SSHRemoteLogFile{}, false
	}
	remotePath := parts[2]
	if remotePath == "" {
		return models.SSHRemoteLogFile{}, false
	}
	return models.SSHRemoteLogFile{Path: remotePath, Size: size, ModTime: modTime}, true
}

// parseFindModTime 解析 GNU find %T@ 的秒.纳秒格式，避免浮点精度损失。
func parseFindModTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, fmt.Errorf("时间戳为空")
	}
	secondsPart := raw
	fractionPart := ""
	if i := strings.IndexByte(raw, '.'); i >= 0 {
		secondsPart = raw[:i]
		fractionPart = raw[i+1:]
	}
	seconds, err := strconv.ParseInt(secondsPart, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("时间戳无效: %q", raw)
	}
	var nanoseconds int64
	if fractionPart != "" {
		if len(fractionPart) > 9 {
			fractionPart = fractionPart[:9]
		}
		for len(fractionPart) < 9 {
			fractionPart += "0"
		}
		nanoseconds, err = strconv.ParseInt(fractionPart, 10, 64)
		if err != nil {
			return time.Time{}, fmt.Errorf("时间戳无效: %q", raw)
		}
	}
	return time.Unix(seconds, nanoseconds).UTC(), nil
}

// readSSHLogScanResults 解析扫描模式的 NUL/TSV 输出；解析失败的单条记录跳过。
// 达到 maxResults 时返回 errSSHLogSearchLimit，以便调用方终止远端 find。
func readSSHLogScanResults(reader io.Reader, maxResults int) ([]models.SSHRemoteLogFile, bool, error) {
	files := make([]models.SSHRemoteLogFile, 0, maxResults)
	truncated := false
	err := readNULRecords(reader, func(record string) error {
		if record == "" {
			return nil
		}
		file, ok := parseSSHLogFindRecord(record)
		if !ok {
			return nil
		}
		files = append(files, file)
		if len(files) >= maxResults {
			truncated = true
			return errSSHLogSearchLimit
		}
		return nil
	})
	return files, truncated, err
}

// readSSHLogContentPaths 解析内容检索模式的 NUL 分隔路径；空记录跳过。
func readSSHLogContentPaths(reader io.Reader, maxResults int) ([]string, bool, error) {
	paths := make([]string, 0, maxResults)
	truncated := false
	err := readNULRecords(reader, func(record string) error {
		if record == "" {
			return nil
		}
		paths = append(paths, record)
		if len(paths) >= maxResults {
			truncated = true
			return errSSHLogSearchLimit
		}
		return nil
	})
	return paths, truncated, err
}

// sortSSHLogFiles 按修改时间降序、路径升序排序。
func sortSSHLogFiles(files []models.SSHRemoteLogFile) {
	sort.SliceStable(files, func(i, j int) bool {
		if files[i].ModTime.Equal(files[j].ModTime) {
			return files[i].Path < files[j].Path
		}
		return files[i].ModTime.After(files[j].ModTime)
	})
}

// isSSHLogSearchNoMatchExit 判断内容检索是否属于"grep 无匹配/部分文件无权限"。
// xargs 在 grep 返回 1 时汇总为 123；部分 xargs 实现直接返回 1。
func isSSHLogSearchNoMatchExit(err error) bool {
	var exitErr *ssh.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}
	switch exitErr.ExitStatus() {
	case 1, 123:
		return true
	default:
		return false
	}
}

// newSSHLogSearchSFTPClient 创建 SFTP 客户端；取消/超时时关闭底层连接唤醒握手。
func newSSHLogSearchSFTPClient(ctx context.Context, client *ssh.Client) (*sftp.Client, error) {
	if client == nil {
		return nil, fmt.Errorf("SSH 客户端未初始化")
	}
	type sftpResult struct {
		client *sftp.Client
		err    error
	}
	done := make(chan sftpResult, 1)
	go func() {
		c, err := sftp.NewClient(client)
		done <- sftpResult{client: c, err: err}
	}()
	select {
	case res := <-done:
		return res.client, res.err
	case <-ctx.Done():
		_ = client.Close()
		return nil, ctx.Err()
	}
}

// SearchSSHLogs 在远程 Linux 主机上检索日志文件。
// 返回命中文件（已排序）以及是否因 MaxResults 截断；只执行只读命令。
func SearchSSHLogs(ctx context.Context, opts SSHOptions, search SSHLogSearchOptions) ([]models.SSHRemoteLogFile, bool, error) {
	ctx = ensureContext(ctx)

	normalized, err := normalizeSSHOptions(opts)
	if err != nil {
		return nil, false, err
	}
	command, err := buildSSHLogFindCommand(search)
	if err != nil {
		return nil, false, err
	}
	maxResults := normalizeSSHLogSearchMaxResults(search.MaxResults)
	if err := stoppedError(ctx); err != nil {
		return nil, false, err
	}

	conn, err := dialSSH(ctx, normalized)
	if err != nil {
		if stop := stoppedError(ctx); stop != nil {
			return nil, false, stop
		}
		return nil, false, fmt.Errorf("%w: 连接 %s 失败: %w",
			errs.ErrSSHConnectionFailed, hostLabel(normalized), err)
	}
	defer conn.Close()

	host := hostLabel(normalized)
	if !search.SearchContent {
		return runSSHLogScanSearch(ctx, conn.client, host, command, maxResults)
	}
	return runSSHLogContentSearch(ctx, conn.client, host, command, maxResults)
}

func runSSHLogScanSearch(ctx context.Context, client *ssh.Client, host, command string, maxResults int) ([]models.SSHRemoteLogFile, bool, error) {
	var files []models.SSHRemoteLogFile
	var truncated bool

	stderrText, runErr := runSSHStreamDetailed(ctx, client, command, func(reader io.Reader) error {
		result, wasTruncated, parseErr := readSSHLogScanResults(reader, maxResults)
		files = result
		truncated = wasTruncated
		return parseErr
	})
	if errors.Is(runErr, errSSHLogSearchLimit) {
		truncated = true
		runErr = nil
	}
	if runErr != nil {
		if ctx.Err() != nil {
			return nil, false, stoppedError(ctx)
		}
		if len(files) > 0 {
			// find 的 exit 1 通常是个别目录无权限；已经取得结果时忽略。
			runErr = nil
		}
	}
	if runErr == nil && len(files) == 0 {
		if detail := strings.TrimSpace(stderrText); detail != "" {
			return nil, false, fmt.Errorf("SSH %s 远程日志检索失败: %s", host, truncateString(detail, 2048))
		}
	}
	if runErr != nil {
		return nil, false, fmt.Errorf("SSH %s 远程日志检索失败: %v", host, runErr)
	}

	sortSSHLogFiles(files)
	return files, truncated, nil
}

func runSSHLogContentSearch(ctx context.Context, client *ssh.Client, host, command string, maxResults int) ([]models.SSHRemoteLogFile, bool, error) {
	var paths []string
	var truncated bool

	stderrText, runErr := runSSHStreamDetailed(ctx, client, command, func(reader io.Reader) error {
		result, wasTruncated, parseErr := readSSHLogContentPaths(reader, maxResults)
		paths = result
		truncated = wasTruncated
		return parseErr
	})
	if errors.Is(runErr, errSSHLogSearchLimit) {
		truncated = true
		runErr = nil
	}

	// degraded 表示把远端非零退出码当作"无匹配"处理，此时 stderr 里的
	// find 权限告警同样不再作为错误上报。
	degraded := false
	if runErr != nil {
		switch {
		case ctx.Err() != nil:
			return nil, false, stoppedError(ctx)
		case len(paths) > 0:
			// 已取得结果，忽略个别文件无权限等原因导致的非零退出。
			runErr = nil
		case isSSHLogSearchNoMatchExit(runErr):
			runErr = nil
			degraded = true
		}
	}
	if runErr == nil && len(paths) == 0 && !degraded {
		if detail := strings.TrimSpace(stderrText); detail != "" {
			return nil, false, fmt.Errorf("SSH %s 远程日志检索失败: %s", host, truncateString(detail, 2048))
		}
	}
	if runErr != nil {
		return nil, false, fmt.Errorf("SSH %s 远程日志检索失败: %v", host, runErr)
	}
	if len(paths) == 0 {
		return []models.SSHRemoteLogFile{}, truncated, nil
	}

	sftpClient, sftpErr := newSSHLogSearchSFTPClient(ctx, client)
	if sftpErr != nil {
		if ctx.Err() != nil {
			return nil, false, stoppedError(ctx)
		}
		return nil, false, fmt.Errorf("%w: SSH %s 启动 SFTP 失败: %w",
			errs.ErrSSHConnectionFailed, host, sftpErr)
	}
	defer sftpClient.Close()

	// SFTP 读写没有 context 参数；取消时关闭客户端唤醒阻塞调用。
	sftpWatchDone := make(chan struct{})
	defer close(sftpWatchDone)
	go func() {
		select {
		case <-ctx.Done():
			_ = sftpClient.Close()
		case <-sftpWatchDone:
		}
	}()

	files := make([]models.SSHRemoteLogFile, 0, len(paths))
	for _, remotePath := range paths {
		if ctx.Err() != nil {
			return nil, false, stoppedError(ctx)
		}
		info, statErr := sftpClient.Stat(remotePath)
		if statErr != nil {
			// 单个文件可能在检索与 Stat 之间被轮转/删除，跳过即可。
			continue
		}
		if info.IsDir() {
			continue
		}
		files = append(files, models.SSHRemoteLogFile{
			Path:    remotePath,
			Size:    info.Size(),
			ModTime: info.ModTime(),
		})
	}

	sortSSHLogFiles(files)
	return files, truncated, nil
}
