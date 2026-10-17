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
	dispatchMu sync.Mutex
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
// Reload 手动触发配置重载。
// Swap 与 onChange 回调整体串行化，保证回调按重载顺序投递；
// 注意 onChange 内不得再调用 Reload（同 goroutine 重入会死锁）。
func (w *ConfigWatcher[T]) Reload() error {
	data, err := w.source.Read()
	if err != nil {
		return err
	}

	newCfg, err := w.parser(data)
	if err != nil {
		return err
	}

	w.dispatchMu.Lock()
	defer w.dispatchMu.Unlock()

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
