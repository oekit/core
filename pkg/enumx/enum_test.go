package enumx

import (
	"testing"
)

type Status int

const (
	StatusActive   Status = 1
	StatusInactive Status = 2
	StatusDeleted  Status = 3
)

var statusLabels = map[Status]string{
	StatusActive:   "启用",
	StatusInactive: "禁用",
	StatusDeleted:  "已删除",
}

func (s Status) Value() int    { return int(s) }
func (s Status) Label() string { return statusLabels[s] }
func (s Status) IsValid() bool { _, ok := statusLabels[s]; return ok }

var StatusRegistry = NewRegistry(StatusActive, StatusInactive, StatusDeleted)

func TestRegistry_Get(t *testing.T) {
	s, ok := StatusRegistry.Get(1)
	if !ok {
		t.Fatal("expected to find status 1")
	}
	if s != StatusActive {
		t.Errorf("expected StatusActive, got %v", s)
	}

	_, ok = StatusRegistry.Get(999)
	if ok {
		t.Error("should not find status 999")
	}
}

func TestRegistry_MustGet(t *testing.T) {
	s := StatusRegistry.MustGet(1)
	if s != StatusActive {
		t.Errorf("expected StatusActive, got %v", s)
	}

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for invalid value")
		}
	}()
	StatusRegistry.MustGet(999)
}

func TestRegistry_Label(t *testing.T) {
	label := StatusRegistry.Label(1)
	if label != "启用" {
		t.Errorf("expected '启用', got '%s'", label)
	}

	label = StatusRegistry.Label(999)
	if label != "unknown(999)" {
		t.Errorf("expected 'unknown(999)', got '%s'", label)
	}
}

func TestRegistry_All(t *testing.T) {
	all := StatusRegistry.All()
	if len(all) != 3 {
		t.Errorf("expected 3 enums, got %d", len(all))
	}
}

func TestRegistry_Values(t *testing.T) {
	values := StatusRegistry.Values()
	if len(values) != 3 {
		t.Errorf("expected 3 values, got %d", len(values))
	}
}

func TestRegistry_IsValid(t *testing.T) {
	if !StatusRegistry.IsValid(1) {
		t.Error("expected 1 to be valid")
	}
	if StatusRegistry.IsValid(999) {
		t.Error("expected 999 to be invalid")
	}
}

func TestRegistry_Len(t *testing.T) {
	if l := StatusRegistry.Len(); l != 3 {
		t.Errorf("expected 3, got %d", l)
	}
}

func TestRegistry_Map(t *testing.T) {
	m := StatusRegistry.Map()
	if m[1] != "启用" {
		t.Errorf("expected '启用', got '%s'", m[1])
	}
}
