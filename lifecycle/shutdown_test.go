package lifecycle

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestManager_Shutdown(t *testing.T) {
	mgr := New(5 * time.Second)
	var count int32

	mgr.Add("first", func(ctx context.Context) error {
		atomic.AddInt32(&count, 1)
		return nil
	}, 0)

	mgr.Add("second", func(ctx context.Context) error {
		atomic.AddInt32(&count, 1)
		return nil
	}, 0)

	mgr.Shutdown(context.Background())

	if atomic.LoadInt32(&count) != 2 {
		t.Errorf("expected 2 jobs executed, got %d", count)
	}
}

func TestManager_Shutdown_OnceOnly(t *testing.T) {
	var count int32
	mgr := New(5 * time.Second)

	mgr.Add("counter", func(ctx context.Context) error {
		atomic.AddInt32(&count, 1)
		return nil
	}, 0)

	mgr.Shutdown(context.Background())
	mgr.Shutdown(context.Background())

	if atomic.LoadInt32(&count) != 1 {
		t.Errorf("expected 1 execution, got %d", count)
	}
}

func TestManager_Errors(t *testing.T) {
	mgr := New(5 * time.Second)
	expectedErr := errors.New("test error")

	mgr.Add("fails", func(ctx context.Context) error {
		return expectedErr
	}, 0)

	mgr.Shutdown(context.Background())
	errs := mgr.Errors()
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	if !errors.Is(errs[0], expectedErr) {
		t.Errorf("expected %v, got %v", expectedErr, errs[0])
	}
}

func TestManager_Errors_NoSideEffect(t *testing.T) {
	var count int32
	mgr := New(5 * time.Second)

	mgr.Add("counter", func(ctx context.Context) error {
		atomic.AddInt32(&count, 1)
		return nil
	}, 0)

	// Errors 不应触发 Shutdown
	_ = mgr.Errors()
	if atomic.LoadInt32(&count) != 0 {
		t.Errorf("expected 0 executions (Errors should not trigger Shutdown), got %d", count)
	}

	// 显式 Shutdown 后 Errors 返回收集的错误
	mgr.Shutdown(context.Background())
	_ = mgr.Errors()
	if atomic.LoadInt32(&count) != 1 {
		t.Errorf("expected 1 execution after explicit Shutdown, got %d", count)
	}
}

func TestManager_AddTeardown(t *testing.T) {
	mgr := New(5 * time.Second)
	var called int32

	mgr.AddTeardown(func() {
		atomic.AddInt32(&called, 1)
	})

	mgr.Shutdown(context.Background())

	if atomic.LoadInt32(&called) != 1 {
		t.Errorf("expected teardown called, got %d", called)
	}
}

func TestManager_Timeout(t *testing.T) {
	mgr := New(100 * time.Millisecond)

	mgr.Add("slow", func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
			return nil
		}
	}, 50*time.Millisecond)

	done := make(chan struct{})
	go func() {
		mgr.Shutdown(context.Background())
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown did not complete within timeout")
	}
}

func TestManager_Acquire_Release(t *testing.T) {
	mgr := New(5 * time.Second)

	// 初始状态应该可以获取许可
	if !mgr.Acquire() {
		t.Error("expected Acquire to return true")
	}

	if mgr.GetRefCount() != 1 {
		t.Errorf("expected refCount 1, got %d", mgr.GetRefCount())
	}

	mgr.Release()

	if mgr.GetRefCount() != 0 {
		t.Errorf("expected refCount 0, got %d", mgr.GetRefCount())
	}
}

func TestManager_Acquire_Multiple(t *testing.T) {
	mgr := New(5 * time.Second)

	// 获取多个许可
	for i := 0; i < 10; i++ {
		if !mgr.Acquire() {
			t.Errorf("expected Acquire to return true at iteration %d", i)
		}
	}

	if mgr.GetRefCount() != 10 {
		t.Errorf("expected refCount 10, got %d", mgr.GetRefCount())
	}

	// 释放所有许可
	for i := 0; i < 10; i++ {
		mgr.Release()
	}

	if mgr.GetRefCount() != 0 {
		t.Errorf("expected refCount 0, got %d", mgr.GetRefCount())
	}
}

func TestManager_Acquire_DuringShutdown(t *testing.T) {
	mgr := New(5 * time.Second)

	// 启动停机
	go mgr.Shutdown(context.Background())

	// 等待停机开始
	time.Sleep(50 * time.Millisecond)

	// 停机期间应该无法获取许可
	if mgr.Acquire() {
		t.Error("expected Acquire to return false during shutdown")
	}
}

func TestManager_IsAccepting(t *testing.T) {
	mgr := New(5 * time.Second)

	if !mgr.IsAccepting() {
		t.Error("expected IsAccepting to return true")
	}

	// 启动停机
	go mgr.Shutdown(context.Background())

	// 等待停机开始
	time.Sleep(50 * time.Millisecond)

	if mgr.IsAccepting() {
		t.Error("expected IsAccepting to return false after shutdown started")
	}
}

func TestManager_Drain(t *testing.T) {
	mgr := New(5 * time.Second)

	// 模拟进行中的请求
	mgr.Acquire()

	// 在后台完成请求
	go func() {
		time.Sleep(100 * time.Millisecond)
		mgr.Release()
	}()

	// Drain 应该等待请求完成
	start := time.Now()
	mgr.Drain(context.Background())
	elapsed := time.Since(start)

	if elapsed < 50*time.Millisecond {
		t.Error("expected Drain to wait for requests")
	}
}

func TestManager_Drain_NoPendingRequests(t *testing.T) {
	mgr := New(5 * time.Second)

	// 没有待处理请求时 Drain 应立即返回
	start := time.Now()
	mgr.Drain(context.Background())
	elapsed := time.Since(start)

	if elapsed > 1*time.Second {
		t.Errorf("expected Drain to return quickly, took %v", elapsed)
	}
}
