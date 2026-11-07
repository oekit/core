package network

import (
	"net"
	"testing"
)

func TestIsPublicIP(t *testing.T) {
	tests := []struct {
		name string
		ip   net.IP
		want bool
	}{
		{"loopback", net.ParseIP("127.0.0.1"), false},
		{"private 10", net.ParseIP("10.0.0.1"), false},
		{"private 172", net.ParseIP("172.16.0.1"), false},
		{"private 192", net.ParseIP("192.168.1.1"), false},
		{"public", net.ParseIP("8.8.8.8"), true},
		// ---- core#53 回归矩阵 ----
		{"ipv6 global unicast", net.ParseIP("2606:4700::1"), true},
		{"ipv6 unique local", net.ParseIP("fd00::1"), false},
		{"ipv6 loopback", net.ParseIP("::1"), false},
		{"ipv6 link local", net.ParseIP("fe80::1"), false},
		{"ipv6 multicast", net.ParseIP("ff02::1"), false},
		{"ipv6 unspecified", net.ParseIP("::"), false},
		{"ipv4 multicast", net.ParseIP("224.0.0.1"), false},
		{"ipv4 broadcast", net.ParseIP("255.255.255.255"), false},
		{"cgnat 100.64/10 lower", net.ParseIP("100.64.0.1"), false},
		{"cgnat 100.64/10 upper", net.ParseIP("100.127.255.254"), false},
		{"public below cgnat", net.ParseIP("100.63.0.1"), true},
		{"reserved 240/4", net.ParseIP("240.1.2.3"), false},
		{"benchmark 198.18/15 lower", net.ParseIP("198.18.0.1"), false},
		{"benchmark 198.18/15 upper", net.ParseIP("198.19.255.1"), false},
		{"test-net-1 192.0.2/24", net.ParseIP("192.0.2.1"), false},
		{"test-net-2 198.51.100/24", net.ParseIP("198.51.100.1"), false},
		{"test-net-3 203.0.113/24", net.ParseIP("203.0.113.1"), false},
		{"special 192.0.0/24", net.ParseIP("192.0.0.1"), false},
		{"link local 169.254/16", net.ParseIP("169.254.1.1"), false},
		{"unspecified", net.ParseIP("0.0.0.0"), false},
		{"nil ip", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsPublicIP(tt.ip); got != tt.want {
				t.Errorf("IsPublicIP() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestClassifyServerIP 钉住 ServerIP 内外网归类的判定口径（core#54）：
// 172.16/12 私网不再被字符串前缀漏判成「外网」。
func TestClassifyServerIP(t *testing.T) {
	tests := []struct {
		name         string
		ips          []net.IP
		wantExternal string
		wantInternal string
	}{
		{"172.16/12 is internal (#54 回归)", []net.IP{net.ParseIP("172.16.0.5")}, "", "172.16.0.5"},
		{"172.31/12 upper edge", []net.IP{net.ParseIP("172.31.255.1")}, "", "172.31.255.1"},
		{"mixed external and internal", []net.IP{net.ParseIP("8.8.8.8"), net.ParseIP("10.0.0.1")}, "8.8.8.8", "10.0.0.1"},
		{"192.168/16 internal", []net.IP{net.ParseIP("192.168.1.1")}, "", "192.168.1.1"},
		{"192.0.2/24 public-range external", []net.IP{net.ParseIP("192.0.2.1")}, "192.0.2.1", ""},
		{"loopback skipped", []net.IP{net.ParseIP("127.0.0.1")}, "", ""},
		{"link local skipped", []net.IP{net.ParseIP("169.254.1.1")}, "", ""},
		{"ipv6 ula internal", []net.IP{net.ParseIP("fd00::1")}, "", "fd00::1"},
		{"ipv6 global external", []net.IP{net.ParseIP("2606:4700::1")}, "2606:4700::1", ""},
		{"ipv6 link local skipped", []net.IP{net.ParseIP("fe80::1")}, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ext, intn := classifyServerIP(tt.ips)
			if ext != tt.wantExternal || intn != tt.wantInternal {
				t.Errorf("classifyServerIP() = (%q, %q), want (%q, %q)", ext, intn, tt.wantExternal, tt.wantInternal)
			}
		})
	}
}

func TestIsIPv4(t *testing.T) {
	tests := []struct {
		name string
		ip   net.IP
		want bool
	}{
		{"ipv4", net.ParseIP("192.168.1.1"), true},
		{"ipv6", net.ParseIP("::1"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsIPv4(tt.ip); got != tt.want {
				t.Errorf("IsIPv4() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetLanIPs(t *testing.T) {
	ips, err := GetLanIPs()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("LAN IPs: %v", ips)
}

func TestHostName(t *testing.T) {
	host := HostName()
	if host == "" {
		t.Error("HostName() should not return empty")
	}
	t.Logf("Hostname: %s", host)
}

func TestServerIPString(t *testing.T) {
	result := ServerIPString()
	t.Logf("Server IPs: %s", result)
}

func TestServerIP(t *testing.T) {
	eIP, iIP := ServerIP()
	t.Logf("External IP: %s, Internal IP: %s", eIP, iIP)
}

func TestGetServerIP(t *testing.T) {
	ip := GetServerIP()
	t.Logf("Server IP: %s", ip)
}
