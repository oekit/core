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
//
// 钩子在**panic 隔离**下被调用：钩子内部 panic 被吞掉、本次上报丢弃，
// 不影响原始 panic 的错误返回（见 reportIsolated 的说明）。
//
// 与 middleware/chainmw.WithRecoveryReporter 的差异：那边参数是
// *PanicError 且带 logx 日志；本包为了保持零依赖与既有调用面不做统一
// （改签名对显式声明函数类型的调用方是编译期破坏）。
func WithPanicReporter(fn func(ctx context.Context, req any, pv PanicError)) RecoveryOption {
	return func(c *recoveryConfig) { c.reporter = fn }
}

// reportIsolated 在 panic 隔离下调用钩子（core#135 单源：chain 包内所有
// 「钩子自身可能 panic」的调用点——Recovery 的上报与 Timeout 的迟到 panic
// 上报——共用这一个隔离壳，参数形态差异由调用点闭包包一层；「钩子自身
// panic 会打死进程」的论证只留这一份）。
//
// 为什么必须有隔离：Recovery 的 report 调用发生在**已经 recover 过的 defer
// 闭包内部**——该闭包里再 panic，同级的其他 defer 已经执行完毕，没有任何
// recover 会接住它 ⇒ **整个进程退出**。而钩子恰恰是最可能 panic 的
// 代码（第三方监控 SDK、序列化 req 失败、并发 map 写入等）。
// middleware/chainmw 的同名钩子早有 reportIsolated，本包这个原始实现漏了。
//
// 隔离后钩子自身的失败**静默丢弃**：本包是零依赖的框架无关契约包（只依赖
// 标准库），没有日志后端可写。需要观测「reporter 自己也挂了」请改用
// middleware/chainmw（它有 logx 可记 reporter_panic）。这里优先级明确：
// 保住进程 > 记录钩子失败。
func reportIsolated(report func()) {
	defer func() { _ = recover() }()
	report()
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
						reportIsolated(func() { cfg.reporter(ctx, req, pv) })
					}
					resp, err = nil, &pv
				}
			}()
			resp, err = next(ctx, req)
			// 下游以 *PanicError **形式返回**（例如下游中间的 Timeout 已把 panic
			// 收编成错误再往上抛）⇒ 同样走上报钩子，与「直接 panic」的路径
			// 口径一致（否则这类 panic 不会出现在 Sentry 等上报里）。
			if pe, ok := err.(*PanicError); ok && cfg.reporter != nil {
				reportIsolated(func() { cfg.reporter(ctx, req, *pe) })
			}
			return resp, err
		}
	}
}
