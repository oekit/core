package config

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/oekit/logx"
)

// captureLogger is a Logger implementation that records error calls.
// 仅实现 Error：watcher 只会调用 Error，其余方法由嵌入接口占位（不会被调用）。
type captureLogger struct {
	logx.Logger
	mu     sync.Mutex
	calls  []string
	fields [][]any
}

func (l *captureLogger) Error(msg string, keysAndValues ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, msg)
	l.fields = append(l.fields, keysAndValues)
}

func (l *captureLogger) lastMessage() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.calls) == 0 {
		return ""
	}
	return l.calls[len(l.calls)-1]
}

// errSource is a ConfigSource whose Read always fails.
type errSource struct{}

func (errSource) Read() ([]byte, error)                      { return nil, errors.New("read failed") }
func (errSource) Watch(ctx context.Context, onChange func()) { <-ctx.Done() }

func TestConfigWatcher_SetLogger(t *testing.T) {
	source := newMockSource([]byte(`{"port":1}`))
	parser := func(d []byte) (*testConfig, error) {
		var cfg testConfig
		return &cfg, json.Unmarshal(d, &cfg)
	}
	watcher := NewConfigWatcher(source, parser)

	cap := &captureLogger{}
	watcher.SetLogger(cap)

	// Verify SetLogger accepts the new logger (and Start uses it on
	// subsequent Watch-triggered reload failures — see
	// TestConfigWatcher_Watch_ReloadFailure).
	failSrc := errSource{}
	watcher2 := NewConfigWatcher(failSrc, parser)
	watcher2.SetLogger(cap)
	_ = watcher2
	_ = watcher
	_ = source
}

func TestConfigWatcher_Reload_ReadError(t *testing.T) {
	parser := func(d []byte) (*testConfig, error) {
		var cfg testConfig
		return &cfg, json.Unmarshal(d, &cfg)
	}
	watcher := NewConfigWatcher(errSource{}, parser)
	if err := watcher.Reload(); err == nil {
		t.Error("expected error from Read failure")
	}
}

func TestConfigWatcher_Reload_ParseError(t *testing.T) {
	source := newMockSource([]byte("not-json"))
	parser := func(d []byte) (*testConfig, error) {
		var cfg testConfig
		return &cfg, json.Unmarshal(d, &cfg)
	}
	watcher := NewConfigWatcher(source, parser)
	if err := watcher.Reload(); err == nil {
		t.Error("expected error from parse failure")
	}
}

func TestConfigWatcher_Start_ReloadFails(t *testing.T) {
	parser := func(d []byte) (*testConfig, error) {
		var cfg testConfig
		return &cfg, json.Unmarshal(d, &cfg)
	}
	watcher := NewConfigWatcher(errSource{}, parser)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := watcher.Start(ctx); err == nil {
		t.Error("expected Start to return error from initial Reload failure")
	}
}

func TestConfigWatcher_Watch_ReloadFailure(t *testing.T) {
	// A source whose Watch immediately fires the onChange callback with
	// a Read that fails. The Watch handler should call our logger.
	source := &watchErrorSource{}
	parser := func(d []byte) (*testConfig, error) {
		var cfg testConfig
		return &cfg, json.Unmarshal(d, &cfg)
	}
	watcher := NewConfigWatcher(source, parser)

	cap := &captureLogger{}
	watcher.SetLogger(cap)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Wait for the onChange callback to fire.
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if cap.lastMessage() != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if cap.lastMessage() == "" {
		t.Error("expected logger to receive error from watch-driven reload failure")
	}
}

// watchErrorSource calls Read successfully once (for initial load) and
// then on subsequent Watch callbacks returns an error from Read.
type watchErrorSource struct {
	called int
}

func (w *watchErrorSource) Read() ([]byte, error) {
	w.called++
	if w.called == 1 {
		return []byte(`{"port":1}`), nil
	}
	return nil, errors.New("read failed after init")
}

func (w *watchErrorSource) Watch(ctx context.Context, onChange func()) {
	// Fire onChange after a short delay so initial load can complete first.
	go func() {
		time.Sleep(20 * time.Millisecond)
		onChange()
	}()
	<-ctx.Done()
}

var _ = sync.Mutex{} // keep sync import for captureLogger use
