package config

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// core#137 回归：Reload 此前在 dispatchMu 内同步执行用户回调 onChange——
// 回调里再调 Reload（同 goroutine 重入）会死锁、回调同步等待另一 goroutine
// 的 Reload 完成同样死锁、慢回调卡住 Watch 通道的后续重载投递；约束只写在
// 注释里。现回调经队列在独立 dispatch goroutine 上投递（入队序 = Swap 序，
// 保序、锁外执行），三类死锁形态都应有解且有序性不回退。

func newDispatchWatcher(t *testing.T, parser func([]byte) (*testConfig, error)) (*ConfigWatcher[testConfig], *blockingSource) {
	t.Helper()
	src := &blockingSource{data: []byte(`{"name":"gen0"}`)}
	w := NewConfigWatcher[testConfig](src, parser)
	if err := w.Reload(); err != nil { // 初始加载（不投递回调）
		t.Fatalf("initial reload: %v", err)
	}
	return w, src
}

// TestConfigWatcher_OnChangeReentrantReload 回归 core#137 形态一：onChange
// 内再调 Reload（同 goroutine 重入）——旧实现死锁在 dispatchMu 上，本测试
// 超时即失败。
func TestConfigWatcher_OnChangeReentrantReload(t *testing.T) {
	w, src := newDispatchWatcher(t, func(d []byte) (*testConfig, error) {
		var cfg testConfig
		return &cfg, json.Unmarshal(d, &cfg)
	})

	reloads := atomic.Int64{}
	entered := make(chan struct{}, 2)
	w.OnChange(func(old, new *testConfig) {
		entered <- struct{}{}
		if reloads.Add(1) == 1 {
			// 回调内重入：旧实现同 goroutine 重入 dispatchMu ⇒ 死锁。
			src.set([]byte(`{"name":"gen2"}`))
			_ = w.Reload()
		}
	})

	src.set([]byte(`{"name":"gen1"}`))
	done := make(chan error, 1)
	go func() { done <- w.Reload() }()

	for range 2 {
		select {
		case <-entered:
		case <-time.After(3 * time.Second):
			t.Fatal("onChange 重入 Reload 死锁（core#137）：回调未在预算内全部投递")
		}
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("reload: %v", err)
		}
	default:
		t.Fatal("首次 Reload 未返回（重入死锁）")
	}
	if w.Get().Name != "gen2" {
		t.Fatalf("最终配置 = %q, want gen2", w.Get().Name)
	}
}

// TestConfigWatcher_OnChangeCrossGoroutineWait 回归 core#137 形态二：onChange
// 同步等待另一 goroutine 的 Reload 完成——旧实现下那次 Reload 死锁在
// dispatchMu（被本回调持有）上，等待永不返回。
func TestConfigWatcher_OnChangeCrossGoroutineWait(t *testing.T) {
	w, src := newDispatchWatcher(t, func(d []byte) (*testConfig, error) {
		var cfg testConfig
		return &cfg, json.Unmarshal(d, &cfg)
	})

	otherReloadDone := make(chan struct{})
	w.OnChange(func(old, new *testConfig) {
		// 模拟「回调经别的 goroutine 间接等 Reload」：等其它 goroutine 的
		// Reload 完成信号再返回。
		<-otherReloadDone
	})

	src.set([]byte(`{"name":"gen1"}`))
	go func() { _ = w.Reload() }()

	// 另一 goroutine 的 Reload 必须能在预算内完成（旧实现：死锁）。
	src.set([]byte(`{"name":"gen2"}`))
	done := make(chan error, 1)
	go func() { done <- w.Reload() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("cross-goroutine reload: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("并发 Reload 被慢回调卡死（core#137 形态二/三）")
	}
	close(otherReloadDone) // 放行首个回调，dispatch goroutine 得以继续
}

// TestConfigWatcher_SlowCallbackDoesNotBlockReload 回归 core#137 形态三：
// 慢回调执行期间，后续 Reload 的 Swap（Get 可见新值）不被阻塞。
func TestConfigWatcher_SlowCallbackDoesNotBlockReload(t *testing.T) {
	w, src := newDispatchWatcher(t, func(d []byte) (*testConfig, error) {
		var cfg testConfig
		return &cfg, json.Unmarshal(d, &cfg)
	})

	release := make(chan struct{})
	started := make(chan struct{})
	w.OnChange(func(old, new *testConfig) {
		started <- struct{}{}
		<-release // 慢回调：卡住直到放行
	})

	src.set([]byte(`{"name":"gen1"}`))
	_ = w.Reload()
	<-started // 回调已开始执行（dispatch goroutine 上）

	// 慢回调未返回期间：重载 + Get 必须照常完成。
	src.set([]byte(`{"name":"gen2"}`))
	done := make(chan error, 1)
	go func() { done <- w.Reload() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("reload during slow callback: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("慢回调阻塞了后续重载（core#137 形态三）")
	}
	if got := w.Get().Name; got != "gen2" {
		t.Fatalf("慢回调期间 Get = %q, want gen2", got)
	}
	close(release)
}

// TestConfigWatcher_DispatchOrder 钉住投递保序：回调按重载应用的次序
// （Swap 序）逐个投递——每个回调的 old 必须等于上一个回调的 new，
// 最后投递的 new 必须等于 Get()（消费方不会把旧配置当最新应用，core#81
// 的序约束在队列化后不回退）。并发发起内容互异的重载：被代次裁决丢弃的
// 重载不投递，投递集合是重载集合的子集，但链式关系必须严格成立。
func TestConfigWatcher_DispatchOrder(t *testing.T) {
	w, src := newDispatchWatcher(t, func(d []byte) (*testConfig, error) {
		var cfg testConfig
		return &cfg, json.Unmarshal(d, &cfg)
	})

	const n = 24
	var mu sync.Mutex
	type pair struct{ old, nw string }
	var deliveries []pair
	w.OnChange(func(old, new *testConfig) {
		mu.Lock()
		deliveries = append(deliveries, pair{old: old.Name, nw: new.Name})
		mu.Unlock()
	})

	// 并发发起 n 次重载（内容互异，逐个换源再发）。
	var fireWg sync.WaitGroup
	for i := range n {
		fireWg.Add(1)
		go func(i int) {
			defer fireWg.Done()
			src.set([]byte(fmt.Sprintf(`{"name":"gen%d"}`, i+1)))
			_ = w.Reload() // 竞争失败（代次落后被丢弃）也是合法结局
		}(i)
	}
	fireWg.Wait()

	final := w.Get().Name
	// 入队在 Reload 返回前完成（入队于 dispatchMu 内）⇒ fireWg.Wait 后队列
	// 已收齐本批全部投递；剩余动作只是 dispatch goroutine 排空。等待计数
	// 稳定即到齐（被代次裁决丢弃的重载不投递，投递数 ≤ n 属正常）。
	deadline := time.Now().Add(5 * time.Second)
	last := -1
	for {
		mu.Lock()
		got := len(deliveries)
		mu.Unlock()
		if got == last && got > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("回调未排空: got %d", got)
		}
		last = got
		time.Sleep(20 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(deliveries) == 0 {
		t.Fatal("没有回调投递")
	}
	if deliveries[0].old != "gen0" {
		t.Fatalf("首次投递 old = %q, want gen0（初始配置）", deliveries[0].old)
	}
	for i := 1; i < len(deliveries); i++ {
		if deliveries[i].old != deliveries[i-1].nw {
			t.Fatalf("投递乱序（core#81 序约束回退）: [%d].old=%q != [%d].new=%q; all=%v",
				i, deliveries[i].old, i-1, deliveries[i-1].nw, deliveries)
		}
	}
	if last := deliveries[len(deliveries)-1].nw; last != final {
		t.Fatalf("最后一次投递 = %q, Get = %q（消费方拿到旧配置当最新）", last, final)
	}
}

// TestConfigWatcher_OnChangePanicIsolated 钉住新语义：回调 panic 被隔离为
// 日志，dispatch 循环存续——后续变更照常投递（历史行为：回调 panic 打死
// 进程面；队列化后若不隔离则永久哑掉通知面，更糟）。
func TestConfigWatcher_OnChangePanicIsolated(t *testing.T) {
	w, src := newDispatchWatcher(t, func(d []byte) (*testConfig, error) {
		var cfg testConfig
		return &cfg, json.Unmarshal(d, &cfg)
	})

	first := true
	delivered := make(chan string, 2)
	w.OnChange(func(old, new *testConfig) {
		if first {
			first = false
			delivered <- "panic"
			panic("boom: onChange 控制不了的用户代码")
		}
		delivered <- new.Name
	})

	src.set([]byte(`{"name":"gen1"}`))
	_ = w.Reload()
	if v := <-delivered; v != "panic" {
		t.Fatalf("首投递失真: %q", v)
	}

	// panic 后循环必须还活着：第二次变更照常投递。
	src.set([]byte(`{"name":"gen2"}`))
	_ = w.Reload()
	select {
	case v := <-delivered:
		if v != "gen2" {
			t.Fatalf("panic 后投递失真: %q", v)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("回调 panic 后 dispatch 循环死亡，后续变更通知丢失")
	}
}

// TestConfigWatcher_DispatchLoopStartedLazily 钉住懒启动：无回调投递
// （无 OnChange 或仅初始加载）不启动 dispatch goroutine——运行时
// goroutine 数可静态断言（runtime.NumGoroutine 的窗口断言在 CI 上抖，
// 这里用内部状态直接断言 pendingOnce 未触发）。
func TestConfigWatcher_DispatchLoopStartedLazily(t *testing.T) {
	w, src := newDispatchWatcher(t, func(d []byte) (*testConfig, error) {
		var cfg testConfig
		return &cfg, json.Unmarshal(d, &cfg)
	})

	// 无 OnChange 的重载：不投递、不启动 dispatch goroutine。
	src.set([]byte(`{"name":"gen1"}`))
	if err := w.Reload(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	time.Sleep(50 * time.Millisecond) // 给「若会启动早就启动」留窗口
	if w.dispatchRunning.Load() {
		t.Fatal("无回调时不应启动 dispatch goroutine")
	}

	// 有回调的重载：懒启动生效。
	w.OnChange(func(old, new *testConfig) {})
	src.set([]byte(`{"name":"gen2"}`))
	if err := w.Reload(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for !w.dispatchRunning.Load() {
		if time.Now().After(deadline) {
			t.Fatal("有回调投递时应启动 dispatch goroutine")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// 编译期钉住 ConfigSource 契约未变（core#137 只动投递路径，不动源接口）。
var _ ConfigSource = (*blockingSource)(nil)

var _ = context.Background
