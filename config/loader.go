package config

import (
	"errors"
	"fmt"
	"os"
	"regexp"
)

// envPattern matches ${VAR} or ${VAR:default} placeholders.
var envPattern = regexp.MustCompile(`\$\{([^}:]+)(?::([^}]*))?\}`)

// expandEnv replaces ${VAR} and ${VAR:default} placeholders in data with
// environment variable values. If VAR is unset and a default is provided,
// the default value is used. Unset VAR without default preserves the placeholder
// as-is (i.e. the literal "${VAR}" string becomes the config value).
func expandEnv(data []byte) []byte {
	return envPattern.ReplaceAllFunc(data, func(match []byte) []byte {
		groups := envPattern.FindSubmatch(match)
		key := string(groups[1])
		if val, ok := os.LookupEnv(key); ok {
			return []byte(val)
		}
		// groups[2] is nil when ":default" part is absent → preserve placeholder
		if groups[2] == nil {
			return match
		}
		return groups[2]
	})
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
	expanded := expandEnv(data)
	if err := unmarshalConfig(expanded, dst, o); err != nil {
		return &parseErr{err: err}
	}
	if err := applyTagPasses(dst, expanded, o.envNaming); err != nil {
		return &tagErr{err: err}
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
// env 非零值会覆盖文件值。
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
// （见 overlay.go），env 非零值会覆盖文件值。
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
