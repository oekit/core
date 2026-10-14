package stringx

import (
	"testing"
)

func TestContains(t *testing.T) {
	tests := []struct {
		name string
		list []string
		str  string
		want bool
	}{
		{"found", []string{"a", "b", "c"}, "b", true},
		{"not found", []string{"a", "b", "c"}, "d", false},
		{"empty list", []string{}, "a", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Contains(tt.list, tt.str); got != tt.want {
				t.Errorf("Contains() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFilter(t *testing.T) {
	// Filter 过滤掉满足条件的字符
	result := Filter("abc123def456", func(r rune) bool {
		return r >= '0' && r <= '9'
	})
	if result != "abcdef" {
		t.Errorf("Filter() = %v, want %v", result, "abcdef")
	}
}

func TestFirstN(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		n        int
		ellipsis []string
		want     string
	}{
		{"short enough", "hello", 10, nil, "hello"},
		{"with ellipsis", "hello world", 5, []string{"..."}, "hello..."},
		{"exact", "hello", 5, nil, "hello"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FirstN(tt.s, tt.n, tt.ellipsis...); got != tt.want {
				t.Errorf("FirstN() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasEmpty(t *testing.T) {
	if !HasEmpty("a", "", "c") {
		t.Error("HasEmpty() should return true")
	}
	if HasEmpty("a", "b", "c") {
		t.Error("HasEmpty() should return false")
	}
}

func TestNotEmpty(t *testing.T) {
	if !NotEmpty("a", "b", "c") {
		t.Error("NotEmpty() should return true")
	}
	if NotEmpty("a", "", "c") {
		t.Error("NotEmpty() should return false")
	}
}

func TestJoin(t *testing.T) {
	result := Join(',', "a", "b", "c")
	if result != "a,b,c" {
		t.Errorf("Join() = %v, want %v", result, "a,b,c")
	}
}

func TestRemove(t *testing.T) {
	result := Remove([]string{"a", "b", "c", "b"}, "b")
	if len(result) != 2 || result[0] != "a" || result[1] != "c" {
		t.Errorf("Remove() = %v, want [a c]", result)
	}
}

func TestReverse(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want string
	}{
		{"ascii", "hello", "olleh"},
		{"unicode", "你好", "好你"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Reverse(tt.s); got != tt.want {
				t.Errorf("Reverse() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSubstr(t *testing.T) {
	result, err := Substr("hello world", 0, 5)
	if err != nil {
		t.Fatal(err)
	}
	if result != "hello" {
		t.Errorf("Substr() = %v, want %v", result, "hello")
	}
}

func TestTakeOne(t *testing.T) {
	if got := TakeOne("a", "b"); got != "a" {
		t.Errorf("TakeOne() = %v, want a", got)
	}
	if got := TakeOne("", "b"); got != "b" {
		t.Errorf("TakeOne() = %v, want b", got)
	}
}

func TestToCamelCase(t *testing.T) {
	if got := ToCamelCase("Hello"); got != "hello" {
		t.Errorf("ToCamelCase() = %v, want hello", got)
	}
}

func TestUnion(t *testing.T) {
	result := Union([]string{"a", "b"}, []string{"b", "c"})
	if len(result) != 3 {
		t.Errorf("Union() length = %v, want 3", len(result))
	}
}

func TestJoinWith(t *testing.T) {
	result := JoinWith([]string{"a", "b", "c"}, "-")
	if result != "a-b-c" {
		t.Errorf("JoinWith() = %v, want %v", result, "a-b-c")
	}
}

func TestSplitWith(t *testing.T) {
	result := SplitWith("a,b;c", []string{",", ";"})
	if len(result) != 3 {
		t.Errorf("SplitWith() length = %v, want 3", len(result))
	}
}

func TestSplitByComma(t *testing.T) {
	result := SplitByComma("a,b,c")
	if len(result) != 3 {
		t.Errorf("SplitByComma() length = %v, want 3", len(result))
	}
}

func TestJoinByComma(t *testing.T) {
	result := JoinByComma([]string{"a", "b", "c"})
	if result != "a,b,c" {
		t.Errorf("JoinByComma() = %v, want %v", result, "a,b,c")
	}
}

// TestPluralize_NegativeCount 回归：手写 itoa 对负数返回空串，
// Pluralize(-3, "file") 曾输出 " files"。
func TestPluralize_NegativeCount(t *testing.T) {
	if got := Pluralize(-3, "file"); got != "-3 files" {
		t.Errorf("Pluralize(-3, file) = %q, want %q", got, "-3 files")
	}
}
