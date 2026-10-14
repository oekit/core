package convert

import (
	"testing"
)

func TestDeepCopy(t *testing.T) {
	t.Run("map", func(t *testing.T) {
		original := map[string]any{
			"key1": "value1",
			"key2": 123,
			"key3": []any{1, 2, 3},
		}
		copied := DeepCopy(original).(map[string]any)

		// 修改原值不应影响拷贝
		original["key1"] = "modified"
		if copied["key1"] != "value1" {
			t.Error("DeepCopy did not create independent copy")
		}
	})

	t.Run("slice", func(t *testing.T) {
		original := []any{1, 2, 3}
		copied := DeepCopy(original).([]any)

		// 修改原值不应影响拷贝
		original[0] = 99
		if copied[0] != 1 {
			t.Error("DeepCopy did not create independent copy")
		}
	})

	t.Run("primitive", func(t *testing.T) {
		original := "hello"
		copied := DeepCopy(original)
		if copied != "hello" {
			t.Errorf("DeepCopy() = %v, want hello", copied)
		}
	})
}

func TestAppendStrings(t *testing.T) {
	a := []string{"a", "b"}
	b := []string{"c", "d"}
	result := AppendStrings(a, b)
	if len(result) != 4 {
		t.Errorf("AppendStrings() length = %v, want 4", len(result))
	}
}

func TestConvert(t *testing.T) {
	type Source struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	type Target struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	source := Source{Name: "test", Age: 18}
	var target Target
	err := Convert(source, &target)
	if err != nil {
		t.Fatal(err)
	}
	if target.Name != "test" || target.Age != 18 {
		t.Errorf("Convert() = %+v, want {Name:test Age:18}", target)
	}
}
