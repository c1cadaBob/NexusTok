package controller

import (
	"context"
	"net/http"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/service/upstreamaccount"

	"github.com/gin-gonic/gin"
)

const upstreamAccountPreviewTimeout = 45 * time.Second

// PreviewUpstreamAccount 使用临时账号密码读取目标平台密钥、分组、倍率和余额预览。
//
// 账号密码只用于本次后端请求，不会落库；完整 API Key 仅保存在短期预览缓存中，
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
