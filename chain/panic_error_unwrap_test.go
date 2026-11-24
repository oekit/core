package chain_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oekit/core/chain"
)

// ---------------------------------------------------------------------------
// PanicError 的 Unwrap：panic(err) 业务错误身份透传（#207）
// ---------------------------------------------------------------------------

// errBoom 模拟业务侧 panic 出去的普通错误（errors.New 形态）。
var errBoom = errors.New("boom")

// bizSentinel 模拟带类型的业务哨兵（errors.As 要能穿透到它）。
type bizSentinel struct{ code string }

func (e *bizSentinel) Error() string { return "biz error: " + e.code }

// TestPanicError_Unwrap_ErrorValue 直接覆盖 Unwrap 语义：panic 值是 error
// 时必须原样返回，errors.Is / errors.As 穿透 *PanicError 直达原始错误。
func TestPanicError_Unwrap_ErrorValue(t *testing.T) {
	sentinel := &bizSentinel{code: "x001"}
	err := error(&chain.PanicError{Value: sentinel, Stack: "stack"})

	if !errors.Is(err, sentinel) {
		t.Fatalf("errors.Is 应穿透 PanicError 命中原始哨兵，实得 %v", err)
	}
	var bs *bizSentinel
	if !errors.As(err, &bs) || bs.code != "x001" {
		t.Fatalf("errors.As 应穿透 PanicError 拿到 *bizSentinel，实得 %v", err)
	}
	if u := err.(*chain.PanicError).Unwrap(); u != error(sentinel) {
		t.Fatalf("Unwrap() = %v，want 原始 panic 值 %v", u, sentinel)
	}
}

// TestRecovery_PanicSentinel_IdentityPreserved 回归：handler 以 panic(err)
// 表达不可恢复错误，经 Recovery 收编后调用方 errors.Is / errors.As 必须仍
// 能命中——否则业务哨兵判定全失效（#207 病灶）。
func TestRecovery_PanicSentinel_IdentityPreserved(t *testing.T) {
	h := chain.Finalize(
		func(context.Context, any) (any, error) { panic(errBoom) },
		chain.Recovery(),
	)

	resp, err := h(context.Background(), "req")
	if resp != nil {
		t.Fatalf("panic 后应返回 nil resp，实得 %v", resp)
	}
	if !errors.Is(err, errBoom) {
		t.Fatalf("errors.Is(err, errBoom) 应命中，实得 %v", err)
	}
	var pe *chain.PanicError
	if !errors.As(err, &pe) {
		t.Fatalf("外层形态仍应是 *PanicError，实得 %v", err)
	}
}

// TestRecovery_PanicTypedSentinel_AsHits 覆盖 errors.As 路径：panic 带类型
// 的业务哨兵（自定义 struct 实现 error）经 Recovery 收编后仍能按类型取出。
func TestRecovery_PanicTypedSentinel_AsHits(t *testing.T) {
	h := chain.Finalize(
		func(context.Context, any) (any, error) { panic(&bizSentinel{code: "x002"}) },
		chain.Recovery(),
	)

	_, err := h(context.Background(), "req")
	var bs *bizSentinel
	if !errors.As(err, &bs) || bs.code != "x002" {
		t.Fatalf("errors.As 应穿透 *PanicError 命中 *bizSentinel，实得 %v", err)
	}
}

// TestTimeout_PanicSentinel_IdentityPreserved 覆盖 Timeout 路径：Timeout 把
// 下游放进新 goroutine 并自行 recover 成 *PanicError 回传，身份透传同样要
// 生效（canonical 组合 Recovery+Timeout 下调用方拿到的就是这条路径的产物）。
func TestTimeout_PanicSentinel_IdentityPreserved(t *testing.T) {
	h := chain.Finalize(
		func(context.Context, any) (any, error) { panic(errBoom) },
		chain.Recovery(),
		chain.Timeout(time.Second),
	)

	resp, err := h(context.Background(), "req")
	if resp != nil {
		t.Fatalf("panic 后应返回 nil resp，实得 %v", resp)
	}
	if !errors.Is(err, errBoom) {
		t.Fatalf("经 Timeout 收编后 errors.Is(err, errBoom) 仍应命中，实得 %v", err)
	}
	var pe *chain.PanicError
	if !errors.As(err, &pe) {
		t.Fatalf("外层形态仍应是 *PanicError，实得 %v", err)
	}
}

// TestPanicError_Unwrap_NonErrorValueNil 反向对照：panic 非 error 值
// （string、int 等）时 Unwrap 返回 nil，不产生任何错误的身份匹配；
// 既有行为（*PanicError 形态返回、Error() 文本含原值）不变。
func TestPanicError_Unwrap_NonErrorValueNil(t *testing.T) {
	h := chain.Finalize(
		func(context.Context, any) (any, error) { panic("string-boom") },
		chain.Recovery(),
	)

	_, err := h(context.Background(), "req")
	var pe *chain.PanicError
	if !errors.As(err, &pe) {
		t.Fatalf("非 error panic 值仍应转成 *PanicError 返回，实得 %v", err)
	}
	if u := pe.Unwrap(); u != nil {
		t.Fatalf("panic 值非 error 时 Unwrap() 应返回 nil，实得 %v", u)
	}
	if errors.Is(err, errBoom) {
		t.Fatal("panic 值非 error 时不得伪造与任何哨兵的身份匹配")
	}
	if want := "panic: string-boom"; pe.Error() != want {
		t.Fatalf("Error() = %q，want %q（非 error 值行为不变）", pe.Error(), want)
	}
}
