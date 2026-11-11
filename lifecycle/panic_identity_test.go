package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// errCloserBoom 是 closer 经 panic(err) 上抛的业务哨兵——部署门禁/审计对
// 关停失败分流靠 errors.Is 定位错误类（core#136：此前 %v 把身份拍平，
// Is/As 全失效）。
var errCloserBoom = errors.New("closer: connection pool drain failed")

// TestManager_PanicErrorIdentityPreserved 回归 core#136：closer 以
// panic(err) 失败时，Manager 错误面必须保留原始 error 身份——
// errors.Is(errors.Join(m.Errors()...), errCloserBoom) 成立。
func TestManager_PanicErrorIdentityPreserved(t *testing.T) {
	m := New(time.Second)
	m.Add("pool", func(context.Context) error { panic(errCloserBoom) }, 0)
	m.Shutdown(context.Background())

	errs := m.Errors()
	if len(errs) != 1 {
		t.Fatalf("expected 1 recorded error, got %d: %v", len(errs), errs)
	}
	if !errors.Is(errs[0], errCloserBoom) {
		t.Fatalf("panic(err) 身份丢失（core#136）: %v", errs[0])
	}
	// 类型信息恒在场（*PanicError.Error() = "panic: %T: %v"）。
	if !strings.Contains(errs[0].Error(), "*errors.errorString") ||
		!strings.Contains(errs[0].Error(), "closer: connection pool drain failed") {
		t.Fatalf("错误文本应含 panic 值类型与值: %v", errs[0])
	}
}

// TestManager_PanicNonErrorValueStillRecorded 非 error 的 panic 值照常收编，
// 文本带类型信息（%T），errors.Is 不误判。
func TestManager_PanicNonErrorValueStillRecorded(t *testing.T) {
	m := New(time.Second)
	m.AddTeardown(func() { panic("teardown exploded") })
	m.Shutdown(context.Background())

	errs := m.Errors()
	if len(errs) != 1 {
		t.Fatalf("expected 1 recorded error, got %d: %v", len(errs), errs)
	}
	if errors.Is(errs[0], errCloserBoom) {
		t.Fatal("不同 panic 值不应被 errors.Is 误判")
	}
	if !strings.Contains(errs[0].Error(), "panic: string: teardown exploded") {
		t.Fatalf("非 error panic 值文本失真: %v", errs[0])
	}
}
