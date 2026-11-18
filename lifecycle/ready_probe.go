package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/oekit/pkg/contextx"
	pkgerrors "github.com/oekit/pkg/errors"
)

// 探活时序默认值：与 hertz/kitex bootstrap 两侧下沉前的实现逐值一致
// （defaultReadyProbeTimeout=5s / DialTimeout=500ms / 重试 sleep=100ms），
// 下沉不改行为，消费方（hertz#327 / kitex 联动 PR）迁移后日志与时间观感不漂移。
const (
	// DefaultProbeTimeout 是未配置 WithProbeTimeout 时的默认探活总超时
	// （含等就绪门耗时）。探测类默认值 ≤10s（AGENTS.md 超时区间）。
	DefaultProbeTimeout = 5 * time.Second
	// DefaultDialTimeout 是单次 TCP dial 的默认超时。
	DefaultDialTimeout = 500 * time.Millisecond
	// DefaultRetryInterval 是两次 dial 之间的默认重试间隔。
	DefaultRetryInterval = 100 * time.Millisecond
)

// ErrProbeAborted 是「探活被正常关停打断」的哨兵错误（#48，#208 自
// hertz/kitex bootstrap 两侧同名哨兵下沉，文案逐字一致）。
//
// 语义拆分：**探活失败**（启动失败 ⇒ 按失败处理、请求关机）与**探活被关停打断**
// （正常终止 ⇒ 预期路径）是两件事。关停不是失败——调用方用 errors.Is 判定后
// 只记一条 Info，且不再请求关机（进程本就在关停，再请求一次是多余动作；旧实现
// 把关停记成 ERROR「ready probe failed」，值班的人会误以为服务启动失败）。
//
// 该错误在探活 ctx 被取消时由 Run 返回；探活超时等真正的启动失败仍返回普通
// error（可按错误文本/类型继续分流，但分流不应依赖文本——三类终态的判定入口
// 是 errors.Is + Run 的返回值契约，见 Run）。
var ErrProbeAborted = errors.New("ready probe aborted: app shutting down")

// ErrProbeDisabled 是「探活被显式禁用」的哨兵错误：WithProbeTimeout 传负数时
// Disabled() 为 true，Run 不调用 Target、不发任何 dial、不等就绪门，直接返回
// 本哨兵——由调用方改走「启动 server 之前同步就绪回调」路径（对称 hertz/kitex
// 的 WithReadyProbeTimeout 负数语义，#129/#146）。禁用探活的典型动机恰是
// 「addr 不可拨号」场景（自定义 listener / 不可路由主机名）：若仍照跑探活，
// 必然失败并触发关机——被禁用的探活反而把服务在启动期杀掉。
var ErrProbeDisabled = errors.New("ready probe disabled")

// ReadyProbeOption 配置 ReadyProbe 的函数选项。
type ReadyProbeOption func(*ReadyProbe)

// ReadyProbe 就绪探活器：循环 TCP dial 探活目标地址，可建连（并通过退出复核）
// 后执行就绪回调。
//
// 自 hertz/kitex bootstrap 的 RunServer 探活骨架下沉（oekit/core#208）——两侧
// 原本各持一份逐字平行的实现（defaultReadyProbeTimeout=5s、errProbeAborted
// 哨兵、probeAndReady 的 dial 循环：500ms DialTimeout / deadline 判定 /
// ctx.Err→哨兵 / 100ms 重试 sleep），本类型是它的单源机制件；hertz/kitex 侧
// 消费方联动 PR 跟进迁移，领域皮（各自的目标解析、就绪门来源、日志）留在
// 各自仓内。本包为框架无关底层契约包，探活只用标准库 net/context/time。
//
// 典型用法（server 的监听端口在 Run 内部才绑定，探活必须与 server 启动并发，
// #43/#38——先探活后启动必然 connection refused，探活必超时）：
//
//	probe := lifecycle.NewReadyProbe() // 默认 5s 总超时 / 500ms dial / 100ms 重试
//	go func() {
//	    err := probe.Run(probeCtx, lifecycle.ProbeRequest{
//	        Target:  func(ctx context.Context) (string, string) { return "tcp", ":9090" },
//	        Started: started,  // server 进入 serving 后 close（nil = 立即）
//	        Exited:  runDone,  // server 退出即 close（nil = 不复核）
//	        OnReady: func(ctx context.Context) { /* 延迟注册等就绪动作 */ },
//	    })
//	    switch {
//	    case err == nil:
//	    case errors.Is(err, lifecycle.ErrProbeAborted):
//	        // 正常关停打断：预期路径，记 Info，不请求关机
//	    default:
//	        // 启动失败（超时/复核失败/回调 panic）：请求优雅关机
//	    }
//	}()
//
// # 返回值契约
//
// Run 阻塞直到四类终态之一，调用方按 errors.Is 分流、不要解析错误文本：
//
//   - nil：探活通过，OnReady（若设置）已完成；
//   - errors.Is(err, ErrProbeAborted)：探活被正常关停打断（ctx 取消）——预期
//     路径，只记 Info，不要再请求关机；
//   - errors.Is(err, ErrProbeDisabled)：探活被显式禁用（WithProbeTimeout 负数）
//     ——Run 未产生任何副作用；调用方应在启动 server **之前**同步执行就绪
//     回调（Disabled() 为 true 时本可以完全不调 Run，此哨兵是防御性出口）；
//   - 其他 error：**启动失败**（探活超时 / 退出复核失败 / OnReady panic），
//     按启动失败处置（如请求优雅关机）——dial 失败的根因经 %w 保留在链上。
//
// # 超时预算
//
// 探活超时（WithProbeTimeout，默认 DefaultProbeTimeout=5s）是**总预算**：等
// 就绪门与 dial 重试循环共享同一 deadline。单次 dial 另受 WithDialTimeout
// （默认 500ms）约束，失败按 WithRetryInterval（默认 100ms）重试，直至总超时。
// 上报的超时错误携带最后一次 dial 错误（%w），启动失败可归因。
//
// 并发安全：配置在构造期固定，多个 goroutine 可同时 Run（互不影响）；Run 自身
// 不内部起 goroutine，由调用方决定在哪个协程执行（hertz/kitex 侧经
// concurrency/async.GoSafe 起，panic 由其 recover 收编——Run 返回 error 而非
// panic，OnReady 的 panic 已在 Run 内转 error，见 ProbeRequest.OnReady）。
type ReadyProbe struct {
	probeTimeout time.Duration // <0 禁用；0/未设置 = DefaultProbeTimeout
	dialTimeout  time.Duration // 0/未设置 = DefaultDialTimeout（负数构造期拒绝）
	retry        time.Duration // 0/未设置 = DefaultRetryInterval（负数构造期拒绝）
}

// NewReadyProbe 用默认时序（5s 总超时 / 500ms 单次 dial / 100ms 重试）构造
// 探活器，可选 opts 覆盖。构造期非法值 fail-fast：WithDialTimeout /
// WithRetryInterval 传负数直接 panic（单次 dial 时长无「禁用」语义，静默回退
// 默认值会掩盖调用方 bug）；「禁用探活」只用 WithProbeTimeout 负数表达。
func NewReadyProbe(opts ...ReadyProbeOption) *ReadyProbe {
	p := &ReadyProbe{
		probeTimeout: DefaultProbeTimeout,
		dialTimeout:  DefaultDialTimeout,
		retry:        DefaultRetryInterval,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// WithProbeTimeout 配置探活总超时（含等就绪门耗时；默认 DefaultProbeTimeout=
// 5s，传 0 或未设置同为默认）。
//
// **设为负数禁用探活**（对称 hertz/kitex 的 WithReadyProbeTimeout 负数语义，
// #129/#146）：Disabled() 变为 true，Run 不 dial、不等就绪门、不调 Target，
// 直接返回 ErrProbeDisabled。注意语义方向不能用「probeEnabled bool」表达——
// bool 零值 false 会让「未配置」被误判成禁用，而缺省语义是「探活 + 默认 5s
// 超时」（两侧下沉前同款教训，#129）。「禁用」是彻底禁用：零探活协程、零 dial
// （即使探活目标可建连也不碰）、探活失败触发关机的路径整个不存在。不推荐
// 禁用：回调时端口可能尚未监听（无原生就绪信号可用），消费方需自行接受该窗口。
func WithProbeTimeout(d time.Duration) ReadyProbeOption {
	return func(p *ReadyProbe) { p.probeTimeout = d }
}

// WithDialTimeout 配置单次 TCP dial 的超时（默认 DefaultDialTimeout=500ms，
// 传 0 或未设置同为默认）。负数是构造期错误：panic——dial 时长没有「禁用」
// 语义，要表达「不做探活」请用 WithProbeTimeout 负数。
func WithDialTimeout(d time.Duration) ReadyProbeOption {
	return func(p *ReadyProbe) {
		if d < 0 {
			panic("lifecycle: WithDialTimeout 不接受负数（单次 dial 超时无「禁用」语义；" +
				"要禁用探活请用 WithProbeTimeout 负数）")
		}
		p.dialTimeout = d
	}
}

// WithRetryInterval 配置 dial 失败后的重试间隔（默认 DefaultRetryInterval=
// 100ms，传 0 或未设置同为默认）。负数是构造期错误：panic——重试间隔没有
// 「禁用」语义，要禁用探活请用 WithProbeTimeout 负数。
func WithRetryInterval(d time.Duration) ReadyProbeOption {
	return func(p *ReadyProbe) {
		if d < 0 {
			panic("lifecycle: WithRetryInterval 不接受负数（重试间隔无「禁用」语义；" +
				"要禁用探活请用 WithProbeTimeout 负数）")
		}
		p.retry = d
	}
}

// Disabled 报告探活是否被显式禁用（WithProbeTimeout 传负数）。为 true 时
// Run 返回 ErrProbeDisabled 且无任何副作用，调用方应改走「启动 server 之前
// 同步就绪回调」路径。
func (p *ReadyProbe) Disabled() bool {
	return p.probeTimeout < 0
}

// ProbeRequest 描述一次探活的输入。所有字段可选：Target 缺省视为无拨号目标，
// Started 缺省视为立即就绪，Exited 缺省跳过复核，OnReady 缺省为纯可达性探活
// ——四者全缺省时 Run 立即返回 nil（调用方至少应提供其一，否则探活没有意义）。
type ProbeRequest struct {
	// Target 返回本次探活的目标 (network, addr)。Run 开始时调用一次；
	// 返回空 addr 表示**无拨号目标**（对称 hertz #131：server 将在 Run 内绑定
	// 随机端口、地址事先未知）——探活退化为「等 Started 就绪门通过后直接回调
	// OnReady」，不发任何 dial，仍受同一总超时与关停语义约束。network 为空按
	// "tcp" 处理。
	// 用回调而非静态字段，是为了让消费方在探活开始时才解析地址（hertz 侧
	// 监听地址在 server.Run 内部才绑定，配置随时可能被并发读取）。
	Target func(ctx context.Context) (network, addr string)

	// Started 是**正向就绪门**：探活只在该通道关闭（server 已进入 serving）后
	// 才发起 dial / 回调 OnReady。nil = 无门，立即进入拨号。
	// 用途：端口绑定发生在 server.Run 内部，探活必须与 server 启动并发
	// （#43/#38）；消费方在 server 的就绪钩子里 close 本通道（hertz 侧由
	// IsRunning 轮询转成 close，kitex 侧由 RegisterStartHook 直接 close）。
	// 等门期间受同一总超时与关停语义约束：超时报「server not started」，
	// ctx 取消返回 ErrProbeAborted，Exited 关闭按启动失败返回（server 在探活
	// 通过前就退出了，不会有人来 serve）。
	Started <-chan struct{}

	// Exited 是**退出复核信号**：建连成功后复核该通道未关闭再回调——探活连上
	// 的可能是**他人**占着同端口的监听器，而本进程 server 实际已启动失败
	// （端口归属 TOCTOU，#120/#131），此时回调会把不属于自己的实例注册到注册
	// 中心。nil = 无复核。等门期间该通道关闭同样按启动失败处理。
	Exited <-chan struct{}

	// OnReady 就绪回调，探活通过后以 Run 收到的 ctx 调用一次；nil 合法（纯
	// 可达性探活）。panic 在 Run 内被转为 error 返回（#152）：OnReady 是就绪/
	// 注册动作（典型：延迟注册注册中心），panic 意味着就绪路径已炸——若不
	// 收编，调用方「探活失败 ⇒ 请求关机」的处置会被 panic 跳过，server 以
	// 「探活已过、回调丢失、永不注册」的半启动形态继续 serve。panic 值经
	// pkg/errors.Recovered 收编（core#136 单源口径）：panic(err) 的原始错误
	// 身份不丢，errors.Is 可直达。
	OnReady func(ctx context.Context)
}

// Run 执行一次探活，阻塞直到就绪回调完成、探活失败、探活被关停打断或探活被
// 禁用——四类终态与分流规则见类型文档「返回值契约」。nil ctx 按 Background
// 处理（core#159 全仓口径）。
func (p *ReadyProbe) Run(ctx context.Context, req ProbeRequest) error {
	// 口径单源：pkg/contextx.OrBackground（core#159：nil = Background）。
	ctx = contextx.OrBackground(ctx)
	if p.Disabled() {
		// 禁用是彻底禁用（#129/#146）：在 Target 之前返回——Target 不被调用，
		// dial 循环不可达，即使探活目标可建连也不碰。
		return ErrProbeDisabled
	}

	network, addr := "", ""
	if req.Target != nil {
		network, addr = req.Target(ctx)
	}
	if network == "" {
		network = "tcp"
	}

	timeout := p.timeout()
	deadline := time.Now().Add(timeout)
	if err := p.waitGate(ctx, req, deadline, addr, timeout); err != nil {
		return err
	}
	if addr == "" {
		// 无拨号目标：就绪门通过即视为就绪，直接回调（对称 hertz #131）——
		// 仍受同一总超时与关停语义约束（见 waitGate）。
		return p.callOnReady(ctx, req)
	}
	return p.dialLoop(ctx, req, deadline, network, addr, timeout)
}

// waitGate 等待正向就绪门（Started），nil = 无门立即通过。
//
// 关停优先（#48）：每个失败分支先复核 ctx.Err()，避免「关停与超时/退出同时
// 发生」时把正常关停误报成启动失败。Exited 为 nil 时该分支永不就绪（nil
// channel 的 receive 不参与 select 竞争），无复核信号的门等待照常工作。
func (p *ReadyProbe) waitGate(ctx context.Context, req ProbeRequest, deadline time.Time, addr string, timeout time.Duration) error {
	if req.Started == nil {
		return nil
	}
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	select {
	case <-req.Started:
		return nil
	case <-req.Exited:
		if ctx.Err() != nil {
			return ErrProbeAborted
		}
		// server 在探活通过前退出（典型：端口归属预检后被抢占 ⇒ bind 失败
		// ⇒ Run 返回 error）：按启动失败处理，绝不回调 OnReady（#120）。
		return fmt.Errorf("ready probe: server exited before ready probe passed, addr=%s", addr)
	case <-ctx.Done():
		return ErrProbeAborted
	case <-timer.C:
		if ctx.Err() != nil {
			return ErrProbeAborted
		}
		if addr == "" {
			return fmt.Errorf("ready probe timeout after %s: server not started (no dial target)", timeout)
		}
		return fmt.Errorf("ready probe timeout after %s, addr=%s: server not started", timeout, addr)
	}
}

// dialLoop 循环 TCP dial 探活目标，可建连并通过退出复核后执行就绪回调。
// 自 hertz/kitex probeAndReady 的 dial 循环逐行下沉（#208），守则：
//
//   - 关停优先于超时判定（#48）：ctx 已取消 ⇒ 探活是被正常关停打断，返回
//     哨兵 ErrProbeAborted，而不是「探活超时」——两者在调用方是不同语义
//     （正常终止 vs 启动失败）。每次判定超时前先看 ctx.Err()。
//   - 建连成功后复核 Exited（#120/#131）：端口被他人占用时，探活连上的是
//     对方的监听器，而本进程的 server 实际已启动失败，不得回调 OnReady。
//   - 单次 dial 受 DialTimeout 约束，失败按 RetryInterval 重试，直到总超时；
//     上报的超时错误保留最后一次 dial 错误（%w）。
func (p *ReadyProbe) dialLoop(ctx context.Context, req ProbeRequest, deadline time.Time, network, addr string, timeout time.Duration) error {
	dialer := net.Dialer{Timeout: p.dialTimeoutValue()}
	for {
		conn, err := dialer.DialContext(ctx, network, addr)
		if err == nil {
			_ = conn.Close()
			if req.Exited != nil {
				select {
				case <-req.Exited:
					return fmt.Errorf("ready probe connected but server already exited, addr=%s", addr)
				default:
				}
			}
			if ctx.Err() != nil {
				// 建连与回调之间 ctx 被取消：探活被正常关停打断，不回调
				// （#48；kitex 侧原有守则，hertz 侧同样受益）。
				return ErrProbeAborted
			}
			return p.callOnReady(ctx, req)
		}
		// 关停优先于超时：ctx 已取消 ⇒ 探活被正常关停打断，不是启动失败（#48）。
		if ctx.Err() != nil {
			return ErrProbeAborted
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("ready probe timeout after %s, addr=%s: %w", timeout, addr, err)
		}
		select {
		case <-ctx.Done():
			return ErrProbeAborted
		case <-time.After(p.retryInterval()):
		}
	}
}

// callOnReady 执行就绪回调并把 panic 转为 error 返回（#152）。panic 值收编
// 单源走 pkg/errors.Recovered（core#136，与 manager.go 的 runFunc/runCloser
// 同款口径）：panic(err) 时身份不丢（Unwrap 透传，errors.Is/As 可直达业务侧
// panic 出去的原始错误），文本形态 "panic: %T: %v"。
func (p *ReadyProbe) callOnReady(ctx context.Context, req ProbeRequest) (err error) {
	if req.OnReady == nil {
		return nil
	}
	defer func() {
		if perr := pkgerrors.Recovered(recover()); perr != nil {
			err = pkgerrors.Wrapf(perr, "ready probe onReady panicked")
		}
	}()
	req.OnReady(ctx)
	return nil
}

// timeout 归一探活总超时：0/未设置 = DefaultProbeTimeout（#129）。负数不会
// 走到这里（Disabled 时 Run 在入口直接返回 ErrProbeDisabled），不存在
// 「负数悄悄换默认值继续探」的回退。
func (p *ReadyProbe) timeout() time.Duration {
	if p.probeTimeout > 0 {
		return p.probeTimeout
	}
	return DefaultProbeTimeout
}

// dialTimeoutValue / retryInterval 归一单次 dial 超时与重试间隔：0/未设置 =
// 对应 Default 常量。负数不会出现（WithDialTimeout / WithRetryInterval 构造期
// panic，NewReadyProbe 的默认值恒为正）。
func (p *ReadyProbe) dialTimeoutValue() time.Duration {
	if p.dialTimeout > 0 {
		return p.dialTimeout
	}
	return DefaultDialTimeout
}

func (p *ReadyProbe) retryInterval() time.Duration {
	if p.retry > 0 {
		return p.retry
	}
	return DefaultRetryInterval
}
