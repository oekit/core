package config

import (
	"testing"
)

type envCfg struct {
	Host    string `yaml:"host" env:"APP_HOST" default:"localhost"`
	Port    int    `yaml:"port" env:"APP_PORT" default:"8080"`
	Verbose bool   `yaml:"verbose" env:"APP_VERBOSE"`
}

func TestApplyEnvOverlay_EnvWinsOverFileValue(t *testing.T) {
	// 模拟「yaml 显式配置」已填充的目标
	dst := &envCfg{Host: "file-host", Port: 9000}

	t.Setenv("APP_HOST", "env-host")
	t.Setenv("APP_PORT", "7000")
	// APP_VERBOSE 未设置 → 不动 dst

	if err := ApplyEnvOverlay(dst); err != nil {
		t.Fatalf("ApplyEnvOverlay: %v", err)
	}
	if dst.Host != "env-host" {
		t.Fatalf("Host = %q, want env-host（env 应覆盖文件值）", dst.Host)
	}
	if dst.Port != 7000 {
		t.Fatalf("Port = %d, want 7000", dst.Port)
	}
}

func TestApplyEnvOverlay_UnsetEnvKeepsFileValue(t *testing.T) {
	dst := &envCfg{Host: "file-host", Port: 9000}
	t.Setenv("APP_HOST", "") // 显式空串 = 未设置
	t.Setenv("APP_PORT", "7000")

	if err := ApplyEnvOverlay(dst); err != nil {
		t.Fatal(err)
	}
	if dst.Host != "file-host" {
		t.Fatalf("unset env should keep file value, got %q", dst.Host)
	}
	if dst.Port != 7000 {
		t.Fatalf("Port = %d", dst.Port)
	}
}

func TestApplyEnvOverlay_DocumentationContract(t *testing.T) {
	// 钉住 #2 文档契约：ApplyEnv 只填零值（yaml 优先），ApplyEnvOverlay 相反
	file := &envCfg{Host: "file-host"}
	t.Setenv("APP_HOST", "env-host")
	_ = ApplyEnv(file) // yaml 优先语义：字段已非零 → env 不覆盖
	if file.Host != "file-host" {
		t.Fatalf("ApplyEnv contract broken: %q", file.Host)
	}
}
