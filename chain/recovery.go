package chain

import (
	"context"
	"fmt"
	"runtime/debug"
)

// PanicError 是 Recovery 把下游 panic 转换出的错误。
type PanicError struct {
	// Value 是 panic 的原始值。
	Value any
	// Stack 是 panic 时的调用栈（debug.Stack）。
	Stack string
}

func (e *PanicError) Error() string { return fmt.Sprintf("panic: %v", e.Value) }

// RecoveryOption 配置 Recovery 中间件。
type RecoveryOption func(*recoveryConfig)

type recoveryConfig struct {
	reporter func(ctx context.Context, req any, pv PanicError)
}

// WithPanicReporter 注入监控上报钩子（transport 无关版；panic 转换为
// PanicError 后回调）。hertz 侧对接 Sentry 等可在此桥接。
func WithPanicReporter(fn func(ctx context.Context, req any, pv PanicError)) RecoveryOption {
	return func(c *recoveryConfig) { c.reporter = fn }
}

// Recovery 返回 panic 恢复中间件：下游 panic 被转换为 *PanicError 返回，
// 链不中断（调用方拿到 error 而非进程崩溃）。
func Recovery(opts ...RecoveryOption) Middleware {
	cfg := &recoveryConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	return func(next Handler) Handler {
		return func(ctx context.Context, req any) (resp any, err error) {
			defer func() {
				if r := recover(); r != nil {
					pv := PanicError{Value: r, Stack: string(debug.Stack())}
					if cfg.reporter != nil {
						cfg.reporter(ctx, req, pv)
					}
					resp, err = nil, &pv
				}
			}()
			return next(ctx, req)
		}
	}
}
