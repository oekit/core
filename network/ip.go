package network

import (
	"errors"
	"log/slog"
	"net"
	"os"
	"strings"

	"github.com/oekit/pkg/urlguard"
)

// IsPublicIP 判断 IP 是否是公网（全球可路由单播）IP。
//
// 判定统一走 net.IP 的标准方法 + pkg/urlguard 的特殊段清单，不做字符串
// 前缀匹配：
//   - 两族通用拒绝：loopback、link-local、组播（IsMulticast 覆盖 IPv4 224/4
//     与 IPv6 ff00::/8，含链路本地组播）、未指定地址、私网（IsPrivate：10/8、
//     172.16/12、192.168/16 及 IPv6 ULA fc00::/7）；
//   - **特殊用途段判定单源在 pkg/urlguard.IsBlockedIP**（core#132 治本）：
//     IANA 特殊段知识（IPv4 的 0.0.0.0/8、CGNAT 100.64/10、192.0.0.0/24、
//     基准测试 198.18/15、TEST-NET 三段、保留段 240/4；IPv6 的 IPv4-compatible
//     ::/96、NAT64 64:ff9b::/96 与 64:ff9b:1::/48、discard-only 100::/64、
//     dummy 100:0:0:1::/64、Teredo、benchmarking、ORCHID/ORCHIDv2、文档段
//     2001:db8::/32 与 3fff::/20、6to4 2002::/16、SRv6 5f00::/16、site-local
//     fec0::/10）此前在本包以「ipv6SpecialCIDRs 列表 + IPv4 字节级 switch」
//     形态自持一份、与 urlguard 的 blockedCIDRs 靠注释手工对账（#297 加段
//     就是三处联动），现删除本地清单改引 urlguard——出站目标判定（urlguard）
//     拒的段，「是否公网」判定同样拒，加段只改 pkg 一处；
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
	// net.IP 标准方法未覆盖的特殊段（core#98 的 0.0.0.0/8、fec0::/10，
	// core#73 的 ::/96 等）全在 urlguard 的缺省阻断清单里；v4-mapped 写法
	// （::ffff:127.0.0.1）由 stdlib 方法与 urlguard 的 To4 归一化同判。
	return !urlguard.IsBlockedIP(ip)
}

// IsIPv4 判断是否是 IPv4 地址。
func IsIPv4(ip net.IP) bool {
	return ip.To4() != nil
}

// collectInterfaceIPs 一次 net.InterfaceAddrs 取址并摘出 IP（core#134 单源）：
// GetLanIPs / ServerIPString / ServerIP / GetLocalIP 的过滤谓词各不相同，
// 「遍历 addrs → 断言 *net.IPNet → 取 IP」的循环此前复制四份；现收敛到这
// 一个取址点，各函数变为谓词 + 归约（纯函数谓词可用注入 IP 列表驱动测试，
// classifyServerIP 先例）。调用方共享同一 []net.IP 切片（只读，不修改元素）。
func collectInterfaceIPs() ([]net.IP, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	ips := make([]net.IP, 0, len(addrs))
	for _, address := range addrs {
		if ipnet, ok := address.(*net.IPNet); ok {
			ips = append(ips, ipnet.IP)
		}
	}
	return ips, nil
}

// GetLanIPs 获取局域网本地 IP 地址列表。
func GetLanIPs() ([]string, error) {
	ips, err := collectInterfaceIPs()
	if err != nil {
		return nil, err
	}
	var out []string
	for _, ip := range ips {
		if !ip.IsLoopback() &&
			!ip.IsLinkLocalMulticast() &&
			!ip.IsLinkLocalUnicast() &&
			IsIPv4(ip) &&
			!IsPublicIP(ip) {
			out = append(out, ip.String())
		}
	}
	return out, nil
}

// HostName 返回主机名。
func HostName() string {
	host, _ := os.Hostname()
	return host
}

// ServerIPString 返回所有服务器 IP 的逗号分隔字符串。
func ServerIPString() string {
	ips, err := collectInterfaceIPs()
	if err != nil {
		return ""
	}
	var out []string
	for _, ip := range ips {
		if !ip.IsLoopback() && IsIPv4(ip) {
			out = append(out, ip.String())
		}
	}
	return strings.Join(out, ",")
}

// ServerIP 返回服务器的外网 IP 和内网 IP。
//
// 内外网判定不再做字符串前缀匹配——旧实现把 172.16/12 私网判成外网
// （纯内网部署下注册中心拿到的地址是错的），又把全部 192.x.x.x 判成内网
// （TEST-NET 等公网段遭殃），`127.` 分支更是 loopback 过滤后的死代码
// （core#54）。现统一走 net.IP 标准方法（见 classifyServerIP），
// IPv4 与 IPv6 同一条判定线。
func ServerIP() (externalIP, internalIP string) {
	ips, err := collectInterfaceIPs()
	if err != nil {
		return "", ""
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

// errNoLocalIP 是 GetLocalIP 网卡探测拿不到可用地址时的哨兵错误（包级唯一，
// 便于调用方 errors.Is 判定，替代每处 errors.New 的匿名错误）。
var errNoLocalIP = errors.New("network: no suitable local IP found")

// GetLocalIP 返回本机首选非回环 IPv4 地址。
// 优先读取 POD_IP 环境变量（K8s 注入），其次遍历网卡获取第一个非回环 IPv4。
// 未找到时返回错误。这是 ekit 中获取本机 IP 的唯一实现，
// uidgen、nacos、registry 等包应委托至此，避免逻辑重复。
//
// POD_IP 经 net.ParseIP 校验，必须是 IPv4 且**非回环、非未指定**（本函数契约是
// 「非回环 IPv4」，core#87 只挡了「形状非法」，core#120 补上「语义非法」）：
// 注入串写错（not-an-ip、带端口的 1.2.3.4:8080、IPv6）与语义非法（127.0.0.1
// 等回环、0.0.0.0 未指定——以它们注册的实例不可被其他节点拨号，同文件
// podIPFallback 对回环是 ERROR 级口径）都不再原样返回（core#87 此前 err=nil
// 且错误地址直达注册面，与同文件其余路径的 fail-closed 口径相反）——非法值打
// WARN 后忽略，回退网卡探测；判据用 net.IP 标准方法，v4-mapped 写法
// （::ffff:127.0.0.1）同判。
func GetLocalIP() (string, error) {
	if ip := os.Getenv("POD_IP"); ip != "" {
		if p := net.ParseIP(ip); p != nil && p.To4() != nil &&
			!p.IsLoopback() && !p.IsUnspecified() {
			return ip, nil
		}
		// 非法/非 IPv4/回环/未指定：不中断（拿不到 IP 的代价是注册不可达，
		// 回退探测仍有救），但必须可观测——静默忽略等于把「注入错了」藏起来。
		slog.Warn("network: POD_IP 不是合法的非回环 IPv4，忽略并回退网卡探测",
			"component", "network.podip", "pod_ip", ip)
	}
	ips, err := collectInterfaceIPs()
	if err != nil {
		return "", err
	}
	for _, ip := range ips {
		if !ip.IsLoopback() && ip.To4() != nil {
			return ip.String(), nil
		}
	}
	return "", errNoLocalIP
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
