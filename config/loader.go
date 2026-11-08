package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// envPattern matches ${VAR} or ${VAR:default} placeholders.
var envPattern = regexp.MustCompile(`\$\{([^}:]+)(?::([^}]*))?\}`)

// expandEnv replaces ${VAR} and ${VAR:default} placeholders in data with
// environment variable values. If VAR is unset and a default is provided,
// the default value is used. Unset VAR without default preserves the placeholder
// as-is (i.e. the literal "${VAR}" string becomes the config value).
//
// 替换发生在 YAML 解码**之前**，无法感知占位符所在的标量上下文，因此对会让
// 值**静默损坏**的替换值 fail-closed 报错（不回显值本身——这类值多半是密钥）：
// 换行/回车（双引号标量折叠为空格）、前导 `#`（整个值变注释）、` #`
// （后半截被当注释截断）与反斜杠（core#83：双引号标量里 `\n`/`\t`/`\\` 被
// 重新解释，值被静默改写）。含 `"` 的值在双引号标量里会破坏配对导致解析
// 失败，本就 fail-closed——且该解析错误现经 wrapEnvParseError 点名 env 变量
// （core#83 第二点）。多行证书/含注释符的 token 这类值请经 ApplyEnvOverlay
// （结构体级注入，无文本上下文问题）传入（core#59）。
func expandEnv(data []byte) ([]byte, error) {
	var expandErr error
	out := envPattern.ReplaceAllFunc(data, func(match []byte) []byte {
		groups := envPattern.FindSubmatch(match)
		key := string(groups[1])
		if val, ok := os.LookupEnv(key); ok {
			if snippet, reason := yamlCorruptingSequence(val); snippet != "" {
				if expandErr == nil {
					expandErr = fmt.Errorf("expand ${%s}: env value contains %s — %s; refusing to load a silently corrupted config value, inject it via ApplyEnvOverlay instead", key, snippet, reason)
				}
				return match // 报错即中止加载，占位符保留与否不影响结果
			}
			return []byte(val)
		}
		// groups[2] is nil when ":default" part is absent → preserve placeholder
		if groups[2] == nil {
			return match
		}
		return groups[2]
	})
	return out, expandErr
}

// yamlCorruptingSequence 找出替换值里会被 YAML 静默重释的字符序列：
// 返回 (特征描述, 后果说明)；干净值返回 ("", "")。判定集与 expandEnv 的
// 拦截清单一致——只拦「解析成功但值已损坏」的静默失败面（core#59；
// 反斜杠转义是 core#83 补的残留面）。
func yamlCorruptingSequence(val string) (snippet, reason string) {
	if strings.ContainsAny(val, "\n\r") {
		return "a line break", "YAML folds line breaks to spaces inside quoted scalars"
	}
	if strings.HasPrefix(val, "#") {
		return "a leading '#'", "YAML parses the entire value as a comment leaving the field empty"
	}
	if strings.Contains(val, " #") {
		return "' #'", "YAML treats everything after ' #' as a comment and truncates the value"
	}
	if strings.Contains(val, "\\") {
		// 反斜杠（core#83）：替换是解码前的纯文本替换，落进**双引号标量**
		// 后 YAML 继续解释转义——`\n` 变真换行、`\t` 变制表符、`\\` 变 `\`，
		// 值被静默改写且无报错（Windows 路径 C:\temp、正则、密钥常含 `\`）。
		// 无法感知标量上下文 ⇒ 含反斜杠一律 fail-closed。
		return "a backslash", "YAML re-interprets backslash escapes inside double-quoted scalars (\\n becomes a real line break) silently rewriting the value"
	}
	return "", ""
}

// parseErr / tagErr：区分两类失败，使各入口能给出**与历史一致**的错误文案。
//
// 为什么不用一个 wrap：LoadBootstrap 的历史文案是「parse config file X: ...」与
// 「apply struct tags for config file X: ...」两条——若把二者合成一种，就无法在
// 抽出 LoadFile / Unmarshal 后仍保持既有调用方看到的文案逐字不变。
type parseErr struct{ err error }

func (e *parseErr) Error() string { return e.err.Error() }
func (e *parseErr) Unwrap() error { return e.err }

type tagErr struct{ err error }

func (e *tagErr) Error() string { return e.err.Error() }
func (e *tagErr) Unwrap() error { return e.err }

// unmarshalInto 是**唯一的解码线**：展开 ${VAR} → 可选严格判定 → 解码 → tag 接线。
// Unmarshal / LoadFile / LoadBootstrap 三个入口全部走它——严格模式、tag 语义、
// 错误类型只有一份实现，不会出现「某个入口漏了严格判定」这类不一致。
func unmarshalInto(data []byte, dst any, o loadOptions) error {
	expanded, err := expandEnv(data)
	if err != nil {
		return err
	}
	if err := unmarshalConfig(expanded, dst, o); err != nil {
		if wrapped := wrapEnvParseError(data, expanded, err); wrapped != nil {
			return &parseErr{err: wrapped}
		}
		return &parseErr{err: err}
	}
	if err := applyTagPasses(dst, expanded, o.envNaming); err != nil {
		return &tagErr{err: err}
	}
	return nil
}

// wrapEnvParseError 在「替换前语法合法、替换后解析失败」时把 yaml 错误包装上
// env 变量名（core#83）：裸的 `yaml: found unknown escape character` 指不到是
// 哪个变量注入的坏值，排查现场与配置现场对不上。判定三步：
//  1. 原文与展开结果一致 ⇒ 本次没有发生替换，错误与 env 无关；
//  2. 严格模式的 UnknownKeyError 是键面错误，与替换无关；
//  3. 原文本身解析失败 ⇒ 错误在文件里，env 不是元凶。
//
// 三者皆否 ⇒ 展开破坏了语法，报错点名**被替换的变量**（不回显值——注入值
// 多半是密钥），并提示改走 ApplyEnvOverlay。
func wrapEnvParseError(original, expanded []byte, err error) error {
	if bytes.Equal(original, expanded) {
		return nil
	}
	var uke *UnknownKeyError
	if errors.As(err, &uke) {
		return nil
	}
	// 原文语法探测（非 nil 返回 = 不包装）：原文本身解析合法才可能是 env
	// 注入值破坏语法；原文解析失败说明错误在文件里，env 不是元凶。
	var probe yaml.Node
	if yaml.Unmarshal(original, &probe) == nil {
		var keys []string
		for _, m := range envPattern.FindAllSubmatch(original, -1) {
			name := string(m[1])
			if _, ok := os.LookupEnv(name); ok && !slices.Contains(keys, name) {
				keys = append(keys, fmt.Sprintf("${%s}", name))
			}
		}
		if len(keys) > 0 {
			sort.Strings(keys)
			return fmt.Errorf("yaml became unparseable after expanding env placeholders (%s): %w — an injected env value likely broke its scalar context; inject it via ApplyEnvOverlay instead",
				strings.Join(keys, ", "), err)
		}
	}
	return nil
}

// wrapFileError 给解码错误补上文件路径上下文。
func wrapFileError(path string, err error) error {
	var pe *parseErr
	if errors.As(err, &pe) {
		return fmt.Errorf("parse config file %s: %w", path, pe.err)
	}
	var te *tagErr
	if errors.As(err, &te) {
		return fmt.Errorf("apply struct tags for config file %s: %w", path, te.err)
	}
	return fmt.Errorf("parse config file %s: %w", path, err)
}

// wrapByteError 给解码错误补上字节级上下文（无路径）。
func wrapByteError(err error) error {
	var pe *parseErr
	if errors.As(err, &pe) {
		return fmt.Errorf("parse config: %w", pe.err)
	}
	var te *tagErr
	if errors.As(err, &te) {
		return fmt.Errorf("apply struct tags: %w", te.err)
	}
	return err
}

// Unmarshal 解码 YAML 字节 → 结构体指针：展开 ${VAR} → 可选严格判定 → tag 接线
// （ApplyEnv → FillDefaults → ValidateTags；非结构体类型跳过 tag 接线）。
//
// 这是**与路径无关**的入口：配置来自 API 下发、模板渲染产物、测试夹具时用它。
//
// 严格模式（传 WithStrict()）的错误面与文件路径版**同源**——同一套
// unmarshalConfig、同一套 *UnknownKeyError（嵌套路径 + 该层合法键清单 +
// 近似拼写建议），**不会因为来源不同而丢诊断**。
//
// ⚠️ 该接线是「yaml 优先于 env」。12-factor（env 优先于配置文件）场景请勿直接
// 复用此语义——在 yaml 填充后调用 ApplyEnvOverlay(dst)（见 overlay.go），
// env 命中即覆盖文件值（含显式零值 false/0）。
func Unmarshal(data []byte, dst any, opts ...LoadOption) error {
	if err := unmarshalInto(data, dst, newLoadOptions(opts)); err != nil {
		return wrapByteError(err)
	}
	return nil
}

// LoadFile 从**任意路径**读配置并解码——不带 etc/ 约定查找。
//
// 适用：CLI -config 任意路径、容器内镜像内固定挂载位（如 /app/plugins.yaml）、
// 部署方自定路径等**不遵循 etc/config.yaml 约定**的宿主。
//
// 严格模式（传 WithStrict()）的错误面与 LoadBootstrap 同源。
func LoadFile(path string, dst any, opts ...LoadOption) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config file %s: %w", path, err)
	}
	if err := unmarshalInto(data, dst, newLoadOptions(opts)); err != nil {
		return wrapFileError(path, err)
	}
	return nil
}

// LoadBootstrap reads a local YAML config file, expands ${VAR} and ${VAR:default}
// environment variable placeholders, and unmarshals it into the provided struct type T.
//
// After unmarshal, the struct tag passes run in order ApplyEnv → FillDefaults →
// ValidateTags (precedence: explicit yaml value > env tag > default tag; see
// tags.go for the supported tags). Non-struct config types skip the tag passes.
//
// ⚠️ 该接线是「yaml 优先于 env」。12-factor（env 优先于配置文件）场景请勿直接
// 复用 LoadBootstrap 的语义——在 yaml 填充后调用 ApplyEnvOverlay(dst)
// （见 overlay.go），env 命中即覆盖文件值（含显式零值 false/0）。
//
// Lookup order:
//  1. etc/config.yaml — used in Docker images (renamed from config-{env}.yaml at build time)
//  2. etc/config-{envFlag}.yaml — used in local development (envFlag = "dev", "local", "prod")
//
// 可选传 WithStrict() 开启严格模式：未知键直接拒载，错误信息带嵌套路径与
// 该层合法键清单（见 strict.go）。默认仍是历史行为（未知键静默忽略）。
//
// **不遵循 etc/ 约定的宿主**请用 LoadFile（任意路径）或 Unmarshal（字节源）——
// 本函数只负责 etc/ 约定下的路径查找，解码语义与它们**同源**（unmarshalInto）。
func LoadBootstrap[T any](envFlag string, opts ...LoadOption) (*T, error) {
	filename := "etc/config.yaml"
	data, err := os.ReadFile(filename)
	if err != nil {
		filename = fmt.Sprintf("etc/config-%s.yaml", envFlag)
		data, err = os.ReadFile(filename)
		if err != nil {
			return nil, fmt.Errorf("read config file etc/config.yaml or %s: %w", filename, err)
		}
	}

	var cfg T
	if err := unmarshalInto(data, &cfg, newLoadOptions(opts)); err != nil {
		return nil, wrapFileError(filename, err)
	}
	return &cfg, nil
}
