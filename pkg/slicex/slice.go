package slicex

import (
	"fmt"
	"strings"
)

// Contains 检查切片中是否包含指定元素。
func Contains[T comparable](slice []T, target T) bool {
	for _, v := range slice {
		if v == target {
			return true
		}
	}
	return false
}

// Index 返回元素在切片中的索引，不存在返回 -1。
func Index[T comparable](slice []T, target T) int {
	for i, v := range slice {
		if v == target {
			return i
		}
	}
	return -1
}

// Unique 去重，保持原始顺序。
func Unique[T comparable](slice []T) []T {
	seen := make(map[T]struct{}, len(slice))
	result := make([]T, 0, len(slice))
	for _, v := range slice {
		if _, ok := seen[v]; !ok {
			seen[v] = struct{}{}
			result = append(result, v)
		}
	}
	return result
}

// Filter 过滤切片，保留满足条件的元素。
func Filter[T any](slice []T, fn func(T) bool) []T {
	result := make([]T, 0, len(slice))
	for _, v := range slice {
		if fn(v) {
			result = append(result, v)
		}
	}
	return result
}

// Map 对切片中每个元素应用转换函数。
func Map[T, R any](slice []T, fn func(T) R) []R {
	result := make([]R, len(slice))
	for i, v := range slice {
		result[i] = fn(v)
	}
	return result
}

// Reduce 将切片归约为单个值。
func Reduce[T, R any](slice []T, init R, fn func(R, T) R) R {
	result := init
	for _, v := range slice {
		result = fn(result, v)
	}
	return result
}

// Join 将切片元素用分隔符连接为字符串。
func Join[T any](slice []T, sep string) string {
	parts := make([]string, len(slice))
	for i, v := range slice {
		parts[i] = fmt.Sprintf("%v", v)
	}
	return strings.Join(parts, sep)
}

// GroupBy 按 key 函数分组。
func GroupBy[T any, K comparable](slice []T, fn func(T) K) map[K][]T {
	result := make(map[K][]T)
	for _, v := range slice {
		key := fn(v)
		result[key] = append(result[key], v)
	}
	return result
}

// ToMap 将切片转为 map，key 由 keyFn 提取。
func ToMap[T any, K comparable](slice []T, keyFn func(T) K) map[K]T {
	result := make(map[K]T, len(slice))
	for _, v := range slice {
		result[keyFn(v)] = v
	}
	return result
}

// Chunk 将切片分成指定大小的块。
func Chunk[T any](slice []T, size int) [][]T {
	if size <= 0 {
		return nil
	}
	var result [][]T
	for i := 0; i < len(slice); i += size {
		end := i + size
		if end > len(slice) {
			end = len(slice)
		}
		result = append(result, slice[i:end])
	}
	return result
}

// Reverse 反转切片（返回新切片，不修改原始切片）。
func Reverse[T any](slice []T) []T {
	result := make([]T, len(slice))
	for i, v := range slice {
		result[len(slice)-1-i] = v
	}
	return result
}

// First 返回 slice 中第一个满足 fn 的元素。
// 若无元素满足，返回零值。
func First[T any](fn func(T) bool, slice ...T) T {
	for _, v := range slice {
		if fn(v) {
			return v
		}
	}
	var zero T
	return zero
}

// Flatten 将二维切片展平为一维。
func Flatten[T any](slices [][]T) []T {
	total := 0
	for _, s := range slices {
		total += len(s)
	}
	result := make([]T, 0, total)
	for _, s := range slices {
		result = append(result, s...)
	}
	return result
}
