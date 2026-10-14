package httpjson

// Retry-After 捕获/解析/取舍单测（收编自 agentkit llm/retryafter_test.go）。

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseRetryAfter(t *testing.T) {
	cases := map[string]time.Duration{
		"120":  120 * time.Second,
		"0":    0,
		"-5":   0,
		"":     0,
		"abc":  0,
		" 30 ": 30 * time.Second,
	}
	for in, want := range cases {
		if got := parseRetryAfter(in); got != want {
			t.Fatalf("parseRetryAfter(%q) = %v, want %v", in, got, want)
		}
	}
	// HTTP-date 形态：未来时间 → 正数；过去时间 → 0
	future := time.Now().Add(90 * time.Second).UTC().Format(http.TimeFormat)
	if got := parseRetryAfter(future); got <= 0 {
		t.Fatalf("未来 date 应为正: %v", got)
	}
	if got := parseRetryAfter(time.Now().Add(-time.Hour).UTC().Format(http.TimeFormat)); got != 0 {
		t.Fatalf("过去 date 应为 0: %v", got)
	}
}

func TestSelectRetryDelay(t *testing.T) {
	cases := []struct {
		name        string
		local, hint time.Duration
		want        time.Duration
	}{
		{"无建议→本地", 8 * time.Second, 0, 8 * time.Second},
		{"建议短于本地→采信", 8 * time.Second, 3 * time.Second, 3 * time.Second},
		{"建议长于本地但≤5min→采信", 8 * time.Second, 90 * time.Second, 90 * time.Second},
		{"建议超 5min→本地", 8 * time.Second, 10 * time.Minute, 8 * time.Second},
		{"建议超 5min 且长于本地→本地", 8 * time.Minute, 10 * time.Minute, 8 * time.Minute},
	}
	for _, c := range cases {
		if got := SelectRetryDelay(c.local, c.hint); got != c.want {
			t.Fatalf("%s: SelectRetryDelay(%v,%v) = %v, want %v", c.name, c.local, c.hint, got, c.want)
		}
	}
}

// 传输层端到端：429 + Retry-After 头 → ctx sink 捕获；非 429 不捕获；
// 无 sink 的请求捕获跳过不炸。
func TestRetryAfterTransportCaptures(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	tr := RetryAfterTransport{}
	ctx := WithRetryAfterSink(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if got := RetryAfterFrom(ctx); got != 7*time.Second {
		t.Fatalf("sink 应捕获 7s: %v", got)
	}

	req2, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	resp2, err := tr.RoundTrip(req2)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp2.Body.Close()
	if calls.Load() != 2 {
		t.Fatalf("服务端应被调用两次: %d", calls.Load())
	}
}
