package governance

import (
	"errors"
	"strings"
	"testing"
)

// core#141 的护栏：lookup 前缀档重写为「LastIndexByte 段游走」（零切片分配、
// 命中路径消灭第二次拼接、method 不再 TrimSuffix 反推）。两道护栏——
// ① 与旧算法（测试内对照版）差分比对：段边界语义（起始分隔符不算边界、
//
//	分隔符必须落在段尾、从长到短逐档、每档 qualified 先于 bare）逐查询一致；
//
// ② 分配预算：AllocsPerRun 钉住热路径分配上限，重写退化时立即爆红。

// naiveSegmentPrefixes / naiveLookupPolicyKey 是重写前实现的对照版（仅测试）：
// 保留「物化前缀切片 + 整体倒置 + TrimSuffix 反推 method」的旧形态。
func naiveSegmentPrefixes(s, sep string) []string {
	var out []string
	for i := 0; i < len(s); i++ {
		if i > 0 && s[i] == sep[0] {
			out = append(out, s[:i])
		}
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func naiveLookupPolicyKey[T any](m map[string]T, method, path string) (T, string, bool) {
	var zero T
	if m == nil {
		return zero, "", false
	}
	full := method + ":" + path
	if v, ok := m[full]; ok {
		return v, full, true
	}
	if v, ok := m[path]; ok {
		return v, path, true
	}
	for _, p := range naiveSegmentPrefixes(path, "/") {
		if v, ok := m[method+":"+p]; ok {
			return v, method + ":" + p, true
		}
		if v, ok := m[p]; ok {
			return v, p, true
		}
	}
	v, ok := m[""]
	return v, "", ok
}

func naiveLookup[T any](m map[string]T, svc, method string) (T, bool) {
	var zero T
	if m == nil {
		return zero, false
	}
	if v, ok := m[svc+":"+method]; ok {
		return v, true
	}
	if v, ok := m[svc]; ok {
		return v, true
	}
	for _, p := range naiveSegmentPrefixes(svc, ".") {
		if v, ok := m[p]; ok {
			return v, true
		}
	}
	v, ok := m[""]
	return v, ok
}

// TestLookup_DifferentialVsNaive 新实现与旧算法逐查询差分：路径/方法/键集
// 全组合下命中值、命中键、命中与否三项必须完全一致（含空 path、起始「/」、
// 尾部「/」、双「//」、无分隔符、method 为空串等边界）。
func TestLookup_DifferentialVsNaive(t *testing.T) {
	paths := []string{
		"", "/order", "/order/", "/order/123", "/a//b", "/a/b/c/d/e",
		"orders", "/orders", "noslash", "/:weird", "//x", "/", "x:y/z",
	}
	methods := []string{"GET", "POST", ""}
	keys := []string{
		"", "GET:/order", "/order", "POST:/order", ":/order", "/a",
		"GET:/a", "/a/b", "GET:/a/b",
	}
	for _, path := range paths {
		for _, method := range methods {
			for _, keyset := range keysetsUnderTest(keys) {
				m := make(map[string]LimiterPolicy, len(keyset))
				for i, k := range keyset {
					m[k] = LimiterPolicy{Rate: i + 1}
				}
				cfg := &Config{Limiter: m}
				wantV, wantK, wantOK := naiveLookupPolicyKey(m, method, path)
				gotV, gotK, gotOK := cfg.LookupLimiterKey(method, path)
				if gotOK != wantOK || gotK != wantK || gotV != wantV {
					t.Fatalf("LookupLimiterKey(%q,%q) keys=%q = (%+v,%q,%v), want (%+v,%q,%v)",
						method, path, keyset, gotV, gotK, gotOK, wantV, wantK, wantOK)
				}
			}
		}
	}

	svcs := []string{"com.example.svc", "com.example", "svc", "", ".x.", "a.b.c.d", "trailing."}
	svcKeys := []string{"", "com.example", "com", "com.example.svc", "svc", "a.b", "a.b.c"}
	for _, svc := range svcs {
		for _, method := range methods {
			for _, keyset := range keysetsUnderTest(svcKeys) {
				m := make(map[string]int, len(keyset))
				for i, k := range keyset {
					m[k] = i + 1
				}
				cfg := &Config{TimeoutMs: m}
				wantV, wantOK := naiveLookup(m, svc, method)
				gotV, gotOK := cfg.LookupTimeout(svc, method)
				if gotOK != wantOK || gotV != wantV {
					t.Fatalf("LookupTimeout(%q,%q) keys=%q = (%d,%v), want (%d,%v)",
						svc, method, keyset, gotV, gotOK, wantV, wantOK)
				}
			}
		}
	}
}

// keysetsUnderTest 返回 keys 的全部非空子集（指数级但 |keys| 小，网格有界）。
func keysetsUnderTest(keys []string) [][]string {
	n := 1 << len(keys)
	out := make([][]string, 0, n)
	for mask := 0; mask < n; mask++ {
		var set []string
		for i, k := range keys {
			if mask&(1<<i) != 0 {
				set = append(set, k)
			}
		}
		out = append(out, set)
	}
	return out
}

// TestLookup_AllocBudget 热路径分配预算（core#141）：前缀命中路径
// ≤2 次分配（精确档拼接 1 + 候选 qualified 拼接 1，命中复用同一串）；
// 旧实现同路径为 5 次（full + ":"+prefix + 前缀切片 + 候选键 + 命中键）。
func TestLookup_AllocBudget(t *testing.T) {
	cfg := &Config{Limiter: map[string]LimiterPolicy{"GET:/order": {Rate: 100}}}
	allocs := testing.AllocsPerRun(200, func() {
		_, _, _ = cfg.LookupLimiterKey("GET", "/order/123")
	})
	if allocs > 2 {
		t.Fatalf("LookupLimiterKey 前缀命中路径分配 %v 次/查询，预算 ≤2（旧实现 5）——热路径是否退化？", allocs)
	}

	// 无前缀档的未命中（单段 path）：精确档 1 次拼接封顶
	cfg2 := &Config{Limiter: map[string]LimiterPolicy{"GET:/other": {Rate: 1}}}
	allocs2 := testing.AllocsPerRun(200, func() {
		_, _, _ = cfg2.LookupLimiterKey("GET", "/health")
	})
	if allocs2 > 1 {
		t.Fatalf("单段 path 未命中分配 %v 次/查询，预算 ≤1", allocs2)
	}
}

func BenchmarkLookupLimiterKey(b *testing.B) {
	cfg := &Config{Limiter: map[string]LimiterPolicy{"GET:/order": {Rate: 100}}}
	b.ReportAllocs()
	for b.Loop() {
		_, _, _ = cfg.LookupLimiterKey("GET", "/order/123")
	}
}

// TestValidate_CBPolicyBoundaryMessage 回归 core#143：校验实现是闭区间
// [0,1]，0 与 1 合法；越界文案与 retry 的 "[0,10]" 同格式。
func TestValidate_CBPolicyBoundaryMessage(t *testing.T) {
	for _, threshold := range []float64{0, 1} {
		cfg := &Config{CircuitBreaker: map[string]CBPolicy{"svc": {ErrorRateThreshold: threshold}}}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("threshold=%v 应通过校验（闭区间端点合法）：%v", threshold, err)
		}
	}
	for _, threshold := range []float64{-0.1, 1.1} {
		cfg := &Config{CircuitBreaker: map[string]CBPolicy{"svc": {ErrorRateThreshold: threshold}}}
		err := cfg.Validate()
		if err == nil {
			t.Fatalf("threshold=%v 应拒载", threshold)
		}
		if !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("应挂 ErrInvalidConfig 哨兵：%v", err)
		}
		if !strings.Contains(err.Error(), "out of range [0,1]") {
			t.Fatalf("文案应写闭区间 [0,1]（core#143），实际：%v", err)
		}
	}
}
