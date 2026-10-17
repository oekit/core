package transport_test

import (
	"slices"
	"testing"

	"github.com/oekit/core/transport"
)

func TestMapHeaderSemantics(t *testing.T) {
	h := transport.MapHeader{}
	h.Add("X-Multi", "a")
	h.Add("X-Multi", "b")
	if vs := h.Values("X-Multi"); len(vs) != 2 || vs[0] != "a" || vs[1] != "b" {
		t.Fatalf("Values = %v, want [a b]", vs)
	}
	if h.Get("X-Multi") != "a" {
		t.Fatalf("Get = %q, want first value a", h.Get("X-Multi"))
	}
	h.Set("X-Multi", "c")
	if vs := h.Values("X-Multi"); len(vs) != 1 || vs[0] != "c" {
		t.Fatalf("Set should overwrite, got %v", vs)
	}
	h.Del("X-Multi")
	if h.Get("X-Multi") != "" {
		t.Fatal("Del should remove key")
	}
	if len(h.Keys()) != 0 {
		t.Fatalf("Keys = %v, want empty", h.Keys())
	}
}

func TestMapHeaderKeysSorted(t *testing.T) {
	h := transport.MapHeader{}
	h.Set("B", "1")
	h.Set("A", "2")
	h.Set("C", "3")
	keys := h.Keys()
	if !slices.Equal(keys, []string{"A", "B", "C"}) {
		t.Fatalf("Keys = %v, want sorted [A B C]", keys)
	}
}

func TestNoopHeader(t *testing.T) {
	var h transport.NoopHeader
	h.Set("X", "1")
	h.Add("X", "2")
	h.Del("X")
	if h.Get("X") != "" || len(h.Values("X")) != 0 || len(h.Keys()) != 0 {
		t.Fatal("NoopHeader must be a zero-value sink")
	}
}
