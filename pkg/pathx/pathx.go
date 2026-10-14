// Package pathx 提供文件路径匹配工具。
package pathx

import (
	"path/filepath"
	"strings"
)

// MatchesPath 检查路径是否匹配任意一个 glob 模式。
// 支持标准 filepath.Match 模式以及 /** 目录前缀匹配。
//
// 模式示例:
//   - "*.go"          → 匹配根目录下的 .go 文件
//   - "vendor/*"      → 匹配 vendor 目录下的直接子项
//   - "node_modules/**" → 匹配 node_modules 下所有层级的文件
//   - "test/*.txt"    → 匹配 test 目录下的 .txt 文件
//
// 用法:
//
//	pathx.MatchesPath([]string{"vendor/**", "*.test"}, "vendor/foo/bar.go") // true
//	pathx.MatchesPath([]string{"*.go"}, "main.go")                          // true
func MatchesPath(patterns []string, path string) bool {
	path = filepath.ToSlash(strings.TrimSpace(path))
	if path == "" {
		return false
	}

	for _, pattern := range patterns {
		pattern = filepath.ToSlash(strings.TrimSpace(pattern))
		if pattern == "" {
			continue
		}
		// 标准 glob 匹配
		if ok, _ := filepath.Match(pattern, path); ok {
			return true
		}
		// /** 目录前缀匹配: "node_modules/**" 匹配 "node_modules/foo/bar"
		if strings.HasSuffix(pattern, "/**") {
			prefix := strings.TrimSuffix(pattern, "**")
			if strings.HasPrefix(path, prefix) {
				return true
			}
		}
	}
	return false
}
