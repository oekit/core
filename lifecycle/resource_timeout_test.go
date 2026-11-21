package lifecycle

import (
	"context"
	"testing"
	"time"
)

// 编译期钉住 core#227 的能力面：Manager 与 bootstrap.App 同签名的
// AddResourceTimeout 必须让 Manager 满足可选能力接口。
var _ ResourceAdderWithTimeout = (*Manager)(nil)

// TestManagerAddResourceTimeout_BudgetIsolated 钉住 core#227：
// AddResourceTimeout 的预算必须**独立生效**，不被 Manager 默认超时封顶。
// 默认超时故意设得远小于资源预算——若实现忽略 timeout（或误用默认值），
// closer 会立刻拿到已取消的 ctx，测试失败。
func TestManagerAddResourceTimeout_BudgetIsolated(t *testing.T) {
	const (
		defaultTimeout = 5 * time.Millisecond
		resourceBudget = 120 * time.Millisecond
	)

	m := New(defaultTimeout)

	var (
		deadlineAtEntry time.Duration
		hasDeadline     bool
	)
	m.AddResourceTimeout("slow", resourceBudget, func(ctx context.Context) error {
		deadline, ok := ctx.Deadline()
		hasDeadline = ok
		if ok {
			deadlineAtEntry = time.Until(deadline)
		}
		<-ctx.Done()
		return ctx.Err()
	})

	start := time.Now()
	m.Shutdown(context.Background())
	elapsed := time.Since(start)

	if !hasDeadline {
		t.Fatal("budgeted closer must receive a ctx with deadline")
	}
	if deadlineAtEntry <= defaultTimeout*2 {
		t.Fatalf("budget = %v at closer entry, want close to %v (Manager default %v must not cap per-resource budget)",
			deadlineAtEntry, resourceBudget, defaultTimeout)
	}
	if elapsed < resourceBudget*3/4 {
		t.Fatalf("shutdown returned after %v, want >= ~%v (per-resource budget not applied)", elapsed, resourceBudget)
	}
}

// TestAddResourceWithTimeout_PrefersCapability 钉住 core#227 的统一注册面：
// App 实现可选能力接口时走 AddResourceTimeout（timeout 透传）。
func TestAddResourceWithTimeout_PrefersCapability(t *testing.T) {
	app := &timedApp{}
	AddResourceWithTimeout(app, "svc", 3*time.Second, func(context.Context) error { return nil })

	if len(app.names) != 0 {
		t.Fatalf("capability path must not fall back to AddResource, got %v", app.names)
	}
	if app.name != "svc" || app.timeout != 3*time.Second {
		t.Fatalf("AddResourceTimeout got (name=%q, timeout=%v), want (svc, 3s)", app.name, app.timeout)
	}
}

// TestAddResourceWithTimeout_FallsBackToAddResource 钉住 core#227 的兼容
// 路径：无预算能力的旧 App 实现回退到 AddResource，timeout 被忽略但仍完成
// 注册（不得静默丢资源）。
func TestAddResourceWithTimeout_FallsBackToAddResource(t *testing.T) {
	app := &plainApp{}
	called := false
	AddResourceWithTimeout(app, "svc", 3*time.Second, func(context.Context) error {
		called = true
		return nil
	})

	if len(app.names) != 1 || app.names[0] != "svc" {
		t.Fatalf("fallback must register resource via AddResource, got %v", app.names)
	}
	if err := app.closers["svc"](context.Background()); err != nil || !called {
		t.Fatalf("fallback closer not wired correctly: err=%v called=%v", err, called)
	}
}

// plainApp 只实现最小 App 契约（无 per-resource 预算能力）。
type plainApp struct {
	names   []string
	closers map[string]func(context.Context) error
}

func (a *plainApp) Context() context.Context { return context.Background() }

func (a *plainApp) AddResource(name string, closer func(context.Context) error) {
	a.names = append(a.names, name)
	if a.closers == nil {
		a.closers = make(map[string]func(context.Context) error)
	}
	a.closers[name] = closer
}

func (a *plainApp) GoBackground(string, func(context.Context)) {}

func (a *plainApp) RequestShutdown() {}

// timedApp 在最小契约之上实现可选能力接口。
type timedApp struct {
	plainApp
	name    string
	timeout time.Duration
}

func (a *timedApp) AddResourceTimeout(name string, timeout time.Duration, _ func(context.Context) error) {
	a.name = name
	a.timeout = timeout
}
