// Package config 提供 YAML 配置文件加载、struct tag 增强和反射合并。
//
// 所有 enjoydream 后端服务共享此包，统一配置加载方式。
//
// # 核心特性
//
//   - 泛型加载：LoadBootstrap[T] 自动反序列化到目标结构体
//   - Struct tag 增强：default/env/range/options 零值填充、环境变量注入与 fail-closed 校验
//   - 反射合并：OverlayMerge 支持嵌套结构体和指针字段的深度合并
//   - Nacos 集成：配合 nacos.LoadAndMerge 实现远程配置覆盖本地默认值
//
// # 快速接入
//
// 定义配置结构体：
//
//	type AppConfig struct {
//	    Port    int            `yaml:"port"`
//	    Nacos   nacos.Bootstrap `yaml:"nacos"`
//	    DB      struct {
//	        DSN string `yaml:"dsn"`
//	    } `yaml:"db"`
//	}
//
// 加载配置：
//
//	cfg, err := config.LoadBootstrap[AppConfig]("dev")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// # Struct Tag 增强（default/env/range/options/require）
//
// LoadBootstrap 在 yaml unmarshal 之后依次执行 ApplyEnv → FillDefaults →
// ValidateTags（经 ApplyTagPasses，带 FieldSet）；四个函数也可单独用于任意
// 结构体指针：
//
//	type Server struct {
//	    Host    string        `default:"localhost"`
//	    Port    int           `default:"8080" env:"APP_PORT" range:"(0,65535]"`
//	    Timeout time.Duration `default:"5s" range:"(0,30s]"`
//	    Mode    string        `default:"dev" options:"dev,prod"`
//	    Secret  string        `require:"true" env:"APP_SECRET"` // 必须外部提供
//	}
//
// 优先级（写死）：yaml 显式值 > env tag（env 已设置且字段为零值时才填）>
// default tag。
//
// # 零值歧义与 FieldSet
//
// 0/""/false/空切片与"未设置"对反射不可区分。yaml 显式写 `port: 0` 时：
//
//   - LoadBootstrap / ApplyTagPasses：内部构建 FieldSet，显式零值保留，不被
//     default / env 覆盖；
//   - 单独调用 FillDefaults / ApplyEnv（不带 WithFieldSet）：保持历史行为，零值
//     会被 default 覆盖——需要区分请传 FieldSetFromYAML + WithFieldSet，或对
//     少量字段用指针类型。
//
// 整份配置都要区分零值时，全量 *T 改造成本很高：优先用 FieldSet；少量关键
// 字段用指针类型仍可行。不要在未评估成本前把配置结构体整体指针化。
//
// # require
//
// 字段必须由 yaml / env 显式提供，default 不算「提供」（FillDefaults 永不填
// require 字段）。带 FieldSet 时显式零值也算通过；ValidateTags 在缺失时报错
// 并指出应设置的 env 变量名。
//
// # env 卫生与命名检查
//
// ValidateTags 只做**静态**检查（与进程环境无关）：
//
//   - env 变量名须匹配 `[A-Za-z_][A-Za-z0-9_]*`；
//   - 同一次校验内重复的 env 名报错。
//
// 命名一致性是**可选**的：`LintEnvNaming` 返回「声明名 ≠ 推导约定名」
// （`backup_archive_format` → `BACKUP_ARCHIVE_FORMAT`）的诊断列表；需要
// fail-closed 时传 `WithEnvNamingLint()`。有意自定义名（如 `LOG_PATH`）请打
// `ignorederived:"true"` 豁免。不要依赖「环境里恰好有某变量」做拼写判定——
// 会误报 PATH/HOME 等系统变量，且漏掉无人设置的真拼写错误（core#8）。
//
// 支持类型：标量（string/int*/uint*/float*/bool）、time.Duration（time.ParseDuration
// 文法）、[]string（default/env 逗号切分）。range 仅数值字段，文法
// `[a,b]`/`(a,b)`/`[a,b)`/`(a,b]`/单端 `[a,)`/`(,b]`（Duration 字段边界用
// duration 文法如 "(0,30s]"）；options 仅 string 字段，空串也算未命中枚举
// （可选枚举请配 default）。tag 非法或值越界一律报错（fail-closed）并携带
// 字段路径（嵌套 `Server.Port`、切片 `Items[1].Qty`）。
//
// 遍历覆盖嵌套 struct、*struct（nil 按需分配）、[]struct；不遍历 map，
// 跳过非导出字段。
//
// ConfigWatcher 不自动执行这些 pass（其 parser 由调用方提供）；热更场景可在
// parser 里自行调用 ApplyTagPasses / ApplyEnv / FillDefaults / ValidateTags。
//
// # 配置文件约定
//
// 配置文件路径：etc/config-{env}.yaml
// env 由启动参数或环境变量决定，如 dev、prod。
package config
