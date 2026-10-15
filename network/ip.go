package network

import (
	"errors"
	"net"
	"os"
	"strings"
)

// IsPublicIP 判断 IP 是否是公网 IP。
func IsPublicIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalMulticast() || ip.IsLinkLocalUnicast() {
		return false
	}
	if ip4 := ip.To4(); ip4 != nil {
		switch {
		case ip4[0] == 10:
			return false
		case ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31:
			return false
		case ip4[0] == 192 && ip4[1] == 168:
			return false
		default:
			return true
		}
	}
	return false
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
func ServerIP() (externalIP, internalIP string) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", ""
	}

	for _, address := range addrs {
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				ipaddr := ipnet.IP.String()
				if strings.HasPrefix(ipaddr, "10.") || strings.HasPrefix(ipaddr, "192.") || strings.HasPrefix(ipaddr, "127.") {
					internalIP = ipaddr
				} else {
					externalIP = ipaddr
				}
			}
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
// 这是 GetLocalIP 的便捷封装，uidgen、nacos、registry 应委托至此。
func GetPodIP() string {
	ip, err := GetLocalIP()
	if err != nil {
		return "127.0.0.1"
	}
	return ip
}
