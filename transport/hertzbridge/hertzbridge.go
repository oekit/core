// Package hertzbridge 把 Hertz 的请求上下文与 header 适配为 core/transport
// 的协议无关抽象——hertz RequestHeader/ResponseHeader → transport.Header 的
// **单源适配层**（middleware#177/#180 治本落点）。
//
// # 为什么收进 core
//
// 「hertz header → transport.Header」的适配器此前在两个仓各持一份
// 「同构实现」，靠注释互相约束一致——实际已经漂移（middleware#177）：
// hertz 仓 #163 把 Values 的键匹配改成大小写不敏感（与 Get 同口径），
// middleware 仓的同构副本仍是修复前的原文比较，「Get 命中而 Values 落空」
// 的缺陷在 middleware 的活跃路径（accesslog → chainmw 内核）上继续存活。
// 治本是把适配下沉到 core（core 对 hertz 已有依赖，transport 根包保持
// 仅标准库、hertzbridge 作为独立子包引入框架依赖），两个仓 import 同一份，
// 消灭「同构实现」这个漂移源。
//
// # 消费方（后续 wave 改引，本包落成时那两仓不动）
//
//   - oekit/middleware（wave 3）：transporter.go 的 hertzTransporter 与
//     hertzRequestHeader/hertzReplyHeader 删除，改引本包
//     NewServerContext/NewServerTransport；
//   - oekit/hertz（wave 4）：transportx 改为薄委托本包（或直接由消费方
//     改引），#163 的 EqualFold 口径随单源固化。
//
// # 键匹配口径
//
// Get 与 Values 同一口径——**大小写不敏感**（EqualFold）：hertz 存取键均
// 规范化为 MIME canonical 形态（NormalizeHeaderKey），Get 天然大小写不敏感
// 命中；Values 此前原文比较导致调用方传大小写变体时落空（#163）。
// Keys() 返回 hertz 规范化后的 canonical 键（排序去重），消费方应以大小写
// 不敏感方式回读，不要拿 Keys() 的键做原文匹配。
package hertzbridge

import (
	"context"
	"maps"
	"slices"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol"
	"github.com/oekit/core/transport"
)

// NewServerTransport 把服务端 RequestContext 适配为 transport.Transporter。
// Kind=http；Endpoint 为 Host；Operation 为请求 PATH。
// 返回的 Transporter 持有 RequestContext 引用，生命周期与请求一致。
func NewServerTransport(c *app.RequestContext) transport.Transporter {
	return &serverTransport{c: c}
}

// NewServerContext 返回携带服务端 Transporter 的 context
// （transport.NewServerContext 的 hertz 便捷形态）。
func NewServerContext(ctx context.Context, c *app.RequestContext) context.Context {
	return transport.NewServerContext(ctx, NewServerTransport(c))
}

// NewRequestHeader 把 hertz RequestHeader 适配为 transport.Header
// （服务端请求头与客户端出站头同型）。
func NewRequestHeader(h *protocol.RequestHeader) transport.Header {
	return &requestHeader{h: h}
}

// NewReplyHeader 把 hertz ResponseHeader 适配为 transport.Header（服务端响应头）。
func NewReplyHeader(h *protocol.ResponseHeader) transport.Header {
	return &replyHeader{h: h}
}

type serverTransport struct {
	c *app.RequestContext
}

func (t *serverTransport) Kind() transport.Kind { return transport.KindHTTP }

func (t *serverTransport) Endpoint() string { return string(t.c.Host()) }

func (t *serverTransport) Operation() string { return string(t.c.Path()) }

func (t *serverTransport) RequestHeader() transport.Header {
	return NewRequestHeader(&t.c.Request.Header)
}

func (t *serverTransport) ReplyHeader() transport.Header {
	return NewReplyHeader(&t.c.Response.Header)
}

// requestHeader 适配 hertz RequestHeader。
type requestHeader struct {
	h *protocol.RequestHeader
}

// Get 与 Values 键匹配同口径（#163）：hertz 存取键均规范化为 MIME canonical
// 形态（NormalizeHeaderKey），Get 大小写不敏感命中。
func (a *requestHeader) Get(key string) string { return a.h.Get(key) }

// Values 返回 key 的全部值，键匹配与 Get 同一口径——大小写不敏感（#163）。
//
// 此前为原文比较：VisitAll 产出的键是 hertz 规范化后的 canonical 形态
// （如 "X-Trace-Id"），调用方传 "x-trace-id" 这类大小写变体时 Get 命中、
// Values 返回 nil——同一适配器两套键口径，Get 命中而 Values 落空。
func (a *requestHeader) Values(key string) []string {
	var out []string
	a.h.VisitAll(func(k, v []byte) {
		if strings.EqualFold(string(k), key) {
			out = append(out, string(v))
		}
	})
	return out
}

// Set 直传 hertz。注意 hertz 上游 quirk：Set 对同 key 多值**只替换首个
// 出现**（setArgBytes 首中即返，不做整体收敛）——transport.Header 的
// 「覆盖为单值」契约在多值键上由调用方先 Del 再 Set 达成。本适配是两个
// 既有实现（middleware/transporter.go、hertz/transportx）的共同口径，
// 单源不另起语义（test 钉住透传行为）。
func (a *requestHeader) Set(key, value string) { a.h.Set(key, value) }

func (a *requestHeader) Add(key, value string) { a.h.Add(key, value) }

func (a *requestHeader) Del(key string) { a.h.Del(key) }

// Keys 返回全部键（已排序，保证确定性）。键为 hertz 规范化的 MIME canonical
// 形态（如 Set("X-Trace-ID",…) 后 Keys() 产出 "X-Trace-Id"）——消费方应以
// 大小写不敏感方式回读（Get/Values 与 Get 同口径，#163），不要拿 Keys() 的
// 键做原文匹配。
func (a *requestHeader) Keys() []string {
	return keysFromVisitAll(a.h.VisitAll)
}

// replyHeader 适配 hertz ResponseHeader。
type replyHeader struct {
	h *protocol.ResponseHeader
}

// Get 与 Values 键匹配同口径（#163）：hertz 存取键均规范化为 MIME canonical
// 形态，Get 大小写不敏感命中。
func (a *replyHeader) Get(key string) string { return a.h.Get(key) }

// Values 返回 key 的全部值，键匹配与 Get 同一口径——大小写不敏感（#163，
// requestHeader.Values 同病同修）。
func (a *replyHeader) Values(key string) []string {
	var out []string
	a.h.VisitAll(func(k, v []byte) {
		if strings.EqualFold(string(k), key) {
			out = append(out, string(v))
		}
	})
	return out
}

func (a *replyHeader) Set(key, value string) { a.h.Set(key, value) }

// Add 追加（同 key 多值）——与 requestHeader.Add 及 transport.Header.Add
// 语义一致；此前有实现误用 Set（覆盖），后写的值会顶掉先写的值，
// Vary/Set-Cookie 类多值响应头被静默丢值（middleware#99 同源修复）。
func (a *replyHeader) Add(key, value string) { a.h.Add(key, value) }

func (a *replyHeader) Del(key string) { a.h.Del(key) }

// Keys 返回全部键（已排序，保证确定性）；键为 hertz 规范化的 MIME canonical
// 形态，消费方应以大小写不敏感方式回读（Get/Values 与 Get 同口径，#163）。
func (a *replyHeader) Keys() []string {
	return keysFromVisitAll(a.h.VisitAll)
}

// keysFromVisitAll 汇总 VisitAll 遍历出的去重排序 key 列表。
// visitAll 形参是 header 的遍历方法值：func(fn func(k, v []byte))。
func keysFromVisitAll(visitAll func(fn func(k, v []byte))) []string {
	seen := make(map[string]struct{})
	visitAll(func(k, _ []byte) {
		seen[string(k)] = struct{}{}
	})
	return slices.Sorted(maps.Keys(seen))
}
