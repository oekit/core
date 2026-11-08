package governance

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestParse_Valid(t *testing.T) {
	cfg, err := Parse([]byte(`{
		"retry": {"svc:Method": {"max_attempts": 3, "backoff_ms": 50, "max_backoff_ms": 1000, "jitter_ms": 20}},
		"timeout_ms": {"svc:Method": 200, "svc": 500},
		"circuit_breaker": {"svc": {"error_rate_threshold": 0.5, "min_requests": 100, "open_ms": 10000}},
		"limiter": {"POST:/order": {"rate": 100, "burst": 200}}
	}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got, ok := cfg.LookupRetry("svc", "Method"); !ok || got.MaxAttempts != 3 {
		t.Errorf("LookupRetry = %+v ok=%v", got, ok)
	}
	if got, ok := cfg.LookupTimeout("svc", "Method"); !ok || got != 200 {
		t.Errorf("LookupTimeout exact = %d ok=%v, want 200", got, ok)
	}
	if got, ok := cfg.LookupTimeout("svc", "Other"); !ok || got != 500 {
		t.Errorf("LookupTimeout svc fallback = %d ok=%v, want 500", got, ok)
	}
	if _, ok := cfg.LookupTimeout("other", "X"); ok {
		t.Error("LookupTimeout no-match should be false")
	}
	if got, ok := cfg.LookupLimiter("POST", "/order"); !ok || got.Rate != 100 || got.Burst != 200 {
		t.Errorf("LookupLimiter = %+v ok=%v", got, ok)
	}
}

func TestParse_Invalid(t *testing.T) {
	cases := map[string]string{
		"retry out of range": `{"retry":{"a":{"max_attempts":11}}}`,
		"negative timeout":   `{"timeout_ms":{"a":-1}}`,
		"bad error rate":     `{"circuit_breaker":{"a":{"error_rate_threshold":1.5}}}`,
		"zero rate":          `{"limiter":{"a":{"rate":0}}}`,
		"malformed json":     `{`,
	}
	for name, data := range cases {
		if _, err := Parse([]byte(data)); !errors.Is(err, ErrInvalidConfig) {
			t.Errorf("%s: Parse err = %v, want ErrInvalidConfig", name, err)
		}
	}
}

func TestStore_AtomicUpdate(t *testing.T) {
	s := NewStore(nil)
	if _, ok := s.Load().LookupTimeout("a", "b"); ok {
		t.Error("empty store should not match")
	}
	if s.Version() != 0 {
		t.Errorf("initial version = %d, want 0", s.Version())
	}
	cfg, _ := Parse([]byte(`{"timeout_ms":{"a":1}}`))
	s.Update(cfg)
	if s.Version() != 1 {
		t.Errorf("version after update = %d, want 1", s.Version())
	}
	if v, ok := s.Load().LookupTimeout("a", "b"); !ok || v != 1 {
		t.Errorf("after update LookupTimeout = %d ok=%v", v, ok)
	}
	s.Update(nil) // nil 等价空配置
	if len(s.Load().TimeoutMs) != 0 {
		t.Error("Update(nil) should reset to empty config")
	}
}

func TestLookup_LexicalFallbackOrder(t *testing.T) {
	cfg := &Config{TimeoutMs: map[string]int{"": 9999, "svc": 500, "svc:Method": 100}}
	if v, _ := cfg.LookupTimeout("svc", "Method"); v != 100 {
		t.Errorf("exact = %d, want 100", v)
	}
	if v, _ := cfg.LookupTimeout("svc", "Other"); v != 500 {
		t.Errorf("svc = %d, want 500", v)
	}
	if v, _ := cfg.LookupTimeout("other", "X"); v != 9999 {
		t.Errorf("global = %d, want 9999", v)
	}
}

// TestValidate_ZeroTimeoutRejected 回归：ms=0 曾被放行——kitex 对 <=0 走
// "无超时" fast path（等于关闭超时保护），hertz timeout 中间件 0 立即到期。
// 清空超时用删除 timeout_ms 段表达，0 值必须拒绝。
func TestValidate_ZeroTimeoutRejected(t *testing.T) {
	cfg := &Config{TimeoutMs: map[string]int{"Method": 0}}
	if err := cfg.Validate(); err == nil {
		t.Error("timeout_ms=0 must be rejected (kitex treats <=0 as no deadline)")
	}
	cfg2 := &Config{TimeoutMs: map[string]int{"Method": -5}}
	if err := cfg2.Validate(); err == nil {
		t.Error("negative timeout_ms must be rejected")
	}
}

// TestLookup_PrefixMatching 回归 core#78：路径/service 前缀策略此前永不命中
// ——lookup 只做两档精确匹配，「前缀最长匹配」档整个缺失，/order 的限流配置
// 对 /order/123 静默失效，LookupLimiterKey 恒退化为全局键。
func TestLookup_PrefixMatching(t *testing.T) {
	cfg := &Config{
		TimeoutMs: map[string]int{"com.example": 500, "": 1000},
		Limiter:   map[string]LimiterPolicy{"/order": {Rate: 100}},
	}
	// service 按「.」段前缀命中
	if v, ok := cfg.LookupTimeout("com.example.svc", "Method"); !ok || v != 500 {
		t.Errorf("LookupTimeout(com.example.svc) = %d ok=%v, want 500 via com.example prefix", v, ok)
	}
	// path 按「/」段前缀命中
	if p, ok := cfg.LookupLimiter("GET", "/order/123"); !ok || p.Rate != 100 {
		t.Errorf("LookupLimiter(GET,/order/123) = %+v ok=%v, want rate=100 via /order prefix", p, ok)
	}
	// LookupLimiterKey 返回实际命中的粗粒度键（不再恒为 ""）
	p, key, ok := cfg.LookupLimiterKey("GET", "/order/123")
	if !ok || p.Rate != 100 || key != "/order" {
		t.Errorf("LookupLimiterKey = %+v key=%q ok=%v, want rate=100 key=/order", p, key, ok)
	}
	// method 限定的前缀键更具体，优先于裸 path 前缀
	cfg2 := &Config{Limiter: map[string]LimiterPolicy{
		"/order":     {Rate: 100},
		"GET:/order": {Rate: 200},
	}}
	if p, key, _ := cfg2.LookupLimiterKey("GET", "/order/123"); p.Rate != 200 || key != "GET:/order" {
		t.Errorf("method-scoped prefix should win: got %+v key=%q", p, key)
	}
	// 段边界是硬约束：/orders 不能被 /order 命中
	cfg3 := &Config{Limiter: map[string]LimiterPolicy{"/order": {Rate: 100}}}
	if _, key, ok := cfg3.LookupLimiterKey("GET", "/orders"); ok || key != "" {
		t.Errorf("/orders must not match /order prefix, got key=%q ok=%v", key, ok)
	}
	if _, key, ok := cfg3.LookupLimiterKey("GET", "/order/123"); !ok || key != "/order" {
		t.Errorf("/order/123 should match /order, got key=%q ok=%v", key, ok)
	}
	// service 段边界同理：com.examplex 不被 com.example 命中（分隔符必须落
	// 在段尾；com.example.org 反而**应该**被命中——它是 com.example 的
	// 下级域，语义与 DNS 前缀一致）
	cfg4 := &Config{TimeoutMs: map[string]int{"com.example": 500}}
	if _, ok := cfg4.LookupTimeout("com.examplex", "M"); ok {
		t.Error("com.examplex must not match com.example prefix (segment boundary)")
	}
	if v, ok := cfg4.LookupTimeout("com.example.org", "M"); !ok || v != 500 {
		t.Errorf("com.example.org should match com.example (parent domain), got %d ok=%v", v, ok)
	}
	// 最长前缀优先：/order/detail 胜过 /order
	cfg5 := &Config{Limiter: map[string]LimiterPolicy{
		"/order":        {Rate: 100},
		"/order/detail": {Rate: 300},
	}}
	if _, key, _ := cfg5.LookupLimiterKey("GET", "/order/detail/1"); key != "/order/detail" {
		t.Errorf("longest prefix should win: got key=%q", key)
	}
	// 全局兜底仍是最后一档
	if v, ok := cfg.LookupTimeout("other.svc", "X"); !ok || v != 1000 {
		t.Errorf("global fallback = %d ok=%v, want 1000", v, ok)
	}
}

// TestStore_ConsistentValueAndVersion 回归 core#79：值与版本此前是两个原子
// 变量且「先换值、后进版本」，并发读侧在交错窗口能拿到「新值配旧版本」的
// 自相矛盾对——按 Version 判变化的消费方漏判。改为单单元发布后不存在该窗口。
func TestStore_ConsistentValueAndVersion(t *testing.T) {
	s := NewStore(nil)
	const writers = 4
	const iters = 5000
	var wg sync.WaitGroup
	stop := make(chan struct{})
	// 读侧：值内标记的代次必须与读到的版本一致（或至多偏新——跨两次读）。
	for range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 1; i <= iters; i++ {
				cfg := &Config{TimeoutMs: map[string]int{"mark": i}}
				s.Update(cfg)
			}
		}()
	}
	inconsistent := int64(0)
	var rWG sync.WaitGroup
	for range 2 {
		rWG.Add(1)
		go func() {
			defer rWG.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				cfg, ver := s.LoadVersion()
				if cfg == nil {
					continue
				}
				if mark := cfg.TimeoutMs["mark"]; int64(mark) > ver {
					atomic.AddInt64(&inconsistent, 1)
				}
			}
		}()
	}
	for range writers {
		wg.Wait()
	}
	close(stop)
	rWG.Wait()
	if atomic.LoadInt64(&inconsistent) != 0 {
		t.Errorf("observed %d inconsistent (value, version) pairs", inconsistent)
	}
	// 收敛后版本恰为 writers*iters（每次 Update 恰好推进一版，不重不漏）
	if got := s.Version(); got != writers*iters {
		t.Errorf("final version = %d, want %d", got, writers*iters)
	}
}

// TestStore_UpdateDeepCopies 回归 core#80：Update 此前只存指针，发布方在
// Update 之后改写入参会直接改写「已发布的不可变快照」且版本不前进。
func TestStore_UpdateDeepCopies(t *testing.T) {
	s := NewStore(nil)
	cfg := &Config{TimeoutMs: map[string]int{"x": 1}}
	s.Update(cfg)
	if v := s.Version(); v != 1 {
		t.Fatalf("version after update = %d, want 1", v)
	}
	// 发布方事后改写入参：快照不受影响
	cfg.TimeoutMs["x"] = 999
	cfg.TimeoutMs["y"] = 888
	snap := s.Load()
	if snap.TimeoutMs["x"] != 1 {
		t.Errorf("published snapshot was mutated via caller input: TimeoutMs[x]=%d, want 1", snap.TimeoutMs["x"])
	}
	if _, ok := snap.TimeoutMs["y"]; ok {
		t.Error("published snapshot gained a key the caller added after Update")
	}
	if v := s.Version(); v != 1 {
		t.Errorf("version moved without Update: %d, want 1", v)
	}
	// NewStore 同样深拷贝：改构造入参不影响初始快照
	seed := &Config{Limiter: map[string]LimiterPolicy{"k": {Rate: 5}}}
	s2 := NewStore(seed)
	seed.Limiter["k"] = LimiterPolicy{Rate: 777}
	seed.Limiter["j"] = LimiterPolicy{Rate: 1}
	if got := s2.Load().Limiter["k"]; got.Rate != 5 {
		t.Errorf("NewStore snapshot mutated via seed: Limiter[k].Rate=%d, want 5", got.Rate)
	}
	if _, ok := s2.Load().Limiter["j"]; ok {
		t.Error("NewStore snapshot gained a key the caller added after construction")
	}
	// nil 语义保持：Update(nil) 等价空配置，版本仍前进
	s.Update(nil)
	if v := s.Version(); v != 2 {
		t.Errorf("version after Update(nil) = %d, want 2", v)
	}
	if got := s.Load(); got == nil || got.TimeoutMs != nil {
		t.Errorf("Update(nil) should publish an empty config with nil maps, got %+v", got)
	}
}
