// Package chain 定义 transport 无关的中间件链原语（参照 go-kratos middleware）。
//
// Handler 签名 func(ctx, req any) (any, error) 与 nns-endpoint.Endpoint、
// go-kratos Handler 同形：一份中间件（recovery/timeout/访问日志等）即可在
// hertz、kitex、nns-endpoint 各形态间经桥接复用。
//
// 本包为框架无关的底层契约包：只依赖标准库，不得 import 任何框架或 oekit 模块。
package chain

import "context"

// Handler 是被中间件链调用的处理器。
type Handler func(ctx context.Context, req any) (any, error)

// Middleware 是 transport 无关中间件：包装 next Handler。
type Middleware func(Handler) Handler

// Chain 把 m 按顺序包在 next 外层：Chain(a, b)(next) == a(b(next))。
// 空链返回透传。
func Chain(m ...Middleware) Middleware {
	return func(next Handler) Handler {
		for i := len(m) - 1; i >= 0; i-- {
			next = m[i](next)
		}
		return next
	}
}

// Finalize 等价 Chain(m...)(terminal)，惯用 sugar。
func Finalize(terminal Handler, m ...Middleware) Handler {
	return Chain(m...)(terminal)
}
