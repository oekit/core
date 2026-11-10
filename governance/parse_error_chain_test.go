package governance

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// TestParse_KeepsJSONErrorChain 回归 core#118：Parse 此前用 `%v` 把
// json.Unmarshal 的错误链拍平成纯文本——*json.SyntaxError /
// *json.UnmarshalTypeError 不可 errors.As，UnmarshalTypeError 自带的
// Struct/Field 结构化定位全部丢失，运维工具只能正则扒非契约文案分流
// （"of type map[string]int" 随版本/字段变动即断；kitx#70 同构换仓复发）。
// 修后双 `%w`：ErrInvalidConfig 哨兵与 json 错误同时在链上，errors.Is /
// errors.As 双双可判定。
func TestParse_KeepsJSONErrorChain(t *testing.T) {
	// 语法损坏：SyntaxError 必须可 As——分流到「JSON 语法坏了」这一类
	_, err := Parse([]byte(`{"retry":`))
	var synErr *json.SyntaxError
	if !errors.As(err, &synErr) {
		t.Fatalf("Parse(malformed json): errors.As(*json.SyntaxError) = false, err = %v", err)
	}
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("ErrInvalidConfig 哨兵不得因双 %%w 丢失, err = %v", err)
	}

	// 类型不符：UnmarshalTypeError 必须可 As——Value/Struct/Field 结构化定位
	_, err = Parse([]byte(`{"timeout_ms":{"svc":"not-a-number"}}`))
	var typeErr *json.UnmarshalTypeError
	if !errors.As(err, &typeErr) {
		t.Fatalf("Parse(type mismatch): errors.As(*json.UnmarshalTypeError) = false, err = %v", err)
	}
	if typeErr.Value != "string" {
		t.Errorf("UnmarshalTypeError.Value = %q, want \"string\"", typeErr.Value)
	}
	if !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("type mismatch 路径同样要保住 ErrInvalidConfig 哨兵, err = %v", err)
	}

	// 文案形态不回退：既有消费方按「invalid config: <原始错误>」前缀匹配的
	// 面不能破坏（双 %w 与 %v 的文案逐字一致，只差错误链深度）
	if !strings.HasPrefix(err.Error(), "governance: invalid config: ") {
		t.Errorf("错误文案前缀变了, got %q", err.Error())
	}
}
