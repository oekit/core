package config

import (
	"reflect"
)

// OverlayMerge 将 src 中非零值字段覆盖合并到 dst 中，支持嵌套结构体。
//
// 支持的 merge tag：
//   - "overwrite": 整体替换（包括零值），适用于需要强制覆盖的字段
//   - "bool_or": 布尔字段使用 OR 语义（dst || src），适用于 enable 标志
//   - "zeroable": 零值视为有效值合并（而非跳过），适用于需要显式设置 0/""/false 的字段
func OverlayMerge[T any](dst *T, src *T) {
	// nil 防护：reflect.ValueOf(nil 指针).Elem() 直接 panic
	if dst == nil || src == nil {
		return
	}
	dstVal := reflect.ValueOf(dst).Elem()
	srcVal := reflect.ValueOf(src).Elem()
	overlayFields(dstVal, srcVal)
}

// isZeroable 检查 tag 是否包含 "zeroable"。
func isZeroable(tag string) bool {
	return tag == "zeroable" || tag == "overwrite"
}

func overlayFields(dst, src reflect.Value) {
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

		tag := t.Field(i).Tag.Get("merge")

		switch srcField.Kind() {
		case reflect.String:
			if isZeroable(tag) || srcField.String() != "" {
				dstField.SetString(srcField.String())
			}
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			if isZeroable(tag) || srcField.Int() != 0 {
				dstField.SetInt(srcField.Int())
			}
		case reflect.Float32, reflect.Float64:
			if isZeroable(tag) || srcField.Float() != 0 {
				dstField.SetFloat(srcField.Float())
			}
		case reflect.Bool:
			if tag == "bool_or" {
				dstField.SetBool(dstField.Bool() || srcField.Bool())
			} else if isZeroable(tag) || srcField.Bool() {
				dstField.SetBool(srcField.Bool())
			}
		case reflect.Slice:
			if isZeroable(tag) || srcField.Len() > 0 {
				// 深拷贝(2026-09-06 复审修复):直接 Set 会让 dst 与 src
				// 共享底层数组,合并后改 dst 元素会反向污染 src
				dstField.Set(deepCopyValue(srcField))
			}
		case reflect.Map:
			if isZeroable(tag) || srcField.Len() > 0 {
				// 深拷贝:map 是引用类型,共享后两侧写操作互相可见
				dstField.Set(deepCopyValue(srcField))
			}
		case reflect.Struct:
			// zeroable 对 struct 同样生效：零值 struct = 显式清空该配置块
			// （此前被静默当成默认合并,运维无法通过远程配置整体清零）
			switch {
			case tag == "overwrite":
				dstField.Set(srcField)
			case tag == "zeroable" && srcField.IsZero():
				dstField.SetZero()
			default:
				overlayFields(dstField, srcField)
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
				overlayFields(dstField.Elem(), srcField.Elem())
			} else {
				// 深拷贝:非 struct 指针(*string/*int 等)与 overwrite 指针
				// 原实现共享 pointee,改 dst 可见 src
				dstField.Set(deepCopyValue(srcField))
			}
		default:
			if isZeroable(tag) || !srcField.IsZero() {
				dstField.Set(srcField)
			}
		}
	}
}

// deepCopyValue 递归拷贝容器类型(map/slice/pointer/interface 内动态值),
// 值类型(字符串/数字/bool/struct/数组)按 Go 值语义直接拷贝。
// 仅用于 OverlayMerge 的合并路径(config 合并是低频操作,开销可忽略)。
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
