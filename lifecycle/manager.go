package lifecycle

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"time"
)

// defaultLogger 是 Manager 未注入 WithLogger 时的内部日志出口。
// 使用 stdlib slog（component=shutdown），保证 lifecycle 包零外部依赖。
func defaultLogger() *slog.Logger {
	return slog.Default().With("component", "shutdown")
}

// logSink 是 Manager 内部日志出口的最小接口：*slog.Logger 与 logx.Logger
// 的方法签名兼容（`...any` 键值对），二者均可直接作为实现。
type logSink interface {
	Info(msg string, keysAndValues ...any)
	Warn(msg string, keysAndValues ...any)
	Error(msg string, keysAndValues ...any)
}

// ManagerOption 配置 Manager 的函数选项。
type ManagerOption func(*Manager)

// WithLogger 注入宿主 slog.Logger：Manager 内部日志（含 job 名等结构化字段）
// 写入该 logger，便于与宿主日志体系统一采集、告警。
// 未注入时保持默认行为（stdlib slog，component=shutdown）。
// 传入 nil 等同未注入。
func WithLogger(l *slog.Logger) ManagerOption {
	return func(m *Manager) {
		if l != nil {
			m.logger = l
		}
	}
}

// Manager 管理优雅关闭的资源。
type Manager struct {
	jobs         []job
	timeout      time.Duration
	drainTimeout time.Duration
	once         sync.Once
	mu           sync.Mutex
	teardowns    []func()
	shutdownErrs []error
	shutdownDone atomic.Bool
	logger       logSink

	// 引用计数和请求阻断
	refCount  atomic.Int64
	accepting atomic.Bool
	acqMu     sync.Mutex // 保护 Acquire 与 Shutdown 之间的原子性
}

// defaultDrainTimeout 是 drain 阶段未显式配置时的默认等待时长。
const defaultDrainTimeout = 30 * time.Second

type job struct {
	name    string
	closer  func(ctx context.Context) error
	timeout time.Duration
}

// New 创建一个关闭管理器。defaultTimeout 是默认的单个资源关闭超时。
// 可选 WithLogger 注入宿主日志；不传则使用 stdlib slog。
func New(defaultTimeout time.Duration, opts ...ManagerOption) *Manager {
	m := &Manager{
		timeout: defaultTimeout,
		logger:  defaultLogger(),
	}
	m.accepting.Store(true)
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Acquire 请求许可（返回 false 表示正在停机）。
// 在处理请求前调用，成功时必须调用 Release。
// 使用 acqMu 保证 accept 检查与 refCount 递增的原子性，
// 避免与 Shutdown 之间的竞态窗口。
func (m *Manager) Acquire() bool {
	m.acqMu.Lock()
	defer m.acqMu.Unlock()
	if !m.accepting.Load() {
		return false
	}
	m.refCount.Add(1)
	return true
}

// Release 释放请求许可。
// 在请求处理完成后调用。
func (m *Manager) Release() {
	m.refCount.Add(-1)
}

// GetRefCount 获取当前进行中的请求数（用于监控）。
func (m *Manager) GetRefCount() int64 {
	return m.refCount.Load()
}

// IsAccepting 是否正在接受新请求。
func (m *Manager) IsAccepting() bool {
	return m.accepting.Load()
}

// Add 注册一个需要在关闭时清理的资源。
// timeout 传 0 使用 Manager 的默认超时。
func (m *Manager) Add(name string, closer func(ctx context.Context) error, timeout time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if timeout == 0 {
		timeout = m.timeout
	}
	m.jobs = append(m.jobs, job{name: name, closer: closer, timeout: timeout})
}

// AddCloser 注册一个 io.Closer 资源。
func (m *Manager) AddCloser(name string, closer interface{ Close() error }, timeout time.Duration) {
	m.Add(name, func(_ context.Context) error { return closer.Close() }, timeout)
}

// AddTeardown 注册一个无参数的清理函数（不接受 context，不返回 error）。
func (m *Manager) AddTeardown(fn func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.teardowns = append(m.teardowns, fn)
}

// Listen 监听系统信号，收到信号后执行关闭。
// 会阻塞直到所有资源关闭完成或超时。
func (m *Manager) Listen(signals ...os.Signal) {
	if len(signals) == 0 {
		signals = []os.Signal{os.Interrupt}
	}
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, signals...)
	sig := <-ch
	m.logger.Info("received signal", "signal", sig)
	m.Shutdown(context.Background())
}

// Shutdown 执行所有注册的清理任务。
// 按注册顺序依次执行；每个任务拥有独立的超时预算（从 Background 派生），
// 前序任务的耗时不会挤占后续任务的超时。ctx 仅作为取消信号传播到正在
// 执行的任务（外部取消/超时会中止当前任务），不作为任务间共享的总预算；
// 需要全局兜底时请为每个任务设置合理超时（Add 的 timeout 参数）。
// 任务 panic 被隔离为错误记录，不会中断剩余清理。
// 多次调用是安全的，只有第一次调用会执行清理。
func (m *Manager) Shutdown(ctx context.Context) {
	m.once.Do(func() {
		// 1. 停止接受新请求（与 Acquire 互斥，确保不会有请求被漏算）
		m.acqMu.Lock()
		m.accepting.Store(false)
		m.acqMu.Unlock()
		m.logger.Info("stopped accepting new requests")

		// 2. 先执行 teardowns
		m.mu.Lock()
		teardowns := make([]func(), len(m.teardowns))
		copy(teardowns, m.teardowns)
		jobs := make([]job, len(m.jobs))
		copy(jobs, m.jobs)
		m.mu.Unlock()

		for _, fn := range teardowns {
			if err := runFunc("teardown", fn); err != nil {
				m.mu.Lock()
				m.shutdownErrs = append(m.shutdownErrs, err)
				m.mu.Unlock()
				m.logger.Error("teardown failed", "err", err)
			}
		}

		// 3. 再执行 closer jobs（使用本地副本，避免读 m.jobs 时与 Add 竞争）
		for _, j := range jobs {
			if err := m.runCloser(j, ctx); err != nil {
				m.mu.Lock()
				m.shutdownErrs = append(m.shutdownErrs, fmt.Errorf("%s: %w", j.name, err))
				m.mu.Unlock()
				m.logger.Error("resource close failed", "resource", j.name, "err", err)
			} else {
				m.logger.Info("resource closed", "resource", j.name)
			}
		}

		m.shutdownDone.Store(true)
		m.logger.Info("shutdown completed")
	})
}

// runFunc 执行无参清理函数并隔离 panic：panic 会跳过剩余清理任务，
// 且 sync.Once 视其为已完成，导致后续 Shutdown 静默 no-op。
func runFunc(name string, fn func()) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%s: panicked: %v", name, r)
		}
	}()
	fn()
	return nil
}

// runCloser 执行单个 closer 任务：独立超时预算 + 父 context 取消传播 + panic 隔离。
func (m *Manager) runCloser(j job, parent context.Context) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%s: panicked: %v", j.name, r)
		}
	}()

	// 独立预算：从 Background 派生，前序任务的耗时不会挤占本任务的超时
	jobCtx, cancel := context.WithTimeout(context.Background(), j.timeout)
	defer cancel()
	// 调用方 ctx 已取消/超时：同步取消本任务，保证 closer 立即观察到取消状态
	if err := parent.Err(); err != nil {
		cancel()
		return j.closer(jobCtx)
	}
	// 调用方 ctx 取消作为外部中止信号同步取消当前任务
	stop := context.AfterFunc(parent, cancel)
	defer stop()

	return j.closer(jobCtx)
}

// SetDrainTimeout 设置 drain 阶段等待进行中请求完成的最长时长。
// 该旋钮独立于 per-job 超时（New 的 defaultTimeout）：>0 显式超时；
// 0 使用默认 30s；负数表示不自行限时，仅由 Drain 收到的 ctx 结束等待。
func (m *Manager) SetDrainTimeout(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.drainTimeout = d
}

// Drain 等待所有进行中的请求完成。
// 应在停止接受新请求之后、关闭资源之前调用。
// 使用 Acquire/Release 管理请求生命周期时，Drain 会等待所有已 Acquire 的请求 Release。
func (m *Manager) Drain(ctx context.Context) {
	m.waitForRequests(ctx)
}

// waitForRequests 等待进行中的请求完成。
func (m *Manager) waitForRequests(ctx context.Context) {
	m.mu.Lock()
	drainTimeout := m.drainTimeout
	m.mu.Unlock()

	var deadline time.Time
	switch {
	case drainTimeout > 0:
		deadline = time.Now().Add(drainTimeout)
	case drainTimeout == 0:
		deadline = time.Now().Add(defaultDrainTimeout)
	default: // 负数：不自行限时，仅受 ctx 控制
	}

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		refCount := m.refCount.Load()
		if refCount <= 0 {
			m.logger.Info("all requests completed")
			return
		}

		if !deadline.IsZero() && time.Now().After(deadline) {
			m.logger.Warn("timeout waiting for requests", "pending", refCount)
			return
		}

		select {
		case <-ctx.Done():
			m.logger.Warn("context cancelled, requests still pending", "pending", refCount)
			return
		case <-ticker.C:
			// 继续等待
		}
	}
}

// Errors 返回 Shutdown 过程中收集的错误：teardown 失败（含 panic）与
// closer job 失败（返回 error 或 panic），按发生顺序排列。
// 不会触发 Shutdown；如需触发请显式调用 Shutdown。
// 返回的是副本，调用方修改不影响内部状态。
func (m *Manager) Errors() []error {
	m.mu.Lock()
	defer m.mu.Unlock()
	errs := make([]error, len(m.shutdownErrs))
	copy(errs, m.shutdownErrs)
	return errs
}
