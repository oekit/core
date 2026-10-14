package uidgen

import (
	"testing"
)

func TestUUID(t *testing.T) {
	id := UUID()
	if len(id) != 32 {
		t.Errorf("UUID() length = %v, want 32", len(id))
	}
	t.Logf("UUID: %s", id)
}

func TestShortUUID(t *testing.T) {
	id := ShortUUID()
	if len(id) != 16 {
		t.Errorf("ShortUUID() length = %v, want 16", len(id))
	}
	t.Logf("ShortUUID: %s", id)
}

func TestUUIDToUpper(t *testing.T) {
	id := UUIDToUpper()
	if len(id) != 32 {
		t.Errorf("UUIDToUpper() length = %v, want 32", len(id))
	}
	// 检查是否全大写
	for _, c := range id {
		if c >= 'a' && c <= 'f' {
			t.Errorf("UUIDToUpper() contains lowercase: %s", id)
			break
		}
	}
	t.Logf("UUIDToUpper: %s", id)
}

func TestShortUUIDToUpper(t *testing.T) {
	id := ShortUUIDToUpper()
	if len(id) != 16 {
		t.Errorf("ShortUUIDToUpper() length = %v, want 16", len(id))
	}
	t.Logf("ShortUUIDToUpper: %s", id)
}

func TestUUIDWithHyphen(t *testing.T) {
	id := UUIDWithHyphen()
	if len(id) != 36 {
		t.Errorf("UUIDWithHyphen() length = %v, want 36", len(id))
	}
	t.Logf("UUIDWithHyphen: %s", id)
}

func TestUUIDUniqueness(t *testing.T) {
	ids := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		id := UUID()
		if ids[id] {
			t.Errorf("Duplicate UUID generated: %s", id)
		}
		ids[id] = true
	}
}
