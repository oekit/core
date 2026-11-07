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

// TestApplyEnvOverlay_ExplicitZeroEnvWins 钉住 core#50（采纳阻断复盘）：
// 12-factor 场景「显式 env 关布尔」必须可行——KUBOARD_MCP_ENABLED=false
// 压过文件里的 true；APP_PORT=0 能把非零端口清零。此前「非零值才覆盖」的
// OverlayMerge 把显式 false/0 当零值跳过，env 永远压不过文件值（静默失效）。
func TestApplyEnvOverlay_ExplicitZeroEnvWins(t *testing.T) {
	dst := &envCfg{Host: "file-host", Port: 9000, Verbose: true}

	t.Setenv("APP_VERBOSE", "false") // 消费方实录：显式关功能
	t.Setenv("APP_PORT", "0")        // 显式清零

	if err := ApplyEnvOverlay(dst); err != nil {
		t.Fatal(err)
	}
	if dst.Verbose {
		t.Fatalf("explicit env false must win over file true, got Verbose=%v", dst.Verbose)
	}
	if dst.Port != 0 {
		t.Fatalf("explicit env 0 must clear non-zero file value, got %d", dst.Port)
	}
	if dst.Host != "file-host" {
		t.Fatalf("unset env must keep file value, got %q", dst.Host)
	}
}

// TestApplyEnvOverlay_LenientSkipDoesNotZeroDst 钉住 #50×#51 交叠面：
// 宽容档下解析失败的 env 不进命中集合——不会用零值清掉 dst 的文件值。
func TestApplyEnvOverlay_LenientSkipDoesNotZeroDst(t *testing.T) {
	dst := &envCfg{Host: "file-host", Port: 9000}
	t.Setenv("APP_PORT", "abc") // SERVER_PORT=abc 笔误场景

	if err := ApplyEnvOverlay(dst, WithLenientEnv()); err != nil {
		t.Fatalf("lenient mode should skip instead of failing, got %v", err)
	}
	if dst.Port != 9000 {
		t.Fatalf("skipped field must keep file value, got %d", dst.Port)
	}
}
