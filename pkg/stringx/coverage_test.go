package stringx

import (
	"strings"
	"testing"
)

// --- id.go ---

// TestGenerateID_Format verifies the ID format: timestamp-XXXXXXXX.
func TestGenerateID_Format(t *testing.T) {
	id := GenerateID()
	parts := strings.Split(id, "-")
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts separated by '-', got %d in %q", len(parts), id)
	}
	// Timestamp: 14 digits
	if len(parts[0]) != 14 {
		t.Errorf("expected 14-digit timestamp, got %d chars in %q", len(parts[0]), parts[0])
	}
	// Hex suffix: 8 chars (4 bytes)
	if len(parts[1]) != 8 {
		t.Errorf("expected 8-char hex suffix, got %d chars in %q", len(parts[1]), parts[1])
	}
}

// TestContainsAny_CaseInsensitive verifies the case-insensitive match.
func TestContainsAny_CaseInsensitive(t *testing.T) {
	if !ContainsAny("Security Vulnerability", "security", "vuln") {
		t.Error("expected case-insensitive match")
	}
	if ContainsAny("normal text", "critical", "major") {
		t.Error("expected no match for unrelated text")
	}
}

// TestContainsAny_EmptyKeywords covers the no-keywords path.
func TestContainsAny_EmptyKeywords(t *testing.T) {
	if ContainsAny("any text") {
		t.Error("expected no match with no keywords")
	}
}

// TestContainsAny_EmptyText covers the empty-text path.
func TestContainsAny_EmptyText(t *testing.T) {
	if ContainsAny("", "any") {
		t.Error("expected no match for empty text")
	}
}

// TestIndentText_MultipleLines verifies second+ lines are indented.
func TestIndentText_MultipleLines(t *testing.T) {
	got := IndentText("line1\nline2\nline3", "  ")
	want := "line1\n  line2\n  line3"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

// TestIndentText_SingleLine covers the no-newline case.
func TestIndentText_SingleLine(t *testing.T) {
	got := IndentText("only", "  ")
	if got != "only" {
		t.Errorf("expected %q, got %q", "only", got)
	}
}

// TestIndentText_EmptyPrefix covers the empty-prefix case.
func TestIndentText_EmptyPrefix(t *testing.T) {
	got := IndentText("a\nb", "")
	if got != "a\nb" {
		t.Errorf("expected no indent with empty prefix, got %q", got)
	}
}

// --- pluralize.go ---

// TestPluralize_CountOne covers the "count==1" branch.
func TestPluralize_CountOne(t *testing.T) {
	if got := Pluralize(1, "file"); got != "1 file" {
		t.Errorf("expected '1 file', got %q", got)
	}
}

// TestPluralize_Regular covers the default "+s" branch.
func TestPluralize_Regular(t *testing.T) {
	if got := Pluralize(3, "file"); got != "3 files" {
		t.Errorf("expected '3 files', got %q", got)
	}
}

// TestPluralize_ConsonantY covers the "y" → "ies" branch (story).
func TestPluralize_ConsonantY(t *testing.T) {
	if got := Pluralize(5, "story"); got != "5 stories" {
		t.Errorf("expected '5 stories', got %q", got)
	}
}

// TestPluralize_VowelY covers the "y" with vowel prefix — must
// NOT change (toy → toys, not toies).
func TestPluralize_VowelY(t *testing.T) {
	if got := Pluralize(3, "toy"); got != "3 toys" {
		t.Errorf("expected '3 toys', got %q", got)
	}
}

// TestPluralize_VowelYAllFive covers all 5 vowel cases.
func TestPluralize_VowelYAllFive(t *testing.T) {
	for _, word := range []string{"day", "boy", "guy", "key", "way"} {
		got := Pluralize(2, word)
		want := "2 " + word + "s"
		if got != want {
			t.Errorf("for %q: expected %q, got %q", word, want, got)
		}
	}
}

// TestPluralize_SuffixES covers the "s/x/z/sh/ch" → "+es" branch.
func TestPluralize_SuffixES(t *testing.T) {
	tests := []struct {
		word string
		want string
	}{
		{"box", "5 boxes"},
		{"buzz", "5 buzzes"},
		{"dish", "5 dishes"},
		{"match", "5 matches"},
		{"bus", "5 buses"},
	}
	for _, tt := range tests {
		if got := Pluralize(5, tt.word); got != tt.want {
			t.Errorf("for %q: expected %q, got %q", tt.word, tt.want, got)
		}
	}
}

// TestPluralize_EmptyWord covers the empty-word edge case.
func TestPluralize_EmptyWord(t *testing.T) {
	if got := Pluralize(0, ""); got != "0" {
		t.Errorf("expected '0' for empty word, got %q", got)
	}
}

// TestItoa covers the integer-to-string helper.
func TestItoa(t *testing.T) {
	tests := []struct {
		in   int
		want string
	}{
		{0, "0"},
		{1, "1"},
		{42, "42"},
		{123456789, "123456789"},
	}
	for _, tt := range tests {
		if got := itoa(tt.in); got != tt.want {
			t.Errorf("itoa(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// --- string.go: TakeWithPriority ---

// TestTakeWithPriority_FirstNonEmpty covers the basic happy path.
func TestTakeWithPriority_FirstNonEmpty(t *testing.T) {
	got := TakeWithPriority(
		func() string { return "" },
		func() string { return "first" },
		func() string { return "second" },
	)
	if got != "first" {
		t.Errorf("expected 'first', got %q", got)
	}
}

// TestTakeWithPriority_AllEmpty covers the all-empty path.
func TestTakeWithPriority_AllEmpty(t *testing.T) {
	got := TakeWithPriority(
		func() string { return "" },
		func() string { return "" },
	)
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

// TestTakeWithPriority_NoFuncs covers the zero-args path.
func TestTakeWithPriority_NoFuncs(t *testing.T) {
	if got := TakeWithPriority(); got != "" {
		t.Errorf("expected empty for no funcs, got %q", got)
	}
}

// TestToCamelCase_Extended covers the first-letter-lowercase
// transformation with edge cases (existing string_test.go covers
// the basic "Hello" → "hello" case).
func TestToCamelCase_Extended(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"Hello", "hello"},
		{"hello", "hello"},
		{"A", "a"},
		{"", ""},
		{"ABC", "aBC"}, // only first letter is lowered
	}
	for _, tt := range tests {
		if got := ToCamelCase(tt.in); got != tt.want {
			t.Errorf("ToCamelCase(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
