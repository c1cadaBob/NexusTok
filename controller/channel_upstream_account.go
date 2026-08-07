package controller

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/service/upstreamaccount"

	"github.com/gin-gonic/gin"
)

const upstreamAccountPreviewTimeout = 45 * time.Second

// PreviewUpstreamAccount 使用临时账号密码或已保存的上游凭据读取目标平台密钥、分组、
// 倍率和余额预览。
//
// 账号密码只用于本次后端请求，不会落库；如果请求携带 channel_id 且不再提交密码，
// 后端会从该渠道已保存的加密凭据中恢复登录。完整 API Key 仅保存在短期预览缓存中，
// 返回给前端的 snapshot 会清空 key 字段，只保留 masked_key 供管理员确认。
func PreviewUpstreamAccount(c *gin.Context) {
	var req upstreamaccount.PreviewRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "无效的请求参数: "+err.Error())
		return
	}
	if err := applyUpstreamCaptureCredential(c, &req.Credential); err != nil {
		common.ApiError(c, err)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), upstreamAccountPreviewTimeout)
	defer cancel()
	result, err := upstreamaccount.Preview(ctx, req)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    result,
	})
}

// CompleteUpstreamAccount2FA 使用二次验证码继续上游账号预览。
//
// 第一阶段只缓存目标平台 pending session，不缓存账号密码。验证码完成后后端继续读取
// 目标平台密钥、分组、倍率和余额，并返回与普通预览一致的脱敏 snapshot。
func CompleteUpstreamAccount2FA(c *gin.Context) {
	var req upstreamaccount.Preview2FARequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "无效的请求参数: "+err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), upstreamAccountPreviewTimeout)
	defer cancel()
	result, err := upstreamaccount.CompletePreview2FA(ctx, req)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    result,
	})
}

// CreateUpstreamAccountChannel 根据预览快照创建一个渠道和多条渠道账号。
//
// 请求只引用 preview_id 和用户在页面上确认后的配置；后端从短期缓存取完整 key，
// 因此前端不需要也不应该回传完整密钥。
func CreateUpstreamAccountChannel(c *gin.Context) {
	var req upstreamaccount.CreateRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "无效的请求参数: "+err.Error())
		return
	}
	result, err := upstreamaccount.CreateFromPreview(req)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

// RefreshUpstreamAccountChannel 使用重新输入或已保存的上游账号凭据刷新已有账号同步渠道。
//
// 刷新不会保存账号密码；后端会优先复用渠道 settings 中已保存的加密凭据，若本次
// 请求显式提交了账号密码，则会用新凭据重新登录目标平台，并把新快照应用到已有
// ChannelAccount。缺失密钥是否自动禁用由请求中的 disable_missing_key 控制。
func RefreshUpstreamAccountChannel(c *gin.Context) {
	channelID, err := strconv.Atoi(c.Param("id"))
	if err != nil || channelID <= 0 {
		common.ApiErrorMsg(c, "无效的渠道 ID")
		return
	}
	var req upstreamaccount.RefreshRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "无效的请求参数: "+err.Error())
		return
	}
	req.ChannelID = channelID
	if err := applyUpstreamCaptureCredential(c, &req.Credential); err != nil {
		common.ApiError(c, err)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), upstreamAccountPreviewTimeout)
	defer cancel()
	result, err := upstreamaccount.RefreshChannelFromCredential(ctx, req)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

// StartUpstreamAccountBrowserAuth 是目标站 OAuth 自动化的预留入口。
//
// 第一版不模拟 new-api/sub2api 站点中的 GitHub、LinuxDO/L 站等第三方登录流程；这些
// 目标站常常叠加验证码、人机验证、站点自定义回调和同源策略。这里先保留稳定 API
// 形状，后续按具体平台实现 provider 时不需要再调整前端调用位置。
func StartUpstreamAccountBrowserAuth(c *gin.Context) {
	var req upstreamaccount.BrowserAuthRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "无效的请求参数: "+err.Error())
		return
	}
	result, err := upstreamaccount.StartBrowserAuth(c.Request.Context(), req)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

// CompleteUpstreamAccountBrowserAuth 是目标站 OAuth 自动化的预留完成入口。
func CompleteUpstreamAccountBrowserAuth(c *gin.Context) {
	var req upstreamaccount.BrowserAuthRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "无效的请求参数: "+err.Error())
		return
	}
	result, err := upstreamaccount.CompleteBrowserAuth(c.Request.Context(), req)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

// StartUpstreamAccountCaptureSession 创建油猴脚本登录态采集会话。
func StartUpstreamAccountCaptureSession(c *gin.Context) {
	var req upstreamaccount.CaptureSessionStartRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "无效的请求参数: "+err.Error())
		return
	}
	result, err := upstreamaccount.StartCaptureSession(c.GetInt("id"), req, externalRequestBaseURL(c), frontendRequestBaseURL(c))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

// GetUpstreamAccountCaptureSession 返回当前采集会话的脱敏状态。
func GetUpstreamAccountCaptureSession(c *gin.Context) {
	result, err := upstreamaccount.GetCaptureSessionStatus(c.GetInt("id"), c.Param("id"), externalRequestBaseURL(c))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

// GetUpstreamAccountCaptureUserscript 动态生成仅匹配目标站的 Tampermonkey 脚本。
func GetUpstreamAccountCaptureUserscript(c *gin.Context) {
	script, err := upstreamaccount.RenderCaptureUserscriptWithInstallToken(c.Param("id"), c.Query("install_token"), externalRequestBaseURL(c))
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "install token") {
			c.Header("Cache-Control", "no-store")
			c.Header("Content-Type", "text/plain; charset=utf-8")
			c.String(http.StatusForbidden, "capture helper install token is invalid")
			return
		}
		if strings.Contains(strings.ToLower(err.Error()), "expired") {
			c.Header("Cache-Control", "no-store")
			c.Header("Content-Type", "text/plain; charset=utf-8")
			c.String(http.StatusGone, "capture helper session expired")
			return
		}
		common.ApiError(c, err)
		return
	}
	c.Header("Content-Type", "application/javascript; charset=utf-8")
	c.Header("Cache-Control", "no-store")
	c.Header("X-Nexustok-Helper-Version", upstreamaccount.CaptureHelperVersion())
	c.String(http.StatusOK, script)
}

// GetUpstreamAccountCaptureHelperUserscript 返回可长期安装的稳定采集助手脚本。
//
// 该脚本本身不包含 capture_secret 或目标站登录态，只在管理员创建短时采集会话并跳转
// 到目标站时，加载对应会话的一次性签名脚本。公开下载不会授予 NexusTok 管理权限。
func GetUpstreamAccountCaptureHelperUserscript(c *gin.Context) {
	script, err := upstreamaccount.RenderCaptureHelperUserscript(externalRequestBaseURL(c))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.Header("Content-Type", "application/javascript; charset=utf-8")
	c.Header("Cache-Control", "no-store")
	c.Header("X-Nexustok-Helper-Version", upstreamaccount.CaptureHelperVersion())
	c.String(http.StatusOK, script)
}

// CompleteUpstreamAccountCaptureSession 接收油猴脚本回传的目标站登录态。
//
// 该接口不依赖 NexusTok 登录态，因为脚本运行在目标站页面中，无法稳定携带 NexusTok
// 后台 Cookie；安全边界由一次性 capture_secret、目标 origin 校验和短 TTL 缓存共同保证。
func CompleteUpstreamAccountCaptureSession(c *gin.Context) {
	var req upstreamaccount.CaptureSessionCompleteRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "无效的请求参数: "+err.Error())
		return
	}
	result, err := upstreamaccount.CompleteCaptureSession(c.Param("id"), req)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

// ParseUpstreamAccountCredential 解析管理员手动粘贴的登录态并返回脱敏摘要。
//
// 该接口只服务页面即时校验，不会保存 token/cookie，也不会把明文凭据回传到浏览器。
func ParseUpstreamAccountCredential(c *gin.Context) {
	var req upstreamaccount.CredentialParseRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "无效的请求参数: "+err.Error())
		return
	}
	result, err := upstreamaccount.ParseCredentialDraft(req)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func applyUpstreamCaptureCredential(c *gin.Context, credential *upstreamaccount.Credential) error {
	if credential == nil || strings.TrimSpace(credential.CaptureID) == "" {
		return nil
	}
	captured, err := upstreamaccount.ResolveCaptureCredential(c.GetInt("id"), credential.CaptureID)
	if err != nil {
		return err
	}
	captureID := credential.CaptureID
	*credential = captured
	credential.CaptureID = captureID
	return nil
}

func externalRequestBaseURL(c *gin.Context) string {
	scheme := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto"))
	if scheme == "" {
		scheme = strings.TrimSpace(c.GetHeader("X-Forwarded-Protocol"))
	}
	if scheme == "" {
		scheme = "http"
		if c.Request != nil && c.Request.TLS != nil {
			scheme = "https"
		}
	}
	host := strings.TrimSpace(c.GetHeader("X-Forwarded-Host"))
	if host == "" {
		host = strings.TrimSpace(c.Request.Host)
	}
	return scheme + "://" + host
}

// frontendRequestBaseURL 返回发起后台页面请求的前端来源。
//
// 正式部署通常由同一个域名托管前端和 API，此时 externalRequestBaseURL 足够。
// 本地开发和部分反向代理会让浏览器访问前端 dev server，再由 dev server 代理 API；
// 后端看到的 Host 是 API 地址，而 return_url 属于前端地址。这里仅把 Origin/Referer
// 提取成“允许回跳来源”，不用于生成 userscript 下载地址或 complete 回调地址。
func frontendRequestBaseURL(c *gin.Context) string {
	for _, raw := range []string{
		strings.TrimSpace(c.GetHeader("Origin")),
		strings.TrimSpace(c.GetHeader("Referer")),
	} {
		if raw == "" {
			continue
		}
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			continue
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			continue
		}
		return parsed.Scheme + "://" + parsed.Host
	}
	return ""
}
