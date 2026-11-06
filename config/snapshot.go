package config

import (
	"fmt"
	"sync/atomic"
)

// Snapshot 是一个**已解析配置值**的原子发布/无判空读取单元。
//
// 它解决的是一类随处可见的重复样板：热更回调（Nacos/K8s ConfigMap/文件 watch/
// 管理端点 POST）里把新配置原子发布出去，读取方每次读都要判空兜底——
//
//	var configSnapshot atomic.Pointer[Config]        // 各服务各写一份
//
//	func SetConfig(c *Config) { configSnapshot.Store(c) }
//	func GetConfig() *Config {
//	    if c := configSnapshot.Load(); c != nil { return c }
//	    return &Config{}                             // 兜底口径各服务自定
//	}
//
// 这套手搓有三个每次都要重新回答的问题，而每个服务的答案都不一样：
//
//  1. **未发布过时读到什么**——返回零值？返回包级默认？panic？答案是「零值」时，
//     读取方会在配置还没到的那几百毫秒里用一个**空配置**正常提供服务（端口为 0、
//     超时为 0 意味着不超时），失效方向是静默的。
//  2. **读取方要不要判空**——只要 Get 可能返 nil，每个读取点都要写一次判空，
//     判空样板随消费点线性扩散；漏写一处就是 nil 解引用 panic。
//  3. **「配置换过没有」**——拿指针比对上一条（`if c != last`）看起来能用，但它
//     回答的是「指针不同」而不是「发布过 n 次」：同一份配置被重新解码后发布
//     （Nacos 长轮询拿到相同内容也会回调）、或 A→B→A 的 ABA 序列，都会让指针
//     比对给出错误答案。
//
// Snapshot 把这三件事钉死成一份实现：
//
//	// 构造期注入默认值 ⇒ Get 永不返 nil，读取方零判空
//	snap := config.NewSnapshot(&defaults)
//
//	// Publish 的签名就是 func(*T) ⇒ 能直接当回调传下去
//	client.Listen(snap.Publish)
//
//	// 读取侧
//	cur := snap.Get()          // 永不为 nil
//	val, gen := snap.Load()    // 值与代次的**一致**一对
//	if snap.Version() > seen { // 换过没有：单调代次，不吃 ABA
//	    rebuild(val)
//	    seen = gen
//	}
//
// # 契约
//
//   - **Get 永不返 nil**。默认值在构造期注入；`NewSnapshot` 传 nil 直接 panic
//     （构造期错误在启动时炸，比在第一个请求上炸便宜）。
//   - **值与代次绑在同一个单元里发布**，因此 `Load` 拿不到「新值配旧代次」这种
//     自相矛盾的一对（把值与代次拆成两个原子变量的实现，读者能同时看到新旧两半，
//     而症状是「配置换过了但读到旧值」，比崩溃难查得多）。
//   - **代次是发布值的投影，不是第二个计数器**：每次成功发布从**观察到的**上一代
//     推导下一代（CAS 循环），并发发布者各自重试 ⇒ 代次唯一、单调、不回卷。
//     默认值的代次是 0。
//   - **发布出去的值视为不可变**：`Get` 返回的就是发布的那个指针（零拷贝），发布者
//     在 Publish 之后**不得再改**它的字段——就地修改会让读取方看到撕裂状态（数据
//     竞争），而这类竞争不会稳定复现。要改就先拷一份再发布。
//   - **Publish(nil) 被忽略**：保持上一份好值，代次不前进。理由见 Publish 注释。
//
// # 与 ConfigWatcher 的分工
//
// `ConfigWatcher[T]` 管的是**来源**：Read/Watch/解析/重载节奏，它自己持有当前值，
// 且 `Get` 在一开始还没有值时会返 nil（它必须先 Reload 一次）。
// `Snapshot[T]` 管的是**值**：原子发布与无判空读取，不认识来源、不解析、不重载。
// 两者可组合——需要「源驱动的自动重载 + 无判空读取」时，在
// `ConfigWatcher.OnChange` 里把新值 `snap.Publish` 出去即可；只需要「别人给我值，
// 我原子发布」时（Nacos Listen 回调、管理端点、K8s informer），只要 Snapshot。
//
// Snapshot 的**零值不可用**：内部单元未初始化，`Get`/`Load`/`Version` 会在空指针上
// panic（不是返 nil——nil 会让判空样板重新扩散回来）。一律用 NewSnapshot 构造。
type Snapshot[T any] struct {
	cell atomic.Pointer[snapshotCell[T]]
}

// snapshotCell 是发布值的**单真相**单元。
//
// 值与代次必须同生同灭：拆成两个原子变量时，读侧的一次 Load 会在跨字段的交错
// 窗口里读到「新值 + 旧代次」或「旧值 + 新代次」，而 Snapshot 存在的理由之一就是
// 让「换过没有」有唯一答案。
type snapshotCell[T any] struct {
	val *T
	gen uint64
}

// NewSnapshot 用一份默认配置构造 Snapshot。
//
// 默认值同时承担两件事：未发布过时 `Get` 返回它，以及「发布过几次」的基线
// （代次 0）。因此传达「还没有配置」这件事**不能**用 nil——想知道「外界发布过
// 没有」，看 `Version() > 0`。
//
// defaults 为 nil 时 panic：Get 永不返 nil 是这个类型的全部意义，允许 nil 默认值
// 等于让契约在运行期静默失效。
func NewSnapshot[T any](defaults *T) *Snapshot[T] {
	if defaults == nil {
		panic("config: NewSnapshot 的 defaults 不能为 nil（Get 永不返 nil 是本类型的契约；" +
			"想表达「还没有配置」请用 Version() > 0 判断）")
	}
	s := &Snapshot[T]{}
	s.cell.Store(&snapshotCell[T]{val: defaults, gen: 0})
	return s
}

// Get 返回当前发布的配置，**永不返 nil**（未发布过时返构造期的默认值）。
//
// 返回的是发布者交进来的那个指针（零拷贝）——读取方**不得修改**它，发布者也
// 不得在 Publish 之后再改它。要按配置派生一份可变副本，自行浅拷贝。
func (s *Snapshot[T]) Get() *T {
	return s.cell.Load().val
}

// Load 返回**一致的一对**：当前配置与它的代次。
//
// 需要「拿到值的同时知道它是第几代」（典型：据此决定派生状态要不要重建）时必须
// 用它而不是 `Get()` + `Version()` 两次调用——两次调用之间可能插进一次发布，
// 于是拿到的值与代次来自不同代（仓内同类教训：快照 gen 与全局 gen 自相矛盾的一对）。
func (s *Snapshot[T]) Load() (*T, uint64) {
	c := s.cell.Load()
	return c.val, c.gen
}

// Version 返回当前配置的代次：默认值 0，此后每次**成功**发布 +1。
//
// 它是发布单元的投影（不是独立计数器），因此并发发布下依然单调、唯一、不回卷；
// 用来回答「换过没有」而不是「值是不是同一个」（指针比对会被 ABA 与「相同内容
// 重新发布」骗过）。
func (s *Snapshot[T]) Version() uint64 {
	return s.cell.Load().gen
}

// Publish 原子发布一份新配置。签名是 `func(*T)`，可直接作为变更回调传入
// （`client.Listen(snap.Publish)`）。
//
// 并发安全：多个发布者同时调用，每次成功发布恰好推进一代（CAS 循环，观察到的
// 上一代推导下一代），代次不会重复、不会回退。
//
// **nil 被忽略**：保持上一份好值、代次不前进。这是个刻意的取舍——
//
//   - 契约要求 Get 永不返 nil，而 Publish 必须**没有返回值**才能当回调直接传，
//     所以「拒绝了」这件事无法经返回值告诉调用方；
//   - 能发出 nil 的只有调用方自己的解码器（`parser` 忘了返回错误、管理端点收了
//     空 body）或第三方 SDK 的兜底回调，而 Publish 最常被调用的位置是**别人 SDK
//     的 goroutine**——那里没有我们的 recover，一次 panic 就是进程级退出；
//   - 两种失效方向不对称：保留上一份好值 ⇒ 配置停在旧版本（服务继续跑，可观测）；
//     panic ⇒ 进程死掉。选前者，并把它变成可观测的：**看 Version 有没有前进**。
//
// 想区分「没有发布」与「发布了但被拒」的服务，请在调用 Publish **之前**校验
// 解码结果（那才是知识所在的位置），不要指望这里报告。
func (s *Snapshot[T]) Publish(v *T) {
	if v == nil {
		return
	}
	for {
		cur := s.cell.Load()
		next := &snapshotCell[T]{val: v, gen: cur.gen + 1}
		if s.cell.CompareAndSwap(cur, next) {
			return
		}
	}
}

// String 便于在日志里打印当前代次（不含配置内容——配置里常有密钥，而
// fmt 的 %v 会把它们一并打出来）。
func (s *Snapshot[T]) String() string {
	val, gen := s.Load()
	return fmt.Sprintf("config.Snapshot[%T]{gen:%d,val:%p}", val, gen, val)
}
