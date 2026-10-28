package chain

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestRecovery_ComposedWithTimeout_CatchesPanic 回归测试：Recovery 与 Timeout
// **组合**使用时，下游 panic 必须被转成 *PanicError 返回，**不得**打死进程。
//
// 缺陷成因：Timeout 把下游放到**新 goroutine** 执行，而 Recovery 的 recover
// 装在**调用方** goroutine 上 ⇒ 下游 panic 根本到不了 Recovery ⇒ panic 发生在
// 无人 recover 的 goroutine 里 ⇒ **整个进程崩溃**。
//
// 而 `chain.Finalize(h, chain.Recovery(), chain.Timeout(d))` 正是这个顺序
// （Chain(a,b)(next) == a(b(next))）⇒ Recovery 的 godoc「下游 panic 被转换为
// *PanicError 返回…而非进程崩溃」在组合使用时是假的。
//
// 注意：不修复时本用例会让测试进程**直接崩掉**（这正是缺陷的表现）。
func TestRecovery_ComposedWithTimeout_CatchesPanic(t *testing.T) {
	var reported []PanicError
	h := Finalize(
		func(ctx context.Context, req any) (any, error) {
			panic("boom")
		},
		Recovery(WithPanicReporter(func(ctx context.Context, req any, pv PanicError) {
			reported = append(reported, pv)
		})),
		Timeout(time.Second),
	)

	resp, err := h(context.Background(), "req")
	if resp != nil {
		t.Fatalf("panic 后应返回 nil resp，实得 %v", resp)
	}
	var pe *PanicError
	if !errors.As(err, &pe) {
		t.Fatalf("应返回 *PanicError，实得 %v", err)
	}
	if pe.Value != "boom" {
		t.Fatalf("PanicError.Value = %v，期望 boom", pe.Value)
	}
	if pe.Stack == "" {
		t.Fatal("PanicError.Stack 应非空")
	}
	// 上报钩子也要被调用（Timeout 收编成错误后，Recovery 仍要上报）。
	if len(reported) != 1 {
		t.Fatalf("上报次数 = %d，期望 1", len(reported))
	}
}

// TestRecovery_Timeout_NoPanic_Unaffected 回归护栏：不 panic 时行为不变。
func TestRecovery_Timeout_NoPanic_Unaffected(t *testing.T) {
	h := Finalize(
		func(ctx context.Context, req any) (any, error) { return "ok", nil },
		Recovery(),
		Timeout(time.Second),
	)
	resp, err := h(context.Background(), "req")
	if err != nil {
		t.Fatalf("不应有错误，实得 %v", err)
	}
	if resp != "ok" {
		t.Fatalf("resp = %v，期望 ok", resp)
	}
}
