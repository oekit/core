// Package governance 定义 kitex/hertz 共用的治理配置模板(audit 5.2)。
//
// 目标:业务只维护**一份** Nacos 治理配置模板,kitex 与 hertz 两侧各自
// 适配消费,不再各管一套格式。
//
// 收敛路线(分两阶段):
//   - 阶段 1(本包):统一 schema + 原子 Store。hertz 侧直接消费
//     (限流/超时中间件、httpx 重试策略);kitex 侧继续用既有 per-category
//     dataID(NewGovernanceConfig),通过 Parse 对齐字段语义。
//   - 阶段 2:kitex NewGovernanceConfig 迁移到同一份模板,per-category
//     dataID 转为模板内 category 字段。
//
// 配置格式(JSON,Nacos dataId 建议 "{service}.governance"):
//
//	{
//	  "retry": { "svc:Method": {"max_attempts": 3, "backoff_ms": 50, "max_backoff_ms": 1000, "jitter_ms": 20} },
//	  "timeout_ms": { "svc:Method": 200, "svc": 500 },
//	  "circuit_breaker": { "svc": {"error_rate_threshold": 0.5, "min_requests": 100, "open_ms": 10000} },
//	  "limiter": { "POST:/order": {"rate": 100, "burst": 200} }
//	}
//
// key 匹配约定:精确优先("svc:Method" / "METHOD:path"),其后前缀最长匹配
// ("svc"),最后全局 ""(可选)。
package governance

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
)

// ErrInvalidConfig 是治理模板校验失败的 sentinel(用 errors.Is 判定)。
var ErrInvalidConfig = errors.New("governance: invalid config")

// RetryPolicy 是单个 key 的重试策略。
type RetryPolicy struct {
	// MaxAttempts 总尝试次数(含首次);<=1 表示不重试。
	MaxAttempts int `json:"max_attempts"`
	// BackOffMs 重试退避基数(毫秒,指数退避)。
	BackOffMs int `json:"backoff_ms,omitempty"`
	// MaxBackoffMs 退避上限(毫秒)。
	MaxBackoffMs int `json:"max_backoff_ms,omitempty"`
	// JitterMs 随机抖动上限(毫秒)。
	JitterMs int `json:"jitter_ms,omitempty"`
}

// CBPolicy 是单个 key 的熔断策略。
type CBPolicy struct {
	// ErrorRateThreshold 错误率阈值(0-1)。
	ErrorRateThreshold float64 `json:"error_rate_threshold"`
	// MinRequests 触发统计的最小请求数。
	MinRequests int64 `json:"min_requests"`
	// OpenMs 熔断开启时长(毫秒)。
	OpenMs int64 `json:"open_ms"`
}

// LimiterPolicy 是单个 key 的限流策略(令牌桶)。
type LimiterPolicy struct {
	// Rate 每秒允许的请求数。
	Rate int `json:"rate"`
	// Burst 桶容量(突发上限);0 表示取 Rate。
	Burst int `json:"burst,omitempty"`
}

// Config 是统一治理模板。所有 map 的 key 为
// "service:Method"(kitex)或 "METHOD:path"(hertz),省略冒号后缀表示
// service/path 级默认。
type Config struct {
	Retry          map[string]RetryPolicy   `json:"retry,omitempty"`
	TimeoutMs      map[string]int           `json:"timeout_ms,omitempty"`
	CircuitBreaker map[string]CBPolicy      `json:"circuit_breaker,omitempty"`
	Limiter        map[string]LimiterPolicy `json:"limiter,omitempty"`
}

// Parse 从 Nacos 配置内容(JSON)解析并校验治理模板。
func Parse(data []byte) (*Config, error) {
	cfg := &Config{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Validate 校验模板字段语义(非法值直接拒绝,避免坏配置毒化运行时)。
func (c *Config) Validate() error {
	for key, r := range c.Retry {
		if r.MaxAttempts < 0 || r.MaxAttempts > 10 {
			return fmt.Errorf("%w: retry[%q].max_attempts=%d out of range [0,10]", ErrInvalidConfig, key, r.MaxAttempts)
		}
		if r.BackOffMs < 0 || r.MaxBackoffMs < 0 || r.JitterMs < 0 {
			return fmt.Errorf("%w: retry[%q] negative backoff/jitter", ErrInvalidConfig, key)
		}
	}
	for key, ms := range c.TimeoutMs {
		// ms=0 拒绝：kitex 对 <=0 走"无超时"fast path（等价关闭超时保护，
		// server hang 时请求无限堆积）；hertz timeout 中间件 0 同样立即到
		// 期。清空超时用删除 timeout_ms 段表达，不靠 0 值。
		if ms <= 0 {
			return fmt.Errorf("%w: timeout_ms[%q]=%d must be positive", ErrInvalidConfig, key, ms)
		}
	}
	for key, cb := range c.CircuitBreaker {
		if cb.ErrorRateThreshold < 0 || cb.ErrorRateThreshold > 1 {
			return fmt.Errorf("%w: circuit_breaker[%q].error_rate_threshold out of (0,1)", ErrInvalidConfig, key)
		}
		if cb.OpenMs < 0 {
			return fmt.Errorf("%w: circuit_breaker[%q].open_ms negative", ErrInvalidConfig, key)
		}
	}
	for key, l := range c.Limiter {
		if l.Rate <= 0 {
			return fmt.Errorf("%w: limiter[%q].rate must be > 0", ErrInvalidConfig, key)
		}
		if l.Burst < 0 {
			return fmt.Errorf("%w: limiter[%q].burst negative", ErrInvalidConfig, key)
		}
	}
	return nil
}

// Store 是原子替换的配置持有者:消费方 Load 拿到的是不可变快照,
// 发布方 Update 整体替换 — 读侧无锁、无部分更新可见。
type Store struct {
	val     atomic.Pointer[Config]
	version atomic.Int64
}

// NewStore 用初始配置建 Store(nil 合法,视为空配置)。
func NewStore(cfg *Config) *Store {
	s := &Store{}
	if cfg == nil {
		cfg = &Config{}
	}
	s.val.Store(cfg)
	return s
}

// Load 返回当前配置快照(不可变;消费方不得修改)。
func (s *Store) Load() *Config { return s.val.Load() }

// Version 返回配置版本号(每次 Update +1),消费方可据此判断是否变化。
func (s *Store) Version() int64 { return s.version.Load() }

// Update 原子替换配置(nil 合法,等价空配置)并递增版本。
func (s *Store) Update(cfg *Config) {
	if cfg == nil {
		cfg = &Config{}
	}
	s.val.Store(cfg)
	s.version.Add(1)
}

// LookupTimeout 按 "svc:Method" → "svc" → "" 顺序取超时(毫秒);无命中返回 (0,false)。
func (c *Config) LookupTimeout(svc, method string) (int, bool) {
	return lookup(c.TimeoutMs, svc, method)
}

// LookupRetry 按 "svc:Method" → "svc" → "" 顺序取重试策略。
func (c *Config) LookupRetry(svc, method string) (RetryPolicy, bool) {
	v, ok := lookupPolicy(c.Retry, svc, method)
	return v, ok
}

// LookupLimiter 按 "METHOD:path" → "path" → "" 顺序取限流策略。
func (c *Config) LookupLimiter(method, path string) (LimiterPolicy, bool) {
	v, _, ok := lookupPolicyKey(c.Limiter, method+":"+path, path)
	return v, ok
}

// LookupLimiterKey 行为同 LookupLimiter,额外返回实际命中的配置键
// ("METHOD:path" / "path" / "")。消费方应优先用配置键(而非原始请求路径)
// 做限流桶分片——高基数 path(如 /order/123)在粗粒度(前缀/全局)策略下
// 若按原始 path 建桶,桶表会无界增长。
func (c *Config) LookupLimiterKey(method, path string) (LimiterPolicy, string, bool) {
	return lookupPolicyKey(c.Limiter, method+":"+path, path)
}

func lookup[T any](m map[string]T, svc, method string) (T, bool) {
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
	v, ok := m[""]
	return v, ok
}

func lookupPolicy[T any](m map[string]T, full, prefix string) (T, bool) {
	return lookup(m, full, prefix)
}

func lookupPolicyKey[T any](m map[string]T, full, prefix string) (T, string, bool) {
	var zero T
	if m == nil {
		return zero, "", false
	}
	if v, ok := m[full]; ok {
		return v, full, true
	}
	if v, ok := m[prefix]; ok {
		return v, prefix, true
	}
	v, ok := m[""]
	return v, "", ok
}
