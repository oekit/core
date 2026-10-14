package convert

import "encoding/json"

// DeepCopy 对 JSON 动态数据（map[string]any / []any 递归嵌套）进行深拷贝。
// 仅处理 any 承载的动态容器；强类型容器（如 []map[string]any、
// map[string][]string）与其他类型原样返回（浅拷贝），调用方自行注意。
func DeepCopy(value any) any {
	if valueMap, ok := value.(map[string]any); ok {
		newMap := make(map[string]any)
		for k, v := range valueMap {
			newMap[k] = DeepCopy(v)
		}
		return newMap
	} else if valueSlice, ok := value.([]any); ok {
		newSlice := make([]any, len(valueSlice))
		for k, v := range valueSlice {
			newSlice[k] = DeepCopy(v)
		}
		return newSlice
	}
	return value
}

// AppendStrings 将 b 切片追加到 a 切片。
func AppendStrings(a, b []string) []string {
	for i := 0; i < len(b); i++ {
		a = append(a, b[i])
	}
	return a
}

// Convert 通过 JSON 序列化/反序列化将 data 转换到 target。
// 注意：此方法会丢失精度，不适用于需要精确转换的场景。
func Convert(data any, target any) error {
	bytes, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return json.Unmarshal(bytes, target)
}
