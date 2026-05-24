// Package middleware - i18n.go
// 该文件实现了国际化（i18n）语言检测中间件
//
// 语言检测优先级（从高到低）：
// 1. 用户个人设置中的语言偏好（仅已登录用户）
// 2. HTTP 请求头 Accept-Language
// 3. 系统默认语言
//
// 检测到的语言会存储到 Gin 上下文中，供后续处理器和模板使用
// 可通过 GetLanguage 函数获取当前请求的语言
package middleware

import (
	"github.com/gin-gonic/gin"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/i18n"
)

// I18n 国际化语言检测中间件工厂函数
// 创建并返回一个 Gin 中间件，用于检测并设置请求的语言偏好
// 检测到的语言会存储到 Gin 上下文的 constant.ContextKeyLanguage 键中
func I18n() gin.HandlerFunc {
	return func(c *gin.Context) {
		lang := detectLanguage(c)
		c.Set(string(constant.ContextKeyLanguage), lang)
		c.Next()
	}
}

// detectLanguage 检测当前请求的语言偏好
// 按照优先级依次尝试以下来源：
// 1. 用户个人设置（通过 Auth 中间件注入到上下文）
// 2. HTTP Accept-Language 请求头
// 3. 系统默认语言（i18n.DefaultLang）
func detectLanguage(c *gin.Context) string {
	// 1. Try to get language from user setting (set by auth middleware)
	if userSetting, ok := common.GetContextKeyType[dto.UserSetting](c, constant.ContextKeyUserSetting); ok {
		if userSetting.Language != "" && i18n.IsSupported(userSetting.Language) {
			return userSetting.Language
		}
	}

	// 2. Parse Accept-Language header
	acceptLang := c.GetHeader("Accept-Language")
	if acceptLang != "" {
		lang := i18n.ParseAcceptLanguage(acceptLang)
		if i18n.IsSupported(lang) {
			return lang
		}
	}

	// 3. Return default language
	return i18n.DefaultLang
}

// GetLanguage 从 Gin 上下文中获取当前请求的语言
// 如果上下文中未设置语言，返回系统默认语言
// 可在控制器和服务层中调用此函数获取当前语言
func GetLanguage(c *gin.Context) string {
	if lang := c.GetString(string(constant.ContextKeyLanguage)); lang != "" {
		return lang
	}
	return i18n.DefaultLang
}
