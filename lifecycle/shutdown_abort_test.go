package lifecycle

import (
	"context"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// core#142 / core#150 的护栏。

// TestListenLoop_SecondSignalAbortsShutdown 回归 #142：资源关闭执行期再收到
// 一个信号必须强制中止关停——在跑的 closer 收到 ctx 取消，尚未开始的任务
// 全部跳过并逐个记入 Errors()，不会被静默吞掉（此前第二枪只覆盖 drain
// 窗口，Shutdown 执行期信号被缓冲进无人读的 channel，30s drain + N×per-job
// 预算的长关停既不加速也不能终止）。
func TestListenLoop_SecondSignalAbortsShutdown(t *testing.T) {
	m := New(time.Second)
	m.SetDrainTimeout(50 * time.Millisecond) // 无在飞请求，drain 立即结束

	entered := make(chan struct{})
	var skippedRan atomic.Bool
	m.Add("slow", func(ctx context.Context) error {
		close(entered) // 标记已进入资源关闭阶段
		<-ctx.Done()   // 尊重 ctx：第二枪取消后立即返回
		return ctx.Err()
	}, 2*time.Second)
	m.Add("later", func(context.Context) error {
		skippedRan.Store(true)
		return nil
	}, 0)

	sig := make(chan os.Signal, 2)
	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		m.listenLoop(sig)
	}()

	sig <- syscall.SIGTERM
	waitUntil(t, 2*time.Second, "第一个 closer 应已开始执行（说明已进入资源关闭阶段）", func() bool {
		select {
		case <-entered:
			return true
		default:
			return false
		}
	})

	start := time.Now()
	sig <- syscall.SIGTERM // 关闭执行期第二枪

	select {
	case <-loopDone:
	case <-time.After(5 * time.Second):
		t.Fatal("关闭执行期第二枪未生效：Listen 仍卡在 closer（2s 预算 + 跳过逻辑都未触发）")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("第二枪应尽快结束关停（closer 收到取消即返回），实际耗时 %v", elapsed)
	}
	if skippedRan.Load() {
		t.Fatal("第二枪后尚未开始的任务不得再执行")
	}
	found := false
	for _, err := range m.Errors() {
		if strings.Contains(err.Error(), "skipped, shutdown aborted by signal") {
			found = true
		}
	}
	if !found {
		t.Fatalf("被跳过的任务应逐个记入 Errors()（skipped 记录），实际：%v", m.Errors())
	}
}

// TestShutdown_CtxCancelDoesNotSkipRemaining 钉住公开 Shutdown 的语义边界
// （core#142 特意保持不变）：ctx 取消只中止**当前正在执行**的任务，剩余
// 任务照常补齐执行——「收到信号即跳过剩余」是 Listen 信号路径的专属行为，
// 不经 ctx 泄漏给直接调用 Shutdown 的宿主。
func TestShutdown_CtxCancelDoesNotSkipRemaining(t *testing.T) {
	m := New(time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 已取消：first 拿到的 jobCtx 同步取消，返回 ctx.Err

	var laterRan atomic.Bool
	m.Add("first", func(ctx context.Context) error { return ctx.Err() }, 0)
	m.Add("second", func(context.Context) error {
		laterRan.Store(true)
		return nil
	}, 0)

	m.Shutdown(ctx)

	if !laterRan.Load() {
		t.Fatal("公开 Shutdown 的 ctx 取消不得跳过剩余任务（语义边界，core#142）")
	}
}

// panicOnInfoSink 是宿主注入 logger 的故障模拟：Info 必炸、Warn 照常记录
// ——#150 的 panic 面（关停路径上宿主 logger panic 即整进程 abort）。
type panicOnInfoSink struct {
	mu    sync.Mutex
	warns []string
}

func (s *panicOnInfoSink) Info(msg string, _ ...any) { panic("logger injected panic: " + msg) }
func (s *panicOnInfoSink) Warn(msg string, _ ...any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.warns = append(s.warns, msg)
}
func (s *panicOnInfoSink) Error(string, ...any) {}

func (s *panicOnInfoSink) warnCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.warns)
}

// TestDrainWithSignal_LoggerPanicIsolated 回归 #150：drain goroutine 体内
// 的注入 logger panic 必须就地收编——drainWithSignal 照常返回（排空视为
// 结束，继续关资源），panic 上报为 Warn，进程不被打崩。
func TestDrainWithSignal_LoggerPanicIsolated(t *testing.T) {
	m := New(time.Second)
	sink := &panicOnInfoSink{}
	m.logger = sink // 同包测试直设注入面

	ch := make(chan os.Signal, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		m.drainWithSignal(ch)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("drain goroutine panic 未被收编：drainWithSignal 未返回（无 recover 时整进程 abort）")
	}
	if sink.warnCount() == 0 {
		t.Fatal("drain goroutine 的 panic 应上报为 Warn 日志（不留痕的 recover 等于藏故障）")
	}
}
