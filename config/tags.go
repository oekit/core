package config

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// durationType 用于识别 time.Duration 字段：Duration 与普通 int64 同 kind，
// 但取值走 time.ParseDuration 文法（default/env 值与 range 边界皆如此）。
var durationType = reflect.TypeOf(time.Duration(0))

// targetTagNames 是本包支持的独立结构体 tag 集合（与 yaml/merge tag 互不冲突），
// 供按需分配深层 *struct 时快速判断"该结构体里有没有值得填充的字段"。
var targetTagNames = [...]string{"default", "env", "range", "options", "required"}

// FillDefaults 依据 default tag 为零值字段填充默认值，非零字段一律不动。
//
// 优先级约定（与 LoadBootstrap 内 ApplyEnv → FillDefaults 的接线顺序配合）：
// yaml 显式值 > env tag > default tag。
//
// # 零值歧义与消解（#5）
//
// 单看字段值，0/""/false/空切片与"未设置"对反射不可区分。不带选项调用时沿用
// 「零值即未设置」判定：yaml 显式写 `port: 0` 会被 default 覆盖。要区分二者，
// 把来源信息传进来——WithExplicit(ExplicitFields)：集合内字段即便为零值也不填。
// LoadBootstrap 已自动采集（yaml 出现的键 + 已设置的 env），调用方零改动；自建
// 加载流程用 ExplicitFromYAML / ExplicitFromEnv 采集后传入：
//
//	explicit, _ := config.ExplicitFromYAML(data, &cfg)
//	envSet, _ := config.ExplicitFromEnv(&cfg)
//	_ = config.ApplyEnv(&cfg, config.WithExplicit(explicit))
//	explicit.Merge(envSet)
//	_ = config.FillDefaults(&cfg, config.WithExplicit(explicit))
//
// 指针类型（*int/*bool）也能区分，但只适合少量字段；整份配置都要区分时改指针会让
// 每个消费方多写解引用、配置可读性塌掉——用上面的来源标记，或把「必须外部提供、
// 不吃 default」的字段标 `required:"true"`（见 CheckRequired）。
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
	pc := newPassConfig(opts)
	if err := walkTaggedStruct(rv, "", true, fillDefaultVisitor(pc.explicit)); err != nil {
		return fmt.Errorf("config: FillDefaults: %w", err)
	}
	return nil
}

// ApplyEnv 依据 env tag 把环境变量写入字段：env 已设置且字段当前为零值时才填，
// 已有值（yaml 显式设置）不覆盖。
//
// 优先级约定：yaml 显式值 > env tag > default tag——LoadBootstrap 先 ApplyEnv 后
// FillDefaults，env 填过的非零字段不会再被 default 覆盖。传入 WithExplicit（yaml
// 已显式出现的字段）时，这些字段即便为零值也不写 env——yaml 显式零值 > env，
// 与上述优先级一致（不传则退回「零值即未设置」判定）。
//
// env 设为空串（或纯空白）视同未设置；env 值无法按字段类型解析时 fail-closed
// 报错（错误信息携带字段路径与变量名）。支持类型与遍历范围同 FillDefaults。
//
// v 必须是非 nil 的结构体指针，否则返回错误。
func ApplyEnv(v any, opts ...PassOption) error {
	rv, err := targetStructValue(v, "ApplyEnv")
	if err != nil {
		return err
	}
	pc := newPassConfig(opts)
	if err := walkTaggedStruct(rv, "", true, applyEnvVisitor(pc.explicit)); err != nil {
		return fmt.Errorf("config: ApplyEnv: %w", err)
	}
	return nil
}

// ValidateTags 依据 range / options tag 做 fail-closed 校验，只读不写；并顺带
// 校验 env tag 文法（含空白或 '=' 的变量名永远匹配不到环境，属拼写错误）。
//
//   - range：仅数值字段（含 time.Duration，边界用 duration 文法如 "(0,30s]"）。
//     文法：`[a,b]`/`(a,b)`/`[a,b)`/`(a,b]` 四种区间 + 单端开区间 `[a,)`/`(,b]`
//     （开侧方括号方向不敏感），边界支持小数；tag 文法非法或值越界均报错，
//     错误信息携带字段路径（嵌套用 `.` 连接，切片元素用 `Items[1].Qty`）。
//   - options：仅 string 字段的枚举校验；当前值（含空串）必须命中枚举之一，
//     可选枚举字段请配合 default tag 给默认值。
//   - env：变量名不得含空白或 '='。拼成另一个合法名字（BACKUP_ARCHIVE_ARCHIVE_FORMAT）
//     静态查不出来——用 CheckEnvOrphans 在启动期按前缀抓「环境里有、没人消费」的孤儿变量。
//
// 与 FillDefaults/ApplyEnv 配合使用时放在最后（LoadBootstrap 接线顺序
// ApplyEnv → FillDefaults → CheckRequired → ValidateTags），校验的是填充后的最终值。
//
// v 必须是非 nil 的结构体指针，否则返回错误。
func ValidateTags(v any) error {
	rv, err := targetStructValue(v, "ValidateTags")
	if err != nil {
		return err
	}
	// 只读校验，不分配：nil 的 *struct 里没有可校验的值。
	if err := walkTaggedStruct(rv, "", false, validateTagFields); err != nil {
		return fmt.Errorf("config: ValidateTags: %w", err)
	}
	return nil
}

// applyTagPasses 是 LoadBootstrap 的接线点：yaml unmarshal 成功后依次执行
// ApplyEnv → FillDefaults → CheckRequired → ValidateTags（优先级 yaml 显式值 >
// env > default）。data 为原始 yaml 文档，用于采集显式出现的字段（#5：yaml 显式
// 零值不再被 env/default 覆盖）；env 已设置的字段同样记为显式，default 不再覆盖。
// 非结构体配置类型（如 map）不适用 struct tag，直接跳过以保持既有行为。
func applyTagPasses(cfg any, data []byte) error {
	rv := reflect.ValueOf(cfg)
	if rv.Kind() != reflect.Pointer || rv.IsNil() || rv.Elem().Kind() != reflect.Struct {
		return nil
	}
	explicit, err := ExplicitFromYAML(data, cfg)
	if err != nil {
		return err
	}
	envSet, err := ExplicitFromEnv(cfg)
	if err != nil {
		return err
	}
	if err := ApplyEnv(cfg, WithExplicit(explicit)); err != nil {
		return err
	}
	explicit.Merge(envSet)
	if err := FillDefaults(cfg, WithExplicit(explicit)); err != nil {
		return err
	}
	if err := CheckRequired(cfg, explicit); err != nil {
		return err
	}
	return ValidateTags(cfg)
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
// v 必须是可寻址的 struct 值（来自指针 Elem 或切片元素）。
func walkTaggedStruct(v reflect.Value, path string, alloc bool, visit func(reflect.StructField, reflect.Value, string) error) error {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if !sf.IsExported() {
			continue // 非导出字段不可 Set，也不属于配置面
		}
		fv := v.Field(i)
		fpath := sf.Name
		if path != "" {
			fpath = path + "." + sf.Name
		}
		if err := visit(sf, fv, fpath); err != nil {
			return err
		}
		switch sf.Type.Kind() {
		case reflect.Struct:
			if err := walkTaggedStruct(fv, fpath, alloc, visit); err != nil {
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
			if err := walkTaggedStruct(fv.Elem(), fpath, alloc, visit); err != nil {
				return err
			}
		case reflect.Slice:
			elem := sf.Type.Elem()
			switch {
			case elem.Kind() == reflect.Struct:
				for j := 0; j < fv.Len(); j++ {
					if err := walkTaggedStruct(fv.Index(j), fmt.Sprintf("%s[%d]", fpath, j), alloc, visit); err != nil {
						return err
					}
				}
			case elem.Kind() == reflect.Pointer && elem.Elem().Kind() == reflect.Struct:
				for j := 0; j < fv.Len(); j++ {
					ev := fv.Index(j)
					if ev.IsNil() {
						continue
					}
					if err := walkTaggedStruct(ev.Elem(), fmt.Sprintf("%s[%d]", fpath, j), alloc, visit); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
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
	for i := 0; i < t.NumField(); i++ {
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

// fillDefaultVisitor 返回 FillDefaults 的单字段处理：有 default tag 且字段为零值
// 且未被来源显式提供时，按字段类型解析并写入。
func fillDefaultVisitor(explicit ExplicitFields) func(reflect.StructField, reflect.Value, string) error {
	return func(sf reflect.StructField, fv reflect.Value, path string) error {
		def, ok := sf.Tag.Lookup("default")
		if !ok || def == "" {
			return nil
		}
		if !isZeroValue(fv) {
			return nil // 已有显式值（yaml/env）不覆盖
		}
		if explicit.Has(path) {
			return nil // 来源显式给了零值（yaml port: 0 / env X=0），不是"未设置"
		}
		return setScalarFromString(fv, def, fmt.Sprintf("default %q", def), path)
	}
}

// applyEnvVisitor 返回 ApplyEnv 的单字段处理：env 已设置（非空）且字段为零值
// 且未被 yaml 显式提供时，按字段类型解析环境变量值并写入。
func applyEnvVisitor(explicit ExplicitFields) func(reflect.StructField, reflect.Value, string) error {
	return func(sf reflect.StructField, fv reflect.Value, path string) error {
		name, ok := sf.Tag.Lookup("env")
		if !ok || name == "" {
			return nil
		}
		raw, ok := os.LookupEnv(name)
		if !ok {
			return nil
		}
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return nil // 显式设为空串视同未设置
		}
		if !isZeroValue(fv) {
			return nil // yaml 显式值优先
		}
		if explicit.Has(path) {
			return nil // yaml 显式零值同样优先于 env
		}
		return setScalarFromString(fv, raw, fmt.Sprintf("env %s=%q", name, raw), path)
	}
}

// validateTagFields 是 ValidateTags 的单字段处理：依次执行 env 文法、range 与
// options 三类校验（range/options 按字段类型天然互斥，同时滥用会收到其中之一的类型错误）。
func validateTagFields(sf reflect.StructField, fv reflect.Value, path string) error {
	if name, ok := sf.Tag.Lookup("env"); ok && name != "" {
		if strings.ContainsAny(name, " \t\r\n=") {
			return fmt.Errorf("field %q: invalid env tag %q (variable name must not contain whitespace or '=')", path, name)
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

// setScalarFromString 把字符串 s 按字段类型解析后写入 fv。
// source 用于错误信息（如 `default "abc"` 或 `env PORT="abc"`）。
func setScalarFromString(fv reflect.Value, s, source, path string) error {
	s = strings.TrimSpace(s)
	if fv.Type() == durationType {
		d, err := time.ParseDuration(s)
		if err != nil {
			return fmt.Errorf("field %q: %s is not a valid duration: %w", path, source, err)
		}
		fv.SetInt(int64(d))
		return nil
	}
	switch fv.Kind() {
	case reflect.String:
		fv.SetString(s)
	case reflect.Bool:
		b, err := strconv.ParseBool(s)
		if err != nil {
			return fmt.Errorf("field %q: %s is not a valid bool: %w", path, source, err)
		}
		fv.SetBool(b)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(s, 10, fv.Type().Bits())
		if err != nil {
			return fmt.Errorf("field %q: %s is not a valid int: %w", path, source, err)
		}
		fv.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(s, 10, fv.Type().Bits())
		if err != nil {
			return fmt.Errorf("field %q: %s is not a valid uint: %w", path, source, err)
		}
		fv.SetUint(n)
	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(s, fv.Type().Bits())
		if err != nil {
			return fmt.Errorf("field %q: %s is not a valid float: %w", path, source, err)
		}
		fv.SetFloat(f)
	case reflect.Slice:
		if fv.Type().Elem().Kind() != reflect.String {
			return fmt.Errorf("field %q: %s cannot be applied to type %s", path, source, fv.Type())
		}
		parts := splitCommaList(s)
		out := reflect.MakeSlice(fv.Type(), len(parts), len(parts))
		for i, p := range parts {
			out.Index(i).SetString(p)
		}
		fv.Set(out)
	default:
		return fmt.Errorf("field %q: %s cannot be applied to type %s", path, source, fv.Type())
	}
	return nil
}

// isZeroValue 按类型判断字段当前值是否为零值（0/""/false/nil/空切片）。
func isZeroValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.String:
		return v.String() == ""
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Slice:
		return v.Len() == 0
	case reflect.Pointer, reflect.Map, reflect.Interface, reflect.Chan, reflect.Func:
		return v.IsNil()
	default:
		return v.IsZero()
	}
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
func validateRangeField(fv reflect.Value, rng, path string) error {
	isDur := fv.Type() == durationType
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
// 返回上下界（nil 表示开侧无界）与两侧是否闭区间。开侧的方括号方向不敏感。
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
