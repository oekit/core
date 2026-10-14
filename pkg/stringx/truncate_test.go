package stringx

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncRunes(t *testing.T) {
	out, truncated := TruncRunes("技能名字很长", 2)
	if !truncated || out != "技能" {
		t.Fatalf("out=%q truncated=%v", out, truncated)
	}
	if out2, tr := TruncRunes("abc", 5); tr || out2 != "abc" {
		t.Fatalf("无需截断应原样: %q %v", out2, tr)
	}
	if out3, tr := TruncRunes("abc", -1); !tr || out3 != "" {
		t.Fatalf("负上限按全截断: %q %v", out3, tr)
	}
}

func TestTruncBytesRuneAligned(t *testing.T) {
	s := "技能名字很长" // 每字 3 字节
	out, truncated := TruncBytes(s, 4) // 预算落在第 2 字内 → 回退到 3 字节边界
	if !truncated || out != "技" {
		t.Fatalf("out=%q truncated=%v（应回退到完整 rune 边界）", out, truncated)
	}
	if out2, tr := TruncBytes(s, len(s)); tr || out2 != s {
		t.Fatalf("预算足够应原样返回: %q %v", out2, tr)
	}
	if out3, tr := TruncBytes(s, 0); !tr || out3 != "" {
		t.Fatalf("零预算应返回空串: %q %v", out3, tr)
	}
}

func TestTruncEllipsis(t *testing.T) {
	if got := TruncEllipsis("abcdef", 3); got != "abc…" {
		t.Fatalf("got %q", got)
	}
	if got := TruncEllipsis("ab", 3); got != "ab" {
		t.Fatalf("无需截断原样: %q", got)
	}
	if got := TruncEllipsis("x", 0); got != "…" {
		t.Fatalf("零上限: %q", got)
	}
	long := strings.Repeat("界", 100)
	if got := TruncEllipsis(long, 5); !utf8.ValidString(got) || len([]rune(got)) != 6 {
		t.Fatalf("rune 安全截断: %q", got)
	}
}
