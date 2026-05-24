// Package controller - playground.go
// 该文件实现了 Playground（在线测试）功能的 API 控制器
//
// Playground 允许已登录用户在不使用 API Token 的情况下测试 AI 模型
// 通过创建临时 Token 并复用 Relay 通道实现
//
// 主要 API：
// - Playground：处理 Playground 请求，转发到 Relay
package controller

import (
	"errors"
	"fmt"

	"github.com/c1cada/NexusTok/middleware"
	"github.com/c1cada/NexusTok/model"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/types"

	"github.com/gin-gonic/gin"
)

// Playground 处理 Playground 请求
//
// 流程：
// 1. 验证用户已登录（不支持 access token）
// 2. 生成 Relay 信息
// 3. 写入用户上下文
// 4. 创建临时 Token
// 5. 转发到 Relay 处理
//
// 参数：
//   - c: Gin 上下文
func Playground(c *gin.Context) {
	var newAPIError *types.NexusTokError

	defer func() {
		if newAPIError != nil {
			c.JSON(newAPIError.StatusCode, gin.H{
				"error": newAPIError.ToOpenAIError(),
			})
		}
	}()

	useAccessToken := c.GetBool("use_access_token")
	if useAccessToken {
		newAPIError = types.NewError(errors.New("暂不支持使用 access token"), types.ErrorCodeAccessDenied, types.ErrOptionWithSkipRetry())
		return
	}

	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatOpenAI, nil, nil)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
		return
	}

	userId := c.GetInt("id")

	// Write user context to ensure acceptUnsetRatio is available
	userCache, err := model.GetUserCache(userId)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeQueryDataError, types.ErrOptionWithSkipRetry())
		return
	}
	userCache.WriteContext(c)

	tempToken := &model.Token{
		UserId: userId,
		Name:   fmt.Sprintf("playground-%s", relayInfo.UsingGroup),
		Group:  relayInfo.UsingGroup,
	}
	_ = middleware.SetupContextForToken(c, tempToken)

	Relay(c, types.RelayFormatOpenAI)
}
