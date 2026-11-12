package config

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"

	"github.com/oekit/core/internal/logkey"
	pkgerrors "github.com/oekit/pkg/errors"
)

// ConfigSource 是配置源接口。
type ConfigSource interface {
	// Read 读取配置内容。
	Read() ([]byte, error)

	// Watch 启动配置监听，变化时调用 onChange。
	//
	// 契约（core#144 钉死，同包 OverlaySource.Watch 依赖同一口径）：
	//   - **非阻塞**：启动监听后即返回，监听在实现自起的 goroutine 内进行——
	//     阻塞式实现会让 ConfigWatcher.Start 在同步调用本方法时永不返回；
	//   - ctx 取消时停止监听，自起的 goroutine 退出；
	//   - onChange 由实现自起的 goroutine 调用（可能滞后于源的实际变化），
	//     实现负责对回调 panic 自行收编（OverlaySource 是参考实现）。
	Watch(ctx context.Context, onChange func())
}

// ConfigWatcher 是配置热更新监听器。
// 支持监听配置文件变化并自动重载。
type ConfigWatcher[T any] struct {
	current  atomic.Pointer[T]
	source   ConfigSource
	parser   func([]byte) (*T, error)
	onChange func(old, new *T)
	mu       sync.Mutex
	// dispatchMu 串行化 Reload 的 Swap+回调**入队**：并发 Reload（手动触发与
	// Watch 回调同时到）曾可能乱序投递，消费方把旧配置当最新应用。
	// Read+parse 仍在此锁之外（慢读不阻塞别的重载），读取次序由 readSeq
	// 代号裁决——见 Reload。**用户回调不在此锁内执行**（core#137：回调
	// 队列化到独立 dispatch goroutine，锁内只做 Swap 与入队两件快事）。
	dispatchMu sync.Mutex
	// readSeq 给每次 Read 完成的读取发一个单调递增的代号。解析完成后
	// Swap 前比对代号：落后于已应用代号的迟到结果直接丢弃——先发起的
	// 重载若解析慢，会在后发起的重载之后完成 Swap，把旧值盖回去（core#81
	// 实测：旧配置赢了，而源已是新内容）。代号取在 Read 返回时刻而非
	// 发起时刻：谁读到的源内容更新，谁才应该赢。
	readSeq    atomic.Int64
	appliedSeq int64 // dispatchMu 保护：已应用的最大代号
	loggerV    atomic.Pointer[slog.Logger]

	// 回调投递队列（core#137）：入队在 dispatchMu 内（入队序 = Swap 序 ⇒
	// 投递保序），执行在单独的 dispatch goroutine（pendingOnce 惰性启动，
	// 与 watcher 同生命周期）——用户回调永不持锁执行：
	//   - onChange 内再调 Reload（同 goroutine 重入）不再死锁——Reload 的
	//     锁都归得回来，回调只是再入队；
	//   - onChange 同步等待另一 goroutine 的 Reload 完成，同样不再死锁；
	//   - 慢回调不再卡住 Watch 通道的后续重载投递（只排队，不阻塞 Swap）。
	pendingMu   sync.Mutex
	pendingCond *sync.Cond
	pending     []pendingChange[T]
	pendingOnce sync.Once
	// dispatchRunning 在 dispatchLoop 启动瞬间置位（懒启动的观测点，测试用）。
	dispatchRunning atomic.Bool
}

// pendingChange 是一次待投递的回调：fn 在**入队时**捕获——OnChange 换了
// 新回调后，旧回调不会收到它注册之后才入队的事件之外的事件（投递时再读
// w.onChange 会让新回调收到注册前的旧事件，语义反而更差）。
type pendingChange[T any] struct {
	fn      func(old, new *T)
	old, nw *T
}

// logger 返回当前 logger(atomic 加载)。
func (w *ConfigWatcher[T]) logger() *slog.Logger { return w.loggerV.Load() }

// NewConfigWatcher 创建配置监听器。
//
// 参数:
//   - source: 配置源（如文件、Nacos——见 nacos.NewConfigSource）
//   - parser: 配置解析函数（如 YAML 解析）
func NewConfigWatcher[T any](source ConfigSource, parser func([]byte) (*T, error)) *ConfigWatcher[T] {
	w := &ConfigWatcher[T]{
		source: source,
		parser: parser,
	}
	w.pendingCond = sync.NewCond(&w.pendingMu)
	w.loggerV.Store(slog.Default().With("component", "config-watcher"))
	return w
}

// SetLogger 设置自定义日志记录器。
// 任意时刻可调:logger 经 atomic.Pointer 保护(2026-09-06 复审修复,
// 原实现无锁写,Start 后 Watch 回调 goroutine 并发读构成数据竞争)。
func (w *ConfigWatcher[T]) SetLogger(l *slog.Logger) {
	if l != nil {
		w.loggerV.Store(l)
	}
}

// Get 获取当前配置。
func (w *ConfigWatcher[T]) Get() *T {
	return w.current.Load()
}

// Start 启动配置监听。
// 会先执行一次初始加载，然后监听配置变化。
//
// 初始加载**不触发 OnChange 回调**（core#138 钉死契约）：回调只在第二次
// 及以后的重载投递（且 old/new 两值齐备——首载没有 old 可交）。要拿到
// 初始配置，在 Start 返回后直接 Get()；组合 config.Snapshot 时请手动
// 补一次首载发布（见 Snapshot 包文档「与 ConfigWatcher 的分工」示例）。
//
// 随后对 ConfigSource.Watch 的调用依赖其**非阻塞**契约（见 ConfigSource
// 接口文档，core#144）：Watch 在自起 goroutine 内监听、立即返回，本方法
// 因此同步返回 nil——不会等待任何监听结束。
func (w *ConfigWatcher[T]) Start(ctx context.Context) error {
	// 初始加载
	if err := w.Reload(); err != nil {
		return err
	}

	// 监听变化
	w.source.Watch(ctx, func() {
		if err := w.Reload(); err != nil {
			w.logger().Error("config reload failed", logkey.Err, err)
		}
	})

	return nil
}

// OnChange 注册配置变更回调。
//
// **首载不回调**（core#138 钉死契约）：Start 的初始加载与 Reload 首次
// 应用（此前 Get 返回 nil）都不会触发回调——回调只在第二次及以后的
// 重载投递，old/new 两值齐备。这是有意行为（watcher_test 有测试钉住）：
// 首载交不出有意义的 old 值，回调里「diff 新旧」的常见写法会被 nil 打穿。
// 要处理初始配置，请在 Start 返回后直接读 Get()。
//
// 回调在**独立的 dispatch goroutine 上串行执行**（core#137 队列化投递）：
// 同一 watcher 的回调按重载应用的次序（Swap 序）逐个投递，永不并发、永不
// 持 watcher 内部锁执行——回调里再调 Reload（含经别的 goroutine 间接等待
// Reload 完成）不会死锁；慢回调不阻塞后续重载的 Swap（Get 立即可见新值，
// 回调随后送达）。回调自身 panic 被隔离为 ERROR 日志，dispatch 循环存续
// （历史行为是 panic 直接打死调用方 goroutine 所在的进程面）。隔离是
// **无条件的**（core#157）：收编出口自身防 panic——宿主 logger（SetLogger
// 注入）在记录这条日志时再 panic 也只降级 stderr 留痕，循环照样存续。
//
// 首次有回调可投递时才启动 dispatch goroutine；它与 watcher 同生命周期
// （watcher 无 Close 语义，与既有行为一致）。
func (w *ConfigWatcher[T]) OnChange(fn func(old, new *T)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onChange = fn
}

// Reload 手动触发配置重载。
// 读取次序按 Read 完成时刻的代号裁决——代号落后的迟到结果（解析慢的重载）
// 被丢弃，**最新读到的配置**最终胜出，不会出现「先发起的重载后完成 Swap，
// 把新配置盖回旧值」（core#81）。
//
// 返回时 Swap 已完成（Get 可见新值）；onChange 回调经队列在独立 dispatch
// goroutine 上随后按序投递（core#137）——回调不再在 Reload 的调用栈上同步
// 执行，也不在 watcher 内部锁内执行。
func (w *ConfigWatcher[T]) Reload() error {
	data, err := w.source.Read()
	if err != nil {
		return err
	}

	// 代号取在 Read 返回时刻：Read 交出的内容反映源在该时刻的状态，
	// 谁的 Read 更晚完成，谁的内容就更新——它才应该在竞态中胜出
	// （按发起时刻发号会把「读得早但解析慢」的一方误判为新）。
	seq := w.readSeq.Add(1)

	newCfg, err := w.parser(data)
	if err != nil {
		return err
	}

	w.dispatchMu.Lock()

	// 代次校验：解析期间已有更新（代号更大）的重载应用过 ⇒ 本次读到的是
	// 旧内容，Swap 只会把旧值盖到新值之上。丢弃（返回 nil——重载这件事
	// 已经由代号更新的那次完成，迟到方没有可报告的失败）。
	if seq < w.appliedSeq {
		w.dispatchMu.Unlock()
		return nil
	}
	w.appliedSeq = seq

	oldCfg := w.current.Swap(newCfg)

	// 回调入队（仍在 dispatchMu 内 ⇒ 入队序 = Swap 序 ⇒ 投递保序），
	// 执行在 dispatch goroutine 上（锁外）——core#137。
	var fn func(old, new *T)
	w.mu.Lock()
	fn = w.onChange
	w.mu.Unlock()
	if fn != nil && oldCfg != nil {
		w.pendingMu.Lock()
		w.pending = append(w.pending, pendingChange[T]{fn: fn, old: oldCfg, nw: newCfg})
		w.pendingMu.Unlock()
		w.pendingCond.Signal()
		w.pendingOnce.Do(func() { go w.dispatchLoop() })
	}

	w.dispatchMu.Unlock()
	return nil
}

// dispatchLoop 是回调投递的专职 goroutine：从队列逐条取出（FIFO），
// 在**不持有任何 watcher 锁**的状态下执行用户回调；回调 panic 就地收编
// 为 ERROR 日志（pkg/errors.Recovered，core#136 同一单源），循环存续——
// 一次回调失控不能永久哑掉后续所有配置变更通知。
func (w *ConfigWatcher[T]) dispatchLoop() {
	w.dispatchRunning.Store(true)
	for {
		w.pendingMu.Lock()
		for len(w.pending) == 0 {
			w.pendingCond.Wait()
		}
		item := w.pending[0]
		w.pending = w.pending[1:]
		w.pendingMu.Unlock()

		w.deliver(item)
	}
}

// logRecovered 在 panic 收编出口内安全留痕（core#157 单源：deliver 与
// OverlaySource.Watch 的收编出口共用这一份防御，「收编出口再 panic 会打死
// goroutine」的论证只留这一份，chain.reportIsolated 同款论证的镜像）。
//
// 为什么必须有兜底：收编出口运行在**已消费过 recover 的 defer 闭包内**——
// 留痕调用（宿主注入的 logger / slog.Default：第三方日志 SDK 内部错误、
// 并发 map 写、序列化故障都可能 panic）在出口内再 panic 时，同一 goroutine
// 上已无任何 recover 可接 ⇒ goroutine 整体 abort（进程级退出）。「回调
// panic 被隔离、循环存续」的承诺隐含了「logger 不 panic」这一未声明前提。
//
// 兜底再降级：logger 挂了说明日志管线本身故障，此时连 slog.Default 都可能
// 是同一个坏 handler——os.Stderr 写入不能 panic（失败仅忽略），是最后的
// 留痕通道。不留痕的兜底等于把故障藏两次，运维现场至少要剩一条事故痕迹。
func logRecovered(fallback string, log func()) {
	defer func() {
		if pkgerrors.Recovered(recover()) != nil {
			_, _ = fmt.Fprintf(os.Stderr, "%s\n", fallback)
		}
	}()
	log()
}

// deliver 执行单条回调，panic 收编为日志。
//
// 收编出口自身防 panic（core#157，logRecovered 单源）：宿主注入的 logger
// 在这条恢复路径上再 panic 只降级 stderr，dispatch goroutine 不死——
// OnChange 的「dispatch 循环存续」承诺不再隐含「logger 不 panic」前提。
func (w *ConfigWatcher[T]) deliver(item pendingChange[T]) {
	defer func() {
		perr := pkgerrors.Recovered(recover())
		if perr == nil {
			return
		}
		stack := ""
		var pe *pkgerrors.PanicError
		if errors.As(perr, &pe) {
			stack = string(pe.Stack)
		}
		logRecovered(
			fmt.Sprintf("config: onChange panicked (dispatch loop survived, logger panicked while reporting): %v\n%s",
				perr, stack),
			func() {
				w.logger().Error("config onChange panicked (isolated by dispatch loop)",
					logkey.Err, perr.Error(), "stack", stack)
			},
		)
	}()
	item.fn(item.old, item.nw)
}
