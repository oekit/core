package validator

import (
	"errors"
	"fmt"
	"strings"

	bizerrors "github.com/yi-nology/ekit-core/pkg/errors"
)

// ErrTranslationNotSupported 表示当前验证器不支持错误消息翻译。
// PlaygroundValidator 的错误消息已通过 formatMessage 内置英文提示，
// 如需自定义消息请扩展 formatMessage 或实现自定义 Validator。
var ErrTranslationNotSupported = errors.New("validator: translation not supported; messages are built-in via formatMessage")

// ValidationError 包含多个字段验证错误。
// 实现 BizError 接口（code=400），handler 层的 Fail 会自动识别为 400 Bad Request。
type ValidationError struct {
	errors []FieldError
}

// FieldError 表示单个字段的验证错误。
type FieldError struct {
	Field   string `json:"field"`           // 字段名
	Tag     string `json:"tag"`             // 验证标签
	Param   string `json:"param,omitempty"` // 标签参数
	Message string `json:"message"`         // 错误消息
}

// Error 实现 error 接口，返回第一个错误消息。
func (e *ValidationError) Error() string {
	if len(e.errors) == 0 {
		return "validation failed"
	}
	return e.errors[0].Message
}

// Unwrap 返回底层的 BizError（BizCode=400, HTTPCode=400），使 errors.As 能识别为 BizError。
// 这样 handler 层的 Fail 会自动返回 400 Bad Request，而非 500。
func (e *ValidationError) Unwrap() error {
	return bizerrors.NewBizErrorWithStatus(400, 400, e.Error())
}

// Errors 返回所有字段错误。
func (e *ValidationError) Errors() []FieldError {
	return e.errors
}

// Map 返回 field→message 映射，适用于 JSON 响应。
func (e *ValidationError) Map() map[string]string {
	m := make(map[string]string, len(e.errors))
	for _, fe := range e.errors {
		m[fe.Field] = fe.Message
	}
	return m
}

// Len 返回错误数量。
func (e *ValidationError) Len() int {
	return len(e.errors)
}

// String 实现 fmt.Stringer。
func (e *ValidationError) String() string {
	var sb strings.Builder
	sb.WriteString("validation failed: ")
	for i, fe := range e.errors {
		if i > 0 {
			sb.WriteString("; ")
		}
		fmt.Fprintf(&sb, "%s (%s)", fe.Field, fe.Tag)
	}
	return sb.String()
}

// NewFieldValidationError 创建包含指定字段错误的验证错误。
// 供 handler/validatorh 等外部包构造 ValidationError。
func NewFieldValidationError(errors []FieldError) *ValidationError {
	return &ValidationError{errors: errors}
}

// IsValidationError 检查错误是否为 ValidationError 类型。
func IsValidationError(err error) bool {
	_, ok := AsValidationError(err)
	return ok
}

// AsValidationError 从错误链中提取 ValidationError（支持 errors.As 链式匹配）。
func AsValidationError(err error) (*ValidationError, bool) {
	if err == nil {
		return nil, false
	}
	var ve *ValidationError
	if errors.As(err, &ve) {
		return ve, true
	}
	return nil, false
}
