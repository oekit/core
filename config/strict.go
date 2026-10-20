package config

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// 严格模式：未知键拒载（#20）。
//
// 为什么不用 yaml.v3 自带的 Decoder.KnownFields(true)：它只说
// 「field X not found in type Y」——**不告诉用户这一层有哪些合法键**，也不给
// **嵌套路径**。对一个 200+ 字段的配置结构体，用户看到「field base_ur not found
// in type Config」时，既不知道自己在哪一层，也不知道正确拼法，只能去翻源码。
//
// 严格模式的错误信息要回答三件事：
//  1. 我在**哪**写错了（嵌套路径，YAML 键形式）；
//  2. 这一层**有哪些合法键**；
//  3. 你是不是想写**别的**（近似拼写建议）。

// UnknownKeyError 未知配置键错误。
type UnknownKeyError struct {
	// Path 嵌套路径（YAML 键形式），如 "llm.base_ur"；顶层键时只有键名。
	Path string
	// Key 未知键名。
	Key string
	// Legal 该层级的合法键清单（YAML 键形式，已排序）。
	Legal []string
	// Near 近似拼写建议（与 Legal 中某键编辑距离最小且足够近时才有值）。
	Near string
}

func (e *UnknownKeyError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "config: 未知配置键 %q", e.Path)
	if e.Near != "" {
		fmt.Fprintf(&b, "（是否想写 %q？）", e.Near)
	}
	if len(e.Legal) > 0 {
		shown := e.Legal
		suffix := ""
		if len(shown) > maxLegalKeysShown {
			suffix = fmt.Sprintf("（共 %d 个，仅列前 %d 个）", len(e.Legal), maxLegalKeysShown)
			shown = shown[:maxLegalKeysShown]
		}
		fmt.Fprintf(&b, "；该层合法键：%s%s", strings.Join(shown, ", "), suffix)
	}
	return b.String()
}

// maxLegalKeysShown 错误信息里最多列出的合法键个数——200+ 字段的结构体全列出来
// 是噪音，比不给还糟。
const maxLegalKeysShown = 12

// LoadOption 加载选项。
type LoadOption func(*loadOptions)

type loadOptions struct {
	strict bool
}

// WithStrict 严格模式：**未知键直接拒载**，不做静默忽略。
//
// 默认（不传本选项）保持历史行为——未知键被 yaml.v3 静默丢弃。严格模式是
// 显式开启的，因为它会让「之前一直能跑（因为键拼错被忽略）」的配置开始报错。
//
// 错误信息带**嵌套路径**与**该层合法键清单**，并给出近似拼写建议：
//
//	config: 未知配置键 "llm.base_ur"（是否想写 "base_url"？）；该层合法键：
//	base_url, timeout_ms, api_key
func WithStrict() LoadOption {
	return func(o *loadOptions) { o.strict = true }
}

func newLoadOptions(opts []LoadOption) loadOptions {
	var o loadOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	return o
}

// unmarshalConfig 按选项解码 YAML。strict 时先做未知键检查再解码——
// 先检查是为了让错误信息指向「键」而不是解码器内部的类型名。
func unmarshalConfig(data []byte, out any, opts loadOptions) error {
	if opts.strict {
		if err := checkUnknownKeys(data, out); err != nil {
			return err
		}
	}
	return yaml.Unmarshal(data, out)
}

// fieldInfo 一个可被 YAML 键命中的字段。
type fieldInfo struct {
	index []int // reflect 字段索引链
}

// yamlFields 收集结构体在 YAML 映射层级的合法键。
// 语义与 yaml.v3 对齐：yaml tag 优先（`yaml:"name"`），无 tag 时用小写字段名；
// `,inline` 的内嵌结构体字段**拍平到本层**；匿���内嵌结构体（无 yaml tag）同样
// 按 inline 处理；`yaml:"-"` 跳过；未导出字段跳过。
func yamlFields(t reflect.Type, prefix []int) map[string]fieldInfo {
	out := map[string]fieldInfo{}
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		// yaml.v3 的规则是「跳过**私有且非匿名字段**」——匿名的未导出类型
		// （`inner` 内嵌）**仍会被处理**（实测：内嵌 + `,inline` 的未导出结构体
		// 字段照样被解码）。这里必须与 yaml.v3 一致，否则合法键会被误报成未知键。
		if !sf.IsExported() && !sf.Anonymous {
			continue
		}
		tag := sf.Tag.Get("yaml")
		parts := strings.Split(tag, ",")
		name := parts[0]
		inline := false
		for _, opt := range parts[1:] {
			if opt == "inline" {
				inline = true
			}
		}
		if name == "-" {
			continue
		}
		idx := append(append([]int{}, prefix...), i)
		ft := sf.Type
		for ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}
		// 只有**显式** `,inline` 才拍平。yaml.v3 对「内嵌但无 yaml tag」的字段
		// 并**不**拍平——它被当作普通字段，键名是类型名的小写（实测确认）。
		if inline {
			if ft.Kind() == reflect.Struct {
				for k, v := range yamlFields(ft, idx) {
					if _, dup := out[k]; !dup {
						out[k] = v
					}
				}
				continue
			}
			// inline map：任意键都合法，交给调用方按 map 处理
			continue
		}
		if name == "" {
			name = strings.ToLower(sf.Name)
		}
		if _, dup := out[name]; !dup {
			out[name] = fieldInfo{index: idx}
		}
	}
	return out
}

// checkUnknownKeys 解析 YAML 并沿结构体类型下降，找出第一个未知键。
func checkUnknownKeys(data []byte, out any) error {
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil
	}
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return nil // 语法错误交给 yaml.Unmarshal 统一报，这里不重复
	}
	rv := reflect.ValueOf(out)
	for rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil
	}
	return walkStrict(&node, rv.Type(), "")
}

// walkStrict 沿 node 与类型同步下降；path 是 YAML 键形式的嵌套路径。
func walkStrict(node *yaml.Node, t reflect.Type, path string) error {
	if node == nil {
		return nil
	}
	switch node.Kind {
	case yaml.DocumentNode:
		if len(node.Content) == 0 {
			return nil
		}
		return walkStrict(node.Content[0], t, path)
	case yaml.AliasNode:
		return walkStrict(node.Alias, t, path)
	case yaml.MappingNode:
		return walkStrictMapping(node, t, path)
	case yaml.SequenceNode:
		// 切片元素：路径不带键（元素无稳定名），逐个按元素类型检查
		et := t
		for et.Kind() == reflect.Ptr {
			et = et.Elem()
		}
		if et.Kind() != reflect.Slice && et.Kind() != reflect.Array {
			return nil
		}
		for i, child := range node.Content {
			_ = i
			if err := walkStrict(child, et.Elem(), path); err != nil {
				return err
			}
		}
		return nil
	default:
		return nil
	}
}

func walkStrictMapping(node *yaml.Node, t reflect.Type, path string) error {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Struct:
		fields := yamlFields(t, nil)
		legal := make([]string, 0, len(fields))
		for k := range fields {
			legal = append(legal, k)
		}
		sort.Strings(legal)
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i]
			val := node.Content[i+1]
			if key.Kind != yaml.ScalarNode {
				continue
			}
			info, ok := fields[key.Value]
			if !ok {
				return &UnknownKeyError{
					Path:  joinPath(path, key.Value),
					Key:   key.Value,
					Legal: legal,
					Near:  nearestKey(key.Value, legal),
				}
			}
			ft := fieldTypeAt(t, info.index)
			if err := walkStrict(val, ft, joinPath(path, key.Value)); err != nil {
				return err
			}
		}
		return nil
	case reflect.Map:
		// map 的键由调用方定义，任意键都合法——但值若是结构体，仍要查其内部
		et := t.Elem()
		for i := 0; i+1 < len(node.Content); i += 2 {
			if err := walkStrict(node.Content[i+1], et, path); err != nil {
				return err
			}
		}
		return nil
	default:
		return nil
	}
}

func joinPath(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

func fieldTypeAt(t reflect.Type, index []int) reflect.Type {
	for _, i := range index {
		t = t.Field(i).Type
		for t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
	}
	return t
}

// nearestKey 在 legal 里找与 key 编辑距离最近且足够接近的候选（拼写建议）。
func nearestKey(key string, legal []string) string {
	best, bestDist := "", 1<<30
	for _, c := range legal {
		d := editDistance(strings.ToLower(key), strings.ToLower(c))
		// 距离阈值随长度放宽：长键多一个字符很常见
		limit := 1
		if len(key) >= 5 {
			limit = 2
		}
		if d < bestDist && d <= limit {
			best, bestDist = c, d
		}
	}
	return best
}

// editDistance 编辑距离（Levenshtein），只算长度差以内的 DP。
func editDistance(a, b string) int {
	ar, br := []rune(a), []rune(b)
	if len(ar) == 0 {
		return len(br)
	}
	if len(br) == 0 {
		return len(ar)
	}
	prev := make([]int, len(br)+1)
	cur := make([]int, len(br)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ar); i++ {
		cur[0] = i
		for j := 1; j <= len(br); j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			cur[j] = minInt(cur[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(br)]
}

func minInt(vals ...int) int {
	m := vals[0]
	for _, v := range vals[1:] {
		if v < m {
			m = v
		}
	}
	return m
}
