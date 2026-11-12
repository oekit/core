package lifecycle

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestShutdownConcurrentAddAndShutdown exercises the data race between
// Shutdown reading m.jobs (without m.mu) and Add writing m.jobs (under m.mu).
// The race window is between m.mu.Unlock() after copying teardowns and the
// range m.jobs evaluation. Run with -race to verify.
func TestShutdownConcurrentAddAndShutdown(t *testing.T) {
	for range 100 {
		m := New(time.Second)
		m.Add("first", func(ctx context.Context) error { return nil }, 0)

		var wg sync.WaitGroup

		// Concurrently add jobs while Shutdown reads m.jobs without lock.
		wg.Go(func() {
			for range 50 {
				m.Add("concurrent", func(ctx context.Context) error { return nil }, 0)
			}
		})

		wg.Go(func() {
			m.Shutdown(context.Background())
		})

		wg.Wait()
	}
}

// TestShutdownConcurrentErrors exercises the data race between Shutdown
// appending to m.shutdownErrs (without lock) and Errors reading m.shutdownErrs
// (without lock).
func TestShutdownConcurrentErrors(t *testing.T) {
	for range 100 {
		m := New(time.Second)
		m.Add("fail", func(ctx context.Context) error {
			return context.DeadlineExceeded
		}, 0)

		var wg sync.WaitGroup

		wg.Go(func() {
			m.Shutdown(context.Background())
		})

		wg.Go(func() {
			// Continuously read Errors while Shutdown appends to shutdownErrs.
			for range 50 {
				_ = m.Errors()
			}
		})

		wg.Wait()
	}
}
