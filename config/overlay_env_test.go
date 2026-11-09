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

// TestApplyEnvOverlay_UnsetEnvDoesNotZeroTaggedFields 回归 core#101：
// ApplyEnvOverlay 文档承诺「env 未设置的字段不影响 dst（不进命中集合）」，
// 但此前合并走 OverlayMergeExplicit，merge:"overwrite"/"zeroable" 使
// forced || isZeroable(tag) 的中间分支恒真——overlay 里 env 未命中的零值
// 工件（nil map / nil slice / false / 零值 struct / alloc 出的 nil 化点）
// 被无条件写进 dst：凡配置结构体同时用于 OverlayMerge 热更（overwrite
// 语义）与 ApplyEnvOverlay（12-factor），后者每次执行都会把 overwrite/
// zeroable 字段静默清掉。现 ApplyEnvOverlay 走 env 合并变体
// （overlayMergeEnv）：未命中字段零值即跳过，显式命中仍覆盖（core#50 不回退）。
func TestApplyEnvOverlay_UnsetEnvDoesNotZeroTaggedFields(t *testing.T) {
	type block struct {
		Name  string `yaml:"name"`
		Limit int    `yaml:"limit" env:"APP_BLOCK_LIMIT"`
	}
	type cfg struct {
		Region string         `yaml:"region" env:"APP_REGION"`
		Flags  map[string]int `yaml:"flags" merge:"overwrite"`
		Slice  []string       `yaml:"slice" merge:"overwrite"`
		Toggle bool           `yaml:"toggle" env:"APP_TOGGLE" merge:"overwrite"`
		Block  block          `yaml:"block" merge:"zeroable"`
		Ptr    *block         `yaml:"ptr" merge:"zeroable"`
		Over   *block         `yaml:"over" merge:"overwrite"`
	}
	dst := &cfg{
		Region: "cn-east-1",
		Flags:  map[string]int{"a": 1, "b": 2},
		Slice:  []string{"x", "y"},
		Toggle: true,
		Block:  block{Name: "keepme", Limit: 42},
		Ptr:    &block{Name: "ptr"},
		Over:   &block{Name: "over", Limit: 7},
	}
	// issue 复现形态：只有 REGION 有 env 值，其余字段无 env
	t.Setenv("APP_REGION", "cn-north-1")

	if err := ApplyEnvOverlay(dst); err != nil {
		t.Fatalf("ApplyEnvOverlay: %v", err)
	}
	if dst.Region != "cn-north-1" {
		t.Fatalf("Region = %q, want cn-north-1", dst.Region)
	}
	if len(dst.Flags) != 2 || dst.Flags["a"] != 1 || dst.Flags["b"] != 2 {
		t.Fatalf("Flags = %v, want map[a:1 b:2]（overwrite map 不得被 env 未命中的 nil 清空）", dst.Flags)
	}
	if len(dst.Slice) != 2 || dst.Slice[0] != "x" || dst.Slice[1] != "y" {
		t.Fatalf("Slice = %v, want [x y]（overwrite slice 不得被清空）", dst.Slice)
	}
	if !dst.Toggle {
		t.Fatal("Toggle = false, want true（overwrite bool 不得被 env 未命中的 false 翻转）")
	}
	if dst.Block.Name != "keepme" || dst.Block.Limit != 42 {
		t.Fatalf("Block = %+v, want {keepme 42}（zeroable struct 不得被清零）", dst.Block)
	}
	if dst.Ptr == nil || dst.Ptr.Name != "ptr" {
		t.Fatalf("Ptr = %+v, want keep（zeroable 指针不得被 nil 化）", dst.Ptr)
	}
	if dst.Over == nil || dst.Over.Name != "over" || dst.Over.Limit != 7 {
		t.Fatalf("Over = %+v, want keep（overwrite 指针不得被 alloc 的零值点整体替换）", dst.Over)
	}

	// 反向：env 显式命中的 overwrite/zeroable 字段仍覆盖（core#50 语义不回退）
	t.Setenv("APP_TOGGLE", "false")
	t.Setenv("APP_BLOCK_LIMIT", "0")
	dst2 := &cfg{Region: "cn-east-1", Toggle: true, Block: block{Name: "keepme", Limit: 42}}
	if err := ApplyEnvOverlay(dst2); err != nil {
		t.Fatalf("ApplyEnvOverlay(dst2): %v", err)
	}
	if dst2.Toggle {
		t.Fatal("explicit env false must still clear overwrite-tagged bool（core#50 不回退）")
	}
	if dst2.Block.Limit != 0 || dst2.Block.Name != "keepme" {
		t.Fatalf("Block = %+v, want {keepme 0}：env 命中叶子清零、未命中字段保持", dst2.Block)
	}
}
