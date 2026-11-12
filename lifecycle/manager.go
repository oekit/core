package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oekit/core/internal/logkey"
	pkgerrors "github.com/oekit/pkg/errors"
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

// New 创建一个关闭管理器。defaultTimeout 是默认的单个资源关闭超时；
// **0 表示关闭任务不设超时**（显式无超时，core#86）——此前 New(0) 会让
// context.WithTimeout(ctx, 0) 立即到期，所有用默认超时的关闭任务拿到
// 已取消的 context，资源实际未关却对外打「shutdown completed」。
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
// timeout 传 0 使用 Manager 的默认超时；默认超时也为 0（New(0)）或 timeout
// 为负时，任务**不设超时**——closer 拿到的 context 不会带 deadline，跑多久
// 都等它返回（core#86：0 曾被直接喂给 context.WithTimeout，立即到期，资源
// 未关却对外报「shutdown completed」）。0/负数 = 显式无超时，与
// SetDrainTimeout 的「负数不限时」口径对齐。
//
// 关停期间注册（core#116）：Shutdown 已开始后经 Add 登记的任务**不会被
// 静默漏调**——Shutdown 按收敛循环执行（每轮锁内取增量、锁外执行，直到
// 取空），执行期间 Add 的任务会被本次关停补齐执行、顺序保持注册序；仅
// 「最后一批任务执行完毕后的收尾瞬间」并发注册的极端时序仍可能漏掉（与
// bootstrap closeAll 同一取舍）——关停开始后不应再注册资源，延迟拿到
// 反注册句柄（nacos 延迟注册）的场景应在已注册 closer 的执行期内完成登记。
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
// 关停期间注册的口径与 Add 相同（core#116）：teardown 执行期间经
// AddTeardown 登记的函数会被本次关停补齐执行，不静默漏调。
func (m *Manager) AddTeardown(fn func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.teardowns = append(m.teardowns, fn)
}

// Listen 监听系统信号，收到信号后执行**优雅关闭**：
// 停止接受新请求 → 排空在飞请求（Drain）→ 执行 teardown/closer。
// 会阻塞直到排空完成、所有资源关闭完成或各自超时。
//
// 为什么必须排空：Shutdown 只「停收新请求 + 跑清理任务」，**不等
// refCount 归零**。用 Acquire/Release 管理请求生命周期的服务若直接
// Shutdown，会在在飞请求还握着资源时把资源关掉——请求报错、连接被重置。
// 此前 Listen 正是直接调 Shutdown，而 Drain 的文档要求它插在「停收新请求
// 之后、关资源之前」，阻塞式的 Listen 却没给调用方任何插入位置 ⇒ 按推荐
// 用法接 Listen 的服务**永远不排空**。
//
// 排空时长由 SetDrainTimeout 控制（默认 30s）。
//
// **第二枪语义覆盖两个阶段（core#142）**：① 排空期间再收到一个信号
// （编排器连发两次 SIGTERM、运维连按两次 Ctrl-C）立即放弃等待并进入资源
// 关闭——第二枪的语义就是「别等了，现在就关」；② **资源关闭执行期**再收到
// 一个信号则强制中止关停：正在执行的 closer 收到 context 取消（能否立即
// 返回取决于 closer 是否尊重 ctx），尚未开始的任务**全部跳过**并逐个记入
// Errors()（"skipped, shutdown aborted by signal"），终态日志打
// "shutdown aborted by signal"（与正常 "shutdown completed" 可区分）。
// Go 无法硬中断运行中的 goroutine——第二枪给的是「取消 + 不再启动后续
// 任务」，彻底卡死的 closer 仍需编排器 SIGKILL 兜底。
func (m *Manager) Listen(signals ...os.Signal) {
	if len(signals) == 0 {
		signals = []os.Signal{os.Interrupt}
	}
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, signals...)
	// **必须 Stop**：signal.Notify 会把对应信号的默认行为（终止进程）关掉。
	// 不 Stop 的话，Listen 返回后这些信号仍被捕获进这个没人读的 channel ⇒
	// **第二次 Ctrl-C / 容器 SIGTERM 不再能终止进程**，编排器只能升级到
	// SIGKILL 才能收尸。
	defer signal.Stop(ch)
	m.listenLoop(ch)
}

// listenLoop 是 Listen 的心跳：等第一个信号 → 停收新请求 → 排空 → 关闭资源。
// 抽成独立方法（而非内联在 Listen 里）是为了让「信号 → 排空 → 关闭」这段
// **顺序契约**能被单元测试直接驱动：测试只需自己往 ch 里塞信号，
// 不必真的 kill 进程（真信号在 register 完成前送达会按默认行为终止进程，
// 那样的测试既不可靠也不安全）。
func (m *Manager) listenLoop(ch <-chan os.Signal) {
	sig := <-ch
	m.logger.Info("received signal", "signal", sig)

	// 顺序不可颠倒：先停收新请求，再排空。accepting 仍为 true 时 refCount
	// 会持续被新请求抬起 ⇒ drain 永远等不到归零（只能在 drainTimeout 上
	// 空等到超时，等于没排空）。
	m.stopAccepting()
	m.drainWithSignal(ch)

	m.shutdownWithSignal(ch)
}

// shutdownWithSignal 在信号驱动路径上执行资源关闭（core#142）：关闭执行期
// 再收到一个信号（第二枪）即强制中止——在跑的 closer 收到 ctx 取消（能否
// 立即返回取决于 closer 是否尊重 ctx），尚未开始的任务全部跳过并记入
// Errors()。公开的 Shutdown 语义不变（ctx 只传播取消、不跳任务）——
// 第二枪中止是 Listen 信号编排的专属行为。
// 并发语义与 Shutdown 一致：经 once 幂等，已有一次关停在跑/完成时本调用
// no-op（此时不启动第二枪观察 goroutine）。
func (m *Manager) shutdownWithSignal(abort <-chan os.Signal) {
	m.once.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// 第二枪观察 goroutine：收到信号 → 取消 ctx（在飞 closer 经
		// context.AfterFunc 同步收到取消）+ 关闭 aborted 旗标（任务间检查
		// 跳过剩余任务）。
		// AGENTS.md:30 豁免说明（core#150 同款口径）：core 引
		// concurrency/async 会把本仓矩阵内依赖从 [pkg] 扩为 [concurrency,
		// pkg]，牵动矩阵文档 wave 与地板联动，单点修复不做该扩展；改用
		// 仓内 panic 收编单源 pkg/errors.Recovered 内联（dispatchLoop/
		// runFunc/runCloser 同款）。体内无用户注入面（channel 读 + cancel），
		// recover 是兜底护栏。
		aborted := make(chan struct{})
		stop := make(chan struct{})
		defer close(stop)
		go func() {
			defer func() {
				if perr := pkgerrors.Recovered(recover()); perr != nil {
					m.logger.Warn("shutdown signal watcher panicked (isolated)", logkey.Err, perr.Error())
				}
			}()
			select {
			case sig := <-abort:
				m.logger.Warn("received signal during shutdown, aborting remaining close tasks", "signal", sig)
				// 先关旗标再取消：aborted 关闭 → cancel → closer 观察到取消
				// → 主循环回到任务间检查——这条 happens-before 链保证任何
				// 「因取消而返回」的时点，跳过旗标已可见（反序则存在主循环
				// 先于旗标关闭到达检查点的窗口，剩余任务会被误执行）。
				close(aborted)
				cancel()
			case <-stop:
			}
		}()

		m.runShutdown(ctx, aborted)
	})
}

// stopAccepting 停止接受新请求（幂等）。
// Acquire 与 Shutdown/Listen 共用此入口，保证「读 accepting」与
// 「refCount 递增」互斥，不会漏算正在进行中的请求。
func (m *Manager) stopAccepting() {
	m.acqMu.Lock()
	m.accepting.Store(false)
	m.acqMu.Unlock()
}

// drainWithSignal 阻塞式排空在飞请求；abort 上收到信号即放弃等待。
func (m *Manager) drainWithSignal(abort <-chan os.Signal) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	drained := make(chan struct{})
	go func() {
		// AGENTS.md:30「禁止裸 go func」的注释豁免（core#150）：本仓矩阵内
		// 依赖只有 pkg，引 concurrency/async.GoSafe 会把依赖面扩为
		// [concurrency, pkg]（牵动矩阵总览表 doc wave 与地板联动），单点
		// 修复不值当——改用仓内 panic 收编单源 pkg/errors.Recovered 内联
		// （watcher.dispatchLoop / runFunc / runCloser 同款）。panic 面是
		// 真实的：Drain → waitForRequests 会调用注入的 logSink（WithLogger
		// 接受宿主实现，可 panic），关停路径被宿主 logger 打崩是最坏时机的
		// 进程 abort。defer 顺序：recover 先注册后执行、close(drained) 最后
		// 执行 ⇒ panic 也会唤醒 select 侧（排空视为已结束，继续关资源）。
		defer close(drained)
		defer func() {
			if perr := pkgerrors.Recovered(recover()); perr != nil {
				m.logger.Warn("drain goroutine panicked (isolated)", logkey.Err, perr.Error())
			}
		}()
		m.Drain(ctx)
	}()

	select {
	case <-drained:
	case sig := <-abort:
		m.logger.Warn("received signal during drain, closing anyway",
			"signal", sig, "pending", m.GetRefCount())
		cancel()
		<-drained // 等 Drain 观察到取消并返回，避免与其日志/状态竞争
	}
}

// Shutdown 执行所有注册的清理任务。
// 按注册顺序依次执行；每个任务拥有独立的超时预算（从 Background 派生），
// 前序任务的耗时不会挤占后续任务的超时。ctx 仅作为取消信号传播到正在
// 执行的任务（外部取消/超时会中止当前任务），不作为任务间共享的总预算；
// 需要全局兜底时请为每个任务设置合理超时（Add 的 timeout 参数）。
// 任务 panic 被隔离为错误记录，不会中断剩余清理。
// 关停执行期间经 Add / AddTeardown 新注册的任务同样会被补齐执行（收敛
// 循环，core#116），不静默漏调——见 Add 的 godoc。
// 多次调用是安全的，只有第一次调用会执行清理。
//
// 注意 ctx 的语义边界（core#142）：ctx 取消**不会**让剩余任务被跳过——
// 它只中止当前正在执行的任务，后续任务照常补齐执行（与「收敛循环不漏调」
// 的承诺一致）；「收到信号即跳过剩余任务」的强制语义只存在于信号驱动的
// Listen 路径（见 Listen 第二枪语义）。
func (m *Manager) Shutdown(ctx context.Context) {
	m.once.Do(func() {
		m.runShutdown(ctx, nil)
	})
}

// runShutdown 是 Shutdown 的实体（once 内执行）：收敛循环取任务、锁外执行
// （core#116），任务间检查 aborted 旗标——Listen 路径的第二枪把 aborted
// 关闭后（core#142），尚未开始的任务不再启动、逐个记为 skipped；nil
// aborted（公开 Shutdown 路径）永不中止。
func (m *Manager) runShutdown(ctx context.Context, aborted <-chan struct{}) {
	// 1. 停止接受新请求（与 Acquire 互斥，确保不会有请求被漏算）
	m.stopAccepting()
	m.logger.Info("stopped accepting new requests")

	// 两条收敛循环的游标提前到函数级：abortShutdown 需要把「未消费的剩余
	// 任务」按同一游标口径记为 skipped（core#142）。
	teardownDone, jobsDone := 0, 0

	// 2. 执行 teardowns —— 收敛循环（core#116）：每轮锁内取「尚未执行的
	// 尾部增量」、锁外执行，直到一轮取空。此前是入口处一次性快照，
	// 快照拷贝之后经 AddTeardown 追加的函数被静默漏调（零日志零错误）。
	for {
		batch := pendingBatch(&m.mu, &m.teardowns, &teardownDone)
		if len(batch) == 0 {
			break
		}
		for idx, fn := range batch {
			if abortRequested(aborted) {
				// 游标回退：本批未执行的（idx 起）交由 abortShutdown 统一
				// 记账为 skipped，不静默消失（core#142）。
				teardownDone -= len(batch) - idx
				m.abortShutdown(&teardownDone, &jobsDone)
				return
			}
			if err := runFunc("teardown", fn); err != nil {
				m.mu.Lock()
				m.shutdownErrs = append(m.shutdownErrs, err)
				m.mu.Unlock()
				m.logger.Error("teardown failed", logkey.Err, err)
			}
		}
	}

	// 3. 执行 closer jobs —— 同款收敛循环：closer 执行期间经 Add 登记
	// 的任务（nacos 延迟注册的反注册句柄）在下一轮被补齐执行，失败
	// 与快照内任务同口径进 shutdownErrs。
	for {
		batch := pendingBatch(&m.mu, &m.jobs, &jobsDone)
		if len(batch) == 0 {
			break
		}
		for idx, j := range batch {
			if abortRequested(aborted) {
				jobsDone -= len(batch) - idx
				m.abortShutdown(&teardownDone, &jobsDone)
				return
			}
			if err := m.runCloser(j, ctx); err != nil {
				m.mu.Lock()
				m.shutdownErrs = append(m.shutdownErrs, fmt.Errorf("%s: %w", j.name, err))
				m.mu.Unlock()
				m.logger.Error("resource close failed", "resource", j.name, logkey.Err, err)
			} else {
				m.logger.Info("resource closed", "resource", j.name)
			}
		}
	}

	m.shutdownDone.Store(true)
	m.logger.Info("shutdown completed")
}

// abortRequested 非阻塞探测第二枪（core#142）。nil 通道 = 公开 Shutdown
// 路径，永不中止。
func abortRequested(aborted <-chan struct{}) bool {
	if aborted == nil {
		return false
	}
	select {
	case <-aborted:
		return true
	default:
		return false
	}
}

// abortShutdown 把被第二枪跳过的剩余任务逐个记账（core#142）：跳过不是
// 静默的——每条进 Errors()（"skipped, shutdown aborted by signal"）并打
// Warn，运维从关停报告能看出哪些资源没关成、需要 SIGKILL 后补。收尾
// shutdownDone 置位（本次关停已收敛：不会再有任务执行，后续 Shutdown
// 调用 no-op），终态日志 "shutdown aborted by signal" 与正常
// "shutdown completed" 可区分。
func (m *Manager) abortShutdown(teardownDone, jobsDone *int) {
	for {
		batch := pendingBatch(&m.mu, &m.teardowns, teardownDone)
		if len(batch) == 0 {
			break
		}
		for range batch {
			err := errors.New("teardown skipped: shutdown aborted by signal")
			m.mu.Lock()
			m.shutdownErrs = append(m.shutdownErrs, err)
			m.mu.Unlock()
			m.logger.Warn("teardown skipped by shutdown abort")
		}
	}
	for {
		batch := pendingBatch(&m.mu, &m.jobs, jobsDone)
		if len(batch) == 0 {
			break
		}
		for _, j := range batch {
			err := fmt.Errorf("%s: skipped, shutdown aborted by signal", j.name)
			m.mu.Lock()
			m.shutdownErrs = append(m.shutdownErrs, err)
			m.mu.Unlock()
			m.logger.Warn("resource close skipped by shutdown abort", "resource", j.name)
		}
	}
	m.shutdownDone.Store(true)
	m.logger.Warn("shutdown aborted by signal")
}

// pendingBatch 取「*src 中尚未消费的尾部增量」快照并推进 done 游标，锁内
// 完成（core#116 收敛循环的原语）。*src 只追加不收缩（Add/AddTeardown 均
// append-only，追加可能重分配底层数组但不动已有元素），done 之前的前缀已
// 全部执行、下标稳定；快照在锁内拷出，执行在锁外进行——与 Add 的 append
// 及消费方回调（可能在 closer 执行期内再 Add）互不持锁，无死锁、无竞争。
// 每轮取空即返回 nil，调用方据此结束循环。
func pendingBatch[T any](mu *sync.Mutex, src *[]T, done *int) []T {
	mu.Lock()
	defer mu.Unlock()
	n := len(*src)
	if *done >= n {
		return nil
	}
	batch := make([]T, n-*done)
	copy(batch, (*src)[*done:])
	*done = n
	return batch
}

// runFunc 执行无参清理函数并隔离 panic：panic 会跳过剩余清理任务，
// 且 sync.Once 视其为已完成，导致后续 Shutdown 静默 no-op。
//
// panic 值收编单源走 pkg/errors.Recovered（core#136）：panic 值本身是
// error 时身份不丢（Unwrap 透传，errors.Is/As 可直达业务侧 panic 出去的
// 原始错误——closer 以 panic(err) 失败是常见写法，此前 %v 拍平后部署
// 门禁/审计无法定位原始错误类）；文本形态从 "panicked: %v" 变为
// "panic: %T: %v"（*PanicError.Error()，panic 值类型信息恒在场）。
func runFunc(name string, fn func()) (err error) {
	defer func() {
		if perr := pkgerrors.Recovered(recover()); perr != nil {
			err = fmt.Errorf("%s: %w", name, perr)
		}
	}()
	fn()
	return nil
}

// runCloser 执行单个 closer 任务：独立超时预算 + 父 context 取消传播 + panic 隔离。
// panic 收编同 runFunc（pkg/errors.Recovered 单源，core#136）。
func (m *Manager) runCloser(j job, parent context.Context) (err error) {
	defer func() {
		if perr := pkgerrors.Recovered(recover()); perr != nil {
			err = fmt.Errorf("%s: %w", j.name, perr)
		}
	}()

	// 独立预算：从 Background 派生，前序任务的耗时不会挤占本任务的超时。
	// timeout <= 0 = 显式无超时（core#86）：WithTimeout(ctx, 0) 会立即到期，
	// 每个 closer 拿到的都是已取消的 context——资源未关却对外报 completed。
	jobCtx, cancel := context.WithCancel(context.Background())
	if j.timeout > 0 {
		jobCtx, cancel = context.WithTimeout(context.Background(), j.timeout)
	}
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
//
// 信号驱动的 Listen 流程**已经自带**这一步（Listen → stopAccepting →
// Drain → Shutdown），无需手工再调；只有自己编排关闭流程（如自己接管
// signal.Notify）时才需要显式调用本方法。
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

// Errors 返回 Shutdown 过程中收集的错误：teardown 失败（含 panic）、
// closer job 失败（返回 error 或 panic）与被 Listen 第二枪跳过的剩余任务
// （"skipped, shutdown aborted by signal"，core#142），按发生顺序排列。
// 不会触发 Shutdown；如需触发请显式调用 Shutdown。
// 返回的是副本，调用方修改不影响内部状态。
func (m *Manager) Errors() []error {
	m.mu.Lock()
	defer m.mu.Unlock()
	errs := make([]error, len(m.shutdownErrs))
	copy(errs, m.shutdownErrs)
	return errs
}
