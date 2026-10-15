package config

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

type testConfig struct {
	Port  int    `json:"port"`
	Debug bool   `json:"debug"`
	Name  string `json:"name"`
}

// mockSource 是一个模拟的配置源。
type mockSource struct {
	data     []byte
	mu       sync.RWMutex
	onChange func()
}

func newMockSource(data []byte) *mockSource {
	return &mockSource{data: data}
}

func (m *mockSource) Read() ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.data, nil
}

func (m *mockSource) Watch(ctx context.Context, onChange func()) {
	m.mu.Lock()
	m.onChange = onChange
	m.mu.Unlock()

	// 模拟监听：等待 context 取消
	<-ctx.Done()
}

func (m *mockSource) Update(data []byte) {
	m.mu.Lock()
	m.data = data
	onChange := m.onChange
	m.mu.Unlock()

	if onChange != nil {
		onChange()
	}
}

func TestConfigWatcher_Get(t *testing.T) {
	data := []byte(`{"port":8080,"debug":true,"name":"test"}`)
	source := newMockSource(data)

	parser := func(d []byte) (*testConfig, error) {
		var cfg testConfig
		return &cfg, json.Unmarshal(d, &cfg)
	}

	watcher := NewConfigWatcher(source, parser)
	if err := watcher.Reload(); err != nil {
		t.Fatalf("Reload failed: %v", err)
	}

	cfg := watcher.Get()
	if cfg == nil {
		t.Fatal("expected config to be non-nil")
	}
	if cfg.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Port)
	}
	if !cfg.Debug {
		t.Error("expected debug to be true")
	}
	if cfg.Name != "test" {
		t.Errorf("expected name 'test', got %q", cfg.Name)
	}
}

func TestConfigWatcher_Reload(t *testing.T) {
	data := []byte(`{"port":8080}`)
	source := newMockSource(data)

	parser := func(d []byte) (*testConfig, error) {
		var cfg testConfig
		return &cfg, json.Unmarshal(d, &cfg)
	}

	watcher := NewConfigWatcher(source, parser)
	if err := watcher.Reload(); err != nil {
		t.Fatalf("Reload failed: %v", err)
	}

	// 更新配置
	source.Update([]byte(`{"port":9090}`))
	if err := watcher.Reload(); err != nil {
		t.Fatalf("Reload failed: %v", err)
	}

	cfg := watcher.Get()
	if cfg.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Port)
	}
}

func TestConfigWatcher_OnChange(t *testing.T) {
	data := []byte(`{"port":8080}`)
	source := newMockSource(data)

	parser := func(d []byte) (*testConfig, error) {
		var cfg testConfig
		return &cfg, json.Unmarshal(d, &cfg)
	}

	watcher := NewConfigWatcher(source, parser)

	var called bool
	var mu sync.Mutex
	watcher.OnChange(func(old, new *testConfig) {
		mu.Lock()
		called = true
		mu.Unlock()
	})

	// 初始加载
	if err := watcher.Reload(); err != nil {
		t.Fatalf("Reload failed: %v", err)
	}

	// 初始加载不应该触发回调
	mu.Lock()
	if called {
		t.Error("OnChange should not be called on first load")
	}
	mu.Unlock()

	// 更新配置
	source.Update([]byte(`{"port":9090}`))
	if err := watcher.Reload(); err != nil {
		t.Fatalf("Reload failed: %v", err)
	}

	// 第二次加载应该触发回调
	mu.Lock()
	if !called {
		t.Error("OnChange should be called on second load")
	}
	mu.Unlock()
}

func TestConfigWatcher_Start(t *testing.T) {
	data := []byte(`{"port":8080}`)
	source := newMockSource(data)

	parser := func(d []byte) (*testConfig, error) {
		var cfg testConfig
		return &cfg, json.Unmarshal(d, &cfg)
	}

	watcher := NewConfigWatcher(source, parser)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Start 后应该有初始配置
	cfg := watcher.Get()
	if cfg == nil {
		t.Fatal("expected config to be non-nil after Start")
	}
	if cfg.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Port)
	}
}
