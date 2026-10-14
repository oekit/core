package convert

import (
	"context"
	"net/http/httptest"
	"testing"
)

func TestStr2Int64(t *testing.T) {
	tests := []struct {
		s   string
		def int64
		out int64
	}{
		{"123", 0, 123},
		{"-1", 0, -1},
		{"abc", 42, 42},
		{"", 99, 99},
	}
	for _, tt := range tests {
		got := Str2Int64(tt.s, tt.def)
		if got != tt.out {
			t.Errorf("Str2Int64(%q, %d) = %d, want %d", tt.s, tt.def, got, tt.out)
		}
	}
}

func TestStr2Float64(t *testing.T) {
	tests := []struct {
		s   string
		def float64
		out float64
	}{
		{"3.14", 0, 3.14},
		{"abc", 1.0, 1.0},
	}
	for _, tt := range tests {
		got := Str2Float64(tt.s, tt.def)
		if got != tt.out {
			t.Errorf("Str2Float64(%q, %f) = %f, want %f", tt.s, tt.def, got, tt.out)
		}
	}
}

func TestStr2Bool(t *testing.T) {
	if !Str2Bool("true", false) {
		t.Error("expected true")
	}
	if Str2Bool("abc", false) {
		t.Error("expected false (default)")
	}
	if !Str2Bool("abc", true) {
		t.Error("expected true (default)")
	}
}

func TestBool2Str(t *testing.T) {
	if Bool2Str(true) != "true" {
		t.Error("expected 'true'")
	}
	if Bool2Str(false) != "false" {
		t.Error("expected 'false'")
	}
}

func TestInt64ToStr(t *testing.T) {
	if Int64ToStr(123) != "123" {
		t.Error("expected '123'")
	}
}

func TestConvertStruct(t *testing.T) {
	type Src struct {
		Name string
		Age  int
	}
	type Dst struct {
		Name string
		Age  int
		City string
	}

	src := Src{Name: "test", Age: 18}
	var dst Dst
	dst.City = "beijing"

	if err := ConvertStruct(src, &dst); err != nil {
		t.Fatal(err)
	}
	if dst.Name != "test" || dst.Age != 18 {
		t.Errorf("expected {test 18 beijing}, got %+v", dst)
	}
	if dst.City != "beijing" {
		t.Error("expected city preserved")
	}
}

func TestGetClientIP_XForwardedFor(t *testing.T) {
	// XFF 语义：每个代理把上一跳追加到右端。RemoteAddr=10.0.0.1（可信）时，
	// 5.6.7.8 是代理亲眼所见的前一跳（真实客户端），而最左端的 1.2.3.4 是
	// 由非可信的 5.6.7.8 自行声称的（可伪造）。取最右侧非可信 IP。
	r := httptest.NewRequestWithContext(context.Background(), "GET", "/", nil)
	r.RemoteAddr = "10.0.0.1:1234" // 可信代理
	r.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")
	ip := GetClientIP(r)
	if ip != "5.6.7.8" {
		t.Errorf("expected 5.6.7.8 (rightmost non-trusted), got %s", ip)
	}
}

func TestGetClientIP_XRealIP(t *testing.T) {
	r := httptest.NewRequestWithContext(context.Background(), "GET", "/", nil)
	r.RemoteAddr = "10.0.0.1:1234" // 可信代理
	r.Header.Set("X-Real-IP", "9.8.7.6")
	ip := GetClientIP(r)
	if ip != "9.8.7.6" {
		t.Errorf("expected 9.8.7.6, got %s", ip)
	}
}

func TestGetClientIP_RemoteAddr(t *testing.T) {
	r := httptest.NewRequestWithContext(context.Background(), "GET", "/", nil)
	ip := GetClientIP(r)
	if ip == "" {
		t.Error("expected non-empty IP")
	}
}

func TestGetClientIP_UntrustedProxy(t *testing.T) {
	// RemoteAddr 不在可信代理列表中，X-Forwarded-For 应被忽略
	r := httptest.NewRequestWithContext(context.Background(), "GET", "/", nil)
	r.RemoteAddr = "203.0.113.1:1234" // 公网 IP，不是可信代理
	r.Header.Set("X-Forwarded-For", "1.2.3.4")
	ip := GetClientIP(r)
	if ip != "203.0.113.1" {
		t.Errorf("expected 203.0.113.1 (untrusted proxy ignored), got %s", ip)
	}
}

func TestGetClientIP_IPv6Loopback(t *testing.T) {
	r := httptest.NewRequestWithContext(context.Background(), "GET", "/", nil)
	r.RemoteAddr = "[::1]:1234" // IPv6 回环，可信代理
	r.Header.Set("X-Forwarded-For", "2001:db8::1")
	ip := GetClientIP(r)
	if ip != "2001:db8::1" {
		t.Errorf("expected 2001:db8::1, got %s", ip)
	}
}

func TestGetClientIP_XForwardedFor_AllProxies(t *testing.T) {
	// X-Forwarded-For 中所有 IP 都是可信代理，应返回第一个
	r := httptest.NewRequestWithContext(context.Background(), "GET", "/", nil)
	r.RemoteAddr = "10.0.0.1:1234"
	r.Header.Set("X-Forwarded-For", "10.0.0.2, 10.0.0.3")
	ip := GetClientIP(r)
	if ip != "10.0.0.2" {
		t.Errorf("expected 10.0.0.2, got %s", ip)
	}
}

func TestGetClientIPWithProxies_Custom(t *testing.T) {
	r := httptest.NewRequestWithContext(context.Background(), "GET", "/", nil)
	r.RemoteAddr = "203.0.113.1:1234" // 自定义可信代理
	r.Header.Set("X-Forwarded-For", "1.2.3.4")
	ip := GetClientIPWithProxies(r, []string{"203.0.113.0/24"})
	if ip != "1.2.3.4" {
		t.Errorf("expected 1.2.3.4, got %s", ip)
	}
}

func TestGetClientIPWithProxies_Nil(t *testing.T) {
	// nil 可信代理列表，应只使用 RemoteAddr
	r := httptest.NewRequestWithContext(context.Background(), "GET", "/", nil)
	r.RemoteAddr = "10.0.0.1:1234"
	r.Header.Set("X-Forwarded-For", "1.2.3.4")
	ip := GetClientIPWithProxies(r, nil)
	if ip != "10.0.0.1" {
		t.Errorf("expected 10.0.0.1 (nil proxies = ignore headers), got %s", ip)
	}
}
