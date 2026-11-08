package config

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
)

// ConfigSource 是配置源接口。
type ConfigSource interface {
	// Read 读取配置内容。
	Read() ([]byte, error)

	// Watch 监听配置变化，变化时调用 onChange。
	// 应在 ctx 取消时停止监听。
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
	// dispatchMu 串行化 Reload 的 Swap+回调投递：并发 Reload（手动触发与
	// Watch 回调同时到）曾可能乱序投递，消费方把旧配置当最新应用。
	// Read+parse 仍在此锁之外（慢读不阻塞别的重载），读取次序由 readSeq
	// 代号裁决——见 Reload。
	dispatchMu sync.Mutex
	// readSeq 给每次 Read 完成的读取发一个单调递增的代号。解析完成后
	// Swap 前比对代号：落后于已应用代号的迟到结果直接丢弃——先发起的
	// 重载若解析慢，会在后发起的重载之后完成 Swap，把旧值盖回去（core#81
	// 实测：旧配置赢了，而源已是新内容）。代号取在 Read 返回时刻而非
	// 发起时刻：谁读到的源内容更新，谁才应该赢。
	readSeq    atomic.Int64
	appliedSeq int64 // dispatchMu 保护：已应用的最大代号
	loggerV    atomic.Pointer[slog.Logger]
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
func (w *ConfigWatcher[T]) Start(ctx context.Context) error {
	// 初始加载
	if err := w.Reload(); err != nil {
		return err
	}

	// 监听变化
	w.source.Watch(ctx, func() {
		if err := w.Reload(); err != nil {
			w.logger().Error("config reload failed", "err", err)
		}
	})

	return nil
}

// OnChange 注册配置变更回调。
func (w *ConfigWatcher[T]) OnChange(fn func(old, new *T)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onChange = fn
}

// Reload 手动触发配置重载。
// Swap 与 onChange 回调整体串行化；读取次序按 Read 完成时刻的代号裁决——
// 代号落后的迟到结果（解析慢的重载）被丢弃，**最新读到的配置**最终胜出，
// 不会出现「先发起的重载后完成 Swap，把新配置盖回旧值」（core#81）。
// 注意 onChange 内不得再调用 Reload（同 goroutine 重入会死锁）。
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
	defer w.dispatchMu.Unlock()

	// 代次校验：解析期间已有更新（代号更大）的重载应用过 ⇒ 本次读到的是
	// 旧内容，Swap 只会把旧值盖到新值之上。丢弃（返回 nil——重载这件事
	// 已经由代号更新的那次完成，迟到方没有可报告的失败）。
	if seq < w.appliedSeq {
		return nil
	}
	w.appliedSeq = seq

	oldCfg := w.current.Swap(newCfg)

	// 触发回调
	w.mu.Lock()
	onChange := w.onChange
	w.mu.Unlock()

	if onChange != nil && oldCfg != nil {
		onChange(oldCfg, newCfg)
	}

	return nil
}
