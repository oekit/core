package lifecycle

import (
	"context"
	"testing"
	"time"
)

// TestManager_PanicInJob_DoesNotAbortRemainingJobs 回归测试（W1-M1）：
// 单个 closer panic 必须被隔离为错误记录，剩余任务继续执行；
// 且 panic 不得使 sync.Once 中毒（Shutdown 可重入语义保持）。
func TestManager_PanicInJob_DoesNotAbortRemainingJobs(t *testing.T) {
	m := New(time.Second)
	secondRan := false
	m.Add("panicking", func(context.Context) error { panic("closer exploded") }, 0)
	m.Add("second", func(context.Context) error { secondRan = true; return nil }, 0)

	m.Shutdown(context.Background())

	if !secondRan {
		t.Fatal("job after a panicking job was not executed")
	}
	errs := m.Errors()
	if len(errs) != 1 {
		t.Fatalf("expected 1 recorded error, got %d: %v", len(errs), errs)
	}
}

// TestManager_PanicInTeardown_DoesNotAbortRemainingTeardowns 回归测试（W1-M1）。
func TestManager_PanicInTeardown_DoesNotAbortRemainingTeardowns(t *testing.T) {
	m := New(time.Second)
	secondRan := false
	m.AddTeardown(func() { panic("teardown exploded") })
	m.AddTeardown(func() { secondRan = true })

	m.Shutdown(context.Background())

	if !secondRan {
		t.Fatal("teardown after a panicking teardown was not executed")
	}
	if len(m.Errors()) == 0 {
		t.Fatal("expected the panic to be recorded as an error")
	}
}

// TestManager_PerJobIndependentTimeout 回归测试（W1-M2）：
// 前序任务耗尽大量时间后，后续任务仍应获得完整的独立超时，
// 而不是拿到一个已经被挤占的 context（否则尾部 deregister 会静默失败留幽灵实例）。
func TestManager_PerJobIndependentTimeout(t *testing.T) {
	m := New(time.Second)

	secondGotFreshCtx := false
	m.Add("slow", func(ctx context.Context) error {
		select {
		case <-ctx.Done():
		case <-time.After(150 * time.Millisecond):
		}
		return nil
	}, 0)
	m.Add("tail", func(ctx context.Context) error {
		secondGotFreshCtx = ctx.Err() == nil
		return nil
	}, 0)

	m.Shutdown(context.Background())

	if !secondGotFreshCtx {
		t.Fatal("tail job did not receive a fresh context — per-job budgets are shared")
	}
}

// TestManager_ParentExpiry_CancelsRunningJob_NextJobsStillRun：
// 调用方 ctx 超时作为外部中止信号：取消当前任务，但后续任务仍会执行
// （拿到已取消的 ctx，由 closer 自行决定如何快速退出）。
func TestManager_ParentExpiry_CancelsRunningJob_NextJobsStillRun(t *testing.T) {
	m := New(time.Second)

	parent, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	firstCancelled := make(chan struct{})
	tailRan := false
	tailSawCancelled := false

	m.Add("slow", func(ctx context.Context) error {
		<-ctx.Done()
		close(firstCancelled)
		return ctx.Err()
	}, 0)
	m.Add("tail", func(ctx context.Context) error {
		tailRan = true
		tailSawCancelled = ctx.Err() != nil
		return nil
	}, 0)

	done := make(chan struct{})
	go func() {
		m.Shutdown(parent)
		close(done)
	}()

	<-firstCancelled
	<-done

	if !tailRan {
		t.Fatal("tail job was skipped — remaining jobs must still execute")
	}
	if !tailSawCancelled {
		t.Fatal("tail job should observe the cancelled parent as its ctx state")
	}
}

// TestManager_ParentCancelPropagatesToRunningJob：调用方 ctx 取消时，
// 进行中的任务应被同步取消（独立预算不意味着脱离调用方控制）。
func TestManager_ParentCancelPropagatesToRunningJob(t *testing.T) {
	m := New(time.Second)

	parent, cancel := context.WithCancel(context.Background())
	jobCancelled := make(chan struct{})

	m.Add("long", func(ctx context.Context) error {
		<-ctx.Done()
		close(jobCancelled)
		return ctx.Err()
	}, 10*time.Second)

	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()

	m.Shutdown(parent)

	select {
	case <-jobCancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("running job was not cancelled when parent context was cancelled")
	}
}
