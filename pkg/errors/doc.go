// Package errors 提供线程安全的多错误聚合。
//
// 支持 errors.Is 链式检查，使用 sync.Pool 优化字符串格式化。
//
// # 快速接入
//
//	var err error
//	err = errors.Append(err, op1())
//	err = errors.Append(err, op2())
//	if err != nil { ... }
//
// # 支持 errors.Is
//
//	if errors.Is(err, os.ErrNotExist) { ... }
//
// # 遍历所有错误
//
//	if merr, ok := err.(*errors.MultiError); ok {
//	    for _, e := range merr.Errors() { ... }
//	}
//
// # 使用场景
//
//   - 批量操作：同时执行多个操作，收集所有错误
//   - 资源清理：关闭多个资源，记录所有失败
//   - 验证：收集所有验证错误
//
// # 性能优化
//
//   - 使用 sync.Pool 复用 bytes.Buffer
//   - Copy-on-write 优化常见的"反复 Append 到左侧"模式
//   - 自动展开嵌套的 MultiError
package errors
