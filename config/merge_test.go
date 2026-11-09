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

// ---- OverlayMergeExplicit（core#50：显式零值合并逃生门） ----

// TestOverlayMergeExplicit_ExplicitPathsOverrideEvenZero 钉住 core#50：
// 显式集合命中的字段无条件取 src 值（含零值），未命中路径保持
// 「非零值才覆盖」的历史语义。
func TestOverlayMergeExplicit_ExplicitPathsOverrideEvenZero(t *testing.T) {
	dst := &TestConfig{Name: "keep", Port: 8080, Enabled: true}
	src := &TestConfig{} // 全零 overlay
	OverlayMergeExplicit(dst, src, FieldSet{"Port": true})

	if dst.Port != 0 {
		t.Errorf("explicit path must override with zero, got %d", dst.Port)
	}
	if dst.Name != "keep" || !dst.Enabled {
		t.Errorf("non-explicit paths must keep nonzero-only semantics, got %+v", *dst)
	}
}

// TestOverlayMergeExplicit_NestedPath 集合支持嵌套路径（与 FieldSet 键同口径）。
func TestOverlayMergeExplicit_NestedPath(t *testing.T) {
	dst := &TestConfig{Nested: SubConfig{Host: "file-host", Port: 9000}}
	src := &TestConfig{Nested: SubConfig{}} // Host 清空、Port 清零
	OverlayMergeExplicit(dst, src, FieldSet{"Nested.Host": true})

	if dst.Nested.Host != "" {
		t.Errorf("explicit nested path must override with zero, got %q", dst.Nested.Host)
	}
	if dst.Nested.Port != 9000 {
		t.Errorf("sibling non-explicit path must keep value, got %d", dst.Nested.Port)
	}
}

// TestOverlayMergeExplicit_BoolOrBypassedOnlyForExplicit：显式命中压过
// bool_or（OR 会把显式 false 错留在 true 上——env 命中即覆盖，core#50）；
// 未命中的 bool_or 字段保持 OR 语义。
func TestOverlayMergeExplicit_BoolOrBypassedOnlyForExplicit(t *testing.T) {
	type cfg struct {
		FeatureOn bool `merge:"bool_or"`
		Fallback  bool `merge:"bool_or"`
	}
	dst := &cfg{FeatureOn: true, Fallback: true}
	src := &cfg{} // 双 false
	OverlayMergeExplicit(dst, src, FieldSet{"FeatureOn": true})

	if dst.FeatureOn {
		t.Error("explicit hit must take src false, bypassing bool_or")
	}
	if !dst.Fallback {
		t.Error("non-explicit bool_or field must keep OR semantics (true)")
	}
}

// TestOverlayMergeExplicit_NilSetMatchesOverlayMerge：集合为 nil 时与
// OverlayMerge 完全等价（OverlayMerge 即这样委托的）。
func TestOverlayMergeExplicit_NilSetMatchesOverlayMerge(t *testing.T) {
	dst := &TestConfig{Name: "keep", Port: 8080}
	src := &TestConfig{Port: 0}
	OverlayMergeExplicit(dst, src, nil)
	if dst.Port != 8080 || dst.Name != "keep" {
		t.Errorf("nil set must behave like OverlayMerge, got %+v", *dst)
	}
}

// TestOverwriteStructDeepCopy 回归 core#84 与 core#90（同族重报）：merge:"overwrite"
// 的结构体分支此前整体 Set——Go 值语义对引用型字段是**浅**拷贝，dst 与 src
// 共享底层 map/slice，改 dst 反向污染 src；deepCopyValue 缺 reflect.Struct
// 分支，指针分支（overwrite 指针解引用）同样共享。两处一起钉住。
func TestOverwriteStructDeepCopy(t *testing.T) {
	type sub struct {
		Tags []string          `merge:"overwrite"`
		M    map[string]string `merge:"overwrite"`
	}
	type outer struct {
		Sub    *sub `merge:"overwrite"`
		SubPtr *sub `merge:"overwrite"`
	}
	src := &outer{
		Sub:    &sub{Tags: []string{"a"}, M: map[string]string{"k": "v"}},
		SubPtr: &sub{Tags: []string{"b"}, M: map[string]string{"j": "w"}},
	}
	dst := &outer{}
	OverlayMerge(dst, src)

	// 改 dst 的值类型字段：不得泄漏到 src
	dst.Sub.Tags[0] = "MUT"
	dst.Sub.M["k"] = "MUT"
	if src.Sub.Tags[0] != "a" || src.Sub.M["k"] != "v" {
		t.Errorf("dst mutation leaked into src.Sub: src.Sub=%+v", src.Sub)
	}
	// 指针分支：overwrite 指针解引用同样不得共享
	dst.SubPtr.Tags[0] = "MUT"
	dst.SubPtr.M["j"] = "MUT"
	if src.SubPtr.Tags[0] != "b" || src.SubPtr.M["j"] != "w" {
		t.Errorf("dst mutation leaked into src.SubPtr: src.SubPtr=%+v", src.SubPtr)
	}
	// 反向亦然：改 src 不得泄漏到 dst
	src.Sub.M["new"] = "from-src"
	if _, ok := dst.Sub.M["new"]; ok {
		t.Error("src mutation leaked into dst.Sub")
	}
	// 覆盖语义不变：dst 拿到的是 src 的值
	if dst.Sub.Tags[0] != "MUT" || dst.Sub.M["k"] != "MUT" {
		t.Errorf("overwrite semantics broken: dst.Sub=%+v", dst.Sub)
	}

	// 数组元素是 struct（内含 map）时整体赋值同样共享——数组分支钉住
	type cell struct {
		M map[string]string
	}
	type withArray struct {
		Cells [2]cell `merge:"overwrite"`
	}
	srcA := &withArray{Cells: [2]cell{{M: map[string]string{"x": "1"}}, {}}}
	dstA := &withArray{}
	OverlayMerge(dstA, srcA)
	dstA.Cells[0].M["x"] = "MUT"
	if srcA.Cells[0].M["x"] != "1" {
		t.Errorf("array-of-struct aliasing: src.Cells=%+v", srcA.Cells)
	}
}

// TestOverwriteDeepCopyPointerCycle 回归 core#102：#84 深拷贝修复把浅 Set
// 换成无环检测的递归拷贝，src 出现指针环（自引用/互引 struct）时无限递归
// → fatal error: stack overflow 崩进程（exit 2），导出函数 OverlayMerge /
// OverlayMergeExplicit 可直接触发。现 deepCopyValue 按指针身份记忆已拷贝
// 节点，环上第二次访问直接返回已拷贝节点；环内结构共享关系保留，且 dst
// 与 src 不得别名。
func TestOverwriteDeepCopyPointerCycle(t *testing.T) {
	type node struct {
		Name string
		Next *node
	}
	type cfg struct {
		Chain *node `merge:"overwrite"` // 指针 overwrite：走 deepCopyValue 指针分支
		Block node  `merge:"overwrite"` // 值 struct overwrite：走 struct 分支
	}

	// 双节点互指成环 + 单节点自环
	a, b := &node{Name: "a"}, &node{Name: "b"}
	a.Next, b.Next = b, a
	self := &node{Name: "self"}
	self.Next = self

	src := &cfg{Chain: a, Block: node{Name: "block", Next: b}}
	dst := &cfg{}
	OverlayMerge(dst, src) // 修复前：stack overflow 崩进程

	if dst.Chain == nil || dst.Chain.Name != "a" {
		t.Fatalf("Chain = %+v, want copy of a", dst.Chain)
	}
	if dst.Chain.Next == nil || dst.Chain.Next.Name != "b" {
		t.Fatalf("Chain.Next = %+v, want copy of b", dst.Chain.Next)
	}
	if dst.Chain.Next.Next != dst.Chain {
		t.Fatal("mutual cycle must be preserved in the copy")
	}
	if dst.Block.Name != "block" {
		t.Fatalf("Block = %+v, want copy of src.Block", dst.Block)
	}
	// Block 与 Chain 是两次独立的 deepCopyValue 调用（各自 visited 集），
	// Block.Next 是环 b 的又一份拷贝：环内闭合、但不与 src 别名。
	if dst.Block.Next == nil || dst.Block.Next.Name != "b" ||
		dst.Block.Next.Next == nil || dst.Block.Next.Next.Name != "a" ||
		dst.Block.Next.Next.Next != dst.Block.Next {
		t.Fatalf("Block.Next = %+v, want a closed copy of the a<->b cycle (b2->a2->b2)", dst.Block.Next)
	}
	if dst.Block.Next == b {
		t.Fatal("Block.Next must not alias src")
	}

	// 不与 src 别名：改 dst 不得泄漏到 src
	dst.Chain.Name = "MUT"
	dst.Chain.Next.Name = "MUT"
	if a.Name != "a" || b.Name != "b" {
		t.Errorf("dst mutation leaked into src cycle: a=%s b=%s", a.Name, b.Name)
	}
	if dst.Chain == a || dst.Chain.Next == b {
		t.Fatal("copy must not alias src pointers")
	}

	// 自环
	src2 := &cfg{Chain: self}
	dst2 := &cfg{}
	OverlayMerge(dst2, src2)
	if dst2.Chain == nil || dst2.Chain.Name != "self" || dst2.Chain.Next != dst2.Chain {
		t.Fatalf("self cycle copy broken: %+v", dst2.Chain)
	}
	if dst2.Chain == self {
		t.Fatal("self cycle copy must not alias src")
	}

	// DAG 共享结构（非环）保留身份：**同一次** deepCopyValue（同一 overwrite
	// struct 内部）两个指针指向同一节点，拷贝后仍同一身份——结构共享关系
	// 原样保留。visited 集按调用隔离：合并层逐字段两次深拷贝各自独立。
	type pair struct {
		P1, P2 *node
	}
	type holder struct {
		Pair pair `merge:"overwrite"`
	}
	shared := &node{Name: "shared"}
	src3 := &holder{Pair: pair{P1: shared, P2: shared}}
	dst3 := &holder{}
	OverlayMerge(dst3, src3)
	if dst3.Pair.P1 == nil || dst3.Pair.P1.Name != "shared" || dst3.Pair.P2 != dst3.Pair.P1 {
		t.Fatal("shared (non-cyclic) pointer identity must be preserved within one deep copy")
	}
	if dst3.Pair.P1 == shared {
		t.Fatal("shared pointer copy must not alias src")
	}
}
