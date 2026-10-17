package transport

import "sort"

// MapHeader 是内存 Header 实现（map[string][]string），供测试与适配器使用。
// 非并发安全——并发场景由使用方加锁或经框架适配层。
type MapHeader map[string][]string

func (h MapHeader) Get(key string) string {
	vs := h[key]
	if len(vs) == 0 {
		return ""
	}
	return vs[0]
}

func (h MapHeader) Values(key string) []string {
	vs := h[key]
	if vs == nil {
		return nil
	}
	out := make([]string, len(vs))
	copy(out, vs)
	return out
}

func (h MapHeader) Set(key, value string) {
	h[key] = []string{value}
}

func (h MapHeader) Add(key, value string) {
	h[key] = append(h[key], value)
}

func (h MapHeader) Del(key string) {
	delete(h, key)
}

func (h MapHeader) Keys() []string {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// NoopHeader 是只读空实现：客户端实现的 ReplyHeader 占位，写入零值返回。
type NoopHeader struct{}

func (NoopHeader) Get(string) string      { return "" }
func (NoopHeader) Values(string) []string { return nil }
func (NoopHeader) Set(string, string)     {}
func (NoopHeader) Add(string, string)     {}
func (NoopHeader) Del(string)             {}
func (NoopHeader) Keys() []string         { return nil }
