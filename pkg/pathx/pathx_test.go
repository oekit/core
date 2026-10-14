package pathx

import (
	"testing"
)

func TestMatchesPath(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		path     string
		want     bool
	}{
		// 标准 glob 匹配
		{"single pattern match", []string{"*.go"}, "main.go", true},
		{"single pattern no match", []string{"*.go"}, "main.js", false},
		{"extension pattern", []string{"*.test"}, "foo.test", true},

		// /** 目录前缀匹配
		{"globstar nested", []string{"vendor/**"}, "vendor/foo/bar.go", true},
		{"globstar direct child", []string{"node_modules/**"}, "node_modules/x", true},
		{"globstar no match", []string{"vendor/**"}, "src/main.go", false},

		// 多模式
		{"multiple patterns first matches", []string{"vendor/**", "*.test"}, "vendor/foo/bar.go", true},
		{"multiple patterns second matches", []string{"vendor/**", "*.test"}, "foo.test", true},
		{"multiple patterns none match", []string{"vendor/**", "*.test"}, "src/main.go", false},

		// 目录直接子项
		{"directory single child", []string{"test/*.txt"}, "test/a.txt", true},

		// 边界条件
		{"empty path", []string{"*.go"}, "", false},
		{"empty patterns", []string{}, "main.go", false},
		{"whitespace path", []string{"*.go"}, "  ", false},
		{"empty pattern skipped", []string{"", "*.go"}, "main.go", true},
		{"whitespace pattern skipped", []string{"   ", "*.go"}, "main.go", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MatchesPath(tt.patterns, tt.path); got != tt.want {
				t.Errorf("MatchesPath(%v, %q) = %v, want %v", tt.patterns, tt.path, got, tt.want)
			}
		})
	}
}
