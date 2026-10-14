package stringx

import "unicode/utf8"

// TruncRunes 按 rune 截断，避免多字节字符被腰斩产生非法 UTF-8。
// 第二个返回值 = 是否真发生了截断。n<0 视为非法上限，按"全部截断"处理
//（返回空串），不 panic。
func TruncRunes(s string, n int) (string, bool) {
	if n < 0 {
		return "", true
	}
	// 先计数再转换：无需截断时不产生 []rune 全量分配
	if utf8.RuneCountInString(s) <= n {
		return s, false
	}
	r := []rune(s)
	return string(r[:n]), true
}

// TruncBytes 按字节预算截断（rune 对齐：预算落点切进多字节字符时回退到
// RuneStart 边界，不产生非法 UTF-8 尾字节）。第二个返回值 = 是否真发生了
// 截断。budget<0 视为非法上限，按"全部截断"处理（返回空串），不 panic。
func TruncBytes(s string, budget int) (string, bool) {
	if budget < 0 {
		return "", true
	}
	if len(s) <= budget {
		return s, false
	}
	cut := budget
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut], true
}

// TruncEllipsis 截断并追加省略号（列表/标题/事件载荷展示面统一语义）。
// n<=0 返回省略号；无需截断时原样返回。
func TruncEllipsis(s string, n int) string {
	if n <= 0 {
		return "…"
	}
	out, truncated := TruncRunes(s, n)
	if truncated {
		return out + "…"
	}
	return out
}
