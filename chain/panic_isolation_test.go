package chain_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oekit/core/chain"
)

// ---------------------------------------------------------------------------
// Recovery 的 reporter panic 隔离（#55）
// ---------------------------------------------------------------------------

// TestRecovery_ReporterPanic_DoesNotKillProcess 回归：reporter 自己 panic 时
// 必须被隔离，原始 panic 仍以 *PanicError 返回。
//
// 缺陷成因：reporter 的调用发生在**已经 recover 过的 defer 闭包内部**，
// 该闭包里再 panic，同级 defer 已全部执行完毕，没有任何 recover 会接住
// ⇒ 整个进程退出。而 reporter 恰恰是最可能 panic 的代码（第三方监控 SDK）。
//
// 注意：隔离缺失时本用例会**直接打死测试进程**（正是缺陷的表现），
// 不是普通的断言失败。
func TestRecovery_ReporterPanic_DoesNotKillProcess(t *testing.T) {
	h := chain.Finalize(
		func(context.Context, any) (any, error) { panic("downstream-boom") },
		chain.Recovery(chain.WithPanicReporter(func(context.Context, any, chain.PanicError) {
			panic("reporter-boom")
		})),
	)

	resp, err := h(context.Background(), "req")
	if resp != nil {
		t.Fatalf("panic 后应返回 nil resp，实得 %v", resp)
	}
	var pe *chain.PanicError
	if !errors.As(err, &pe) {
		t.Fatalf("原始 panic 仍应转成 *PanicError 返回，实得 %v", err)
	}
	if pe.Value != "downstream-boom" {
		t.Fatalf("PanicError.Value = %v，want downstream-boom（reporter 的 panic 不得覆盖原始 panic）", pe.Value)
	}
}

// TestRecovery_ReporterPanic_OnPanicErrorReturnPath 覆盖第二个 reporter 调用点：
// 下游以 *PanicError **形式返回**（Timeout 收编后上抛）时的上报路径同样要隔离。
func TestRecovery_ReporterPanic_OnPanicErrorReturnPath(t *testing.T) {
	h := chain.Finalize(
		func(context.Context, any) (any, error) {
			return nil, &chain.PanicError{Value: "as-error"}
		},
		chain.Recovery(chain.WithPanicReporter(func(context.Context, any, chain.PanicError) {
			panic("reporter-boom")
		})),
	)

	_, err := h(context.Background(), "req")
	var pe *chain.PanicError
	if !errors.As(err, &pe) {
		t.Fatalf("应原样返回 *PanicError，实得 %v", err)
	}
	if pe.Value != "as-error" {
		t.Fatalf("PanicError.Value = %v，want as-error", pe.Value)
	}
}

// ---------------------------------------------------------------------------
// Timeout 的迟到 panic 上报（#56）
// ---------------------------------------------------------------------------

// TestTimeout_LatePanic_Reported 回归：超时已返回后下游才 panic，
// 该 *PanicError 必须被上报，而不是写进一个没人读的缓冲 channel 后消失。
//
// 缺陷成因：Timeout 把下游放进新 goroutine，recover 后的结果写进
// `done`（缓冲 1）；超时路径已从 ctx.Done() 返回 ⇒ 无人读取 ⇒ 既不进错误链
// 也不上报。排障时只剩「超时」一个假象。
func TestTimeout_LatePanic_Reported(t *testing.T) {
	var mu sync.Mutex
	var got []*chain.PanicError

	h := chain.Finalize(
		func(context.Context, any) (any, error) {
			time.Sleep(80 * time.Millisecond)
			panic("late-boom")
		},
		chain.Timeout(20*time.Millisecond, chain.WithLatePanicReporter(func(_ context.Context, pv *chain.PanicError) {
			mu.Lock()
			got = append(got, pv)
			mu.Unlock()
		})),
	)

	_, err := h(context.Background(), "req")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("超时路径应返回 DeadlineExceeded，实得 %v", err)
	}

	// 事件驱动等待：迟到 panic 发生在超时返回之后，不能靠睡眠猜。
	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		n := len(got)
		mu.Unlock()
		if n > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("超时返回后下游 panic 未被上报（迟到 *PanicError 被静默丢弃）")
		}
		time.Sleep(5 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 {
		t.Fatalf("迟到 panic 应恰好上报 1 次，实得 %d 次（重复上报会污染监控口径）", len(got))
	}
	if got[0].Value != "late-boom" {
		t.Fatalf("上报的 panic 值 = %v，want late-boom", got[0].Value)
	}
	if got[0].Stack == "" {
		t.Fatal("迟到的 PanicError 应带调用栈，否则无法定位")
	}
}

// TestTimeout_LatePanic_DefaultSlogSink 未注入钩子时必须落 stdlib slog：
// 「无人接收的 panic」至少要留下痕迹。
func TestTimeout_LatePanic_DefaultSlogSink(t *testing.T) {
	var buf bytes.Buffer
	var logMu sync.Mutex
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&lockedWriter{w: &buf, mu: &logMu}, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	h := chain.Finalize(
		func(context.Context, any) (any, error) {
			time.Sleep(80 * time.Millisecond)
			panic("late-default-sink")
		},
		chain.Timeout(20*time.Millisecond),
	)

	if _, err := h(context.Background(), "req"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("应返回 DeadlineExceeded，实得 %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		logMu.Lock()
		out := buf.String()
		logMu.Unlock()
		if strings.Contains(out, "late-default-sink") {
			if !strings.Contains(out, "chain.timeout") {
				t.Fatalf("默认出口应带 component=chain.timeout 便于过滤，实得日志：%s", out)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("未注入钩子时迟到 panic 应落 stdlib slog，实得日志：%q", out)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestTimeout_LatePanicReporter_NotCalledOnNormalPanic 反向对照：下游在超时
// **之前** panic 时，错误经 done 正常回传（外层 Recovery 负责上报），
// 迟到钩子**不得**被调用——否则同一笔 panic 会被报两次。
func TestTimeout_LatePanicReporter_NotCalledOnNormalPanic(t *testing.T) {
	var lateCalls atomic.Int32
	var recoverCalls atomic.Int32

	h := chain.Finalize(
		func(context.Context, any) (any, error) { panic("fast-boom") },
		chain.Recovery(chain.WithPanicReporter(func(context.Context, any, chain.PanicError) {
			recoverCalls.Add(1)
		})),
		chain.Timeout(time.Second, chain.WithLatePanicReporter(func(context.Context, *chain.PanicError) {
			lateCalls.Add(1)
		})),
	)

	_, err := h(context.Background(), "req")
	var pe *chain.PanicError
	if !errors.As(err, &pe) {
		t.Fatalf("应返回 *PanicError，实得 %v", err)
	}
	if n := recoverCalls.Load(); n != 1 {
		t.Fatalf("Recovery 的 reporter 应被调用 1 次，实得 %d", n)
	}
	// 迟到路径的上报在调用方 goroutine 内同步完成前已确定不会触发；
	// 这里再给一个短窗口，防止异步误报。
	time.Sleep(50 * time.Millisecond)
	if n := lateCalls.Load(); n != 0 {
		t.Fatalf("未超时的 panic 不得走迟到上报（会重复上报），实得 %d 次", n)
	}
}

// TestTimeout_LateReporterPanic_Isolated 迟到上报钩子自身 panic 也必须隔离：
// 它可能跑在下游 goroutine 里（那里没有 recover）。
func TestTimeout_LateReporterPanic_Isolated(t *testing.T) {
	h := chain.Finalize(
		func(context.Context, any) (any, error) {
			time.Sleep(80 * time.Millisecond)
			panic("boom")
		},
		chain.Timeout(20*time.Millisecond, chain.WithLatePanicReporter(func(context.Context, *chain.PanicError) {
			panic("late-reporter-boom")
		})),
	)

	if _, err := h(context.Background(), "req"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("应返回 DeadlineExceeded，实得 %v", err)
	}
	// 钩子在下游 goroutine 里被调用；隔离缺失时整个测试进程会被打死，
	// 因此这里只需要「活着跑到终点」即算通过。
	time.Sleep(150 * time.Millisecond)
}

// lockedWriter 让 bytes.Buffer 在 slog 的并发写入下安全。
type lockedWriter struct {
	w  *bytes.Buffer
	mu *sync.Mutex
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}
