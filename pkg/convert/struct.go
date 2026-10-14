package convert

import (
	"reflect"
	"strings"
)

// ConvertStruct 将 src 结构体的同名字段映射到 dst。
// dst 必须是指针。通过反射匹配字段名，相同名称且类型兼容的字段会被复制。
//
//	type Src struct { Name string; Age int }
//	type Dst struct { Name string; Age int }
//	var d Dst
//	ConvertStruct(Src{Name: "test", Age: 18}, &d)
func ConvertStruct(src, dst any) error {
	srcVal := reflect.ValueOf(src)
	dstVal := reflect.ValueOf(dst)

	if dstVal.Kind() != reflect.Pointer || dstVal.IsNil() {
		return nil
	}

	srcVal = reflect.Indirect(srcVal)
	dstVal = dstVal.Elem()

	if srcVal.Kind() != reflect.Struct || dstVal.Kind() != reflect.Struct {
		return nil
	}

	srcType := srcVal.Type()
	for i := 0; i < srcVal.NumField(); i++ {
		srcField := srcVal.Field(i)
		srcFieldName := srcType.Field(i).Name

		dstField := dstVal.FieldByName(srcFieldName)
		if !dstField.IsValid() || !dstField.CanSet() {
			continue
		}

		if srcField.Type() == dstField.Type() {
			dstField.Set(srcField)
		}
	}

	return nil
}

// StructToMap 将结构体转为 map[string]any。
// 通过 json tag 获取 key 名，无 json tag 则用字段名。
func StructToMap(v any) map[string]any {
	result := make(map[string]any)
	val := reflect.ValueOf(v)
	val = reflect.Indirect(val)

	if val.Kind() != reflect.Struct {
		return result
	}

	typ := val.Type()
	for i := 0; i < val.NumField(); i++ {
		field := typ.Field(i)
		fieldVal := val.Field(i)

		if !fieldVal.CanInterface() {
			continue
		}

		key := field.Tag.Get("json")
		// 剥离 tag 选项（如 omitempty/string），只取名字部分，
		// 与 encoding/json 的键名语义一致
		if i := strings.Index(key, ","); i >= 0 {
			key = key[:i]
		}
		if key == "-" {
			// 与 encoding/json 一致：json:"-" 显式排除（敏感字段防泄漏）
			continue
		}
		if key == "" {
			key = field.Name
		}

		result[key] = fieldVal.Interface()
	}

	return result
}
