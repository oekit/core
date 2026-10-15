package config

import (
	"strconv"
	"testing"
	"time"
)

func TestGet(t *testing.T) {
	t.Setenv("CONFIG_TEST_VAR", "hello")
	if got := Get("CONFIG_TEST_VAR"); got != "hello" {
		t.Errorf("expected 'hello', got %q", got)
	}
	if got := Get("CONFIG_TEST_MISSING"); got != "" {
		t.Errorf("expected empty for missing, got %q", got)
	}
}

func TestGetInt_Success(t *testing.T) {
	t.Setenv("CONFIG_TEST_INT", "42")
	v, ok := GetInt("CONFIG_TEST_INT")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if v != 42 {
		t.Errorf("expected 42, got %d", v)
	}
}

func TestGetInt_NotSet(t *testing.T) {
	if v, ok := GetInt("CONFIG_TEST_MISSING_INT"); ok || v != 0 {
		t.Errorf("expected (0, false), got (%d, %v)", v, ok)
	}
}

func TestGetInt_Invalid(t *testing.T) {
	t.Setenv("CONFIG_TEST_INVALID_INT", "not-a-number")
	if v, ok := GetInt("CONFIG_TEST_INVALID_INT"); ok || v != 0 {
		t.Errorf("expected (0, false), got (%d, %v)", v, ok)
	}
}

func TestGetBool_True(t *testing.T) {
	for _, val := range []string{"true", "1", "TRUE", "True"} {
		t.Setenv("CONFIG_TEST_BOOL", val)
		v, ok := GetBool("CONFIG_TEST_BOOL")
		if !ok || !v {
			t.Errorf("input %q: expected (true, true), got (%v, %v)", val, v, ok)
		}
	}
}

func TestGetBool_False(t *testing.T) {
	t.Setenv("CONFIG_TEST_BOOL", "false")
	v, ok := GetBool("CONFIG_TEST_BOOL")
	if !ok || v {
		t.Errorf("expected (false, true), got (%v, %v)", v, ok)
	}
}

func TestGetBool_NotSet(t *testing.T) {
	if v, ok := GetBool("CONFIG_TEST_MISSING_BOOL"); ok || v {
		t.Errorf("expected (false, false), got (%v, %v)", v, ok)
	}
}

func TestGetBool_Invalid(t *testing.T) {
	t.Setenv("CONFIG_TEST_INVALID_BOOL", "not-a-bool")
	if v, ok := GetBool("CONFIG_TEST_INVALID_BOOL"); ok || v {
		t.Errorf("expected (false, false), got (%v, %v)", v, ok)
	}
}

func TestGetStringWithDefault_Set(t *testing.T) {
	t.Setenv("CONFIG_TEST_STR", "value")
	if got := GetStringWithDefault("CONFIG_TEST_STR", "fallback"); got != "value" {
		t.Errorf("expected 'value', got %q", got)
	}
}

func TestGetStringWithDefault_Missing(t *testing.T) {
	if got := GetStringWithDefault("CONFIG_TEST_MISSING_STR", "fallback"); got != "fallback" {
		t.Errorf("expected 'fallback', got %q", got)
	}
}

func TestGetStringWithDefault_Empty(t *testing.T) {
	t.Setenv("CONFIG_TEST_EMPTY_STR", "")
	if got := GetStringWithDefault("CONFIG_TEST_EMPTY_STR", "fallback"); got != "fallback" {
		t.Errorf("expected 'fallback' for empty, got %q", got)
	}
}

func TestGetIntWithDefault_Set(t *testing.T) {
	t.Setenv("CONFIG_TEST_INT_DEF", "100")
	if got := GetIntWithDefault("CONFIG_TEST_INT_DEF", 50); got != 100 {
		t.Errorf("expected 100, got %d", got)
	}
}

func TestGetIntWithDefault_Missing(t *testing.T) {
	if got := GetIntWithDefault("CONFIG_TEST_MISSING_INT_DEF", 50); got != 50 {
		t.Errorf("expected 50, got %d", got)
	}
}

func TestGetIntWithDefault_Invalid(t *testing.T) {
	t.Setenv("CONFIG_TEST_INVALID_INT_DEF", "not-a-number")
	if got := GetIntWithDefault("CONFIG_TEST_INVALID_INT_DEF", 50); got != 50 {
		t.Errorf("expected fallback 50, got %d", got)
	}
}

func TestMustGet_Set(t *testing.T) {
	t.Setenv("CONFIG_TEST_MUST", "secret")
	if got := MustGet("CONFIG_TEST_MUST"); got != "secret" {
		t.Errorf("expected 'secret', got %q", got)
	}
}

func TestMustGet_PanicsOnMissing(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for missing required env")
		}
	}()
	MustGet("CONFIG_TEST_DEFINITELY_MISSING")
}

func TestParseDuration_Valid(t *testing.T) {
	got := ParseDuration("5m", time.Second)
	if got != 5*time.Minute {
		t.Errorf("expected 5m, got %v", got)
	}
}

func TestParseDuration_Empty(t *testing.T) {
	got := ParseDuration("", 3*time.Second)
	if got != 3*time.Second {
		t.Errorf("expected fallback 3s, got %v", got)
	}
}

func TestParseDuration_Invalid(t *testing.T) {
	got := ParseDuration("not-a-duration", 2*time.Second)
	if got != 2*time.Second {
		t.Errorf("expected fallback 2s, got %v", got)
	}
}

// Suppress unused import warning for strconv (used implicitly via the
// package being tested through GetInt etc).
var _ = strconv.Itoa
