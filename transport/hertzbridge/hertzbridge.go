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
// # 键匹配口径（#155：适配器自扛，不依赖 hertz 键规范化开关）
//
// Get 与 Values 同一口径——**大小写不敏感**（EqualFold），且由本适配器
// 自行兑现：Get/Values/Del 都经 VisitAll 按 EqualFold 匹配，**不依赖 hertz
// 的键规范化处于开启状态**——hertz 的键规范化可被公开 API 关闭
// （server.WithDisableHeaderNamesNormalizing(true) / RequestHeader.
// DisableNormalizing()，代理网关/保留原始大小写场景用），关闭后 hertz 自身
// 的 Get/Del 退化为大小写敏感（NormalizeHeaderKey 直接 return），本适配器
// 的 EqualFold 承诺在两种形态下都成立（探针测试钉住）。Set 是 hertz 直通
// 例外：其「覆盖为单值」的作用面仍随 hertz 规范化开关走（见 requestHeader.
// Set 的披露）。
//
// # 值形态 quirk 披露（#154：Get/Values 在多值头上的两种形态）
//
//   - 响应侧 Set-Cookie：每枚 cookie 是独立头行，Get 返回**第一枚**，
//     取全部用 Values——此前 Get 直通 hertz 会把全部 cookie 用 "; " 连成
//     一个 blob 返回（与单枚 cookie 的属性序列无法区分），「Get=第一个值」
//     契约失真（#154，已修）；
//   - 请求侧 Cookie：RFC 6265 单头文法（全部 cookie 分号连接在一行），
//     Get 返回合并串**语义正确**，不在 #154 缺陷内；
//   - 合成视图（synthesized view）：hertz 对未显式设置的 Content-Type
//     （缺省 "text/plain; charset=utf-8"）/Content-Length/Connection 等键
//     经 peek/VisitAll 合成缺省值——「不存在返回空串」契约对这类键不成立
//     （读到的将是 hertz 实际会发送的合成值，与 wire 形态一致）；判断
//     「是否显式设置」不要依赖 Header 读面；
//   - Set 上游 quirk：hertz 的 Set 对同 key 多值**只替换首个出现**
//     （setArgBytes 首中即返，不做整体收敛）——transport.Header 的
//     「覆盖为单值」契约在多值键上由调用方先 Del 再 Set 达成（Del 已
//     EqualFold 收口，变体键删得净）。
//
// Keys() 返回 hertz 规范化后的 canonical 键（排序去重），消费方应以大小写
// 不敏感方式回读（Get/Values 已 EqualFold），不要拿 Keys() 的键做原文匹配。
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
// Kind=http；Endpoint 为请求寻址信息（= 请求的 Host 头/URI host——**客户端
// 可控的请求面输入**，不得用于安全判定/ACL/本机地址推断，#156）；Operation
// 为请求 PATH。返回的 Transporter 持有 RequestContext 引用，生命周期与请求
// 一致。
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

// Endpoint 返回请求寻址信息：hertz 实现为请求的 Host（HTTP/1.1 Host 头或
// absolute-form URI 的 authority——**客户端可控的请求面输入**，携带
// "Host: evil.com" 这类任意值会原样流出，#156）。消费方只可将其用于日志/
// 观测维度（且要意识到不可信），不得用于 ACL、安全判定或本机地址推断；
// 「监听地址」语义需要消费方注入服务端自身信息，RequestContext 上拿不到。
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

// Get 返回 key 的第一个值，不存在返回空串（根包 transport.Header 契约）。
//
// 键匹配 EqualFold（#163/#155）：经 VisitAll 自行匹配，不依赖 hertz 键
// 规范化开关——DisableNormalizing 下 hertz 自身 Get 退化大小写敏感，本
// 适配器仍兑现 EqualFold。请求侧 Cookie 是 RFC 6265 单头文法，Get 返回
// 合并串语义正确（#154）。
func (a *requestHeader) Get(key string) string {
	return firstFoldMatch(a.h.VisitAll, key)
}

// Values 返回 key 的全部值，键匹配与 Get 同一口径——大小写不敏感（#163）。
//
// 此前为原文比较：VisitAll 产出的键是 hertz 规范化后的 canonical 形态
// （如 "X-Trace-Id"），调用方传 "x-trace-id" 这类大小写变体时 Get 命中、
// Values 返回 nil——同一适配器两套键口径，Get 命中而 Values 落空。
func (a *requestHeader) Values(key string) []string {
	return foldMatches(a.h.VisitAll, key)
}

// Set 直传 hertz。注意 hertz 上游 quirk：Set 对同 key 多值**只替换首个
// 出现**（setArgBytes 首中即返，不做整体收敛）——transport.Header 的
// 「覆盖为单值」契约在多值键上由调用方先 Del 再 Set 达成（Del 已 EqualFold
// 收口，变体键删得净，#155）。本适配是两个既有实现（middleware/
// transporter.go、hertz/transportx）的共同口径，单源不另起语义（test 钉住
// 透传行为）。Set 的替换作用面仍随 hertz 键规范化开关走（normalize-off 时
// 大小写变体的既有值不会被本次 Set 替换——先 Del 再 Set 即收敛）。
func (a *requestHeader) Set(key, value string) { a.h.Set(key, value) }

func (a *requestHeader) Add(key, value string) { a.h.Add(key, value) }

// Del 删除 key 的全部值（根包契约）。键匹配 EqualFold（#155）：先 VisitAll
// 收集全部大小写变体的实际键再逐个删——hertz 自身 Del 在 DisableNormalizing
// 下只匹配原文键，变体删不掉（Del 与 Get/Values 三方口径分裂的最后一环）。
func (a *requestHeader) Del(key string) {
	for _, actual := range foldKeys(a.h.VisitAll, key) {
		a.h.Del(actual)
	}
}

// Keys 返回全部键（已排序，保证确定性）。键为 hertz 规范化的 MIME canonical
// 形态（如 Set("X-Trace-ID",…) 后 Keys() 产出 "X-Trace-Id"）——消费方应以
// 大小写不敏感方式回读（Get/Values 已 EqualFold，#163），不要拿 Keys() 的
// 键做原文匹配。
func (a *requestHeader) Keys() []string {
	return keysFromVisitAll(a.h.VisitAll)
}

// replyHeader 适配 hertz ResponseHeader。
type replyHeader struct {
	h *protocol.ResponseHeader
}

// Get 返回 key 的第一个值，不存在返回空串（根包 transport.Header 契约）。
//
// 键匹配 EqualFold（#163/#155，同 requestHeader.Get）。
//
// ⚠️ 值形态（#154）：响应侧 Set-Cookie 的每枚 cookie 是**独立头行**，Get
// 返回第一枚；取全部用 Values——此前 Get 直通 hertz 的 peek，会把全部
// cookie 用 "; " 连成一个 blob 返回，与单枚 cookie 的属性序列无法区分
// （消费方按「取第一个 cookie」解析必错），且与 MapHeader.Get（vs[0]）同
// 接口两种语义。请求侧 Cookie 的合并是 RFC 6265 单头文法、语义正确。
//
// ⚠️ 合成视图：hertz 对未显式设置的 Content-Type 等键合成缺省值
// （peek/VisitAll 一致），「不存在返回空串」对这类键不成立（见包文档）。
func (a *replyHeader) Get(key string) string {
	return firstFoldMatch(a.h.VisitAll, key)
}

// Values 返回 key 的全部值，键匹配与 Get 同一口径——大小写不敏感（#163，
// requestHeader.Values 同病同修）。
func (a *replyHeader) Values(key string) []string {
	return foldMatches(a.h.VisitAll, key)
}

func (a *replyHeader) Set(key, value string) { a.h.Set(key, value) }

// Add 追加（同 key 多值）——与 requestHeader.Add 及 transport.Header.Add
// 语义一致；此前有实现误用 Set（覆盖），后写的值会顶掉先写的值，
// Vary/Set-Cookie 类多值响应头被静默丢值（middleware#99 同源修复）。
func (a *replyHeader) Add(key, value string) { a.h.Add(key, value) }

// Del 删除 key 的全部值（根包契约）。键匹配 EqualFold（#155，同
// requestHeader.Del）——含 Set-Cookie（VisitAll 逐枚产出，Del("Set-Cookie")
// 经 hertz 的 del 特例清空全部 cookie）与大小写变体实际键。
func (a *replyHeader) Del(key string) {
	for _, actual := range foldKeys(a.h.VisitAll, key) {
		a.h.Del(actual)
	}
}

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

// firstFoldMatch 返回首个 EqualFold 命中的值，未命中空串（Get 的实现基座，
// #154/#155：Get=首值 + 大小写不敏感，不依赖 hertz 规范化开关）。
func firstFoldMatch(visitAll func(fn func(k, v []byte)), key string) string {
	var out string
	visitAll(func(k, v []byte) {
		if out == "" && strings.EqualFold(string(k), key) {
			out = string(v)
		}
	})
	return out
}

// foldMatches 返回全部 EqualFold 命中的值（Values 的实现基座，#163）。
func foldMatches(visitAll func(fn func(k, v []byte)), key string) []string {
	var out []string
	visitAll(func(k, v []byte) {
		if strings.EqualFold(string(k), key) {
			out = append(out, string(v))
		}
	})
	return out
}

// foldKeys 收集全部 EqualFold 命中的**实际存储键**（去重保序；Del 的实现
// 基座，#155：规范化关闭时 hertz Del 只匹配原文键，须先收集实际键再逐个删）。
func foldKeys(visitAll func(fn func(k, v []byte)), key string) []string {
	seen := make(map[string]struct{})
	visitAll(func(k, _ []byte) {
		if strings.EqualFold(string(k), key) {
			seen[string(k)] = struct{}{}
		}
	})
	return slices.Sorted(maps.Keys(seen))
}
