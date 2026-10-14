package slicex

import (
	"testing"
)

func TestPartition_AllMatch(t *testing.T) {
	matching, rest := Partition([]int{1, 2, 3}, func(x int) bool { return x > 0 })
	if len(matching) != 3 || len(rest) != 0 {
		t.Errorf("got (%v, %v), want (3, 0)", matching, rest)
	}
}

func TestPartition_NoneMatch(t *testing.T) {
	matching, rest := Partition([]int{1, 2, 3}, func(x int) bool { return x > 100 })
	if len(matching) != 0 || len(rest) != 3 {
		t.Errorf("got (%v, %v), want (0, 3)", matching, rest)
	}
}

func TestPartition_Mixed(t *testing.T) {
	matching, rest := Partition([]int{1, 2, 3, 4, 5}, func(x int) bool { return x%2 == 0 })
	if len(matching) != 2 || len(rest) != 3 {
		t.Errorf("got (%v, %v), want (2, 3)", matching, rest)
	}
	if matching[0] != 2 || matching[1] != 4 {
		t.Errorf("matching = %v, want [2 4]", matching)
	}
	if rest[0] != 1 || rest[1] != 3 || rest[2] != 5 {
		t.Errorf("rest = %v, want [1 3 5]", rest)
	}
}

func TestPartition_Empty(t *testing.T) {
	matching, rest := Partition([]int{}, func(x int) bool { return true })
	if len(matching) != 0 || len(rest) != 0 {
		t.Errorf("got (%v, %v), want (0, 0)", matching, rest)
	}
}

func TestPartition_PreservesOrder(t *testing.T) {
	in := []string{"a", "b", "c", "d", "e"}
	matching, rest := Partition(in, func(s string) bool { return s == "a" || s == "c" || s == "e" })
	if matching[0] != "a" || matching[1] != "c" || matching[2] != "e" {
		t.Errorf("matching order broken: %v", matching)
	}
	if rest[0] != "b" || rest[1] != "d" {
		t.Errorf("rest order broken: %v", rest)
	}
}

type partitionResult struct {
	Name  string
	Error error
}

func TestPartition_ResultType(t *testing.T) {
	results := []partitionResult{
		{Name: "a", Error: nil},
		{Name: "b", Error: errFake},
		{Name: "c", Error: nil},
		{Name: "d", Error: errFake},
	}
	success, failure := Partition(results, func(r partitionResult) bool {
		return r.Error == nil
	})
	if len(success) != 2 || success[0].Name != "a" || success[1].Name != "c" {
		t.Errorf("success = %v", success)
	}
	if len(failure) != 2 || failure[0].Name != "b" || failure[1].Name != "d" {
		t.Errorf("failure = %v", failure)
	}
}

var errFake = &fakeErr{msg: "fake"}

type fakeErr struct{ msg string }

func (e *fakeErr) Error() string { return e.msg }
