// Package config 提供 YAML 配置文件加载和反射合并。
//
// 所有 enjoydream 后端服务共享此包，统一配置加载方式。
//
// # 核心特性
//
//   - 泛型加载：LoadBootstrap[T] 自动反序列化到目标结构体
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
// # 配置文件约定
//
// 配置文件路径：etc/config-{env}.yaml
// env 由启动参数或环境变量决定，如 dev、prod。
package config
