package config

import (
	"fmt"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
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

// LoadBootstrap reads a local YAML config file, expands ${VAR} and ${VAR:default}
// environment variable placeholders, and unmarshals it into the provided struct type T.
//
// After unmarshal, the struct tag passes run in order ApplyEnv → FillDefaults →
// CheckRequired → ValidateTags (precedence: explicit yaml value > env tag > default
// tag; see tags.go for the supported tags). Non-struct config types skip the tag passes.
//
// 零值歧义（#5）在此已消解：yaml 文档里显式出现的键（`port: 0` 也算；`port: ~`
// 不算）与已设置的 env 变量都记为「显式提供」——这些字段即便是零值也不再被
// env/default 覆盖，无需把配置字段改成指针。`required:"true"` 的字段必须由 yaml
// 或 env 提供，否则 fail-fast。
//
// ⚠️ 该接线是「yaml 优先于 env」。12-factor（env 优先于配置文件）场景请勿直接
// 复用 LoadBootstrap 的语义——在 yaml 填充后调用 ApplyEnvOverlay(dst)
// （见 overlay.go），env 非零值会覆盖文件值。
//
// Lookup order:
//  1. etc/config.yaml — used in Docker images (renamed from config-{env}.yaml at build time)
//  2. etc/config-{envFlag}.yaml — used in local development (envFlag = "dev", "local", "prod")
func LoadBootstrap[T any](envFlag string) (*T, error) {
	filename := "etc/config.yaml"
	data, err := os.ReadFile(filename)
	if err != nil {
		filename = fmt.Sprintf("etc/config-%s.yaml", envFlag)
		data, err = os.ReadFile(filename)
		if err != nil {
			return nil, fmt.Errorf("read config file etc/config.yaml or %s: %w", filename, err)
		}
	}

	data = expandEnv(data)

	var cfg T
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config file %s: %w", filename, err)
	}
	if err := applyTagPasses(&cfg, data); err != nil {
		return nil, fmt.Errorf("apply struct tags for config file %s: %w", filename, err)
	}
	return &cfg, nil
}
