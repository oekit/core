package lifecycle

import (
	"context"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"
)

// waitUntil 轮询等待 cond 成立；超时则 Fatal。事件驱动而不是固定睡眠：
// 固定睡眠在慢机器上会随机假红，在快机器上白等。
func waitUntil(t *testing.T, d time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("等待超时（%v）：%s", d, what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// TestListenLoop_DrainsInFlightBeforeClosing 回归 #52：信号驱动的关闭必须
// 「停收新请求 → 排空在飞请求 → 关资源」，而不是直接关资源。
//
// 缺陷成因：Listen 收到信号后直接 Shutdown，而 Shutdown 不等 refCount 归零
// ⇒ 在飞请求还握着资源时资源就被关掉。Drain 的文档要求它插在「停收新请求
// 之后、关资源之前」，但阻塞式 Listen 没给调用方任何插入位置。
//
// 用例断言两件事：① 资源关闭时 GetRefCount() 必须已归零；② 在飞请求
// Release 之前，closer **不得**被调用（这是「跳过 Drain」的直接表现）。
func TestListenLoop_DrainsInFlightBeforeClosing(t *testing.T) {
	m := New(time.Second)
	if !m.Acquire() {
		t.Fatal("初始状态应允许 Acquire")
	}

	var mu sync.Mutex
	var refAtClose int64 = -1
	closed := make(chan struct{})
	m.Add("db", func(context.Context) error {
		mu.Lock()
		refAtClose = m.GetRefCount()
		mu.Unlock()
		close(closed)
		return nil
	}, 0)

	sig := make(chan os.Signal, 2)
	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		m.listenLoop(sig)
	}()

	sig <- syscall.SIGTERM

	// 停收新请求必须发生在排空之前，否则 refCount 会被新请求持续抬起。
	waitUntil(t, 2*time.Second, "收到信号后应立即停止接受新请求", func() bool {
		return !m.IsAccepting()
	})

	select {
	case <-closed:
		t.Fatal("在飞请求尚未 Release 就关闭了资源——Listen 跳过了 Drain")
	case <-time.After(80 * time.Millisecond):
		// 期望路径：仍在排空
	}

	if n := m.GetRefCount(); n != 1 {
		t.Fatalf("排空期间 in-flight 应为 1，实得 %d", n)
	}

	m.Release()

	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("在飞请求 Release 后 drain 应结束并执行资源关闭")
	}
	select {
	case <-loopDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Listen 应在资源关闭后返回")
	}

	mu.Lock()
	defer mu.Unlock()
	if refAtClose != 0 {
		t.Fatalf("资源关闭时在飞请求数应为 0，实得 %d——资源被提前关闭", refAtClose)
	}
}

// TestListenLoop_SecondSignalAbortsDrain 排空期间再收到一个信号应当**立即
// 放弃等待**并进入资源关闭：编排器连发两次 SIGTERM / 运维连按两次 Ctrl-C
// 的语义就是「别等了，现在就关」。
func TestListenLoop_SecondSignalAbortsDrain(t *testing.T) {
	m := New(time.Second)
	m.SetDrainTimeout(time.Hour) // 若第二枪不生效，本用例会明显卡住
	m.Acquire()

	closed := make(chan struct{})
	m.Add("db", func(context.Context) error {
		close(closed)
		return nil
	}, 0)

	sig := make(chan os.Signal, 2)
	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		m.listenLoop(sig)
	}()

	sig <- syscall.SIGTERM
	waitUntil(t, 2*time.Second, "收到第一个信号后应停止接受新请求", func() bool {
		return !m.IsAccepting()
	})

	start := time.Now()
	sig <- syscall.SIGTERM

	select {
	case <-loopDone:
	case <-time.After(5 * time.Second):
		t.Fatal("排空期间第二个信号未生效：Listen 仍在等 drainTimeout（1h）")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("第二个信号应尽快结束排空，实际耗时 %v", elapsed)
	}

	select {
	case <-closed:
	default:
		t.Fatal("放弃排空后仍必须执行资源关闭")
	}
}

// TestListenLoop_DrainTimeoutBounds 唯一的等待不该是无限的：drainTimeout
// 到点即进入资源关闭。
func TestListenLoop_DrainTimeoutBounds(t *testing.T) {
	m := New(time.Second)
	m.SetDrainTimeout(120 * time.Millisecond)
	m.Acquire() // 永不 Release：模拟卡死的请求

	m.Add("db", func(context.Context) error { return nil }, 0)

	sig := make(chan os.Signal, 1)
	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		m.listenLoop(sig)
	}()

	start := time.Now()
	sig <- syscall.SIGTERM

	select {
	case <-loopDone:
	case <-time.After(5 * time.Second):
		t.Fatal("drainTimeout 未生效：Listen 没有自行结束等待")
	}
	// 下界断言是安全的（超时早于配置值返回必错）；上界放宽避免慢机器假红。
	if elapsed := time.Since(start); elapsed < 120*time.Millisecond {
		t.Fatalf("不应早于 drainTimeout 结束排空，实际耗时 %v", elapsed)
	}
}

// TestListenLoop_NoInFlight_NoDrainWait 没有在飞请求时不得白等
// drainTimeout（默认 30s）——否则「没请求的服务 Ctrl-C 后卡半分钟」。
func TestListenLoop_NoInFlight_NoDrainWait(t *testing.T) {
	m := New(time.Second)
	m.Add("db", func(context.Context) error { return nil }, 0)

	sig := make(chan os.Signal, 1)
	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		m.listenLoop(sig)
	}()

	start := time.Now()
	sig <- syscall.SIGTERM

	select {
	case <-loopDone:
	case <-time.After(3 * time.Second):
		t.Fatal("空跑的 drain 不应阻塞（默认 drainTimeout 30s）")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("无在飞请求时关闭应立即完成，实际耗时 %v", elapsed)
	}
}
