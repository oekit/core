package config

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// core#144 的护栏：ConfigSource.Watch 的契约为「启动监听后即返回，监听在
// 自起 goroutine 内进行，ctx 取消时停止」（钉死在接口 godoc，overlay.go
// 同源引用）。本用例从消费方视角钉住契约的**可观测面**：按契约实现的
// 非阻塞 source 必须能被 ConfigWatcher.Start 同步使用——Start 返回时不
// 等待任何监听结束；阻塞式实现（select{<-evt,<-ctx.Done()} 直接写在
// Watch 里）在旧文档的歧义下是「合法实现」，新契约下明确非法，无法通过
// 本形态的使用方式自检。
type contractSource struct {
	data      atomic.Pointer[[]byte]
	watchRet  chan struct{} // Watch 返回信号
	watchExit chan struct{} // 自起监听 goroutine 退出信号（ctx 取消后）
}

func newContractSource(data []byte) *contractSource {
	s := &contractSource{
		watchRet:  make(chan struct{}),
		watchExit: make(chan struct{}),
	}
	s.data.Store(&data)
	return s
}

func (s *contractSource) Read() ([]byte, error) { return *s.data.Load(), nil }

// Watch 按契约实现：自起 goroutine 承载监听，本方法立即返回；ctx 取消时
// 监听 goroutine 退出。
func (s *contractSource) Watch(ctx context.Context, onChange func()) {
	go func() {
		defer close(s.watchExit)
		<-ctx.Done() // 真实监听循环的占位：存续到 ctx 取消
	}()
	close(s.watchRet)
}

// TestStart_DoesNotWaitForWatch_StartReturnsAfterInitialLoad 钉住 Start 的
// 同步语义：初始加载完成后 Watch 一返回 Start 就返回（不等待监听结束），
// Get() 已可见首载配置。
func TestStart_DoesNotWaitForWatch_StartReturnsAfterInitialLoad(t *testing.T) {
	src := newContractSource([]byte(`{"port":8080}`))
	watcher := NewConfigWatcher(src, func(b []byte) (*testConfig, error) {
		c := &testConfig{}
		if err := Unmarshal(b, c); err != nil {
			return nil, err
		}
		return c, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- watcher.Start(ctx) }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Start 不应返回错误：%v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start 在初始加载后未返回——违反 Watch 非阻塞契约（core#144）")
	}

	if watcher.Get() == nil {
		t.Fatal("Start 返回后首载配置应可见")
	}

	// ctx 取消后契约的自起监听 goroutine 应退出（监听生命周期由 ctx 驱动）
	cancel()
	select {
	case <-src.watchExit:
	case <-time.After(2 * time.Second):
		t.Fatal("ctx 取消后 Watch 自起的监听 goroutine 应退出")
	}
}
