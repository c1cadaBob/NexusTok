package service

import (
	"net/http"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/types"

	"github.com/gin-gonic/gin"
)

// ProcessPoolAccountError 只更新全局账号池账号状态，不自动禁用整个渠道。
func ProcessPoolAccountError(c *gin.Context, channelError types.ChannelError, err *types.NexusTokError) {
	if err == nil || channelError.PoolAccountId <= 0 {
		return
	}
	ExcludePoolAccountForRequest(c, channelError.PoolAccountId)
	common.SetContextKey(c, constant.ContextKeyChannelAccountRetryChannelId, channelError.ChannelId)

	reason := err.ErrorWithStatusCode()
	updates := map[string]interface{}{
		"last_error": reason,
	}

	if shouldDisableChannelAccount(err) && channelError.AutoBan {
		updates["status"] = common.ChannelStatusAutoDisabled
		updates["schedulable"] = false
		updates["disabled_reason"] = reason
		updates["temp_disabled_until"] = 0
		updates["rate_limited_until"] = 0
		updates["overload_until"] = 0
	} else if err.StatusCode == http.StatusTooManyRequests {
		updates["rate_limited_until"] = retryAfterUntil(err.RetryAfter, defaultChannelAccountRateLimitCooldown)
		updates["disabled_reason"] = reason
	} else if isChannelAccountOverloadError(err) {
		updates["overload_until"] = common.GetTimestamp() + int64(defaultChannelAccountOverloadCooldown.Seconds())
		updates["disabled_reason"] = reason
	}

	if updateErr := model.UpdatePoolAccountErrorState(channelError.PoolAccountId, updates); updateErr != nil {
		common.SysLog("failed to update pool account error state: " + updateErr.Error())
	}
}
