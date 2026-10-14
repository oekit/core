package encoding

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Marshal 将 v 序列化为 JSON 字节。
func Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

// MarshalToString 将 v 序列化为 JSON 字符串。
func MarshalToString(v any) (string, error) {
	data, err := Marshal(v)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Unmarshal 将 JSON 字节反序列化到 v。
// 使用 UseNumber 模式解析数字，避免精度丢失。
// 与 encoding/json.Unmarshal 语义一致：整个输入必须是单一 JSON 值，
// 尾部残留数据（截断修复失败/拼接脏数据）视为错误而非静默忽略。
func Unmarshal(data []byte, v any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := unmarshalStrict(decoder, v); err != nil {
		return formatError(string(data), err)
	}
	return nil
}

// UnmarshalFromString 将 JSON 字符串反序列化到 v。
// 语义同 Unmarshal：尾部残留数据视为错误。
func UnmarshalFromString(str string, v any) error {
	decoder := json.NewDecoder(strings.NewReader(str))
	if err := unmarshalStrict(decoder, v); err != nil {
		return formatError(str, err)
	}
	return nil
}

// UnmarshalFromReader 从 Reader 读取 JSON 并反序列化到 v。
func UnmarshalFromReader(reader io.Reader, v any) error {
	var buf strings.Builder
	teeReader := io.TeeReader(reader, &buf)
	decoder := json.NewDecoder(teeReader)
	if err := unmarshalStrict(decoder, v); err != nil {
		return formatError(buf.String(), err)
	}
	return nil
}

func unmarshalUseNumber(decoder *json.Decoder, v any) error {
	decoder.UseNumber()
	return decoder.Decode(v)
}

// unmarshalStrict 解析首个 JSON 值后继续读，尾部必须是 EOF——
// 否则 Decode 只消费第一个值，"{...} garbage" 会静默成功。
func unmarshalStrict(decoder *json.Decoder, v any) error {
	if err := unmarshalUseNumber(decoder, v); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err != nil {
			return fmt.Errorf("invalid trailing data: %w", err)
		}
		return errTrailingData
	}
	return nil
}

// errTrailingData 尾部残留数据（非单一 JSON 值）。
var errTrailingData = errors.New("invalid character after top-level value: input must be a single JSON value")

// formatError 构造带输入摘录的错误。摘录截断到 256 字节：完整原文会把
// 请求体里的 token/密码等敏感字段带进错误日志，大 payload 也会撑爆日志。
func formatError(v string, err error) error {
	const maxExcerpt = 256
	excerpt := v
	if len(excerpt) > maxExcerpt {
		excerpt = excerpt[:maxExcerpt] + "...(truncated)"
	}
	return fmt.Errorf("string: `%s`, error: `%w`", excerpt, err)
}

// MarshalIndent 将 v 序列化为带缩进的 JSON 字节。
func MarshalIndent(v any, prefix, indent string) ([]byte, error) {
	return json.MarshalIndent(v, prefix, indent)
}

// Valid 检查 data 是否是合法的 JSON。
func Valid(data []byte) bool {
	return json.Valid(data)
}
