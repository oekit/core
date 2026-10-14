// Package version 提供编译时版本信息注入和运行时查询。
//
// 所有 enjoydream 后端服务共享此包，通过 -ldflags 在编译时注入 git 信息。
//
// # 注入方式
//
// 在 build.sh 或 Makefile 中：
//
//	GIT_COMMIT=$(git rev-parse --short HEAD)
//	GIT_BRANCH=$(git rev-parse --abbrev-ref HEAD)
//	BUILD_TIME=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
//	MOD="github.com/yi-nology/ekit-core"
//	go build -ldflags "-X ${MOD}/version.BuildCommit=${GIT_COMMIT} \
//	  -X ${MOD}/version.BuildBranch=${GIT_BRANCH} \
//	  -X ${MOD}/version.BuildTime=${BUILD_TIME}" -o myapp .
//
// # 服务注册元数据
//
// 注册服务时注入构建信息：
//
//	nnsc.Register(port, registry.WithMetadata(version.Metadata()))
//
// # HTTP 端点
//
// 版本信息 HTTP Handler 已拆分到 handler/versionh 包（避免 version 包依赖 Hertz）：
//
//	h.GET("/buildVersion", versionh.Handler)
//
// # 导出变量
//
//   - Version: 语义化版本号
//   - BuildCommit: Git commit hash
//   - BuildBranch: Git 分支名
//   - BuildTime: 编译时间
//   - StartTime: 进程启动时间
package version
