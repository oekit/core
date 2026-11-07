package network

import (
	"errors"
	"log/slog"
	"net"
	"os"
	"strings"
)

// IsPublicIP 判断 IP 是否是公网（全球可路由单播）IP。
//
// 判定统一走 net.IP 的标准方法，不做字符串前缀匹配：
//   - 两族通用拒绝：loopback、link-local、组播（IsMulticast 覆盖 IPv4 224/4
//     与 IPv6 ff00::/8，含链路本地组播）、未指定地址、私网（IsPrivate：10/8、
//     172.16/12、192.168/16 及 IPv6 ULA fc00::/7）；
//   - IPv4 额外拒绝特殊用途段：CGNAT 100.64/10、192.0.0.0/24、基准测试段
//     198.18/15、TEST-NET 192.0.2.0/24 / 198.51.100.0/24 / 203.0.113.0/24、
//     保留段 240/4（含广播 255.255.255.255）；
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
func GetLocalIP() (string, error) {
	if ip := os.Getenv("POD_IP"); ip != "" {
		return ip, nil
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
