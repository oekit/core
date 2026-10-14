package convert

import (
	"net/http/httptest"
	"testing"
)

// TestGetClientIPWithProxies_XFFSpoofing 回归：XFF 每个代理把上一跳追加到
// 右端，左侧是客户端可任意伪造的。从左向右扫描取"第一个非可信 IP"会把
// 攻击者注入的伪造值当成客户端 IP。钉住：从右向左跳过可信代理。
func TestGetClientIPWithProxies_XFFSpoofing(t *testing.T) {
	// 场景：客户端(1.2.3.4)伪造 XFF 后经内网代理 10.0.0.1 → 10.0.0.2 到达，
	// 服务端 RemoteAddr=10.0.0.2，XFF="9.9.9.9, 1.2.3.4, 10.0.0.1"
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.2:12345"
	r.Header.Set("X-Forwarded-For", "9.9.9.9, 1.2.3.4, 10.0.0.1")

	got := GetClientIPWithProxies(r, defaultTrustedProxies)
	if got != "1.2.3.4" {
		t.Errorf("GetClientIP = %q, want 1.2.3.4 (rightmost non-trusted), forged leftmost 9.9.9.9 must be ignored", got)
	}
}

// TestGetClientIPWithProxies_PureChain 无伪造的干净代理链。
func TestGetClientIPWithProxies_PureChain(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.2:12345"
	r.Header.Set("X-Forwarded-For", "1.2.3.4, 10.0.0.1")

	if got := GetClientIPWithProxies(r, defaultTrustedProxies); got != "1.2.3.4" {
		t.Errorf("GetClientIP = %q, want 1.2.3.4", got)
	}
}

// TestGetClientIPWithProxies_AllTrusted 全链路可信（内网互调）：回退 XFF 最左端。
func TestGetClientIPWithProxies_AllTrusted(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.2:12345"
	r.Header.Set("X-Forwarded-For", "10.0.0.0, 10.0.0.1")

	if got := GetClientIPWithProxies(r, defaultTrustedProxies); got != "10.0.0.0" {
		t.Errorf("GetClientIP = %q, want 10.0.0.0 (leftmost when all trusted)", got)
	}
}

// TestGetClientIPWithProxies_DirectUntrusted 直连（RemoteAddr 非可信）时忽略 XFF。
func TestGetClientIPWithProxies_DirectUntrusted(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "5.6.7.8:9999"
	r.Header.Set("X-Forwarded-For", "9.9.9.9")

	if got := GetClientIPWithProxies(r, defaultTrustedProxies); got != "5.6.7.8" {
		t.Errorf("GetClientIP = %q, want 5.6.7.8 (RemoteAddr, XFF from untrusted peer ignored)", got)
	}
}
