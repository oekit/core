package config

import (
	"fmt"
	"log/slog"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	pkgconfig "github.com/oekit/pkg/config"
)

// targetTagNames 是本包支持的独立结构体 tag 集合（与 yaml/merge tag 互不冲突），
// 供按需分配深层 *struct 时快速判断"该结构体里有没有值得填充的字段"。
var targetTagNames = [...]string{"default", "env", "range", "options", "require"}

// envNamePattern 限定 env tag 变量名形如 POSIX 标识符（大写风格不强制，
// 但禁止空格/点号等会导致 os.LookupEnv 永远 miss 的拼写）。
var envNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// FillDefaults 依据 default tag 为零值字段填充默认值，非零字段一律不动。
//
// 优先级约定（与 LoadBootstrap 内 ApplyEnv → FillDefaults 的接线顺序配合）：
// yaml 显式值 > env tag > default tag。
//
// 零值歧义：字段当前的 0/""/false/nil 切片与"未设置"对反射不可区分
// （判定走 stdlib reflect.Value.IsZero，core#133；v0.15.0 起**非 nil 空切片**
// 不再算零值——显式 `field: []` 不会被 default 覆盖、不会被 env 覆盖，退化
// 校验下也算「已提供」）。yaml 里显式写 `port: 0` 时，**不带 FieldSet** 调用
// 本函数仍会被 default 覆盖（历史行为）；需要保留显式零值请传入 WithFieldSet
// （LoadBootstrap / ApplyTagPasses 已默认接线），或对该字段打 require tag
// （default 永不填充，必须由 yaml/env 提供）。
// 整份配置都要区分零值时，少量字段用指针类型仍可行；改造成本过高则用
// FieldSet，而不是全量 *T。
//
// 带 require tag 的字段本函数永不填充（default 不算「提供」）。
//
// 支持类型：string、int*/uint*/float*、bool、time.Duration（time.ParseDuration
// 文法，如 "5s"）、[]string（逗号切分，元素去首尾空格并丢弃空元素）；标量值
// 解析前同样去除首尾空格。其他类型携带 default tag 视为配置错误并报错。
//
// 遍历范围：嵌套 struct、*struct（nil 时若子结构体携带目标 tag 则按需分配后
// 继续填充）、[]struct / []*struct（仅已存在的元素）；不遍历 map，跳过非导出字段。
//
// v 必须是非 nil 的结构体指针，否则返回错误。
func FillDefaults(v any, opts ...PassOption) error {
	rv, err := targetStructValue(v, "FillDefaults")
	if err != nil {
		return err
	}
	cfg := buildPassConfig(opts)
	fill := func(sf reflect.StructField, fv reflect.Value, path, _ string, _ bool) error {
		return fillDefaultField(sf, fv, path, cfg.set)
	}
	if err := walkTaggedStruct(rv, "", "", true, true, fill); err != nil {
		return fmt.Errorf("config: FillDefaults: %w", err)
	}
	return nil
}

// ApplyEnv 依据 env tag 把环境变量写入字段：env 已设置且字段当前为零值时才填，
// 已有值（yaml 显式设置）不覆盖。
//
// 优先级约定：yaml 显式值 > env tag > default tag——LoadBootstrap 先 ApplyEnv 后
// FillDefaults，env 填过的非零字段不会再被 default 覆盖。
//
// 带 FieldSet 时，YAML 显式提供的字段（含显式零值）不会被 env 覆盖；env 成功
// 写入会补记进 FieldSet，避免后续 FillDefaults 用 default 冲掉 env 值。
//
// env 设为空串（或纯空白）视同未设置；env 值无法按字段类型解析时 fail-closed
// 报错（错误信息携带字段路径与变量名）——可传 WithLenientEnv 转为 WARN + 跳过
// （见 fieldset.go）。**字符串字段写入原始 env 值**（core#88）：首尾空白是
// 口令/token 的合法内容，不做裁剪——与 ${VAR} 文本替换路径同值；非字符串
// 类型解析前仍裁剪（数值/布尔/duration 文法不容空白）。传 WithTrimStrings
// （#104）可开启裁剪档：字符串字段写入前裁掉首尾空白（部署侧显式补档，
// 边界见 fieldset.go）。支持类型与遍历范围同 FillDefaults。
//
// v 必须是非 nil 的结构体指针，否则返回错误。
func ApplyEnv(v any, opts ...PassOption) error {
	rv, err := targetStructValue(v, "ApplyEnv")
	if err != nil {
		return err
	}
	cfg := buildPassConfig(opts)
	apply := func(sf reflect.StructField, fv reflect.Value, path, yamlPath string, addr bool) error {
		return applyEnvField(sf, fv, path, yamlPath, addr, cfg.set, cfg.envNaming, cfg.lenientEnv, cfg.trimEnvStrings)
	}
	if err := walkTaggedStruct(rv, "", "", true, true, apply); err != nil {
		return fmt.Errorf("config: ApplyEnv: %w", err)
	}
	return nil
}

// ValidateTags 依据 range / options tag 做 fail-closed 校验，只读不写；
// 同时执行 require 与 env tag 卫生检查。
//
//   - range：仅数值字段（含 time.Duration，边界用 duration 文法如 "(0,30s]"）。
//     文法：`[a,b]`/`(a,b)`/`[a,b)`/`(a,b]` 四种区间 + 单端开区间 `[a,)`/`(,b]`，
//     区间**必须以 `[` 或 `(` 开头、以 `]` 或 `)` 结尾**——`]a,b[` 这类反向括号
//     写法不支持（缺省侧以空边界表达），边界支持小数；tag 文法非法或值越界
//     均报错，错误信息携带字段路径（嵌套用 `.` 连接，切片元素用 `Items[1].Qty`）。
//   - options：仅 string 字段的枚举校验；当前值（含空串）必须命中枚举之一，
//     可选枚举字段请配合 default tag 给默认值。
//   - require：字段必须由 yaml / env 显式提供（default 不算）。带 FieldSet 时
//     以集合为准（显式零值算通过）；不带 FieldSet 时退化为「最终值非零」。
//   - env 卫生（静态）：变量名须为 `[A-Za-z_][A-Za-z0-9_]*`；同一次校验内重复的
//     env 名报错。不读进程环境——与环境相关的检查会误报 PATH/HOME 等系统变量、
//     且漏掉无人设置的真拼写错误（core#8），见 LintEnvNaming / WithEnvNamingLint。
//
// 与 FillDefaults/ApplyEnv 配合使用时放在最后（LoadBootstrap 接线顺序
// ApplyEnv → FillDefaults → ValidateTags），校验的是填充后的最终值。
//
// v 必须是非 nil 的结构体指针，否则返回错误。
func ValidateTags(v any, opts ...PassOption) error {
	rv, err := targetStructValue(v, "ValidateTags")
	if err != nil {
		return err
	}
	cfg := buildPassConfig(opts)
	if err := checkEnvNaming(cfg.envNaming); err != nil {
		return fmt.Errorf("config: ValidateTags: %w", err)
	}
	envSeen := map[string]envSeenEntry{} // env name -> first field
	validate := func(sf reflect.StructField, fv reflect.Value, path, yamlPath string, addr bool) error {
		return validateTagFields(sf, fv, path, yamlPath, addr, cfg.set, envSeen, cfg.envNamingLint, cfg.envNaming)
	}
	// 只读校验，不分配：nil 的 *struct 里没有可校验的值。
	if err := walkTaggedStruct(rv, "", "", true, false, validate); err != nil {
		return fmt.Errorf("config: ValidateTags: %w", err)
	}
	return nil
}

// applyTagPasses 是 LoadBootstrap 的接线点：yaml unmarshal 成功后依次执行
// ApplyEnv → FillDefaults → ValidateTags（优先级 yaml 显式值 > env > default），
// 并带 FieldSet 保留 YAML 显式零值。非结构体配置类型（如 map）不适用 struct
// tag，直接跳过以保持既有行为。
//
// o 携带 env 命名策略（前缀 / 逐字段覆盖，见 envnaming.go）与 env 值裁剪开关
// （WithEnvTrimStrings，#104）。
func applyTagPasses(cfg any, yamlData []byte, o loadOptions) error {
	rv := reflect.ValueOf(cfg)
	if rv.Kind() != reflect.Pointer || rv.IsNil() || rv.Elem().Kind() != reflect.Struct {
		return nil
	}
	set, err := FieldSetFromYAML(cfg, yamlData)
	if err != nil {
		return err
	}
	envOpts := make([]PassOption, 0, 3)
	envOpts = append(envOpts, WithFieldSet(set), WithEnvNaming(o.envNaming))
	if o.trimEnvStrings {
		envOpts = append(envOpts, WithTrimStrings())
	}
	if err := ApplyEnv(cfg, envOpts...); err != nil {
		return err
	}
	if err := FillDefaults(cfg, WithFieldSet(set)); err != nil {
		return err
	}
	return ValidateTags(cfg, WithFieldSet(set), WithEnvNaming(o.envNaming))
}

// ApplyTagPasses 对任意结构体指针执行完整 tag 接线：先按 yamlData 构建
// FieldSet，再 ApplyEnv → FillDefaults → ValidateTags。与 LoadBootstrap 内部
// 同一套语义；热更 / 手写 loader 可直接复用。
//
// 可传 WithEnvNaming 启用 env 命名策略（前缀 / 逐字段覆盖）；可传
// WithTrimStrings 使 ApplyEnv 裁剪 env 值首尾空白（#104）。
func ApplyTagPasses(cfg any, yamlData []byte, opts ...PassOption) error {
	pc := buildPassConfig(opts)
	return applyTagPasses(cfg, yamlData, loadOptions{envNaming: pc.envNaming, trimEnvStrings: pc.trimEnvStrings})
}

// targetStructValue 校验 v 是非 nil 的结构体指针并返回其 Elem()，
// fn 用于错误信息里标明调用方（FillDefaults/ApplyEnv/ValidateTags）。
func targetStructValue(v any, fn string) (reflect.Value, error) {
	if v == nil {
		return reflect.Value{}, fmt.Errorf("config: %s: target must be a non-nil pointer to struct, got nil", fn)
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() || rv.Elem().Kind() != reflect.Struct {
		return reflect.Value{}, fmt.Errorf("config: %s: target must be a non-nil pointer to struct, got %T", fn, v)
	}
	return rv.Elem(), nil
}

// walkTaggedStruct 深度优先遍历结构体的导出字段，对每个字段调用 visit，
// 并递归进入嵌套 struct、*struct、[]struct / []*struct；不遍历 map。
//
// alloc 为 true 时，nil 的 *struct 字段在子结构体携带目标 tag 的前提下按需
// 分配后再递归（让 default/env 填充能到达深层）；alloc 为 false（只读 pass）
// 跳过 nil 指针。路径格式：`Server.Port`、`Items[0].Qty`。
//
// visit 同时收到该字段的 **yaml 键路径**（yamlPath）与**可寻址标记**（addr），
// 口径与严格模式诊断（UnknownKeyError.Path）一致：嵌套用 `.` 连接、切片元素
// 不编号（`items.qty` 而非 `items[0].qty`）、`yaml:",inline"` 的子字段拍平到
// 父级、`yaml:"-"` 的字段及其子树不可寻址（addr=false）。
// env 命名策略（覆盖表 / 前缀）按 yaml 键路径寻址，故需要这条并行口径。
//
// v 必须是可寻址的 struct 值（来自指针 Elem 或切片元素）。
func walkTaggedStruct(v reflect.Value, path, yamlPath string, yamlAddr bool, alloc bool, visit func(reflect.StructField, reflect.Value, string, string, bool) error) error {
	t := v.Type()
	for i := range t.NumField() {
		sf := t.Field(i)
		if !sf.IsExported() {
			continue // 非导出字段不可 Set，也不属于配置面
		}
		fv := v.Field(i)
		fpath := sf.Name
		if path != "" {
			fpath = path + "." + sf.Name
		}
		// yaml 键路径与可寻址性：inline 拍平到父级、yaml:"-" 不可寻址。
		fyaml, faddr, childYaml, childAddr := yamlChildPath(sf, yamlPath, yamlAddr)
		if err := visit(sf, fv, fpath, fyaml, faddr); err != nil {
			return err
		}
		switch sf.Type.Kind() {
		case reflect.Struct:
			if err := walkTaggedStruct(fv, fpath, childYaml, childAddr, alloc, visit); err != nil {
				return err
			}
		case reflect.Pointer:
			if sf.Type.Elem().Kind() != reflect.Struct {
				continue
			}
			if fv.IsNil() {
				if !alloc || !structHasTargetTags(sf.Type.Elem()) {
					continue
				}
				fv.Set(reflect.New(sf.Type.Elem()))
			}
			if err := walkTaggedStruct(fv.Elem(), fpath, childYaml, childAddr, alloc, visit); err != nil {
				return err
			}
		case reflect.Slice:
			elem := sf.Type.Elem()
			switch {
			case elem.Kind() == reflect.Struct:
				for j := range fv.Len() {
					// yaml 路径不编号元素（与严格模式同口径），Go 路径照旧带下标。
					if err := walkTaggedStruct(fv.Index(j), fmt.Sprintf("%s[%d]", fpath, j), childYaml, childAddr, alloc, visit); err != nil {
						return err
					}
				}
			case elem.Kind() == reflect.Pointer && elem.Elem().Kind() == reflect.Struct:
				for j := range fv.Len() {
					ev := fv.Index(j)
					if ev.IsNil() {
						continue
					}
					if err := walkTaggedStruct(ev.Elem(), fmt.Sprintf("%s[%d]", fpath, j), childYaml, childAddr, alloc, visit); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// yamlChildPath 依据 yaml tag 推出子字段的 yaml 键路径口径（与严格模式诊断
// UnknownKeyError.Path 一致）。
//
// 返回四项：
//
//   - fyaml：字段自身的 yaml 键路径（父路径 + yaml 键）；
//   - faddr：字段是否可按 yaml 键路径寻址——`yaml:"-"` 的字段及其子树不可寻址
//     （它们不出现在 yaml 里，也就没有对应的键路径）；`yaml:",inline"` 的字段
//     **自身**也不可寻址（它被拍平进父级，没有自己的键），但它的**子字段**可寻址
//     （路径 = 父路径 + 子字段自己的键）；
//   - childYaml / childAddr：字段**子结构体**的 yaml 路径基准与可寻址性——
//     inline 字段拍平到父级（子字段直接挂在父路径下，与 yaml.v3 的 inline
//     语义一致）；`yaml:"-"` 的子树整体不可寻址。
func yamlChildPath(sf reflect.StructField, parentYaml string, parentAddr bool) (fyaml string, faddr bool, childYaml string, childAddr bool) {
	key := yamlKey(sf)
	if key == "" {
		// yaml:"-"：该字段不出现在 yaml 里，键路径不可寻址；子树同样不可寻址。
		return "", false, "", false
	}
	if !parentAddr {
		// 父级不可寻址（yaml:"-" 子树）→ 子级同样不可寻址。
		return "", false, "", false
	}
	if yamlInline(sf) {
		// inline：字段自身没有 yaml 键（拍平进父级），子字段挂到父路径下。
		return parentYaml, false, parentYaml, true
	}
	fyaml = key
	if parentYaml != "" {
		fyaml = parentYaml + "." + key
	}
	return fyaml, true, fyaml, true
}

// structHasTargetTags 递归判断结构体类型（含嵌套 struct/*struct/[]struct）中
// 是否存在任一目标 tag 的字段；seen 防御递归类型。
func structHasTargetTags(t reflect.Type) bool {
	return typeHasTargetTags(t, map[reflect.Type]bool{})
}
func typeHasTargetTags(t reflect.Type, seen map[reflect.Type]bool) bool {
	if t == nil || seen[t] {
		return false
	}
	seen[t] = true
	if t.Kind() != reflect.Struct {
		return false
	}
	for i := range t.NumField() {
		sf := t.Field(i)
		for _, name := range targetTagNames {
			if _, ok := sf.Tag.Lookup(name); ok {
				return true
			}
		}
		switch sf.Type.Kind() {
		case reflect.Struct:
			if typeHasTargetTags(sf.Type, seen) {
				return true
			}
		case reflect.Pointer:
			if sf.Type.Elem().Kind() == reflect.Struct && typeHasTargetTags(sf.Type.Elem(), seen) {
				return true
			}
		case reflect.Slice:
			elem := sf.Type.Elem()
			if elem.Kind() == reflect.Struct && typeHasTargetTags(elem, seen) {
				return true
			}
			if elem.Kind() == reflect.Pointer && elem.Elem().Kind() == reflect.Struct && typeHasTargetTags(elem.Elem(), seen) {
				return true
			}
		}
	}
	return false
}

// fillDefaultField 是 FillDefaults 的单字段处理：有 default tag 且字段为零值
// 时，按字段类型解析并写入。require 字段永不走 default；FieldSet 命中的路径
// （yaml 显式提供）同样跳过。
func fillDefaultField(sf reflect.StructField, fv reflect.Value, path string, set FieldSet) error {
	if set.Has(path) {
		return nil // yaml 显式提供（含显式零值）
	}
	if _, ok := sf.Tag.Lookup("require"); ok {
		return nil // require：必须由 yaml/env 提供，default 不算
	}
	def, ok := sf.Tag.Lookup("default")
	if !ok || def == "" {
		return nil
	}
	if !fv.IsZero() {
		return nil // 已有显式值（yaml/env）不覆盖
	}
	return setScalarFromString(fv, def, fmt.Sprintf("default %q", def), path)
}

// applyEnvField 是 ApplyEnv 的单字段处理：env 已设置（非空）且字段为零值时，
// 按字段类型解析环境变量值并写入。FieldSet 命中的路径（yaml 显式提供）不覆盖；
// 成功写入后补记进 FieldSet。lenient 开启时解析失败 WARN + 跳过（不补记，
// core#51 宽容档），缺省 fail-closed；trim 开启时字符串字段写入前裁掉首尾
// 空白（#104 WithTrimStrings），缺省按 core#88 写原始值。
//
// env 名按 EnvNaming 的顺序解析（覆盖表 → env tag → 路径派生名，见 envnaming.go）：
// 覆盖表命中即用表里的绝对名；否则用 env tag（有前缀则加前缀）；都没有且设了
// 前缀时按 yaml 键路径派生名。
func applyEnvField(sf reflect.StructField, fv reflect.Value, path, yamlPath string, addr bool, set FieldSet, n EnvNaming, lenient, trim bool) error {
	binding := resolveEnvName(sf, yamlPath, addr, n)
	if binding.name == "" {
		return nil
	}
	if set.Has(path) {
		return nil // yaml 显式值优先（含显式零值）
	}
	raw, ok := os.LookupEnv(binding.name)
	if !ok {
		return nil
	}
	if strings.TrimSpace(raw) == "" {
		return nil // 判定用裁剪值：显式设为空串/纯空白视同未设置
	}
	if !fv.IsZero() {
		return nil // yaml 显式值优先
	}
	// 写入用**原始值**（core#88）：字符串字段保留首尾空白——口令/token/
	// 签名串两端空格可能是合法内容，静默裁剪会让认证失败现场与配置现场
	// 对不上；${VAR} 文本替换路径本就不裁剪，两条路径须同值。非字符串类型
	// 解析前仍裁剪（数值/布尔/duration 文法不容空白，裁剪是解析容错）。
	// 裁剪档（#104 WithTrimStrings）例外：调用方显式声明部署空白皆意外，
	// 字符串字段写入前裁掉首尾空白。
	if fv.Kind() == reflect.String {
		if trim {
			raw = strings.TrimSpace(raw)
		}
		fv.SetString(raw)
		if set != nil {
			set[path] = true
		}
		return nil
	}
	if err := setScalarFromString(fv, raw, fmt.Sprintf("env %s=%q", binding.name, raw), path); err != nil {
		if lenient {
			// 宽容档：跳过而非拒载——字段保持零值（default 可兜底）、
			// FieldSet 不补记（不算「已提供」）。
			slog.Default().With("component", "config.env").Warn(
				"skip unparsable env value (lenient mode)",
				"field", path, "env", binding.name, "err", err.Error())
			return nil
		}
		return err
	}
	if set != nil {
		set[path] = true
	}
	return nil
}

// validateTagFields 是 ValidateTags 的单字段处理：依次执行 require、range、
// options 与 env 卫生检查。
func validateTagFields(sf reflect.StructField, fv reflect.Value, path, yamlPath string, addr bool, set FieldSet, envSeen map[string]envSeenEntry, envNamingLint bool, n EnvNaming) error {
	if err := validateRequireField(sf, fv, path, set, yamlPath, addr, n); err != nil {
		return err
	}
	if err := validateEnvField(sf, path, yamlPath, addr, envSeen, n); err != nil {
		return err
	}
	if envNamingLint {
		if err := validateEnvNaming(sf, path, yamlPath, addr, n); err != nil {
			return err
		}
	}
	if rng, ok := sf.Tag.Lookup("range"); ok && strings.TrimSpace(rng) != "" {
		if err := validateRangeField(fv, rng, path); err != nil {
			return err
		}
	}
	if opts, ok := sf.Tag.Lookup("options"); ok && strings.TrimSpace(opts) != "" {
		if err := validateOptionsField(fv, opts, path); err != nil {
			return err
		}
	}
	return nil
}

// envNameMismatch 是 env 命名一致性检查的**纯判定内核**（core#131：
// validateEnvNaming 与 LintEnvNaming 的 walk 闭包此前逐行同构、仅输出
// 不同——判定收敛到这一份，两个入口只负责格式化输出；命名策略再演进时
// lint 与硬校验的口径不会再分叉）。
//
// 判定：env tag 声明名与派生约定名不一致即命中（`ignorederived:"true"`
// 豁免、覆盖表命中豁免、无派生名或已一致不命中）。启用命名策略后约定名
// 口径随 EnvNaming 变化（见 envnaming.go）：设了前缀时按**路径级**派生名
// 比对；未启用策略口径与历史一致（叶子键派生名）。
//
// 返回 declared（声明名）与 derived（派生名）供输出格式化；mismatched 为
// false 时二者无意义。
func envNameMismatch(sf reflect.StructField, yamlPath string, addr bool, n EnvNaming) (declared, derived string, mismatched bool) {
	name, ok := sf.Tag.Lookup("env")
	if !ok || name == "" {
		return "", "", false
	}
	if _, skip := sf.Tag.Lookup("ignorederived"); skip {
		return "", "", false
	}
	// 覆盖表命中的字段：最终名由表决定，不参与派生名一致性检查。
	if addr && n.Overrides != nil {
		if _, hit := n.Overrides[yamlPath]; hit {
			return "", "", false
		}
	}
	der := derivedEnvName(sf)
	if n.Prefix != "" && addr {
		// 前缀策略下的约定名是**路径级**派生名（去前缀后比对）。
		der = derivedPathEnvName(yamlPath)
	}
	if der == "" || der == name {
		return "", "", false
	}
	return name, der, true
}

// validateEnvNaming 纯静态命名一致性：声明的 env 名应与由 yaml key（缺省字段名）
// 推导的约定名一致。有意自定义名请打 `ignorederived:"true"`。判定内核见
// envNameMismatch（core#131 收敛）。
func validateEnvNaming(sf reflect.StructField, path, yamlPath string, addr bool, n EnvNaming) error {
	name, der, mismatched := envNameMismatch(sf, yamlPath, addr, n)
	if !mismatched {
		return nil
	}
	return fmt.Errorf("field %q: env tag %q differs from derived name %q (intentional? add ignorederived:\"true\")", path, name, der)
}

// LintEnvNaming 静态扫描 env tag 与推导约定名不一致的字段，返回诊断行
// （不读进程环境、不 fail-closed）。适合 CI/lint 脚本或启动前自检。
// 有意自定义名请打 `ignorederived:"true"`。
//
// 可传 WithEnvNaming 启用命名策略口径：覆盖表命中的字段豁免；设了前缀则按
// **路径级**派生名比对（与运行时实际读的变量名同口径）。判定内核与
// validateEnvNaming 共用 envNameMismatch（core#131 收敛）。
func LintEnvNaming(v any, opts ...PassOption) ([]string, error) {
	rv, err := targetStructValue(v, "LintEnvNaming")
	if err != nil {
		return nil, err
	}
	n := buildPassConfig(opts).envNaming
	var out []string
	walk := func(sf reflect.StructField, _ reflect.Value, path, yamlPath string, addr bool) error {
		name, der, mismatched := envNameMismatch(sf, yamlPath, addr, n)
		if mismatched {
			out = append(out, fmt.Sprintf("field %q: env tag %q differs from derived name %q", path, name, der))
		}
		return nil
	}
	if err := walkTaggedStruct(rv, "", "", true, false, walk); err != nil {
		return nil, fmt.Errorf("config: LintEnvNaming: %w", err)
	}
	return out, nil
}

// validateRequireField 校验 require 字段确实由外部提供。带 FieldSet 时以
// 「yaml/env 显式」为准（显式零值算通过）；不带 FieldSet 退化为非零检查。
//
// 错误信息里的 env 变量名按命名策略解析后的**最终名**给出（见 envnaming.go）——
// 否则提示的名字不是真正生效的那个，等于把人指向错误的变量。
func validateRequireField(sf reflect.StructField, fv reflect.Value, path string, set FieldSet, yamlPath string, addr bool, n EnvNaming) error {
	if _, ok := sf.Tag.Lookup("require"); !ok {
		return nil
	}
	if set != nil {
		if set.Has(path) {
			return nil
		}
		source := "yaml"
		if binding := resolveEnvName(sf, yamlPath, addr, n); binding.name != "" {
			source = fmt.Sprintf("yaml or env %s", binding.name)
		}
		return fmt.Errorf("field %q: required but not provided (set %s)", path, source)
	}
	if fv.IsZero() {
		return fmt.Errorf("field %q: required but value is zero (provide via yaml or env before ValidateTags)", path)
	}
	return nil
}

// validateEnvField 校验 env 名的静态卫生：变量名格式、同名重复。
// 不读进程环境——依赖运行环境的“拼写冒烟检查”会误报 PATH/HOME 等系统变量、
// 且漏掉无人设置的真拼写错误（见 core#8），已改为可选的 LintEnvNaming。
//
// 校验对象是**最终生效的 env 名**（覆盖表 / 前缀 / tag 解析之后，见 envnaming.go）——
// 校验声明名而不校验生效名，等于放行了一个永远 miss 的变量名。
//
// 重名判定按 **yaml 键路径**去重：同一个 env 名落在**不同** yaml 路径上是歧义
// （两个字段抢同一个变量，必有一个静默拿不到值），报错；落在**同一** yaml 路径上
// 是同一逻辑字段的多个实例（切片元素），不报错——否则任何 `[]struct` 配置配上
// env tag 或前缀都会被误判。
func validateEnvField(sf reflect.StructField, path, yamlPath string, addr bool, envSeen map[string]envSeenEntry, n EnvNaming) error {
	binding := resolveEnvName(sf, yamlPath, addr, n)
	if binding.name == "" {
		return nil
	}
	if !envNamePattern.MatchString(binding.name) {
		return fmt.Errorf("field %q: env name %q is not a valid variable name ([A-Za-z_][A-Za-z0-9_]*)", path, binding.name)
	}
	if prev, dup := envSeen[binding.name]; dup {
		if prev.yamlPath == yamlPath {
			return nil // 同一逻辑字段的多个实例（切片元素）
		}
		return fmt.Errorf("field %q: env name %q already used by field %q", path, binding.name, prev.path)
	}
	envSeen[binding.name] = envSeenEntry{path: path, yamlPath: yamlPath}
	return nil
}

// envSeenEntry 记录某个 env 名首次出现的字段。
type envSeenEntry struct {
	// path 首次出现的 Go 字段路径（错误信息用）。
	path string
	// yamlPath 首次出现的 yaml 键路径（重名去重用——同一逻辑字段的多个实例不算重名）。
	yamlPath string
}

// setScalarFromString 把字符串 s 按字段类型解析后写入 fv。
// source 用于错误信息（如 `default "abc"` 或 `env PORT="abc"`）。
//
// 标量解析**单源走 pkg/config.SetFromString**（core#129）：与 pkg/config
// 引擎共享同一份「字符串按 reflect.Kind 解析落位」实现与错误口径，两套
// 平行实现（含 duration 的类型同一性识别）到此收敛——本包只在 pkg 口径
// 外包一层 field/source 前缀，错误文案由英文短句统一为 pkg 口径（changelog
// 已注明的行为面）。[]string 的逗号切分**保留在本包**：丢弃空段是本包
// tag 文法的成文契约（见 FillDefaults 的支持类型说明），与 pkg 原语的
// 通用切片语义（保留空段、支持数值元素）有意不同，不属重复实现。
func setScalarFromString(fv reflect.Value, s, source, path string) error {
	// 标量值解析前去首尾空白（成文契约）；字符串字段经 ApplyEnv 的旁路
	// （写原始值，core#88）不走本函数，FillDefaults 的字符串默认值在此裁剪。
	s = strings.TrimSpace(s)
	if fv.Kind() == reflect.Slice {
		if fv.Type().Elem().Kind() != reflect.String {
			return fmt.Errorf("field %q: %s cannot be applied to type %s", path, source, fv.Type())
		}
		parts := splitCommaList(s)
		out := reflect.MakeSlice(fv.Type(), len(parts), len(parts))
		for i, p := range parts {
			out.Index(i).SetString(p)
		}
		fv.Set(out)
		return nil
	}
	if err := pkgconfig.SetFromString(s, fv); err != nil {
		return fmt.Errorf("field %q: %s: %w", path, source, err)
	}
	return nil
}

// splitCommaList 逗号切分并去除每段首尾空格，丢弃空段（`"a, b"` → ["a","b"]）。
func splitCommaList(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// validateRangeField 校验数值字段（含 time.Duration）是否落在 range 区间内。
// Duration 的类型同一性判定单源在 pkg/config.DurationType（core#129）：
// 「这个字段是不是时长字段」全矩阵一份答案，标量解析与 range 文法不各判各的。
func validateRangeField(fv reflect.Value, rng, path string) error {
	isDur := fv.Type() == pkgconfig.DurationType
	if !isDur && !isNumericKind(fv.Kind()) {
		return fmt.Errorf("field %q: range tag only applies to numeric fields, got %s", path, fv.Type())
	}

	// Duration 字段边界用 duration 文法；普通数值字段边界用十进制数（支持小数）。
	parseBound := func(s string) (float64, error) {
		if isDur {
			d, err := time.ParseDuration(s)
			if err != nil {
				return 0, fmt.Errorf("invalid duration bound %q (duration fields need time.ParseDuration syntax like 30s)", s)
			}
			return float64(d), nil
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid numeric bound %q", s)
		}
		return f, nil
	}

	low, high, lowInc, highInc, err := parseInterval(rng, parseBound)
	if err != nil {
		return fmt.Errorf("field %q: invalid range tag %q: %w", path, rng, err)
	}

	var val float64
	switch {
	case isDur || isSignedKind(fv.Kind()):
		val = float64(fv.Int())
	case fv.Kind() == reflect.Float32 || fv.Kind() == reflect.Float64:
		val = fv.Float()
	default: // 无符号整数
		val = float64(fv.Uint())
	}

	outOfRange := (low != nil && ((lowInc && val < *low) || (!lowInc && val <= *low))) ||
		(high != nil && ((highInc && val > *high) || (!highInc && val >= *high)))
	if !outOfRange {
		return nil
	}
	valStr := strconv.FormatFloat(val, 'g', -1, 64)
	if isDur {
		valStr = time.Duration(int64(val)).String()
	}
	return fmt.Errorf("field %q: value %s out of range %s", path, valStr, strings.TrimSpace(rng))
}

// parseInterval 解析 range 文法 `[a,b]`/`(a,b)`/`[a,b)`/`(a,b]`/`[a,)`/`(,b]`，
// 返回上下界（nil 表示该侧无界）与两侧是否闭区间。
// 区间必须以 `[` 或 `(` 开头、以 `]` 或 `)` 结尾——`]a,b[` 这类反向括号写法
// 不支持（开侧用空边界表达，如 `[a,)`），报 "must start with '[' or '('"。
func parseInterval(rng string, parseBound func(string) (float64, error)) (low, high *float64, lowInc, highInc bool, err error) {
	s := strings.TrimSpace(rng)
	if len(s) < 2 {
		return nil, nil, false, false, fmt.Errorf("too short")
	}
	switch s[0] {
	case '[':
		lowInc = true
	case '(':
	default:
		return nil, nil, false, false, fmt.Errorf("must start with '[' or '('")
	}
	switch s[len(s)-1] {
	case ']':
		highInc = true
	case ')':
	default:
		return nil, nil, false, false, fmt.Errorf("must end with ']' or ')'")
	}

	parts := strings.Split(s[1:len(s)-1], ",")
	if len(parts) != 2 {
		return nil, nil, false, false, fmt.Errorf(`expected form "[a,b]", got %q`, s)
	}
	left := strings.TrimSpace(parts[0])
	right := strings.TrimSpace(parts[1])
	if left == "" && right == "" {
		return nil, nil, false, false, fmt.Errorf("both bounds are empty")
	}
	if left != "" {
		v, perr := parseBound(left)
		if perr != nil {
			return nil, nil, false, false, fmt.Errorf("lower bound: %w", perr)
		}
		low = &v
	}
	if right != "" {
		v, perr := parseBound(right)
		if perr != nil {
			return nil, nil, false, false, fmt.Errorf("upper bound: %w", perr)
		}
		high = &v
	}
	if low != nil && high != nil && *low > *high {
		return nil, nil, false, false, fmt.Errorf("lower bound %g greater than upper bound %g", *low, *high)
	}
	return low, high, lowInc, highInc, nil
}

// isSignedKind 判断是否符号整数 kind（含 time.Duration 的 Int64）。
func isSignedKind(k reflect.Kind) bool {
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return true
	default:
		return false
	}
}

// isNumericKind 判断是否数值 kind（符号/无符号整数 + 浮点）。
func isNumericKind(k reflect.Kind) bool {
	return isSignedKind(k) ||
		k == reflect.Uint || k == reflect.Uint8 || k == reflect.Uint16 || k == reflect.Uint32 || k == reflect.Uint64 ||
		k == reflect.Float32 || k == reflect.Float64
}

// validateOptionsField 校验 string 字段当前值是否命中 options 枚举之一
// （空串同样必须命中；可选枚举字段请配合 default tag）。
func validateOptionsField(fv reflect.Value, opts, path string) error {
	if fv.Kind() != reflect.String {
		return fmt.Errorf("field %q: options tag only applies to string fields, got %s", path, fv.Type())
	}
	allowed := splitCommaList(opts)
	if len(allowed) == 0 {
		return fmt.Errorf("field %q: options tag has no values", path)
	}
	val := fv.String()
	for _, a := range allowed {
		if val == a {
			return nil
		}
	}
	return fmt.Errorf("field %q: value %q not in options (%s)", path, val, strings.Join(allowed, ","))
}
