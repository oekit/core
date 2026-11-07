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
		// ---- core#73：IANA IPv6 Special-Purpose 段全部拒绝（issue 表格 13 例） ----
		{"ipv6 doc 2001:db8::/32", net.ParseIP("2001:db8::1"), false},
		{"ipv6 doc 3fff::/20", net.ParseIP("3fff::1"), false},
		{"ipv6 discard-only 100::/64", net.ParseIP("100::1"), false},
		{"ipv6 dummy 100:0:0:1::/64", net.ParseIP("100:0:0:1::1"), false},
		{"ipv6 benchmarking 2001:2::/48", net.ParseIP("2001:2::1"), false},
		{"ipv6 srv6 5f00::/16", net.ParseIP("5f00::1"), false},
		{"ipv6 orchid 2001:10::/28", net.ParseIP("2001:10::1"), false},
		{"ipv6 orchidv2 2001:20::/28", net.ParseIP("2001:20::1"), false},
		{"ipv6 teredo 2001::/32", net.ParseIP("2001::1"), false},
		{"ipv6 6to4 2002::/16", net.ParseIP("2002::1"), false},
		{"ipv6 nat64 well-known 64:ff9b::/96", net.ParseIP("64:ff9b::7f00:1"), false},
		{"ipv6 nat64 local-use 64:ff9b:1::/48", net.ParseIP("64:ff9b:1::7f00:1"), false},
		{"ipv4-compatible loopback ::127.0.0.1", net.ParseIP("::127.0.0.1"), false},
		// 整段拒绝，非个别地址（issue 实测过的上下边界）
		{"ipv6 doc 2001:db8::/32 lower edge", net.ParseIP("2001:db8::"), false},
		{"ipv6 doc 2001:db8::/32 upper edge", net.ParseIP("2001:db8:ffff:ffff:ffff:ffff:ffff:ffff"), false},
		{"ipv6 srv6 5f00::/16 upper edge", net.ParseIP("5f00:ffff::1"), false},
		// 正常公网 IPv6 不受影响
		{"ipv6 public 2606:4700::1111", net.ParseIP("2606:4700::1111"), true},
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

// TestIsPublicIPIPv4CompatibleLoopback 钉住 core#73 最硬的绕过路径：
// ::127.0.0.1 是回环的 IPv4-compatible 写法，net.IP.To4() 对它返回 nil
// （只认 ::ffff: 映射形式）、IsLoopback 抓不到，此前整个绕过判定被当公网放行。
// 先断言前提（能解析、To4 确为 nil），再断言 IsPublicIP 拒绝——避免
// ParseIP 失败导致的假绿。
func TestIsPublicIPIPv4CompatibleLoopback(t *testing.T) {
	ip := net.ParseIP("::127.0.0.1")
	if ip == nil {
		t.Fatal(`net.ParseIP("::127.0.0.1") = nil, 前提不成立`)
	}
	if v4 := ip.To4(); v4 != nil {
		t.Fatalf("::127.0.0.1 To4() = %v, want nil", v4)
	}
	if IsPublicIP(ip) {
		t.Error("IsPublicIP(::127.0.0.1) = true, want false")
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
