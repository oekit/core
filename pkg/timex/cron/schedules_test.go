package cron

import (
	"testing"
	"time"
)

// TestWrapSchedule_Next verifies WrapSchedule.Next adds the delay
// to the wrapped schedule's next time.
func TestWrapSchedule_Next(t *testing.T) {
	base := ConstantDelaySchedule{Delay: time.Second}
	wrapped := WrapSchedule{Schedule: base, Delay: 500 * time.Millisecond}

	now := time.Now()
	got := wrapped.Next(now)
	want := base.Next(now).Add(500 * time.Millisecond)
	if !got.Equal(want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

// TestDelaySchedule_Next verifies DelaySchedule.Next adds the
// configured delay to t.
func TestDelaySchedule_Next(t *testing.T) {
	d := NewDelaySchedule(2 * time.Second)
	now := time.Now()
	got := d.Next(now)
	want := now.Add(2 * time.Second)
	if !got.Equal(want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

// TestNewDelaySchedule_StoresDelay verifies the constructor stores
// the delay value.
func TestNewDelaySchedule_StoresDelay(t *testing.T) {
	d := NewDelaySchedule(3 * time.Second)
	if d.Delay != 3*time.Second {
		t.Errorf("expected Delay=3s, got %v", d.Delay)
	}
}

// TestNewRandomDelaySchedule_StoresMinMax verifies the constructor
// stores min and max.
func TestNewRandomDelaySchedule_StoresMinMax(t *testing.T) {
	r := NewRandomDelaySchedule(100*time.Millisecond, 500*time.Millisecond)
	if r.Min != 100*time.Millisecond {
		t.Errorf("expected Min=100ms, got %v", r.Min)
	}
	if r.Max != 500*time.Millisecond {
		t.Errorf("expected Max=500ms, got %v", r.Max)
	}
}

// TestRandomDelaySchedule_Next_WithinRange verifies the random
// delay is always within [Min, Max] bounds.
func TestRandomDelaySchedule_Next_WithinRange(t *testing.T) {
	min, max := 100*time.Millisecond, 500*time.Millisecond
	r := NewRandomDelaySchedule(min, max)
	now := time.Now()

	for i := 0; i < 100; i++ {
		got := r.Next(now)
		delay := got.Sub(now)
		if delay < min || delay > max {
			t.Errorf("iteration %d: delay %v outside [%v, %v]", i, delay, min, max)
		}
	}
}

// TestRandomDelaySchedule_Next_ZeroDelta covers the edge case
// where Min == Max (delta == 0) — must return t + Min deterministically.
func TestRandomDelaySchedule_Next_ZeroDelta(t *testing.T) {
	r := NewRandomDelaySchedule(200*time.Millisecond, 200*time.Millisecond)
	now := time.Now()
	got := r.Next(now)
	want := now.Add(200 * time.Millisecond)
	if !got.Equal(want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

// TestRandomDelaySchedule_Next_NegativeDelta covers the edge case
// where Max < Min — must return t + Min (defensive).
func TestRandomDelaySchedule_Next_NegativeDelta(t *testing.T) {
	// max < min — delta is negative, falls into the "<=0" branch
	r := NewRandomDelaySchedule(500*time.Millisecond, 100*time.Millisecond)
	now := time.Now()
	got := r.Next(now)
	want := now.Add(500 * time.Millisecond) // Min
	if !got.Equal(want) {
		t.Errorf("expected %v (Min), got %v", want, got)
	}
}
