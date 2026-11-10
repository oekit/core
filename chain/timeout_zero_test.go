package chain

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestTimeout_ZeroMeansNoTimeout 回归 core#117：Timeout(0) 此前把 0 无检查地
// 喂给 context.WithTimeout——ctx 立即到期，每个请求 0ms 死于 DeadlineExceeded
// （超时来自配置面时零值透传的服务「进程活着、全量请求瞬死」，且无日志无
// 指标指向 Timeout(0) 本身）。与 lifecycle 在 core#86 确立的口径统一：
// **0 = 显式无超时**——原样透传 next，不派生 ctx、不加 deadline。
func TestTimeout_ZeroMeansNoTimeout(t *testing.T) {
	h := Timeout(0)(func(ctx context.Context, req any) (any, error) {
		if _, has := ctx.Deadline(); has {
			t.Error("Timeout(0) must not attach a deadline to the downstream ctx (core#117)")
		}
		time.Sleep(30 * time.Millisecond) // 修复前：ctx 已到期，调用方 0ms 拿到 DeadlineExceeded
		return "ok", nil
	})
	resp, err := h(context.Background(), "req")
	if err != nil {
		t.Fatalf("Timeout(0) 应为显式无超时（core#117）, err = %v", err)
	}
	if resp != "ok" {
		t.Fatalf("resp = %v, want ok", resp)
	}
}

// TestTimeout_Zero_ComposesWithRecovery 0 = 透传的连带契约：下游回到调用方
// goroutine 上执行，Recovery 的 recover 恢复生效（d>0 时下游在独立 goroutine、
// 由 Timeout 内部 recover 收编——两条路径都不得留进程级逃逸口）。
func TestTimeout_Zero_ComposesWithRecovery(t *testing.T) {
	h := Finalize(
		func(ctx context.Context, req any) (any, error) { panic("boom") },
		Recovery(),
		Timeout(0),
	)
	_, err := h(context.Background(), "req")
	var pe *PanicError
	if !errors.As(err, &pe) || pe.Value != "boom" {
		t.Fatalf("Timeout(0) 透传形态下 Recovery 仍要收编下游 panic, err = %v", err)
	}
}

// TestTimeout_NegativeFailsLoud 回归 core#117 的另一半：负值此前与 0 同一
// 静默杀伤形态（产出立即到期的中间件）。负值只可能来自上游计算错误，按
// AGENTS.md「Option 非法值总则①」构造期 fail-fast：panic，不留静默坑。
func TestTimeout_NegativeFailsLoud(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("Timeout(-1s) 必须构造期 panic（fail-loud，core#117），此前静默产出立即到期的中间件")
		}
		if !strings.Contains(fmt.Sprint(r), "-1s") {
			t.Errorf("panic 信息应携带非法值本身, got %v", r)
		}
	}()
	_ = Timeout(-time.Second)
}
