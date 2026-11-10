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
	"strings"
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
//
// 解析失败的错误链同时挂 ErrInvalidConfig 哨兵与 json.Unmarshal 的原始
// 错误（双 %w，core#118）——*json.SyntaxError / *json.UnmarshalTypeError
// 经 errors.As 可辨，UnmarshalTypeError 自带的 Struct/Field 结构化定位
// 不丢失，配置解析失败可按错误类型分流（语法损坏 vs 字段类型不符）。
// %v 会把链拍平成纯文本，两类失败不可区分（kitx#70 同款缺陷）。
func Parse(data []byte) (*Config, error) {
	cfg := &Config{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidConfig, err)
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
		// server hang 时请求无限堆积）。chain.Timeout(0) 现与 lifecycle 同口径
		// 定义为「显式无超时」（core#117），模板里的 0 不再承担「关闭超时」
		// 的歧义语义——清空超时用删除 timeout_ms 段表达，不靠 0 值。
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
//
// 值与版本绑在**同一个原子单元**里发布(单 atomic.Pointer[cell]):
// 拆成两个原子变量的实现,读者会在跨字段的交错窗口拿到「新值配旧代次」
// 的自相矛盾对(core#79 实测 20 万次 Update 观察到 345 次)——按
// Version()==last 判断变化的消费方会漏判,派生状态停留在旧配置。
// 同构范式见 config.Snapshot(单单元发布 + CAS 推进代次)。
type Store struct {
	cell atomic.Pointer[storeCell]
}

// storeCell 是发布值的单真相单元:值与版本同生同灭,读侧一次 Load
// 拿到的一致的一对,不存在「配置换过但版本没动」的窗口。
type storeCell struct {
	val *Config
	gen int64
}

// NewStore 用初始配置建 Store(nil 合法,视为空配置)。初始版本为 0,
// 此后每次 Update +1。入参在入库前深拷贝——发布方之后对 cfg 的任何
// 改写都不会波及已发布快照(见 Update)。
func NewStore(cfg *Config) *Store {
	s := &Store{}
	if cfg == nil {
		cfg = &Config{}
	}
	s.cell.Store(&storeCell{val: deepCopyConfig(cfg), gen: 0})
	return s
}

// Load 返回当前配置快照(不可变;消费方不得修改,发布方也改不到——
// Update 入库前深拷贝,发布方复用/改写入参不影响已发布快照)。
func (s *Store) Load() *Config { return s.cell.Load().val }

// Version 返回配置版本号(每次 Update +1),消费方可据此判断是否变化。
// 版本是发布单元的投影(与值同一次原子替换),因此「看到新值」必然
// 「看到新版本」;Load() 与 Version() 两次调用之间若插进一次 Update,
// 版本只会**偏新**不会偏旧——最多多重建一次派生状态,不会漏判
// (core#79)。要一次拿到一致的一对请用 LoadVersion。
func (s *Store) Version() int64 { return s.cell.Load().gen }

// LoadVersion 一次返回一致的一对:当前配置与它的版本号。需要「拿到值的
// 同时知道它是第几代」(典型:据此决定派生状态要不要重建)时用它,而不是
// Load() + Version() 两次调用——两次调用之间可能插进一次 Update,拿到的
// 值与版本来自不同代。
func (s *Store) LoadVersion() (*Config, int64) {
	c := s.cell.Load()
	return c.val, c.gen
}

// Update 原子替换配置(nil 合法,等价空配置)并递增版本。
//
// 入参**入库前深拷贝**(core#80):此前只存指针,发布方在 Update 之后继续
// 改自己的 cfg(复用同一结构体补字段、异步补默认值),已发布快照被静默
// 改写而版本不前进——消费方按版本判「没变化」,永久用旧派生状态;并发下
// 发布方写 map 与消费方读 map 构成数据竞争。深拷贝后发布方怎么改入参
// 都不影响快照,「不可变快照」承诺对发布方与消费方同时成立。
//
// 并发安全:多个发布者同时调用,每次成功发布恰好推进一版(CAS 循环),
// 版本不会重复、不会回退。
func (s *Store) Update(cfg *Config) {
	if cfg == nil {
		cfg = &Config{}
	}
	next := &storeCell{val: deepCopyConfig(cfg)}
	for {
		cur := s.cell.Load()
		next.gen = cur.gen + 1
		if s.cell.CompareAndSwap(cur, next) {
			return
		}
	}
}

// deepCopyConfig 深拷贝治理模板的四个 map(值类型元素,一层条目拷贝即完整)。
// nil map 保持 nil——与零值 Config 的形态一致,Lookup 侧对 nil map 已有兜底。
func deepCopyConfig(cfg *Config) *Config {
	out := &Config{}
	if cfg == nil {
		return out
	}
	if cfg.Retry != nil {
		out.Retry = make(map[string]RetryPolicy, len(cfg.Retry))
		for k, v := range cfg.Retry {
			out.Retry[k] = v
		}
	}
	if cfg.TimeoutMs != nil {
		out.TimeoutMs = make(map[string]int, len(cfg.TimeoutMs))
		for k, v := range cfg.TimeoutMs {
			out.TimeoutMs[k] = v
		}
	}
	if cfg.CircuitBreaker != nil {
		out.CircuitBreaker = make(map[string]CBPolicy, len(cfg.CircuitBreaker))
		for k, v := range cfg.CircuitBreaker {
			out.CircuitBreaker[k] = v
		}
	}
	if cfg.Limiter != nil {
		out.Limiter = make(map[string]LimiterPolicy, len(cfg.Limiter))
		for k, v := range cfg.Limiter {
			out.Limiter[k] = v
		}
	}
	return out
}

// LookupTimeout 按 "svc:Method" → "svc" → service 前缀最长匹配("com.example"
// 命中 "com.example.svc",按「.」段边界) → "" 顺序取超时(毫秒);无命中返回 (0,false)。
func (c *Config) LookupTimeout(svc, method string) (int, bool) {
	return lookup(c.TimeoutMs, svc, method)
}

// LookupRetry 顺序同 LookupTimeout,取重试策略。
func (c *Config) LookupRetry(svc, method string) (RetryPolicy, bool) {
	v, ok := lookupPolicy(c.Retry, svc, method)
	return v, ok
}

// LookupLimiter 按 "METHOD:path" → "path" → path 前缀最长匹配(按「/」段边界,
// method 限定的 "GET:/order" 优先于裸 "/order") → "" 顺序取限流策略。
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
	// 前缀档（core#78）：service 按「.」段前缀从长到短匹配——com.example
	// 命中 com.example.svc。没有这一档时，按 service 前缀统一配置的治理
	// 策略对高基数 service 名静默失效（与包 doc「前缀最长匹配」契约矛盾）。
	for _, p := range segmentPrefixes(svc, ".") {
		if v, ok := m[p]; ok {
			return v, true
		}
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
	// 前缀档（core#78）：path 按「/」段前缀从长到短逐档试，每档先试
	// method 限定键（"GET:/order" 比 "/order" 更具体，命中即返回），再试
	// 裸 path 键。命中的是**配置键**而非原始 path：LookupLimiterKey 借此
	// 把粗粒度键交给消费方做限流桶分片（桶表有界）。
	method := strings.TrimSuffix(full, ":"+prefix)
	for _, p := range segmentPrefixes(prefix, "/") {
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

// segmentPrefixes 返回 s 按段分隔符 sep 切分的全部**真段前缀**（不含 s 自身
// 与空串），从长到短：/order/123 → ["/order"]，com.example.svc →
// ["com.example", "com"]。「段」边界是硬约束——/orders 不会为 /order 产生
// 候选（分隔符必须落在段尾），com.examplex 也不会为 com.example 产生候选。
// 起始分隔符（path 开头的 "/"）不算边界（i == 0 跳过），否则会产出 ""。
func segmentPrefixes(s, sep string) []string {
	var out []string
	for i := 0; i < len(s); i++ {
		if i > 0 && s[i] == sep[0] {
			out = append(out, s[:i])
		}
	}
	// 从长到短：候选按产生序是短→长，倒置后先试最具体的前缀。
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}
