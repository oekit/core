// Package dotenv 环境文件加载（.env 纪律单源）：按候选路径顺序取第一个
// 存在的 .env，逐行解析 KEY=VALUE（# 注释行跳过、值两侧引号剥除），已存在的
// 环境变量不被覆盖（真实环境优先于文件）。收编自 argus internal/config
// LoadDotEnv（v0.32.0）——服务开发期「自动加载 lacus/.env」的通用引导语义。
package config

import (
	"os"
	"strings"
)

// DefaultDotEnvPaths 常见 .env 位置（服务根目录向上逐级：本地 go run 与
// 子目录测试二进制的工作目录差异）。
var DefaultDotEnvPaths = []string{"./.env", "../.env", "../../.env", "../../../.env"}

// LoadDotEnv 依次尝试候选路径，加载第一个存在的 .env 后返回（不叠加多文件）。
// paths 为空时用 DefaultDotEnvPaths。解析失败的行跳过不致命；已设置的环境
// 变量不被文件值覆盖。无任何候选文件存在时静默返回（无 .env 是常态）。
func LoadDotEnv(paths ...string) {
	if len(paths) == 0 {
		paths = DefaultDotEnvPaths
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			k, v, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			k = strings.TrimSpace(k)
			v = strings.Trim(strings.TrimSpace(v), `"'`)
			if os.Getenv(k) == "" {
				_ = os.Setenv(k, v)
			}
		}
		return
	}
}
