package uidgen

import (
	"sync"
	"testing"
	"time"
)

func TestInitSnowflake(t *testing.T) {
	resetForTest()
	defer resetForTest()

	if err := InitSnowflake(0); err != nil {
		t.Fatalf("first init: %v", err)
	}
	// 第二次 init 必须失败
	if err := InitSnowflake(1); err == nil {
		t.Error("expected error on second init")
	}
}

func TestInitSnowflake_OutOfRange(t *testing.T) {
	resetForTest()
	defer resetForTest()

	if err := InitSnowflake(-1); err == nil {
		t.Error("expected error for negative machineID")
	}
	if err := InitSnowflake(1024); err == nil {
		t.Error("expected error for machineID > 1023")
	}
	// 边界值必须 OK
	if err := InitSnowflake(0); err != nil {
		t.Errorf("machineID=0 should be valid: %v", err)
	}
	resetForTest()
	if err := InitSnowflake(1023); err != nil {
		t.Errorf("machineID=1023 should be valid: %v", err)
	}
}

func TestInitSnowflakeFromEnv(t *testing.T) {
	t.Run("empty falls back to 0", func(t *testing.T) {
		resetForTest()
		t.Setenv("MACHINE_ID", "")
		if err := InitSnowflakeFromEnv(); err != nil {
			t.Fatalf("init: %v", err)
		}
		if got := SnowflakeMachineID(); got != 0 {
			t.Errorf("expected machineID 0, got %d", got)
		}
	})

	t.Run("valid value", func(t *testing.T) {
		resetForTest()
		t.Setenv("MACHINE_ID", "5")
		if err := InitSnowflakeFromEnv(); err != nil {
			t.Fatalf("init: %v", err)
		}
		if got := SnowflakeMachineID(); got != 5 {
			t.Errorf("expected machineID 5, got %d", got)
		}
	})

	t.Run("invalid value falls back to 0", func(t *testing.T) {
		resetForTest()
		t.Setenv("MACHINE_ID", "not-a-number")
		if err := InitSnowflakeFromEnv(); err != nil {
			t.Fatalf("init: %v", err)
		}
		if got := SnowflakeMachineID(); got != 0 {
			t.Errorf("expected fallback machineID 0, got %d", got)
		}
	})

	t.Run("out-of-range value rejected", func(t *testing.T) {
		resetForTest()
		t.Setenv("MACHINE_ID", "9999")
		if err := InitSnowflakeFromEnv(); err == nil {
			t.Error("expected error for out-of-range MACHINE_ID")
		}
	})
}

func TestSnowflakeID_Basic(t *testing.T) {
	resetForTest()
	defer resetForTest()

	if err := InitSnowflake(7); err != nil {
		t.Fatal(err)
	}

	before := time.Now()
	id := SnowflakeID()
	after := time.Now()

	if id <= 0 {
		t.Errorf("expected positive ID, got %d", id)
	}

	// ID 里的时间戳应在 [before, after] 之间（提取后比对）
	ts := ExtractSnowflakeTime(id)
	if ts.Before(before.Truncate(time.Millisecond)) || ts.After(after.Add(time.Millisecond)) {
		t.Errorf("extracted time %v out of expected range [%v, %v]", ts, before, after)
	}
}

func TestSnowflakeID_AutoInit(t *testing.T) {
	resetForTest()
	defer resetForTest()
	// 不调 Init，直接用，应自动以 machineID=0 初始化
	id := SnowflakeID()
	if id <= 0 {
		t.Errorf("expected positive ID after auto-init, got %d", id)
	}
	if got := SnowflakeMachineID(); got != 0 {
		t.Errorf("auto-init should use machineID 0, got %d", got)
	}
}

func TestSnowflakeID_Uniqueness(t *testing.T) {
	resetForTest()
	defer resetForTest()

	if err := InitSnowflake(7); err != nil {
		t.Fatal(err)
	}

	seen := make(map[int64]struct{}, 10000)
	for i := 0; i < 10000; i++ {
		id := SnowflakeID()
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate ID: %d", id)
		}
		seen[id] = struct{}{}
	}
}

func TestSnowflakeID_Concurrent(t *testing.T) {
	resetForTest()
	defer resetForTest()

	if err := InitSnowflake(7); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	seen := make(map[int64]struct{})
	var wg sync.WaitGroup

	const goroutines = 100
	const perGoroutine = 100

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				id := SnowflakeID()
				mu.Lock()
				if _, dup := seen[id]; dup {
					t.Errorf("duplicate ID: %d", id)
				}
				seen[id] = struct{}{}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if got := len(seen); got != goroutines*perGoroutine {
		t.Errorf("expected %d unique IDs, got %d", goroutines*perGoroutine, got)
	}
}

func TestExtractSnowflakeFields(t *testing.T) {
	resetForTest()
	defer resetForTest()

	if err := InitSnowflake(42); err != nil {
		t.Fatal(err)
	}
	id := SnowflakeID()

	mid := ExtractSnowflakeMachineID(id)
	seq := ExtractSnowflakeSequence(id)
	ts := ExtractSnowflakeTime(id)

	if mid != 42 {
		t.Errorf("expected machineID 42, got %d", mid)
	}
	if seq != 0 {
		t.Errorf("expected sequence 0 on first ID, got %d", seq)
	}
	if ts.IsZero() {
		t.Error("expected non-zero time")
	}
	// ts 应在最近 5s 内
	if diff := time.Since(ts); diff < -time.Second || diff > 5*time.Second {
		t.Errorf("extracted time off by %v", diff)
	}
}

func TestSnowflakeID_SequenceIncrement(t *testing.T) {
	resetForTest()
	defer resetForTest()

	if err := InitSnowflake(1); err != nil {
		t.Fatal(err)
	}

	// Tight loop 强制同 ms: 在同一毫秒内连续取, sequence 必须严格递增.
	// 用 100ms budget, 同 ms 内至少能拿到几十个 ID.
	var ids []int64
	deadline := time.Now().Add(100 * time.Millisecond)
	for time.Now().Before(deadline) {
		ids = append(ids, SnowflakeID())
	}

	if len(ids) < 10 {
		t.Fatalf("expected to generate ≥10 IDs in 100ms, got %d", len(ids))
	}

	// 找一组同 ms 的连续 ID 断言 sequence 单调递增.
	sawSameMS := false
	for i := 1; i < len(ids); i++ {
		ts1 := ExtractSnowflakeTime(ids[i-1])
		ts2 := ExtractSnowflakeTime(ids[i])
		seq1 := ExtractSnowflakeSequence(ids[i-1])
		seq2 := ExtractSnowflakeSequence(ids[i])

		if ts1.Equal(ts2) {
			sawSameMS = true
			if seq2 <= seq1 {
				t.Fatalf("same-ms sequence not strictly increasing at i=%d: seq1=%d seq2=%d", i, seq1, seq2)
			}
		}
	}

	if !sawSameMS {
		t.Errorf("did not observe any same-ms consecutive IDs in %d samples; tight loop too slow", len(ids))
	}

	// 100ms 内 sequence 不应跨过 12-bit 边界 (4096/ms 远超 tight loop 速率).
	for _, id := range ids {
		if seq := ExtractSnowflakeSequence(id); seq > sequenceMask {
			t.Errorf("sequence %d exceeds mask %d", seq, sequenceMask)
		}
	}
}
