package config

import (
	"fmt"
	"reflect"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

// FieldSet 记录结构体字段是否被**显式提供**（YAML 里出现过对应 key，或 env tag
// 成功写入）。用于消除零值歧义：`port: 0` 与未写字段对反射不可区分，但前者
// 是调用方的显式选择，不应被 default 覆盖，也不应被 env 覆盖。
//
// 键为字段路径（`Server.Port`、`Items[0].Qty`），与 tag 校验错误信息同格式。
// 由 FieldSetFromYAML 构建，经 WithFieldSet 传入 FillDefaults / ApplyEnv /
// ValidateTags；ApplyEnv 成功写入时会在集合里补记该路径。
type FieldSet map[string]bool

// Has 报告字段路径是否被显式提供。
func (s FieldSet) Has(path string) bool {
	if s == nil {
		return false
	}
	return s[path]
}

// PassOption 定制 FillDefaults / ApplyEnv / ValidateTags 的单次执行。
type PassOption func(*passConfig)

type passConfig struct {
	set           FieldSet
	envNamingLint bool
}

// WithFieldSet 传入显式提供集合：命中的路径视为「已有调用方值」——
// FillDefaults 不再填 default，ApplyEnv 不再覆盖（即使当前是零值）。
// ApplyEnv 成功写入时会向该集合补记路径（map 引用语义，调用方可见）。
func WithFieldSet(s FieldSet) PassOption {
	return func(c *passConfig) { c.set = s }
}

// WithEnvNamingLint 让 ValidateTags 对「声明的 env 名 ≠ 由字段/yaml key 推导的
// 约定名」报错（纯静态，不读进程环境）。有意使用自定义名时在字段上打
// `ignorederived:"true"` 豁免。默认关闭——命名不一致并不一定是错误。
func WithEnvNamingLint() PassOption {
	return func(c *passConfig) { c.envNamingLint = true }
}

func buildPassConfig(opts []PassOption) *passConfig {
	c := &passConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}
	return c
}

// FieldSetFromYAML 解析 YAML data，标记 v 的类型里在 data 中**显式出现**的字段。
// key 存在即算显式提供（含 `port: 0` / `name: ""` / `port: null`）；key 缺失则
// 该字段（及其嵌套）不算提供。
//
// v 必须是非 nil 的结构体指针；data 为空或注释-only 时返回空集合。
// 与 LoadBootstrap 同源使用时请传入 ${VAR} 展开后的同一份 data。
func FieldSetFromYAML(v any, data []byte) (FieldSet, error) {
	rv, err := targetStructValue(v, "FieldSetFromYAML")
	if err != nil {
		return nil, err
	}
	set := FieldSet{}
	if len(strings.TrimSpace(string(data))) == 0 {
		return set, nil
	}
	var root any
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("config: FieldSetFromYAML: parse yaml: %w", err)
	}
	markSetFromYAML(rv, root, "", set)
	return set, nil
}

// markSetFromYAML 沿 struct 类型与 YAML 树同步下降，key 出现即标记字段路径。
// node 为 map[string]any（或 nil）；切片元素对应 []any。
func markSetFromYAML(v reflect.Value, node any, path string, set FieldSet) {
	m, _ := node.(map[string]any)
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if !sf.IsExported() {
			continue
		}
		fv := v.Field(i)
		fpath := sf.Name
		if path != "" {
			fpath = path + "." + sf.Name
		}
		// yaml ",inline"：子字段与父结构体共用同一映射。通用 map 解码下 inline
		// 子字段被拍平到父级，按字段名取 key 在 m 里永远 miss——须以**同一 node**
		// 继续下推（含 nil 的 *struct 用零值仅作类型下推，不写入）。
		if yamlInline(sf) {
			switch {
			case sf.Type.Kind() == reflect.Struct:
				markSetFromYAML(fv, node, fpath, set)
			case sf.Type.Kind() == reflect.Pointer && sf.Type.Elem().Kind() == reflect.Struct:
				if !fv.IsNil() {
					markSetFromYAML(fv.Elem(), node, fpath, set)
				} else {
					markSetFromYAML(reflect.New(sf.Type.Elem()).Elem(), node, fpath, set)
				}
			}
			continue
		}
		key := yamlKey(sf)
		if key == "" {
			continue
		}
		child, ok := m[key]
		if !ok {
			continue // YAML 未提供该 key → 字段及其嵌套均不算显式
		}
		set[fpath] = true
		switch sf.Type.Kind() {
		case reflect.Struct:
			markSetFromYAML(fv, child, fpath, set)
		case reflect.Pointer:
			if sf.Type.Elem().Kind() != reflect.Struct {
				continue
			}
			if fv.IsNil() {
				// YAML 提供了该 key，但指针仍 nil（如 `ptr: null`）——
				// 已标记 fpath，无需再进子字段。
				continue
			}
			markSetFromYAML(fv.Elem(), child, fpath, set)
		case reflect.Slice:
			elems, ok := child.([]any)
			if !ok {
				continue
			}
			elem := sf.Type.Elem()
			// 按 YAML 元素个数遍历：set 标记只关心「YAML 里写没写」，
			// 不依赖 struct 切片当前长度（未填充时 fv.Len() 为 0）。
			for j := 0; j < len(elems); j++ {
				epath := fmt.Sprintf("%s[%d]", fpath, j)
				switch {
				case elem.Kind() == reflect.Struct:
					set[epath] = true
					var ev reflect.Value
					if j < fv.Len() {
						ev = fv.Index(j)
					} else {
						ev = reflect.New(elem).Elem() // 零值仅用于按类型下推路径
					}
					markSetFromYAML(ev, elems[j], epath, set)
				case elem.Kind() == reflect.Pointer && elem.Elem().Kind() == reflect.Struct:
					set[epath] = true
					if j < fv.Len() && !fv.Index(j).IsNil() {
						markSetFromYAML(fv.Index(j).Elem(), elems[j], epath, set)
					} else {
						markSetFromYAML(reflect.New(elem.Elem()).Elem(), elems[j], epath, set)
					}
				}
			}
		}
	}
}

// yamlKey 取字段的 YAML 序列化名：`yaml:"foo"` → foo；缺省与 yaml.v3 一致取
// 字段名全小写；`yaml:"-"` 或空名返回空串表示不参与。
func yamlKey(sf reflect.StructField) string {
	tag, ok := sf.Tag.Lookup("yaml")
	if !ok {
		return strings.ToLower(sf.Name)
	}
	name, _, _ := strings.Cut(tag, ",")
	if name == "-" {
		return ""
	}
	if name == "" {
		return strings.ToLower(sf.Name)
	}
	return name
}

// yamlInline 报告字段是否带 `yaml:",inline"` 选项。
func yamlInline(sf reflect.StructField) bool {
	tag, ok := sf.Tag.Lookup("yaml")
	if !ok {
		return false
	}
	for _, opt := range strings.Split(tag, ",")[1:] {
		if opt == "inline" {
			return true
		}
	}
	return false
}

// derivedEnvName 由 yaml key（缺省则字段名）推导「约定 env 名」：
// max_open / MaxOpen / backupArchiveFormat → MAX_OPEN / MAX_OPEN /
// BACKUP_ARCHIVE_FORMAT。用于 env tag 拼写冒烟检查。
func derivedEnvName(sf reflect.StructField) string {
	return toScreamingSnake(yamlKey(sf))
}

// toScreamingSnake 把标识符转换为 SCREAMING_SNAKE_CASE。
func toScreamingSnake(s string) string {
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "-", "_")
	runes := []rune(s)
	var b strings.Builder
	for i, r := range runes {
		if r == '_' {
			b.WriteByte('_')
			continue
		}
		if unicode.IsUpper(r) && i > 0 {
			p := runes[i-1]
			if unicode.IsLower(p) || unicode.IsDigit(p) {
				b.WriteByte('_')
			}
		}
		b.WriteRune(unicode.ToUpper(r))
	}
	return b.String()
}
