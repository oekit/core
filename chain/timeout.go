package chain

import (
	"context"
	"time"
)

// Timeout 返回超时中间件：为下游派生带 deadline 的 ctx，超时先到即返回
// context.DeadlineExceeded。
//
// 语义边界：与 go-kratos 相同——若下游 handler 不理会 ctx 取消（阻塞在
// 不受 ctx 控制的 IO 上），超时返回后其 goroutine 仍在后台运行至自然结束，
// 本中间件无法强制中断。
func Timeout(d time.Duration) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req any) (any, error) {
			ctx, cancel := context.WithTimeout(ctx, d)
			defer cancel()

			type result struct {
				resp any
				err  error
			}
			done := make(chan result, 1) // 带缓冲：超时路径不阻塞下游完成回传
			go func() {
				r, err := next(ctx, req)
				done <- result{resp: r, err: err}
			}()

			select {
			case r := <-done:
				return r.resp, r.err
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}
}
