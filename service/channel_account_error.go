package service

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/types"

	"github.com/gin-gonic/gin"
)

const (
	defaultChannelAccountRateLimitCooldown = 5 * time.Minute
	defaultChannelAccountOverloadCooldown  = 10 * time.Minute
)

func ProcessChannelAccountError(c *gin.Context, channelError types.ChannelError, err *types.NexusTokError) {
	if err == nil || channelError.ChannelAccountId <= 0 {
		return
	}
	ExcludeChannelAccountForRequest(c, channelError.ChannelAccountId)
	common.SetContextKey(c, constant.ContextKeyChannelAccountRetryChannelId, channelError.ChannelId)

	reason := err.ErrorWithStatusCode()
	updates := map[string]interface{}{
		"last_error": reason,
	}

	if shouldDisableChannelAccount(err) && channelError.AutoBan {
		updates["status"] = common.ChannelStatusAutoDisabled
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

	if updateErr := model.UpdateChannelAccountErrorState(channelError.ChannelId, channelError.ChannelAccountId, updates); updateErr != nil {
		common.SysLog("failed to update channel account error state: " + updateErr.Error())
	}
}

func shouldDisableChannelAccount(err *types.NexusTokError) bool {
	if err == nil {
		return false
	}
	if err.StatusCode == http.StatusUnauthorized || err.StatusCode == http.StatusForbidden {
		return true
	}
	if err.GetErrorCode() == types.ErrorCodeChannelInvalidKey {
		return true
	}
	return ShouldDisableChannel(err)
}

func isChannelAccountOverloadError(err *types.NexusTokError) bool {
	if err == nil {
		return false
	}
	if err.StatusCode == 529 {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "overload")
}

func retryAfterUntil(retryAfter string, defaultCooldown time.Duration) int64 {
	now := time.Now()
	retryAfter = strings.TrimSpace(retryAfter)
	if retryAfter == "" {
		return now.Add(defaultCooldown).Unix()
	}
	if seconds, err := strconv.Atoi(retryAfter); err == nil && seconds > 0 {
		return now.Add(time.Duration(seconds) * time.Second).Unix()
	}
	if retryTime, err := http.ParseTime(retryAfter); err == nil && retryTime.After(now) {
		return retryTime.Unix()
	}
	return now.Add(defaultCooldown).Unix()
}
