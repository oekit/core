package validator

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

// PlaygroundValidator 是基于 go-playground/validator 的验证器实现。
type PlaygroundValidator struct {
	validate *validator.Validate
}

// New 创建 PlaygroundValidator 实例。
func New() *PlaygroundValidator {
	v := validator.New()

	// 使用 json tag 作为字段名
	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return ""
		}
		if name == "" {
			return fld.Name
		}
		return name
	})

	return &PlaygroundValidator{validate: v}
}

// Validate 验证结构体。
func (pv *PlaygroundValidator) Validate(obj any) error {
	err := pv.validate.Struct(obj)
	if err == nil {
		return nil
	}

	validationErrors, ok := err.(validator.ValidationErrors)
	if !ok {
		return err
	}

	fieldErrors := make([]FieldError, 0, len(validationErrors))
	for _, fe := range validationErrors {
		fieldErrors = append(fieldErrors, FieldError{
			Field:   fe.Field(),
			Tag:     fe.Tag(),
			Param:   fe.Param(),
			Message: formatMessage(fe),
		})
	}

	return NewFieldValidationError(fieldErrors)
}

// ValidateVar 验证单个变量。
func (pv *PlaygroundValidator) ValidateVar(field any, tag string) error {
	err := pv.validate.Var(field, tag)
	if err == nil {
		return nil
	}

	validationErrors, ok := err.(validator.ValidationErrors)
	if !ok {
		return err
	}

	fieldErrors := make([]FieldError, 0, len(validationErrors))
	for _, fe := range validationErrors {
		fieldErrors = append(fieldErrors, FieldError{
			Field:   fe.Field(),
			Tag:     fe.Tag(),
			Param:   fe.Param(),
			Message: formatMessage(fe),
		})
	}

	return NewFieldValidationError(fieldErrors)
}

// RegisterValidation 注册自定义验证规则。
func (pv *PlaygroundValidator) RegisterValidation(tag string, fn Func) error {
	return pv.validate.RegisterValidation(tag, func(fl validator.FieldLevel) bool {
		return fn(fl.Field().Interface())
	})
}

// RegisterTranslation 注册验证错误的翻译。
//
// 当前实现不支持翻译引擎：错误消息已通过 formatMessage English prompts via formatMessage。
// 调用将始终返回 ErrTranslationNotSupported，避免调用方误以为翻译已注册成功。
// 如需自定义消息，请扩展 formatMessage 或实现自定义 Validator。
func (pv *PlaygroundValidator) RegisterTranslation(locale, tag, msg string) error {
	return ErrTranslationNotSupported
}

// Validate 底层验证器实例，供高级用户直接使用。
func (pv *PlaygroundValidator) Validate_() *validator.Validate {
	return pv.validate
}

// formatMessage 生成友好的错误消息。
func formatMessage(fe validator.FieldError) string {
	field := fe.Field()
	tag := fe.Tag()

	switch tag {
	case "required":
		return fmt.Sprintf("%s is required", field)
	case "email":
		return fmt.Sprintf("%s must be a valid email address", field)
	case "min":
		return fmt.Sprintf("%s length must be at least %s", field, fe.Param())
	case "max":
		return fmt.Sprintf("%s length must be at most %s", field, fe.Param())
	case "gte":
		return fmt.Sprintf("%s must be greater than or equal to %s", field, fe.Param())
	case "lte":
		return fmt.Sprintf("%s must be less than or equal to %s", field, fe.Param())
	case "gt":
		return fmt.Sprintf("%s must be greater than %s", field, fe.Param())
	case "lt":
		return fmt.Sprintf("%s must be less than %s", field, fe.Param())
	case "oneof":
		return fmt.Sprintf("%s must be one of: %s", field, fe.Param())
	case "len":
		return fmt.Sprintf("%s length must be %s", field, fe.Param())
	case "url":
		return fmt.Sprintf("%s must be a valid URL", field)
	case "uri":
		return fmt.Sprintf("%s must be a valid URI", field)
	case "uuid":
		return fmt.Sprintf("%s must be a valid UUID", field)
	case "numeric":
		return fmt.Sprintf("%s must be numeric", field)
	case "alpha":
		return fmt.Sprintf("%s must contain only letters", field)
	case "alphanum":
		return fmt.Sprintf("%s must contain only letters and numbers", field)
	default:
		return fmt.Sprintf("%s validation failed (%s)", field, tag)
	}
}
