package stringx

import "strconv"

// Pluralize 返回英文单词的复数形式。
// 规则:
//   - count == 1 → "1 word"
//   - 以 "y" 结尾且前一个字符为辅音 → 去 "y" 加 "ies" (story → stories)
//   - 以 "s", "sh", "ch", "x", "z" 结尾 → 加 "es" (box → boxes)
//   - 其他 → 加 "s" (file → files)
//
// 用法:
//
//	stringx.Pluralize(3, "file")   // "3 files"
//	stringx.Pluralize(1, "file")   // "1 file"
//	stringx.Pluralize(5, "story")  // "5 stories"
func Pluralize(count int, word string) string {
	if count == 1 {
		return "1 " + word
	}

	n := len(word)
	if n == 0 {
		return "0"
	}

	last := word[n-1]
	if last == 'y' && n > 1 {
		prev := word[n-2]
		if prev != 'a' && prev != 'e' && prev != 'i' && prev != 'o' && prev != 'u' {
			return itoa(count) + " " + word[:n-1] + "ies"
		}
	}

	if last == 's' || last == 'x' || last == 'z' {
		return itoa(count) + " " + word + "es"
	}
	if n >= 2 {
		tail := word[n-2:]
		if tail == "sh" || tail == "ch" {
			return itoa(count) + " " + word + "es"
		}
	}

	return itoa(count) + " " + word + "s"
}

// itoa 简单整数转字符串，避免引入 strconv。
func itoa(n int) string {
	// strconv 标准实现：原手写版 for n>0 循环对负数返回空串
	return strconv.Itoa(n)
}
