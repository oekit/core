package mathx

import (
	"cmp"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strconv"
	"sync"
	"time"
)

// rng 是包级随机数生成器，避免每次调用分配新实例。
var (
	rng   = rand.New(rand.NewSource(time.Now().UnixNano()))
	rngMu sync.Mutex
)

// RandIntn 返回 [0, maxInt) 范围内的随机整数。
// 并发安全，使用包级 rng 避免同纳秒碰撞。
func RandIntn(maxInt int) int {
	rngMu.Lock()
	v := rng.Intn(maxInt)
	rngMu.Unlock()
	return v
}

// DivisionN 执行除法并保留 n 位小数（截断，非四舍五入）。
func DivisionN(dividend, divisor float64, n int) float64 {
	if dividend == 0 || divisor == 0 {
		return 0
	}
	m := math.Pow10(n)
	return float64(int64((dividend/divisor)*m)) / m
}

// Round 对 float64 进行四舍五入，保留 n 位小数。
func Round(x float64, n int) float64 {
	times := math.Pow10(n)
	x = math.Round(x * times)
	return toFixed(x/times, n)
}

// Floor 对 float64 向下取整，保留 n 位小数。
func Floor(x float64, n int) float64 {
	times := math.Pow10(n)
	x = math.Floor(x * times)
	return toFixed(x/times, n)
}

// ProbabilityElems 计算概率分布的累积边界。
// elems 是概率值切片（所有权重必须 > 0），precise 是精度基数。
// 不修改输入切片（内部排序副本）。
// 存在 0/负权重时直接 panic：log10(precise/0) 产生 +Inf/NaN，band 会静默
// 退化为全 0，Probability 恒选 index 0——概率路由静默出错比快速失败更危险。
func ProbabilityElems(elems []float64, precise float64) []int {
	if len(elems) == 0 {
		return nil
	}
	sorted := make([]float64, len(elems))
	copy(sorted, elems)
	sort.Float64s(sorted)

	if sorted[0] <= 0 {
		panic("mathx: ProbabilityElems requires all weights > 0, got " +
			fmt.Sprintf("%v", elems))
	}

	n := math.Ceil(math.Log10(precise / sorted[0]))
	p := math.Pow10(int(n))

	l := len(sorted)
	band := make([]int, l)
	band[0] = int(sorted[0] * p)
	for i := 1; i < l; i++ {
		band[i] = band[i-1] + int(sorted[i]*p)
	}
	return band
}

// Probability 根据累积概率边界返回随机选中的索引。
// band 为空时返回 (0, 0)。
func Probability(band []int) (index, r int) {
	l := len(band)
	if l == 0 {
		return 0, 0
	}
	max := band[l-1]
	if max <= 0 {
		return 0, 0
	}
	r = RandIntn(max)
	for i, v := range band {
		if r <= v {
			return i, r
		}
	}
	return l - 1, max
}

// ToFixed 将 float64 格式化为指定小数位数。
func ToFixed(num float64, precise int) float64 {
	return toFixed(num, precise)
}

func toFixed(num float64, precise int) float64 {
	format := fmt.Sprintf("%%.%df", precise)
	fnum, _ := strconv.ParseFloat(fmt.Sprintf(format, num), 64)
	return fnum
}

// RangeRandFloat64 返回 [min, max) 范围内的随机浮点数。
func RangeRandFloat64(min, max float64) float64 {
	if min > max {
		min, max = max, min
	}
	r := rand.Float64()
	return min + ((max - min) * r)
}

// Clamp 将 v 限制在 [lo, hi] 范围内。
// 支持 int、float、string 等所有实现了 cmp.Ordered 的类型。
func Clamp[T cmp.Ordered](v, lo, hi T) T {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
