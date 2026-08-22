package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/service"
	"github.com/c1cada/NexusTok/types"
	"github.com/gin-gonic/gin"
)

// SetupContextForRoutingCandidate 按统一调度已经选中的候选写入 Relay 上下文。
//
// 与旧的 SetupContextForSelectedChannel 不同，本函数不会再次在渠道内部轮询 key/account，
// 而是按候选中的 multi-key 索引、ChannelAccount ID 或 PoolAccount ID 精确取凭证。这样可以
// 保证“统一评分选中的密钥”就是本次请求真正调用的密钥。
func SetupContextForRoutingCandidate(c *gin.Context, candidate *model.RoutingCandidate, modelName string) *types.NexusTokError {
	c.Set("original_model", modelName)
	clearUpstreamRatioConversionContext(c)
	if candidate == nil {
		return types.NewError(errors.New("routing candidate is nil"), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}
	channel, err := model.CacheGetChannel(candidate.ChannelID)
	if err != nil || channel == nil {
		if err == nil {
			err = fmt.Errorf("渠道# %d 不存在", candidate.ChannelID)
		}
		return types.NewError(err, types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}
	service.ReleaseSelectedChannelAccount(c)
	service.ReleaseSelectedPoolAccount(c)
	setupRoutingCandidateBaseContext(c, channel, candidate, modelName)

	switch candidate.Kind {
	case model.RoutingCredentialKindSingleKey:
		return setupSingleKeyRoutingCandidate(c, channel)
	case model.RoutingCredentialKindMultiKey:
		return setupMultiKeyRoutingCandidate(c, channel, candidate.MultiKeyIndex)
	case model.RoutingCredentialKindChannelAccount:
		return setupChannelAccountRoutingCandidate(c, channel, candidate, modelName)
	case model.RoutingCredentialKindPoolAccount:
		return setupPoolAccountRoutingCandidate(c, channel, candidate, modelName)
	default:
		return types.NewErrorWithStatusCode(fmt.Errorf("未知路由候选类型: %s", candidate.Kind), types.ErrorCodeChannelNoAvailableKey, http.StatusServiceUnavailable, types.ErrOptionWithSkipRetry())
	}
}

func setupRoutingCandidateBaseContext(c *gin.Context, channel *model.Channel, candidate *model.RoutingCandidate, modelName string) {
	common.SetContextKey(c, constant.ContextKeyRoutingCandidate, candidate.Clone())
	common.SetContextKey(c, constant.ContextKeyChannelId, channel.Id)
	common.SetContextKey(c, constant.ContextKeyChannelName, channel.Name)
	common.SetContextKey(c, constant.ContextKeyChannelType, channel.Type)
	common.SetContextKey(c, constant.ContextKeyChannelCreateTime, channel.CreatedTime)
	common.SetContextKey(c, constant.ContextKeyChannelAutoBan, channel.GetAutoBan())
	common.SetContextKey(c, constant.ContextKeyChannelCredentialMode, channel.GetCredentialMode())
	common.SetContextKey(c, constant.ContextKeyChannelAccountPool, false)
	common.SetContextKey(c, constant.ContextKeyChannelAccountId, 0)
	common.SetContextKey(c, constant.ContextKeyChannelAccountName, "")
	common.SetContextKey(c, constant.ContextKeyPoolGroupId, 0)
	common.SetContextKey(c, constant.ContextKeyPoolGroupName, "")
	common.SetContextKey(c, constant.ContextKeyPoolAccountId, 0)
	common.SetContextKey(c, constant.ContextKeyPoolAccountName, "")
	common.SetContextKey(c, constant.ContextKeyPoolAccountAuthType, "")
	c.Set("model", modelName)
}

func setupSingleKeyRoutingCandidate(c *gin.Context, channel *model.Channel) *types.NexusTokError {
	applyChannelContext(c, channel, nil)
	common.SetContextKey(c, constant.ContextKeyChannelIsMultiKey, false)
	common.SetContextKey(c, constant.ContextKeyChannelMultiKeyIndex, 0)
	common.SetContextKey(c, constant.ContextKeyChannelKey, channel.Key)
	common.SetContextKey(c, constant.ContextKeySystemPromptOverride, false)
	return nil
}

func setupMultiKeyRoutingCandidate(c *gin.Context, channel *model.Channel, keyIndex int) *types.NexusTokError {
	keys := channel.GetKeys()
	if keyIndex < 0 || keyIndex >= len(keys) {
		return types.NewErrorWithStatusCode(fmt.Errorf("多密钥索引 %d 不可用", keyIndex), types.ErrorCodeChannelNoAvailableKey, http.StatusServiceUnavailable, types.ErrOptionWithSkipRetry())
	}
	if channel.ChannelInfo.MultiKeyStatusList != nil {
		if status, ok := channel.ChannelInfo.MultiKeyStatusList[keyIndex]; ok && status != common.ChannelStatusEnabled {
			return types.NewErrorWithStatusCode(fmt.Errorf("多密钥索引 %d 未启用", keyIndex), types.ErrorCodeChannelNoAvailableKey, http.StatusServiceUnavailable, types.ErrOptionWithSkipRetry())
		}
	}
	key := strings.TrimSpace(keys[keyIndex])
	if key == "" {
		return types.NewErrorWithStatusCode(fmt.Errorf("多密钥索引 %d 为空", keyIndex), types.ErrorCodeChannelNoAvailableKey, http.StatusServiceUnavailable, types.ErrOptionWithSkipRetry())
	}
	applyChannelContext(c, channel, nil)
	common.SetContextKey(c, constant.ContextKeyChannelIsMultiKey, true)
	common.SetContextKey(c, constant.ContextKeyChannelMultiKeyIndex, keyIndex)
	common.SetContextKey(c, constant.ContextKeyChannelKey, keys[keyIndex])
	common.SetContextKey(c, constant.ContextKeySystemPromptOverride, false)
	return nil
}

func setupChannelAccountRoutingCandidate(c *gin.Context, channel *model.Channel, candidate *model.RoutingCandidate, modelName string) *types.NexusTokError {
	usingGroup := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	account, err := service.SelectSpecificChannelAccount(c, channel, modelName, usingGroup, candidate.ChannelAccountID, c.GetInt("relay_mode"))
	if err != nil {
		return types.NewErrorWithStatusCode(err, types.ErrorCodeChannelNoAvailableKey, http.StatusServiceUnavailable, types.ErrOptionWithSkipRetry())
	}
	applyChannelContext(c, channel, account)
	common.SetContextKey(c, constant.ContextKeyChannelIsMultiKey, false)
	common.SetContextKey(c, constant.ContextKeyChannelMultiKeyIndex, 0)
	common.SetContextKey(c, constant.ContextKeyChannelKey, account.Key)
	common.SetContextKey(c, constant.ContextKeyChannelAccountPool, true)
	common.SetContextKey(c, constant.ContextKeyChannelAccountId, account.Id)
	common.SetContextKey(c, constant.ContextKeyChannelAccountName, account.Name)
	common.SetContextKey(c, constant.ContextKeySystemPromptOverride, false)
	return nil
}

func setupPoolAccountRoutingCandidate(c *gin.Context, channel *model.Channel, candidate *model.RoutingCandidate, modelName string) *types.NexusTokError {
	usingGroup := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	group, account, err := service.SelectSpecificPoolAccount(c, channel, modelName, usingGroup, candidate.PoolGroupID, candidate.PoolAccountID, c.GetInt("relay_mode"))
	if err != nil {
		return types.NewErrorWithStatusCode(err, types.ErrorCodeChannelNoAvailableKey, http.StatusServiceUnavailable, types.ErrOptionWithSkipRetry())
	}
	channelKey, err := service.BuildPoolAccountChannelKey(account)
	if err != nil {
		service.ReleaseSelectedPoolAccount(c)
		return types.NewErrorWithStatusCode(err, types.ErrorCodeChannelNoAvailableKey, http.StatusServiceUnavailable, types.ErrOptionWithSkipRetry())
	}
	applyPoolAccountContext(c, channel, group, account)
	common.SetContextKey(c, constant.ContextKeyChannelIsMultiKey, false)
	common.SetContextKey(c, constant.ContextKeyChannelMultiKeyIndex, 0)
	common.SetContextKey(c, constant.ContextKeyChannelKey, channelKey)
	common.SetContextKey(c, constant.ContextKeyChannelAccountPool, true)
	common.SetContextKey(c, constant.ContextKeyPoolGroupId, group.Id)
	common.SetContextKey(c, constant.ContextKeyPoolGroupName, group.Name)
	common.SetContextKey(c, constant.ContextKeyPoolAccountId, account.Id)
	common.SetContextKey(c, constant.ContextKeyPoolAccountName, account.Name)
	common.SetContextKey(c, constant.ContextKeyPoolAccountAuthType, account.AuthType)
	common.SetContextKey(c, constant.ContextKeySystemPromptOverride, false)
	return nil
}
