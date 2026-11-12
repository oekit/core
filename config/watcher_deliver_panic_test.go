package config

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

// core#157 的护栏：panic 收编出口自身必须防 panic——回调 panic 之后宿主
// logger 再 panic（第三方日志 SDK 内部错误形态），不能打死 dispatch
// goroutine / 子源监听 goroutine（进程级 abort），「循环存续」的承诺必须
// 无条件成立。

// panicHandler 是最坏故障 logger：任何 Handle 都 panic（模拟宿主日志
// SDK 内部错误 / 并发 map 写 / 序列化故障）。
type panicHandler struct{}

func (panicHandler) Enabled(context.Context, slog.Level) bool { return true }
func (panicHandler) Handle(context.Context, slog.Record) error {
	panic("logger internal failure")
}
func (panicHandler) WithAttrs([]slog.Attr) slog.Handler { return panicHandler{} }
func (panicHandler) WithGroup(string) slog.Handler      { return panicHandler{} }

// TestDeliver_LoggerPanicContained 单元面：回调 panic + logger 在收编出口内
// 再 panic 的双重故障下，deliver 必须正常返回（未防护时这里的第二个 panic
// 直接炸穿测试进程）。
func TestDeliver_LoggerPanicContained(t *testing.T) {
	watcher := NewConfigWatcher(newMockSource([]byte(`{"port":8080}`)),
		func(b []byte) (*testConfig, error) {
			var c testConfig
			return &c, json.Unmarshal(b, &c)
		})
	watcher.SetLogger(slog.New(panicHandler{}))

	watcher.deliver(pendingChange[testConfig]{
		fn:  func(old, new *testConfig) { panic("callback boom") },
		old: &testConfig{}, nw: &testConfig{},
	})
}

// TestDispatchLoop_SurvivesLoggerPanic 回归面（issue #157 点名的承诺）：
// logger.Error panic 时 dispatch 循环仍存续、后续配置变更仍投递。
// 回调前两次必 panic（经 dispatch goroutine 投递）、第三次正常——
// 循环必须把第三次的回调送出。
func TestDispatchLoop_SurvivesLoggerPanic(t *testing.T) {
	source := newMockSource([]byte(`{"port":8080}`))
	watcher := NewConfigWatcher(source, func(b []byte) (*testConfig, error) {
		var c testConfig
		return &c, json.Unmarshal(b, &c)
	})
	watcher.SetLogger(slog.New(panicHandler{}))

	var seq atomic.Int32
	good := make(chan *testConfig, 1)
	watcher.OnChange(func(old, new *testConfig) {
		if seq.Add(1) <= 2 {
			panic("callback panic (attempt)")
		}
		good <- new
	})

	// 首载不投递；随后三次重载：前两次回调 panic，第三次正常送达
	for i := 0; i < 4; i++ {
		if err := watcher.Reload(); err != nil {
			t.Fatalf("Reload failed: %v", err)
		}
		source.Update([]byte(`{"port":8081}`))
	}

	select {
	case cfg := <-good:
		if cfg == nil || cfg.Port != 8081 {
			t.Fatalf("第三次回调应送达且携带最新配置，实得 %+v", cfg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("logger 再 panic 两次后 dispatch 循环未存续：后续配置变更未投递（core#157）")
	}
}

// panickingSource 是必然 panic 的子源（子源实现自身故障形态）。
type panickingSource struct{}

func (panickingSource) Read() ([]byte, error) { return []byte("{}"), nil }
func (panickingSource) Watch(context.Context, func()) {
	panic("child source internal failure")
}

// TestOverlaySource_ChildPanicWithPanickingLogger_Continues 同族位置
// （OverlaySource.Watch 收编出口，issue #157 一并处理）：slog.Default
// 也换成故障 handler 后，子源 panic + 留痕 panic 的双重故障被兜住——
// 其余子源的扇入继续工作。未防护时本用例直接炸穿测试进程。
func TestOverlaySource_ChildPanicWithPanickingLogger_Continues(t *testing.T) {
	orig := slog.Default()
	slog.SetDefault(slog.New(panicHandler{}))
	t.Cleanup(func() { slog.SetDefault(orig) })

	childA := panickingSource{}                   // Watch 必 panic：触发收编出口
	childB := newMockSource([]byte(`{"port":1}`)) // 正常子源：扇入必须不受牵连
	overlay := OverlaySource(childA, childB)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got := make(chan struct{}, 1)
	overlay.Watch(ctx, func() { got <- struct{}{} })

	// childB 的 onChange 在其监听 goroutine 内注册——事件驱动等待注册完成，
	// 避免 Update 早于注册的时序假红。
	deadline := time.Now().Add(2 * time.Second)
	for func() bool {
		childB.mu.RLock()
		defer childB.mu.RUnlock()
		return childB.onChange == nil
	}() {
		if time.Now().After(deadline) {
			t.Fatal("childB 的 onChange 未注册（OverlaySource 未启动子源监听 goroutine）")
		}
		time.Sleep(2 * time.Millisecond)
	}

	childB.Update([]byte(`{"port":2}`))
	select {
	case <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("双重故障后其余子源扇入失效：子源 panic + logger panic 未被收编出口兜住（core#157）")
	}
}
