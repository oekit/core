package version

import (
	"time"
)

// 编译时通过 -ldflags 注入，未注入时 Version 为 "dev"（避免把过期版本号误报给注册中心/监控）
var (
	Version     = "dev" // 语义化版本，通过 -ldflags "-X .../version.Version=vX.Y.Z" 注入
	BuildCommit = "unknown"
	BuildTime   = "unknown"
	BuildBranch = "unknown"
)

// StartTime 记录进程启动时间，包加载时自动初始化。
var StartTime = time.Now()

// Metadata 返回构建版本元数据 map，用于服务实例注册等场景。
// 典型用法：nnsc.Register(port, registry.WithMetadata(version.Metadata()))
func Metadata() map[string]string {
	return map[string]string{
		"version":      Version,
		"build_commit": BuildCommit,
		"build_branch": BuildBranch,
		"build_time":   BuildTime,
	}
}
