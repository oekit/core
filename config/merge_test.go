package config

import (
	"testing"
)

type TestConfig struct {
	Name    string    `yaml:"name"`
	Port    int       `yaml:"port"`
	Weight  float64   `yaml:"weight"`
	Enabled bool      `yaml:"enabled"`
	Tags    []string  `yaml:"tags"`
	Nested  SubConfig `yaml:"nested"`
}

type SubConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

func TestOverlayMerge_String(t *testing.T) {
	dst := &TestConfig{Name: "default"}
	src := &TestConfig{Name: "override"}
	OverlayMerge(dst, src)
	if dst.Name != "override" {
		t.Errorf("expected 'override', got %q", dst.Name)
	}
}

func TestOverlayMerge_StringEmpty(t *testing.T) {
	dst := &TestConfig{Name: "keep"}
	src := &TestConfig{Name: ""}
	OverlayMerge(dst, src)
	if dst.Name != "keep" {
		t.Errorf("expected 'keep', got %q", dst.Name)
	}
}

func TestOverlayMerge_Int(t *testing.T) {
	dst := &TestConfig{Port: 8080}
	src := &TestConfig{Port: 9090}
	OverlayMerge(dst, src)
	if dst.Port != 9090 {
		t.Errorf("expected 9090, got %d", dst.Port)
	}
}

func TestOverlayMerge_IntZeroSkipped(t *testing.T) {
	dst := &TestConfig{Port: 8080}
	src := &TestConfig{Port: 0}
	OverlayMerge(dst, src)
	if dst.Port != 8080 {
		t.Errorf("expected 8080 (zero skipped), got %d", dst.Port)
	}
}

func TestOverlayMerge_Float64(t *testing.T) {
	dst := &TestConfig{Weight: 1.0}
	src := &TestConfig{Weight: 2.5}
	OverlayMerge(dst, src)
	if dst.Weight != 2.5 {
		t.Errorf("expected 2.5, got %f", dst.Weight)
	}
}

func TestOverlayMerge_Float64ZeroSkipped(t *testing.T) {
	dst := &TestConfig{Weight: 1.5}
	src := &TestConfig{Weight: 0}
	OverlayMerge(dst, src)
	if dst.Weight != 1.5 {
		t.Errorf("expected 1.5 (zero skipped), got %f", dst.Weight)
	}
}

func TestOverlayMerge_Bool(t *testing.T) {
	dst := &TestConfig{Enabled: false}
	src := &TestConfig{Enabled: true}
	OverlayMerge(dst, src)
	if !dst.Enabled {
		t.Error("expected true")
	}
}

func TestOverlayMerge_Slice(t *testing.T) {
	dst := &TestConfig{Tags: []string{"a"}}
	src := &TestConfig{Tags: []string{"b", "c"}}
	OverlayMerge(dst, src)
	if len(dst.Tags) != 2 || dst.Tags[0] != "b" {
		t.Errorf("expected [b c], got %v", dst.Tags)
	}
}

func TestOverlayMerge_SliceEmpty(t *testing.T) {
	dst := &TestConfig{Tags: []string{"a"}}
	src := &TestConfig{Tags: nil}
	OverlayMerge(dst, src)
	if len(dst.Tags) != 1 {
		t.Errorf("expected [a] (empty slice skipped), got %v", dst.Tags)
	}
}

func TestOverlayMerge_Nested(t *testing.T) {
	dst := &TestConfig{Nested: SubConfig{Host: "localhost", Port: 8080}}
	src := &TestConfig{Nested: SubConfig{Host: "remote"}}
	OverlayMerge(dst, src)
	if dst.Nested.Host != "remote" {
		t.Errorf("expected 'remote', got %q", dst.Nested.Host)
	}
	if dst.Nested.Port != 8080 {
		t.Errorf("expected 8080 (unchanged), got %d", dst.Nested.Port)
	}
}

// PtrConfig 用于测试指针到结构体的递归合并。
type PtrConfig struct {
	Name string     `yaml:"name"`
	Sub  *SubConfig `yaml:"sub"`
}

func TestOverlayMerge_PointerToStruct_Recursive(t *testing.T) {
	dst := &PtrConfig{
		Name: "default",
		Sub:  &SubConfig{Host: "localhost", Port: 8080},
	}
	src := &PtrConfig{
		Sub: &SubConfig{Host: "remote"},
	}
	OverlayMerge(dst, src)
	if dst.Sub.Host != "remote" {
		t.Errorf("expected 'remote', got %q", dst.Sub.Host)
	}
	if dst.Sub.Port != 8080 {
		t.Errorf("expected 8080 (unchanged), got %d", dst.Sub.Port)
	}
}

func TestOverlayMerge_PointerToStruct_NilDst(t *testing.T) {
	dst := &PtrConfig{}
	src := &PtrConfig{
		Sub: &SubConfig{Host: "remote", Port: 9090},
	}
	OverlayMerge(dst, src)
	if dst.Sub == nil {
		t.Fatal("expected Sub to be allocated")
	}
	if dst.Sub.Host != "remote" {
		t.Errorf("expected 'remote', got %q", dst.Sub.Host)
	}
	if dst.Sub.Port != 9090 {
		t.Errorf("expected 9090, got %d", dst.Sub.Port)
	}
}

func TestOverlayMerge_PointerToStruct_NilSrc(t *testing.T) {
	dst := &PtrConfig{
		Sub: &SubConfig{Host: "localhost", Port: 8080},
	}
	src := &PtrConfig{} // Sub is nil
	OverlayMerge(dst, src)
	if dst.Sub == nil || dst.Sub.Host != "localhost" {
		t.Errorf("expected dst.Sub unchanged, got %v", dst.Sub)
	}
}

// ZeroableConfig 用于测试 merge:"zeroable" tag。
type ZeroableConfig struct {
	MaxRetries int     `yaml:"max_retries" merge:"zeroable"`
	Timeout    float64 `yaml:"timeout" merge:"zeroable"`
	Debug      bool    `yaml:"debug" merge:"zeroable"`
	Name       string  `yaml:"name" merge:"zeroable"`
}

func TestOverlayMerge_Zeroable_Int(t *testing.T) {
	dst := &ZeroableConfig{MaxRetries: 5}
	src := &ZeroableConfig{MaxRetries: 0}
	OverlayMerge(dst, src)
	if dst.MaxRetries != 0 {
		t.Errorf("expected 0 (zeroable), got %d", dst.MaxRetries)
	}
}

func TestOverlayMerge_Zeroable_Float(t *testing.T) {
	dst := &ZeroableConfig{Timeout: 3.14}
	src := &ZeroableConfig{Timeout: 0}
	OverlayMerge(dst, src)
	if dst.Timeout != 0 {
		t.Errorf("expected 0 (zeroable), got %f", dst.Timeout)
	}
}

func TestOverlayMerge_Zeroable_Bool(t *testing.T) {
	dst := &ZeroableConfig{Debug: true}
	src := &ZeroableConfig{Debug: false}
	OverlayMerge(dst, src)
	if dst.Debug {
		t.Error("expected false (zeroable)")
	}
}

func TestOverlayMerge_Zeroable_String(t *testing.T) {
	dst := &ZeroableConfig{Name: "old"}
	src := &ZeroableConfig{Name: ""}
	OverlayMerge(dst, src)
	if dst.Name != "" {
		t.Errorf("expected empty (zeroable), got %q", dst.Name)
	}
}

// TestOverlayMerge_NoAliasSharing 钉住 2026-09-06 修复:合并后 dst 不得与
// src 共享底层数据 — slice/map/非 struct 指针全部深拷贝,改 dst 不影响 src。
func TestOverlayMerge_NoAliasSharing(t *testing.T) {
	type cfg struct {
		Tags  []string          `merge:"zeroable"`
		Extra map[string]string `merge:"overwrite"`
		Name  *string           `merge:"zeroable"`
	}
	src := &cfg{
		Tags:  []string{"a", "b"},
		Extra: map[string]string{"k": "v"},
	}
	name := "src-name"
	src.Name = &name
	dst := &cfg{}

	OverlayMerge(dst, src)

	// 改 dst 的 slice 不得污染 src
	dst.Tags[0] = "MUTATED"
	dst.Tags = append(dst.Tags, "extra")
	if src.Tags[0] != "a" || len(src.Tags) != 2 {
		t.Errorf("src.Tags polluted via slice alias: %v", src.Tags)
	}

	// 改 dst 的 map 不得污染 src
	dst.Extra["k"] = "MUTATED"
	dst.Extra["new"] = "x"
	if src.Extra["k"] != "v" || len(src.Extra) != 1 {
		t.Errorf("src.Extra polluted via map alias: %v", src.Extra)
	}

	// 改 dst 的指针 pointee 不得污染 src
	*dst.Name = "MUTATED"
	if *src.Name != "src-name" {
		t.Errorf("src.Name polluted via pointer alias: %s", *src.Name)
	}

	// 合并语义保持:dst 确实拿到了 src 的值
	if dst.Tags[0] != "MUTATED" || dst.Extra["k"] != "MUTATED" || *dst.Name != "MUTATED" {
		t.Error("merge semantics broken: dst should hold copied values")
	}
}

// TestOverlayMerge_ZeroableStructField 回归：zeroable 对 struct / *struct
// 字段曾被静默当成默认合并——运维无法通过远程配置整体清零嵌套配置块。
// 钉住：零值 struct = 显式清空该块；非零值照常递归合并。
func TestOverlayMerge_ZeroableStructField(t *testing.T) {
	type Inner struct {
		Host string
		Port int
	}
	type Outer struct {
		Svc  Inner  `merge:"zeroable"`
		Ptr  *Inner `merge:"zeroable"`
		Keep Inner
	}

	dst := &Outer{Svc: Inner{Host: "a", Port: 1}, Ptr: &Inner{Host: "p", Port: 2}, Keep: Inner{Host: "k", Port: 3}}
	// src 的 Svc 是零值 struct、Ptr 为 nil → 都按"清空"处理；
	// Keep 非零 → 递归合并（零值字段跳过）
	src := &Outer{Keep: Inner{Host: "k2"}}
	OverlayMerge(dst, src)

	if dst.Svc != (Inner{}) {
		t.Errorf("zeroable struct with zero src must be cleared, got %+v", dst.Svc)
	}
	if dst.Ptr != nil {
		t.Errorf("zeroable pointer-to-struct with zero src must be nil-ed, got %+v", dst.Ptr)
	}
	if dst.Keep.Host != "k2" {
		t.Errorf("non-zeroable struct should merge fields, got %+v", dst.Keep)
	}
}

// TestOverlayMerge_NilArgs 回归：nil 入参曾直接 panic（reflect ValueOf(nil).Elem()）。
func TestOverlayMerge_NilArgs(t *testing.T) {
	type S struct{ A int }
	OverlayMerge[S](nil, nil) // 不 panic 即通过
	dst := &S{A: 1}
	OverlayMerge(dst, nil)
	OverlayMerge(nil, dst)
	if dst.A != 1 {
		t.Error("nil src must not modify dst")
	}
}
