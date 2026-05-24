// Package middleware - turnstile-check.go
// 该文件实现了 Cloudflare Turnstile 验证中间件
// Turnstile 是 Cloudflare 提供的免费验证码服务，用于防止机器人攻击
//
// 工作流程：
// 1. 前端集成 Turnstile 组件，用户完成验证后获得 token
// 2. 前端将 token 作为查询参数传递给后端
// 3. 后端将 token 发送到 Cloudflare 服务器验证
// 4. 验证通过后，将结果保存到会话中，避免重复验证
//
// 适用场景：
// - 用户注册
// - 用户登录
// - 密码重置
// - 敏感操作确认
package middleware

import (
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/c1cada/NexusTok/common"    // 公共工具包
	"github.com/gin-contrib/sessions" // 会话管理
	"github.com/gin-gonic/gin"        // Gin 框架
)

// turnstileCheckResponse Turnstile 验证响应结构体
type turnstileCheckResponse struct {
	Success bool `json:"success"` // 验证是否成功
}

// TurnstileCheck Turnstile 验证中间件
// 检查请求是否通过了 Turnstile 验证
//
// 验证流程：
// 1. 检查是否启用 Turnstile 验证
// 2. 检查会话中是否已有验证记录
// 3. 从查询参数获取 Turnstile token
// 4. 将 token 发送到 Cloudflare 服务器验证
// 5. 验证通过后，将结果保存到会话中
//
// 返回值：
//   - gin.HandlerFunc: Gin 中间件函数
func TurnstileCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 检查是否启用 Turnstile 验证
		if common.TurnstileCheckEnabled {
			// 获取会话
			session := sessions.Default(c)

			// 检查会话中是否已有验证记录
			turnstileChecked := session.Get("turnstile")
			if turnstileChecked != nil {
				// 已验证，继续处理请求
				c.Next()
				return
			}

			// 从查询参数获取 Turnstile token
			response := c.Query("turnstile")
			if response == "" {
				// token 为空，返回错误
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": "Turnstile token 为空",
				})
				c.Abort()
				return
			}

			// 向 Cloudflare 服务器发送验证请求
			rawRes, err := http.PostForm("https://challenges.cloudflare.com/turnstile/v0/siteverify", url.Values{
				"secret":   {common.TurnstileSecretKey}, // 服务器端密钥
				"response": {response},                   // 前端传来的 token
				"remoteip": {c.ClientIP()},               // 客户端 IP 地址
			})
			if err != nil {
				// 请求失败，返回错误
				common.SysLog(err.Error())
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": err.Error(),
				})
				c.Abort()
				return
			}
			defer rawRes.Body.Close()

			// 解析验证响应
			var res turnstileCheckResponse
			err = json.NewDecoder(rawRes.Body).Decode(&res)
			if err != nil {
				// 解析失败，返回错误
				common.SysLog(err.Error())
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": err.Error(),
				})
				c.Abort()
				return
			}

			// 检查验证结果
			if !res.Success {
				// 验证失败，返回错误
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": "Turnstile 校验失败，请刷新重试！",
				})
				c.Abort()
				return
			}

			// 验证通过，将结果保存到会话中
			session.Set("turnstile", true)
			err = session.Save()
			if err != nil {
				// 保存会话失败，返回错误
				c.JSON(http.StatusOK, gin.H{
					"message": "无法保存会话信息，请重试",
					"success": false,
				})
				return
			}
		}

		// 继续处理请求
		c.Next()
	}
}
