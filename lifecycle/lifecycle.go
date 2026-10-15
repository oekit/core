// Package lifecycle 定义 transport 适配层依赖的应用生命周期契约。
//
// 抽出本接口是为了解耦 transport 适配包（hertz/kitex）与 composition root
// （bootstrap）：transport 适配层只需要注册资源、启动后台任务和读取根
// context，不应反向 import bootstrap——否则任何只想用 hertz 日志桥接的
// 服务都会被连带引入 bootstrap → nacos 等完整依赖树，也阻塞未来的模块拆分。
//
// bootstrap.App 隐式满足本接口，调用方无需任何适配代码。
// 本包为框架无关的底层契约包，不得引入任何 transport 或注册中心依赖。
package lifecycle

import "context"

// App 是 transport 适配层（如 hertz.RunServer / kitex.RunServer）
// 所需的最小应用生命周期契约。
type App interface {
	// Context 返回应用的根 context，可传递给后台任务；关闭时被取消。
	Context() context.Context

	// AddResource 注册需要在优雅关闭时释放的资源，
	// name 用于日志标识，closer 是关闭函数。
	AddResource(name string, closer func(ctx context.Context) error)

	// GoBackground 启动一个带 panic recovery 的后台 goroutine，
	// 关闭时通过 ctx 取消信号通知退出并等待完成。
	GoBackground(name string, fn func(ctx context.Context))

	// RequestShutdown 请求应用进入关闭流程（幂等，可安全并发调用）。
	// 取消根 context 以解除 userFn 中对 Context().Done() 的阻塞，
	// 进而触发正常优雅关闭序列（资源按注册逆序释放）。
	// 典型用途：后台任务检测到不可恢复错误（如 server 端口占用启动失败）
	// 时主动触发关机，避免进程无服务地"假活"。
	RequestShutdown()
}
