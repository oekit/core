package lifecycle

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// TestWithLogger_RoutesInternalLogsToHostLogger 验证注入宿主 slog.Logger 后，
// Manager 内部日志写入该 logger，且 job 名出现在日志字段中。
func TestWithLogger_RoutesInternalLogsToHostLogger(t *testing.T) {
	var buf bytes.Buffer
	host := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	mgr := New(time.Second, WithLogger(host))
	mgr.Add("myjob", func(context.Context) error { return errors.New("boom") }, 0)
	mgr.Shutdown(context.Background())

	out := buf.String()
	if !strings.Contains(out, "resource close failed") {
		t.Errorf("expected failure log in host logger, got: %s", out)
	}
	if !strings.Contains(out, "myjob") {
		t.Errorf("expected job name in host logger output, got: %s", out)
	}
	if !strings.Contains(out, "shutdown completed") {
		t.Errorf("expected completion log in host logger, got: %s", out)
	}
}

// TestWithLogger_NilKeepsDefault 传 nil 等同未注入，不 panic 且默认 sink 正常工作。
func TestWithLogger_NilKeepsDefault(t *testing.T) {
	mgr := New(time.Second, WithLogger(nil))
	var called bool
	mgr.Add("ok", func(context.Context) error { called = true; return nil }, 0)
	mgr.Shutdown(context.Background())
	if !called {
		t.Fatal("job was not executed")
	}
	if len(mgr.Errors()) != 0 {
		t.Errorf("expected no errors, got %v", mgr.Errors())
	}
}

// TestErrors_CollectsTeardownAndJobFailures 验证 Errors 同时覆盖 teardown 失败
// 与 closer job 失败（停机错误可经 Errors() 事后观测）。
func TestErrors_CollectsTeardownAndJobFailures(t *testing.T) {
	mgr := New(time.Second)
	jobErr := errors.New("job failed")

	mgr.AddTeardown(func() { panic("teardown boom") })
	mgr.Add("fails", func(context.Context) error { return jobErr }, 0)

	mgr.Shutdown(context.Background())

	errs := mgr.Errors()
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors (teardown + job), got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Error(), "teardown") {
		t.Errorf("expected first error to be teardown failure, got %v", errs[0])
	}
	if !errors.Is(errs[1], jobErr) {
		t.Errorf("expected second error to wrap jobErr, got %v", errs[1])
	}
}

// TestErrors_ReturnsCopy 验证返回切片是副本，外部修改不影响内部状态。
func TestErrors_ReturnsCopy(t *testing.T) {
	mgr := New(time.Second)
	mgr.Add("fails", func(context.Context) error { return errors.New("x") }, 0)
	mgr.Shutdown(context.Background())

	errs := mgr.Errors()
	errs[0] = nil
	if mgr.Errors()[0] == nil {
		t.Error("Errors() should return a copy; internal slice was mutated")
	}
}
