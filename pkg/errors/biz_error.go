package errors

import (
	stderrors "errors"
	"fmt"
)

// BizError 是业务层错误类型，携带业务码（BizCode）和 HTTP 状态码（HTTPCode）。
// 在 service 层返回此错误后，handler 层可通过 errors.As 提取 code 和 message。
//
// 字段语义（v0.17.0 起分离）：
//   - BizCode：业务语义标识，写入响应 body 的 code 字段（如 1001、40404）
//   - HTTPCode：HTTP 状态码；为 0 时由 middleware.StatusMapper 从 BizCode 推导
//
// 用法:
//
//	return nil, errors.NewNotFoundError("项目不存在")
//	return nil, errors.NewBadRequestError("keyword 不能为空")
//	return nil, errors.NewBizError(1001, "余额不足")              // HTTPCode=0, 走 mapper
//	return nil, errors.NewBizErrorWithStatus(1001, 402, "余额不足") // 显式两码
type BizError struct {
	BizCode  int    `json:"biz_code"`  // 业务码（语义标识）
	HTTPCode int    `json:"http_code"` // HTTP 状态码（0 = 由 StatusMapper 推导）
	Message  string `json:"message"`   // 业务描述
	Cause    error  `json:"-"`         // 原始错误（可选）
}

// Error 实现 error 接口。
// 包含 BizCode 以确保日志中不丢失业务码上下文。
func (e *BizError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("biz_code:%d %s: %s", e.BizCode, e.Message, e.Cause.Error())
	}
	return fmt.Sprintf("biz_code:%d %s", e.BizCode, e.Message)
}

// Unwrap 实现 errors.Unwrap 接口，支持 errors.Is / errors.As 链式匹配。
func (e *BizError) Unwrap() error {
	return e.Cause
}

// String 实现 fmt.Stringer 接口。
func (e *BizError) String() string {
	return fmt.Sprintf("biz_code:%d http_code:%d msg:%s", e.BizCode, e.HTTPCode, e.Message)
}

// NewBizError 创建一个自定义业务码错误，HTTPCode 留空（0），
// handler 层会经 StatusMapper 从 BizCode 推导 HTTP 状态码。
//
// 适用于业务码与 HTTP 对齐的场景（如 BizCode=404 → HTTP 404），
// 或接受默认映射（BizCode=1001 → HTTP 400）。
// 需要显式分离两码时使用 NewBizErrorWithStatus。
func NewBizError(code int, msg string) *BizError {
	return &BizError{BizCode: code, Message: msg}
}

// NewBizErrorWithStatus 创建一个显式指定 HTTP 状态码的业务错误。
// 适用于业务码与 HTTP 状态码需要分离的场景（如 BizCode=1001 但需返回 HTTP 402）。
func NewBizErrorWithStatus(bizCode, httpCode int, msg string) *BizError {
	return &BizError{BizCode: bizCode, HTTPCode: httpCode, Message: msg}
}

// NewNotFoundError 创建 404 业务错误（资源不存在）。
func NewNotFoundError(msg string) *BizError {
	return &BizError{BizCode: 404, HTTPCode: 404, Message: msg}
}

// NewBadRequestError 创建 400 业务错误（请求参数错误）。
func NewBadRequestError(msg string) *BizError {
	return &BizError{BizCode: 400, HTTPCode: 400, Message: msg}
}

// NewUnauthorizedError 创建 401 业务错误（未认证）。
func NewUnauthorizedError(msg string) *BizError {
	return &BizError{BizCode: 401, HTTPCode: 401, Message: msg}
}

// NewForbiddenError 创建 403 业务错误（无权限）。
func NewForbiddenError(msg string) *BizError {
	return &BizError{BizCode: 403, HTTPCode: 403, Message: msg}
}

// NewConflictError 创建 409 业务错误（资源冲突）。
func NewConflictError(msg string) *BizError {
	return &BizError{BizCode: 409, HTTPCode: 409, Message: msg}
}

// WrapBizError 包装原始错误为业务错误，HTTPCode 留空（0），
// handler 层经 StatusMapper 从 BizCode 推导。
func WrapBizError(code int, msg string, cause error) *BizError {
	return &BizError{BizCode: code, Message: msg, Cause: cause}
}

// WrapBizErrorWithStatus 包装原始错误为业务错误，显式指定 HTTP 状态码。
func WrapBizErrorWithStatus(bizCode, httpCode int, msg string, cause error) *BizError {
	return &BizError{BizCode: bizCode, HTTPCode: httpCode, Message: msg, Cause: cause}
}

// IsBizError 检查错误是否为 BizError 类型。
func IsBizError(err error) bool {
	var bizErr *BizError
	return stderrors.As(err, &bizErr)
}

// AsBizError 从错误链中提取 BizError。
func AsBizError(err error) (*BizError, bool) {
	var bizErr *BizError
	if stderrors.As(err, &bizErr) {
		return bizErr, true
	}
	return nil, false
}
