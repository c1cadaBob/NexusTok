// Package controller - custom_oauth.go
// 该文件实现了自定义 OAuth 提供商管理的 API 控制器
//
// 自定义 OAuth 允许管理员配置第三方 OAuth 提供商（如 GitLab、Keycloak 等）
// 用户可以绑定/解绑自定义 OAuth 账户进行登录
//
// 主要 API：
// - GetCustomOAuthProviders：获取所有自定义 OAuth 提供商
// - GetCustomOAuthProvider：获取单个提供商详情
// - CreateCustomOAuthProvider：创建提供商
// - UpdateCustomOAuthProvider：更新提供商
// - DeleteCustomOAuthProvider：删除提供商
// - FetchCustomOAuthDiscovery：获取 OIDC Discovery 配置
// - GetUserOAuthBindings：获取当前用户的 OAuth 绑定
// - UnbindCustomOAuth：解绑 OAuth 账户
package controller

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/oauth"
	"github.com/gin-gonic/gin"
)

// CustomOAuthProviderResponse 自定义 OAuth 提供商响应结构体
// 排除了 client_secret 等敏感字段
type CustomOAuthProviderResponse struct {
	Id                    int    `json:"id"`
	Name                  string `json:"name"`
	Slug                  string `json:"slug"`
	Icon                  string `json:"icon"`
	Enabled               bool   `json:"enabled"`
	ClientId              string `json:"client_id"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserInfoEndpoint      string `json:"user_info_endpoint"`
	Scopes                string `json:"scopes"`
	UserIdField           string `json:"user_id_field"`
	UsernameField         string `json:"username_field"`
	DisplayNameField      string `json:"display_name_field"`
	EmailField            string `json:"email_field"`
	WellKnown             string `json:"well_known"`
	AuthStyle             int    `json:"auth_style"`
	AccessPolicy          string `json:"access_policy"`
	AccessDeniedMessage   string `json:"access_denied_message"`
}

// UserOAuthBindingResponse 用户 OAuth 绑定响应结构体
type UserOAuthBindingResponse struct {
	ProviderId     int    `json:"provider_id"`
	ProviderName   string `json:"provider_name"`
	ProviderSlug   string `json:"provider_slug"`
	ProviderIcon   string `json:"provider_icon"`
	ProviderUserId string `json:"provider_user_id"`
}

// toCustomOAuthProviderResponse 将模型对象转换为响应对象（排除敏感字段）
func toCustomOAuthProviderResponse(p *model.CustomOAuthProvider) *CustomOAuthProviderResponse {
	return &CustomOAuthProviderResponse{
		Id:                    p.Id,
		Name:                  p.Name,
		Slug:                  p.Slug,
		Icon:                  p.Icon,
		Enabled:               p.Enabled,
		ClientId:              p.ClientId,
		AuthorizationEndpoint: p.AuthorizationEndpoint,
		TokenEndpoint:         p.TokenEndpoint,
		UserInfoEndpoint:      p.UserInfoEndpoint,
		Scopes:                p.Scopes,
		UserIdField:           p.UserIdField,
		UsernameField:         p.UsernameField,
		DisplayNameField:      p.DisplayNameField,
		EmailField:            p.EmailField,
		WellKnown:             p.WellKnown,
		AuthStyle:             p.AuthStyle,
		AccessPolicy:          p.AccessPolicy,
		AccessDeniedMessage:   p.AccessDeniedMessage,
	}
}

// GetCustomOAuthProviders 获取所有自定义 OAuth 提供商列表
//
// 返回排除敏感字段的提供商列表
func GetCustomOAuthProviders(c *gin.Context) {
	providers, err := model.GetAllCustomOAuthProviders()
	if err != nil {
		common.ApiError(c, err)
		return
	}

	response := make([]*CustomOAuthProviderResponse, len(providers))
	for i, p := range providers {
		response[i] = toCustomOAuthProviderResponse(p)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    response,
	})
}

// GetCustomOAuthProvider 获取单个自定义 OAuth 提供商详情
//
// 路径参数：
//   - id: 提供商 ID
func GetCustomOAuthProvider(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		common.ApiErrorMsg(c, "无效的 ID")
		return
	}

	provider, err := model.GetCustomOAuthProviderById(id)
	if err != nil {
		common.ApiErrorMsg(c, "未找到该 OAuth 提供商")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    toCustomOAuthProviderResponse(provider),
	})
}

// CreateCustomOAuthProviderRequest 创建自定义 OAuth 提供商请求结构体
type CreateCustomOAuthProviderRequest struct {
	Name                  string `json:"name" binding:"required"`
	Slug                  string `json:"slug" binding:"required"`
	Icon                  string `json:"icon"`
	Enabled               bool   `json:"enabled"`
	ClientId              string `json:"client_id" binding:"required"`
	ClientSecret          string `json:"client_secret" binding:"required"`
	AuthorizationEndpoint string `json:"authorization_endpoint" binding:"required"`
	TokenEndpoint         string `json:"token_endpoint" binding:"required"`
	UserInfoEndpoint      string `json:"user_info_endpoint" binding:"required"`
	Scopes                string `json:"scopes"`
	UserIdField           string `json:"user_id_field"`
	UsernameField         string `json:"username_field"`
	DisplayNameField      string `json:"display_name_field"`
	EmailField            string `json:"email_field"`
	WellKnown             string `json:"well_known"`
	AuthStyle             int    `json:"auth_style"`
	AccessPolicy          string `json:"access_policy"`
	AccessDeniedMessage   string `json:"access_denied_message"`
}

// FetchCustomOAuthDiscoveryRequest 获取 OIDC Discovery 配置请求结构体
type FetchCustomOAuthDiscoveryRequest struct {
	WellKnownURL string `json:"well_known_url"`
	IssuerURL    string `json:"issuer_url"`
}

// FetchCustomOAuthDiscovery 通过后端获取 OIDC Discovery 配置（仅管理员）
//
// 支持两种方式：
//   - 直接提供 well_known_url
//   - 提供 issuer_url，自动拼接 /.well-known/openid-configuration
func FetchCustomOAuthDiscovery(c *gin.Context) {
	var req FetchCustomOAuthDiscoveryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "无效的请求参数: "+err.Error())
		return
	}

	wellKnownURL := strings.TrimSpace(req.WellKnownURL)
	issuerURL := strings.TrimSpace(req.IssuerURL)

	if wellKnownURL == "" && issuerURL == "" {
		common.ApiErrorMsg(c, "请先填写 Discovery URL 或 Issuer URL")
		return
	}

	targetURL := wellKnownURL
	if targetURL == "" {
		targetURL = strings.TrimRight(issuerURL, "/") + "/.well-known/openid-configuration"
	}
	targetURL = strings.TrimSpace(targetURL)

	parsedURL, err := url.Parse(targetURL)
	if err != nil || parsedURL.Host == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		common.ApiErrorMsg(c, "Discovery URL 无效，仅支持 http/https")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		common.ApiErrorMsg(c, "创建 Discovery 请求失败: "+err.Error())
		return
	}
	httpReq.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		common.ApiErrorMsg(c, "获取 Discovery 配置失败: "+err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = resp.Status
		}
		common.ApiErrorMsg(c, "获取 Discovery 配置失败: "+message)
		return
	}

	var discovery map[string]any
	if err = common.DecodeJson(resp.Body, &discovery); err != nil {
		common.ApiErrorMsg(c, "解析 Discovery 配置失败: "+err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"well_known_url": targetURL,
			"discovery":      discovery,
		},
	})
}

// CreateCustomOAuthProvider 创建新的自定义 OAuth 提供商
//
// 创建前会检查 slug 是否与现有提供商冲突
// 创建成功后会注册到 OAuth 提供商注册表
func CreateCustomOAuthProvider(c *gin.Context) {
	var req CreateCustomOAuthProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "无效的请求参数: "+err.Error())
		return
	}

	// Check if slug is already taken
	if model.IsSlugTaken(req.Slug, 0) {
		common.ApiErrorMsg(c, "该 Slug 已被使用")
		return
	}

	// Check if slug conflicts with built-in providers
	if oauth.IsProviderRegistered(req.Slug) && !oauth.IsCustomProvider(req.Slug) {
		common.ApiErrorMsg(c, "该 Slug 与内置 OAuth 提供商冲突")
		return
	}

	provider := &model.CustomOAuthProvider{
		Name:                  req.Name,
		Slug:                  req.Slug,
		Icon:                  req.Icon,
		Enabled:               req.Enabled,
		ClientId:              req.ClientId,
		ClientSecret:          req.ClientSecret,
		AuthorizationEndpoint: req.AuthorizationEndpoint,
		TokenEndpoint:         req.TokenEndpoint,
		UserInfoEndpoint:      req.UserInfoEndpoint,
		Scopes:                req.Scopes,
		UserIdField:           req.UserIdField,
		UsernameField:         req.UsernameField,
		DisplayNameField:      req.DisplayNameField,
		EmailField:            req.EmailField,
		WellKnown:             req.WellKnown,
		AuthStyle:             req.AuthStyle,
		AccessPolicy:          req.AccessPolicy,
		AccessDeniedMessage:   req.AccessDeniedMessage,
	}

	if err := model.CreateCustomOAuthProvider(provider); err != nil {
		common.ApiError(c, err)
		return
	}

	// Register the provider in the OAuth registry
	oauth.RegisterOrUpdateCustomProvider(provider)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "创建成功",
		"data":    toCustomOAuthProviderResponse(provider),
	})
}

// UpdateCustomOAuthProviderRequest 更新自定义 OAuth 提供商请求结构体
// 使用指针类型字段支持部分更新（nil 表示不更新）
type UpdateCustomOAuthProviderRequest struct {
	Name                  string  `json:"name"`
	Slug                  string  `json:"slug"`
	Icon                  *string `json:"icon"`    // Optional: if nil, keep existing
	Enabled               *bool   `json:"enabled"` // Optional: if nil, keep existing
	ClientId              string  `json:"client_id"`
	ClientSecret          string  `json:"client_secret"` // Optional: if empty, keep existing
	AuthorizationEndpoint string  `json:"authorization_endpoint"`
	TokenEndpoint         string  `json:"token_endpoint"`
	UserInfoEndpoint      string  `json:"user_info_endpoint"`
	Scopes                string  `json:"scopes"`
	UserIdField           string  `json:"user_id_field"`
	UsernameField         string  `json:"username_field"`
	DisplayNameField      string  `json:"display_name_field"`
	EmailField            string  `json:"email_field"`
	WellKnown             *string `json:"well_known"`            // Optional: if nil, keep existing
	AuthStyle             *int    `json:"auth_style"`            // Optional: if nil, keep existing
	AccessPolicy          *string `json:"access_policy"`         // Optional: if nil, keep existing
	AccessDeniedMessage   *string `json:"access_denied_message"` // Optional: if nil, keep existing
}

// UpdateCustomOAuthProvider 更新现有的自定义 OAuth 提供商
//
// 如果 slug 发生变化，会先注销旧 slug 再注册新 slug
func UpdateCustomOAuthProvider(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		common.ApiErrorMsg(c, "无效的 ID")
		return
	}

	var req UpdateCustomOAuthProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "无效的请求参数: "+err.Error())
		return
	}

	// Get existing provider
	provider, err := model.GetCustomOAuthProviderById(id)
	if err != nil {
		common.ApiErrorMsg(c, "未找到该 OAuth 提供商")
		return
	}

	oldSlug := provider.Slug

	// Check if new slug is taken by another provider
	if req.Slug != "" && req.Slug != provider.Slug {
		if model.IsSlugTaken(req.Slug, id) {
			common.ApiErrorMsg(c, "该 Slug 已被使用")
			return
		}
		// Check if slug conflicts with built-in providers
		if oauth.IsProviderRegistered(req.Slug) && !oauth.IsCustomProvider(req.Slug) {
			common.ApiErrorMsg(c, "该 Slug 与内置 OAuth 提供商冲突")
			return
		}
	}

	// Update fields
	if req.Name != "" {
		provider.Name = req.Name
	}
	if req.Slug != "" {
		provider.Slug = req.Slug
	}
	if req.Icon != nil {
		provider.Icon = *req.Icon
	}
	if req.Enabled != nil {
		provider.Enabled = *req.Enabled
	}
	if req.ClientId != "" {
		provider.ClientId = req.ClientId
	}
	if req.ClientSecret != "" {
		provider.ClientSecret = req.ClientSecret
	}
	if req.AuthorizationEndpoint != "" {
		provider.AuthorizationEndpoint = req.AuthorizationEndpoint
	}
	if req.TokenEndpoint != "" {
		provider.TokenEndpoint = req.TokenEndpoint
	}
	if req.UserInfoEndpoint != "" {
		provider.UserInfoEndpoint = req.UserInfoEndpoint
	}
	if req.Scopes != "" {
		provider.Scopes = req.Scopes
	}
	if req.UserIdField != "" {
		provider.UserIdField = req.UserIdField
	}
	if req.UsernameField != "" {
		provider.UsernameField = req.UsernameField
	}
	if req.DisplayNameField != "" {
		provider.DisplayNameField = req.DisplayNameField
	}
	if req.EmailField != "" {
		provider.EmailField = req.EmailField
	}
	if req.WellKnown != nil {
		provider.WellKnown = *req.WellKnown
	}
	if req.AuthStyle != nil {
		provider.AuthStyle = *req.AuthStyle
	}
	if req.AccessPolicy != nil {
		provider.AccessPolicy = *req.AccessPolicy
	}
	if req.AccessDeniedMessage != nil {
		provider.AccessDeniedMessage = *req.AccessDeniedMessage
	}

	if err := model.UpdateCustomOAuthProvider(provider); err != nil {
		common.ApiError(c, err)
		return
	}

	// Update the provider in the OAuth registry
	if oldSlug != provider.Slug {
		oauth.UnregisterCustomProvider(oldSlug)
	}
	oauth.RegisterOrUpdateCustomProvider(provider)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "更新成功",
		"data":    toCustomOAuthProviderResponse(provider),
	})
}

// DeleteCustomOAuthProvider 删除自定义 OAuth 提供商
//
// 删除前会检查是否存在用户绑定，如有绑定则拒绝删除
func DeleteCustomOAuthProvider(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		common.ApiErrorMsg(c, "无效的 ID")
		return
	}

	// Get existing provider to get slug
	provider, err := model.GetCustomOAuthProviderById(id)
	if err != nil {
		common.ApiErrorMsg(c, "未找到该 OAuth 提供商")
		return
	}

	// Check if there are any user bindings
	count, err := model.GetBindingCountByProviderId(id)
	if err != nil {
		common.SysError("Failed to get binding count for provider " + strconv.Itoa(id) + ": " + err.Error())
		common.ApiErrorMsg(c, "检查用户绑定时发生错误，请稍后重试")
		return
	}
	if count > 0 {
		common.ApiErrorMsg(c, "该 OAuth 提供商还有用户绑定，无法删除。请先解除所有用户绑定。")
		return
	}

	if err := model.DeleteCustomOAuthProvider(id); err != nil {
		common.ApiError(c, err)
		return
	}

	// Unregister the provider from the OAuth registry
	oauth.UnregisterCustomProvider(provider.Slug)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "删除成功",
	})
}

// buildUserOAuthBindingsResponse 构建用户 OAuth 绑定响应
//
// 查询用户的所有 OAuth 绑定并附加提供商信息
func buildUserOAuthBindingsResponse(userId int) ([]UserOAuthBindingResponse, error) {
	bindings, err := model.GetUserOAuthBindingsByUserId(userId)
	if err != nil {
		return nil, err
	}

	response := make([]UserOAuthBindingResponse, 0, len(bindings))
	for _, binding := range bindings {
		provider, err := model.GetCustomOAuthProviderById(binding.ProviderId)
		if err != nil {
			continue
		}
		response = append(response, UserOAuthBindingResponse{
			ProviderId:     binding.ProviderId,
			ProviderName:   provider.Name,
			ProviderSlug:   provider.Slug,
			ProviderIcon:   provider.Icon,
			ProviderUserId: binding.ProviderUserId,
		})
	}

	return response, nil
}

// GetUserOAuthBindings 获取当前用户的所有 OAuth 绑定
func GetUserOAuthBindings(c *gin.Context) {
	userId := c.GetInt("id")
	if userId == 0 {
		common.ApiErrorMsg(c, "未登录")
		return
	}

	response, err := buildUserOAuthBindingsResponse(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    response,
	})
}

// GetUserOAuthBindingsByAdmin 管理员获取指定用户的 OAuth 绑定
//
// 路径参数：
//   - id: 用户 ID
func GetUserOAuthBindingsByAdmin(c *gin.Context) {
	userIdStr := c.Param("id")
	userId, err := strconv.Atoi(userIdStr)
	if err != nil {
		common.ApiErrorMsg(c, "invalid user id")
		return
	}

	targetUser, err := model.GetUserById(userId, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	myRole := c.GetInt("role")
	if myRole <= targetUser.Role && myRole != common.RoleRootUser {
		common.ApiErrorMsg(c, "no permission")
		return
	}

	response, err := buildUserOAuthBindingsResponse(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    response,
	})
}

// UnbindCustomOAuth 解绑当前用户的指定 OAuth 提供商
//
// 路径参数：
//   - provider_id: 提供商 ID
func UnbindCustomOAuth(c *gin.Context) {
	userId := c.GetInt("id")
	if userId == 0 {
		common.ApiErrorMsg(c, "未登录")
		return
	}

	providerIdStr := c.Param("provider_id")
	providerId, err := strconv.Atoi(providerIdStr)
	if err != nil {
		common.ApiErrorMsg(c, "无效的提供商 ID")
		return
	}

	if err := model.DeleteUserOAuthBinding(userId, providerId); err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "解绑成功",
	})
}

// UnbindCustomOAuthByAdmin 管理员解绑指定用户的 OAuth 提供商
//
// 路径参数：
//   - id: 用户 ID
//   - provider_id: 提供商 ID
func UnbindCustomOAuthByAdmin(c *gin.Context) {
	userIdStr := c.Param("id")
	userId, err := strconv.Atoi(userIdStr)
	if err != nil {
		common.ApiErrorMsg(c, "invalid user id")
		return
	}

	targetUser, err := model.GetUserById(userId, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	myRole := c.GetInt("role")
	if myRole <= targetUser.Role && myRole != common.RoleRootUser {
		common.ApiErrorMsg(c, "no permission")
		return
	}

	providerIdStr := c.Param("provider_id")
	providerId, err := strconv.Atoi(providerIdStr)
	if err != nil {
		common.ApiErrorMsg(c, "invalid provider id")
		return
	}

	if err := model.DeleteUserOAuthBinding(userId, providerId); err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "success",
	})
}
