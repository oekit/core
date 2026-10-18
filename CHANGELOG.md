# Changelog — oekit/core

由 ekit v0.34.0（git.enjoye.top/enjoydream/ekit，冻结维护）拆分而来。所有 notable 变更记录如下。

## [v0.8.0] — 2026-10-18

- feat(config): `ApplyEnvOverlay[T](dst *T)`——「env 优先于配置文件」的 12-factor 形态（[#2](https://github.com/oekit/core/issues/2)：ApplyEnv 只填零值、yaml 优先，两者语义相反且此前未文档化）；LoadBootstrap 文档补优先级警告

## [v0.7.0] — 2026-10-17

### Added（go-kratos 对齐）
- `transport` 新包：协议无关传输层上下文抽象——`Transporter`（Kind/Endpoint/Operation/RequestHeader/ReplyHeader）、`Header`、`MapHeader`/`NoopHeader`、Server/Client ctx 存取；仅标准库，中间件/日志/trace 透传可一份实现跨 hertz/kitex 复用
- `chain` 新包：transport 无关中间件链——`Handler`/`Middleware`/`Chain`/`Finalize` + `Recovery`（PanicError + 可选上报钩子）/ `Timeout`；仅标准库，与 nns-endpoint.Endpoint、go-kratos Handler 同形
- `config.OverlaySource`：多 ConfigSource 组合（按序 JSON 深合并，后者覆盖前者；任一子源变化即触发 onChange），配合 `ConfigWatcher[T]` 实现多源热更新

## [v0.6.0] — 2026-10-17

### Added
- `lifecycle.Manager`：优雅关停资源管理器（自 [oekit/logx](https://github.com/oekit/logx) `logx/shutdown`
  迁入，与 `lifecycle.App` 契约同域）。注册 closer/teardown、信号监听、
  Drain 引用计数、per-job 超时、panic 隔离、`Errors()` 观测；`WithLogger`
  注入宿主 `*slog.Logger`。默认日志走 stdlib slog（component=shutdown），
  **lifecycle 包零外部依赖**
  （[oekit#1](https://github.com/oekit/oekit/issues/1)）

### Changed
- **breaking**：`config.ConfigWatcher` 日志接口改为 `*slog.Logger`（原 `logx.Logger`），
  `SetLogger(*slog.Logger)`。core 模块自此**不再依赖 logx**，消除 L0→L1 反向依赖
  （[oekit#1](https://github.com/oekit/oekit/issues/1)）

## [v0.5.0] — 2026-10-16

### Added
- `config` struct tag 增强（additive，不改现有行为；不做 go-zero 全套 mapping 引擎）：
  独立 tag `default:"8080"` / `env:"MYAPP_PORT"` / `range:"(0,100]"` / `options:"dev,prod"`，
  三个导出入口 `FillDefaults` / `ApplyEnv` / `ValidateTags`（共享单个反射 walker）；
  `LoadBootstrap` unmarshal 后自动接线 ApplyEnv → FillDefaults → ValidateTags，
  非结构体配置类型（如 map）跳过以保持旧行为
- 优先级（写死）：yaml 显式值 > env tag（env 有值且字段零值时才填）> default tag；
  零值歧义（yaml 显式 `port: 0` 与未写不可区分、会被 default 覆盖）见 config/doc.go
- 支持类型：标量（string/int*/uint*/float*/bool）、time.Duration（time.ParseDuration
  文法）、[]string（default/env 逗号切分）；range 仅数值字段（Duration 边界用
  duration 文法如 `(0,30s]`；四种区间 + 单端 `[a,)`/`(,b]` + 小数边界），
  options 仅 string 字段；tag 非法/值越界 fail-closed 报错且携带字段路径
  （嵌套 `Server.Port`、切片 `Items[1].Qty`）
- 遍历：嵌套 struct、*struct（nil 按需分配）、[]struct / []*struct；不遍历 map；
  跳过非导出字段。`ConfigWatcher` 不自动执行这些 pass（parser 由调用方提供，
  可在 parser 里自行调用）

## [v0.4.0] — 2026-10-16

### Added
- `config` 子包（自 oekit/registry v0.2.x 迁入，API 不变、仅 import 路径变化）：
  `LoadBootstrap[T]`（YAML 加载 + `${VAR}`/`${VAR:default}` 占位符展开，
  etc/config.yaml → etc/config-{env}.yaml 回退）、`LoadDotEnv`、
  `Get/GetInt/GetBool/...` 环境变量系列、`ConfigWatcher[T]` + `ConfigSource`
  热更新、`OverlayMerge[T]`

### 迁移
- `github.com/oekit/registry/config` → `github.com/oekit/core/config`
  （config 只依赖 yaml+logx；需要 Nacos 配置源的继续用 registry/nacos 的
  `NewConfigSource`，它现在实现 `core/config.ConfigSource`）

## [v0.3.3] — 2026-10-15

- v0.3.3

## [v0.1.0] — 2026-10-15

自 ekit v0.34.0（git.enjoye.top/enjoydream/ekit，冻结维护）拆分而来的初始公开版本，完成拆分稳定化：模块路径统一为 `github.com/oekit/core`，多轮拆分/合并收敛至当前模块边界，组织基建（共享 CI workflow、golangci v2、Dependabot）就位。