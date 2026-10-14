package slicex

import (
	"reflect"
	"testing"
)

func TestContains(t *testing.T) {
	if !Contains([]string{"a", "b", "c"}, "b") {
		t.Error("expected true")
	}
	if Contains([]string{"a", "b"}, "d") {
		t.Error("expected false")
	}
	if !Contains([]int{1, 2, 3}, 2) {
		t.Error("expected true")
	}
}

func TestIndex(t *testing.T) {
	if Index([]string{"a", "b", "c"}, "b") != 1 {
		t.Error("expected 1")
	}
	if Index([]string{"a", "b"}, "d") != -1 {
		t.Error("expected -1")
	}
}

func TestUnique(t *testing.T) {
	got := Unique([]int{1, 1, 2, 3, 2})
	expected := []int{1, 2, 3}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("expected %v, got %v", expected, got)
	}
}

func TestFilter(t *testing.T) {
	got := Filter([]int{1, 2, 3, 4, 5}, func(v int) bool { return v > 3 })
	expected := []int{4, 5}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("expected %v, got %v", expected, got)
	}
}

func TestMap(t *testing.T) {
	got := Map([]int{1, 2, 3}, func(v int) int { return v * 2 })
	expected := []int{2, 4, 6}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("expected %v, got %v", expected, got)
	}
}

func TestReduce(t *testing.T) {
	got := Reduce([]int{1, 2, 3, 4}, 0, func(acc, v int) int { return acc + v })
	if got != 10 {
		t.Errorf("expected 10, got %d", got)
	}
}

func TestJoin(t *testing.T) {
	if Join([]int64{1, 2, 3}, ",") != "1,2,3" {
		t.Error("expected '1,2,3'")
	}
	if Join([]string{"a", "b"}, "-") != "a-b" {
		t.Error("expected 'a-b'")
	}
}

func TestGroupBy(t *testing.T) {
	data := []int{1, 2, 3, 4, 5, 6}
	grouped := GroupBy(data, func(v int) string {
		if v%2 == 0 {
			return "even"
		}
		return "odd"
	})
	if len(grouped["odd"]) != 3 || len(grouped["even"]) != 3 {
		t.Errorf("expected 3 odd and 3 even, got %v", grouped)
	}
}

func TestToMap(t *testing.T) {
	type Item struct {
		ID   int
		Name string
	}
	items := []Item{{1, "a"}, {2, "b"}, {3, "c"}}
	m := ToMap(items, func(i Item) int { return i.ID })
	if m[2].Name != "b" {
		t.Error("expected b")
	}
}

func TestChunk(t *testing.T) {
	chunks := Chunk([]int{1, 2, 3, 4, 5}, 2)
	if len(chunks) != 3 {
		t.Errorf("expected 3 chunks, got %d", len(chunks))
	}
	if !reflect.DeepEqual(chunks[0], []int{1, 2}) {
		t.Errorf("expected [1 2], got %v", chunks[0])
	}
	if !reflect.DeepEqual(chunks[2], []int{5}) {
		t.Errorf("expected [5], got %v", chunks[2])
	}
}

func TestReverse(t *testing.T) {
	got := Reverse([]int{1, 2, 3})
	expected := []int{3, 2, 1}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("expected %v, got %v", expected, got)
	}
}

func TestFlatten(t *testing.T) {
	got := Flatten([][]int{{1, 2}, {3}, {4, 5}})
	expected := []int{1, 2, 3, 4, 5}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("expected %v, got %v", expected, got)
	}
}
