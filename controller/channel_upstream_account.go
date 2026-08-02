package controller

import (
	"context"
	"net/http"
	"strconv"
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
