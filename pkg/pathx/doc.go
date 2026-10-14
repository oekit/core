// Package pathx 提供文件路径匹配工具。
//
// MatchesPath 支持标准 glob 模式和 /** 目录前缀匹配，
// 适用于 .gitignore 风格的路径过滤场景（代码审查忽略路径、构建排除等）。
//
// 用法:
//
//	pathx.MatchesPath([]string{"vendor/**", "*.test"}, "vendor/foo/bar.go") // true
//	pathx.MatchesPath([]string{"*.go"}, "main.go")                          // true
package pathx
