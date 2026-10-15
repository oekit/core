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
// # Struct Tag 增强（default/env/range/options）
//
// LoadBootstrap 在 yaml unmarshal 之后依次执行 ApplyEnv → FillDefaults →
// ValidateTags；三个函数也可单独用于任意结构体指针：
//
//	type Server struct {
//	    Host    string        `default:"localhost"`
//	    Port    int           `default:"8080" env:"APP_PORT" range:"(0,65535]"`
//	    Timeout time.Duration `default:"5s" range:"(0,30s]"`
//	    Mode    string        `default:"dev" options:"dev,prod"`
//	}
//
// 优先级（写死）：yaml 显式值 > env tag（env 已设置且字段为零值时才填）>
// default tag。注意零值歧义：yaml 显式写 `port: 0` 与未写字段对反射不可区分，
// 0 会被 default 覆盖（设计如此；需要区分请用指针类型）。
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
// parser 里自行调用 ApplyEnv/FillDefaults/ValidateTags。
//
// # 配置文件约定
//
// 配置文件路径：etc/config-{env}.yaml
// env 由启动参数或环境变量决定，如 dev、prod。
package config
