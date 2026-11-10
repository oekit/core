package network

import (
	"bytes"
	"errors"
	"log/slog"
	"net"
	"strings"
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
		// ---- core#98：0.0.0.0/8 与 fec0::/10 全段拒绝 ----
		{"this network 0.0.0.0/8 non-zero", net.ParseIP("0.1.2.3"), false},
		{"this network 0.0.0.0/8 upper edge", net.ParseIP("0.255.255.255"), false},
		{"public control 1.1.1.1", net.ParseIP("1.1.1.1"), true},
		{"ipv6 site-local fec0::/10", net.ParseIP("fec0::1"), false},
		{"ipv6 site-local fec0::/10 upper edge", net.ParseIP("feff:ffff::1"), false},
		{"ipv6 ula fc00::/7 lower half", net.ParseIP("fc00::1"), false},
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

// TestIsPublicIPIssue98Samples 钉住 core#98 报告的两类误判地址与验收负对照。
// 0.1.2.3（0.0.0.0/8 内非零，IsUnspecified 抓不到）与 fec0::1（site-local，
// IsPrivate/IsLinkLocalUnicast 都覆盖不到）此前被误判公网，与同生态
// urlguard 的阻断结论矛盾。负对照钉住「不能靠收紧到整体拒绝蒙过去」：
// 真公网地址（1.1.1.1、2606:4700::1）必须仍放行（core#53 老教训），
// 既有拒绝行为（0.0.0.0、fe80::1、fc00::1、10.0.0.1）不回归。
// 变异性负对照由表驱动保证：还原两处补丁，0.1.2.3 / fec0::1 两条即转红。
func TestIsPublicIPIssue98Samples(t *testing.T) {
	tests := []struct {
		name string
		ip   string
		want bool
	}{
		// core#98 现象复现（issue 表格两行）
		{"this network 0.1.2.3", "0.1.2.3", false},
		{"site-local fec0::1", "fec0::1", false},
		// 负对照：真公网仍放行
		{"public control 1.1.1.1", "1.1.1.1", true},
		{"ipv6 public control 2606:4700::1", "2606:4700::1", true},
		// 负对照：既有拒绝行为不回归（issue 验收第 3 条）
		{"unspecified 0.0.0.0", "0.0.0.0", false},
		{"link local fe80::1", "fe80::1", false},
		{"ula fc00::1", "fc00::1", false},
		{"private 10.0.0.1", "10.0.0.1", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("net.ParseIP(%q) = nil, 前提不成立", tt.ip)
			}
			if got := IsPublicIP(ip); got != tt.want {
				t.Errorf("IsPublicIP(%v) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

// TestIsPublicIPIssue77Samples 钉住 core#77 报告的样本地址清单。#77 是
// core#73（PR #74，v0.13.2）的重报单：它列的地址全部落在 #74 已补的
// IANA IPv6 特殊用途段清单内，本测试逐个复跑收口——全部拒绝即证据。
func TestIsPublicIPIssue77Samples(t *testing.T) {
	tests := []struct {
		name string
		ip   net.IP
		want bool
	}{
		// NAT64 well-known 前缀内嵌回环/私网（issue 表格第 1、2 行）
		{"nat64 embedded loopback 64:ff9b::7f00:1", net.ParseIP("64:ff9b::7f00:1"), false},
		{"nat64 embedded private 64:ff9b::a00:1", net.ParseIP("64:ff9b::a00:1"), false},
		// 6to4 内嵌回环（issue 表格第 3 行）
		{"6to4 embedded loopback 2002:7f00:1::1", net.ParseIP("2002:7f00:1::1"), false},
		// 文档段与 discard-only（issue 表格第 4、5 行）
		{"doc 2001:db8::1", net.ParseIP("2001:db8::1"), false},
		{"discard-only 100::1", net.ParseIP("100::1"), false},
		// 真·全球单播放行不受影响（issue 表格第 6 行）
		{"global unicast 2606:4700::1", net.ParseIP("2606:4700::1"), true},
		// 回环与 IPv4-mapped 回环（issue 表格第 7、8 行）
		{"loopback 127.0.0.1", net.ParseIP("127.0.0.1"), false},
		{"ipv4-mapped loopback ::ffff:127.0.0.1", net.ParseIP("::ffff:127.0.0.1"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsPublicIP(tt.ip); got != tt.want {
				t.Errorf("IsPublicIP(%v) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

// TestGetLocalIPPODIPValidation 钉住 core#87：POD_IP 非法值不再原样返回。
// 合法 IPv4 透传；not-an-ip / 带端口 / IPv6 一律忽略并回退网卡探测。
func TestGetLocalIPPODIPValidation(t *testing.T) {
	// 合法 IPv4：原样透传
	t.Setenv("POD_IP", "10.1.2.3")
	ip, err := GetLocalIP()
	if err != nil || ip != "10.1.2.3" {
		t.Errorf("GetLocalIP() = %q, %v; want 10.1.2.3, nil", ip, err)
	}

	// 非法形态：忽略并回退——绝不能把注入串带出去
	for _, bad := range []string{"not-an-ip", "1.2.3.4:8080", "fd00::1", "999.1.2.3", " "} {
		t.Run("bad:"+bad, func(t *testing.T) {
			t.Setenv("POD_IP", bad)
			ip, err := GetLocalIP()
			if err == nil && ip == bad {
				t.Errorf("GetLocalIP() 原样返回了非法 POD_IP %q（err=nil）", ip)
			}
			// 回退路径：要么拿到网卡 IPv4，要么报「找不到」的错——两者都不是注入串
			if err == nil && ip == "" {
				t.Error("GetLocalIP() = empty string with nil error")
			}
		})
	}
}

// TestGetLocalIPPODIPRejectsLoopbackAndUnspecified 回归（core#120）：POD_IP 的
// 校验不能只判「是 IPv4」——127.0.0.1 / 0.0.0.0 形状合法但语义非法（回环地址
// 不可被其他节点拨号，podIPFallback 对回环打 ERROR 级日志、注册侧 fail-closed
// 拒绝；0.0.0.0 是未指定地址），core#87 的判据（ParseIP + To4）把它们原样放行
// 且零日志，与同文件口径相反。语义非法必须与形状非法同路：WARN 后回退网卡探测。
// 断言三层：绝不原样返回 / 回退结果是非回环 IPv4 或走 errNoLocalIP 明确报错 /
// WARN 留痕且携带注入值（pod_ip 属性是「注入错了」的第一现场）。v4-mapped
// 写法（::ffff:x.x.x.x）与点分十进制同判——同一判据两种书写形态。
func TestGetLocalIPPODIPRejectsLoopbackAndUnspecified(t *testing.T) {
	bads := []string{
		"127.0.0.1", "127.0.0.53", // 回环 127/8（IsLoopback 判前缀）
		"0.0.0.0",                            // 未指定（IsUnspecified）
		"::ffff:127.0.0.1", "::ffff:0.0.0.0", // 上两者的 v4-mapped 写法
	}
	for _, bad := range bads {
		t.Run(bad, func(t *testing.T) {
			var buf bytes.Buffer
			prev := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
			defer slog.SetDefault(prev)

			t.Setenv("POD_IP", bad)
			ip, err := GetLocalIP()
			if err == nil && ip == bad {
				t.Fatalf("GetLocalIP() 原样返回了语义非法的 POD_IP %q（err=nil）", ip)
			}
			if err == nil {
				p := net.ParseIP(ip)
				if p == nil || p.To4() == nil || p.IsLoopback() || p.IsUnspecified() {
					t.Errorf("GetLocalIP() 回退结果 %q 不是非回环 IPv4", ip)
				}
			} else if !errors.Is(err, errNoLocalIP) {
				t.Errorf("GetLocalIP() 错误应是网卡探测失败哨兵 errNoLocalIP，得到 %v", err)
			}

			out := buf.String()
			if !strings.Contains(out, "level=WARN") {
				t.Fatalf("语义非法 POD_IP 必须打 WARN 后回退，实际日志：\n%s", out)
			}
			for _, want := range []string{"network.podip", "不是合法的非回环 IPv4"} {
				if !strings.Contains(out, want) {
					t.Errorf("WARN 日志应包含 %q（观测口径与判据一致），实际日志：\n%s", want, out)
				}
			}
			// WARN 必须携带注入值原文（strings.Cut 取 pod_ip 属性）——运维靠它
			// 定位「K8s 模板注入错了」，少了它只剩一句文案无从下手。
			_, rest, ok := strings.Cut(out, "pod_ip=")
			if !ok {
				t.Fatalf("WARN 日志缺 pod_ip 属性，实际日志：\n%s", out)
			}
			line, _, _ := strings.Cut(rest, "\n")
			got, _, _ := strings.Cut(line, " ")
			if v := strings.Trim(got, `"`); v != bad {
				t.Errorf("WARN 的 pod_ip 属性 = %q, want %q", v, bad)
			}

			// GetPodIP 是 uidgen/nacos/registry 的委托入口：同样不得静默放行
			//（拿不到网卡地址时经 podIPFallback 返回 127.0.0.1 但必须有 ERROR）。
			if got := GetPodIP(); got == bad && !strings.Contains(out, "level=ERROR") {
				t.Errorf("GetPodIP() 静默返回了语义非法的 POD_IP %q（无 ERROR 级回退日志）", got)
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
