package config

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
)

// EnvNaming 描述 **env 变量名**的命名策略：全局前缀 + 逐字段覆盖。
//
// 现状（本包 v0.11.0 之前）：字段只有打了 `env:"NAME"` tag 才会读 env，名字由
// 结构体作者逐字段写死；「约定名」（由 yaml 键推导的 SCREAMING_SNAKE）只被
// LintEnvNaming / WithEnvNamingLint 用来**检查**名字是否一致，并不能**决定**
// 实际读的变量名。
//
// 于是两类宿主接不上：
//
//   - 用统一前缀的 12-factor 宿主（`PB_` / `APP_` / `SVC_`）：每个字段都得手写
//     `env:"PB_XXX"`，且**无法**让「没打 env tag 的字段」也按约定参与 env 读取；
//   - 历史 env 名与 yaml 键对不上、需保留兼容的宿主：只能在结构体 tag 里钉死，
//     结构体不是自己的就改不动。
//
// EnvNaming 补这两条入口。**名字解析顺序（先命中先赢）**：
//
//  1. **覆盖表**（Overrides，键为 yaml 键路径）：命中即用表里的名字——**绝对名**，
//     不加前缀；值为空串表示**该字段不走 env**。
//  2. **env tag**：字段显式声明了 `env:"NAME"` 就用它（加了前缀则前缀在前）。
//  3. **路径派生名**：没有显式名时按 yaml 键路径推导
//     （`upload.local_import.enabled` → `UPLOAD_LOCAL_IMPORT_ENABLED`）。
//
// 前缀（Prefix）加在**本包自己推导的名字**上（第 2、3 步），覆盖表里的名字不受
// 影响。默认（既无前缀也无覆盖表）行为与历史**逐字一致**：只有打了 env tag 的
// 字段读 env。
//
// 覆盖表的键是 **yaml 键路径**（与严格模式 UnknownKeyError.Path 同口径，便于
// 对账）：嵌套用 `.` 连接、切片元素不编号（`items.qty` 而非 `items[0].qty`）、
// `yaml:",inline"` 的子字段拍平到父级、`yaml:"-"` 的字段及其子树不可寻址。
type EnvNaming struct {
	// Prefix 全局前缀，加在 env tag 名与路径派生名之前。空串 = 不加前缀。
	// 例："PB_" 使 yaml 键 `upload.local_import.enabled` 读
	// `PB_UPLOAD_LOCAL_IMPORT_ENABLED`。
	Prefix string
	// Overrides 逐字段覆盖：键为 yaml 键路径，值为该字段的 env 变量名。
	// 值为空串表示该字段**不走 env**。nil / 空 map = 无覆盖。
	Overrides map[string]string
}

// envPrefixPattern 限定前缀只能含变量名字符——前缀会被拼进最终变量名，
// 含 `-` / `.` / 空格等字符会让 os.LookupEnv 永远 miss（静默失效）。
var envPrefixPattern = regexp.MustCompile(`^[A-Za-z0-9_]*$`)

// WithEnvNaming 给单独调用 ApplyEnv / ValidateTags / ApplyEnvOverlay 的场景
// 传命名策略（LoadOption 层的 WithEnvPrefix / WithEnvOverrides 是它的语法糖）。
//
// 典型用法是 12-factor 的 env 优先接线——yaml 填完后再用 env 覆盖：
//
//	_ = config.ApplyEnvOverlay(&cfg, config.WithEnvNaming(config.EnvNaming{
//	    Prefix: "PB_",
//	}))
func WithEnvNaming(n EnvNaming) PassOption {
	return func(c *passConfig) { c.envNaming = n }
}

// WithEnvPrefix 给 env 名加全局前缀——给「用统一前缀的 12-factor 宿主」用。
//
// 前缀加在**本包推导的名字**上（env tag 名与路径派生名），覆盖表里的名字是
// 绝对名不受影响。例：
//
//	type Cfg struct {
//	    Upload struct {
//	        LocalImport struct {
//	            Enabled bool `yaml:"enabled"`
//	        } `yaml:"local_import"`
//	    } `yaml:"upload"`
//	}
//	// WithEnvPrefix("PB_") 后：Enabled 读 PB_UPLOAD_LOCAL_IMPORT_ENABLED
//	cfg, err := config.LoadBootstrap[Cfg]("dev", config.WithEnvPrefix("PB_"))
//
// 前缀为空时行为与历史完全一致：**只有打了 env tag 的字段读 env**。加了前缀后
// 额外多一层能力——**没打 env tag 的字段也按路径派生名参与 env 读取**（如上例的
// Enabled），这正是「前缀即约定」的含义：宿主不必逐字段写 tag。
//
// 与 WithEnvOverrides 叠加时：覆盖表命中的字段用表里的绝对名，其余字段按本
// 前缀规则。
func WithEnvPrefix(prefix string) LoadOption {
	return func(o *loadOptions) { o.envNaming.Prefix = prefix }
}

// WithEnvOverrides 逐字段指定 env 变量名——给「历史 env 名与 yaml 键对不上、
// 需保留兼容」的宿主用。
//
// 键为 **yaml 键路径**（与严格模式诊断路径同口径）：嵌套用 `.` 连接、切片元素
// 不编号、`yaml:",inline"` 的子字段拍平到父级。值为空串表示**该字段不走 env**
// （显式豁免，压过 env tag 与前缀）。
//
//	WithEnvOverrides(map[string]string{
//	    "upload.local_import.enabled": "PB_LOCAL_IMPORT_ENABLED",
//	    "legacy.retries":              "", // 该字段不读 env
//	})
//
// 覆盖表**只影响命中的字段**：未命中的字段行为不变（仍按 env tag 读 env，没有
// tag 就不读）。覆盖优先于 env tag——表里写了就是表里说了算。
//
// 覆盖表里若有不存在的 yaml 键路径，该条目不生效（不报错）：同一张表可以跨多个
// 配置结构体共用。
func WithEnvOverrides(m map[string]string) LoadOption {
	return func(o *loadOptions) {
		if len(m) == 0 {
			return
		}
		if o.envNaming.Overrides == nil {
			o.envNaming.Overrides = make(map[string]string, len(m))
		}
		for k, v := range m {
			o.envNaming.Overrides[k] = v
		}
	}
}

// envBinding 是某字段的 env 绑定解析结果。
type envBinding struct {
	// name 最终 env 变量名；空串表示该字段不读 env。
	name string
	// explicit 该名字是否由调用方显式给出（env tag / 覆盖表）。
	// 路径派生名非显式——它由 yaml 键路径推导而来。
	explicit bool
	// fromOverride 该名字是否来自覆盖表（覆盖表是绝对名，不加前缀）。
	fromOverride bool
}

// resolveEnvName 解析某字段实际该读哪个 env 变量。
//
// yamlPath 是该字段的 yaml 键路径（严格模式同口径）；addr 为 false 表示该字段
// 没有 yaml 身份（`yaml:"-"` 及其子树）——不参与覆盖表与派生名，只认自己的
// env tag。顺序见 EnvNaming 文档。
func resolveEnvName(sf reflect.StructField, yamlPath string, addr bool, n EnvNaming) envBinding {
	// 1) 覆盖表：命中即用（绝对名，不加前缀）；空串 = 该字段不走 env。
	if addr && n.Overrides != nil {
		if v, ok := n.Overrides[yamlPath]; ok {
			if v == "" {
				return envBinding{}
			}
			return envBinding{name: v, explicit: true, fromOverride: true}
		}
	}
	// 2) env tag：显式声明优先于派生名。
	if tag, ok := sf.Tag.Lookup("env"); ok && tag != "" {
		return envBinding{name: n.Prefix + tag, explicit: true}
	}
	// 3) 路径派生名：仅在设了前缀时参与 env 读取——没前缀时历史行为是
	//    「没打 env tag 的字段不读 env」。
	if addr && n.Prefix != "" {
		if d := derivedPathEnvName(yamlPath); d != "" && envSettableType(sf.Type) {
			return envBinding{name: n.Prefix + d}
		}
	}
	return envBinding{}
}

// envSettableType 报告该字段类型能否从字符串解析赋值（setScalarFromString 的
// 支持集：string / int* / uint* / float* / bool / time.Duration / []string）。
//
// 用于**路径派生名**：派生名是自动生成的，没有调用方的显式确认——若让结构体、
// map 之类的类型也去读 env，一旦同名环境变量恰好被设置，就会报出
// 「cannot be applied to type」这种指向不明的错误。显式 env tag 不受此限
// （那是调用方自己写的，历史上照报）。
func envSettableType(t reflect.Type) bool {
	if t == durationType {
		return true
	}
	switch t.Kind() {
	case reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	case reflect.Slice:
		return t.Elem().Kind() == reflect.String
	default:
		return false
	}
}

// derivedPathEnvName 由 yaml 键路径推导约定 env 名：
// upload.local_import.enabled → UPLOAD_LOCAL_IMPORT_ENABLED。
//
// 与 fieldset.go 的 derivedEnvName（单字段叶子键 → SCREAMING_SNAKE）不同：
// 这里按**整条路径**推导，前缀策略下的约定名因此天然带路径层级——两个不同子
// 树下的同名字段不会撞名（叶子口径会撞）。
func derivedPathEnvName(yamlPath string) string {
	if yamlPath == "" {
		return ""
	}
	parts := strings.Split(yamlPath, ".")
	out := make([]string, 0, len(parts))
	for _, seg := range parts {
		if seg == "" {
			continue
		}
		out = append(out, toScreamingSnake(seg))
	}
	return strings.Join(out, "_")
}

// checkEnvNaming 静态校验命名策略本身：前缀字符合法、覆盖表里的名字是合法变量名。
//
// 非法名会让 os.LookupEnv 永远 miss（静默失效）——这正是本包一直在治的病，
// 所以在 ValidateTags 阶段 fail-closed，而不是等到运行期才发现「配了没生效」。
func checkEnvNaming(n EnvNaming) error {
	if n.Prefix != "" && !envPrefixPattern.MatchString(n.Prefix) {
		return fmt.Errorf("env prefix %q contains invalid characters (only [A-Za-z0-9_] allowed)", n.Prefix)
	}
	for path, name := range n.Overrides {
		if name == "" {
			continue // 显式不走 env
		}
		if !envNamePattern.MatchString(name) {
			return fmt.Errorf("env override %q: name %q is not a valid variable name ([A-Za-z_][A-Za-z0-9_]*)", path, name)
		}
	}
	return nil
}
