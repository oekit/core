// Package dotenv 环境文件加载（.env 纪律单源）：按候选路径顺序取第一个
// 存在的 .env，逐行解析 KEY=VALUE（# 注释行跳过、值两侧引号剥除），已存在的
// 环境变量不被覆盖（真实环境优先于文件）。收编自 argus internal/config
// LoadDotEnv（v0.32.0）——服务开发期「自动加载 lacus/.env」的通用引导语义。
//
// 「已存在」按 `os.LookupEnv` 的 ok 判定，**不是** `os.Getenv(k) != ""`：
// `LOG_LEVEL=`（运维显式清空）算已设置，文件里的 `LOG_LEVEL=debug` 不得把它
// 反向覆盖——与本包 env.go 的口径一致（空字符串是有效值）。
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
		applyDotEnv(data)
		return
	}
}

// applyDotEnv 逐行应用一份 .env 内容。
func applyDotEnv(data []byte) {
	for _, line := range strings.Split(string(data), "\n") {
		k, v, ok := parseDotEnvLine(line)
		if !ok {
			continue
		}
		// 真实环境优先：**存在即不覆盖**，含「设置但为空」——用 LookupEnv 而不是
		// Getenv，后者把「显式清空」当成未设置，会让文件值反向盖掉运维意图。
		if _, exists := os.LookupEnv(k); exists {
			continue
		}
		_ = os.Setenv(k, v)
	}
}

// parseDotEnvLine 解析一行 KEY=VALUE。
//
// 支持的形态：
//
//	KEY=value          KEY = value          export KEY=value
//	KEY="a b"          KEY='"a"'            KEY='a # not comment'
//	KEY=value # 注释   KEY="a" # 注释       # 整行注释
//
// 不支持的（按解析失败跳过，不致命）：没有 `=` 的行、键里带空白的行。
func parseDotEnvLine(line string) (key, value string, ok bool) {
	line = strings.TrimSpace(line) // 顺带吃掉 CRLF 的 \r
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	k, v, found := strings.Cut(line, "=")
	if !found {
		return "", "", false
	}
	k = strings.TrimSpace(k)
	// `export KEY=VALUE`（shell 风格拷贝）剥掉前缀：不剥的话整行 `export KEY`
	// 会被当成键名写进环境（Go 允许含空格的键名），真实变量永远读不到且零报错。
	if rest := strings.TrimPrefix(k, "export"); rest != k {
		if rest == "" || rest[0] == ' ' || rest[0] == '\t' {
			k = strings.TrimSpace(rest)
		}
	}
	if k == "" || strings.ContainsAny(k, " \t") {
		return "", "", false
	}
	return k, parseDotEnvValue(v), true
}

// parseDotEnvValue 解析值：剥配对引号与未引号值的行内注释，不做转义处理。
func parseDotEnvValue(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if q := raw[0]; q == '"' || q == '\'' {
		// 引号包裹：取到配对引号为止，引号之后的内容忽略（可写行内注释）。
		// 「只剥成对包裹」而不是 strings.Trim(`"'`)——后者会把 `"a" # 注释`
		// 剥成 `a" # 注释`（前引号剥了、后引号没剥，值被截断成垃圾）。
		if end := strings.IndexByte(raw[1:], q); end >= 0 {
			return raw[1 : 1+end]
		}
		// 引号未闭合：按字面返回，不吞内容（非法行不该悄悄变成半个值）。
		return raw
	}
	if raw[0] == '#' {
		// `KEY=#注释`：等号后直接是注释，值为空
		return ""
	}
	// 未加引号：空白之后的 `#` 起是行内注释；`a#b` 这种不带空白的 `#` 属于值本身
	// （口令/令牌里出现 `#` 很常见，按注释切会把值截断成半截）。
	for i := 1; i < len(raw); i++ {
		if raw[i] == '#' && (raw[i-1] == ' ' || raw[i-1] == '\t') {
			return strings.TrimSpace(raw[:i])
		}
	}
	return raw
}
