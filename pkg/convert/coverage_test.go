package convert

import (
	"reflect"
	"testing"
)

// --- conv.go ---

// TestStr2Int covers the int parse path and the failure-default
// branch.
func TestStr2Int(t *testing.T) {
	if got := Str2Int("42", 0); got != 42 {
		t.Errorf("expected 42, got %d", got)
	}
	if got := Str2Int("not-a-number", -1); got != -1 {
		t.Errorf("expected default -1 for invalid, got %d", got)
	}
}

// TestStr2Int32 covers the int32 parse path.
func TestStr2Int32(t *testing.T) {
	if got := Str2Int32("100", 0); got != 100 {
		t.Errorf("expected 100, got %d", got)
	}
	if got := Str2Int32("overflow", -1); got != -1 {
		t.Errorf("expected default -1 for invalid, got %d", got)
	}
}

// TestIntToStr covers the int-to-string path.
func TestIntToStr(t *testing.T) {
	if got := IntToStr(42); got != "42" {
		t.Errorf("expected '42', got %q", got)
	}
	if got := IntToStr(-7); got != "-7" {
		t.Errorf("expected '-7', got %q", got)
	}
	if got := IntToStr(0); got != "0" {
		t.Errorf("expected '0', got %q", got)
	}
}

// TestFloat64ToStr covers the float-to-string path.
func TestFloat64ToStr(t *testing.T) {
	// prec=-1 → shortest representation
	if got := Float64ToStr(3.14, -1); got != "3.14" {
		t.Errorf("expected '3.14', got %q", got)
	}
	// prec=2 → 2 decimal places
	if got := Float64ToStr(3.14159, 2); got != "3.14" {
		t.Errorf("expected '3.14' with prec=2, got %q", got)
	}
}

// TestAnyToStr covers the fmt-based fallback path.
func TestAnyToStr(t *testing.T) {
	if got := AnyToStr(42); got != "42" {
		t.Errorf("expected '42', got %q", got)
	}
	if got := AnyToStr("hello"); got != "hello" {
		t.Errorf("expected 'hello', got %q", got)
	}
}

// TestAnyToFloat64 covers the float conversion branches.
func TestAnyToFloat64_Extended(t *testing.T) {
	// Numeric string parses successfully.
	if got := AnyToFloat64("3.14"); got != 3.14 {
		t.Errorf("expected 3.14, got %v", got)
	}
	// Integer string parses as float.
	if got := AnyToFloat64("42"); got != 42.0 {
		t.Errorf("expected 42.0, got %v", got)
	}
	// Non-numeric string returns 0 (the documented fallback).
	if got := AnyToFloat64("not-a-float"); got != 0 {
		t.Errorf("expected 0 for non-numeric string, got %v", got)
	}
	// bool: true=1, false=0.
	if got := AnyToFloat64(true); got != 1 {
		t.Errorf("expected 1 for true, got %v", got)
	}
	if got := AnyToFloat64(false); got != 0 {
		t.Errorf("expected 0 for false, got %v", got)
	}
	// nil returns 0.
	if got := AnyToFloat64(nil); got != 0 {
		t.Errorf("expected 0 for nil, got %v", got)
	}
}

// --- struct.go ---

// TestStructToMap covers the happy path with json tags.
func TestStructToMap(t *testing.T) {
	type S struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	got := StructToMap(S{Name: "alice", Age: 30})
	if got["name"] != "alice" {
		t.Errorf("expected name=alice, got %v", got["name"])
	}
	if got["age"] != 30 {
		t.Errorf("expected age=30, got %v", got["age"])
	}
	if len(got) != 2 {
		t.Errorf("expected 2 entries, got %d", len(got))
	}
}

// TestStructToMap_NoTags covers the fallback to field name when
// no json tag is present.
func TestStructToMap_NoTags(t *testing.T) {
	type S struct {
		Name string
		Age  int
	}
	got := StructToMap(S{Name: "alice", Age: 30})
	if got["Name"] != "alice" {
		t.Errorf("expected Name=alice, got %v", got["Name"])
	}
	if got["Age"] != 30 {
		t.Errorf("expected Age=30, got %v", got["Age"])
	}
}

// TestStructToMap_DashTag covers the json:"-" case: 与 encoding/json 一致，
// 显式排除的字段不得出现在结果里（曾回退字段名导致敏感字段泄漏）。
func TestStructToMap_DashTag(t *testing.T) {
	type S struct {
		Skip string `json:"-"`
		Keep string `json:"keep"`
	}
	got := StructToMap(S{Skip: "x", Keep: "y"})
	if _, exists := got["Skip"]; exists {
		t.Errorf("json:\"-\" 字段应被排除, got %v", got)
	}
	if got["keep"] != "y" {
		t.Errorf("expected keep=y, got %v", got)
	}
}

// TestStructToMap_NotStruct covers the non-struct input: returns
// empty map (no panic).
func TestStructToMap_NotStruct(t *testing.T) {
	got := StructToMap("not a struct")
	if len(got) != 0 {
		t.Errorf("expected empty map for non-struct, got %v", got)
	}
	got2 := StructToMap(nil)
	if len(got2) != 0 {
		t.Errorf("expected empty map for nil, got %v", got2)
	}
}

// TestConvertStruct_MismatchedFields covers the ConvertStruct path
// where src and dst have different field names/types (only matching
// fields are copied).
func TestConvertStruct_MismatchedFields(t *testing.T) {
	type Src struct {
		Name string
		Age  int
		Only int // not in Dst
	}
	type Dst struct {
		Name string
		Age  int
	}
	dst := &Dst{}
	if err := ConvertStruct(Src{Name: "alice", Age: 30, Only: 99}, dst); err != nil {
		t.Fatalf("ConvertStruct: %v", err)
	}
	if dst.Name != "alice" || dst.Age != 30 {
		t.Errorf("expected name=alice, age=30, got %+v", dst)
	}
}

// TestConvertStruct_NilDst covers the nil-dst case (returns nil
// without panic).
func TestConvertStruct_NilDst(t *testing.T) {
	type S struct{ Name string }
	if err := ConvertStruct(S{Name: "x"}, (*S)(nil)); err != nil {
		t.Errorf("expected nil error for nil dst, got %v", err)
	}
}

// --- copy.go (sanity check existing tests) ---

// TestDeepCopy_Reflect is a smoke test to verify the DeepCopy
// function is callable.
func TestDeepCopy_Reflect(t *testing.T) {
	type Inner struct{ X int }
	type Outer struct{ I Inner }
	src := Outer{I: Inner{X: 42}}
	dst := DeepCopy(src).(Outer)
	if dst.I.X != 42 {
		t.Errorf("expected X=42, got %d", dst.I.X)
	}
	if reflect.DeepEqual(src, dst) == false {
		t.Errorf("expected deep equal, got diff")
	}
}
