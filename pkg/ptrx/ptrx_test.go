package ptrx

import (
	"testing"
)

func TestOf(t *testing.T) {
	t.Run("int", func(t *testing.T) {
		p := Of(42)
		if p == nil {
			t.Fatal("Of(42) returned nil")
		}
		if *p != 42 {
			t.Errorf("Of(42) = %v, want 42", *p)
		}
	})

	t.Run("string", func(t *testing.T) {
		p := Of("hello")
		if *p != "hello" {
			t.Errorf("Of(\"hello\") = %v, want hello", *p)
		}
	})

	t.Run("struct", func(t *testing.T) {
		type user struct{ Name string }
		p := Of(user{Name: "alice"})
		if p.Name != "alice" {
			t.Errorf("Of(user{Name: \"alice\"}) = %v, want alice", p.Name)
		}
	})
}

func TestDeref(t *testing.T) {
	t.Run("non-nil pointer", func(t *testing.T) {
		v := 42
		if got := Deref(&v, 0); got != 42 {
			t.Errorf("Deref(&42, 0) = %v, want 42", got)
		}
	})

	t.Run("nil pointer returns fallback", func(t *testing.T) {
		var p *int
		if got := Deref(p, 99); got != 99 {
			t.Errorf("Deref(nil, 99) = %v, want 99", got)
		}
	})

	t.Run("string non-nil", func(t *testing.T) {
		s := "hello"
		if got := Deref(&s, ""); got != "hello" {
			t.Errorf("Deref(&\"hello\", \"\") = %v, want hello", got)
		}
	})

	t.Run("string nil fallback", func(t *testing.T) {
		var p *string
		if got := Deref(p, "default"); got != "default" {
			t.Errorf("Deref(nil, \"default\") = %v, want default", got)
		}
	})

	t.Run("bool nil fallback", func(t *testing.T) {
		var p *bool
		if got := Deref(p, true); got != true {
			t.Errorf("Deref(nil, true) = %v, want true", got)
		}
	})
}
