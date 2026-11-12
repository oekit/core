package config

import (
	"strings"
	"sync"
	"testing"
)

// snapConfig 是快照用例的载荷：Seq 记录「这是第几次发布写入的值」，
// 因此读者可以自证「拿到的值」与「拿到的代次」属于同一个单元。
type snapConfig struct {
	Seq  uint64
	Name string
}

func TestNewSnapshot_PanicsOnNilDefaults(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("NewSnapshot(nil) 必须 panic——允许 nil 默认值等于让 Get 永不返 nil 的契约在运行期静默失效")
		}
		if msg, ok := r.(string); ok && !strings.Contains(msg, "defaults") {
			t.Fatalf("panic 文案应指出 defaults 有问题，实际: %q", msg)
		}
	}()
	_ = NewSnapshot[snapConfig](nil)
}

func TestSnapshot_ZeroValueUnusable(t *testing.T) {
	// 零值不是「空快照」而是「坏快照」：读路径必须在空单元上立刻炸，
	// 不能悄悄返 nil（返 nil 等于把判空样板放回每个读取点）。
	defer func() {
		if recover() == nil {
			t.Fatal("零值 Snapshot 的 Get 必须 panic（空单元），不得返 nil")
		}
	}()
	var s Snapshot[snapConfig]
	_ = s.Get()
}

func TestSnapshot_GetReturnsDefaultsBeforePublish(t *testing.T) {
	def := &snapConfig{Seq: 0, Name: "defaults"}
	s := NewSnapshot(def)

	if got := s.Get(); got != def {
		t.Fatalf("未发布过时 Get 应返回构造期默认值本体，实际 %p want %p", got, def)
	}
	if got := s.Version(); got != 0 {
		t.Fatalf("默认值代次应为 0，实际 %d", got)
	}
	if got := s.Get(); got == nil {
		t.Fatal("Get 永不返 nil")
	}
}

func TestSnapshot_PublishVisibleAndAdvancesVersion(t *testing.T) {
	s := NewSnapshot(&snapConfig{Name: "defaults"})

	for i := 1; i <= 3; i++ {
		v := &snapConfig{Seq: uint64(i)}
		s.Publish(v)

		if got := s.Get(); got != v {
			t.Fatalf("第 %d 次发布后 Get 应返回发布的指针本体，实际 %p want %p", i, got, v)
		}
		if got := s.Version(); got != uint64(i) {
			t.Fatalf("第 %d 次发布后代次应为 %d，实际 %d", i, i, got)
		}
		val, gen := s.Load()
		if val != v || gen != uint64(i) {
			t.Fatalf("Load 返回的不是一致的一对: val=%p want %p, gen=%d want %d", val, v, gen, i)
		}
	}
}

func TestSnapshot_PublishIsCallbackShaped(t *testing.T) {
	// 这条是编译期契约：Publish 的签名必须能直接当 `Listen(func(*T))` 的回调传下去，
	// 否则每个接热更的服务都要多写一层闭包——而闭包正是「忘了判 nil」的高发点。
	s := NewSnapshot(&snapConfig{})
	// 参数类型就是热更回调的形态；传不进去就编译失败。
	asListenCallback(s.Publish, &snapConfig{Seq: 7})
	if got := s.Get().Seq; got != 7 {
		t.Fatalf("经 func(*T) 回调发布后值应为 7，实际 %d", got)
	}
}

func TestSnapshot_NilPublishKeepsLastGood(t *testing.T) {
	def := &snapConfig{Name: "defaults"}
	s := NewSnapshot(def)
	good := &snapConfig{Seq: 1, Name: "good"}
	s.Publish(good)

	s.Publish(nil)

	if got := s.Get(); got != good {
		t.Fatalf("nil 发布必须保留上一份好值，实际 %p want %p", got, good)
	}
	if got := s.Version(); got != 1 {
		t.Fatalf("nil 发布不得推进代次（否则「换过没有」会被空发布骗到），实际 %d", got)
	}
}

// TestSnapshot_LoadPairConsistentUnderConcurrentPublish 是「单真相」的核心断言：
// 把值与代次拆成两个原子变量时，读者在两次 Load 之间跨过一次发布，就会拿到
// 「新值配旧代次」或「旧值配新代次」，症状是「配置换过了但读到旧值」。
// 载荷自带 Seq（这里发布者只有一个 ⇒ Seq 与该次发布的代次同号），因此每个读者
// 都能自证拿到的一对属于同一代。
func TestSnapshot_LoadPairConsistentUnderConcurrentPublish(t *testing.T) {
	s := NewSnapshot(&snapConfig{Seq: 0, Name: "defaults"})

	const rounds = 20000
	var stop sync.WaitGroup
	stop.Go(func() {
		for i := 1; i <= rounds; i++ {
			s.Publish(&snapConfig{Seq: uint64(i)})
		}
	})

	var readers sync.WaitGroup
	done := make(chan struct{})
	for range 4 {
		readers.Go(func() {
			for {
				select {
				case <-done:
					return
				default:
				}
				val, gen := s.Load()
				if val == nil {
					t.Error("Load 返回了 nil 值")
					return
				}
				if val.Seq != gen {
					t.Errorf("值与代次来自不同代: val.Seq=%d gen=%d", val.Seq, gen)
					return
				}
			}
		})
	}

	stop.Wait()
	close(done)
	readers.Wait()

	if got := s.Version(); got != rounds {
		t.Fatalf("连续发布 %d 次后代次应为 %d，实际 %d", rounds, rounds, got)
	}
	if val, gen := s.Load(); val.Seq != gen {
		t.Fatalf("收尾时值与代次仍须一致: val.Seq=%d gen=%d", val.Seq, gen)
	}
}

// TestSnapshot_ConcurrentPublishUniqueVersions 判「代次不得丢」：非原子的
// 读-改-写（读出上一代 → 各算各的 → 覆盖写）会让两个并发发布者拿到同一个代次，
// 而代次是「配置换过没有」的唯一凭据——代次重复 ⇒ 轮询方看不出配置换过。
func TestSnapshot_ConcurrentPublishUniqueVersions(t *testing.T) {
	s := NewSnapshot(&snapConfig{Name: "defaults"})

	const publishers, rounds = 4, 2000
	var wg sync.WaitGroup
	for p := range publishers {
		wg.Go(func() {
			for i := 1; i <= rounds; i++ {
				s.Publish(&snapConfig{Seq: uint64(p*rounds + i)})
			}
		})
	}
	wg.Wait()

	if got, want := s.Version(), uint64(publishers*rounds); got != want {
		t.Fatalf("并发发布 %d 次后代次应为 %d（不得丢代次/回卷），实际 %d", want, want, got)
	}
}

func TestSnapshot_ConcurrentReadersAndPublisher(t *testing.T) {
	// -race 下的读写混跑：这条不判语义，只兜「读路径动了不该动的锁/字段」。
	s := NewSnapshot(&snapConfig{Name: "defaults"})
	var wg sync.WaitGroup
	wg.Go(func() {
		for i := 1; i <= 500; i++ {
			s.Publish(&snapConfig{Seq: uint64(i)})
		}
	})
	for range 4 {
		wg.Go(func() {
			for range 500 {
				if s.Get() == nil {
					t.Error("Get 返 nil")
					return
				}
				_ = s.Version()
				_, _ = s.Load()
			}
		})
	}
	wg.Wait()
}

func TestSnapshot_StringOmitsConfigContent(t *testing.T) {
	// 配置里常有密钥/口令，把 Snapshot 直接丢进日志时必须只出代次。
	s := NewSnapshot(&snapConfig{Name: "defaults"})
	s.Publish(&snapConfig{Seq: 1, Name: "SECRET-TOKEN-VALUE"})

	got := s.String()
	if strings.Contains(got, "SECRET-TOKEN-VALUE") {
		t.Fatalf("String 不得打印配置内容，实际 %q", got)
	}
	if !strings.Contains(got, "gen:1") {
		t.Fatalf("String 应带上代次便于排查「换过没有」，实际 %q", got)
	}
}

// asListenCallback 只活在类型层：它的第一个参数类型正是 Nacos 那类
// `Listen(func(*T))` 回调的形态。
func asListenCallback[V any](fn func(*V), v *V) { fn(v) }
