package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestManager_Shutdown_PicksUpLateAdd 回归 core#116：Shutdown 进入 once.Do 后
// 此前一次性拷贝 jobs 快照、只执行快照内的任务——closer 执行期间经 Add 注册
// 的 closer 被**静默漏调**（不进 shutdownErrs、Errors() 恒空、零日志；nacos
// 延迟注册形态下 = 注册中心幽灵实例存活到 ephemeral 心跳超时，流量打向已
// 停止的 server）。修后按 bootstrap closeAll 的收敛范式：每轮锁内取「尚未
// 执行的尾部增量」、锁外执行，直到取空——关停执行期间 Add 的任务补齐执行，
// 顺序保持注册序。
func TestManager_Shutdown_PicksUpLateAdd(t *testing.T) {
	m := New(5 * time.Second)
	var mu sync.Mutex
	var executed []string
	order := func(name string) {
		mu.Lock()
		executed = append(executed, name)
		mu.Unlock()
	}

	m.Add("first", func(ctx context.Context) error {
		order("first")
		// 模拟 nacos 延迟注册时序：Register 成功后才拿到反注册句柄——
		// 关停序列已开始，一批反注册 closer 在「first」执行期内才登记
		for i := range 8 {
			name := fmt.Sprintf("late%d", i)
			m.Add(name, func(ctx context.Context) error { order(name); return nil }, 0)
		}
		return nil
	}, 0)

	m.Shutdown(context.Background())

	mu.Lock()
	defer mu.Unlock()
	want := []string{"first", "late0", "late1", "late2", "late3", "late4", "late5", "late6", "late7"}
	if len(executed) != len(want) {
		t.Fatalf("关停执行期间 Add 的 closer 被漏调（core#116）: executed = %v, want %v", executed, want)
	}
	for i := range want {
		if executed[i] != want[i] {
			t.Fatalf("补齐执行应保持注册序: executed = %v, want %v", executed, want)
		}
	}
	if errs := m.Errors(); len(errs) != 0 {
		t.Errorf("补齐执行后 Errors() 应为空, got %v", errs)
	}
}

// TestManager_Shutdown_LateAddErrorsRecorded 补齐执行的 closer 失败要与快照内
// 任务同口径进 Errors()；运维侧对 Errors() 做 errors.Join 聚合后 errors.Is
// 逐一定位哨兵——漏调不再等于不可观测。
func TestManager_Shutdown_LateAddErrorsRecorded(t *testing.T) {
	m := New(5 * time.Second)
	errA := errors.New("late closer A failed")
	errB := errors.New("late closer B failed")

	m.Add("first", func(ctx context.Context) error {
		m.Add("lateA", func(ctx context.Context) error { return errA }, 0)
		m.Add("lateB", func(ctx context.Context) error { return errB }, 0)
		return nil
	}, 0)

	m.Shutdown(context.Background())

	errs := m.Errors()
	if len(errs) != 2 {
		t.Fatalf("补齐执行的 closer 失败要进 Errors(): got %d errs (%v), want 2", len(errs), errs)
	}
	// errors.Join 把切片并成一个值，Is/As 穿透到每个哨兵
	joined := errors.Join(errs...)
	if !errors.Is(joined, errA) || !errors.Is(joined, errB) {
		t.Errorf("Errors() 聚合后哨兵不可定位: joined = %v", joined)
	}
}

// TestManager_Shutdown_LateTeardownPickedUp AddTeardown 与 Add 是同一竞态的
// 两个面：teardown 执行期间经 AddTeardown 登记的函数同样被收敛循环补齐。
func TestManager_Shutdown_LateTeardownPickedUp(t *testing.T) {
	m := New(5 * time.Second)
	var mu sync.Mutex
	var ran []string
	record := func(name string) {
		mu.Lock()
		ran = append(ran, name)
		mu.Unlock()
	}

	m.AddTeardown(func() {
		record("first")
		m.AddTeardown(func() { record("late") })
	})

	m.Shutdown(context.Background())

	mu.Lock()
	defer mu.Unlock()
	if len(ran) != 2 || ran[0] != "first" || ran[1] != "late" {
		t.Fatalf("teardown 执行期间 AddTeardown 的函数被漏调: ran = %v, want [first late]", ran)
	}
	if errs := m.Errors(); len(errs) != 0 {
		t.Errorf("Errors() 应为空, got %v", errs)
	}
}
