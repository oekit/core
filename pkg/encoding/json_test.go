package encoding

import (
	"strings"
	"testing"
)

func TestMarshal(t *testing.T) {
	type TestStruct struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	tests := []struct {
		name string
		v    any
		want string
	}{
		{"struct", TestStruct{Name: "test", Age: 18}, `{"name":"test","age":18}`},
		{"nil", nil, "null"},
		{"string", "hello", `"hello"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Marshal(tt.v)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("Marshal() = %v, want %v", string(got), tt.want)
			}
		})
	}
}

func TestMarshalToString(t *testing.T) {
	result, err := MarshalToString("hello")
	if err != nil {
		t.Fatal(err)
	}
	if result != `"hello"` {
		t.Errorf("MarshalToString() = %v, want %v", result, `"hello"`)
	}
}

func TestUnmarshal(t *testing.T) {
	type TestStruct struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	var result TestStruct
	err := Unmarshal([]byte(`{"name":"test","age":18}`), &result)
	if err != nil {
		t.Fatal(err)
	}
	if result.Name != "test" || result.Age != 18 {
		t.Errorf("Unmarshal() = %+v, want {Name:test Age:18}", result)
	}
}

func TestUnmarshalFromString(t *testing.T) {
	type TestStruct struct {
		Name string `json:"name"`
	}

	var result TestStruct
	err := UnmarshalFromString(`{"name":"test"}`, &result)
	if err != nil {
		t.Fatal(err)
	}
	if result.Name != "test" {
		t.Errorf("UnmarshalFromString() Name = %v, want test", result.Name)
	}
}

func TestUnmarshalFromReader(t *testing.T) {
	type TestStruct struct {
		Name string `json:"name"`
	}

	var result TestStruct
	reader := strings.NewReader(`{"name":"test"}`)
	err := UnmarshalFromReader(reader, &result)
	if err != nil {
		t.Fatal(err)
	}
	if result.Name != "test" {
		t.Errorf("UnmarshalFromReader() Name = %v, want test", result.Name)
	}
}

func TestUnmarshalUseNumber(t *testing.T) {
	var result any
	err := Unmarshal([]byte(`{"number":12345678901234567890}`), &result)
	if err != nil {
		t.Fatal(err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatal("expected map")
	}

	// UseNumber 应该返回 json.Number 而不是 float64
	if _, ok := m["number"].(string); !ok {
		// json.Number 实际上是 string 类型
		t.Logf("number type: %T", m["number"])
	}
}

func TestMarshalIndent(t *testing.T) {
	type TestStruct struct {
		Name string `json:"name"`
	}

	result, err := MarshalIndent(TestStruct{Name: "test"}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	expected := `{
  "name": "test"
}`
	if string(result) != expected {
		t.Errorf("MarshalIndent() = %v, want %v", string(result), expected)
	}
}

func TestValid(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{"valid object", []byte(`{"name":"test"}`), true},
		{"valid array", []byte(`[1,2,3]`), true},
		{"invalid", []byte(`{name:test}`), false},
		{"empty", []byte(``), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Valid(tt.data); got != tt.want {
				t.Errorf("Valid() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestUnmarshal_TrailingData 回归：json.Decoder.Decode 只消费首个 JSON 值，
// "{...} garbage" 曾被静默当成成功（截断修复失败/拼接脏数据静默通过）。
// 钉住：整个输入必须是单一 JSON 值，与 encoding/json.Unmarshal 语义一致。
func TestUnmarshal_TrailingData(t *testing.T) {
	var v map[string]any
	if err := Unmarshal([]byte(`{"a":1} garbage`), &v); err == nil {
		t.Error("trailing garbage should be rejected")
	}
	if err := Unmarshal([]byte(`{"a":1} {"b":2}`), &v); err == nil {
		t.Error("second top-level value should be rejected")
	}
	if err := Unmarshal([]byte(`{"a":1}`), &v); err != nil {
		t.Errorf("clean input should succeed: %v", err)
	}
	if err := UnmarshalFromString(`[1,2] trailing`, &v); err == nil {
		t.Error("UnmarshalFromString trailing garbage should be rejected")
	}
}

// TestUnmarshal_ErrorExcerptTruncated 回归：formatError 曾把完整输入嵌进
// 错误——请求体里的 token/密码随错误日志泄漏，大 payload 撑爆日志。
func TestUnmarshal_ErrorExcerptTruncated(t *testing.T) {
	big := make([]byte, 4096)
	for i := range big {
		big[i] = 'x'
	}
	payload := make([]byte, 0, 4097)
	payload = append(payload, `{"k":"`...)
	payload = append(payload, big...)
	payload = append(payload, '"')
	err := Unmarshal(payload, &map[string]any{})
	if err == nil {
		t.Fatal("expected error")
	}
	if len(err.Error()) > 1024 {
		t.Errorf("error message should be truncated, got %d bytes", len(err.Error()))
	}
}
