package governance

import (
	"errors"
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
