package config

import (
	"os"
	"strconv"
	"time"
)

// Get 读取环境变量，未设置时返回空字符串。
func Get(key string) string {
	return os.Getenv(key)
}

// GetInt 读取环境变量并解析为整数。
// 返回 (value, true) 表示成功，(0, false) 表示未设置或解析失败。
func GetInt(key string) (int, bool) {
	v, err := strconv.Atoi(os.Getenv(key))
	return v, err == nil
}

// GetBool 读取环境变量并解析为布尔值。
// 返回 (value, true) 表示成功，(false, false) 表示未设置或解析失败。
func GetBool(key string) (bool, bool) {
	v, err := strconv.ParseBool(os.Getenv(key))
	return v, err == nil
}

// GetStringWithDefault 读取环境变量，未设置或为空时返回默认值。
func GetStringWithDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

// GetIntWithDefault 读取环境变量并解析为整数，未设置或解析失败时返回默认值。
func GetIntWithDefault(key string, defaultValue int) int {
	if v, ok := GetInt(key); ok {
		return v
	}
	return defaultValue
}

// MustGet 读取环境变量，未设置时 panic。
// 空字符串被视为有效值（已设置但为空）。
func MustGet(key string) string {
	v, ok := os.LookupEnv(key)
	if !ok {
		panic("env variable " + key + " is required but not set")
	}
	return v
}

// ParseDuration 解析 duration 字符串，失败或为空时返回 fallback。
// 适用于 YAML 配置中以字符串存储的 duration 字段。
//
// 用法:
//
//	d := config.ParseDuration(cfg.WorkerPollInterval, time.Second)
func ParseDuration(value string, fallback time.Duration) time.Duration {
	if value == "" {
		return fallback
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return d
}
