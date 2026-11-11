package hertzbridge

import (
	"context"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol"
	"github.com/oekit/core/transport"
)

// 编译期接口满足性断言（stdlib 惯用形态）：三个适配类型实现 transport 全集。
var (
	_ transport.Header      = (*requestHeader)(nil)
	_ transport.Header      = (*replyHeader)(nil)
	_ transport.Transporter = (*serverTransport)(nil)
)

// newTestRequestContext 构造带请求头的 RequestContext（服务端形态）。
func newTestRequestContext(t *testing.T) *app.RequestContext {
	t.Helper()
	c := app.NewContext(1)
	c.Request.Reset()
	c.Request.Header.Set("X-Trace-Id", "t-123")
	c.Request.Header.Add("X-Trace-Id", "t-456") // 同 key 第二值
	c.Request.Header.SetMethod("POST")
	c.Request.SetRequestURI("/api/orders")
	c.Request.URI().SetHost("svc.example.com")
	return c
}

// TestGetValuesEqualFoldCaliber 钉住键匹配口径（#163）：Get 与 Values 同为
// 大小写不敏感——canonical 形态的键（hertz Set/Add 后存储即规范化）用任意
// 大小写变体查询都要命中。middleware 侧同构副本漂移的就是这一条
// （middleware#177：Values 原文比较 → Get 命中而 Values 落空）。
func TestGetValuesEqualFoldCaliber(t *testing.T) {
	c := newTestRequestContext(t)

	req := NewRequestHeader(&c.Request.Header)
	if got := req.Get("x-trace-id"); got != "t-123" {
		t.Fatalf("request Get 大小写变体未命中（应返回首值）: got %q", got)
	}
	if vs := req.Values("X-TRACE-ID"); len(vs) != 2 || vs[0] != "t-123" || vs[1] != "t-456" {
		t.Fatalf("request Values 大小写变体未命中或多值丢失: %v", vs)
	}

	c.Response.Header.Set("Vary", "Origin")
	c.Response.Header.Add("Vary", "Accept-Encoding")
	reply := NewReplyHeader(&c.Response.Header)
	if got := reply.Get("vary"); got != "Origin" {
		t.Fatalf("reply Get 大小写变体未命中: got %q", got)
	}
	if vs := reply.Values("VaRy"); len(vs) != 2 || vs[0] != "Origin" || vs[1] != "Accept-Encoding" {
		t.Fatalf("reply Values 大小写变体未命中或多值丢失: %v", vs)
	}
}

// TestValuesKeysDuality 钉住 Values/Keys 对偶：Keys 返回 canonical 键，
// 以 Keys() 的键（任意大小写）回查 Values 必须命中且含全部值。
func TestValuesKeysDuality(t *testing.T) {
	c := newTestRequestContext(t)
	req := NewRequestHeader(&c.Request.Header)
	req.Set("X-Request-Id", "r-1")

	keys := req.Keys()
	seen := map[string][]string{}
	for _, k := range keys {
		seen[k] = req.Values(k)
	}
	for _, want := range []string{"X-Trace-Id", "X-Request-Id"} {
		if _, ok := seen[want]; !ok {
			t.Fatalf("Keys 缺 canonical 键 %q: %v", want, keys)
		}
		if len(seen[want]) == 0 {
			t.Fatalf("以 Keys 的键 %q 回查 Values 落空（对偶破坏）", want)
		}
	}
	if len(seen["X-Trace-Id"]) != 2 {
		t.Fatalf("X-Trace-Id 应有两值: %v", seen["X-Trace-Id"])
	}
	// 键已排序（确定性契约）。
	for i := 1; i < len(keys); i++ {
		if keys[i-1] > keys[i] {
			t.Fatalf("Keys 未排序: %v", keys)
		}
	}
}

// TestSetAddDelPassthrough 钉住 Set/Add/Del 的**透传**语义（单源取两个既有
// 适配器的共同口径，不另起炉灶）：
//   - Add 追加多值；Del 删净（大小写变体键同删）；
//   - Set 直传 hertz——hertz 的 Set 对同 key 多值**只替换首个出现**
//     （setArgBytes 首中即返，不整体收敛，hertz 上游 quirk；见本包
//     requestHeader.Set 的 godoc）——测试钉的是透传行为本身。
func TestSetAddDelPassthrough(t *testing.T) {
	c := newTestRequestContext(t)
	req := NewRequestHeader(&c.Request.Header)

	req.Set("X-Mode", "a")
	req.Add("X-Mode", "b")
	req.Add("X-Mode", "c")
	if vs := req.Values("x-mode"); len(vs) != 3 || vs[0] != "a" || vs[2] != "c" {
		t.Fatalf("Add 未追加: %v", vs)
	}
	req.Set("X-Mode", "only")
	if vs := req.Values("X-Mode"); len(vs) != 3 || vs[0] != "only" {
		t.Fatalf("Set 透传行为失真（hertz 只替换首个出现）: %v", vs)
	}
	req.Del("X-MODE")
	if vs := req.Values("X-Mode"); len(vs) != 0 {
		t.Fatalf("Del 未删净（大小写变体键也须删掉）: %v", vs)
	}
	if got := req.Get("X-Mode"); got != "" {
		t.Fatalf("Del 后 Get 应为空: %q", got)
	}

	// reply 侧同契约（middleware#99：Add 曾误用 Set，多值响应头丢值）。
	c.Response.Header.Set("Vary", "Origin")
	reply := NewReplyHeader(&c.Response.Header)
	reply.Add("vary", "Accept-Encoding")
	if vs := reply.Values("Vary"); len(vs) != 2 {
		t.Fatalf("reply Add 未追加（Set 语义泄漏）: %v", vs)
	}
}

// TestTransporterMapping 钉住 Transporter 映射：Kind/Endpoint/Operation 与
// Request/Reply header 的接线。
func TestTransporterMapping(t *testing.T) {
	c := newTestRequestContext(t)
	tr := NewServerTransport(c)

	if tr.Kind() != transport.KindHTTP {
		t.Fatalf("Kind 应为 http: %v", tr.Kind())
	}
	if tr.Endpoint() != "svc.example.com" {
		t.Fatalf("Endpoint 应为 Host: %q", tr.Endpoint())
	}
	if tr.Operation() != "/api/orders" {
		t.Fatalf("Operation 应为 PATH: %q", tr.Operation())
	}
	if _, ok := tr.RequestHeader().(*requestHeader); !ok {
		t.Fatalf("RequestHeader 应为 requestHeader 适配")
	}
	if _, ok := tr.ReplyHeader().(*replyHeader); !ok {
		t.Fatalf("ReplyHeader 应为 replyHeader 适配")
	}
}

// TestNewServerContextRoundtrip 钉住 context 注入/取出回路——middleware
// 与 hertz 两仓 wave 3/4 的消费形态。
func TestNewServerContextRoundtrip(t *testing.T) {
	c := newTestRequestContext(t)
	ctx := NewServerContext(context.Background(), c)

	got, ok := transport.FromServerContext(ctx)
	if !ok {
		t.Fatal("FromServerContext 未取到 Transporter")
	}
	if got.Operation() != "/api/orders" {
		t.Fatalf("回路 Operation 失真: %q", got.Operation())
	}
	if v := got.RequestHeader().Get("X-TRACE-ID"); v != "t-123" {
		t.Fatalf("经 context 取出的 header 口径失真: %q", v)
	}
}

// TestEmptyHeaderValues 钉住空表行为：不存在的键 Get 空串、Values nil、Keys 空。
func TestEmptyHeaderValues(t *testing.T) {
	h := &protocol.RequestHeader{}
	req := NewRequestHeader(h)
	if got := req.Get("Nope"); got != "" {
		t.Fatalf("不存在的键应返回空串: %q", got)
	}
	if vs := req.Values("Nope"); vs != nil {
		t.Fatalf("不存在的键 Values 应为 nil: %v", vs)
	}
	if ks := req.Keys(); len(ks) != 0 {
		t.Fatalf("空表 Keys 应为空: %v", ks)
	}
}
