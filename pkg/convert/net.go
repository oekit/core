package convert

import (
	"net"
	"net/http"
	"strings"
)

// defaultTrustedProxies 是默认的可信代理 CIDR 列表（内网地址 + 回环地址）。
var defaultTrustedProxies = []string{
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"127.0.0.0/8",
	"::1/128",   // IPv6 loopback
	"fc00::/7",  // IPv6 unique local address
	"fe80::/10", // IPv6 link-local
}

// GetClientIP 从 HTTP 请求中提取真实客户端 IP。
// 优先级：X-Forwarded-For（仅当请求来自可信代理时） > X-Real-IP > RemoteAddr。
//
// 安全说明：默认只信任内网代理（10/8, 172.16/12, 192.168/16, 127/8）。
// 如果部署在公网代理后面（如 Cloudflare），需要通过 GetClientIPWithProxies 指定可信代理列表。
func GetClientIP(r *http.Request) string {
	return GetClientIPWithProxies(r, defaultTrustedProxies)
}

// GetClientIPWithProxies 从 HTTP 请求中提取真实客户端 IP，指定可信代理 CIDR 列表。
// 只有当 RemoteAddr 属于可信代理时，才信任 X-Forwarded-For 和 X-Real-IP 头。
// trustedProxies 为 nil 或空时，只使用 RemoteAddr。
func GetClientIPWithProxies(r *http.Request, trustedProxies []string) string {
	remoteIP := extractRemoteIP(r)

	// 检查 RemoteAddr 是否是可信代理
	if len(trustedProxies) > 0 && isTrustedProxy(remoteIP, trustedProxies) {
		// X-Forwarded-For 可能包含多个 IP（代理链）。
		// XFF 语义是每个代理把上一跳追加到右端，左侧是客户端可任意伪造的；
		// 必须从右向左跳过可信代理，返回第一个非可信 IP（gin/nginx realip 同款算法）。
		// 从左向右扫描会把攻击者注入的伪造值当成客户端 IP（限流 key/审计/访问控制被绕过）。
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			for i := len(parts) - 1; i >= 0; i-- {
				ip := strings.TrimSpace(parts[i])
				if ip != "" && !isTrustedProxy(ip, trustedProxies) {
					return ip
				}
			}
			// 全链路都是可信代理（如内网健康检查）：回退到最左端的原始发起方
			if len(parts) > 0 {
				return strings.TrimSpace(parts[0])
			}
		}

		if xri := r.Header.Get("X-Real-IP"); xri != "" {
			return strings.TrimSpace(xri)
		}
	}

	return remoteIP
}

// extractRemoteIP 从 RemoteAddr 提取 IP。
func extractRemoteIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// isTrustedProxy 检查 IP 是否在可信代理列表中。
func isTrustedProxy(ipStr string, trustedProxies []string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	for _, cidr := range trustedProxies {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return true
		}
	}
	return false
}
