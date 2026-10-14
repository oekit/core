// Package validator 提供统一的参数验证能力。
//
// 基于 go-playground/validator，封装为面向接口的验证门面。
// 支持 struct tag 验证、自定义规则和结构体验证错误格式化。
// HTTP 框架相关的绑定逻辑位于 handler/validatorh。
//
// 用法:
//
//	type CreateUserReq struct {
//	    Name  string `json:"name" validate:"required,min=1,max=100"`
//	    Email string `json:"email" validate:"required,email"`
//	    Age   int    `json:"age" validate:"gte=0,lte=150"`
//	}
//
//	// 直接验证结构体
//	err := validator.Validate(&req)
package validator
