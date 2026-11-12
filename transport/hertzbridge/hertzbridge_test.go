package hertzbridge

import (
	"context"
	"slices"
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

// —— #154：reply Get 的 Set-Cookie blob 面 ——

// TestReplyHeader_SetCookieGetFirstValue 钉住 #154 治本：响应侧每枚
// Set-Cookie 是独立头行，Get 返回**第一枚**（根包契约「Get=第一个值」），
// 不是 hertz peek 的 "; " 连接 blob——blob 与单枚 cookie 的属性序列无法
// 区分，消费方按「取第一个 cookie」解析必错。Values 逐枚、Del 清全部。
// 对照面：请求侧 Cookie 是 RFC 6265 单头文法，Get 返回合并串语义正确。
func TestReplyHeader_SetCookieGetFirstValue(t *testing.T) {
	h := &protocol.ResponseHeader{}
	h.Add("Set-Cookie", "a=1; path=/")
	h.Add("Set-Cookie", "b=2; path=/")
	reply := NewReplyHeader(h)

	if got := reply.Get("set-cookie"); got != "a=1; path=/" {
		t.Fatalf("★ reply Get(Set-Cookie) 应返回第一枚 cookie（#154，此前是合并 blob）: %q", got)
	}
	if vs := reply.Values("Set-Cookie"); len(vs) != 2 || vs[0] != "a=1; path=/" || vs[1] != "b=2; path=/" {
		t.Fatalf("Values 应逐枚返回 cookie: %v", vs)
	}

	// Del 经 EqualFold 收口：清全部 cookie（hertz del 的 Set-Cookie 特例）。
	reply.Del("SET-COOKIE")
	if vs := reply.Values("Set-Cookie"); len(vs) != 0 {
		t.Fatalf("Del(Set-Cookie) 应清空全部 cookie: %v", vs)
	}
	if got := reply.Get("Set-Cookie"); got != "" {
		t.Fatalf("Del 后 Get 应为空: %q", got)
	}

	// 对照面：请求侧 Cookie 单头文法，Get=合并串语义正确（不在 #154 内）。
	rh := &protocol.RequestHeader{}
	rh.Set("Cookie", "a=1; b=2")
	if got := NewRequestHeader(rh).Get("cookie"); got != "a=1; b=2" {
		t.Fatalf("请求侧 Cookie 合并文法不得被破坏: %q", got)
	}
}

// —— #155：EqualFold 承诺不依赖 hertz 键规范化开关 ——

// TestEqualFoldUnderDisableNormalizing 钉住 #155 兑现路线：hertz 的键规范化
// 可被 DisableNormalizing()（server.WithDisableHeaderNamesNormalizing(true)
// 的逐请求等价面）关闭，关闭后 hertz 自身 Get/Del 退化为大小写敏感——本
// 适配器经 VisitAll 自行匹配，Get/Values/Del 的 EqualFold 承诺在
// normalize-on/off 两种形态下都成立（#163 病灶「Get 命中而 Values 落空」的
// 镜像复发被结构性关死）。
func TestEqualFoldUnderDisableNormalizing(t *testing.T) {
	// 请求侧：两枚大小写变体键。
	h := &protocol.RequestHeader{}
	h.DisableNormalizing()
	h.Add("x-trace-id", "v1")
	h.Add("X-Trace-Id", "v2")
	req := NewRequestHeader(h)

	if got := req.Get("X-TRACE-ID"); got != "v1" {
		t.Fatalf("★ normalize-off 下 Get 仍须 EqualFold 命中首值（#155）: %q", got)
	}
	if vs := req.Values("X-Trace-Id"); len(vs) != 2 || vs[0] != "v1" || vs[1] != "v2" {
		t.Fatalf("normalize-off 下 Values 仍须命中全部变体: %v", vs)
	}
	req.Del("X-TRACE-ID")
	if vs := req.Values("x-trace-id"); len(vs) != 0 {
		t.Fatalf("★ normalize-off 下 Del 须删净全部大小写变体（#155 探针的 Del 缺口）: %v", vs)
	}
	if got := req.Get("X-Trace-Id"); got != "" {
		t.Fatalf("Del 后 Get 应为空: %q", got)
	}

	// 响应侧同契约。
	rh := &protocol.ResponseHeader{}
	rh.DisableNormalizing()
	rh.Add("x-request-id", "r1")
	reply := NewReplyHeader(rh)
	if got := reply.Get("X-REQUEST-ID"); got != "r1" {
		t.Fatalf("★ reply normalize-off 下 Get EqualFold 失效: %q", got)
	}
	reply.Del("x-REQUEST-id")
	if vs := reply.Values("X-Request-Id"); len(vs) != 0 {
		t.Fatalf("★ reply normalize-off 下 Del 变体删不净: %v", vs)
	}
}

// —— #156：Endpoint = 请求面 Host（客户端可控） ——

// TestEndpointHostUntrustedInput 钉住 #156 的不可信输入语义：请求携带的
// Host（含恶意值）会原样成为 Endpoint——契约文档已分层标注「请求寻址信息，
// 不得用于安全判定/ACL」，本测试把该语义钉进行为面，防止后续实现悄悄
// 变成「看似可信」的来源。
func TestEndpointHostUntrustedInput(t *testing.T) {
	c := app.NewContext(1)
	c.Request.Reset()
	c.Request.SetRequestURI("/x")
	c.Request.URI().SetHost("evil.com:8080") // 客户端可控的寻址输入
	tr := NewServerTransport(c)
	if got := tr.Endpoint(); got != "evil.com:8080" {
		t.Fatalf("★ Endpoint 应为请求面 Host（不可信输入原样流出，#156）: %q", got)
	}
}

// —— #164：Get=首值在「首枚命中值为空串」时不得让位 ——

// TestGetFirstValueEmptyFirstMatch 钉住 core#164：多值 header 首枚值为空串
// 时，Get 必须返回 ""（首值），不得穿透到后续非空值。旧实现以 out==""
// 兼作「未命中」判定，空首值被吞、Get 返回了第二个值。
func TestGetFirstValueEmptyFirstMatch(t *testing.T) {
	c := newTestRequestContext(t)
	c.Request.Header.Set("X-Tag", "") // 首枚：空串
	c.Request.Header.Add("X-Tag", "real")
	req := NewRequestHeader(&c.Request.Header)

	if got := req.Get("x-tag"); got != "" {
		t.Fatalf("★ Get 应返回首值（空串），不得穿透到后续值: got %q", got)
	}
	vs := req.Values("X-Tag")
	if len(vs) != 2 || vs[0] != "" || vs[1] != "real" {
		t.Fatalf("Values 应保留空串+real 两枚: %v", vs)
	}
	if ks := req.Keys(); !slices.Contains(ks, "X-Tag") {
		t.Fatalf("Keys 应含 X-Tag（fixture 另有预置键）: %v", ks)
	}

	// 对照组：仅空串一枚也视为命中（与「不存在」区分的口径仍以空串返回，
	// 这里钉的是不 panic、不落后续 header）
	c2 := newTestRequestContext(t)
	c2.Request.Header.Set("X-Only-Empty", "")
	req2 := NewRequestHeader(&c2.Request.Header)
	if got := req2.Get("x-only-empty"); got != "" {
		t.Fatalf("仅空串一枚时 Get 仍应为空串: %q", got)
	}
	if vs2 := req2.Values("X-Only-Empty"); len(vs2) != 1 || vs2[0] != "" {
		t.Fatalf("仅空串一枚时 Values 应恰含一枚空串: %v", vs2)
	}
}
