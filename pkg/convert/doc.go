// Package convert 提供常用类型转换工具函数。
//
// 所有转换函数都支持默认值，转换失败时返回默认值而非 error。
//
//	convert.Str2Int64("123", 0)     // 123
//	convert.Str2Int64("abc", 0)     // 0 (转换失败返回默认值)
//	convert.Str2Float64("3.14", 0)  // 3.14
//	convert.Bool2Str(true)          // "true"
package convert
