package stringx

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	// ErrInvalidStartPosition indicates the start position is invalid.
	ErrInvalidStartPosition = errors.New("start position is invalid")
	// ErrInvalidStopPosition indicates the stop position is invalid.
	ErrInvalidStopPosition = errors.New("stop position is invalid")
	// ErrInvalidRange indicates the substring range is inverted (start > stop).
	ErrInvalidRange = errors.New("substring range is inverted")
)

// placeholder 是一个占位符类型，用于 map 集合操作。
type placeholder = struct{}

var mark placeholder

// Contains 检查 str 是否在 list 中。
func Contains(list []string, str string) bool {
	for _, each := range list {
		if each == str {
			return true
		}
	}
	return false
}

// Filter 从 s 中过滤掉满足 filter 条件的字符。
func Filter(s string, filter func(r rune) bool) string {
	var n int
	chars := []rune(s)
	for i, x := range chars {
		if n < i {
			chars[n] = x
		}
		if !filter(x) {
			n++
		}
	}
	return string(chars[:n])
}

// FirstN 返回 s 的前 n 个字符，可选添加省略号。
func FirstN(s string, n int, ellipsis ...string) string {
	var i int
	for j := range s {
		if i == n {
			ret := s[:j]
			for _, each := range ellipsis {
				ret += each
			}
			return ret
		}
		i++
	}
	return s
}

// HasEmpty 检查 args 中是否存在空字符串。
func HasEmpty(args ...string) bool {
	for _, arg := range args {
		if len(arg) == 0 {
			return true
		}
	}
	return false
}

// Join 将多个字符串用 sep 连接，忽略空字符串。
func Join(sep byte, elem ...string) string {
	var size int
	for _, e := range elem {
		size += len(e)
	}
	if size == 0 {
		return ""
	}

	buf := make([]byte, 0, size+len(elem)-1)
	for _, e := range elem {
		if len(e) == 0 {
			continue
		}
		if len(buf) > 0 {
			buf = append(buf, sep)
		}
		buf = append(buf, e...)
	}
	return string(buf)
}

// NotEmpty 检查 args 中所有字符串是否都不为空。
func NotEmpty(args ...string) bool {
	return !HasEmpty(args...)
}

// Remove 从 strings 中移除指定的 strs。
func Remove(list []string, strs ...string) []string {
	out := append([]string(nil), list...)
	for _, str := range strs {
		var n int
		for _, v := range out {
			if v != str {
				out[n] = v
				n++
			}
		}
		out = out[:n]
	}
	return out
}

// Reverse 反转字符串。
func Reverse(s string) string {
	runes := []rune(s)
	for from, to := 0, len(runes)-1; from < to; from, to = from+1, to-1 {
		runes[from], runes[to] = runes[to], runes[from]
	}
	return string(runes)
}

// Substr 返回 [start, stop) 范围内的子字符串，支持 UTF-8。
func Substr(str string, start, stop int) (string, error) {
	rs := []rune(str)
	length := len(rs)

	if start < 0 || start > length {
		return "", ErrInvalidStartPosition
	}
	if stop < 0 || stop > length {
		return "", ErrInvalidStopPosition
	}
	if start > stop {
		return "", ErrInvalidRange
	}
	return string(rs[start:stop]), nil
}

// TakeOne 返回非空的字符串，如果 valid 非空则返回 valid，否则返回 or。
func TakeOne(valid, or string) string {
	if len(valid) > 0 {
		return valid
	}
	return or
}

// TakeWithPriority 返回第一个非空的函数返回值。
func TakeWithPriority(fns ...func() string) string {
	for _, fn := range fns {
		val := fn()
		if len(val) > 0 {
			return val
		}
	}
	return ""
}

// ToCamelCase 将字符串首字母转为小写。
func ToCamelCase(s string) string {
	for i, v := range s {
		// i 是首 rune 的字节偏移，须按其 UTF-8 宽度切片，
		// 否则多字节首字符会截掉后续字节、产生损坏的 UTF-8
		return string(unicode.ToLower(v)) + s[i+utf8.RuneLen(v):]
	}
	return ""
}

// Union 合并两个字符串切片并去重。
func Union(first, second []string) []string {
	set := make(map[string]placeholder)
	for _, each := range first {
		set[each] = mark
	}
	for _, each := range second {
		set[each] = mark
	}

	merged := make([]string, 0, len(set))
	for k := range set {
		merged = append(merged, k)
	}
	return merged
}

// JoinWith 将字符串切片用指定分隔符连接。
func JoinWith(elem []string, sep string) string {
	var size int
	for _, e := range elem {
		size += len(e)
	}
	if size == 0 {
		return ""
	}

	buf := make([]byte, 0, size+len(elem)-1)
	for _, e := range elem {
		if len(e) == 0 {
			continue
		}
		if len(buf) > 0 {
			buf = append(buf, sep...)
		}
		buf = append(buf, e...)
	}
	return string(buf)
}

// SplitWith 将字符串按指定分隔符列表分割后返回字段切片。
func SplitWith(str string, sep []string) []string {
	for _, v := range sep {
		str = strings.ReplaceAll(str, v, " ")
	}
	return strings.Fields(str)
}

// SplitByComma 将字符串按逗号分割。
func SplitByComma(str string) []string {
	return strings.Split(str, ",")
}

// JoinByComma 将字符串切片用逗号连接。
func JoinByComma(str []string) string {
	return strings.Join(str, ",")
}
