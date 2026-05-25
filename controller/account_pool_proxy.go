// Package controller - account_pool_proxy.go
// 该文件实现了账号池管理接口的反向代理
//
// 将 NexusTok 管理员的请求转发到内部 CLIProxyAPI 管理接口
// 用于管理外部账号池服务（如独立部署的账号池 Sidecar）
//
// 代理特性：
// - 自动添加管理密钥认证
// - 移除逐跳（Hop-by-Hop）头
// - 保留原始请求方法和查询参数
// - 超时和错误处理
package controller

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/middleware"
	"github.com/c1cada/NexusTok/model"
	relayconstant "github.com/c1cada/NexusTok/relay/constant"
	"github.com/c1cada/NexusTok/service"
	"github.com/c1cada/NexusTok/types"

	"github.com/gin-gonic/gin"
)

// accountPoolProxyClient 用于转发请求的 HTTP 客户端
var accountPoolProxyClient = &http.Client{}

// accountPoolHopByHopHeaders 需要移除的逐跳头
//
// 逐跳头（Hop-by-Hop Headers）是 HTTP/1.1 中定义的只对单次传输有意义的头
// 在代理转发时需要移除，参见 RFC 2616 Section 13.5.1
var accountPoolHopByHopHeaders = map[string]struct{}{
	"Connection":          {},
	"Keep-Alive":          {},
	"Proxy-Authenticate":  {},
	"Proxy-Authorization": {},
	"Te":                  {},
	"Trailer":             {},
	"Transfer-Encoding":   {},
	"Upgrade":             {},
}

// accountPoolMainRelayCredentialHeaders 表示 CPAMC api-call 传入的上游凭证头。
//
// CPAMC 的 api-call 原本会在 CLIProxyAPI 内部把 $TOKEN$ 替换为官方账号凭证，
// 然后直接请求上游。现在模型请求改为交给 NexusTok 主 Relay 处理，所以这些
// 头不能继续进入 Relay 链路，否则可能覆盖渠道 Key、绕过账号池组，或让上游
// 官方账号凭证出现在主项目的请求日志和参数覆盖上下文里。
var accountPoolMainRelayCredentialHeaders = map[string]struct{}{
	"Api-Key":        {},
	"Authorization":  {},
	"Mj-Api-Secret":  {},
	"X-Api-Key":      {},
	"X-Goog-Api-Key": {},
}

// accountPoolAPICallRequest 是 CPAMC /api-call 的请求体。
//
// 字段同时兼容 auth_index、authIndex 和 AuthIndex，是为了保持与 CLIProxyAPI
// 管理接口一致。带 authIndex 的请求通常需要 CLIProxyAPI 读取具体官方账号并
// 进行 Token 刷新或 $TOKEN$ 替换，主项目无法在不暴露凭据的前提下完整模拟，
// 因此这类请求仍会继续走原来的 Sidecar 管理代理。
type accountPoolAPICallRequest struct {
	AuthIndexSnake  *string           `json:"auth_index"`
	AuthIndexCamel  *string           `json:"authIndex"`
	AuthIndexPascal *string           `json:"AuthIndex"`
	Method          string            `json:"method"`
	URL             string            `json:"url"`
	Header          map[string]string `json:"header"`
	Data            string            `json:"data"`
}

// accountPoolAPICallResponse 保持 CPAMC apiCallApi 期望的响应包裹格式。
//
// 主 Relay 返回的是 OpenAI/Claude/Gemini 等原始响应体；CPAMC 前端则统一读取
// status_code/header/body 三个字段，因此内部重放完成后仍需要包一层兼容结构。
type accountPoolAPICallResponse struct {
	StatusCode int                 `json:"status_code"`
	Header     map[string][]string `json:"header"`
	Body       string              `json:"body"`
}

// accountPoolMainRelayTarget 表示 CPAMC api-call 可交给 NexusTok 主 Relay 处理的目标。
type accountPoolMainRelayTarget struct {
	Method      string            // HTTP 方法
	Path        string            // NexusTok Relay 路径
	RawQuery    string            // 原始查询参数（移除上游凭据类 query 后的结果）
	RelayFormat types.RelayFormat // Relay 响应格式
	RelayMode   int               // Relay 模式，供无法仅靠路径推断的入口使用
}

// AccountPoolManagementProxy 将 NexusTok 管理员态请求转发到内部 CLIProxyAPI 管理接口
//
// 代理流程：
// 1. 获取目标服务地址（service.AccountPoolCLIProxyURL）
// 2. 构建代理请求 URL
// 3. 复制原始请求头并添加管理密钥
// 4. 移除逐跳头
// 5. 发送代理请求
// 6. 复制响应头和响应体
//
// 参数：
//   - c: Gin 上下文
func AccountPoolManagementProxy(c *gin.Context) {
	if isAccountPoolAPICallPath(c.Param("path")) && c.Request.Method == http.MethodPost {
		if handled := tryHandleAccountPoolMainRelayAPICall(c); handled {
			return
		}
	}

	// 获取目标服务地址
	targetBase := service.AccountPoolCLIProxyURL()

	// 构建代理请求 URL
	targetURL, err := buildAccountPoolProxyURL(targetBase, c.Param("path"), c.Request.URL.RawQuery)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"success": false,
			"message": "账号池服务地址配置错误",
		})
		return
	}

	// 创建代理请求
	proxyReq, err := http.NewRequestWithContext(c.Request.Context(), c.Request.Method, targetURL.String(), c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"success": false,
			"message": "账号池服务请求创建失败",
		})
		return
	}

	// 复制请求头
	proxyReq.ContentLength = c.Request.ContentLength
	copyAccountPoolHeaders(proxyReq.Header, c.Request.Header)
	removeAccountPoolHopByHopHeaders(proxyReq.Header)

	// 替换认证头为管理密钥
	proxyReq.Header.Del("Authorization")
	proxyReq.Header.Del("Proxy-Authorization")
	proxyReq.Header.Set("Authorization", "Bearer "+service.AccountPoolCLIProxyManagementKey())
	proxyReq.Host = targetURL.Host

	// 发送代理请求
	resp, err := accountPoolProxyClient.Do(proxyReq)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"success": false,
			"message": "账号池服务不可用",
		})
		return
	}
	defer resp.Body.Close()

	// 复制响应头
	copyAccountPoolHeaders(c.Writer.Header(), resp.Header)
	removeAccountPoolHopByHopHeaders(c.Writer.Header())

	// 设置响应状态码并复制响应体
	c.Status(resp.StatusCode)
	if _, err = io.Copy(c.Writer, resp.Body); err != nil {
		_ = c.Error(fmt.Errorf("转发账号池响应失败: %w", err))
	}
}

// tryHandleAccountPoolMainRelayAPICall 尝试把 CPAMC 模型 api-call 重放到 NexusTok 主 Relay。
//
// 返回值含义：
// - true：本函数已经写入响应，调用方必须直接 return；
// - false：该请求不适合主 Relay 处理，调用方应继续走 CLIProxyAPI Sidecar 代理。
//
// 当前只拦截“不带 authIndex 的模型调用”。原因是：
//   - OpenAI/Claude/Gemini 兼容的模型测试请求本身就是标准模型请求，交给主 Relay
//     才能统一执行主项目的请求规则、参数覆盖、日志、计费和渠道选择；
//   - 额度查询、账号巡检、OAuth Token 刷新等请求依赖 CPAMC/CLIProxyAPI 的官方账号
//     运行时状态，强行交给主 Relay 会丢失 Token 替换能力，反而破坏账号管理功能。
func tryHandleAccountPoolMainRelayAPICall(c *gin.Context) bool {
	rawBody, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "读取账号池 api-call 请求体失败"})
		return true
	}
	restoreAccountPoolRequestBody(c, rawBody)

	var payload accountPoolAPICallRequest
	if err := common.Unmarshal(rawBody, &payload); err != nil {
		return false
	}

	target, ok := resolveAccountPoolMainRelayTarget(&payload)
	if !ok {
		return false
	}

	result, err := executeAccountPoolMainRelayAPICall(c, payload, target)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return true
	}
	c.JSON(http.StatusOK, result)
	return true
}

// restoreAccountPoolRequestBody 恢复请求体，确保不适合主 Relay 的请求仍可无损转发给 Sidecar。
func restoreAccountPoolRequestBody(c *gin.Context, body []byte) {
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	c.Request.ContentLength = int64(len(body))
}

// resolveAccountPoolMainRelayTarget 判断 CPAMC api-call 是否属于可由 NexusTok Relay 接管的模型请求。
//
// 支持范围聚焦在“会携带模型请求体并触发主项目规则”的路径上，例如：
// - /v1/chat/completions
// - /v1/responses
// - /v1/messages
// - /v1/embeddings
// - /v1/images/*
// - /v1/audio/*
// - /v1/rerank
// - /v1beta/models/{model}:generateContent
//
// GET /models、账号额度查询、profile 查询等不在这里接管，继续由 CLIProxyAPI 处理。
func resolveAccountPoolMainRelayTarget(payload *accountPoolAPICallRequest) (accountPoolMainRelayTarget, bool) {
	if payload == nil {
		return accountPoolMainRelayTarget{}, false
	}
	if firstAccountPoolNonEmptyString(payload.AuthIndexSnake, payload.AuthIndexCamel, payload.AuthIndexPascal) != "" {
		return accountPoolMainRelayTarget{}, false
	}
	method := strings.ToUpper(strings.TrimSpace(payload.Method))
	if method == "" {
		method = http.MethodGet
	}
	if method != http.MethodPost {
		return accountPoolMainRelayTarget{}, false
	}
	parsedURL, err := url.Parse(strings.TrimSpace(payload.URL))
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return accountPoolMainRelayTarget{}, false
	}

	path := normalizeAccountPoolRelayPath(parsedURL.Path)
	if path == "" {
		return accountPoolMainRelayTarget{}, false
	}
	rawQuery := stripAccountPoolCredentialQuery(parsedURL.Query()).Encode()
	target := accountPoolMainRelayTarget{
		Method:   method,
		Path:     path,
		RawQuery: rawQuery,
	}

	switch {
	case strings.HasPrefix(path, "/v1/messages"):
		target.RelayFormat = types.RelayFormatClaude
		target.RelayMode = relayconstant.RelayModeChatCompletions
	case strings.HasPrefix(path, "/v1/responses/compact"):
		target.RelayFormat = types.RelayFormatOpenAIResponsesCompaction
		target.RelayMode = relayconstant.RelayModeResponsesCompact
	case strings.HasPrefix(path, "/v1/responses"):
		target.RelayFormat = types.RelayFormatOpenAIResponses
		target.RelayMode = relayconstant.RelayModeResponses
	case strings.HasPrefix(path, "/v1/embeddings") || strings.Contains(path, "/embeddings"):
		target.RelayFormat = types.RelayFormatEmbedding
		target.RelayMode = relayconstant.RelayModeEmbeddings
	case strings.HasPrefix(path, "/v1/images/") || path == "/v1/edits":
		target.RelayFormat = types.RelayFormatOpenAIImage
		target.RelayMode = relayconstant.Path2RelayMode(path)
	case strings.HasPrefix(path, "/v1/audio/"):
		target.RelayFormat = types.RelayFormatOpenAIAudio
		target.RelayMode = relayconstant.Path2RelayMode(path)
	case strings.HasPrefix(path, "/v1/rerank"):
		target.RelayFormat = types.RelayFormatRerank
		target.RelayMode = relayconstant.RelayModeRerank
	case strings.HasPrefix(path, "/v1beta/models/") || strings.HasPrefix(path, "/v1/models/"):
		target.RelayFormat = types.RelayFormatGemini
		target.RelayMode = relayconstant.RelayModeGemini
	case strings.HasPrefix(path, "/v1/chat/completions") || strings.HasPrefix(path, "/v1/completions"):
		target.RelayFormat = types.RelayFormatOpenAI
		target.RelayMode = relayconstant.Path2RelayMode(path)
	default:
		return accountPoolMainRelayTarget{}, false
	}
	if target.RelayMode == relayconstant.RelayModeUnknown {
		target.RelayMode = relayconstant.Path2RelayMode(path)
	}
	return target, true
}

// normalizeAccountPoolRelayPath 将外部上游 URL 路径规整为 NexusTok Relay 路径。
//
// CPAMC 页面中既可能填写 https://api.openai.com/v1，也可能填写已经包含
// /chat/completions 的兼容网关地址。这里仅做路径层面的归一化，不读取或改写
// 请求体，保证真正的模型上下文仍由主项目 Relay 解析。
func normalizeAccountPoolRelayPath(path string) string {
	path = "/" + strings.TrimLeft(strings.TrimSpace(path), "/")
	for strings.Contains(path, "//") {
		path = strings.ReplaceAll(path, "//", "/")
	}
	switch {
	case path == "/" || path == "":
		return ""
	case strings.HasPrefix(path, "/v1/") || strings.HasPrefix(path, "/v1beta/"):
		return path
	case strings.HasPrefix(path, "/chat/completions"):
		return "/v1" + path
	case strings.HasPrefix(path, "/completions"):
		return "/v1" + path
	case strings.HasPrefix(path, "/responses"):
		return "/v1" + path
	case strings.HasPrefix(path, "/messages"):
		return "/v1" + path
	case strings.HasPrefix(path, "/embeddings"):
		return "/v1" + path
	case strings.HasPrefix(path, "/images/"):
		return "/v1" + path
	case strings.HasPrefix(path, "/audio/"):
		return "/v1" + path
	case strings.HasPrefix(path, "/rerank"):
		return "/v1" + path
	default:
		return path
	}
}

// stripAccountPoolCredentialQuery 移除 URL query 中常见的上游凭据字段。
//
// Gemini 等 API 支持通过 ?key= 上游密钥鉴权。主 Relay 重放时凭证来源必须是
// NexusTok 渠道或账号池组，不能继续使用 CPAMC 表单中的上游密钥，否则会让
// 主项目规则和账号池调度失去意义。
func stripAccountPoolCredentialQuery(values url.Values) url.Values {
	out := url.Values{}
	for key, list := range values {
		if strings.EqualFold(key, "key") || strings.EqualFold(key, "api_key") {
			continue
		}
		for _, value := range list {
			out.Add(key, value)
		}
	}
	return out
}

// executeAccountPoolMainRelayAPICall 在内存中构造一个新的 Gin context 并执行主 Relay。
//
// 不直接复用当前 c.Writer 的原因是 CPAMC apiCallApi 需要固定的
// {status_code, header, body} 包裹格式，而主 Relay 会写出 OpenAI/Claude/Gemini
// 原始响应。使用 httptest.ResponseRecorder 可以完整捕获主 Relay 的响应，再
// 以 CPAMC 兼容格式返回给页面。
func executeAccountPoolMainRelayAPICall(c *gin.Context, payload accountPoolAPICallRequest, target accountPoolMainRelayTarget) (accountPoolAPICallResponse, error) {
	relayPath := target.Path
	if target.RawQuery != "" {
		relayPath += "?" + target.RawQuery
	}

	recorder := httptest.NewRecorder()
	relayCtx, _ := gin.CreateTestContext(recorder)
	requestBody := strings.NewReader(payload.Data)
	relayReq := httptest.NewRequest(target.Method, relayPath, requestBody)
	relayReq.ContentLength = int64(len(payload.Data))
	copyAccountPoolMainRelayHeaders(relayReq.Header, payload.Header)
	if payload.Data != "" && relayReq.Header.Get("Content-Type") == "" {
		relayReq.Header.Set("Content-Type", "application/json")
	}
	relayCtx.Request = relayReq
	relayCtx.Set("account_pool_main_relay", true)
	relayCtx.Set("relay_mode", target.RelayMode)
	copyAccountPoolGinContextValues(relayCtx, c)

	userID := c.GetInt("id")
	userCache, err := model.GetUserCache(userID)
	if err != nil {
		return accountPoolAPICallResponse{}, fmt.Errorf("读取当前用户缓存失败: %w", err)
	}
	userCache.WriteContext(relayCtx)
	common.SetContextKey(relayCtx, constant.ContextKeyUsingGroup, userCache.Group)
	tempToken := &model.Token{
		UserId:         userID,
		Name:           fmt.Sprintf("cpamc-main-relay-%d", userID),
		Key:            fmt.Sprintf("cpamc-main-relay-%d", userID),
		Group:          userCache.Group,
		UnlimitedQuota: true,
		Status:         common.TokenStatusEnabled,
	}
	if err := middleware.SetupContextForToken(relayCtx, tempToken); err != nil {
		return accountPoolAPICallResponse{}, err
	}

	channel, ok := middleware.PrepareRelayChannelContext(relayCtx)
	defer common.CleanupBodyStorage(relayCtx)
	if ok {
		defer service.ReleaseSelectedChannelAccount(relayCtx)
		defer service.ReleaseSelectedPoolAccount(relayCtx)
		Relay(relayCtx, target.RelayFormat)
		middleware.RecordRelayChannelAffinityIfSucceeded(relayCtx, channel)
	}

	return accountPoolAPICallResponse{
		StatusCode: recorder.Code,
		Header:     cloneAccountPoolResponseHeaders(recorder.Header()),
		Body:       recorder.Body.String(),
	}, nil
}

// copyAccountPoolGinContextValues 复制主请求中与身份、日志追踪有关的 Gin context 值。
func copyAccountPoolGinContextValues(dst *gin.Context, src *gin.Context) {
	if dst == nil || src == nil {
		return
	}
	for _, key := range []string{
		"id",
		"username",
		"role",
		"group",
		"user_group",
		"use_access_token",
		common.RequestIdKey,
	} {
		if value, ok := src.Get(key); ok {
			dst.Set(key, value)
		}
	}
}

// copyAccountPoolMainRelayHeaders 复制 CPAMC 请求头到主 Relay，并移除凭据类头。
func copyAccountPoolMainRelayHeaders(dst http.Header, src map[string]string) {
	for key, value := range src {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, hop := accountPoolHopByHopHeaders[http.CanonicalHeaderKey(key)]; hop {
			continue
		}
		if _, credential := accountPoolMainRelayCredentialHeaders[http.CanonicalHeaderKey(key)]; credential {
			continue
		}
		dst.Set(key, value)
	}
	dst.Set("X-NexusTok-CPAMC-Main-Relay", "true")
}

// cloneAccountPoolResponseHeaders 克隆主 Relay 响应头，避免后续写响应时被修改。
func cloneAccountPoolResponseHeaders(src http.Header) map[string][]string {
	out := make(map[string][]string, len(src))
	for key, values := range src {
		copied := make([]string, len(values))
		copy(copied, values)
		out[key] = copied
	}
	return out
}

// firstAccountPoolNonEmptyString 返回第一个非空字符串指针值。
func firstAccountPoolNonEmptyString(values ...*string) string {
	for _, value := range values {
		if value == nil {
			continue
		}
		if text := strings.TrimSpace(*value); text != "" {
			return text
		}
	}
	return ""
}

// isAccountPoolAPICallPath 判断当前管理代理路径是否为 CPAMC 通用 api-call。
func isAccountPoolAPICallPath(path string) bool {
	path = "/" + strings.TrimLeft(strings.TrimSpace(path), "/")
	return path == "/api-call"
}

// buildAccountPoolProxyURL 构建代理请求的完整 URL
//
// URL 格式：{base}/v0/management{path}?{query}
//
// 参数：
//   - base: 目标服务基础 URL
//   - rawPath: 原始请求路径
//   - rawQuery: 原始查询参数
//
// 返回值：
//   - *url.URL: 构建的 URL
//   - error: URL 解析错误
func buildAccountPoolProxyURL(base string, rawPath string, rawQuery string) (*url.URL, error) {
	parsedBase, err := url.Parse(base)
	if err != nil {
		return nil, err
	}
	if parsedBase.Scheme == "" || parsedBase.Host == "" {
		return nil, fmt.Errorf("invalid account pool proxy url: %s", base)
	}
	proxyPath := rawPath
	if proxyPath == "" {
		proxyPath = "/"
	}
	if !strings.HasPrefix(proxyPath, "/") {
		proxyPath = "/" + proxyPath
	}
	// 拼接管理 API 路径前缀
	parsedBase.Path = strings.TrimRight(parsedBase.Path, "/") + "/v0/management" + proxyPath
	parsedBase.RawQuery = rawQuery
	return parsedBase, nil
}

// copyAccountPoolHeaders 复制 HTTP 头
//
// 参数：
//   - dst: 目标头
//   - src: 源头
func copyAccountPoolHeaders(dst http.Header, src http.Header) {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

// removeAccountPoolHopByHopHeaders 移除逐跳头
//
// 逐跳头在代理转发时需要移除，包括：
// - Connection 头中列出的头
// - 标准逐跳头列表中的头
//
// 参数：
//   - header: 需要处理的 HTTP 头
func removeAccountPoolHopByHopHeaders(header http.Header) {
	// 移除 Connection 头中列出的头
	for _, value := range header.Values("Connection") {
		for _, token := range strings.Split(value, ",") {
			if token = strings.TrimSpace(token); token != "" {
				header.Del(token)
			}
		}
	}
	// 移除标准逐跳头
	for key := range accountPoolHopByHopHeaders {
		header.Del(key)
	}
}
