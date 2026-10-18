package config

import (
	"fmt"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// ExplicitFields 是「来源已显式提供」的字段路径集合（路径格式与 ValidateTags 错误
// 信息一致：`Server.Port`、`Items[1].Qty`）。FillDefaults / ApplyEnv 经 WithExplicit
// 拿到它后对集合内字段不再填充——yaml 显式写 `port: 0` 不会再被 default 覆盖，
// 零值歧义就此消除，而不必把整份配置改成指针类型
// （[#5](https://github.com/oekit/core/issues/5)）。
//
// LoadBootstrap 内部自动采集（yaml 文档里出现且非 null 的键 + 已设置的 env 变量）；
// 自建加载流程用 ExplicitFromYAML / ExplicitFromEnv 手工采集后传给各 pass。
type ExplicitFields map[string]struct{}

// Mark 记录一条字段路径。
func (e ExplicitFields) Mark(path string) { e[path] = struct{}{} }

// Has 判断字段路径是否已被显式提供。nil 集合恒为 false。
func (e ExplicitFields) Has(path string) bool {
	_, ok := e[path]
	return ok
}

// Merge 并入另一集合。
func (e ExplicitFields) Merge(other ExplicitFields) {
	for k := range other {
		e[k] = struct{}{}
	}
}

// Paths 返回排序后的全部路径（测试 / 诊断用）。
func (e ExplicitFields) Paths() []string {
	out := make([]string, 0, len(e))
	for k := range e {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// PassOption 配置 FillDefaults / ApplyEnv 的单次执行。
type PassOption func(*passConfig)

type passConfig struct {
	explicit ExplicitFields
}

// WithExplicit 告知本次 pass 哪些字段已被来源显式提供：FillDefaults 对这些字段
// 不填 default（即便当前是零值）；ApplyEnv 对这些字段不写 env（yaml 显式零值 >
// env，与文档化的优先级一致）。不传则退回「零值即未设置」的既有判定。
func WithExplicit(e ExplicitFields) PassOption {
	return func(c *passConfig) { c.explicit = e }
}

func newPassConfig(opts []PassOption) *passConfig {
	c := &passConfig{}
	for _, o := range opts {
		if o != nil {
			o(c)
		}
	}
	return c
}

// ---------- 来源采集 ----------

// ExplicitFromYAML 解析 yaml 文档，返回其中显式出现且非 null 的字段路径集合。
// 键名按 v 的结构体类型与 yaml tag 解析，规则同 yaml.v3：`yaml:"name"` 指定键、
// 缺省为字段名全小写、`,inline` 内联到父映射、`yaml:"-"` 跳过；支持锚点别名与
// `<<` 合并键。yaml 中出现但结构体没有对应字段的键被忽略（它们本来也进不了结构体）。
//
// `port: ~` / `port: null` 视为未提供（显式交回 default）；`port: 0`、`hosts: []`
// 才是显式零值。v 必须是非 nil 的结构体指针（只取类型，不写入）。
func ExplicitFromYAML(data []byte, v any) (ExplicitFields, error) {
	rv, err := targetStructValue(v, "ExplicitFromYAML")
	if err != nil {
		return nil, err
	}
	out := ExplicitFields{}
	if len(strings.TrimSpace(string(data))) == 0 {
		return out, nil
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("config: ExplicitFromYAML: %w", err)
	}
	root := &doc
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		root = root.Content[0]
	}
	root = derefAlias(root)
	if root.Kind != yaml.MappingNode {
		return out, nil // 顶层不是映射（空文档 / 标量），结构体不会收到任何字段
	}
	markStructFromMapping(rv.Type(), root, "", out)
	return out, nil
}

// ExplicitFromEnv 返回 env tag 对应变量已设置（非空白）的字段路径集合——即
// ApplyEnv 会写入的全部候选（不论字段当前是否为零值）。用于让 FillDefaults 知道
// `REDIS_DB=0` 这类「env 显式给零值」不该再被 default 覆盖。
//
// 遍历按类型进行：nil 的 *struct 同样下探（ApplyEnv 会为其分配），已存在的切片
// 元素按下标展开。只读，不写字段。v 必须是非 nil 的结构体指针。
func ExplicitFromEnv(v any) (ExplicitFields, error) {
	rv, err := targetStructValue(v, "ExplicitFromEnv")
	if err != nil {
		return nil, err
	}
	out := ExplicitFields{}
	walkTypeWithValue(rv.Type(), rv, "", func(sf reflect.StructField, _ reflect.Value, path string) {
		name, ok := sf.Tag.Lookup("env")
		if !ok || name == "" {
			return
		}
		if raw, ok := os.LookupEnv(name); ok && strings.TrimSpace(raw) != "" {
			out.Mark(path)
		}
	})
	return out, nil
}

// ---------- required ----------

// CheckRequired 校验携带 `required:"true"` tag 的字段都在 explicit 集合内（已被
// yaml / env 显式提供），缺失则一次性报出全部字段并附 env 变量名提示。
// 「必须外部提供、不吃 default」的声明方式：
//
//	type Postgres struct {
//	    Port int `yaml:"port" env:"PG_PORT" required:"true"`
//	}
//
// required 与 default 互斥（有 default 永不缺失），同时出现视为 tag 配置错误。
// LoadBootstrap 在 FillDefaults 之后自动执行；自建流程请传入 ExplicitFromYAML /
// ExplicitFromEnv 的并集。nil 的 *struct 同样下探（里面的 required 字段一样缺失）。
func CheckRequired(v any, explicit ExplicitFields) error {
	rv, err := targetStructValue(v, "CheckRequired")
	if err != nil {
		return err
	}
	var missing []string
	var tagErr error
	walkTypeWithValue(rv.Type(), rv, "", func(sf reflect.StructField, _ reflect.Value, path string) {
		if tagErr != nil {
			return
		}
		raw, ok := sf.Tag.Lookup("required")
		if !ok {
			return
		}
		req, perr := strconv.ParseBool(raw)
		if perr != nil {
			tagErr = fmt.Errorf("field %q: invalid required tag %q (want true/false)", path, raw)
			return
		}
		if !req {
			return
		}
		if def, has := sf.Tag.Lookup("default"); has && def != "" {
			tagErr = fmt.Errorf("field %q: required and default tags are mutually exclusive", path)
			return
		}
		if explicit.Has(path) {
			return
		}
		hint := path
		if env, ok := sf.Tag.Lookup("env"); ok && env != "" {
			hint = fmt.Sprintf("%s (env %s)", path, env)
		}
		missing = append(missing, hint)
	})
	if tagErr != nil {
		return fmt.Errorf("config: CheckRequired: %w", tagErr)
	}
	if len(missing) > 0 {
		return fmt.Errorf("config: CheckRequired: required fields not provided by yaml/env: %s", strings.Join(missing, ", "))
	}
	return nil
}

// ---------- env 拼错侦测 ----------

// CheckEnvOrphans 返回进程环境里带任一 prefix、却没有任何 env tag 消费的变量名
// （排序）。用于在启动期抓 env tag 拼错——`env:"BACKUP_ARCHIVE_ARCHIVE_FORMAT"`
// 这种错编译 / vet / test 全过、服务也正常启动，只有线上覆盖静默失效；而部署时
// 真正的 BACKUP_ARCHIVE_FORMAT 已在环境里，没人消费它就是孤儿。
//
// prefixes 不能为空（否则整份环境都是孤儿）；匹配按字节前缀（传 "APP_" 而非 "APP"
// 可避免 APPLE_X 误报）。返回非空时由调用方决定告警或 fail-fast：
//
//	if orphans, _ := config.CheckEnvOrphans(cfg, "BACKUP_", "PG_"); len(orphans) > 0 {
//	    log.Fatalf("env vars set but consumed by no field (typo in env tag?): %v", orphans)
//	}
//
// 只读；按类型遍历（nil *struct 内的 env tag 同样计入消费方）。
func CheckEnvOrphans(v any, prefixes ...string) ([]string, error) {
	rv, err := targetStructValue(v, "CheckEnvOrphans")
	if err != nil {
		return nil, err
	}
	var valid []string
	for _, p := range prefixes {
		if p = strings.TrimSpace(p); p != "" {
			valid = append(valid, p)
		}
	}
	if len(valid) == 0 {
		return nil, fmt.Errorf("config: CheckEnvOrphans: at least one non-empty prefix is required")
	}
	consumed := map[string]bool{}
	walkTypeWithValue(rv.Type(), rv, "", func(sf reflect.StructField, _ reflect.Value, _ string) {
		if name, ok := sf.Tag.Lookup("env"); ok && name != "" {
			consumed[name] = true
		}
	})
	var orphans []string
	for _, kv := range os.Environ() {
		name, _, ok := strings.Cut(kv, "=")
		if !ok || consumed[name] {
			continue
		}
		for _, p := range valid {
			if strings.HasPrefix(name, p) {
				orphans = append(orphans, name)
				break
			}
		}
	}
	sort.Strings(orphans)
	return orphans, nil
}

// ---------- 内部：按类型遍历 / yaml 键解析 ----------

// walkTypeWithValue 按类型深度优先遍历导出字段：v 有效时随值下探（切片按下标），
// 指向 nil 的 *struct 仍按类型下探（v 传无效值）。路径格式同 walkTaggedStruct。
// 只读，不分配不写入；seen 防御递归类型（仅在无值的类型级下探时生效）。
func walkTypeWithValue(t reflect.Type, v reflect.Value, path string, visit func(reflect.StructField, reflect.Value, string)) {
	walkTypeWithValueSeen(t, v, path, visit, map[reflect.Type]int{})
}

func walkTypeWithValueSeen(t reflect.Type, v reflect.Value, path string, visit func(reflect.StructField, reflect.Value, string), seen map[reflect.Type]int) {
	if t.Kind() != reflect.Struct {
		return
	}
	if !v.IsValid() {
		// 类型级下探：同一类型最多出现一次，避免 type Node struct{ Next *Node } 无限递归
		if seen[t] > 0 {
			return
		}
		seen[t]++
		defer func() { seen[t]-- }()
	}
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if !sf.IsExported() {
			continue
		}
		var fv reflect.Value
		if v.IsValid() {
			fv = v.Field(i)
		}
		fpath := joinPath(path, sf.Name)
		visit(sf, fv, fpath)
		switch sf.Type.Kind() {
		case reflect.Struct:
			walkTypeWithValueSeen(sf.Type, fv, fpath, visit, seen)
		case reflect.Pointer:
			if sf.Type.Elem().Kind() != reflect.Struct {
				continue
			}
			if fv.IsValid() && !fv.IsNil() {
				walkTypeWithValueSeen(sf.Type.Elem(), fv.Elem(), fpath, visit, seen)
			} else {
				walkTypeWithValueSeen(sf.Type.Elem(), reflect.Value{}, fpath, visit, seen)
			}
		case reflect.Slice:
			if !fv.IsValid() {
				continue // 没有值就没有元素
			}
			elem := sf.Type.Elem()
			switch {
			case elem.Kind() == reflect.Struct:
				for j := 0; j < fv.Len(); j++ {
					walkTypeWithValueSeen(elem, fv.Index(j), fmt.Sprintf("%s[%d]", fpath, j), visit, seen)
				}
			case elem.Kind() == reflect.Pointer && elem.Elem().Kind() == reflect.Struct:
				for j := 0; j < fv.Len(); j++ {
					ev := fv.Index(j)
					if ev.IsNil() {
						continue
					}
					walkTypeWithValueSeen(elem.Elem(), ev.Elem(), fmt.Sprintf("%s[%d]", fpath, j), visit, seen)
				}
			}
		}
	}
}

func joinPath(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "." + name
}

// yamlKey 按 yaml.v3 规则解析字段的 yaml 键：`yaml:"name,opts"`；"-" 跳过；
// 空名缺省为字段名全小写；opts 含 inline 时内联到父映射。
func yamlKey(sf reflect.StructField) (key string, inline, skip bool) {
	tag := sf.Tag.Get("yaml")
	if tag == "-" {
		return "", false, true
	}
	parts := strings.Split(tag, ",")
	key = parts[0]
	for _, opt := range parts[1:] {
		if opt == "inline" {
			inline = true
		}
	}
	if key == "" {
		key = strings.ToLower(sf.Name)
	}
	return key, inline, false
}

func derefAlias(n *yaml.Node) *yaml.Node {
	for n != nil && n.Kind == yaml.AliasNode && n.Alias != nil {
		n = n.Alias
	}
	return n
}

// lookupKey 在映射节点里找键：先直接键，再 `<<` 合并键引入的映射（单个或序列）。
func lookupKey(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key && m.Content[i].Tag != "!!merge" {
			return m.Content[i+1]
		}
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		k := m.Content[i]
		if k.Tag != "!!merge" && k.Value != "<<" {
			continue
		}
		src := derefAlias(m.Content[i+1])
		switch src.Kind {
		case yaml.MappingNode:
			if got := lookupKey(src, key); got != nil {
				return got
			}
		case yaml.SequenceNode:
			for _, item := range src.Content {
				if item = derefAlias(item); item.Kind == yaml.MappingNode {
					if got := lookupKey(item, key); got != nil {
						return got
					}
				}
			}
		}
	}
	return nil
}

// markStructFromMapping 把映射节点里出现的键按结构体字段标记进 out。
func markStructFromMapping(t reflect.Type, m *yaml.Node, path string, out ExplicitFields) {
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if !sf.IsExported() {
			continue
		}
		key, inline, skip := yamlKey(sf)
		if skip {
			continue
		}
		fpath := joinPath(path, sf.Name)
		if inline {
			ft := sf.Type
			if ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct {
				markStructFromMapping(ft, m, fpath, out)
			}
			continue
		}
		if val := lookupKey(m, key); val != nil {
			markValue(sf.Type, val, fpath, out)
		}
	}
}

// markValue 标记单个字段并按类型继续下探（struct 映射 / 切片序列）。
// null 标量视为未提供（显式交回 default），不标记也不下探。
func markValue(t reflect.Type, n *yaml.Node, path string, out ExplicitFields) {
	n = derefAlias(n)
	if n == nil || (n.Kind == yaml.ScalarNode && n.Tag == "!!null") {
		return
	}
	out.Mark(path)
	switch t.Kind() {
	case reflect.Struct:
		if n.Kind == yaml.MappingNode {
			markStructFromMapping(t, n, path, out)
		}
	case reflect.Pointer:
		if t.Elem().Kind() == reflect.Struct && n.Kind == yaml.MappingNode {
			markStructFromMapping(t.Elem(), n, path, out)
		}
	case reflect.Slice:
		elem := t.Elem()
		if elem.Kind() == reflect.Pointer {
			elem = elem.Elem()
		}
		if elem.Kind() == reflect.Struct && n.Kind == yaml.SequenceNode {
			for j, item := range n.Content {
				markValue(elem, item, fmt.Sprintf("%s[%d]", path, j), out)
			}
		}
	}
}
