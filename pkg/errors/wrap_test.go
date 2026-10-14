package errors

import (
	stderrors "errors"
	"testing"
)

var errBase = stderrors.New("base error")

func TestWrap_NilError(t *testing.T) {
	if err := Wrap(nil, "ignored"); err != nil {
		t.Errorf("Wrap(nil, ...) should return nil, got %v", err)
	}
}

func TestWrap_BasicMessage(t *testing.T) {
	err := Wrap(errBase, "operation X")
	want := "operation X: base error"
	if err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
}

func TestWrap_WithKV(t *testing.T) {
	err := Wrap(errBase, "query user", "user_id", 42, "table", "users")
	got := err.Error()
	if got != "query user: base error user_id=42 table=users" {
		t.Errorf("got %q", got)
	}
}

func TestWrap_HandlesOddKV(t *testing.T) {
	// 奇数长度,自动丢弃最后一个
	err := Wrap(errBase, "op", "k1", "v1", "extra")
	if err.Error() != "op: base error k1=v1" {
		t.Errorf("got %q", err.Error())
	}
}

func TestWrap_Unwrap(t *testing.T) {
	err := Wrap(errBase, "operation X")
	if !stderrors.Is(err, errBase) {
		t.Error("errors.Is should match wrapped err to errBase")
	}
}

func TestWrap_NestedUnwrap(t *testing.T) {
	inner := Wrap(errBase, "inner")
	outer := Wrap(inner, "outer")
	if !stderrors.Is(outer, errBase) {
		t.Error("nested wrap should still match errBase via errors.Is")
	}
}

func TestCause_Layers(t *testing.T) {
	inner := Wrap(errBase, "inner")
	outer := Wrap(inner, "outer")
	cause := Cause(outer)
	if cause != errBase {
		t.Errorf("Cause = %v, want errBase", cause)
	}
}

func TestCause_NotWrapped(t *testing.T) {
	if Cause(errBase) != errBase {
		t.Error("Cause on plain error should return itself")
	}
}

func TestCause_Nil(t *testing.T) {
	if Cause(nil) != nil {
		t.Error("Cause(nil) should return nil")
	}
}

func TestFormatKV(t *testing.T) {
	tests := []struct {
		kv  []any
		exp string
	}{
		{[]any{"a", "1", "b", "2"}, "a=1 b=2"},
		{[]any{}, ""},
		{[]any{"x"}, ""}, // 奇数,丢弃
		{nil, ""},
	}
	for _, tt := range tests {
		got := formatKV(tt.kv)
		if got != tt.exp {
			t.Errorf("formatKV(%v) = %q, want %q", tt.kv, got, tt.exp)
		}
	}
}
