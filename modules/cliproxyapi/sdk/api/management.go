// api - management.go
// 该文件暴露管理端点的辅助函数，用于嵌入 CLIProxyAPI。
// 包装内部管理处理器类型和辅助函数，使外部项目可以集成管理端点
// 而无需导入内部包。包括 token 请求、OAuth 会话管理等功能。

// Package api exposes helpers for embedding CLIProxyAPI.
//
// It wraps internal management handler types and helpers so external projects
// can integrate management endpoints without importing internal packages.
package api

import (
	"context"

	"github.com/gin-gonic/gin"
	internalmanagement "github.com/router-for-me/CLIProxyAPI/v7/internal/api/handlers/management"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
)

// Handler 重新导出内部 HTTP API 使用的管理处理器。
type Handler = internalmanagement.Handler

// ManagementTokenRequester 暴露用于请求 token 的管理端点子集。
type ManagementTokenRequester interface {
	// RequestAnthropicToken 请求 Anthropic token
	RequestAnthropicToken(*gin.Context)
	// RequestGeminiCLIToken 请求 Gemini CLI token
	RequestGeminiCLIToken(*gin.Context)
	// RequestCodexToken 请求 Codex token
	RequestCodexToken(*gin.Context)
	// RequestAntigravityToken 请求 Antigravity token
	RequestAntigravityToken(*gin.Context)
	// RequestKimiToken 请求 Kimi token
	RequestKimiToken(*gin.Context)
	// GetAuthStatus 获取认证状态
	GetAuthStatus(c *gin.Context)
	// PostOAuthCallback 处理 OAuth 回调
	PostOAuthCallback(c *gin.Context)
}

// managementTokenRequester 是 ManagementTokenRequester 的内部实现。
type managementTokenRequester struct {
	handler *Handler
}

// NewHandler 为 SDK 消费者创建管理处理器。
func NewHandler(cfg *config.Config, configFilePath string, manager *coreauth.Manager) *Handler {
	return internalmanagement.NewHandler(cfg, configFilePath, manager)
}

// NewHandlerWithoutConfigFilePath 创建不持久化配置文件的管理处理器。
func NewHandlerWithoutConfigFilePath(cfg *config.Config, manager *coreauth.Manager) *Handler {
	return internalmanagement.NewHandlerWithoutConfigFilePath(cfg, manager)
}

// NewManagementTokenRequester 创建仅暴露 token 请求端点的受限管理处理器。
func NewManagementTokenRequester(cfg *config.Config, manager *coreauth.Manager) ManagementTokenRequester {
	return &managementTokenRequester{
		handler: NewHandlerWithoutConfigFilePath(cfg, manager),
	}
}

func (m *managementTokenRequester) RequestAnthropicToken(c *gin.Context) {
	m.handler.RequestAnthropicToken(c)
}

func (m *managementTokenRequester) RequestGeminiCLIToken(c *gin.Context) {
	m.handler.RequestGeminiCLIToken(c)
}

func (m *managementTokenRequester) RequestCodexToken(c *gin.Context) {
	m.handler.RequestCodexToken(c)
}

func (m *managementTokenRequester) RequestAntigravityToken(c *gin.Context) {
	m.handler.RequestAntigravityToken(c)
}

func (m *managementTokenRequester) RequestKimiToken(c *gin.Context) {
	m.handler.RequestKimiToken(c)
}

func (m *managementTokenRequester) GetAuthStatus(c *gin.Context) {
	m.handler.GetAuthStatus(c)
}

func (m *managementTokenRequester) PostOAuthCallback(c *gin.Context) {
	m.handler.PostOAuthCallback(c)
}

// WriteConfig 将管理配置持久化到磁盘。
func WriteConfig(path string, data []byte) error {
	return internalmanagement.WriteConfig(path, data)
}

// RegisterOAuthSession 记录待处理的 OAuth 回调状态。
func RegisterOAuthSession(state, provider string) {
	internalmanagement.RegisterOAuthSession(state, provider)
}

// SetOAuthSessionError 存储 OAuth 会话的错误信息。
func SetOAuthSessionError(state, message string) {
	internalmanagement.SetOAuthSessionError(state, message)
}

// CompleteOAuthSession 将单个 OAuth 会话标记为已完成。
func CompleteOAuthSession(state string) {
	internalmanagement.CompleteOAuthSession(state)
}

// CompleteOAuthSessionsByProvider 移除指定提供商的所有待处理 OAuth 会话。
func CompleteOAuthSessionsByProvider(provider string) int {
	return internalmanagement.CompleteOAuthSessionsByProvider(provider)
}

// GetOAuthSession 返回当前 OAuth 会话状态。
func GetOAuthSession(state string) (provider string, status string, ok bool) {
	return internalmanagement.GetOAuthSession(state)
}

// IsOAuthSessionPending 检查指定的 provider/state 对是否仍在等待中。
func IsOAuthSessionPending(state, provider string) bool {
	return internalmanagement.IsOAuthSessionPending(state, provider)
}

// ValidateOAuthState 验证 OAuth state token 的有效性。
func ValidateOAuthState(state string) error {
	return internalmanagement.ValidateOAuthState(state)
}

// NormalizeOAuthProvider 将提供商名称规范化为标准形式。
func NormalizeOAuthProvider(provider string) (string, error) {
	return internalmanagement.NormalizeOAuthProvider(provider)
}

// WriteOAuthCallbackFile 将 OAuth 回调载荷写入磁盘。
func WriteOAuthCallbackFile(authDir, provider, state, code, errorMessage string) (string, error) {
	return internalmanagement.WriteOAuthCallbackFile(authDir, provider, state, code, errorMessage)
}

// WriteOAuthCallbackFileForPendingSession 为待处理的 OAuth 会话写入回调载荷。
func WriteOAuthCallbackFileForPendingSession(authDir, provider, state, code, errorMessage string) (string, error) {
	return internalmanagement.WriteOAuthCallbackFileForPendingSession(authDir, provider, state, code, errorMessage)
}

// PopulateAuthContext 将 Gin 上下文中的认证元数据复制到请求上下文中。
func PopulateAuthContext(ctx context.Context, c *gin.Context) context.Context {
	return internalmanagement.PopulateAuthContext(ctx, c)
}
