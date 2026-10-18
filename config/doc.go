// Package config 提供 YAML 配置文件加载、struct tag 增强和反射合并。
//
// 所有 enjoydream 后端服务共享此包，统一配置加载方式。
//
// # 核心特性
//
//   - 泛型加载：LoadBootstrap[T] 自动反序列化到目标结构体
//   - Struct tag 增强：default/env/range/options/required 零值填充、环境变量注入与 fail-closed 校验（yaml/env 显式零值不被 default 覆盖）
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
// # Struct Tag 增强（default/env/range/options/required）
//
// LoadBootstrap 在 yaml unmarshal 之后依次执行 ApplyEnv → FillDefaults →
// CheckRequired → ValidateTags；各函数也可单独用于任意结构体指针：
//
//	type Server struct {
//	    Host    string        `default:"localhost"`
//	    Port    int           `default:"8080" env:"APP_PORT" range:"(0,65535]"`
//	    Timeout time.Duration `default:"5s" range:"(0,30s]"`
//	    Mode    string        `default:"dev" options:"dev,prod"`
//	    DSN     string        `env:"APP_DSN" required:"true"` // 必须由 yaml/env 提供，不吃 default
//	}
//
// 优先级（写死）：yaml 显式值 > env tag（env 已设置且字段为零值时才填）>
// default tag。
//
// 零值歧义（#5）：单看字段值，0/""/false/空切片与"未设置"不可区分。LoadBootstrap
// 已自动消解——yaml 文档里显式出现的键（`port: 0` 也算；`port: ~` 不算）与已设置的
// env 变量记为「显式提供」（ExplicitFields），这些字段即便是零值也不再被 env/default
// 覆盖，无需把配置字段改成指针。单独调用 FillDefaults/ApplyEnv 时不带选项仍是
// 「零值即未设置」；要区分请用 ExplicitFromYAML/ExplicitFromEnv 采集后传
// WithExplicit。少量字段用指针类型（*int）也行，整份配置都要区分时别走指针——
// 每个消费方多写解引用、可读性塌掉；用来源标记，或给「必须外部提供」的字段标 required。
//
// env tag 拼错是静默的（编译/vet/test 全过、覆盖不生效）：ValidateTags 只能拦含
// 空白或 '=' 的非法名；拼成另一个合法名字请在启动期用 CheckEnvOrphans(cfg, "APP_")
// 抓「环境里已设置、却没有任何字段消费」的孤儿变量——部署时真正的变量名已在环境里，
// 它没人消费就是拼错的证据。
//
// 支持类型：标量（string/int*/uint*/float*/bool）、time.Duration（time.ParseDuration
// 文法）、[]string（default/env 逗号切分）。range 仅数值字段，文法
// `[a,b]`/`(a,b)`/`[a,b)`/`(a,b]`/单端 `[a,)`/`(,b]`（Duration 字段边界用
// duration 文法如 "(0,30s]"）；options 仅 string 字段，空串也算未命中枚举
// （可选枚举请配 default）；required 与 default 互斥。tag 非法或值越界一律报错
// （fail-closed）并携带字段路径（嵌套 `Server.Port`、切片 `Items[1].Qty`）。
//
// 遍历覆盖嵌套 struct、*struct（nil 按需分配）、[]struct；不遍历 map，
// 跳过非导出字段。
//
// ConfigWatcher 不自动执行这些 pass（其 parser 由调用方提供）；热更场景可在
// parser 里自行调用 ApplyEnv/FillDefaults/CheckRequired/ValidateTags。
//
// # 配置文件约定
//
// 配置文件路径：etc/config-{env}.yaml
// env 由启动参数或环境变量决定，如 dev、prod。
package config
