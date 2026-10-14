package enumx

import (
	"fmt"
	"sort"
)

// Enum 是枚举接口。
type Enum interface {
	// Value 返回枚举的整数值。
	Value() int

	// Label 返回枚举的显示名称。
	Label() string

	// IsValid 检查枚举值是否有效。
	IsValid() bool
}

// Registry 是枚举注册表，提供类型安全的枚举管理。
type Registry[T Enum] struct {
	enums  map[int]T
	labels map[int]string
}

// NewRegistry 创建枚举注册表。
func NewRegistry[T Enum](enums ...T) *Registry[T] {
	r := &Registry[T]{
		enums:  make(map[int]T, len(enums)),
		labels: make(map[int]string, len(enums)),
	}
	for _, e := range enums {
		r.enums[e.Value()] = e
		r.labels[e.Value()] = e.Label()
	}
	return r
}

// Get 根据整数值获取枚举。
func (r *Registry[T]) Get(value int) (T, bool) {
	e, ok := r.enums[value]
	return e, ok
}

// MustGet 根据整数值获取枚举，不存在时 panic。
func (r *Registry[T]) MustGet(value int) T {
	e, ok := r.enums[value]
	if !ok {
		panic(fmt.Sprintf("enumx: value %d not found", value))
	}
	return e
}

// Label 获取枚举的显示名称。
func (r *Registry[T]) Label(value int) string {
	if label, ok := r.labels[value]; ok {
		return label
	}
	return fmt.Sprintf("unknown(%d)", value)
}

// All 获取所有已注册的枚举值（按 Value 升序排列）。
func (r *Registry[T]) All() []T {
	result := make([]T, 0, len(r.enums))
	for _, e := range r.enums {
		result = append(result, e)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Value() < result[j].Value()
	})
	return result
}

// Values 获取所有有效的枚举整数值（升序排列）。
func (r *Registry[T]) Values() []int {
	result := make([]int, 0, len(r.enums))
	for v := range r.enums {
		result = append(result, v)
	}
	sort.Ints(result)
	return result
}

// IsValid 检查整数值是否是有效的枚举。
func (r *Registry[T]) IsValid(value int) bool {
	_, ok := r.enums[value]
	return ok
}

// Len 返回枚举数量。
func (r *Registry[T]) Len() int {
	return len(r.enums)
}

// Map 返回 value→label 映射。
func (r *Registry[T]) Map() map[int]string {
	m := make(map[int]string, len(r.labels))
	for k, v := range r.labels {
		m[k] = v
	}
	return m
}
