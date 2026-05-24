// Package common - validate.go
// 该文件初始化全局数据验证器
//
// 使用 go-playground/validator 库进行结构体字段验证
// 提供统一的验证器实例，供整个项目使用
//
// 使用场景：
// - API 请求参数验证（通过 Gin 的 ShouldBind + validate 标签）
// - 手动验证结构体字段
// - 自定义验证规则注册
//
// 验证标签示例：
//
//	type Request struct {
//	    Email string `json:"email" validate:"required,email"`
//	    Age   int    `json:"age" validate:"gte=0,lte=130"`
//	}
package common

import "github.com/go-playground/validator/v10"

// Validate 全局验证器实例
//
// 在 init 函数中初始化，供整个项目使用
// 支持所有内置验证标签（required, email, min, max 等）
// 也支持注册自定义验证规则
var Validate *validator.Validate

// init 初始化全局验证器
func init() {
	Validate = validator.New()
}
