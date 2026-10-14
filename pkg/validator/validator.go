package validator

import "sync/atomic"

// Validator 是参数验证接口。
type Validator interface {
	// Validate 验证结构体（基于 struct tag）。
	Validate(obj any) error

	// ValidateVar 验证单个变量。
	ValidateVar(field any, tag string) error

	// RegisterValidation 注册自定义验证规则。
	RegisterValidation(tag string, fn Func) error

	// RegisterTranslation 注册验证错误的翻译。
	RegisterTranslation(locale, tag, msg string) error
}

// Func 是自定义验证函数。
type Func func(val any) bool

// 全局默认验证器，使用 atomic.Pointer 保护并发安全。
//
// 存储为 *Validator（指针指向接口变量），调用方读写都通过接口边界。
var defaultValidator atomic.Pointer[Validator]

func init() {
	var v Validator = New()
	defaultValidator.Store(&v)
}

// SetDefault 设置全局默认验证器。
func SetDefault(v Validator) {
	defaultValidator.Store(&v)
}

// Default 获取全局默认验证器。
func Default() Validator {
	return *defaultValidator.Load()
}

// Validate 验证结构体。
func Validate(obj any) error {
	return Default().Validate(obj)
}

// ValidateVar 验证单个变量。
func ValidateVar(field any, tag string) error {
	return Default().ValidateVar(field, tag)
}

// RegisterValidation 注册自定义验证规则。
// ⚠️ 必须在 init 阶段（任何 Struct 校验开始前）调用：底层
// go-playground/validator 的注册写内部 map 与校验读并发不安全，
// 运行期（流量中）注册会构成数据竞争。
func RegisterValidation(tag string, fn Func) error {
	return Default().RegisterValidation(tag, fn)
}

// RegisterTranslation 注册验证错误的翻译。
func RegisterTranslation(locale, tag, msg string) error {
	return Default().RegisterTranslation(locale, tag, msg)
}
