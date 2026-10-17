package lifecycle

import (
	"context"
	"testing"
	"time"
)

// TestDrainTimeout_IndependentOfJobTimeout 验证 drain 超时是独立旋钮：
// 不再借用 per-job 超时（一旋钮两义），两者互不影响。
func TestDrainTimeout_IndependentOfJobTimeout(t *testing.T) {
	m := New(50 * time.Millisecond) // per-job 超时 50ms
	m.SetDrainTimeout(400 * time.Millisecond)

	m.Acquire() // 模拟进行中的请求（不 Release）
	done := make(chan struct{})
	go func() {
		m.Drain(context.Background())
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("drain must not be bounded by the per-job timeout")
	case <-time.After(150 * time.Millisecond):
		// 仍在等待，符合预期
	}

	m.Release() // 请求完成 → drain 立即返回
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("drain did not return after requests completed")
	}
}

// TestDrainTimeout_NegativeMeansUnlimited 验证负值 = 不自行限时，
// 仅由调用方传入的 ctx（或外部 deadline）结束等待。
func TestDrainTimeout_NegativeMeansUnlimited(t *testing.T) {
	m := New(time.Second)
	m.SetDrainTimeout(-1)
	m.Acquire()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(80 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	m.Drain(ctx)
	if elapsed := time.Since(start); elapsed < 70*time.Millisecond {
		t.Fatalf("drain returned on its own after %v, want bounded only by ctx cancel", elapsed)
	}
}

// TestDrainTimeout_CompletesWhenIdle 验证无进行中请求时立即完成。
func TestDrainTimeout_CompletesWhenIdle(t *testing.T) {
	m := New(time.Second)
	m.SetDrainTimeout(500 * time.Millisecond)

	start := time.Now()
	m.Drain(context.Background())
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("idle drain took %v, want immediate", elapsed)
	}
}
