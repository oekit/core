package stringx

import (
	"strings"
	"testing"
)

func TestFormatTemplate_NoVars(t *testing.T) {
	got := FormatTemplate("no vars here", nil)
	if got != "no vars here" {
		t.Errorf("got %q, want no vars here", got)
	}
}

func TestFormatTemplate_Simple(t *testing.T) {
	got := FormatTemplate("user {user_id} not found", map[string]any{
		"user_id": 42,
	})
	if got != "user 42 not found" {
		t.Errorf("got %q", got)
	}
}

func TestFormatTemplate_Multiple(t *testing.T) {
	got := FormatTemplate("{action} {object} in {tenant}", map[string]any{
		"action": "create",
		"object": "order",
		"tenant": "acme",
	})
	want := "create order in acme"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatTemplate_KeyNotFoundPreserves(t *testing.T) {
	got := FormatTemplate("hello {name}, your id is {id}", map[string]any{
		"name": "alice",
		// "id" 故意缺失
	})
	want := "hello alice, your id is {id}"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatTemplate_UnclosedBraceKeepsLiteral(t *testing.T) {
	// 没有 '}' 闭合,原样 append 剩余
	got := FormatTemplate("path is /a/b/{unclosed", nil)
	if got != "path is /a/b/{unclosed" {
		t.Errorf("got %q", got)
	}
}

func TestFormatTemplate_EmptyValue(t *testing.T) {
	got := FormatTemplate("user {name} role {role}", map[string]any{
		"name": "bob",
		"role": "",
	})
	if got != "user bob role " {
		t.Errorf("got %q", got)
	}
}

func TestFormatTemplate_NoPlaceholders(t *testing.T) {
	got := FormatTemplate("plain text", map[string]any{"x": 1})
	if got != "plain text" {
		t.Errorf("got %q", got)
	}
}

func TestFormatTemplate_AdjacentPlaceholders(t *testing.T) {
	got := FormatTemplate("{a}{b}{c}", map[string]any{
		"a": "1", "b": "2", "c": "3",
	})
	if got != "123" {
		t.Errorf("got %q", got)
	}
}

func TestFormatTemplate_NoKeyMatchInLongString(t *testing.T) {
	longStr := strings.Repeat("x", 1000) + "{key}"
	got := FormatTemplate(longStr, nil)
	if !strings.HasSuffix(got, "{key}") {
		t.Errorf("got should end with {key}")
	}
}
