package uidgen

import (
	"testing"
	"time"
)

// TestSnowflakeMachineID_NotInitialized covers the SnowflakeMachineID
// branch when no Init has been called.
func TestSnowflakeMachineID_NotInitialized(t *testing.T) {
	resetForTest()
	defer resetForTest()

	if got := SnowflakeMachineID(); got != -1 {
		t.Errorf("expected -1 for uninitialized snowflake, got %d", got)
	}
}

// TestSnowflake_Next_ClockBackward covers the busy-wait branch in
// snowflake.next() when time.Now() is behind lastTS.
func TestSnowflake_Next_ClockBackward(t *testing.T) {
	s := &Snowflake{
		epoch:     snowflakeEpoch,
		machineID: 1,
		lastTS:    time.Now().UnixMilli() + 5, // 5ms in the future
	}

	start := time.Now()
	id := s.next()
	elapsed := time.Since(start)

	if id <= 0 {
		t.Errorf("expected positive ID after clock-backward recovery, got %d", id)
	}
	// Should have busy-waited ~5ms; allow generous upper bound.
	if elapsed > 200*time.Millisecond {
		t.Errorf("next() took too long: %v (expected ~5ms)", elapsed)
	}
}

// TestSnowflake_Next_SequenceOverflow covers the same-ms sequence-exhaustion
// branch: pre-set sequence=4095, lastTS=now → first call increments to 0
// and busy-waits for the next ms.
func TestSnowflake_Next_SequenceOverflow(t *testing.T) {
	now := time.Now().UnixMilli()
	s := &Snowflake{
		epoch:     snowflakeEpoch,
		machineID: 2,
		lastTS:    now,
		sequence:  4095, // next increment will overflow to 0
	}

	id := s.next()
	if id <= 0 {
		t.Errorf("expected positive ID after sequence overflow, got %d", id)
	}
	// After overflow recovery, sequence should be 0 (the new ms starts at 0).
	if got := ExtractSnowflakeSequence(id); got != 0 {
		t.Errorf("expected sequence 0 after overflow + new-ms, got %d", got)
	}
}
