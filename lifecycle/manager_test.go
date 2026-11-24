package lifecycle

import (
	"context"
	"testing"
	"time"
)

// core#190/#191：Release 归零后不再推负（Drain 的 <=0 判据不把负数当
// 已排空）；Shutdown/Drain 接受 nil ctx。
func TestReleaseNotNegativeAndNilCtx(t *testing.T) {
	m := New(time.Second)
	m.Release() // 未配对释放：归零即止
	m.Release() // 重复释放：不再推负
	if got := m.refCount.Load(); got != 0 {
		t.Fatalf("refCount = %d, want 0(不推负)", got)
	}
	// nil ctx 是被测语义（core#191）；经变量传入避开 staticcheck SA1012
	// 对字面 nil 的告警。
	var nilCtx context.Context
	done := make(chan struct{})
	go func() { m.Shutdown(nilCtx); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Shutdown(nil) 未返回(nil ctx 应按 Background)")
	}
	m.Drain(nilCtx) // 不 panic
}
