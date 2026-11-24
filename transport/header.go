package transport

import (
	"maps"
	"net/textproto"
	"slices"
)

// MapHeader 是内存 Header 实现（map[string][]string），供测试与适配器使用。
// 非并发安全——并发场景由使用方加锁或经框架适配层。
//
// 键按 MIME 规范形式归一（core#192，net/textproto.CanonicalMIMEHeaderKey，
// 与 http.Header 同语义）："content-type" 与 "Content-Type" 命中同一槽位。
// 此前精确 map 查找与 http.Header / 同生态适配层语义相反——跨实现搬运
// header 时大小写不一致即静默读空/写出重复键。
type MapHeader map[string][]string

func (h MapHeader) Get(key string) string {
	vs := h[textproto.CanonicalMIMEHeaderKey(key)]
	if len(vs) == 0 {
		return ""
	}
	return vs[0]
}

func (h MapHeader) Values(key string) []string {
	vs := h[textproto.CanonicalMIMEHeaderKey(key)]
	if vs == nil {
		return nil
	}
	out := make([]string, len(vs))
	copy(out, vs)
	return out
}

func (h MapHeader) Set(key, value string) {
	h[textproto.CanonicalMIMEHeaderKey(key)] = []string{value}
}

func (h MapHeader) Add(key, value string) {
	k := textproto.CanonicalMIMEHeaderKey(key)
	h[k] = append(h[k], value)
}

func (h MapHeader) Del(key string) {
	delete(h, textproto.CanonicalMIMEHeaderKey(key))
}

func (h MapHeader) Keys() []string {
	return slices.Sorted(maps.Keys(h))
}

// NoopHeader 是只读空实现：客户端实现的 ReplyHeader 占位，写入零值返回。
type NoopHeader struct{}

func (NoopHeader) Get(string) string      { return "" }
func (NoopHeader) Values(string) []string { return nil }
func (NoopHeader) Set(string, string)     {}
func (NoopHeader) Add(string, string)     {}
func (NoopHeader) Del(string)             {}
func (NoopHeader) Keys() []string         { return nil }
