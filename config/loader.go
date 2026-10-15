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
	return &cfg, nil
}
