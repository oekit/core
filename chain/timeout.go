package chain

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"
)

// TimeoutOption 配置 Timeout 中间件。
type TimeoutOption func(*timeoutConfig)

type timeoutConfig struct {
	latePanic func(ctx context.Context, pv *PanicError)
}

// WithLatePanicReporter 注入「超时已返回后下游才 panic」的上报钩子。
//
// 未注入时走 stdlib slog（component=chain.timeout）；需要对接 Sentry 等
// 监控体系时注入本钩子。钩子在**panic 隔离**下调用（同 reportIsolated）。
func WithLatePanicReporter(fn func(ctx context.Context, pv *PanicError)) TimeoutOption {
	return func(c *timeoutConfig) { c.latePanic = fn }
}

// reportLate 上报「无人接收的迟到 panic」。钩子形态与 Recovery 的上报
// 不同（*PanicError 指针），panic 隔离共用 reportIsolated（core#135 单源）。
func (c *timeoutConfig) reportLate(ctx context.Context, pv *PanicError) {
	if c.latePanic != nil {
		reportIsolated(func() { c.latePanic(ctx, pv) })
		return
	}
	// 默认出口：超时路径的迟到 panic 若不落地就等于**彻底消失**——排障时
	// 只剩「超时」一个假象，而真实原因是下游在 ctx 取消后崩了（典型形态：
	// 取消后继续写已关闭的资源、nil 解引用）。这里必须留下痕迹。
	slog.Default().With("component", "chain.timeout").Error(
		"[panic] 超时返回后下游才 panic（错误已无调用方接收）",
		"panic", pv.Value,
		"stack", pv.Stack,
	)
}

// Timeout 返回超时中间件：为下游派生带 deadline 的 ctx，超时先到即返回
// context.DeadlineExceeded。
//
// d 的语义与 lifecycle.Manager 的「显式无超时」口径统一（core#86 ⇒ core#117）：
//   - d > 0：正常超时保护；
//   - d == 0：**显式无超时**——原样透传 next（不派生 ctx、不加 deadline，
//     下游回到调用方 goroutine 上执行）。配置零值透传（如
//     `chain.Timeout(cfg.ReadTimeout)` 的 cfg 未配置）不再等价「每请求 0ms
//     瞬死于 DeadlineExceeded」；要表达「无超时保护」就显式传 0。
//     透传形态下 WithLatePanicReporter 无意义（无独立 goroutine 即无迟到
//     panic），Recovery 的 recover 在调用方 goroutine 上恢复生效；
//   - d < 0：构造期 panic（fail-loud，AGENTS.md「Option 非法值总则①」）。
//     负值只可能来自上游计算错误，静默产出「立即到期」的中间件会让全量
//     请求瞬死且没有任何指向性日志（core#117）。
//
// 语义边界：与 go-kratos 相同——若下游 handler 不理会 ctx 取消（阻塞在
// 不受 ctx 控制的 IO 上），超时返回后其 goroutine 仍在后台运行至自然结束，
// 本中间件无法强制中断。
//
// 迟到结果（超时后下游才完成）的处置：
//   - 普通结果 ⇒ 丢弃（超时胜出，既有语义不变）；
//   - **panic ⇒ 走上报通道**（WithLatePanicReporter，缺省 stdlib slog）。
//     不这么做的话，`Recovery(·)` 装在调用方 goroutine 上、而本中间件把下游
//     放进了新 goroutine ⇒ 迟到的 *PanicError* 只写进一个**没人读**的缓冲
//     channel ⇒ 不进错误链、不上报，观测面完全丢失。
func Timeout(d time.Duration, opts ...TimeoutOption) Middleware {
	if d < 0 {
		panic(fmt.Sprintf(
			"chain.Timeout: 负值超时 %v（fail-loud，core#117）——负值只会来自上游计算错误，"+
				"静默放行会产出立即到期的中间件（全量请求 DeadlineExceeded）；0 表示显式无超时，正值为超时预算",
			d,
		))
	}
	if d == 0 {
		// 显式无超时（core#117，与 lifecycle core#86 口径统一）：恒等中间件，
		// 不把下游放进独立 goroutine、不派生带 deadline 的 ctx。
		return func(next Handler) Handler { return next }
	}
	cfg := &timeoutConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	return func(next Handler) Handler {
		return func(ctx context.Context, req any) (any, error) {
			ctx, cancel := context.WithTimeout(ctx, d)
			defer cancel()

			type result struct {
				resp any
				err  error
			}
			done := make(chan result, 1) // 带缓冲：超时路径不阻塞下游完成回传

			// returned 标记「调用方已放弃等待」。下游 goroutine 据此判断自己
			// 那笔结果是否还有人接收。**「读走 done」与「置位 returned」在
			// 同一把锁下**，所以二者恰好一方生效：既不双报（迟到 panic 既
			// 被读走又被上报），也不静默丢弃（两边都以为对方管）。
			//
			// 只用 atomic.Bool 不够：goroutine 侧「发送 → 检查」与调用方侧
			// 「置位 → 读取」是两对操作的交叉，无锁时存在「发进缓冲、却
			// 无人读且不认为自己是迟到」的窗口。
			var mu sync.Mutex
			returned := false

			deliver := func(res result, pv *PanicError) {
				mu.Lock()
				abandoned := returned
				if !abandoned {
					done <- res // 缓冲 1 且只写一次 ⇒ 永不阻塞
				}
				mu.Unlock()
				if abandoned && pv != nil {
					cfg.reportLate(ctx, pv)
				}
			}

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
						pv := &PanicError{Value: r, Stack: string(debug.Stack())}
						deliver(result{err: pv}, pv)
					}
				}()
				r, err := next(ctx, req)
				deliver(result{resp: r, err: err}, nil)
			}()

			select {
			case r := <-done:
				return r.resp, r.err
			case <-ctx.Done():
				mu.Lock()
				returned = true
				var late *PanicError
				select {
				case r := <-done:
					// 下游恰好在此刻完成。panic 照样没人接收 ⇒ 上报；
					// 普通结果按既有语义丢弃（超时胜出）。
					if pv, ok := r.err.(*PanicError); ok {
						late = pv
					}
				default:
				}
				mu.Unlock()
				if late != nil {
					cfg.reportLate(ctx, late)
				}
				return nil, ctx.Err()
			}
		}
	}
}
