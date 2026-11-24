// Package lifecycle 定义 transport 适配层依赖的应用生命周期契约。
//
// 抽出本接口是为了解耦 transport 适配包（hertz/kitex）与 composition root
// （bootstrap）：transport 适配层只需要注册资源、启动后台任务和读取根
// context，不应反向 import bootstrap——否则任何只想用 hertz 日志桥接的
// 服务都会被连带引入 bootstrap → nacos 等完整依赖树，也阻塞未来的模块拆分。
//
// bootstrap.App 隐式满足本接口，调用方无需任何适配代码。
// 本包为框架无关的底层契约包，不得引入任何 transport 或注册中心依赖。
//
// 本包另含 ReadyProbe 就绪探活机制（自 hertz/kitex bootstrap 的 RunServer
// 探活骨架下沉，oekit/core#208）：RunServer「探活与 server 启动并发 + 探活
// 失败触发优雅关机 + 正常关停打断探活」的单源机制件，hertz/kitex 侧消费方
// 联动 PR 跟进迁移。
package lifecycle

import (
	"context"
	"time"
)

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

// ResourceAdderWithTimeout 是 [App] 的**可选**扩展能力接口：支持为每个资源
// 设置独立的关停预算（per-resource shutdown timeout）。
//
// 不把该方法直接并入 App 是为了保持接口**向后兼容**：既有 App 实现
// （测试替身、第三方适配层）不必一次性补齐新方法；transport 适配层与宿主
// 代码 type-assert 本接口后走有预算路径，未实现则回退
// [App.AddResource]（见 [AddResourceWithTimeout]）。
//
// 签名与 bootstrap.App.AddResourceTimeout 逐字一致（name, timeout, closer，
// 注意 timeout 在中间）——bootstrap.App 无需任何改动即隐式满足本接口；
// lifecycle.Manager 经 [Manager.AddResourceTimeout] 亦满足。
//
// 预算语义由实现定义，但**必须与 bootstrap / lifecycle.Manager 同口径**：
//   - timeout > 0：该 closer 的独立预算（从 Background 派生，不受其它资源
//     耗时挤占，也不被关停阶段总预算封顶）；
//   - timeout <= 0：不单独限时，沿用实现默认（Manager.Add 的「0 用默认
//     超时、默认也为 0 则无限」口径）。
type ResourceAdderWithTimeout interface {
	// AddResourceTimeout 注册需要在优雅关闭时释放的资源，并为该资源设置
	// 独立的关停预算 timeout。name 用于日志标识，closer 是关闭函数。
	AddResourceTimeout(name string, timeout time.Duration, closer func(ctx context.Context) error)
}

// AddResourceWithTimeout 以「尽可能带预算」的方式注册资源：app 实现
// [ResourceAdderWithTimeout] 时走 AddResourceTimeout（timeout 生效），否则
// 回退到 app.AddResource(name, closer)——此时 timeout 被忽略，资源沿用该
// App 的默认关停预算。
//
// 这是 transport 适配层（hertz/kitex 的 RunServer）与宿主代码的统一注册面：
// 既支持 bootstrap.App / lifecycle.Manager 这种有逐资源预算的实现，也兼容
// 只实现最小 App 契约的旧实现，调用方无需各写 type-assert 适配层。
//
// timeout 语义与 [ResourceAdderWithTimeout] 一致；回退路径不保证 timeout
// 生效（无预算能力的 App 只有默认关停预算）。
func AddResourceWithTimeout(app App, name string, timeout time.Duration, closer func(ctx context.Context) error) {
	if timed, ok := app.(ResourceAdderWithTimeout); ok {
		timed.AddResourceTimeout(name, timeout, closer)
		return
	}
	app.AddResource(name, closer)
}
