package convert

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
)

// ===== 字符串 → 标量（失败返回默认值）=====

// Str2Int 字符串转 int，失败返回默认值。
func Str2Int(s string, def int) int {
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}

// Str2Int64 字符串转 int64，失败返回默认值。
func Str2Int64(s string, def int64) int64 {
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return def
	}
	return v
}

// Str2Int32 字符串转 int32，失败返回默认值。
func Str2Int32(s string, def int32) int32 {
	v, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return def
	}
	return int32(v)
}

// Str2Uint 字符串转 uint，失败返回默认值。
// 负数字符串会解析失败并返回默认值。
func Str2Uint(s string, def uint) uint {
	// 按平台 int 位宽解析：64 位宽解析后截断会在 32 位平台上把
	// "4294967296" 静默变成 0，而非走"失败返回默认值"契约
	v, err := strconv.ParseUint(s, 10, strconv.IntSize)
	if err != nil {
		return def
	}
	return uint(v)
}

// Str2Uint64 字符串转 uint64，失败返回默认值。
// 负数字符串会解析失败并返回默认值。
func Str2Uint64(s string, def uint64) uint64 {
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return def
	}
	return v
}

// Str2Float64 字符串转 float64，失败返回默认值。
func Str2Float64(s string, def float64) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return def
	}
	return v
}

// Str2Float32 字符串转 float32，失败返回默认值。
func Str2Float32(s string, def float32) float32 {
	v, err := strconv.ParseFloat(s, 32)
	if err != nil {
		return def
	}
	return float32(v)
}

// Str2Bool 字符串转 bool，失败返回默认值。
// 接受 "1", "t", "T", "TRUE", "true", "True" 为 true。
func Str2Bool(s string, def bool) bool {
	v, err := strconv.ParseBool(s)
	if err != nil {
		return def
	}
	return v
}

// ===== 标量 → 字符串 =====

// IntToStr int 转字符串。
func IntToStr(i int) string {
	return strconv.Itoa(i)
}

// Int32ToStr int32 转字符串。
func Int32ToStr(i int32) string {
	return strconv.FormatInt(int64(i), 10)
}

// Int64ToStr int64 转字符串。
func Int64ToStr(i int64) string {
	return strconv.FormatInt(i, 10)
}

// UintToStr uint 转字符串。
func UintToStr(u uint) string {
	return strconv.FormatUint(uint64(u), 10)
}

// Uint64ToStr uint64 转字符串。
func Uint64ToStr(u uint64) string {
	return strconv.FormatUint(u, 10)
}

// Float64ToStr float64 转字符串，prec 为小数位数（-1 为最短表示）。
func Float64ToStr(f float64, prec int) string {
	return strconv.FormatFloat(f, 'f', prec, 64)
}

// Float32ToStr float32 转字符串，prec 为小数位数（-1 为最短表示）。
func Float32ToStr(f float32, prec int) string {
	return strconv.FormatFloat(float64(f), 'f', prec, 32)
}

// Bool2Str bool 转字符串。
func Bool2Str(b bool) string {
	return strconv.FormatBool(b)
}

// ===== 任意类型 → 标量 =====
//
// Any 系列用于安全转换动态类型，典型场景是 json.Unmarshal 到 map[string]any 后
// 取出的值（数字默认为 float64）。无法转换时返回零值，不会 panic。

// AnyToStr 任意类型转字符串。
func AnyToStr(v any) string {
	return fmt.Sprintf("%v", v)
}

// AnyToInt 任意类型转 int，无法转换返回 0。
func AnyToInt(v any) int {
	return int(AnyToInt64(v))
}

// AnyToInt64 任意类型转 int64，无法转换返回 0。
// 支持 bool（true=1, false=0）、string/[]byte（按十进制解析）、所有数值类型。
func AnyToInt64(v any) int64 {
	switch val := v.(type) {
	case nil:
		return 0
	case int:
		return int64(val)
	case int8:
		return int64(val)
	case int16:
		return int64(val)
	case int32:
		return int64(val)
	case int64:
		return val
	case uint:
		// 超出 int64 表示范围属于"无法转换"，按契约返回 0（而非回绕为负数）
		if uint64(val) > math.MaxInt64 {
			return 0
		}
		return int64(val)
	case uint8:
		return int64(val)
	case uint16:
		return int64(val)
	case uint32:
		return int64(val)
	case uint64:
		if val > math.MaxInt64 {
			return 0
		}
		return int64(val)
	case float32:
		return float64ToInt64Safe(float64(val))
	case float64:
		return float64ToInt64Safe(val)
	case bool:
		if val {
			return 1
		}
		return 0
	case string:
		return Str2Int64(val, 0)
	case []byte:
		return BytesToInt64(val)
	case json.Number:
		// encoding.Unmarshal 使用 UseNumber，数值是 json.Number；
		// 不处理会导致所有 JSON 数值静默归零
		if iv, err := val.Int64(); err == nil {
			return iv
		}
		fv, err := val.Float64()
		if err != nil {
			return 0
		}
		return float64ToInt64Safe(fv)
	}
	return 0
}

// float64ToInt64Safe 浮点转 int64：NaN/±Inf 与超出 int64 表示范围的值
// 按"无法转换"契约返回 0（直接 int64(f) 属于 implementation-defined）。
func float64ToInt64Safe(f float64) int64 {
	if math.IsNaN(f) || math.IsInf(f, 0) || f < math.MinInt64 || f >= 9223372036854775808 {
		return 0
	}
	return int64(f)
}

// float64ToUint64Safe 浮点转 uint64：NaN/±Inf、负数与超出 uint64 表示范围的值返回 0。
func float64ToUint64Safe(f float64) uint64 {
	if math.IsNaN(f) || math.IsInf(f, 0) || f < 0 || f >= 18446744073709551616 {
		return 0
	}
	return uint64(f)
}

// AnyToUint64 任意类型转 uint64，无法转换返回 0。
// 注意：传入负数会按位重新解释为大正数（uint 语义），调用方应自行确保非负。
func AnyToUint64(v any) uint64 {
	switch val := v.(type) {
	case nil:
		return 0
	case uint:
		return uint64(val)
	case uint8:
		return uint64(val)
	case uint16:
		return uint64(val)
	case uint32:
		return uint64(val)
	case uint64:
		return val
	case int:
		return uint64(val)
	case int8:
		return uint64(val)
	case int16:
		return uint64(val)
	case int32:
		return uint64(val)
	case int64:
		return uint64(val)
	case float32:
		return float64ToUint64Safe(float64(val))
	case float64:
		return float64ToUint64Safe(val)
	case bool:
		if val {
			return 1
		}
		return 0
	case string:
		v, _ := strconv.ParseUint(val, 10, 64)
		return v
	case []byte:
		v, _ := strconv.ParseUint(string(val), 10, 64)
		return v
	case json.Number:
		if iv, err := val.Int64(); err == nil && iv > 0 {
			return uint64(iv)
		}
		fv, err := val.Float64()
		if err != nil || fv <= 0 {
			return 0
		}
		return uint64(fv)
	}
	return 0
}

// AnyToFloat64 任意类型转 float64，无法转换返回 0。
// 支持 bool（true=1, false=0）、string/[]byte（按浮点解析）、所有数值类型。
func AnyToFloat64(v any) float64 {
	switch val := v.(type) {
	case nil:
		return 0
	case float32:
		return float64(val)
	case float64:
		return val
	case int:
		return float64(val)
	case int8:
		return float64(val)
	case int16:
		return float64(val)
	case int32:
		return float64(val)
	case int64:
		return float64(val)
	case uint:
		return float64(val)
	case uint8:
		return float64(val)
	case uint16:
		return float64(val)
	case uint32:
		return float64(val)
	case uint64:
		return float64(val)
	case bool:
		if val {
			return 1
		}
		return 0
	case string:
		return Str2Float64(val, 0)
	case []byte:
		return BytesToFloat64(val)
	case json.Number:
		fv, err := val.Float64()
		if err != nil {
			return 0
		}
		return fv
	}
	return 0
}

// AnyToFloat32 任意类型转 float32，无法转换返回 0。
func AnyToFloat32(v any) float32 {
	return float32(AnyToFloat64(v))
}

// AnyToBool 任意类型转 bool，无法转换返回 false。
// 数值类型非零为 true；字符串按 strconv.ParseBool 解析。
func AnyToBool(v any) bool {
	switch val := v.(type) {
	case nil:
		return false
	case bool:
		return val
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return AnyToInt64(val) != 0
	case string:
		return Str2Bool(val, false)
	case []byte:
		return Str2Bool(string(val), false)
	case json.Number:
		return AnyToInt64(val) != 0
	}
	return false
}
