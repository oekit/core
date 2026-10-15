package config

import (
	"os"
	"testing"
)

func TestExpandEnv_Basic(t *testing.T) {
	t.Setenv("TEST_USER", "alice")
	input := []byte(`user: ${TEST_USER}`)
	got := string(expandEnv(input))
	if got != "user: alice" {
		t.Errorf("expected 'user: alice', got %q", got)
	}
}

func TestExpandEnv_Default(t *testing.T) {
	os.Unsetenv("TEST_MISSING")
	input := []byte(`pass: ${TEST_MISSING:default123}`)
	got := string(expandEnv(input))
	if got != "pass: default123" {
		t.Errorf("expected 'pass: default123', got %q", got)
	}
}

func TestExpandEnv_EnvOverridesDefault(t *testing.T) {
	t.Setenv("TEST_OVERRIDE", "real")
	input := []byte(`val: ${TEST_OVERRIDE:fallback}`)
	got := string(expandEnv(input))
	if got != "val: real" {
		t.Errorf("expected 'val: real', got %q", got)
	}
}

func TestExpandEnv_Multiple(t *testing.T) {
	t.Setenv("A", "1")
	t.Setenv("B", "2")
	input := []byte(`a: ${A}, b: ${B}`)
	got := string(expandEnv(input))
	if got != "a: 1, b: 2" {
		t.Errorf("expected 'a: 1, b: 2', got %q", got)
	}
}

func TestExpandEnv_NoPlaceholder(t *testing.T) {
	input := []byte(`plain: text`)
	got := string(expandEnv(input))
	if got != "plain: text" {
		t.Errorf("expected unchanged, got %q", got)
	}
}

func TestExpandEnv_EmptyDefault(t *testing.T) {
	os.Unsetenv("TEST_EMPTY_DEF")
	input := []byte(`x: ${TEST_EMPTY_DEF:}`)
	got := string(expandEnv(input))
	if got != "x: " {
		t.Errorf("expected 'x: ', got %q", got)
	}
}

func TestExpandEnv_UndefinedNoDefault(t *testing.T) {
	os.Unsetenv("TEST_NODEF")
	input := []byte(`x: ${TEST_NODEF}`)
	got := string(expandEnv(input))
	if got != "x: ${TEST_NODEF}" {
		t.Errorf("expected placeholder preserved, got %q", got)
	}
}
