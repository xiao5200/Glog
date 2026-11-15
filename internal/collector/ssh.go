package collector

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"glog/internal/errs"
	"glog/pkg/models"
)

const (
	defaultSSHConcurrency = 8
	defaultSSHTimeout     = 30 * time.Second
	minSSHTimeout         = time.Second
	maxSSHTimeout         = 10 * time.Minute
	maxSSHConcurrency     = 64
	maxFilterPatternBytes = 4096
)

const (
	strategyPlain       = "plain"
	strategyCompress    = "compress"
	strategyFilter      = "filter"
	strategyJournalctl  = "journalctl"
	strategyIncremental = "incremental"
)

// SSHOptions SSH 远程采集选项。
type SSHOptions struct {
	Host     string
	Port     int
	Username string
	Password string
	KeyPath  string
	// KeyPassphrase 是可选扩展：用于解密带口令的私钥。
	KeyPassphrase string
	ProxyJump     string
	Timeout       time.Duration
	Compression   string
	Concurrency   int
	Strategy      string
	FilterPattern string
	RemotePaths   []string
	Since         string
	Until         string
	Unit          string
	TailOffsets   map[string]int64
}

// SSHCollector SSH 远程日志采集器。
type SSHCollector struct {
	opts     SSHOptions
	cOptions Options

	mu      sync.Mutex
	offsets map[string]int64
}

// NewSSHCollector 创建 SSH 采集器，TailOffsets 会被复制，调用方后续修改不影响采集器。
func NewSSHCollector(opts SSHOptions, cOptions Options) *SSHCollector {
	offsets := make(map[string]int64, len(opts.TailOffsets))
	for remotePath, offset := range opts.TailOffsets {
		offsets[remotePath] = offset
	}
	opts.TailOffsets = offsets
	if opts.Concurrency <= 0 {
		opts.Concurrency = cOptions.Concurrency
	}
	return &SSHCollector{
		opts:     opts,
		cOptions: cOptions,
		offsets:  offsets,
	}
}

// Offsets 返回增量采集后的远端文件 offset 快照（解压前/原始文件字节偏移）。
func (c *SSHCollector) Offsets() map[string]int64 {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	out := make(map[string]int64, len(c.offsets))
	for remotePath, offset := range c.offsets {
		out[remotePath] = offset
	}
	return out
}

func (c *SSHCollector) offsetFor(remotePath string) int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.offsets[remotePath]
}

func (c *SSHCollector) setOffset(remotePath string, offset int64) {
	c.mu.Lock()
	if c.offsets == nil {
		c.offsets = make(map[string]int64)
	}
	c.offsets[remotePath] = offset
	c.mu.Unlock()
}

type sshPlan struct {
	strategy       string
	paths          []string
	filter         *regexp.Regexp
	journalCommand string
	journalLabel   string
}

func buildSSHPlan(opts SSHOptions) (*sshPlan, error) {
	switch opts.Strategy {
	case strategyPlain, strategyCompress, strategyFilter, strategyJournalctl, strategyIncremental:
	default:
		return nil, fmt.Errorf("不支持的 SSH 采集策略 %q（支持: plain/compress/filter/journalctl/incremental）", opts.Strategy)
	}

	plan := &sshPlan{strategy: opts.Strategy}

	if opts.Strategy == strategyJournalctl {
		command, err := remoteJournalCommand(opts)
		if err != nil {
			return nil, err
		}
		plan.journalCommand = command
		plan.journalLabel = sshJournalLabel(opts)
		return plan, nil
	}

	if len(opts.RemotePaths) == 0 {
		return nil, fmt.Errorf("未提供远程日志路径")
	}
	seen := make(map[string]struct{}, len(opts.RemotePaths))
	for _, rawPath := range opts.RemotePaths {
		if strings.TrimSpace(rawPath) == "" {
			return nil, fmt.Errorf("远程日志路径不能为空")
		}
		if err := validateRemotePath(rawPath); err != nil {
			return nil, fmt.Errorf("远程路径无效 %q: %w", rawPath, err)
		}
		if _, ok := seen[rawPath]; ok {
			continue
		}
		seen[rawPath] = struct{}{}
		plan.paths = append(plan.paths, rawPath)
	}
	if len(plan.paths) == 0 {
		return nil, fmt.Errorf("未提供有效的远程日志路径")
	}

	switch opts.Strategy {
	case strategyFilter:
		if strings.TrimSpace(opts.FilterPattern) == "" {
			return nil, fmt.Errorf("filter 策略必须提供 FilterPattern")
		}
		if len(opts.FilterPattern) > maxFilterPatternBytes {
			return nil, fmt.Errorf("FilterPattern 过长（超过 %d 字节）", maxFilterPatternBytes)
		}
		pattern, err := regexp.Compile(opts.FilterPattern)
		if err != nil {
			return nil, fmt.Errorf("FilterPattern 不是合法的正则表达式: %w", err)
		}
		plan.filter = pattern
	case strategyCompress:
		switch opts.Compression {
		case "gzip", "none":
			// 支持 tar.gz 与未压缩 tar。
		case "zstd":
			return nil, fmt.Errorf("compress 策略暂不支持 zstd（需要远端 zstd 与本地解码器）")
		default:
			return nil, fmt.Errorf("compress 策略不支持压缩算法 %q（支持: gzip/none）", opts.Compression)
		}
	case strategyIncremental:
		for remotePath, offset := range opts.TailOffsets {
			if offset < 0 {
				return nil, fmt.Errorf("增量采集 offset 不能为负数: %s = %d", remotePath, offset)
			}
		}
	}
	return plan, nil
}

// Collect 按指定策略执行 SSH 远程采集。
func (c *SSHCollector) Collect(ctx context.Context, emit func(models.RawLine) error) (*Result, error) {
	if c == nil {
		return nil, fmt.Errorf("SSH 采集器不能为空")
	}
	ctx = ensureContext(ctx)

	opts, err := normalizeSSHOptions(c.opts)
	if err != nil {
		return nil, err
	}
	plan, err := buildSSHPlan(opts)
	if err != nil {
		return nil, err
	}
	em, err := newEmitter(ctx, emit)
	if err != nil {
		return nil, err
	}
	defer em.Close()

	if err := stoppedError(ctx); err != nil {
		return nil, err
	}

	conn, err := dialSSH(ctx, opts)
	if err != nil {
		if stop := stoppedError(ctx); stop != nil {
			return nil, stop
		}
		return nil, fmt.Errorf("%w: 连接 %s 失败: %w", errs.ErrSSHConnectionFailed, hostLabel(opts), err)
	}
	defer conn.Close()

	acc := newResultAccumulator()
	host := hostLabel(opts)
	switch plan.strategy {
	case strategyJournalctl:
		c.collectRemoteJournal(em.ctx, conn.client, host, plan, acc, em)

	case strategyCompress:
		runParallel(em.ctx, plan.paths, opts.Concurrency, func(workCtx context.Context, remotePath string) {
			c.collectCompressedPath(workCtx, conn.client, host, opts.Compression, remotePath, acc, em)
		})

	case strategyIncremental:
		runParallel(em.ctx, plan.paths, opts.Concurrency, func(workCtx context.Context, remotePath string) {
			c.collectIncrementalPath(workCtx, conn.client, host, remotePath, acc, em)
		})

	default:
		sftpClient, sftpErr := sftp.NewClient(conn.client)
		if sftpErr != nil {
			return acc.snapshot(), fmt.Errorf("%w: SSH %s 启动 SFTP 失败: %w",
				errs.ErrSSHConnectionFailed, hostLabel(opts), sftpErr)
		}
		defer sftpClient.Close()

		// SFTP 读写没有 context 参数。取消时关闭底层连接，唤醒可能阻塞的 Read。
		sftpWatchDone := make(chan struct{})
		defer close(sftpWatchDone)
		go func() {
			select {
			case <-em.ctx.Done():
				_ = sftpClient.Close()
			case <-sftpWatchDone:
			}
		}()

		runParallel(em.ctx, plan.paths, opts.Concurrency, func(workCtx context.Context, remotePath string) {
			c.collectSFTPFile(workCtx, sftpClient, host, remotePath, plan.filter, acc, em)
		})
	}

	return finalize(ctx, em, acc)
}

func normalizeSSHOptions(opts SSHOptions) (SSHOptions, error) {
	opts.Host = strings.TrimSpace(opts.Host)
	if opts.Host == "" {
		return opts, fmt.Errorf("SSH 主机地址不能为空")
	}
	if strings.HasPrefix(opts.Host, "[") && strings.HasSuffix(opts.Host, "]") {
		opts.Host = opts.Host[1 : len(opts.Host)-1]
	}
	if opts.Host == "" {
		return opts, fmt.Errorf("SSH 主机地址不能为空")
	}
	if strings.ContainsAny(opts.Host, " \t\r\n\x00@") {
		return opts, fmt.Errorf("SSH 主机地址包含非法字符: %q", opts.Host)
	}
	if len(opts.Host) > 1024 {
		return opts, fmt.Errorf("SSH 主机地址过长")
	}
	if (strings.HasPrefix(opts.Host, "[") && strings.Contains(opts.Host, "]:")) ||
		strings.Count(opts.Host, ":") == 1 {
		if _, _, splitErr := net.SplitHostPort(opts.Host); splitErr == nil {
			return opts, fmt.Errorf("SSH 主机地址请勿包含端口，请使用 Port 字段")
		}
	}

	opts.Username = strings.TrimSpace(opts.Username)
	if opts.Username == "" {
		return opts, fmt.Errorf("SSH 用户名不能为空")
	}
	if strings.ContainsAny(opts.Username, " \t\r\n\x00") {
		return opts, fmt.Errorf("SSH 用户名包含非法字符")
	}

	if opts.Port == 0 {
		opts.Port = 22
	}
	if opts.Port < 1 || opts.Port > 65535 {
		return opts, fmt.Errorf("SSH 端口无效: %d", opts.Port)
	}
	if opts.Timeout <= 0 {
		opts.Timeout = defaultSSHTimeout
	}
	if opts.Timeout < minSSHTimeout {
		opts.Timeout = minSSHTimeout
	}
	if opts.Timeout > maxSSHTimeout {
		opts.Timeout = maxSSHTimeout
	}
	if opts.Concurrency <= 0 {
		opts.Concurrency = defaultSSHConcurrency
	}
	if opts.Concurrency > maxSSHConcurrency {
		opts.Concurrency = maxSSHConcurrency
	}

	opts.Compression = strings.ToLower(strings.TrimSpace(opts.Compression))
	if opts.Compression == "" {
		opts.Compression = "gzip"
	}
	opts.Strategy = strings.ToLower(strings.TrimSpace(opts.Strategy))
	if opts.Strategy == "" {
		opts.Strategy = strategyPlain
	}
	opts.KeyPath = strings.TrimSpace(opts.KeyPath)
	opts.ProxyJump = strings.TrimSpace(opts.ProxyJump)
	if opts.KeyPath == "" && opts.Password == "" {
		return opts, fmt.Errorf("未提供 SSH 密码或私钥")
	}
	return opts, nil
}

func hostLabel(opts SSHOptions) string {
	return fmt.Sprintf("%s@%s", opts.Username, net.JoinHostPort(opts.Host, strconv.Itoa(opts.Port)))
}

type sshConn struct {
	client *ssh.Client
	jump   *ssh.Client
}

func (c *sshConn) Close() error {
	if c == nil {
		return nil
	}
	var closeErrs []error
	if c.client != nil {
		closeErrs = append(closeErrs, c.client.Close())
	}
	if c.jump != nil {
		closeErrs = append(closeErrs, c.jump.Close())
	}
	return errors.Join(closeErrs...)
}

func dialSSH(ctx context.Context, opts SSHOptions) (*sshConn, error) {
	ctx = ensureContext(ctx)
	authMethods, err := buildSSHAuth(opts)
	if err != nil {
		return nil, err
	}

	targetAddr := net.JoinHostPort(opts.Host, strconv.Itoa(opts.Port))
	// 个人单机分析工具的取舍：不做 known_hosts 校验，以避免首次连接/重装系统
	// 后的交互阻塞。代价是无法防御中间人攻击；连接不可信网络时应改为固定主机密钥。
	hostKeyCallback := ssh.InsecureIgnoreHostKey()
	targetConfig := &ssh.ClientConfig{
		User:            opts.Username,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         opts.Timeout,
	}

	if opts.ProxyJump == "" {
		client, dialErr := dialSSHClient(ctx, targetAddr, targetConfig, opts.Timeout)
		if dialErr != nil {
			return nil, fmt.Errorf("连接 %s 失败: %w", targetAddr, dialErr)
		}
		return &sshConn{client: client}, nil
	}

	jumpUser, jumpAddr, err := parseProxyJump(opts.ProxyJump)
	if err != nil {
		return nil, fmt.Errorf("ProxyJump 配置无效: %w", err)
	}
	if jumpUser == "" {
		jumpUser = opts.Username
	}
	jumpConfig := &ssh.ClientConfig{
		User:            jumpUser,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         opts.Timeout,
	}
	jumpClient, err := dialSSHClient(ctx, jumpAddr, jumpConfig, opts.Timeout)
	if err != nil {
		return nil, fmt.Errorf("连接跳板机 %s 失败: %w", jumpAddr, err)
	}

	rawConn, err := jumpClient.DialContext(ctx, "tcp", targetAddr)
	if err != nil {
		_ = jumpClient.Close()
		return nil, fmt.Errorf("通过跳板机连接 %s 失败: %w", targetAddr, err)
	}
	targetClient, err := newSSHClient(ctx, rawConn, targetAddr, targetConfig, opts.Timeout)
	if err != nil {
		_ = jumpClient.Close()
		return nil, fmt.Errorf("通过跳板机完成 SSH 握手失败: %w", err)
	}
	return &sshConn{client: targetClient, jump: jumpClient}, nil
}

func dialSSHClient(ctx context.Context, addr string, config *ssh.ClientConfig, timeout time.Duration) (*ssh.Client, error) {
	dialer := &net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	return newSSHClient(ctx, conn, addr, config, timeout)
}

func newSSHClient(ctx context.Context, conn net.Conn, addr string, config *ssh.ClientConfig, timeout time.Duration) (*ssh.Client, error) {
	if conn == nil {
		return nil, fmt.Errorf("SSH 底层连接为空")
	}
	ctx = ensureContext(ctx)

	if timeout > 0 {
		_ = conn.SetDeadline(time.Now().Add(timeout))
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()

	clientConn, chans, reqs, err := ssh.NewClientConn(conn, addr, config)
	close(done)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	_ = conn.SetDeadline(time.Time{})
	if err := ctx.Err(); err != nil {
		client := ssh.NewClient(clientConn, chans, reqs)
		_ = client.Close()
		return nil, err
	}
	return ssh.NewClient(clientConn, chans, reqs), nil
}

func parseProxyJump(spec string) (string, string, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return "", "", fmt.Errorf("跳板机地址不能为空")
	}
	if strings.ContainsAny(spec, " \t\r\n\x00") {
		return "", "", fmt.Errorf("跳板机地址包含非法字符")
	}

	user := ""
	if i := strings.LastIndex(spec, "@"); i >= 0 {
		user = spec[:i]
		spec = spec[i+1:]
		if user == "" || strings.ContainsAny(user, " \t\r\n\x00@") {
			return "", "", fmt.Errorf("跳板机用户名无效")
		}
	}
	if spec == "" {
		return "", "", fmt.Errorf("跳板机主机地址不能为空")
	}

	host := spec
	port := 22
	switch {
	case strings.HasPrefix(spec, "[") && strings.HasSuffix(spec, "]"):
		host = spec[1 : len(spec)-1]
	case strings.HasPrefix(spec, "["):
		h, p, err := net.SplitHostPort(spec)
		if err != nil {
			return "", "", fmt.Errorf("跳板机地址格式无效: %w", err)
		}
		host = h
		parsedPort, err := strconv.Atoi(p)
		if err != nil {
			return "", "", fmt.Errorf("跳板机端口无效: %q", p)
		}
		port = parsedPort
	case strings.Count(spec, ":") == 1:
		h, p, err := net.SplitHostPort(spec)
		if err != nil {
			return "", "", fmt.Errorf("跳板机地址格式无效: %w", err)
		}
		host = h
		parsedPort, err := strconv.Atoi(p)
		if err != nil {
			return "", "", fmt.Errorf("跳板机端口无效: %q", p)
		}
		port = parsedPort
	}

	if host == "" {
		return "", "", fmt.Errorf("跳板机主机地址不能为空")
	}
	if strings.ContainsAny(host, " \t\r\n\x00@") {
		return "", "", fmt.Errorf("跳板机主机地址包含非法字符")
	}
	if port < 1 || port > 65535 {
		return "", "", fmt.Errorf("跳板机端口无效: %d", port)
	}
	return user, net.JoinHostPort(host, strconv.Itoa(port)), nil
}

func buildSSHAuth(opts SSHOptions) ([]ssh.AuthMethod, error) {
	methods := make([]ssh.AuthMethod, 0, 2)

	if opts.KeyPath != "" {
		keyPath, err := expandHome(opts.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("展开 SSH 私钥路径失败: %w", err)
		}
		keyInfo, err := os.Stat(keyPath)
		if err != nil {
			return nil, fmt.Errorf("检查 SSH 私钥失败 %s: %w", keyPath, err)
		}
		if !keyInfo.Mode().IsRegular() {
			return nil, fmt.Errorf("SSH 私钥不是常规文件: %s", keyPath)
		}
		const maxKeyBytes = 1 << 20
		if keyInfo.Size() > maxKeyBytes {
			return nil, fmt.Errorf("SSH 私钥文件过大（%d 字节，上限 %d）", keyInfo.Size(), maxKeyBytes)
		}
		pemBytes, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, fmt.Errorf("读取 SSH 私钥失败 %s: %w", keyPath, err)
		}

		var signer ssh.Signer
		if opts.KeyPassphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(pemBytes, []byte(opts.KeyPassphrase))
		} else {
			signer, err = ssh.ParsePrivateKey(pemBytes)
		}
		if err != nil {
			var missingPassphrase *ssh.PassphraseMissingError
			if errors.As(err, &missingPassphrase) {
				return nil, fmt.Errorf("SSH 私钥已加密，请提供 KeyPassphrase: %w", err)
			}
			return nil, fmt.Errorf("解析 SSH 私钥失败 %s: %w", keyPath, err)
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}

	if opts.Password != "" {
		methods = append(methods, ssh.Password(opts.Password))
	}
	if len(methods) == 0 {
		return nil, fmt.Errorf("未提供 SSH 密码或私钥")
	}
	return methods, nil
}

func remoteJournalCommand(opts SSHOptions) (string, error) {
	parts := []string{"journalctl", "--no-pager", "--output", "json"}
	addOption := func(flag, value string) error {
		if value == "" {
			return nil
		}
		if err := validateOptionValue(value); err != nil {
			return fmt.Errorf("%s 参数无效: %w", flag, err)
		}
		quoted, err := shellQuote(flag + "=" + value)
		if err != nil {
			return fmt.Errorf("%s 参数无效: %w", flag, err)
		}
		parts = append(parts, quoted)
		return nil
	}

	if err := addOption("--since", opts.Since); err != nil {
		return "", err
	}
	if err := addOption("--until", opts.Until); err != nil {
		return "", err
	}
	if err := addOption("--unit", opts.Unit); err != nil {
		return "", err
	}
	return strings.Join(parts, " "), nil
}

func sshJournalLabel(opts SSHOptions) string {
	unit := strings.TrimSpace(opts.Unit)
	if unit != "" {
		return "journalctl:" + unit
	}
	return "journalctl:remote"
}

func remoteTailCommand(remotePath string, start int64) (string, error) {
	if start < 0 {
		return "", fmt.Errorf("增量 offset 不能为负数")
	}
	if start >= int64(^uint64(0)>>1) {
		return "", fmt.Errorf("增量 offset 过大")
	}
	quoted, err := shellQuote(remotePath)
	if err != nil {
		return "", fmt.Errorf("远程路径无效: %w", err)
	}
	return fmt.Sprintf("tail -c +%d -- %s", start+1, quoted), nil
}

func runParallel(ctx context.Context, items []string, concurrency int, fn func(context.Context, string)) {
	if len(items) == 0 {
		return
	}
	if concurrency < 1 {
		concurrency = 1
	}
	ctx = ensureContext(ctx)
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for _, item := range items {
		select {
		case <-ctx.Done():
			wg.Wait()
			return
		case sem <- struct{}{}:
		}

		wg.Add(1)
		go func(value string) {
			defer wg.Done()
			defer func() { <-sem }()
			fn(ctx, value)
		}(item)
	}
	wg.Wait()
}

func (c *SSHCollector) collectSFTPFile(ctx context.Context, client *sftp.Client, host, remotePath string,
	filter *regexp.Regexp, acc *resultAccumulator, em *emitter) {

	info, err := client.Stat(remotePath)
	if err != nil {
		acc.addErrorf("SSH %s 获取远程文件信息失败 %s: %v", host, remotePath, err)
		return
	}
	if info.IsDir() {
		acc.addErrorf("SSH %s 远程路径是目录，plain/filter 策略需要具体文件: %s", host, remotePath)
		return
	}
	if !info.Mode().IsRegular() {
		acc.addErrorf("SSH %s 远程路径不是常规文件: %s", host, remotePath)
		return
	}

	f, err := client.Open(remotePath)
	if err != nil {
		acc.addErrorf("SSH %s 打开远程文件失败 %s: %v", host, remotePath, err)
		return
	}
	defer f.Close()

	rd, closeGzip, err := maybeGzipReader(f, remotePath)
	if err != nil {
		acc.addErrorf("SSH %s 解压远程文件失败 %s: %v", host, remotePath, err)
		return
	}
	defer func() { _ = closeGzip() }()

	acc.addFile(remotePath)
	br := bufio.NewReaderSize(rd, readBufferSize)
	limit := c.cOptions.lineLimit()
	var offset int64

	for {
		if err := ctx.Err(); err != nil {
			return
		}
		line, consumed, truncated, readErr := readLineLimited(br, limit)
		startOffset := offset
		offset += consumed
		acc.addBytes(consumed)

		if readErr != nil {
			if readErr == io.EOF {
				return
			}
			if ctx.Err() != nil {
				return
			}
			acc.addErrorf("SSH %s 读取远程文件失败 %s（偏移 %d）: %v",
				host, remotePath, startOffset, readErr)
			return
		}
		if truncated {
			acc.addErrorf("SSH %s 文件 %s 偏移 %d 处存在超长行（超过 %d 字节），已截断",
				host, remotePath, startOffset, limit)
		}
		if filter != nil && !filter.Match(line) {
			continue
		}
		if ctx.Err() != nil {
			return
		}

		raw := models.RawLine{
			SourceID:   c.cOptions.SourceID,
			SourceHost: c.cOptions.SourceHost,
			FilePath:   remotePath,
			Offset:     startOffset,
			Size:       consumed,
			Text:       string(line),
			Structured: false,
		}
		if err := em.Emit(raw); err != nil {
			return
		}
		acc.addLine()
	}
}

func (c *SSHCollector) collectIncrementalPath(ctx context.Context, client *ssh.Client, host, remotePath string,
	acc *resultAccumulator, em *emitter) {

	if isGzipPath(remotePath) {
		acc.addErrorf("SSH %s 增量采集不支持压缩文件: %s", host, remotePath)
		return
	}

	start := c.offsetFor(remotePath)
	command, err := remoteTailCommand(remotePath, start)
	if err != nil {
		acc.addErrorf("SSH %s 构建增量采集命令失败 %s: %v", host, remotePath, err)
		return
	}

	acc.addFile(remotePath)
	var consumed int64
	// committed 只推进到已成功 emit 的行；取消/写入失败时宁可下次重复读取，
	// 也不能跳过尚未输出的数据。
	committed := start
	defer func() {
		c.setOffset(remotePath, committed)
	}()

	runErr := runSSHStream(ctx, client, command, func(reader io.Reader) error {
		br := bufio.NewReaderSize(reader, readBufferSize)
		limit := c.cOptions.lineLimit()
		offset := start

		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			line, lineBytes, truncated, readErr := readLineLimited(br, limit)
			startOffset := offset
			offset += lineBytes
			consumed += lineBytes
			acc.addBytes(lineBytes)

			if readErr != nil {
				if readErr == io.EOF {
					return nil
				}
				return readErr
			}
			if truncated {
				acc.addErrorf("SSH %s 增量文件 %s 偏移 %d 处存在超长行（超过 %d 字节），已截断",
					host, remotePath, startOffset, limit)
			}

			raw := models.RawLine{
				SourceID:   c.cOptions.SourceID,
				SourceHost: c.cOptions.SourceHost,
				FilePath:   remotePath,
				Offset:     startOffset,
				Size:       lineBytes,
				Text:       string(line),
				Structured: false,
			}
			if err := em.Emit(raw); err != nil {
				return err
			}
			committed = offset
			acc.addLine()
		}
	})

	if runErr != nil && ctx.Err() == nil && em.Err() == nil {
		acc.addErrorf("SSH %s 增量采集失败 %s: %v", host, remotePath, runErr)
	}
}

func (c *SSHCollector) collectRemoteJournal(ctx context.Context, client *ssh.Client, host string, plan *sshPlan,
	acc *resultAccumulator, em *emitter) {

	acc.addFile(plan.journalLabel)
	var offset int64

	runErr := runSSHStream(ctx, client, plan.journalCommand, func(reader io.Reader) error {
		br := bufio.NewReaderSize(reader, readBufferSize)
		limit := c.cOptions.lineLimit()

		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			line, consumed, truncated, readErr := readLineLimited(br, limit)
			startOffset := offset
			offset += consumed
			acc.addBytes(consumed)

			if readErr != nil {
				if readErr == io.EOF {
					return nil
				}
				return readErr
			}
			if truncated {
				// journalctl 输出是结构化 JSON，截断后无法解析，直接跳过。
				acc.addErrorf("SSH %s journalctl 输出偏移 %d 处存在超长行（超过 %d 字节），已跳过",
					host, startOffset, limit)
				continue
			}

			raw := models.RawLine{
				SourceID:   c.cOptions.SourceID,
				SourceHost: c.cOptions.SourceHost,
				FilePath:   plan.journalLabel,
				Offset:     startOffset,
				Size:       consumed,
				Text:       string(line),
				Structured: true,
			}
			if err := em.Emit(raw); err != nil {
				return err
			}
			acc.addLine()
		}
	})

	if runErr != nil && ctx.Err() == nil && em.Err() == nil {
		acc.addErrorf("SSH %s journalctl 采集失败: %v", host, runErr)
	}
}

// TestSSHConnection 验证 SSH 认证、命令通道或 SFTP 子系统至少有一条可用。
func TestSSHConnection(opts SSHOptions) error {
	normalized, err := normalizeSSHOptions(opts)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), normalized.Timeout)
	defer cancel()

	conn, err := dialSSH(ctx, normalized)
	if err != nil {
		return fmt.Errorf("%w: 连接 %s 失败: %w", errs.ErrSSHConnectionFailed, hostLabel(normalized), err)
	}
	defer conn.Close()

	// 超时/取消时关闭底层连接，确保 NewSession、Stdout 读取等没有 context
	// 参数的阻塞调用也能返回。
	testDone := make(chan struct{})
	defer close(testDone)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.client.Close()
		case <-testDone:
		}
	}()

	echoErr := runSSHStream(ctx, conn.client, "echo ok", func(reader io.Reader) error {
		data, readErr := io.ReadAll(io.LimitReader(reader, 1024))
		if readErr != nil {
			return readErr
		}
		if strings.TrimSpace(string(data)) != "ok" {
			return fmt.Errorf("远端 echo 输出异常: %q", truncateString(string(data), 128))
		}
		return nil
	})
	if echoErr == nil {
		return nil
	}
	if ctx.Err() != nil {
		return fmt.Errorf("%w: %s 连接测试超时或已取消: %v",
			errs.ErrSSHConnectionFailed, hostLabel(normalized), echoErr)
	}

	// sftp.NewClient 没有 context 参数；放到 goroutine 中，超时则关闭底层
	// 连接并立即返回，避免连接测试永久卡住。
	sftpDone := make(chan error, 1)
	go func() {
		sftpClient, sftpErr := sftp.NewClient(conn.client)
		if sftpErr == nil {
			_ = sftpClient.Close()
			sftpDone <- nil
			return
		}
		sftpDone <- sftpErr
	}()

	select {
	case sftpErr := <-sftpDone:
		if sftpErr == nil {
			return nil
		}
		return fmt.Errorf("%w: %s 命令通道不可用（%v），SFTP 子系统也不可用（%v）",
			errs.ErrSSHConnectionFailed, hostLabel(normalized), echoErr, sftpErr)
	case <-ctx.Done():
		_ = conn.client.Close()
		return fmt.Errorf("%w: %s 连接测试超时或已取消: %v",
			errs.ErrSSHConnectionFailed, hostLabel(normalized), ctx.Err())
	}
}
