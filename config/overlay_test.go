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

// TestApplyEnvOverlaySliceElementEnv 回归 core#85：overlay 此前是空结构体，
// 切片全 nil ⇒ ApplyEnv 遍历 0 个元素 ⇒ 元素字段上的 env tag 与覆盖表条目
// （items.limit，yaml 键路径不编号）静默失效——与 ApplyEnv（yaml 优先路径
// 元素存在、env 生效）行为相反。现在 overlay 按 dst 切片长度构造零值元素，
// env 命中即覆盖（含 dst 已有非零值），未命中的元素字段保持 dst 值。
func TestApplyEnvOverlaySliceElementEnv(t *testing.T) {
	type item struct {
		Limit int    `yaml:"limit" env:"ITEM_LIMIT"`
		Name  string `yaml:"name"`
	}
	type cfg struct {
		Items []item `yaml:"items"`
	}
	mkDst := func() *cfg {
		return &cfg{Items: []item{{Limit: 1, Name: "first"}, {Limit: 2, Name: "second"}}}
	}

	// env 命中即覆盖：dst 元素字段非零也被 env 压过（12-factor env 优先）。
	// 每个场景用独立的 dst——元素合并在 dst 元素上就地写，共享底层切片的
	// 副本会互相看见（生产方传的就是自己的 dst 指针，无此混淆）。
	t.Setenv("ITEM_LIMIT", "9")
	got := mkDst()
	if err := ApplyEnvOverlay(got); err != nil {
		t.Fatalf("ApplyEnvOverlay: %v", err)
	}
	if got.Items[0].Limit != 9 {
		t.Errorf("element env not applied: Items[0].Limit=%d, want 9 (core#85)", got.Items[0].Limit)
	}
	if got.Items[1].Limit != 9 {
		t.Errorf("element env not applied to Items[1]: %d, want 9", got.Items[1].Limit)
	}
	// 未命中字段保持 dst 值（不是整块替换清零）
	if got.Items[0].Name != "first" || got.Items[1].Name != "second" {
		t.Errorf("non-env element fields were wiped: %+v", got.Items)
	}

	// env 未设置：dst 完全不变（构造的零值元素已回缩，不触发整体替换）
	t.Setenv("ITEM_LIMIT", "")
	unchanged := mkDst()
	if err := ApplyEnvOverlay(unchanged); err != nil {
		t.Fatalf("ApplyEnvOverlay: %v", err)
	}
	if unchanged.Items[0].Limit != 1 || unchanged.Items[1].Limit != 2 ||
		unchanged.Items[0].Name != "first" {
		t.Errorf("unset env changed dst: %+v", unchanged.Items)
	}

	// []*struct 元素同样生效；env 未设置时指针元素不被 nil 化
	type pitem struct {
		Limit int `yaml:"limit" env:"ITEM_LIMIT_P"`
	}
	type pcfg struct {
		Items []*pitem `yaml:"items"`
	}
	t.Setenv("ITEM_LIMIT_P", "")
	pnil := &pcfg{Items: []*pitem{{Limit: 1}}}
	if err := ApplyEnvOverlay(pnil); err != nil {
		t.Fatalf("ApplyEnvOverlay: %v", err)
	}
	if pnil.Items[0] == nil || pnil.Items[0].Limit != 1 {
		t.Errorf("unset env disturbed pointer element: %+v", pnil.Items)
	}
	pdst := &pcfg{Items: []*pitem{{Limit: 1}}}
	t.Setenv("ITEM_LIMIT_P", "7")
	if err := ApplyEnvOverlay(pdst); err != nil {
		t.Fatalf("ApplyEnvOverlay: %v", err)
	}
	if pdst.Items[0].Limit != 7 {
		t.Errorf("pointer element env not applied: %d, want 7", pdst.Items[0].Limit)
	}

	// 覆盖表寻址（items.limit 不编号）在 overlay 路径下同样生效
	t.Setenv("ITEM_LIMIT_OVERRIDE", "9")
	ocfg := &cfg{Items: []item{{Limit: 1, Name: "x"}}}
	if err := ApplyEnvOverlay(ocfg, WithEnvNaming(EnvNaming{
		Overrides: map[string]string{"items.limit": "ITEM_LIMIT_OVERRIDE"},
	})); err != nil {
		t.Fatalf("ApplyEnvOverlay with overrides: %v", err)
	}
	if ocfg.Items[0].Limit != 9 {
		t.Errorf("override-addressed element env not applied: %d, want 9 via items.limit override", ocfg.Items[0].Limit)
	}
	if ocfg.Items[0].Name != "x" {
		t.Errorf("override element merge wiped non-env field: Name=%q", ocfg.Items[0].Name)
	}
}
