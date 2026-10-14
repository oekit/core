package cache

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestCheckCachesSuccess(t *testing.T) {
	calls := 0
	c := NewChecker(func(ctx context.Context) error {
		calls++
		return nil
	}, 50*time.Millisecond, 0, WithClock(func() time.Time { return time.Now() }))
	for i := 0; i < 5; i++ {
		if err := c.Check(context.Background()); err != nil {
			t.Fatalf("第 %d 次检查不应报错：%v", i, err)
		}
	}
	if calls != 1 {
		t.Fatalf("TTL 内应只刷新一次，实际 %d", calls)
	}
}

func TestCheckFailShortTTL(t *testing.T) {
	now := time.Now()
	fail := true
	calls := 0
	c := NewChecker(func(ctx context.Context) error {
		calls++
		if fail {
			return errors.New("down")
		}
		return nil
	}, time.Hour, 10*time.Millisecond, WithClock(func() time.Time { return now }))
	if err := c.Check(context.Background()); err == nil {
		t.Fatal("期望失败结果")
	}
	now = now.Add(5 * time.Millisecond)
	if err := c.Check(context.Background()); err == nil {
		t.Fatal("失败短 TTL 内应仍报错")
	}
	if calls != 1 {
		t.Fatalf("失败短 TTL 内不应刷新，实际 %d", calls)
	}
	now = now.Add(6 * time.Millisecond) // 越过 failTTL
	fail = false
	if err := c.Check(context.Background()); err != nil {
		t.Fatalf("越过 failTTL 应刷新并成功：%v", err)
	}
	if calls != 2 {
		t.Fatalf("期望第 2 次刷新，实际 %d", calls)
	}
}

func TestCheckInflightMerged(t *testing.T) {
	release := make(chan struct{})
	var calls int32
	c := NewChecker(func(ctx context.Context) error {
		atomic.AddInt32(&calls, 1)
		<-release
		return nil
	}, time.Hour, 0)

	errCh := make(chan error, 3)
	for i := 0; i < 3; i++ {
		go func() { errCh <- c.Check(context.Background()) }()
	}
	time.Sleep(50 * time.Millisecond) // 等并发者都到齐（后两个合并到首次刷新）
	close(release)
	for i := 0; i < 3; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("并发检查不应报错：%v", err)
		}
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("并发应合并为一次刷新，实际 %d", got)
	}
}

func TestCheckInflightWaiterHonorsCtx(t *testing.T) {
	block := make(chan struct{})
	c := NewChecker(func(ctx context.Context) error {
		<-block
		return nil
	}, time.Hour, 0)
	go c.Check(context.Background()) // 领头者卡在刷新
	time.Sleep(20 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	// 等待者合并在途刷新时应感知 ctx 超时（leader 仍阻塞，刷新未完）
	if err := c.Check(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("等待者应感知 ctx 超时，得到 %v", err)
	}
	close(block) // 放行领头者，避免泄漏 goroutine
}
