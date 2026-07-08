// Package middleware - auth.go
// 该文件实现了认证相关的中间件
// 包括用户认证、管理员认证、Root 认证、Token 认证等
//
// 认证方式：
// 1. Session 认证 - 基于 Cookie 的会话认证（用于 Web 前端）
// 2. Access Token 认证 - 基于 Header 的 Token 认证（用于 API 调用）
// 3. API Token 认证 - 基于 Bearer Token 的认证（用于中继 API）
package middleware

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/c1cada/NexusTok/common"                // 公共工具包
	"github.com/c1cada/NexusTok/constant"              // 常量定义
	"github.com/c1cada/NexusTok/i18n"                  // 国际化
	"github.com/c1cada/NexusTok/logger"                // 日志
	"github.com/c1cada/NexusTok/model"                 // 数据模型
	"github.com/c1cada/NexusTok/service"               // 服务层
	"github.com/c1cada/NexusTok/service/authz"         // 管理权限判定
	"github.com/c1cada/NexusTok/setting/ratio_setting" // 比率设置
	"github.com/c1cada/NexusTok/types"                 // 类型定义

	"github.com/gin-contrib/sessions" // 会话管理
	"github.com/gin-gonic/gin"        // Gin 框架
	"gorm.io/gorm"                    // GORM ORM
)

// legacyUserHeaderName 旧版用户标识头名称
// 为了向后兼容，保留旧的头名称
const legacyUserHeaderName = "New-" + "Api-User"

// validUserInfo 验证用户信息是否有效
// 检查用户名是否为空，角色是否有效
//
// 参数：
//   - username: 用户名
//   - role: 用户角色
//
// 返回值：
//   - bool: 用户信息是否有效
func validUserInfo(username string, role int) bool {
	// 检查用户名是否为空
	if strings.TrimSpace(username) == "" {
		return false
	}

	// 检查角色是否有效
	if !common.IsValidateRole(role) {
		return false
	}

	return true
}

// authHelper 认证辅助函数
// 处理用户认证逻辑，支持 Session 和 Access Token 两种方式
//
// 认证流程：
// 1. 尝试从 Session 获取用户信息
// 2. 如果 Session 中没有，尝试从 Authorization 头获取 Access Token
// 3. 验证用户状态和角色
// 4. 将用户信息设置到上下文中
//
// 参数：
//   - c: Gin 上下文
//   - minRole: 最低角色要求
func authHelper(c *gin.Context, minRole int) {
	// 获取 Session
	session := sessions.Default(c)

	// 从 Session 获取用户信息
	username := session.Get("username")
	role := session.Get("role")
	id := session.Get("id")
	status := session.Get("status")
	useAccessToken := false

	// 如果 Session 中没有用户名，尝试 Access Token 认证
	if username == nil {
		// 从 Authorization 头获取 Access Token
		accessToken := c.Request.Header.Get("Authorization")

		// 如果没有 Token，返回未登录错误
		if accessToken == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": common.TranslateMessage(c, i18n.MsgAuthNotLoggedIn),
			})
			c.Abort()
			return
		}

		// 验证 Access Token
		user, authErr := model.ValidateAccessToken(accessToken)
		if authErr != nil {
			// 数据库错误
			if errors.Is(authErr, model.ErrDatabase) {
				common.SysLog("ValidateAccessToken database error: " + authErr.Error())
				c.JSON(http.StatusInternalServerError, gin.H{
					"success": false,
					"message": common.TranslateMessage(c, i18n.MsgDatabaseError),
				})
			} else {
				// Token 无效
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": common.TranslateMessage(c, i18n.MsgAuthAccessTokenInvalid),
				})
			}
			c.Abort()
			return
		}

		// 验证用户信息
		if user != nil && user.Username != "" {
			if !validUserInfo(user.Username, user.Role) {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": common.TranslateMessage(c, i18n.MsgAuthUserInfoInvalid),
				})
				c.Abort()
				return
			}

			// Token 有效，设置用户信息
			username = user.Username
			role = user.Role
			id = user.Id
			status = user.Status
			useAccessToken = true
		} else {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": common.TranslateMessage(c, i18n.MsgAuthAccessTokenInvalid),
			})
			c.Abort()
			return
		}
	}

	// 优先读取新的 NexusTok 用户标识头，同时兼容旧头，避免老客户端立即失效
	apiUserIdStr := c.Request.Header.Get("NexusTok-User")
	if apiUserIdStr == "" {
		apiUserIdStr = c.Request.Header.Get(legacyUserHeaderName)
	}
	if apiUserIdStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": common.TranslateMessage(c, i18n.MsgAuthUserIdNotProvided),
		})
		c.Abort()
		return
	}
	apiUserId, err := strconv.Atoi(apiUserIdStr)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": common.TranslateMessage(c, i18n.MsgAuthUserIdFormatError),
		})
		c.Abort()
		return

	}
	if id != apiUserId {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": common.TranslateMessage(c, i18n.MsgAuthUserIdMismatch),
		})
		c.Abort()
		return
	}
	if status.(int) == common.UserStatusDisabled {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": common.TranslateMessage(c, i18n.MsgAuthUserBanned),
		})
		c.Abort()
		return
	}
	if role.(int) < minRole {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": common.TranslateMessage(c, i18n.MsgAuthInsufficientPrivilege),
		})
		c.Abort()
		return
	}
	if !validUserInfo(username.(string), role.(int)) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": common.TranslateMessage(c, i18n.MsgAuthUserInfoInvalid),
		})
		c.Abort()
		return
	}
	// 防止不同NexusTok版本冲突，导致数据不通用
	c.Header("Auth-Version", "864b7076dbcd0a3c01b5520316720ebf")
	c.Set("username", username)
	c.Set("role", role)
	c.Set("id", id)
	c.Set("group", session.Get("group"))
	c.Set("user_group", session.Get("group"))
	c.Set("use_access_token", useAccessToken)

	if minRole >= common.RoleAdminUser {
		auditWriter, auditOwner := beginAdminAudit(c)
		c.Next()
		finishAdminAudit(c, auditWriter, auditOwner)
		return
	}

	c.Next()
}

// TryUserAuth 尝试用户认证中间件
// 不强制要求认证，如果用户已登录则设置用户 ID，未登录则继续处理
// 适用于可选认证的接口（如公开页面，登录用户可看到额外信息）
func TryUserAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		id := session.Get("id")
		if id != nil {
			c.Set("id", id)
		}
		c.Next()
	}
}

// UserAuth 用户认证中间件
// 要求用户必须登录，且角色 >= 普通用户（RoleCommonUser）
// 适用于需要登录的用户接口
func UserAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		authHelper(c, common.RoleCommonUser)
	}
}

// AdminAuth 管理员认证中间件
// 要求用户必须登录，且角色 >= 管理员（RoleAdminUser）
// 适用于管理后台接口
func AdminAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		authHelper(c, common.RoleAdminUser)
	}
}

// RootAuth Root 用户认证中间件
// 要求用户必须登录，且角色 >= Root 用户（RoleRootUser）
// 适用于最高权限接口（如系统设置、用户管理等）
func RootAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		authHelper(c, common.RoleRootUser)
	}
}

// RequirePermission 在 AdminAuth/RootAuth 之后执行资源动作权限校验。
//
// 认证中间件负责登录态、用户状态、角色下限和 NexusTok-User 头校验；这里仅按
// authz catalog 中的资源动作做第二层拦截。未注册 permission、普通用户误入或
// 认证上下文缺失都会失败关闭，避免路由权限表漏配时放宽管理边界。
func RequirePermission(permission authz.Permission) func(c *gin.Context) {
	return func(c *gin.Context) {
		role := c.GetInt("role")
		userID := c.GetInt("id")
		if authz.Can(userID, role, permission) {
			c.Next()
			return
		}
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": common.TranslateMessage(c, i18n.MsgAuthInsufficientPrivilege),
		})
		c.Abort()
	}
}

// WssAuth WebSocket 认证中间件
// 当前为空实现，WebSocket 认证通过查询参数或子协议传递 Token
func WssAuth(c *gin.Context) {

}

// TokenOrUserAuth 令牌或用户认证中间件
// 支持两种认证方式：Session 认证（Dashboard 用户）和 API Token 认证（API 客户端）
// 先尝试 Session 认证，如果失败则回退到 Token 认证
// 适用于需要同时支持 Web 界面和 API 调用的接口
func TokenOrUserAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		// Try session auth first (dashboard users)
		session := sessions.Default(c)
		if id := session.Get("id"); id != nil {
			if status, ok := session.Get("status").(int); ok && status == common.UserStatusEnabled {
				c.Set("id", id)
				c.Next()
				return
			}
		}
		// Fall back to token auth (API clients)
		TokenAuth()(c)
	}
}

// TokenAuthReadOnly 宽松版本的令牌认证中间件，用于只读查询接口
// 只验证令牌 key 是否存在，不检查令牌状态、过期时间和额度
// 即使令牌已过期、已耗尽或已禁用，也允许访问
// 仍然检查用户是否被封禁
//
// 适用场景：查询令牌信息、查询使用日志等只读操作
//
// 认证流程：
// 1. 从 Authorization 头获取 Bearer Token
// 2. 解析 Token key（支持 sk- 前缀和 - 分隔符格式）
// 3. 查询数据库验证 Token 存在性
// 4. 检查用户状态是否正常
// 5. 将用户 ID、Token ID、Token Key 设置到上下文
func TokenAuthReadOnly() func(c *gin.Context) {
	return func(c *gin.Context) {
		key := c.Request.Header.Get("Authorization")
		if key == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": common.TranslateMessage(c, i18n.MsgTokenNotProvided),
			})
			c.Abort()
			return
		}
		if strings.HasPrefix(key, "Bearer ") || strings.HasPrefix(key, "bearer ") {
			key = strings.TrimSpace(key[7:])
		}
		key = strings.TrimPrefix(key, "sk-")
		parts := strings.Split(key, "-")
		key = parts[0]

		token, err := model.GetTokenByKey(key, false)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusUnauthorized, gin.H{
					"success": false,
					"message": common.TranslateMessage(c, i18n.MsgTokenInvalid),
				})
			} else {
				common.SysLog("TokenAuthReadOnly GetTokenByKey database error: " + err.Error())
				c.JSON(http.StatusInternalServerError, gin.H{
					"success": false,
					"message": common.TranslateMessage(c, i18n.MsgDatabaseError),
				})
			}
			c.Abort()
			return
		}

		userCache, err := model.GetUserCache(token.UserId)
		if err != nil {
			common.SysLog(fmt.Sprintf("TokenAuthReadOnly GetUserCache error for user %d: %v", token.UserId, err))
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": common.TranslateMessage(c, i18n.MsgDatabaseError),
			})
			c.Abort()
			return
		}
		if userCache.Status != common.UserStatusEnabled {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": common.TranslateMessage(c, i18n.MsgAuthUserBanned),
			})
			c.Abort()
			return
		}

		c.Set("id", token.UserId)
		c.Set("token_id", token.Id)
		c.Set("token_key", token.Key)
		c.Next()
	}
}

// TokenAuth API Token 认证中间件
// 用于中继 API 的严格认证，验证 Token 的有效性、状态、过期时间和额度
//
// 支持的认证方式：
// 1. Authorization: Bearer sk-xxx 标准 OpenAI 格式
// 2. x-api-key: sk-xxx Anthropic Claude 格式
// 3. ?key=xxx 或 x-goog-api-key: xxx Google Gemini 格式
// 4. mj-api-secret: xxx Midjourney 格式
// 5. Sec-WebSocket-Protocol: openai-insecure-api-key.sk-xxx WebSocket 格式
//
// Token 格式：sk-<key>-<extra>，其中 key 是核心标识符
//
// 认证流程：
// 1. 从请求中提取 Token（支持多种格式）
// 2. 解析 Token key
// 3. 验证 Token 有效性和状态
// 4. 检查 Token IP 限制（如果有）
// 5. 检查用户状态
// 6. 检查 Token 分组权限
// 7. 设置上下文信息（用户 ID、Token 信息、分组等）
func TokenAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		// ========== WebSocket 协议处理 ==========
		// Sec-WebSocket-Protocol: realtime, openai-insecure-api-key.sk-xxx, openai-beta.realtime-v1
		if c.Request.Header.Get("Sec-WebSocket-Protocol") != "" {
			key := c.Request.Header.Get("Sec-WebSocket-Protocol")
			parts := strings.Split(key, ",")
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if strings.HasPrefix(part, "openai-insecure-api-key") {
					key = strings.TrimPrefix(part, "openai-insecure-api-key.")
					break
				}
			}
			c.Request.Header.Set("Authorization", "Bearer "+key)
		}
		// ========== Anthropic Claude 格式处理 ==========
		// /v1/messages 和 /v1/models 路径支持 x-api-key 头
		if strings.Contains(c.Request.URL.Path, "/v1/messages") || strings.Contains(c.Request.URL.Path, "/v1/models") {
			anthropicKey := c.Request.Header.Get("x-api-key")
			if anthropicKey != "" {
				c.Request.Header.Set("Authorization", "Bearer "+anthropicKey)
			}
		}
		// ========== Google Gemini 格式处理 ==========
		// /v1beta/models/ 和 /v1/models/ 路径支持 ?key= 查询参数和 x-goog-api-key 头
		if strings.HasPrefix(c.Request.URL.Path, "/v1beta/models") ||
			strings.HasPrefix(c.Request.URL.Path, "/v1beta/openai/models") ||
			strings.HasPrefix(c.Request.URL.Path, "/v1/models/") {
			skKey := c.Query("key")
			if skKey != "" {
				c.Request.Header.Set("Authorization", "Bearer "+skKey)
			}
			xGoogKey := c.Request.Header.Get("x-goog-api-key")
			if xGoogKey != "" {
				c.Request.Header.Set("Authorization", "Bearer "+xGoogKey)
			}
		}
		// ========== 提取并解析 Token ==========
		key := c.Request.Header.Get("Authorization")
		parts := make([]string, 0)
		if strings.HasPrefix(key, "Bearer ") || strings.HasPrefix(key, "bearer ") {
			key = strings.TrimSpace(key[7:])
		}
		// 处理 Midjourney 代理格式
		if key == "" || key == "midjourney-proxy" {
			key = c.Request.Header.Get("mj-api-secret")
			if strings.HasPrefix(key, "Bearer ") || strings.HasPrefix(key, "bearer ") {
				key = strings.TrimSpace(key[7:])
			}
			key = strings.TrimPrefix(key, "sk-")
			parts = strings.Split(key, "-")
			key = parts[0]
		} else {
			key = strings.TrimPrefix(key, "sk-")
			parts = strings.Split(key, "-")
			key = parts[0]
		}
		// ========== 验证 Token ==========
		token, err := model.ValidateUserToken(key)
		if token != nil {
			id := c.GetInt("id")
			if id == 0 {
				c.Set("id", token.UserId)
			}
		}
		if err != nil {
			if errors.Is(err, model.ErrDatabase) {
				common.SysLog("TokenAuth ValidateUserToken database error: " + err.Error())
				abortWithOpenAiMessage(c, http.StatusInternalServerError,
					common.TranslateMessage(c, i18n.MsgDatabaseError))
			} else {
				abortWithOpenAiMessage(c, http.StatusUnauthorized,
					common.TranslateMessage(c, i18n.MsgTokenInvalid))
			}
			return
		}

		// ========== 检查 Token IP 限制 ==========
		allowIps := token.GetIpLimits()
		if len(allowIps) > 0 {
			clientIp := c.ClientIP()
			logger.LogDebug(c, "Token has IP restrictions, checking client IP %s", clientIp)
			ip := net.ParseIP(clientIp)
			if ip == nil {
				abortWithOpenAiMessage(c, http.StatusForbidden, "无法解析客户端 IP 地址")
				return
			}
			if common.IsIpInCIDRList(ip, allowIps) == false {
				abortWithOpenAiMessage(c, http.StatusForbidden, "您的 IP 不在令牌允许访问的列表中", types.ErrorCodeAccessDenied)
				return
			}
			logger.LogDebug(c, "Client IP %s passed the token IP restrictions check", clientIp)
		}

		// ========== 检查用户状态 ==========
		userCache, err := model.GetUserCache(token.UserId)
		if err != nil {
			common.SysLog(fmt.Sprintf("TokenAuth GetUserCache error for user %d: %v", token.UserId, err))
			abortWithOpenAiMessage(c, http.StatusInternalServerError,
				common.TranslateMessage(c, i18n.MsgDatabaseError))
			return
		}
		userEnabled := userCache.Status == common.UserStatusEnabled
		if !userEnabled {
			abortWithOpenAiMessage(c, http.StatusForbidden, common.TranslateMessage(c, i18n.MsgAuthUserBanned))
			return
		}

		// 将用户信息写入上下文
		userCache.WriteContext(c)

		// ========== 检查分组权限 ==========
		userGroup := userCache.Group
		tokenGroup := token.Group
		if tokenGroup != "" {
			// 检查 Token 分组是否在用户可用分组列表中
			if _, ok := service.GetUserUsableGroups(userGroup)[tokenGroup]; !ok {
				abortWithOpenAiMessage(c, http.StatusForbidden, fmt.Sprintf("无权访问 %s 分组", tokenGroup))
				return
			}
			// 检查分组是否在分组比率配置中（auto 分组特殊处理）
			if !ratio_setting.ContainsGroupRatio(tokenGroup) {
				if tokenGroup != "auto" {
					abortWithOpenAiMessage(c, http.StatusForbidden, fmt.Sprintf("分组 %s 已被弃用", tokenGroup))
					return
				}
			}
			userGroup = tokenGroup
		}
		common.SetContextKey(c, constant.ContextKeyUsingGroup, userGroup)

		// ========== 设置 Token 上下文 ==========
		err = SetupContextForToken(c, token, parts...)
		if err != nil {
			return
		}
		c.Next()
	}
}

// SetupContextForToken 将 Token 信息设置到请求上下文
// 设置内容包括用户 ID、Token 信息、额度、模型限制等
// 如果 Token key 中包含额外部分（parts），且用户是管理员，则可以指定特定渠道
//
// 参数：
//   - c: Gin 上下文
//   - token: Token 对象
//   - parts: Token key 的额外部分（用于指定渠道 ID）
//
// 返回值：
//   - error: 设置错误，nil 表示成功
func SetupContextForToken(c *gin.Context, token *model.Token, parts ...string) error {
	if token == nil {
		return fmt.Errorf("token is nil")
	}
	c.Set("id", token.UserId)
	c.Set("token_id", token.Id)
	c.Set("token_key", token.Key)
	c.Set("token_name", token.Name)
	c.Set("token_unlimited_quota", token.UnlimitedQuota)
	if !token.UnlimitedQuota {
		c.Set("token_quota", token.RemainQuota)
	}
	if token.ModelLimitsEnabled {
		c.Set("token_model_limit_enabled", true)
		c.Set("token_model_limit", token.GetModelLimitsMap())
	} else {
		c.Set("token_model_limit_enabled", false)
	}
	common.SetContextKey(c, constant.ContextKeyTokenGroup, token.Group)
	common.SetContextKey(c, constant.ContextKeyTokenCrossGroupRetry, token.CrossGroupRetry)
	if len(parts) > 1 {
		if model.IsAdmin(token.UserId) {
			c.Set("specific_channel_id", parts[1])
		} else {
			c.Header("specific_channel_version", "701e3ae1dc3f7975556d354e0675168d004891c8")
			abortWithOpenAiMessage(c, http.StatusForbidden, "普通用户不支持指定渠道")
			return fmt.Errorf("普通用户不支持指定渠道")
		}
	}
	return nil
}
