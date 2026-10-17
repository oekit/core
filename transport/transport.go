// Package transport 定义协议无关的传输层上下文抽象（参照 go-kratos transport）。
//
// Transporter 携带 Kind/Endpoint/Operation 与 Header 读写能力，经 context
// 在服务端/客户端两侧注入。中间件、日志、trace 透传等通用逻辑面向本包编程，
// 即可以同一份实现跨 HTTP（hertz）与 RPC（kitex）复用；各框架适配层负责
// 将自己的请求上下文适配为 Transporter 并注入。
//
// 本包为框架无关的底层契约包：只依赖标准库，不得 import 任何框架或 oekit 模块。
package transport

import "context"

// Kind defines the type of Transport.
type Kind string

func (k Kind) String() string { return string(k) }

// 传输类型常量。
const (
	// KindHTTP 为 HTTP 传输（hertz）。
	KindHTTP Kind = "http"
	// KindGRPC 为 gRPC 类 RPC 传输（kitex）。
	KindGRPC Kind = "grpc"
)

// Header 是协议无关的头部读写接口。
// 各框架适配层负责映射到自己的 header 实现（注意各框架的大小写规范化差异）。
type Header interface {
	// Get 返回 key 的第一个值，不存在返回空串。
	Get(key string) string
	// Values 返回 key 的全部值。
	Values(key string) []string
	// Set 覆盖 key 的值为单值。
	Set(key, value string)
	// Add 追加 key 的一个值。
	Add(key, value string)
	// Del 删除 key 的全部值。
	Del(key string)
	// Keys 返回全部 key（实现应排序，保证确定性）。
	Keys() []string
}

// Transporter 是放在 context 里的传输层信息。
type Transporter interface {
	// Kind 传输类型（http/grpc）。
	Kind() Kind
	// Endpoint 服务端为监听地址（如 http://127.0.0.1:8000），
	// 客户端为发现目标（如 discovery:///my-service）。
	Endpoint() string
	// Operation 全方法选择器：http 为请求 PATH，grpc 为 /pkg.Service/Method。
	Operation() string
	// RequestHeader 请求头。
	RequestHeader() Header
	// ReplyHeader 响应头；仅服务端实现有效，客户端实现返回只读空实现。
	ReplyHeader() Header
}

type (
	serverTransportKey struct{}
	clientTransportKey struct{}
)

// NewServerContext 返回携带服务端 Transporter 的 context。
func NewServerContext(ctx context.Context, tr Transporter) context.Context {
	return context.WithValue(ctx, serverTransportKey{}, tr)
}

// FromServerContext 取出 context 中的服务端 Transporter。
func FromServerContext(ctx context.Context) (tr Transporter, ok bool) {
	tr, ok = ctx.Value(serverTransportKey{}).(Transporter)
	return
}

// NewClientContext 返回携带客户端 Transporter 的 context。
func NewClientContext(ctx context.Context, tr Transporter) context.Context {
	return context.WithValue(ctx, clientTransportKey{}, tr)
}

// FromClientContext 取出 context 中的客户端 Transporter。
func FromClientContext(ctx context.Context) (tr Transporter, ok bool) {
	tr, ok = ctx.Value(clientTransportKey{}).(Transporter)
	return
}
