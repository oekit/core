package config

import (
	"fmt"
	"reflect"
	"slices"
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
	// skipUnmarshaler 严格模式对自带 UnmarshalYAML 的类型整段跳过（#127，
	// WithSkipUnmarshaler）——见 WithSkipUnmarshaler 的说明。
	skipUnmarshaler bool
	// envNaming env 命名策略（前缀 / 逐字段覆盖）——见 envnaming.go。
	envNaming EnvNaming
	// trimEnvStrings ApplyEnv 写字符串字段前裁剪 env 值首尾空白（#104，
	// WithEnvTrimStrings）——见 fieldset.go 的 WithTrimStrings。
	trimEnvStrings bool
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
//
// YAML merge key（`<<` 锚点合并，#47）是规范内的合法语法，不拦；merge 内容
// 按**本层类型**继续校验——展开后的未知键照样拒载。锚点挂在未知键上（如
// `_base: &base ...` 的顶层草稿键）仍按未知键拒载——那是真未知键，不是语法。
//
// 配置树里有自带 UnmarshalYAML 专门校验的类型时，可叠加 WithSkipUnmarshaler
// 让通用递归不进入该类型（#127）——避免宿主的专门裁决被通用文案降级。
func WithStrict() LoadOption {
	return func(o *loadOptions) { o.strict = true }
}

// WithSkipUnmarshaler 严格模式下，对**自带 yaml.Unmarshaler 的类型**整段跳过
// 通用递归校验（#127）。
//
// 为什么需要：宿主配置树里常有「自带严格校验的类型」（UnmarshalYAML 里有
// 自己的专门校验与专门裁决错误信息）。缺省时通用递归照样钻进去按 struct
// 字段校验其 YAML 内容，产生两重问题：
//
//   - **专门裁决被降级**：宿主写 `mcp.servers.x.unknow_key`，上游通用校验
//     先拦，报出的是「未知配置键 unknow_key（合法键: …）」的通用文案，而
//     宿主的 UnmarshalYAML 本可给出带域语义的裁决文本——运维失去专门判读
//     信息；
//   - **两条判定面**：宿主的 UnmarshalYAML 里有自己的递归边界（动态 map 键、
//     merge key 等）时，通用递归的边界与宿主不一致——同一份配置两套判法。
//
// 开启后：`reflect.PointerTo(t).Implements(yaml.Unmarshaler)`（含值接收者
// 形态）命中的子树，严格检查直接放行——yaml.v3 解码本就会把该子树交给它
// 的 UnmarshalYAML，宿主的专门校验成为唯一判定面（这正是解码语义：严格
// 校验不该比解码器更深入）。
//
// 与 WithStrict 组合使用（单独使用无意义）；根类型自身实现 Unmarshaler 时
// 整棵树跳过——整份配置都由宿主的 UnmarshalYAML 裁决。
//
// 注意：跳过的只是**通用键校验**，yaml.v3 解码仍会调用该类型的
// UnmarshalYAML——宿主的专门校验错误照常拒载。默认（不传本选项）保持
// 历史行为：通用递归照样进入。
func WithSkipUnmarshaler() LoadOption {
	return func(o *loadOptions) { o.skipUnmarshaler = true }
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
		if err := checkUnknownKeys(data, out, opts); err != nil {
			return err
		}
	}
	return yaml.Unmarshal(data, out)
}

// yamlUnmarshalerType 是 yaml.v3 的 Unmarshaler 接口类型（#127 的跳过判定
// 用）。yaml.v3 解码时对「类型或 *类型实现本接口」的节点调用其
// UnmarshalYAML——严格校验的跳过判定与解码器同口径。
var yamlUnmarshalerType = reflect.TypeFor[yaml.Unmarshaler]()

// implementsYAMLUnmarshaler 报告 t（或 *t）是否自带 yaml.Unmarshaler。
func implementsYAMLUnmarshaler(t reflect.Type) bool {
	if t == nil {
		return false
	}
	return t.Implements(yamlUnmarshalerType) || reflect.PointerTo(t).Implements(yamlUnmarshalerType)
}

// fieldInfo 一个可被 YAML 键命中的字段。
type fieldInfo struct {
	index []int // reflect 字段索引链
}

// yamlFields 收集结构体在 YAML 映射层级的合法键。
// 语义与 yaml.v3 对齐：yaml tag 优先（`yaml:"name"`），无 tag 时用小写字段名；
// `,inline` 的内嵌结构体字段**拍平到本层**；内嵌但无 yaml tag 的字段**不**拍平
// （yaml.v3 把它当普通字段，键名是类型名的小写）；`,inline` 的 map 字段吸收
// 任意键——其元素类型经第二个返回值交给调用方（core#82：此前 inline map 不
// 登记任何键，它吸收的键被本层判成未知键直接拒载，合法配置起不来）；
// `yaml:"-"` 跳过；未导出字段跳过（匿名的未导出内嵌类型除外，见函数体内
// 与 yaml.v3 对齐的说明）。
func yamlFields(t reflect.Type, prefix []int) (fields map[string]fieldInfo, inlineMap reflect.Type) {
	fields = map[string]fieldInfo{}
	for i := range t.NumField() {
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
		for ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		// 只有**显式** `,inline` 才拍平。yaml.v3 对「内嵌但无 yaml tag」的字段
		// 并**不**拍平——它被当作普通字段，键名是类型名的小写（实测确认）。
		if inline {
			switch {
			case ft.Kind() == reflect.Struct:
				sub, _ := yamlFields(ft, idx)
				for k, v := range sub {
					if _, dup := fields[k]; !dup {
						fields[k] = v
					}
				}
				// 注意：内嵌结构体**内部的** inline map 不上抛豁免——实测
				// yaml.v3 只在直接解码的这层结构体上兑现 inline map，嵌套
				// inline 里的 map 吸收的键会被**静默丢弃**（解码不落任何
				// 字段）。严格模式把它们判成未知键是对的：那正是要拦的
				// 「配置写了但哪儿都没去」。
			case ft.Kind() == reflect.Map:
				// inline map：任意键都合法（core#82）——本层不再有未知键；
				// 元素类型交给调用方对键值继续下降校验。
				if inlineMap == nil {
					inlineMap = ft
				}
			}
			continue
		}
		if name == "" {
			name = strings.ToLower(sf.Name)
		}
		if _, dup := fields[name]; !dup {
			fields[name] = fieldInfo{index: idx}
		}
	}
	return fields, inlineMap
}

// checkUnknownKeys 解析 YAML 并沿结构体类型下降，找出第一个未知键。
// opts 携带 WithSkipUnmarshaler（#127）：开启时对自带 UnmarshalYAML 的类型
// 整段跳过。
func checkUnknownKeys(data []byte, out any, opts loadOptions) error {
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil
	}
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		// YAML **语法**错误：直接返回（与后续解码同一份错误，由调用方统一包装成
		// 「parse config file X: ...」）。不吞掉——吞错误会让「配置写错了」变成
		// 「什么都没发生」。
		return err
	}
	rv := reflect.ValueOf(out)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil
	}
	return walkStrict(&node, rv.Type(), "", opts)
}

// walkStrict 沿 node 与类型同步下降；path 是 YAML 键形式的嵌套路径。
func walkStrict(node *yaml.Node, t reflect.Type, path string, opts loadOptions) error {
	if node == nil {
		return nil
	}
	// #127 WithSkipUnmarshaler：自带 UnmarshalYAML 的类型由 yaml.v3 解码时
	// 交给它的专门校验，通用递归不再双判（宿主的专门裁决文本因此不被本层
	// 的通用文案抢先降级）。
	if opts.skipUnmarshaler && implementsYAMLUnmarshaler(t) {
		return nil
	}
	switch node.Kind {
	case yaml.DocumentNode:
		if len(node.Content) == 0 {
			return nil
		}
		return walkStrict(node.Content[0], t, path, opts)
	case yaml.AliasNode:
		return walkStrict(node.Alias, t, path, opts)
	case yaml.MappingNode:
		return walkStrictMapping(node, t, path, opts)
	case yaml.SequenceNode:
		// 切片元素：路径不带键（元素无稳定名），逐个按元素类型检查
		et := t
		for et.Kind() == reflect.Pointer {
			et = et.Elem()
		}
		if et.Kind() != reflect.Slice && et.Kind() != reflect.Array {
			return nil
		}
		for _, child := range node.Content {
			if err := walkStrict(child, et.Elem(), path, opts); err != nil {
				return err
			}
		}
		return nil
	default:
		return nil
	}
}

func walkStrictMapping(node *yaml.Node, t reflect.Type, path string, opts loadOptions) error {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Struct:
		fields, inlineMap := yamlFields(t, nil)
		legal := make([]string, 0, len(fields))
		for k := range fields {
			legal = append(legal, k)
		}
		slices.Sort(legal)
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i]
			val := node.Content[i+1]
			if key.Kind != yaml.ScalarNode {
				continue
			}
			// YAML merge key（#47）：`<<` 是规范内的合法合并语法，不是拼错的键——
			// 严格模式拦**拼错的键**，不拦**合法语法**。merge 的语义是「把映射
			// 内容并入当前层」，故其值按**本层类型**继续校验：merge 展开后的
			// 未知键依然逃不过严格模式（比「整段跳过」的放行更严丝合缝）。
			if key.Value == "<<" {
				if err := walkStrictMergeValue(val, t, path, opts); err != nil {
					return err
				}
				continue
			}
			info, ok := fields[key.Value]
			if !ok {
				// `,inline` 的 map 字段吸收本层任意键（core#82）：键合法不报
				// 未知——否则合法 YAML 被直接拒载（代码注释「任意键都合法」
				// 与实现正好相反的那口锅）。值按 map 元素类型继续下降校验：
				// 元素是结构体时其内部未知键照样逃不过严格模式。
				if inlineMap != nil {
					if err := walkStrict(val, inlineMap.Elem(), joinPath(path, key.Value), opts); err != nil {
						return err
					}
					continue
				}
				return &UnknownKeyError{
					Path:  joinPath(path, key.Value),
					Key:   key.Value,
					Legal: legal,
					Near:  nearestKey(key.Value, legal),
				}
			}
			ft := fieldTypeAt(t, info.index)
			if err := walkStrict(val, ft, joinPath(path, key.Value), opts); err != nil {
				return err
			}
		}
		return nil
	case reflect.Map:
		// map 的键由调用方定义，任意键都合法——但值若是结构体，仍要查其内部
		et := t.Elem()
		for i := 0; i+1 < len(node.Content); i += 2 {
			if err := walkStrict(node.Content[i+1], et, path, opts); err != nil {
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

// walkStrictMergeValue 校验 merge key（`<<`）的值。yaml 规范允许两种形态：
// 单映射（含锚点别名，walkStrict 的 AliasNode 分支自动解引）与映射序列
// （多锚点按序合并）。其余形态（非法配置）交回 walkStrict 按默认分支放行，
// 由后续 yaml.v3 解码给出语法层错误。
func walkStrictMergeValue(val *yaml.Node, t reflect.Type, path string, opts loadOptions) error {
	if val.Kind == yaml.SequenceNode {
		for _, child := range val.Content {
			if err := walkStrict(child, t, path, opts); err != nil {
				return err
			}
		}
		return nil
	}
	return walkStrict(val, t, path, opts)
}

func fieldTypeAt(t reflect.Type, index []int) reflect.Type {
	for _, i := range index {
		t = t.Field(i).Type
		for t.Kind() == reflect.Pointer {
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
			cur[j] = min(cur[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(br)]
}
