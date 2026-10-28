# Changelog — oekit/core

由 ekit v0.34.0（git.enjoye.top/enjoydream/ekit，冻结维护）拆分而来。所有 notable 变更记录如下。

## [v0.12.1] — 2026-10-28

- fix(chain): `Timeout` 把下游放到新 goroutine 执行，而 `Recovery` 的 recover 装在**调用方** goroutine 上 ⇒ 下游 panic 根本到不了 Recovery，会打死整个进程（canonical 组合 `chain.Finalize(h, chain.Recovery(), chain.Timeout(d))` 正是这个顺序 ⇒ Recovery 的 godoc 在组合使用时是假的）。修法：在 Timeout 的 goroutine 内 recover 并返回 `*PanicError`；`Recovery` 对下游以 `*PanicError` **形式返回**的情况也走上报钩子
- fix(lifecycle): `Listen` 补 `defer signal.Stop(ch)`——`signal.Notify` 会关掉信号的默认行为（终止进程），不 Stop 的话 Listen 返回后第二次 Ctrl-C / 容器 SIGTERM 不再能终止进程，编排器只能升级到 SIGKILL 才能收尸

## [v0.12.0] — 2026-10-22

### feat(config): env 命名策略——全局前缀 + 逐字段覆盖（[#32](https://github.com/oekit/core/issues/32)）
- 字段的 env 变量名此前只能由结构体作者逐字段写死（`env:"NAME"` tag），
  「约定名」（由 yaml 键推导的 SCREAMING_SNAKE）只被 `LintEnvNaming` /
  `WithEnvNamingLint` 用来**检查**名字是否一致，并不能**决定**实际读的变量名。
  于是两类宿主接不上：用统一前缀的 12-factor 宿主（`PB_` / `APP_` / `SVC_`）
  必须给每个字段手写 tag，且**无法**让没打 tag 的字段也按约定参与 env 读取；
  历史 env 名与 yaml 键对不上、需保留兼容的宿主则只能改结构体 tag——
  结构体不是自己的就改不动。
- 新增 `EnvNaming` 策略（前缀 + 逐字段覆盖）与三个入口：
  - `WithEnvPrefix(prefix string) LoadOption`——全局前缀，加在**本包推导的名字**
    上（env tag 名与路径派生名），覆盖表里的名字是绝对名不受影响；
    `WithEnvPrefix("PB_")` 后 yaml 键 `upload.local_import.enabled` 读
    `PB_UPLOAD_LOCAL_IMPORT_ENABLED`。
  - `WithEnvOverrides(m map[string]string) LoadOption`——逐字段指定 env 变量名，
    键为 **yaml 键路径**（与严格模式诊断路径同口径：嵌套用 `.` 连接、切片元素
    不编号、`yaml:",inline"` 的子字段拍平到父级），值为空串表示**该字段不走
    env**（显式豁免，压过 env tag 与前缀）。
  - `WithEnvNaming(n EnvNaming) PassOption`——给单独调用 `ApplyEnv` /
    `ValidateTags` / `ApplyEnvOverlay` / `ApplyTagPasses` 的场景（12-factor 的
    env 优先接线走 `ApplyEnvOverlay`）。
- 名字解析顺序：覆盖表 → env tag → 路径派生名。**未启用策略时行为与历史逐字
  一致**：只有打了 env tag 的字段读 env。加了前缀后多一层能力——没打 env tag
  的字段也按路径派生名参与 env 读取（「前缀即约定」，宿主不必逐字段写 tag）。
- 命名一致性检查（`WithEnvNamingLint` / `LintEnvNaming`）在新策略下同步调整：
  覆盖表命中的字段豁免（最终名由表决定）；设了前缀时约定名改用**路径级**
  派生名（与运行时实际读的变量名同口径），前缀相同的两条路径派生名撞车
  （`a.b` 与 `a_b` 都推出 `A_B`）会被判为重名。
- env 卫生检查改为校验**最终生效的名字**（覆盖表 / 前缀 / tag 解析之后），
  而非声明名——校验声明名而不校验生效名，等于放行了一个永远 miss 的变量名；
  非法前缀与非法覆盖名在 `ValidateTags` 阶段 fail-closed（否则就是「配了没
  生效」的静默失效）。重名判定按 yaml 键路径去重：同一逻辑字段的多个实例
  （切片元素）不再被误判为重名。
- `require` 字段缺失时的错误提示改为指向**真正生效**的变量名（加前缀后不再
  指向那个不生效的旧名）。
- `ApplyEnvOverlay` / `ApplyTagPasses` / `LintEnvNaming` 新增可选策略参数
  （变参形式，既有调用方零改动）。

## [v0.11.0] — 2026-10-21

### feat(config): 加载入口按来源拆开——`LoadFile`（任意路径）/ `Unmarshal`（字节源）（[#25](https://github.com/oekit/core/issues/25)）
- v0.10.0 的严格模式只有**一个**入口 `LoadBootstrap[T]`，而它的**查找路径写死**
  在 `etc/config.yaml` 约定上——严格模式、`${VAR}` 展开、tag 接线全封在这条固定
  路径的加载线里。**不遵循 etc/ 约定的宿主吃不上**：CLI `-config` 任意路径、
  容器内镜像内固定挂载位（如 `/app/plugins.yaml`）、配置来自 API 下发 / 模板渲染
  产物 / 测试夹具。
  实锤：argus（v3.159.0）评估后接不上，退回 `yaml.v3` 的 `KnownFields(true)`——
  拒载语义有了，但**嵌套路径、合法键清单、拼写建议这三样最有价值的诊断全丢了**，
  等于 #20 的核心收益对这类宿主不可达。
- 新增两个入口：
  - `LoadFile(path string, dst any, opts ...LoadOption) error`——**任意路径**；
  - `Unmarshal(data []byte, dst any, opts ...LoadOption) error`——**字节源**。
- **三个入口共享同一条解码线**（`unmarshalInto`：展开 `${VAR}` → 可选严格判定 →
  解码 → tag 接线 ApplyEnv → FillDefaults → ValidateTags）。因此严格模式的诊断面
  **与来源无关**——不会因为「不是从 etc/ 约定路径读的」就退化成一条
  “field X not found in type Y”。
- `LoadBootstrap` 行为**逐字不变**：历史错误文案（`parse config file X: ...` /
  `apply struct tags for config file X: ...`）保持原样（内部用 `parseErr`/`tagErr`
  区分两类失败以保住文案），既有调用方零改动。
- 测试：两入口的 `*UnknownKeyError` 诊断（路径/合法键清单/拼写建议）逐项一致；
  任意路径加载；字节级入口同样走 tag 接线（env 优先于 default）；字节级入口同样
  展开 `${VAR}`；非严格模式仍忽略未知键；`LoadBootstrap` 错误文案不变。

## [v0.10.0] — 2026-10-20

### feat(config)：严格模式——未知键拒载 + 合法键清单 + 嵌套路径（[#20](https://github.com/oekit/core/issues/20)）
- `LoadBootstrap[T](envFlag string, opts ...LoadOption)` 新增可选参数（向后兼容）：
  传 `config.WithStrict()` 开启严格模式，**未知键直接拒载**，不再静默忽略。
- 错误信息（`*config.UnknownKeyError`）给出三件事：
  1. **嵌套路径**（YAML 键形式，如 `llm.base_ur`）；
  2. **该层合法键清单**（超 12 个时只列前若干并注明总数——200+ 字段全列是噪音）；
  3. **近似拼写建议**（编辑距离，如「是否想写 "base_url"？」）。
- 为何不用 yaml.v3 自带的 `Decoder.KnownFields(true)`：它只说
  「field X not found in type Y」——不告诉用户这一层有哪些合法键，也不给嵌套路径；
  对 200+ 字段的结构体，用户既不知道自己在哪一层，也不知道正确拼法。
- 合法键判定**与 yaml.v3 字段语义逐条对齐**（yaml tag 优先 / 无 tag 取小写字段名 /
  显式 `,inline` 拍平 / 内嵌未导出类型同样处理 / `yaml:"-"` 跳过）——不一致会把
  合法键误报成未知键。已用测试钉死（`TestStrictMatchesYAMLv3FieldSemantics`）。
- 默认行为不变（未知键仍静默忽略），严格模式是显式开启——它会让「之前一直能跑
  （因为键拼错被忽略）」的配置开始报错，切换前需确认现有配置无误。

## [v0.9.2] — 2026-10-19

### Fixed
- **breaking(检查行为)**：移除 `ValidateTags` 中依赖进程环境的 env「拼写冒烟检查」——会误报 `PATH`/`HOME` 等系统变量（合法配置在 CI/宿主机启动失败，如字段 `Path` + `env:"LOG_PATH"` 撞上系统 `PATH`），且漏掉无人设置的真拼写错误（[#8](https://github.com/oekit/core/issues/8)）

### Added
- `LintEnvNaming(v)`：纯静态命名一致性诊断（声明 env 名 ≠ 由 yaml key/字段名推导的约定名），返回诊断列表、不读环境、不 fail-closed；`WithEnvNamingLint()` 使 ValidateTags 对同一判定 fail-closed；字段 `ignorederived:"true"` 豁免有意自定义名
- `ValidateTags` 保留的 env 检查仅限静态项：变量名格式、同名校验重复

## [v0.9.1] — 2026-10-18

### Fixed
- `FieldSetFromYAML` / `ApplyTagPasses` 支持 `yaml:",inline"` 字段：通用 map 解码会把
  inline 子字段拍平到父级映射，而 inline 字段按字段名小写取 key、在父映射里永远查不到——
  inline 叶子的显式零值（`region: ""` / `shards: 0`）仍被 default 覆盖，恰是 FieldSet
  要消灭的那类静默错（v0.9.0 引入的缺口）。修复：以**同一映射**继续下推 inline 结构体
  （含 nil 的 `*struct` 按类型下推，不写入），叶子路径正常标记；3 例回归测试钉住

## [v0.9.0] — 2026-10-18

### Added
- `config.FieldSet` / `FieldSetFromYAML` / `WithFieldSet` / `ApplyTagPasses`：显式提供集合——YAML key 出现即算「调用方给了值」，`port: 0` 不再被 default/env 覆盖（[#5](https://github.com/oekit/core/issues/5)）。`LoadBootstrap` 默认接线；单独 `FillDefaults`/`ApplyEnv` 不带集合时保持历史行为
- `config` struct tag `require`：字段必须由 yaml/env 显式提供，default 不算（FillDefaults 永不填充 require 字段；ValidateTags 缺失时 fail-closed 并指出应设的 env 名）
- `ValidateTags` env 卫生：变量名格式（`[A-Za-z_][A-Za-z0-9_]*`）、同名校验重复；**拼写冒烟检查**——声明的 env 未设置而推导约定名（`backup_archive_format` → `BACKUP_ARCHIVE_FORMAT`）已设置时报错，拦住「tag 手误 → env 静默失效」

### Docs
- doc.go 补「零值歧义与 FieldSet」「require」「env 卫生」三节；明确全量 `*T` 指针化不是默认解，优先 FieldSet

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