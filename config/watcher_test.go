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

// blockingSource 是可控读取时序的配置源：Read 返回 gate 通道放行前的旧内容。
type blockingSource struct {
	mu   sync.Mutex
	data []byte
}

func (b *blockingSource) Read() ([]byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.data, nil
}

func (b *blockingSource) Watch(ctx context.Context, onChange func()) {
	<-ctx.Done()
}

func (b *blockingSource) set(data []byte) {
	b.mu.Lock()
	b.data = data
	b.mu.Unlock()
}

// TestConfigWatcher_StaleReloadCannotOverwriteNewer 回归 core#81：Reload 的
// Read+parse 此前在 dispatchMu 之外，先发起的重载若解析慢，会在后发起的
// 重载（已 Swap 新值）之后完成 Swap，把旧配置盖回新配置之上。修法是按
// Read 完成时刻发代号，解析完成后代号落后即丢弃。用通道编排确定性复现：
// A 读到 gen1 后卡在慢解析；期间源更新为 gen2、B 完成重载；A 醒来必须
// 被丢弃——最终配置停在 gen2（源内容），而不是被 gen1 盖回去。
func TestConfigWatcher_StaleReloadCannotOverwriteNewer(t *testing.T) {
	src := &blockingSource{data: []byte(`{"name":"gen1"}`)}
	inGen1Parse := make(chan struct{})
	releaseGen1 := make(chan struct{})
	var slowArmed bool // 初始加载不走慢路径；B 启动前置位
	parser := func(d []byte) (*testConfig, error) {
		var cfg testConfig
		if slowArmed && string(d) == `{"name":"gen1"}` {
			// A 的解析慢：通知测试已进入，等待放行
			close(inGen1Parse)
			<-releaseGen1
		}
		err := json.Unmarshal(d, &cfg)
		return &cfg, err
	}
	w := NewConfigWatcher(src, parser)

	if err := w.Reload(); err != nil { // 初始加载 gen1
		t.Fatalf("initial Reload: %v", err)
	}
	if w.Get().Name != "gen1" {
		t.Fatalf("initial config = %q, want gen1", w.Get().Name)
	}

	// A：读 gen1，卡在慢解析
	slowArmed = true
	doneA := make(chan error, 1)
	go func() { doneA <- w.Reload() }()
	<-inGen1Parse

	// 源更新为 gen2；B 完成重载（解析快）
	src.set([]byte(`{"name":"gen2"}`))
	doneB := make(chan error, 1)
	go func() { doneB <- w.Reload() }()
	if err := <-doneB; err != nil {
		t.Fatalf("reload B: %v", err)
	}
	if got := w.Get().Name; got != "gen2" {
		t.Fatalf("after reload B config = %q, want gen2", got)
	}

	// 放行 A：它读到的是 gen1，必须被代次校验丢弃
	close(releaseGen1)
	if err := <-doneA; err != nil {
		t.Fatalf("reload A: %v", err)
	}
	if got := w.Get().Name; got != "gen2" {
		t.Errorf("stale reload A overwrote newer config: got %q, want gen2 (core#81)", got)
	}
}
