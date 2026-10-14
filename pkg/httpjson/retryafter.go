package httpjson

// Retry-After 捕获与消费：配额型端点在 429 响应里携带 Retry-After（秒数或
// HTTP-date），服务端建议比本地指数退避更准——限流窗口还剩多久只有端点知道。
//
// 管线三段：
//   1. capture：RetryAfterTransport 包装 RoundTripper，429 响应读取 Retry-After
//      写入请求 ctx 中的 sink（透明；非 429 原样透传）；
//   2. 安装：WithRetryAfterSink 在重试环上游注入 sink（消费方与传输层以此相遇）；
//   3. 消费：RetryAfterFrom 读取建议，重试环按 SelectRetryDelay 的钳制规则
//      取舍——建议 >0 且（≤5min 或 < 本地曲线值）才取代本地退避。
//
// 隐私与内存：sink 只存一个时长；未安装 sink 的请求零开销。
// 收编自 agentkit llm/retryafter.go（v0.10.41 起单源在此）。

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// MaxServerRetryAfter 服务端退避建议的可信上限：超过 5 分钟的建议不直接采信
// （长时间限流应交由上层 failover/切换处置，而非原地等）。
const MaxServerRetryAfter = 5 * time.Minute

type retryAfterHolder struct {
	mu sync.Mutex
	d  time.Duration
}

func (h *retryAfterHolder) set(d time.Duration) {
	h.mu.Lock()
	h.d = d
	h.mu.Unlock()
}

func (h *retryAfterHolder) get() time.Duration {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.d
}

type retryAfterKey struct{}

// WithRetryAfterSink 在 ctx 注入 sink（重试环上游调用一次；传输层据此回写）。
func WithRetryAfterSink(ctx context.Context) context.Context {
	return context.WithValue(ctx, retryAfterKey{}, &retryAfterHolder{})
}

// RetryAfterFrom 读取最近一次 429 的服务端建议（0 = 无）。重试环在计算退避时调用。
// sink 不随尝试清除：仅对本次失败确为限流的场景采信（陈旧建议不得套用到
// 非限流错误上），门控归调用方的重试策略。
func RetryAfterFrom(ctx context.Context) time.Duration {
	if h, ok := ctx.Value(retryAfterKey{}).(*retryAfterHolder); ok {
		return h.get()
	}
	return 0
}

// SelectRetryDelay 退避取舍（「合理性钳制」）：服务端建议 >0 且
// （≤MaxServerRetryAfter 或 < 本地曲线值）→ 采信建议；否则本地曲线。
func SelectRetryDelay(local, hint time.Duration) time.Duration {
	if hint <= 0 {
		return local
	}
	if hint <= MaxServerRetryAfter || hint < local {
		return hint
	}
	return local
}

// RetryAfterTransport 透明捕获 429 的 Retry-After 头。响应体不做任何改写——
// 上层错误构造不受影响。Base 为 nil 时用 http.DefaultTransport。
type RetryAfterTransport struct {
	Base http.RoundTripper
}

func (t RetryAfterTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	resp, err := base.RoundTrip(req)
	if err == nil && resp != nil && resp.StatusCode == http.StatusTooManyRequests {
		if h, ok := req.Context().Value(retryAfterKey{}).(*retryAfterHolder); ok {
			if d := parseRetryAfter(resp.Header.Get("Retry-After")); d > 0 {
				h.set(d)
			}
		}
	}
	return resp, err
}

// parseRetryAfter 解析 Retry-After：延迟秒数（整数）或 HTTP-date。无法解析返回 0。
func parseRetryAfter(v string) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil {
		if secs < 0 {
			return 0
		}
		return time.Duration(secs) * time.Second
	}
	if at, err := http.ParseTime(v); err == nil {
		d := time.Until(at)
		if d > 0 {
			return d
		}
	}
	return 0
}
