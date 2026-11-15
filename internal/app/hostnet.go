package app

import (
	"context"
	"encoding/csv"
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"glog/pkg/models"
)

const (
	hostNetCommandTimeout = 5 * time.Second
	maxHostPorts          = 500
	maxHostConnections    = 1000
	maxExternalIPs        = 500
)

// hostNetEntry 是 netstat/ss 解析后的中间结构，便于纯函数测试。
type hostNetEntry struct {
	protocol      string // TCP / UDP
	localAddress  string
	localPort     int
	remoteAddress string
	remotePort    int
	state         string
	process       string
	pid           int
}

// GetHostNetworkSnapshot 返回当前主机的监听端口与外联连接快照。
// 单项命令或协议失败不会中断整个快照，错误记录在 Errors 中。
func (a *App) GetHostNetworkSnapshot() (*models.HostNetworkSnapshot, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}

	snapshot := &models.HostNetworkSnapshot{
		GeneratedAt: time.Now().UTC(),
		Hostname:    hostname(),
		Platform:    runtime.GOOS,
		Arch:        runtime.GOARCH,
		Ports:       []models.HostPort{},
		Connections: []models.HostConnection{},
		ExternalIPs: []string{},
		Errors:      []string{},
	}

	var entries []hostNetEntry
	switch runtime.GOOS {
	case "windows":
		entries, snapshot.Errors = collectWindowsNetEntries()
	case "linux":
		entries, snapshot.Errors = collectLinuxNetEntries()
	default:
		snapshot.Errors = append(snapshot.Errors,
			"当前操作系统暂不支持网络连接采集（仅支持 Windows/Linux）")
	}
	if snapshot.Errors == nil {
		snapshot.Errors = []string{}
	}

	ports := buildHostPorts(entries)
	connections := buildHostConnections(entries)
	snapshot.TotalPorts = len(ports)
	snapshot.TotalConns = len(connections)
	snapshot.ExternalIPs = collectExternalIPs(connections)

	if len(ports) > maxHostPorts {
		ports = ports[:maxHostPorts]
		snapshot.Truncated = true
	}
	if len(connections) > maxHostConnections {
		connections = connections[:maxHostConnections]
		snapshot.Truncated = true
	}
	snapshot.Ports = ports
	snapshot.Connections = connections
	return snapshot, nil
}

// ---------------------------------------------------------------------------
// 命令执行与平台采集
// ---------------------------------------------------------------------------

func runHostCommand(timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	output, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("执行 %s 超时（超过 %s）", name, timeout)
		}
		detail := strings.TrimSpace(strings.ToValidUTF8(string(output), "�"))
		if detail != "" {
			return "", fmt.Errorf("执行 %s 失败：%v（%s）", name, err, truncateHostText(detail, 200))
		}
		return "", fmt.Errorf("执行 %s 失败：%v", name, err)
	}
	return string(output), nil
}

func collectWindowsNetEntries() ([]hostNetEntry, []string) {
	entries := make([]hostNetEntry, 0, 64)
	problems := make([]string, 0)

	for _, protocol := range []string{"tcp", "udp"} {
		output, err := runHostCommand(hostNetCommandTimeout, "netstat", "-ano", "-p", protocol)
		if err != nil {
			problems = append(problems, fmt.Sprintf("采集 %s 端口与连接失败：%v",
				strings.ToUpper(protocol), err))
			continue
		}
		parsed, lineProblems := parseWindowsNetstatOutput(output)
		entries = append(entries, parsed...)
		problems = append(problems, lineProblems...)
	}

	applyProcessNames(entries, windowsProcessNames())
	return entries, problems
}

func windowsProcessNames() map[int]string {
	ctx, cancel := context.WithTimeout(context.Background(), hostNetCommandTimeout)
	defer cancel()

	// tasklist 失败时仅意味着无法补全进程名，仍应返回端口与连接。
	output, err := exec.CommandContext(ctx, "tasklist", "/FO", "CSV", "/NH").Output()
	if err != nil {
		return nil
	}
	return parseTasklistCSV(string(output))
}

func parseTasklistCSV(output string) map[int]string {
	output = strings.TrimPrefix(output, string(rune(0xFEFF)))
	reader := csv.NewReader(strings.NewReader(output))
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true

	records, err := reader.ReadAll()
	if err != nil && len(records) == 0 {
		return nil
	}
	names := make(map[int]string, len(records))
	for _, record := range records {
		if len(record) < 2 {
			continue
		}
		name := strings.TrimSpace(record[0])
		pid, pidErr := strconv.Atoi(strings.TrimSpace(record[1]))
		if pidErr != nil || pid <= 0 || name == "" {
			continue
		}
		names[pid] = name
	}
	return names
}

func collectLinuxNetEntries() ([]hostNetEntry, []string) {
	problems := make([]string, 0)

	output, err := runHostCommand(hostNetCommandTimeout, "ss", "-H", "-tunap")
	if err == nil {
		entries, parseProblems := parseLinuxSSOutput(output)
		if len(entries) > 0 || len(parseProblems) == 0 {
			return entries, parseProblems
		}
		problems = append(problems, "ss 未返回可解析的网络连接信息，尝试使用 netstat 回退")
		problems = append(problems, parseProblems...)
	} else {
		problems = append(problems, "执行 ss 失败，尝试使用 netstat 回退："+err.Error())
	}

	output, err = runHostCommand(hostNetCommandTimeout, "netstat", "-tunap")
	if err != nil {
		problems = append(problems, "执行 netstat 回退失败："+err.Error())
		return nil, problems
	}
	entries, parseProblems := parseLinuxNetstatOutput(output)
	problems = append(problems, parseProblems...)
	return entries, problems
}

// ---------------------------------------------------------------------------
// Windows netstat 解析
// ---------------------------------------------------------------------------

func parseWindowsNetstatOutput(output string) ([]hostNetEntry, []string) {
	entries := make([]hostNetEntry, 0, 32)
	problems := make([]string, 0)

	for index, rawLine := range strings.Split(output, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(rawLine, "\r"))
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		protocol := strings.ToUpper(fields[0])
		if protocol != "TCP" && protocol != "UDP" {
			continue
		}
		entry, ok := parseWindowsNetstatFields(fields, protocol)
		if !ok {
			problems = append(problems, fmt.Sprintf("解析 netstat 第 %d 行失败：%s",
				index+1, truncateHostText(line, 120)))
			continue
		}
		entries = append(entries, entry)
	}
	return entries, problems
}

func parseWindowsNetstatFields(fields []string, protocol string) (hostNetEntry, bool) {
	if len(fields) < 3 {
		return hostNetEntry{}, false
	}
	localAddress, localPort, ok := parseHostPort(fields[1])
	if !ok {
		return hostNetEntry{}, false
	}

	entry := hostNetEntry{
		protocol:     protocol,
		localAddress: localAddress,
		localPort:    localPort,
	}
	if remoteAddress, remotePort, remoteOK := parseHostPort(fields[2]); remoteOK {
		entry.remoteAddress = remoteAddress
		entry.remotePort = remotePort
	}

	if protocol == "TCP" {
		if len(fields) < 5 {
			return hostNetEntry{}, false
		}
		entry.state = strings.ToUpper(fields[3])
		entry.pid = parseHostPID(fields[4])
		return entry, true
	}

	// UDP：proto local foreign pid；部分版本会额外包含 state 列。
	entry.state = "UDP"
	if len(fields) >= 5 {
		entry.state = strings.ToUpper(fields[3])
		entry.pid = parseHostPID(fields[len(fields)-1])
	} else {
		entry.pid = parseHostPID(fields[3])
	}
	return entry, true
}

func parseHostPID(value string) int {
	pid, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || pid < 0 {
		return 0
	}
	return pid
}

// parseHostPort 解析 host:port、[::]:port、*:port 等地址。
// 端口为 * 或空时返回 0；完全无法解析时返回 false。
func parseHostPort(value string) (string, int, bool) {
	value = strings.TrimSpace(value)
	if value == "" || value == "*" || value == "*:*" {
		return "", 0, false
	}

	var host, portText string
	if strings.HasPrefix(value, "[") {
		end := strings.LastIndex(value, "]")
		if end < 0 {
			return "", 0, false
		}
		host = value[1:end]
		rest := value[end+1:]
		if !strings.HasPrefix(rest, ":") {
			return "", 0, false
		}
		portText = rest[1:]
	} else {
		index := strings.LastIndexByte(value, ':')
		if index < 0 {
			return "", 0, false
		}
		host = value[:index]
		portText = value[index+1:]
	}

	host = strings.TrimSpace(host)
	portText = strings.TrimSpace(portText)
	if portText == "" || portText == "*" {
		return host, 0, true
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 0 || port > 65535 {
		return "", 0, false
	}
	return host, port, true
}

func applyProcessNames(entries []hostNetEntry, names map[int]string) {
	if len(names) == 0 {
		return
	}
	for i := range entries {
		if entries[i].process != "" || entries[i].pid <= 0 {
			continue
		}
		entries[i].process = names[entries[i].pid]
	}
}

// ---------------------------------------------------------------------------
// Linux ss/netstat 解析
// ---------------------------------------------------------------------------

var linuxProcessPattern = regexp.MustCompile(`\("([^"]*)",pid=(\d+)`)

func parseLinuxSSOutput(output string) ([]hostNetEntry, []string) {
	entries := make([]hostNetEntry, 0, 64)
	problems := make([]string, 0)

	for index, rawLine := range strings.Split(output, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(rawLine, "\r"))
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		protocol := normalizeHostProtocol(fields[0])
		if protocol == "" {
			continue
		}
		localAddress, localPort, ok := parseHostPort(fields[4])
		if !ok {
			problems = append(problems, fmt.Sprintf("解析 ss 第 %d 行失败：%s",
				index+1, truncateHostText(line, 120)))
			continue
		}

		entry := hostNetEntry{
			protocol:     protocol,
			state:        strings.ToUpper(fields[1]),
			localAddress: localAddress,
			localPort:    localPort,
		}
		if remoteAddress, remotePort, remoteOK := parseHostPort(fields[5]); remoteOK {
			entry.remoteAddress = remoteAddress
			entry.remotePort = remotePort
		}
		if name, pid, processOK := parseLinuxProcess(strings.Join(fields[6:], " ")); processOK {
			entry.process = name
			entry.pid = pid
		}
		entries = append(entries, entry)
	}
	return entries, problems
}

func parseLinuxNetstatOutput(output string) ([]hostNetEntry, []string) {
	entries := make([]hostNetEntry, 0, 64)
	problems := make([]string, 0)

	for index, rawLine := range strings.Split(output, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(rawLine, "\r"))
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		protocol := normalizeHostProtocol(fields[0])
		if protocol == "" {
			continue
		}
		localAddress, localPort, ok := parseHostPort(fields[3])
		if !ok {
			problems = append(problems, fmt.Sprintf("解析 netstat 第 %d 行失败：%s",
				index+1, truncateHostText(line, 120)))
			continue
		}

		entry := hostNetEntry{
			protocol:     protocol,
			localAddress: localAddress,
			localPort:    localPort,
		}
		if remoteAddress, remotePort, remoteOK := parseHostPort(fields[4]); remoteOK {
			entry.remoteAddress = remoteAddress
			entry.remotePort = remotePort
		}

		processField := fields[len(fields)-1]
		if protocol == "TCP" {
			if len(fields) >= 7 || (len(fields) == 6 && !strings.Contains(fields[5], "/")) {
				entry.state = strings.ToUpper(fields[5])
			}
		} else {
			entry.state = "UDP"
			if len(fields) >= 7 && strings.Contains(fields[6], "/") {
				entry.state = strings.ToUpper(fields[5])
			}
		}
		if name, pid, processOK := parseLinuxProcess(processField); processOK {
			entry.process = name
			entry.pid = pid
		}
		entries = append(entries, entry)
	}
	return entries, problems
}

func parseLinuxProcess(field string) (string, int, bool) {
	if matches := linuxProcessPattern.FindStringSubmatch(field); len(matches) >= 3 {
		pid, err := strconv.Atoi(matches[2])
		if err == nil && pid > 0 {
			return matches[1], pid, true
		}
	}

	// netstat 的 pid/程序名 形式。
	field = strings.TrimSpace(field)
	if index := strings.IndexByte(field, '/'); index > 0 {
		pid, err := strconv.Atoi(field[:index])
		if err == nil && pid > 0 {
			return strings.TrimSpace(field[index+1:]), pid, true
		}
	}
	return "", 0, false
}

func normalizeHostProtocol(value string) string {
	lower := strings.ToLower(strings.TrimSpace(value))
	switch {
	case strings.HasPrefix(lower, "tcp"):
		return "TCP"
	case strings.HasPrefix(lower, "udp"):
		return "UDP"
	default:
		return ""
	}
}

// ---------------------------------------------------------------------------
// 结果构建与分类
// ---------------------------------------------------------------------------

func buildHostPorts(entries []hostNetEntry) []models.HostPort {
	ports := make([]models.HostPort, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))

	for _, entry := range entries {
		if entry.localPort <= 0 || !isListeningEntry(entry) {
			continue
		}
		key := fmt.Sprintf("%s|%s|%d|%d", entry.protocol, entry.localAddress, entry.localPort, entry.pid)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		ports = append(ports, models.HostPort{
			Protocol:     entry.protocol,
			LocalAddress: entry.localAddress,
			LocalPort:    entry.localPort,
			Process:      entry.process,
			PID:          entry.pid,
			Exposed:      isExposedAddress(entry.localAddress),
		})
	}

	sort.SliceStable(ports, func(i, j int) bool {
		if ports[i].LocalPort != ports[j].LocalPort {
			return ports[i].LocalPort < ports[j].LocalPort
		}
		if ports[i].Protocol != ports[j].Protocol {
			return ports[i].Protocol < ports[j].Protocol
		}
		if ports[i].LocalAddress != ports[j].LocalAddress {
			return ports[i].LocalAddress < ports[j].LocalAddress
		}
		return ports[i].PID < ports[j].PID
	})
	return ports
}

func isListeningEntry(entry hostNetEntry) bool {
	switch entry.protocol {
	case "TCP":
		state := strings.ToUpper(strings.TrimSpace(entry.state))
		return state == "LISTEN" || state == "LISTENING"
	case "UDP":
		// 已连接 UDP 套接字有真实对端，归入连接而不是监听端口。
		if entry.remotePort > 0 && !isWildcardHost(entry.remoteAddress) {
			return false
		}
		state := strings.ToUpper(strings.TrimSpace(entry.state))
		return state == "" || state == "UDP" || state == "UNCONN"
	default:
		return false
	}
}

func isExposedAddress(host string) bool {
	host = strings.TrimSpace(host)
	if host == "" || host == "*" {
		return true
	}
	ip := parseHostIP(host)
	return ip != nil && ip.IsUnspecified()
}

func buildHostConnections(entries []hostNetEntry) []models.HostConnection {
	connections := make([]models.HostConnection, 0, len(entries))
	for _, entry := range entries {
		if entry.remotePort <= 0 || entry.remoteAddress == "" {
			continue
		}
		if isWildcardHost(entry.remoteAddress) {
			continue
		}
		connections = append(connections, models.HostConnection{
			Protocol:      entry.protocol,
			LocalAddress:  entry.localAddress,
			LocalPort:     entry.localPort,
			RemoteAddress: entry.remoteAddress,
			RemotePort:    entry.remotePort,
			State:         entry.state,
			Process:       entry.process,
			PID:           entry.pid,
			External:      isExternalIP(entry.remoteAddress),
		})
	}

	// 外联优先展示，其次按远端地址与端口排序。
	sort.SliceStable(connections, func(i, j int) bool {
		if connections[i].External != connections[j].External {
			return connections[i].External
		}
		if connections[i].RemoteAddress != connections[j].RemoteAddress {
			return connections[i].RemoteAddress < connections[j].RemoteAddress
		}
		if connections[i].RemotePort != connections[j].RemotePort {
			return connections[i].RemotePort < connections[j].RemotePort
		}
		if connections[i].LocalPort != connections[j].LocalPort {
			return connections[i].LocalPort < connections[j].LocalPort
		}
		if connections[i].Protocol != connections[j].Protocol {
			return connections[i].Protocol < connections[j].Protocol
		}
		return connections[i].PID < connections[j].PID
	})
	return connections
}

func collectExternalIPs(connections []models.HostConnection) []string {
	seen := make(map[string]struct{}, 16)
	ips := make([]string, 0, 16)
	for _, connection := range connections {
		if !connection.External {
			continue
		}
		address := strings.TrimSpace(connection.RemoteAddress)
		if address == "" {
			continue
		}
		if _, exists := seen[address]; exists {
			continue
		}
		seen[address] = struct{}{}
		ips = append(ips, address)
		if len(ips) >= maxExternalIPs {
			break
		}
	}
	sort.Strings(ips)
	return ips
}

func isWildcardHost(host string) bool {
	host = strings.TrimSpace(host)
	if host == "" || host == "*" {
		return true
	}
	ip := parseHostIP(host)
	return ip != nil && ip.IsUnspecified()
}

// isExternalIP 判断远端地址是否属于公网（排除回环、内网、链路本地、未指定与组播）。
func isExternalIP(host string) bool {
	ip := parseHostIP(host)
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() || ip.IsMulticast() {
		return false
	}
	return true
}

func parseHostIP(host string) net.IP {
	host = strings.TrimSpace(host)
	if zone := strings.LastIndexByte(host, '%'); zone >= 0 {
		host = host[:zone]
	}
	return net.ParseIP(host)
}

func truncateHostText(value string, max int) string {
	if max <= 0 {
		return ""
	}
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max]) + "…"
}
