package config

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// panicWatchSource 在 Watch 调用栈内**同步**触发 onChange / 自身 panic——
// 真实源（Nacos 长轮询、文件 watcher）的形态：回调跑在 source.Watch 的
// 执行 goroutine 上，而不是源自己另开的 goroutine（stubSource 那样自带
// goroutine 的源会把 panic 隔在自己那层，复现不了 #119）。
type panicWatchSource struct {
	consumerPanic bool // true：onChange 回调 panic（消费方代码）；false：Watch 本体 panic（源代码）
}

func (s *panicWatchSource) Read() ([]byte, error) { return []byte("{}"), nil }

func (s *panicWatchSource) Watch(ctx context.Context, onChange func()) {
	if s.consumerPanic {
		onChange() // 回调即 panic——panic 落在 OverlaySource 的扇出 goroutine 上
	} else {
		panic("subsource watch exploded")
	}
	<-ctx.Done() // panic 被收编后走不到这里；ctx 取消时正常退出
}

// TestOverlaySource_Watch_ConsumerPanicIsolated 回归 core#119：OverlaySource.
// Watch 此前把每个子源的 Watch 丢进**裸 goroutine**（无 recover），而 onChange
// 是消费方回调（典型：ConfigWatcher.Start → Reload → 消费方经 OnChange 注册
// 的函数）——回调里的任何 panic 都发生在无人 recover 的 goroutine 里 ⇒ 整个
// 进程崩溃，调用方自己 goroutine 上的 defer recover() 根本看不到（recover
// 只对当前 goroutine 生效）。修后 panic 就地收编并打 ERROR 留痕，进程不受影响。
func TestOverlaySource_Watch_ConsumerPanicIsolated(t *testing.T) {
	testOverlayWatchPanicIsolated(t, true)
}

// TestOverlaySource_Watch_SubSourcePanicIsolated 同一纪律的另一半：子源自身
// Watch 实现里的 panic 同样不得打死进程（子源可能是第三方 SDK 的代码）。
func TestOverlaySource_Watch_SubSourcePanicIsolated(t *testing.T) {
	testOverlayWatchPanicIsolated(t, false)
}

func testOverlayWatchPanicIsolated(t *testing.T, consumerPanic bool) {
	t.Helper()

	// OverlaySource 的 panic 上报走 slog.Default()（与 chain/timeout.go 的
	// 缺省出口同形态）：测试里替换默认 logger 收集痕迹，跑完恢复。
	prev := slog.Default()
	logger, h := newCaptureLogger()
	slog.SetDefault(logger)
	defer slog.SetDefault(prev)

	want := "consumer callback exploded"
	if !consumerPanic {
		want = "subsource watch exploded"
	}
	src := OverlaySource(&panicWatchSource{consumerPanic: consumerPanic})
	src.Watch(context.Background(), func() { panic(want) })

	// 判据一（隐含）：测试进程还活着——修复前 panic 直接打死测试二进制，
	// 本函数根本跑不到断言。判据二：panic 痕迹落进 ERROR 日志（panic 值
	// 可定位），不是无声吞掉。
	for range 200 { // 最多等 2s（10ms × 200）
		h.mu.Lock()
		found := false
		for i, msg := range h.calls {
			if !strings.Contains(msg, "panic") {
				continue
			}
			for j := 0; j+1 < len(h.fields[i]); j += 2 {
				if s, ok := h.fields[i][j+1].(string); ok && strings.Contains(s, want) {
					found = true
				}
			}
		}
		h.mu.Unlock()
		if found {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("子源/回调 panic 未被隔离并留痕（core#119）：ERROR 日志里没看到 %q", want)
}
