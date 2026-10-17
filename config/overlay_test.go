package config

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

// stubSource 测试用 ConfigSource。
type stubSource struct {
	data []byte
	err  error
	ch   chan struct{}
}

func (s *stubSource) Read() ([]byte, error) { return s.data, s.err }

func (s *stubSource) Watch(ctx context.Context, onChange func()) {
	if s.ch == nil {
		return
	}
	go func() {
		select {
		case <-ctx.Done():
		case <-s.ch:
			onChange()
		}
	}()
}

func TestOverlaySourceMergeOrder(t *testing.T) {
	base := &stubSource{data: []byte(`{"a":1,"b":2,"nested":{"x":1,"y":2}}`)}
	over := &stubSource{data: []byte(`{"b":3,"nested":{"y":9}}`)}
	src := OverlaySource(base, over)
	got, err := src.Read()
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(got, &m); err != nil {
		t.Fatal(err)
	}
	if m["a"].(float64) != 1 || m["b"].(float64) != 3 {
		t.Fatalf("scalar merge broken: %v", m)
	}
	nested := m["nested"].(map[string]any)
	if nested["x"].(float64) != 1 || nested["y"].(float64) != 9 {
		t.Fatalf("deep merge broken: %v", nested)
	}
}

func TestOverlaySourceSingleSource(t *testing.T) {
	src := OverlaySource(&stubSource{data: []byte(`{"a":1}`)})
	got, err := src.Read()
	if err != nil || string(got) != `{"a":1}` {
		t.Fatalf("got %s, %v", got, err)
	}
}

func TestOverlaySourceReadErrorPropagates(t *testing.T) {
	src := OverlaySource(
		&stubSource{data: []byte(`{"a":1}`)},
		&stubSource{err: errStub},
	)
	_, err := src.Read()
	if !errors.Is(err, errStub) {
		t.Fatalf("err = %v, want wrapped stub error", err)
	}
}

var errStub = &stubError{}

type stubError struct{}

func (*stubError) Error() string { return "stub" }

func TestOverlaySourceWatchFanIn(t *testing.T) {
	s1 := &stubSource{data: []byte(`{}`), ch: make(chan struct{}, 1)}
	s2 := &stubSource{data: []byte(`{}`), ch: make(chan struct{}, 1)}
	src := OverlaySource(s1, s2)
	fired := make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	src.Watch(ctx, func() { fired <- struct{}{} })

	s2.ch <- struct{}{} // 任一子源触发即触发
	select {
	case <-fired:
	case <-time.After(time.Second):
		t.Fatal("onChange not fired from second source")
	}

	s1.ch <- struct{}{}
	select {
	case <-fired:
	case <-time.After(time.Second):
		t.Fatal("onChange not fired from first source")
	}
}
