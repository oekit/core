package lifecycle

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// errBoom 是 OnReady panic 身份保留（pkg/errors.Recovered 单源，core#136）的
// 探针哨兵：panic(errBoom) 出去的错误必须 errors.Is 可直达。
var errBoom = errors.New("onReady boom")

// probeOpenListener 打开一个真实监听中的 TCP 地址（探活必成功）。
func probeOpenListener(t *testing.T) (net.Listener, string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("open listener: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	return ln, ln.Addr().String()
}

// probeRefusedAddr 返回一个当前无人监听的 TCP 地址（探活 dial 立即 refused）。
// 做法：临时监听拿一个随机空闲端口后立刻关闭——端口随即无人绑定，连接被拒。
func probeRefusedAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("open listener for refused addr: %v", err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("close listener for refused addr: %v", err)
	}
	return addr
}

// staticTarget 返回恒定 (network, addr) 的 Target 回调，并统计调用次数
// （禁用语义断言「Target 不被调用」用）。
func staticTarget(network, addr string, calls *int) func(ctx context.Context) (string, string) {
	return func(context.Context) (string, string) {
		if calls != nil {
			*calls++
		}
		return network, addr
	}
}

// TestReadyProbe_DefaultTimingConstants 钉住下沉值：探活时序默认值必须与
// hertz/kitex bootstrap 两侧下沉前逐值一致（#208），迁移后时间观感不漂移。
func TestReadyProbe_DefaultTimingConstants(t *testing.T) {
	if DefaultProbeTimeout != 5*time.Second {
		t.Errorf("DefaultProbeTimeout = %s, want 5s（两侧下沉前 defaultReadyProbeTimeout 逐值）", DefaultProbeTimeout)
	}
	if DefaultDialTimeout != 500*time.Millisecond {
		t.Errorf("DefaultDialTimeout = %s, want 500ms（两侧下沉前 dial timeout 逐值）", DefaultDialTimeout)
	}
	if DefaultRetryInterval != 100*time.Millisecond {
		t.Errorf("DefaultRetryInterval = %s, want 100ms（两侧下沉前重试 sleep 逐值）", DefaultRetryInterval)
	}
}

// TestReadyProbe_TimeoutNormalization 归一口径：0/未设置 = 默认值；负数探活
// 超时 = 禁用（Disabled()）；不存在「负数悄悄换默认值继续探」的回退（#129）。
func TestReadyProbe_TimeoutNormalization(t *testing.T) {
	if p := NewReadyProbe(); p.Disabled() || p.timeout() != DefaultProbeTimeout {
		t.Errorf("未配置: disabled=%v timeout=%s, want false/%s", p.Disabled(), p.timeout(), DefaultProbeTimeout)
	}
	if p := NewReadyProbe(WithProbeTimeout(0)); p.Disabled() || p.timeout() != DefaultProbeTimeout {
		t.Errorf("WithProbeTimeout(0): disabled=%v timeout=%s, want false/%s（0 同为默认）", p.Disabled(), p.timeout(), DefaultProbeTimeout)
	}
	if p := NewReadyProbe(WithProbeTimeout(3 * time.Second)); p.Disabled() || p.timeout() != 3*time.Second {
		t.Errorf("WithProbeTimeout(3s): disabled=%v timeout=%s, want false/3s", p.Disabled(), p.timeout())
	}
	p := NewReadyProbe(WithProbeTimeout(-time.Second))
	if !p.Disabled() || p.timeout() != DefaultProbeTimeout {
		t.Errorf("WithProbeTimeout(-1s): disabled=%v timeout=%s, want true/默认（负数=禁用）", p.Disabled(), p.timeout())
	}
	if p := NewReadyProbe(WithDialTimeout(0)); p.dialTimeoutValue() != DefaultDialTimeout {
		t.Errorf("WithDialTimeout(0): dialTimeout=%s, want 默认 500ms", p.dialTimeoutValue())
	}
	if p := NewReadyProbe(WithRetryInterval(0)); p.retryInterval() != DefaultRetryInterval {
		t.Errorf("WithRetryInterval(0): retry=%s, want 默认 100ms", p.retryInterval())
	}
}

func TestReadyProbe_NegativeDialTimeoutPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("WithDialTimeout 负数必须构造期 panic（fail-fast，AGENTS.md option 总则）")
		}
	}()
	NewReadyProbe(WithDialTimeout(-time.Millisecond))
}

func TestReadyProbe_NegativeRetryIntervalPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("WithRetryInterval 负数必须构造期 panic（fail-fast，AGENTS.md option 总则）")
		}
	}()
	NewReadyProbe(WithRetryInterval(-time.Millisecond))
}

// TestReadyProbe_Success 探活成功主路径：dial 可建连 → 复核通过 → 回调恰好
// 一次；空 network 归一为 "tcp"。
func TestReadyProbe_Success(t *testing.T) {
	_, addr := probeOpenListener(t)
	targetCalls, readyCalls := 0, 0
	var readyCtx context.Context

	p := NewReadyProbe(WithProbeTimeout(2 * time.Second))
	err := p.Run(context.Background(), ProbeRequest{
		Target: staticTarget("", addr, &targetCalls), // 空 network → 归一 "tcp"
		OnReady: func(ctx context.Context) {
			readyCalls++
			readyCtx = ctx
		},
	})
	if err != nil {
		t.Fatalf("Run err = %v, want nil", err)
	}
	if targetCalls != 1 {
		t.Errorf("Target 调用次数 = %d, want 1（探活开始时解析一次）", targetCalls)
	}
	if readyCalls != 1 {
		t.Errorf("OnReady 调用次数 = %d, want 1", readyCalls)
	}
	if readyCtx == nil {
		t.Error("OnReady 收到的 ctx 为 nil")
	}
}

// TestReadyProbe_TimeoutOnRefusedPort 探活超时（启动失败语义）：对无人监听的
// 地址反复 dial，超时后返回普通 error——不是 ErrProbeAborted、不是
// ErrProbeDisabled；最后一次 dial 错误经 %w 保留（ECONNREFUSED 可 Is 直达）；
// OnReady 不被调用。
func TestReadyProbe_TimeoutOnRefusedPort(t *testing.T) {
	addr := probeRefusedAddr(t)
	readyCalled := false

	p := NewReadyProbe(
		WithProbeTimeout(50*time.Millisecond),
		WithRetryInterval(2*time.Millisecond),
	)
	start := time.Now()
	err := p.Run(context.Background(), ProbeRequest{
		Target:  staticTarget("tcp", addr, nil),
		OnReady: func(context.Context) { readyCalled = true },
	})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("Run err = nil, want 探活超时错误")
	}
	if errors.Is(err, ErrProbeAborted) {
		t.Errorf("超时错误被误标为 ErrProbeAborted: %v（关停与失败是两件事，#48）", err)
	}
	if errors.Is(err, ErrProbeDisabled) {
		t.Errorf("超时错误被误标为 ErrProbeDisabled: %v", err)
	}
	if !errors.Is(err, syscall.ECONNREFUSED) {
		t.Errorf("超时错误链丢失最后一次 dial 根因（%%w）: %v", err)
	}
	if !strings.Contains(err.Error(), "timeout") || !strings.Contains(err.Error(), addr) {
		t.Errorf("超时错误应含 timeout 与 addr，实际: %v", err)
	}
	if readyCalled {
		t.Error("探活超时不得回调 OnReady")
	}
	if elapsed > 2*time.Second {
		t.Errorf("探活超时未受总预算约束: elapsed=%s（预算 50ms）", elapsed)
	}
}

// TestReadyProbe_AbortedByContextCancel 探活被正常关停打断（#48）：ctx 取消
// 返回哨兵 ErrProbeAborted——不是普通失败；OnReady 不被调用。
func TestReadyProbe_AbortedByContextCancel(t *testing.T) {
	addr := probeRefusedAddr(t)
	readyCalled := false

	ctx, cancel := context.WithCancel(context.Background())
	// 30ms 后取消：落在重试间隔（20ms）之间或某次 dial 的取消传播里，
	// 两条路径都必须收敛到 ErrProbeAborted。
	time.AfterFunc(30*time.Millisecond, cancel)

	p := NewReadyProbe(
		WithProbeTimeout(10*time.Second),
		WithRetryInterval(20*time.Millisecond),
	)
	err := p.Run(ctx, ProbeRequest{
		Target:  staticTarget("tcp", addr, nil),
		OnReady: func(context.Context) { readyCalled = true },
	})
	cancel()

	if !errors.Is(err, ErrProbeAborted) {
		t.Fatalf("Run err = %v, want ErrProbeAborted（关停打断是预期路径，#48）", err)
	}
	if readyCalled {
		t.Error("探活被打断不得回调 OnReady")
	}
}

// TestReadyProbe_AbortWinsOverTimeout 关停优先于超时判定（#48）：ctx 已取消
// 且探活预算同时极短——必须判「关停打断」而不是「探活超时」，每次判定超时前
// 先看 ctx.Err()。
func TestReadyProbe_AbortWinsOverTimeout(t *testing.T) {
	addr := probeRefusedAddr(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 入口即取消

	p := NewReadyProbe(
		WithProbeTimeout(time.Millisecond),
		WithRetryInterval(time.Millisecond),
	)
	err := p.Run(ctx, ProbeRequest{
		Target: staticTarget("tcp", addr, nil),
	})
	if !errors.Is(err, ErrProbeAborted) {
		t.Fatalf("Run err = %v, want ErrProbeAborted（取消与超时同时发生时关停优先，#48）", err)
	}
}

// TestReadyProbe_NegativeTimeoutDisables 负超时禁用探活语义（#129/#146）：
// Disabled() 为 true；Run 返回 ErrProbeDisabled 且**零副作用**——Target 不被
// 调用（dial 只会以 Target 解析出的 addr 发起，Target 未调用 ⇒ 零 dial，
// 即使探活目标可建连也不碰）、OnReady 不被调用；禁用下与已取消的 ctx 同时
// 存在时仍判禁用（禁用是配置属性，先于运行时状态判定）。
func TestReadyProbe_NegativeTimeoutDisables(t *testing.T) {
	_, addr := probeOpenListener(t) // 可建连目标：证明「可建连也不碰」
	targetCalls, readyCalled := 0, false
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	p := NewReadyProbe(WithProbeTimeout(-time.Second))
	if !p.Disabled() {
		t.Fatal("WithProbeTimeout(负数) 后 Disabled() = false, want true")
	}
	err := p.Run(canceled, ProbeRequest{
		Target:  staticTarget("tcp", addr, &targetCalls),
		Started: closedChan(),
		OnReady: func(context.Context) { readyCalled = true },
	})
	if !errors.Is(err, ErrProbeDisabled) {
		t.Fatalf("Run err = %v, want ErrProbeDisabled", err)
	}
	if targetCalls != 0 {
		t.Errorf("禁用探活下 Target 被调用 %d 次, want 0（零 dial）", targetCalls)
	}
	if readyCalled {
		t.Error("禁用探活下 Run 不得回调 OnReady（同步回调由调用方在启动 server 前自管）")
	}
}

// closedChan 返回已关闭的通道（门/复核信号用例的现成材料）。
func closedChan() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

// TestReadyProbe_WaitsForStartedGate 正向就绪门：Started 关闭前不发起 dial，
// 关闭后正常走通（探活与 server 启动并发的时序前提，#43/#38）。
func TestReadyProbe_WaitsForStartedGate(t *testing.T) {
	_, addr := probeOpenListener(t)
	gateDelay := 30 * time.Millisecond
	started := make(chan struct{})
	readyCalled := false
	time.AfterFunc(gateDelay, func() { close(started) })

	p := NewReadyProbe(WithProbeTimeout(2 * time.Second))
	begin := time.Now()
	err := p.Run(context.Background(), ProbeRequest{
		Target:  staticTarget("tcp", addr, nil),
		Started: started,
		OnReady: func(context.Context) { readyCalled = true },
	})
	elapsed := time.Since(begin)

	if err != nil {
		t.Fatalf("Run err = %v, want nil", err)
	}
	if elapsed < gateDelay-10*time.Millisecond {
		t.Errorf("Run 在就绪门关闭前返回: elapsed=%s, want ≥ %s（门未开不得回调）", elapsed, gateDelay)
	}
	if elapsed > 2*time.Second {
		t.Errorf("门已开后迟迟未返回: elapsed=%s", elapsed)
	}
	if !readyCalled {
		t.Error("门通过后未回调 OnReady")
	}
}

// TestReadyProbe_GateTimeout 等门超时：Started 永不关闭 → 总预算内报
// 「server not started」启动失败；无拨号目标变体的错误文本带 no dial target。
func TestReadyProbe_GateTimeout(t *testing.T) {
	_, addr := probeOpenListener(t) // 有地址但门不开：证明卡的是门不是 dial
	started := make(chan struct{})  // 永不关闭
	readyCalled := false

	p := NewReadyProbe(WithProbeTimeout(40 * time.Millisecond))
	err := p.Run(context.Background(), ProbeRequest{
		Target:  staticTarget("tcp", addr, nil),
		Started: started,
		OnReady: func(context.Context) { readyCalled = true },
	})
	if err == nil || !strings.Contains(err.Error(), "server not started") || !strings.Contains(err.Error(), addr) {
		t.Fatalf("Run err = %v, want 等门超时（含 server not started 与 addr）", err)
	}
	if errors.Is(err, ErrProbeAborted) {
		t.Errorf("等门超时被误标为 ErrProbeAborted: %v", err)
	}
	if readyCalled {
		t.Error("等门超时不得回调 OnReady")
	}

	// 无拨号目标（Target 返回空 addr，hertz #131 形态）：退化为「等门后回调」，
	// 超时错误应点名 no dial target，且整个探活零 dial。
	err = p.Run(context.Background(), ProbeRequest{
		Target:  staticTarget("", "", nil),
		Started: started,
	})
	if err == nil || !strings.Contains(err.Error(), "no dial target") {
		t.Fatalf("无拨号目标等门超时 err = %v, want 含 no dial target", err)
	}
}

// TestReadyProbe_GateExitedBeforeReady server 在探活通过前退出（#120）：
// Exited 在等门期间关闭 → 按启动失败返回（ctx 未取消时不是 ErrProbeAborted），
// 绝不回调 OnReady。
func TestReadyProbe_GateExitedBeforeReady(t *testing.T) {
	addr := probeRefusedAddr(t)
	started := make(chan struct{}) // 永不关闭
	exited := make(chan struct{})
	readyCalled := false
	time.AfterFunc(20*time.Millisecond, func() { close(exited) })

	p := NewReadyProbe(WithProbeTimeout(2 * time.Second))
	err := p.Run(context.Background(), ProbeRequest{
		Target:  staticTarget("tcp", addr, nil),
		Started: started,
		Exited:  exited,
		OnReady: func(context.Context) { readyCalled = true },
	})
	if err == nil || !strings.Contains(err.Error(), "exited before ready probe passed") {
		t.Fatalf("Run err = %v, want server exited before ready probe passed", err)
	}
	if errors.Is(err, ErrProbeAborted) {
		t.Errorf("退出失败被误标为 ErrProbeAborted: %v", err)
	}
	if readyCalled {
		t.Error("server 已退出不得回调 OnReady")
	}
}

// TestReadyProbe_GateExitedAbortedByShutdown 关停优先（#48）在等门阶段同样
// 成立：Exited 与 ctx 取消同时发生 → 判 ErrProbeAborted（预期路径），不把
// 关停误报成启动失败。
func TestReadyProbe_GateExitedAbortedByShutdown(t *testing.T) {
	addr := probeRefusedAddr(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	exited := closedChan() // 已退出 + 已关停：关停优先

	p := NewReadyProbe(WithProbeTimeout(time.Second))
	err := p.Run(ctx, ProbeRequest{
		Target:  staticTarget("tcp", addr, nil),
		Started: make(chan struct{}),
		Exited:  exited,
	})
	if !errors.Is(err, ErrProbeAborted) {
		t.Fatalf("Run err = %v, want ErrProbeAborted（Exited 与关停同时发生时关停优先）", err)
	}
}

// TestReadyProbe_ConnectedButExited 建连后退出复核（#120/#131）：探活连上了
// 监听器、但本进程 server 已退出（典型：端口被他人占用，本进程 bind 失败）
// → 按启动失败返回，绝不回调 OnReady——否则把不属于自己的实例注册出去。
func TestReadyProbe_ConnectedButExited(t *testing.T) {
	_, addr := probeOpenListener(t) // dial 必成功
	readyCalled := false

	p := NewReadyProbe(WithProbeTimeout(2 * time.Second))
	err := p.Run(context.Background(), ProbeRequest{
		Target:  staticTarget("tcp", addr, nil),
		Exited:  closedChan(), // 建连成功后复核到已退出
		OnReady: func(context.Context) { readyCalled = true },
	})
	if err == nil || !strings.Contains(err.Error(), "connected but server already exited") {
		t.Fatalf("Run err = %v, want connected but server already exited", err)
	}
	if errors.Is(err, ErrProbeAborted) {
		t.Errorf("退出复核失败被误标为 ErrProbeAborted: %v", err)
	}
	if readyCalled {
		t.Error("退出复核失败不得回调 OnReady（端口归属复核，#120）")
	}
}

// TestReadyProbe_OnReadyPanicBecomesError OnReady panic 收敛为 error（#152）：
// panic 沿探活路径展开会让调用方「探活失败 ⇒ 请求关机」的处置被整段跳过，
// server 留在半启动形态；panic 值身份保留（core#136）——panic(boomErr) 出去
// 的错误 errors.Is 可直达。
func TestReadyProbe_OnReadyPanicBecomesError(t *testing.T) {
	_, addr := probeOpenListener(t)

	p := NewReadyProbe(WithProbeTimeout(2 * time.Second))
	err := p.Run(context.Background(), ProbeRequest{
		Target: staticTarget("tcp", addr, nil),
		OnReady: func(context.Context) {
			panic("boom: ready path exploded")
		},
	})
	if err == nil || !strings.Contains(err.Error(), "onReady panicked") {
		t.Fatalf("Run err = %v, want onReady panicked 转成的 error", err)
	}
	if errors.Is(err, ErrProbeAborted) {
		t.Errorf("OnReady panic 被误标为 ErrProbeAborted: %v", err)
	}

	// panic(error)：身份经 pkg/errors.Recovered 保留（core#136 单源口径）。
	err = p.Run(context.Background(), ProbeRequest{
		Target:  staticTarget("tcp", addr, nil),
		Started: closedChan(),
		OnReady: func(context.Context) { panic(errBoom) },
	})
	if !errors.Is(err, errBoom) {
		t.Fatalf("panic(errBoom) 后 errors.Is(err, errBoom) = false, err = %v（panic 值身份必须不丢）", err)
	}
}

// TestReadyProbe_NoDialTargetWaitsGateThenReady 无拨号目标退化路径（hertz
// #131）：addr 为空时不 dial，等就绪门通过后直接回调 OnReady。
func TestReadyProbe_NoDialTargetWaitsGateThenReady(t *testing.T) {
	started := make(chan struct{})
	readyCalled := false
	time.AfterFunc(20*time.Millisecond, func() { close(started) })

	p := NewReadyProbe(WithProbeTimeout(2 * time.Second))
	err := p.Run(context.Background(), ProbeRequest{
		Target:  staticTarget("", "", nil),
		Started: started,
		OnReady: func(context.Context) { readyCalled = true },
	})
	if err != nil {
		t.Fatalf("Run err = %v, want nil", err)
	}
	if !readyCalled {
		t.Error("无拨号目标 + 门通过后未回调 OnReady")
	}
}

// TestReadyProbe_NilCtxTreatedAsBackground nil ctx = Background（core#159 全仓
// 口径）：不得 nil 解引用，按正常探活走通。
func TestReadyProbe_NilCtxTreatedAsBackground(t *testing.T) {
	_, addr := probeOpenListener(t)

	p := NewReadyProbe(WithProbeTimeout(2 * time.Second))
	if err := p.Run(nil, ProbeRequest{Target: staticTarget("tcp", addr, nil)}); err != nil { //nolint:staticcheck // SA1012 故意传 nil，验证 core#159「nil ctx = Background」口径
		t.Fatalf("Run(nil ctx) err = %v, want nil", err)
	}
}

// TestReadyProbe_ConcurrentRuns 并发 Run：配置构造期固定，多个 goroutine 可
// 同时 Run 互不影响（-race 兜底读路径无共享可变状态）。
func TestReadyProbe_ConcurrentRuns(t *testing.T) {
	_, addr1 := probeOpenListener(t)
	_, addr2 := probeOpenListener(t)

	p := NewReadyProbe(WithProbeTimeout(2 * time.Second))
	var wg sync.WaitGroup
	for _, addr := range []string{addr1, addr2, addr1, addr2} {
		wg.Go(func() {
			if err := p.Run(context.Background(), ProbeRequest{Target: staticTarget("tcp", addr, nil)}); err != nil {
				t.Errorf("Run(%s) err = %v", addr, err)
			}
		})
	}
	wg.Wait()
}

// TestReadyProbe_DialLoopRetriesUntilDeadline dial 循环在预算内反复重试：
// 地址先 refused、随后端口就绪 → 前几次失败不是终局，探活恢复并成功（小参数
// 版钉「失败不终局，重试直到 deadline」——两侧下沉前 500ms dial / 100ms 重试
// 的同款节奏）。
func TestReadyProbe_DialLoopRetriesUntilDeadline(t *testing.T) {
	addr := probeRefusedAddr(t)
	readyCalled := false
	// 60ms 后在同端口开始监听：前几次 dial refused，随后建连成功。
	go func() {
		time.Sleep(60 * time.Millisecond)
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			t.Errorf("late listen: %v", err)
			return
		}
		defer func() { _ = ln.Close() }()
		time.Sleep(200 * time.Millisecond) // 保持监听覆盖探活窗口
	}()

	p := NewReadyProbe(
		WithProbeTimeout(2*time.Second),
		WithRetryInterval(10*time.Millisecond),
	)
	err := p.Run(context.Background(), ProbeRequest{
		Target:  staticTarget("tcp", addr, nil),
		OnReady: func(context.Context) { readyCalled = true },
	})
	if err != nil {
		t.Fatalf("先 refused 后就绪的探活应成功: err=%v", err)
	}
	if !readyCalled {
		t.Error("重试成功后未回调 OnReady")
	}
}
