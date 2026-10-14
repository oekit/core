package convert

import (
	"strconv"
)

// BytesToString 字节切片转字符串。
func BytesToString(data []byte) string {
	return string(data)
}

// StringToBytes 字符串转字节切片。
func StringToBytes(s string) []byte {
	return []byte(s)
}

// BytesToInt 字节切片转 int，失败返回 0。
func BytesToInt(data []byte) int {
	val, err := strconv.Atoi(string(data))
	if err != nil {
		return 0
	}
	return val
}

// BytesToInt64 字节切片转 int64，失败返回 0。
func BytesToInt64(data []byte) int64 {
	val, _ := strconv.ParseInt(string(data), 10, 64)
	return val
}

// BytesToFloat32 字节切片转 float32，失败返回 0。
func BytesToFloat32(data []byte) float32 {
	val, _ := strconv.ParseFloat(string(data), 32)
	return float32(val)
}

// BytesToFloat64 字节切片转 float64，失败返回 0。
func BytesToFloat64(data []byte) float64 {
	val, _ := strconv.ParseFloat(string(data), 64)
	return val
}

// ByteToBytes 单个字节转字节切片。
func ByteToBytes(b byte) []byte {
	return []byte{b}
}

// IntToBytes int 转字节切片（十进制文本）。
func IntToBytes(i int) []byte {
	return []byte(strconv.Itoa(i))
}

// Int64ToBytes int64 转字节切片（十进制文本）。
func Int64ToBytes(i int64) []byte {
	return []byte(strconv.FormatInt(i, 10))
}

// Float64ToBytes float64 转字节切片，prec 为小数位数。
func Float64ToBytes(f float64, prec int) []byte {
	return []byte(strconv.FormatFloat(f, 'f', prec, 64))
}

// BoolToBytes bool 转字节切片。
func BoolToBytes(b bool) []byte {
	return []byte(strconv.FormatBool(b))
}
