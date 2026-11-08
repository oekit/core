package network

import (
	"errors"
	"log/slog"
	"net"
	"os"
	"strings"
)

// ipv6SpecialCIDRs 是 IANA「IPv6 Special-Purpose Address Registry」里的
// 特殊用途段（core#73）：它们不是普通全球单播，不该被 IsPublicIP 当公网放行。
// 口径对齐同生态 pkg/urlguard 的 blockedCIDRs——出站目标判定（urlguard）拒的段，
// 「是否公网」判定（IsPublicIP）同样拒。清单固定在编译期，不在运行期增删。
var ipv6SpecialCIDRs = []*net.IPNet{
	{IP: net.ParseIP("::"), Mask: net.CIDRMask(96, 128)},          // IPv4-compatible ::/96（RFC 4291 已废止；::127.0.0.1 这类写法 To4() 返 nil，绕过回环门禁）
	{IP: net.ParseIP("64:ff9b::"), Mask: net.CIDRMask(96, 128)},   // NAT64 well-known 64:ff9b::/96（RFC 6052；可内嵌 IPv4，含 127.0.0.1）
	{IP: net.ParseIP("64:ff9b:1::"), Mask: net.CIDRMask(48, 128)}, // NAT64 local-use 64:ff9b:1::/48（RFC 8215）
	{IP: net.ParseIP("100::"), Mask: net.CIDRMask(64, 128)},       // discard-only 100::/64（RFC 6666）
	{IP: net.ParseIP("100:0:0:1::"), Mask: net.CIDRMask(64, 128)}, // dummy 100:0:0:1::/64（RFC 9780）
	{IP: net.ParseIP("2001::"), Mask: net.CIDRMask(32, 128)},      // Teredo 2001::/32（RFC 4380）
	{IP: net.ParseIP("2001:2::"), Mask: net.CIDRMask(48, 128)},    // benchmarking 2001:2::/48（RFC 5180）
	{IP: net.ParseIP("2001:10::"), Mask: net.CIDRMask(28, 128)},   // ORCHID 2001:10::/28（已废止）
	{IP: net.ParseIP("2001:20::"), Mask: net.CIDRMask(28, 128)},   // ORCHIDv2 2001:20::/28（RFC 7343）
	{IP: net.ParseIP("2001:db8::"), Mask: net.CIDRMask(32, 128)},  // 文档段 2001:db8::/32（RFC 3849，IPv6 侧的 TEST-NET）
	{IP: net.ParseIP("2002::"), Mask: net.CIDRMask(16, 128)},      // 6to4 2002::/16（RFC 3056；可内嵌任意 IPv4）
	{IP: net.ParseIP("3fff::"), Mask: net.CIDRMask(20, 128)},      // 文档段 3fff::/20（RFC 9637）
	{IP: net.ParseIP("5f00::"), Mask: net.CIDRMask(16, 128)},      // SRv6 SID 5f00::/16（RFC 9602）
	{IP: net.ParseIP("fec0::"), Mask: net.CIDRMask(10, 128)},      // site-local fec0::/10（RFC 3879 已废止，存量内网仍在用；IsPrivate 只覆盖 fc00::/7、IsLinkLocalUnicast 只覆盖 fe80::/10，都拦不到它）
}

// IsPublicIP 判断 IP 是否是公网（全球可路由单播）IP。
//
// 判定统一走 net.IP 的标准方法，不做字符串前缀匹配：
//   - 两族通用拒绝：loopback、link-local、组播（IsMulticast 覆盖 IPv4 224/4
//     与 IPv6 ff00::/8，含链路本地组播）、未指定地址、私网（IsPrivate：10/8、
//     172.16/12、192.168/16 及 IPv6 ULA fc00::/7）；
//   - IPv4 额外拒绝特殊用途段：0.0.0.0/8（RFC 1122 "this network"，
//     core#98——IsUnspecified 只覆盖 0.0.0.0 单个地址）、CGNAT 100.64/10、
//     192.0.0.0/24、基准测试段 198.18/15、TEST-NET 192.0.2.0/24 /
//     198.51.100.0/24 / 203.0.113.0/24、保留段 240/4（含广播
//     255.255.255.255）；
//   - IPv6 额外拒绝 IANA 特殊用途段（见 ipv6SpecialCIDRs）：文档段
//     2001:db8::/32 与 3fff::/20、discard-only 100::/64、dummy
//     100:0:0:1::/64、benchmarking 2001:2::/48、ORCHID 2001:10::/28 与
//     2001:20::/28、Teredo 2001::/32、6to4 2002::/16、NAT64 64:ff9b::/96
//     与 64:ff9b:1::/48、SRv6 5f00::/16、site-local fec0::/10（RFC 3879
//     已废止，core#98）、IPv4-compatible ::/96——此前 To4() 对 IPv6 返
//     nil，整个 IPv6 绕过段判定直接放行（core#73）；
//   - 其余 IPv4 与 **IPv6 全球单播地址**（如 2606:4700::1）返回 true——
//     此前实现所有 IPv6 一律 false（core#53）。
func IsPublicIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsMulticast() ||
		ip.IsUnspecified() || ip.IsPrivate() {
		return false
	}
	if ip4 := ip.To4(); ip4 != nil {
		switch {
		case ip4[0] == 0: // "this network" 0.0.0.0/8（RFC 1122；IsUnspecified 只覆盖 0.0.0.0 本身，0.1.2.3 这类同段非零地址要靠这里拦，core#98）
			return false
		case ip4[0] == 100 && ip4[1]&0xC0 == 64: // CGNAT 100.64.0.0/10
			return false
		case ip4[0] == 192 && ip4[1] == 0 && ip4[2] == 0: // 192.0.0.0/24
			return false
		case ip4[0] == 192 && ip4[1] == 0 && ip4[2] == 2: // TEST-NET-1 192.0.2.0/24
			return false
		case ip4[0] == 198 && (ip4[1] == 18 || ip4[1] == 19): // 基准测试段 198.18.0.0/15
			return false
		case ip4[0] == 198 && ip4[1] == 51 && ip4[2] == 100: // TEST-NET-2 198.51.100.0/24
			return false
		case ip4[0] == 203 && ip4[1] == 0 && ip4[2] == 113: // TEST-NET-3 203.0.113.0/24
			return false
		case ip4[0]&0xF0 == 0xF0: // 保留段 240.0.0.0/4（含 255.255.255.255）
			return false
		}
	} else {
		// To4() == nil 的纯 IPv6：过 IANA 特殊用途段清单，命中即非公网。
		// 注意 To4() 只认 ::ffff: 映射形式，::127.0.0.1 这类 IPv4-compatible
		// 写法会走到这里，由 ::/96 段拦下。
		for _, cidr := range ipv6SpecialCIDRs {
			if cidr.Contains(ip) {
				return false
			}
		}
	}
	return true
}

// IsIPv4 判断是否是 IPv4 地址。
func IsIPv4(ip net.IP) bool {
	return ip.To4() != nil
}

// GetLanIPs 获取局域网本地 IP 地址列表。
func GetLanIPs() ([]string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}

	var ips []string
	for _, address := range addrs {
		if ipnet, ok := address.(*net.IPNet); ok &&
			!ipnet.IP.IsLoopback() &&
			!ipnet.IP.IsLinkLocalMulticast() &&
			!ipnet.IP.IsLinkLocalUnicast() &&
			IsIPv4(ipnet.IP) &&
			!IsPublicIP(ipnet.IP) {
			ips = append(ips, ipnet.IP.String())
		}
	}
	return ips, nil
}

// HostName 返回主机名。
func HostName() string {
	host, _ := os.Hostname()
	return host
}

// ServerIPString 返回所有服务器 IP 的逗号分隔字符串。
func ServerIPString() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}

	var ips []string
	for _, address := range addrs {
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				ips = append(ips, ipnet.IP.String())
			}
		}
	}
	return strings.Join(ips, ",")
}

// ServerIP 返回服务器的外网 IP 和内网 IP。
//
// 内外网判定不再做字符串前缀匹配——旧实现把 172.16/12 私网判成外网
// （纯内网部署下注册中心拿到的地址是错的），又把全部 192.x.x.x 判成内网
// （TEST-NET 等公网段遭殃），`127.` 分支更是 loopback 过滤后的死代码
// （core#54）。现统一走 net.IP 标准方法（见 classifyServerIP），
// IPv4 与 IPv6 同一条判定线。
func ServerIP() (externalIP, internalIP string) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", ""
	}

	ips := make([]net.IP, 0, len(addrs))
	for _, address := range addrs {
		if ipnet, ok := address.(*net.IPNet); ok {
			ips = append(ips, ipnet.IP)
		}
	}
	return classifyServerIP(ips)
}

// classifyServerIP 对一组接口 IP 做内外网归类，返回 (外网 IP, 内网 IP)；
// 同侧多个地址时后见者胜（与历史行为一致）。它是 ServerIP 的纯函数核心，
// 便于脱离真实网卡测试。
//
// 内网 = net.IP.IsPrivate（10/8、172.16/12、192.168/16 及 IPv6 ULA fc00::/7）；
// 其余可路由单播 = 外网。loopback、link-local、组播与未指定地址不参与归类：
// 它们既不该被上报为外网，也不该算内网。
func classifyServerIP(ips []net.IP) (externalIP, internalIP string) {
	for _, ip := range ips {
		if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() ||
			ip.IsMulticast() || ip.IsUnspecified() {
			continue
		}
		if ip.IsPrivate() {
			internalIP = ip.String()
		} else {
			externalIP = ip.String()
		}
	}
	return externalIP, internalIP
}

// GetServerIP 返回服务器 IP，优先返回外网 IP。
func GetServerIP() string {
	eIP, iIP := ServerIP()
	if eIP != "" {
		return eIP
	}
	return iIP
}

// GetLocalIP 返回本机首选非回环 IPv4 地址。
// 优先读取 POD_IP 环境变量（K8s 注入），其次遍历网卡获取第一个非回环 IPv4。
// 未找到时返回错误。这是 ekit 中获取本机 IP 的唯一实现，
// uidgen、nacos、registry 等包应委托至此，避免逻辑重复。
//
// POD_IP 经 net.ParseIP 校验且必须是 IPv4（本函数契约是「非回环 IPv4」）：
// 注入串写错（not-an-ip、带端口的 1.2.3.4:8080、IPv6）不再原样返回
// （core#87 此前 err=nil 且错误地址直达注册面，与同文件其余路径的
// fail-closed 口径相反）——非法值打 WARN 后忽略，回退网卡探测。
func GetLocalIP() (string, error) {
	if ip := os.Getenv("POD_IP"); ip != "" {
		if p := net.ParseIP(ip); p != nil && p.To4() != nil {
			return ip, nil
		}
		// 非法/非 IPv4：不中断（拿不到 IP 的代价是注册不可达，回退探测
		// 仍有救），但必须可观测——静默忽略等于把「注入错了」藏起来。
		slog.Warn("network: POD_IP 不是合法 IPv4，忽略并回退网卡探测",
			"component", "network.podip", "pod_ip", ip)
	}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() && ipNet.IP.To4() != nil {
			return ipNet.IP.String(), nil
		}
	}
	return "", errors.New("network: no suitable local IP found")
}

// GetPodIP 返回本机 IP 地址（无 error 版本，适合无法处理 error 的注册场景）。
// 优先读取 POD_IP 环境变量，其次遍历网卡；失败时回退到 127.0.0.1。
// 回退**不再是静默的**（oekit#141）：ERROR 级日志可观测——回环地址对其他
// 节点不可拨号，以它注册出去的实例永远无法被拨通；注册侧防线（如
// registry.ParseAddr）已按 fail-closed 拒绝回环地址，这里的 ERROR 是
// 「注册为什么被拒」的第一现场（IPv6-only 环境、POD_IP 未注入、多网卡
// 取不到 IPv4 都会走到这里）。
// 这是 GetLocalIP 的便捷封装，uidgen、nacos、registry 应委托至此。
func GetPodIP() string {
	ip, err := GetLocalIP()
	if err != nil {
		return podIPFallback(err)
	}
	return ip
}

// podIPFallback 是 GetPodIP 的回退路径（独立成函数便于测试）：打 ERROR 后
// 返回回环地址。返回值保持历史行为不变——调用方若拿它去注册，会被注册侧
// 的回环拒绝（registry#64）拦下；本函数的价值是让「为什么」可见。
func podIPFallback(err error) string {
	slog.Error("network: GetLocalIP 失败，GetPodIP 回退 127.0.0.1——回环地址不可被其他节点拨号，注册侧应拒绝（oekit#141）",
		"component", "network.podip", "error", err)
	return "127.0.0.1"
}
