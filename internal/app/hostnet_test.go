package app

import (
	"testing"
)

const windowsNetstatSample = `Active Connections

  Proto  Local Address          Foreign Address        State           PID
  TCP    0.0.0.0:135            0.0.0.0:0              LISTENING       1234
  TCP    192.168.1.10:52344     93.184.216.34:443      ESTABLISHED     4321
  TCP    192.168.1.10:52345     192.168.1.20:445       ESTABLISHED     4322
  TCP    [::]:445               [::]:0                 LISTENING       1234
  UDP    0.0.0.0:500            *:*                                    5678
  UDP    [::1]:1900             *:*                                    8765
`

func TestParseWindowsNetstatOutput(t *testing.T) {
	entries, problems := parseWindowsNetstatOutput(windowsNetstatSample)
	if len(problems) != 0 {
		t.Fatalf("unexpected parse problems: %v", problems)
	}
	if len(entries) != 6 {
		t.Fatalf("expected 6 entries, got %d: %+v", len(entries), entries)
	}

	var tcpV6, udpV4, udpV6 *hostNetEntry
	for i := range entries {
		entry := &entries[i]
		switch {
		case entry.protocol == "TCP" && entry.localPort == 445:
			tcpV6 = entry
		case entry.protocol == "UDP" && entry.localPort == 500:
			udpV4 = entry
		case entry.protocol == "UDP" && entry.localPort == 1900:
			udpV6 = entry
		}
	}
	if tcpV6 == nil || tcpV6.localAddress != "::" || tcpV6.state != "LISTENING" || tcpV6.pid != 1234 {
		t.Fatalf("unexpected IPv6 TCP entry: %+v", tcpV6)
	}
	if udpV4 == nil || udpV4.state != "UDP" || udpV4.remotePort != 0 || udpV4.pid != 5678 {
		t.Fatalf("unexpected UDP entry: %+v", udpV4)
	}
	if udpV6 == nil || udpV6.localAddress != "::1" || udpV6.localPort != 1900 || udpV6.pid != 8765 {
		t.Fatalf("unexpected IPv6 UDP entry: %+v", udpV6)
	}

	ports := buildHostPorts(entries)
	if len(ports) != 4 {
		t.Fatalf("expected 4 listening ports, got %d: %+v", len(ports), ports)
	}
	for _, port := range ports {
		switch port.LocalPort {
		case 135, 445, 500:
			if !port.Exposed {
				t.Fatalf("expected port %d to be exposed: %+v", port.LocalPort, port)
			}
		case 1900:
			if port.Exposed {
				t.Fatalf("loopback UDP port should not be exposed: %+v", port)
			}
		default:
			t.Fatalf("unexpected listening port: %+v", port)
		}
	}
	if ports[0].LocalPort != 135 || ports[len(ports)-1].LocalPort != 1900 {
		t.Fatalf("ports are not sorted by port: %+v", ports)
	}
}

func TestBuildHostConnections(t *testing.T) {
	entries := []hostNetEntry{
		{
			protocol: "TCP", localAddress: "192.168.1.10", localPort: 52345,
			remoteAddress: "192.168.1.20", remotePort: 445, state: "ESTABLISHED", pid: 4322,
		},
		{
			protocol: "TCP", localAddress: "192.168.1.10", localPort: 52344,
			remoteAddress: "93.184.216.34", remotePort: 443, state: "ESTABLISHED", pid: 4321,
		},
		{
			protocol: "UDP", localAddress: "0.0.0.0", localPort: 500,
			remoteAddress: "", remotePort: 0, state: "UDP", pid: 5678,
		},
	}

	connections := buildHostConnections(entries)
	if len(connections) != 2 {
		t.Fatalf("expected 2 connections, got %d: %+v", len(connections), connections)
	}
	if !connections[0].External || connections[0].RemoteAddress != "93.184.216.34" {
		t.Fatalf("expected external connection first, got %+v", connections[0])
	}
	if connections[1].External || connections[1].RemoteAddress != "192.168.1.20" {
		t.Fatalf("expected private connection second, got %+v", connections[1])
	}

	externalIPs := collectExternalIPs(connections)
	if len(externalIPs) != 1 || externalIPs[0] != "93.184.216.34" {
		t.Fatalf("unexpected external IPs: %+v", externalIPs)
	}
}

func TestIsExternalIP(t *testing.T) {
	cases := map[string]bool{
		"93.184.216.34":        true,
		"8.8.8.8":              true,
		"2001:4860:4860::8888": true,
		"192.168.1.10":         false,
		"10.0.0.1":             false,
		"172.16.0.1":           false,
		"127.0.0.1":            false,
		"::1":                  false,
		"fe80::1%eth0":         false,
		"169.254.10.10":        false,
		"0.0.0.0":              false,
		"::":                   false,
		"224.0.0.1":            false,
		"ff02::1":              false,
		"not-an-ip":            false,
		"":                     false,
	}
	for host, want := range cases {
		if got := isExternalIP(host); got != want {
			t.Errorf("isExternalIP(%q)=%v, want %v", host, got, want)
		}
	}
}

func TestParseHostPort(t *testing.T) {
	cases := []struct {
		value string
		host  string
		port  int
		ok    bool
	}{
		{"0.0.0.0:135", "0.0.0.0", 135, true},
		{"[::]:445", "::", 445, true},
		{"[::1]:1900", "::1", 1900, true},
		{"*:22", "*", 22, true},
		{"0.0.0.0:*", "0.0.0.0", 0, true},
		{"*:*", "", 0, false},
		{"garbage", "", 0, false},
	}
	for _, tc := range cases {
		host, port, ok := parseHostPort(tc.value)
		if host != tc.host || port != tc.port || ok != tc.ok {
			t.Errorf("parseHostPort(%q)=(%q,%d,%v), want (%q,%d,%v)",
				tc.value, host, port, ok, tc.host, tc.port, tc.ok)
		}
	}
}

func TestParseLinuxSSOutput(t *testing.T) {
	output := `tcp   LISTEN 0      4096   0.0.0.0:22    0.0.0.0:*    users:(("sshd",pid=1234,fd=3))
udp   UNCONN 0      0      0.0.0.0:68    0.0.0.0:*    users:(("dhclient",pid=567,fd=6))
tcp   ESTAB  0      0      10.0.0.5:22   203.0.113.9:54321 users:(("sshd",pid=1234,fd=4))
`
	entries, problems := parseLinuxSSOutput(output)
	if len(problems) != 0 {
		t.Fatalf("unexpected parse problems: %v", problems)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d: %+v", len(entries), entries)
	}
	if entries[0].process != "sshd" || entries[0].pid != 1234 || entries[0].state != "LISTEN" {
		t.Fatalf("unexpected sshd entry: %+v", entries[0])
	}
	ports := buildHostPorts(entries)
	if len(ports) != 2 {
		t.Fatalf("expected 2 listening ports, got %d: %+v", len(ports), ports)
	}
	connections := buildHostConnections(entries)
	if len(connections) != 1 || !connections[0].External || connections[0].RemotePort != 54321 {
		t.Fatalf("unexpected connections: %+v", connections)
	}
}

func TestParseTasklistCSV(t *testing.T) {
	output := "\"System Idle Process\",\"0\",\"Services\",\"0\",\"8 K\"\r\n" +
		"\"chrome.exe\",\"4321\",\"Console\",\"1\",\"120,000 K\"\r\n"
	names := parseTasklistCSV(output)
	if len(names) != 1 || names[4321] != "chrome.exe" {
		t.Fatalf("unexpected process map: %+v", names)
	}
}
