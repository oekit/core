package config

import (
	"fmt"
	"reflect"
	"strings"
)

// OverlayMerge 将 src 中非零值字段覆盖合并到 dst 中，支持嵌套结构体。
//
// 支持的 merge tag：
//   - "overwrite": 整体替换（包括零值），适用于需要强制覆盖的字段
//   - "bool_or": 布尔字段使用 OR 语义（dst || src），适用于 enable 标志
//   - "zeroable": 零值视为有效值合并（而非跳过），适用于需要显式设置 0/""/false 的字段
func OverlayMerge[T any](dst *T, src *T) {
	OverlayMergeExplicit(dst, src, nil)
}

// OverlayMergeExplicit 在 OverlayMerge 之上接受**显式字段集合** explicit
// （FieldSet，键为 Go 字段路径，与 tag 校验错误信息同格式：`Server.Port`）：
// 命中路径的字段**无条件取 src 值（含零值）**，绕过「非零值才覆盖」与
// merge tag 的 zeroable/bool_or 语义；未命中路径的行为与 OverlayMerge 完全一致
// （explicit 为 nil 时二者等价）。
//
// 这是「显式零值」的合并逃生门（core#50）：非零覆盖模型无法表达「显式设置
// 的 false/0 压过 dst 里的非零值」，而 12-factor 场景里 env 恰恰是用户意图
// 最强的表达（`KUBOARD_MCP_ENABLED=false` 关功能）。ApplyEnvOverlay 用它实现
// 「env 命中即覆盖」。
//
// 集合通常来自 ApplyEnv 对 FieldSet 的补记——只含标量 / []string 叶子路径；
// 切片字段按整体替换（不递归到 `Items[0].Qty` 这类元素路径），struct 路径
// 不参与（继续下推到叶子）。
func OverlayMergeExplicit[T any](dst *T, src *T, explicit FieldSet) {
	// nil 防护：reflect.ValueOf(nil 指针).Elem() 直接 panic
	if dst == nil || src == nil {
		return
	}
	dstVal := reflect.ValueOf(dst).Elem()
	srcVal := reflect.ValueOf(src).Elem()
	overlayFields(dstVal, srcVal, "", explicit)
}

// isZeroable 检查 tag 是否包含 "zeroable"。
func isZeroable(tag string) bool {
	return tag == "zeroable" || tag == "overwrite"
}

// overlayFields 深度合并 src 到 dst；path 是当前层级的字段路径前缀，
// explicit 命中的字段无条件取 src 值（含零值，见 OverlayMergeExplicit）。
func overlayFields(dst, src reflect.Value, path string, explicit FieldSet) {
	if dst.Kind() != reflect.Struct || src.Kind() != reflect.Struct {
		return
	}
	t := dst.Type()
	for i := 0; i < dst.NumField(); i++ {
		dstField := dst.Field(i)
		srcField := src.Field(i)

		if !dstField.CanSet() {
			continue
		}

		fpath := t.Field(i).Name
		if path != "" {
			fpath = path + "." + fpath
		}
		forced := explicit.Has(fpath)

		tag := t.Field(i).Tag.Get("merge")

		switch srcField.Kind() {
		case reflect.String:
			if forced || isZeroable(tag) || srcField.String() != "" {
				dstField.SetString(srcField.String())
			}
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			if forced || isZeroable(tag) || srcField.Int() != 0 {
				dstField.SetInt(srcField.Int())
			}
		case reflect.Float32, reflect.Float64:
			if forced || isZeroable(tag) || srcField.Float() != 0 {
				dstField.SetFloat(srcField.Float())
			}
		case reflect.Bool:
			switch {
			case forced:
				// 显式命中压过 bool_or：env 命中即覆盖，OR 会把显式 false
				// 错误地留在 true 上（core#50）。
				dstField.SetBool(srcField.Bool())
			case tag == "bool_or":
				dstField.SetBool(dstField.Bool() || srcField.Bool())
			case isZeroable(tag) || srcField.Bool():
				dstField.SetBool(srcField.Bool())
			}
		case reflect.Slice:
			// 元素级显式路径（core#85，ApplyEnvOverlay 的 Items[0].Limit 形态）
			// 存在时**不做**整体替换——整块换会把 dst 元素里非 env 命中的字段
			// 清成零值；逐元素下推合并，dst 只有 env 命中的字段取 src 值。
			if hasElementExplicit(explicit, fpath) {
				mergeSliceElements(dstField, srcField, fpath, explicit)
				continue
			}
			if forced || isZeroable(tag) || srcField.Len() > 0 {
				// 深拷贝(2026-09-06 复审修复):直接 Set 会让 dst 与 src
				// 共享底层数组,合并后改 dst 元素会反向污染 src
				dstField.Set(deepCopyValue(srcField))
			}
		case reflect.Map:
			if forced || isZeroable(tag) || srcField.Len() > 0 {
				// 深拷贝:map 是引用类型,共享后两侧写操作互相可见
				dstField.Set(deepCopyValue(srcField))
			}
		case reflect.Array:
			if forced || isZeroable(tag) || !srcField.IsZero() {
				// 深拷贝（core#84 同族）:数组整体 Set 是值拷贝,但元素是
				// struct/引用类型时底层 map/slice 仍共享
				dstField.Set(deepCopyValue(srcField))
			}
		case reflect.Struct:
			// zeroable 对 struct 同样生效：零值 struct = 显式清空该配置块
			// （此前被静默当成默认合并,运维无法通过远程配置整体清零）。
			// 显式集合不含 struct 路径（ApplyEnv 只补记标量叶子）——继续下推。
			switch {
			case tag == "overwrite":
				// 深拷贝（core#84/#90）：整体 Set 是 Go 值语义的**浅**拷贝，
				// struct 里的 map/slice 字段仍是共享引用——合并后改 dst 会
				// 反向污染 src（与本文件 slice/map 分支已修掉的别名问题同族）。
				dstField.Set(deepCopyValue(srcField))
			case tag == "zeroable" && srcField.IsZero():
				dstField.SetZero()
			default:
				overlayFields(dstField, srcField, fpath, explicit)
			}
		case reflect.Pointer:
			// zeroable 的 nil 同样是有效值：远程配置缺块 = 显式清空
			// （dst 保持旧值会让"删除配置块"的热更语义失效）
			if tag == "zeroable" && srcField.IsNil() {
				dstField.SetZero()
				continue
			}
			if srcField.IsNil() {
				continue
			}
			if srcField.Elem().Kind() == reflect.Struct && tag != "overwrite" {
				if tag == "zeroable" && srcField.Elem().IsZero() {
					dstField.SetZero() // nil 化：清空该配置块
					continue
				}
				if dstField.IsNil() {
					dstField.Set(reflect.New(srcField.Elem().Type()))
				}
				overlayFields(dstField.Elem(), srcField.Elem(), fpath, explicit)
			} else {
				// 深拷贝:非 struct 指针(*string/*int 等)与 overwrite 指针
				// 原实现共享 pointee,改 dst 可见 src
				dstField.Set(deepCopyValue(srcField))
			}
		default:
			if forced || isZeroable(tag) || !srcField.IsZero() {
				dstField.Set(srcField)
			}
		}
	}
}

// hasElementExplicit 报告 explicit 里是否有落在 fpath 元素层级的路径
// （`Items[0].Limit` 形态，前缀 fpath+"["）。只有 ApplyEnvOverlay 的元素级
// env 命中会产生这类路径——命中才走逐元素合并，否则整体替换语义不变。
func hasElementExplicit(explicit FieldSet, fpath string) bool {
	if explicit == nil {
		return false
	}
	prefix := fpath + "["
	for k := range explicit {
		if strings.HasPrefix(k, prefix) {
			return true
		}
	}
	return false
}

// mergeSliceElements 逐元素下推合并：[]struct 元素按 struct 合并（非命中
// 字段保持 dst 值，命中字段取 src 值），[]*struct 元素两两解引用合并、dst
// 元素为 nil 时深拷贝 src 元素补位。dst 比 src 长时多余元素原样保留；
// src 更长时多余元素深拷贝追加（外部直接调 OverlayMergeExplicit 的防御路径
// ——ApplyEnvOverlay 两侧长度相等）。路径带下标（Items[0].Limit），与
// ApplyEnv 补记进 FieldSet 的键同口径。
func mergeSliceElements(dstField, srcField reflect.Value, fpath string, explicit FieldSet) {
	elem := dstField.Type().Elem()
	for j := 0; j < srcField.Len(); j++ {
		epath := fmt.Sprintf("%s[%d]", fpath, j)
		se := srcField.Index(j)
		if j >= dstField.Len() {
			v := deepCopyValue(se)
			dstField.Set(reflect.Append(dstField, v))
			continue
		}
		de := dstField.Index(j)
		switch {
		case elem.Kind() == reflect.Struct:
			overlayFields(de, se, epath, explicit)
		case elem.Kind() == reflect.Pointer && elem.Elem().Kind() == reflect.Struct:
			if se.IsNil() {
				continue
			}
			if de.IsNil() {
				de.Set(deepCopyValue(se))
				continue
			}
			overlayFields(de.Elem(), se.Elem(), epath, explicit)
		}
	}
}

// deepCopyValue 递归拷贝容器类型(map/slice/pointer/interface 内动态值/struct/
// 数组)。struct 不能走 Go 值语义直接拷贝——那对引用型字段是**浅**拷贝
// （core#84：overwrite 指针解引用后仍与 src 共享底层 map/slice），须逐字段
// 递归；数组同理（元素是 struct 时整体赋值仍共享）。仅用于 OverlayMerge 的
// 合并路径(config 合并是低频操作,开销可忽略)。
func deepCopyValue(v reflect.Value) reflect.Value {
	if !v.IsValid() {
		return v
	}
	switch v.Kind() {
	case reflect.Map:
		if v.IsNil() {
			return v
		}
		out := reflect.MakeMapWithSize(v.Type(), v.Len())
		iter := v.MapRange()
		for iter.Next() {
			out.SetMapIndex(deepCopyValue(iter.Key()), deepCopyValue(iter.Value()))
		}
		return out
	case reflect.Slice:
		if v.IsNil() {
			return v
		}
		out := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
		for i := 0; i < v.Len(); i++ {
			out.Index(i).Set(deepCopyValue(v.Index(i)))
		}
		return out
	case reflect.Array:
		out := reflect.New(v.Type()).Elem()
		for i := 0; i < v.Len(); i++ {
			out.Index(i).Set(deepCopyValue(v.Index(i)))
		}
		return out
	case reflect.Struct:
		// 先整块值拷贝（保住未导出字段——reflect 无法 Set 它们），再把
		// 可 Set 的引用型字段逐一替换成深拷贝；标量字段递归返回原值，
		// 等价不动。
		out := reflect.New(v.Type()).Elem()
		out.Set(v)
		for i := 0; i < out.NumField(); i++ {
			f := out.Field(i)
			if !f.CanSet() {
				continue
			}
			f.Set(deepCopyValue(v.Field(i)))
		}
		return out
	case reflect.Pointer:
		if v.IsNil() {
			return v
		}
		out := reflect.New(v.Type().Elem())
		out.Elem().Set(deepCopyValue(v.Elem()))
		return out
	case reflect.Interface:
		if v.IsNil() {
			return v
		}
		// 拷贝动态值,避免接口内隐藏的 map/slice 别名(concrete 值对
		// 接口字段可赋值)
		return deepCopyValue(v.Elem())
	default:
		return v
	}
}
