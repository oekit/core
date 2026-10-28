package chain

import (
	"context"
	"runtime/debug"
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
				// **在 goroutine 内 recover**：本中间件把下游放到新 goroutine 执行，
				// 而 Recovery 的 recover 装在**调用方** goroutine 上 ⇒ 下游 panic
				// 根本到不了 Recovery，会**打死整个进程**（panic 发生在无人 recover
				// 的 goroutine 里）。canonical 组合
				// `chain.Finalize(h, chain.Recovery(), chain.Timeout(d))` 正是这个
				// 顺序 ⇒ Recovery 的 godoc「下游 panic 被转换为 *PanicError 返回…
				// 而非进程崩溃」在组合使用时是假的。
				//
				// 恢复成 *PanicError 返回 ⇒ 外层的 Recovery 认得它（Recovery 接受
				// *PanicError），错误语义与直接调用 Recovery 一致。
				defer func() {
					if r := recover(); r != nil {
						done <- result{err: &PanicError{Value: r, Stack: string(debug.Stack())}}
					}
				}()
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
