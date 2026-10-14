package slicex

// Partition 按 fn 返回 true/false 把 slice 分成两组(matching, rest),
// 保留原顺序。
//
// 业务场景: 批量操作时把"成功/失败"分开;订单状态按"已支付/未支付"
// 分组;任务按"高/低优先级"分组。
//
// 用法:
//
//	success, failure := slicex.Partition(results, func(r Result) bool {
//	    return r.Error == nil
//	})
//
// 注意: 跟 GroupBy 的区别是 GroupBy 返回 map[K][]T,Partition 返回
// 两组互斥切片(matching + rest);如果需要 N 个 key 分组用 GroupBy,
// 二选一分组用 Partition。
func Partition[T any](slice []T, fn func(T) bool) (matching, rest []T) {
	for _, v := range slice {
		if fn(v) {
			matching = append(matching, v)
		} else {
			rest = append(rest, v)
		}
	}
	return matching, rest
}
