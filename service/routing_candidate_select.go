package service

import (
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/logger"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/setting"
	"github.com/gin-gonic/gin"
)

// GetExcludedRoutingCandidateKeys 获取当前请求已经失败的密钥级候选集合。
func GetExcludedRoutingCandidateKeys(c *gin.Context) map[string]bool {
	if c == nil {
		return map[string]bool{}
	}
	if value, ok := common.GetContextKey(c, constant.ContextKeyRoutingCandidateExcludedKeys); ok {
		if excluded, ok := value.(map[string]bool); ok {
			copySet := make(map[string]bool, len(excluded))
			for key, value := range excluded {
				copySet[key] = value
			}
			return copySet
		}
	}
	return map[string]bool{}
}

// AddExcludedRoutingCandidate 将本次 setup 或上游失败的具体候选加入请求级排除集。
// 候选排除比渠道排除更细，允许同渠道内其它 key/account 继续参与下一轮选择。
func AddExcludedRoutingCandidate(c *gin.Context, candidate *model.RoutingCandidate) {
	if c == nil || candidate == nil {
		return
	}
	excluded := GetExcludedRoutingCandidateKeys(c)
	excluded[candidate.CandidateKey().String()] = true
	common.SetContextKey(c, constant.ContextKeyRoutingCandidateExcludedKeys, excluded)
}

// SelectRoutingCandidate 执行统一密钥级调度。
//
// 选择流程：收集当前 group/model/path 下的所有非敏感候选；过滤请求级排除、凭证动态状态
// 和渠道临时健康；在最高 effective priority 层内按策略域选出代表候选，再跨域按
// effective weight 加权选择。旧的 CacheGetRandomSatisfiedChannel 保留为兜底路径。
func SelectRoutingCandidate(param *RetryParam) (*model.RoutingCandidate, string, error) {
	if param == nil {
		return nil, "", errors.New("retry param is nil")
	}
	selectGroup := param.TokenGroup
	userGroup := common.GetContextKeyString(param.Ctx, constant.ContextKeyUserGroup)

	if param.TokenGroup == "auto" {
		if len(setting.GetAutoGroups()) == 0 {
			return nil, selectGroup, errors.New("auto groups is not enabled")
		}
		autoGroups := GetUserAutoGroup(userGroup)
		startGroupIndex := 0
		crossGroupRetry := common.GetContextKeyBool(param.Ctx, constant.ContextKeyTokenCrossGroupRetry)
		if lastGroupIndex, exists := common.GetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex); exists {
			if idx, ok := lastGroupIndex.(int); ok && idx >= 0 {
				startGroupIndex = idx
			}
		}
		for i := startGroupIndex; i < len(autoGroups); i++ {
			autoGroup := autoGroups[i]
			logger.LogDebug(param.Ctx, "Auto selecting routing candidate group: %s", autoGroup)
			candidate, err := selectRoutingCandidateInGroup(param, autoGroup)
			if err != nil {
				return nil, autoGroup, err
			}
			if candidate == nil {
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupRetryIndex, 0)
				param.SetRetry(0)
				continue
			}
			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroup, autoGroup)
			selectGroup = autoGroup
			if crossGroupRetry && param.GetRetry() >= common.RetryTimes {
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
				param.SetRetry(0)
				param.ResetRetryNextTry()
			} else {
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i)
			}
			return candidate, selectGroup, nil
		}
		return nil, selectGroup, nil
	}

	candidate, err := selectRoutingCandidateInGroup(param, param.TokenGroup)
	return candidate, param.TokenGroup, err
}

func selectRoutingCandidateInGroup(param *RetryParam, usingGroup string) (*model.RoutingCandidate, error) {
	excludedChannels := GetExcludedChannelIds(param.Ctx)
	excludedCandidates := GetExcludedRoutingCandidateKeys(param.Ctx)
	candidates, err := model.GetRoutingCandidatesWithExclusions(usingGroup, param.ModelName, param.RequestPath, excludedChannels, excludedCandidates)
	if err != nil || len(candidates) == 0 {
		return nil, err
	}
	usable := make([]*model.RoutingCandidate, 0, len(candidates))
	seenChannelCandidates := make(map[int]struct{})
	seenUsableChannels := make(map[int]struct{})
	now := common.GetTimestamp()
	for _, candidate := range candidates {
		if candidate == nil {
			continue
		}
		if candidate != nil && candidate.ChannelID > 0 {
			seenChannelCandidates[candidate.ChannelID] = struct{}{}
		}
		channel, channelErr := model.CacheGetChannel(candidate.ChannelID)
		if channelErr != nil || channel == nil || channel.Status != common.ChannelStatusEnabled {
			continue
		}
		if !model.RoutingCandidateDynamicAllowed(candidate, channel, usingGroup, param.ModelName, now) {
			continue
		}
		usable = append(usable, candidate)
		seenUsableChannels[candidate.ChannelID] = struct{}{}
	}
	for channelID := range seenChannelCandidates {
		if _, ok := seenUsableChannels[channelID]; ok {
			continue
		}
		AddExcludedChannelId(param.Ctx, channelID)
	}
	if len(usable) == 0 {
		return nil, nil
	}
	return selectRoutingCandidateByStrategy(usingGroup, param.ModelName, usable), nil
}

func selectRoutingCandidateByStrategy(usingGroup string, modelName string, candidates []*model.RoutingCandidate) *model.RoutingCandidate {
	priorityLayer := bestRoutingPriorityCandidates(candidates)
	if len(priorityLayer) == 0 {
		return nil
	}
	healthy := make([]*model.RoutingCandidate, 0, len(priorityLayer))
	for _, candidate := range priorityLayer {
		if candidate != nil && IsChannelRoutingHealthy(usingGroup, modelName, candidate.ChannelID) {
			healthy = append(healthy, candidate)
		}
	}
	if len(healthy) == 0 {
		healthy = leastDegradedRoutingCandidates(usingGroup, modelName, priorityLayer)
	}
	if len(healthy) == 0 {
		return nil
	}

	switch GetRoutingStrategyMode() {
	case "availability":
		healthy = bestRoutingAvailabilityCandidates(usingGroup, modelName, healthy)
	case "cost":
		healthy = bestRoutingWeightCandidates(healthy)
	}

	domainWinners := pickRoutingPolicyDomainCandidates(usingGroup, modelName, healthy)
	return pickRoutingCandidateByEffectiveWeight(domainWinners)
}

func bestRoutingPriorityCandidates(candidates []*model.RoutingCandidate) []*model.RoutingCandidate {
	bestPriority := int64(-1 << 63)
	for _, candidate := range candidates {
		if candidate != nil && candidate.Schedule.EffectivePriority > bestPriority {
			bestPriority = candidate.Schedule.EffectivePriority
		}
	}
	result := make([]*model.RoutingCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate != nil && candidate.Schedule.EffectivePriority == bestPriority {
			result = append(result, candidate)
		}
	}
	return result
}

func leastDegradedRoutingCandidates(group, modelName string, candidates []*model.RoutingCandidate) []*model.RoutingCandidate {
	bestFailureCount := int(^uint(0) >> 1)
	bestCooldown := int64(^uint64(0) >> 1)
	result := make([]*model.RoutingCandidate, 0)
	for _, candidate := range candidates {
		if candidate == nil {
			continue
		}
		state := getChannelRoutingHealthState(group, modelName, candidate.ChannelID)
		if state.FailureCount < bestFailureCount || (state.FailureCount == bestFailureCount && state.CooldownUntil < bestCooldown) {
			bestFailureCount = state.FailureCount
			bestCooldown = state.CooldownUntil
			result = []*model.RoutingCandidate{candidate}
		} else if state.FailureCount == bestFailureCount && state.CooldownUntil == bestCooldown {
			result = append(result, candidate)
		}
	}
	return result
}

func bestRoutingAvailabilityCandidates(group, modelName string, candidates []*model.RoutingCandidate) []*model.RoutingCandidate {
	minFailureCount := int(^uint(0) >> 1)
	for _, candidate := range candidates {
		state := getChannelRoutingHealthState(group, modelName, candidate.ChannelID)
		if state.FailureCount < minFailureCount {
			minFailureCount = state.FailureCount
		}
	}
	result := make([]*model.RoutingCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		state := getChannelRoutingHealthState(group, modelName, candidate.ChannelID)
		if state.FailureCount == minFailureCount {
			result = append(result, candidate)
		}
	}
	return result
}

func bestRoutingWeightCandidates(candidates []*model.RoutingCandidate) []*model.RoutingCandidate {
	bestWeight := -1
	for _, candidate := range candidates {
		if candidate != nil && candidate.Schedule.EffectiveWeight > bestWeight {
			bestWeight = candidate.Schedule.EffectiveWeight
		}
	}
	result := make([]*model.RoutingCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate != nil && candidate.Schedule.EffectiveWeight == bestWeight {
			result = append(result, candidate)
		}
	}
	return result
}

func pickRoutingPolicyDomainCandidates(usingGroup string, modelName string, candidates []*model.RoutingCandidate) []*model.RoutingCandidate {
	domains := map[string][]*model.RoutingCandidate{}
	for _, candidate := range candidates {
		if candidate == nil {
			continue
		}
		domain := strings.TrimSpace(candidate.PolicyDomain)
		if domain == "" {
			domain = candidate.CandidateKey().String()
		}
		domains[domain] = append(domains[domain], candidate)
	}
	domainNames := make([]string, 0, len(domains))
	for domain := range domains {
		domainNames = append(domainNames, domain)
	}
	sort.Strings(domainNames)
	selected := make([]*model.RoutingCandidate, 0, len(domainNames))
	for _, domain := range domainNames {
		winner := pickRoutingCandidateInPolicyDomain(usingGroup, modelName, domains[domain])
		if winner != nil {
			selected = append(selected, winner)
		}
	}
	return selected
}

func pickRoutingCandidateInPolicyDomain(usingGroup string, modelName string, candidates []*model.RoutingCandidate) *model.RoutingCandidate {
	if len(candidates) == 0 {
		return nil
	}
	if len(candidates) == 1 {
		return candidates[0]
	}
	sortRoutingCandidatesByStableKey(candidates)
	switch candidates[0].Kind {
	case model.RoutingCredentialKindMultiKey:
		return pickMultiKeyRoutingCandidate(candidates)
	case model.RoutingCredentialKindChannelAccount:
		return pickChannelAccountRoutingCandidate(usingGroup, modelName, candidates)
	case model.RoutingCredentialKindPoolAccount:
		return pickPoolAccountRoutingCandidate(usingGroup, modelName, candidates)
	default:
		return pickRoutingCandidateByEffectiveWeight(candidates)
	}
}

func pickMultiKeyRoutingCandidate(candidates []*model.RoutingCandidate) *model.RoutingCandidate {
	channel, err := model.CacheGetChannel(candidates[0].ChannelID)
	if err != nil || channel == nil {
		return candidates[0]
	}
	if channel.ChannelInfo.MultiKeyMode == constant.MultiKeyModeRandom {
		return candidates[rand.Intn(len(candidates))]
	}
	keys := channel.GetKeys()
	if len(keys) == 0 {
		return candidates[0]
	}
	lock := model.GetChannelPollingLock(channel.Id)
	lock.Lock()
	defer lock.Unlock()
	start := channel.ChannelInfo.MultiKeyPollingIndex
	if info, infoErr := model.CacheGetChannelInfo(channel.Id); infoErr == nil && info != nil {
		start = info.MultiKeyPollingIndex
	}
	if start < 0 || start >= len(keys) {
		start = 0
	}
	byIndex := make(map[int]*model.RoutingCandidate, len(candidates))
	for _, candidate := range candidates {
		byIndex[candidate.MultiKeyIndex] = candidate
	}
	for offset := 0; offset < len(keys); offset++ {
		idx := (start + offset) % len(keys)
		if candidate := byIndex[idx]; candidate != nil {
			channel.ChannelInfo.MultiKeyPollingIndex = (idx + 1) % len(keys)
			if !common.MemoryCacheEnabled {
				_ = channel.SaveChannelInfo()
			}
			return candidate
		}
	}
	return candidates[0]
}

func pickChannelAccountRoutingCandidate(usingGroup string, modelName string, candidates []*model.RoutingCandidate) *model.RoutingCandidate {
	channel, err := model.CacheGetChannel(candidates[0].ChannelID)
	if err != nil || channel == nil {
		return candidates[0]
	}
	accounts := make([]*model.ChannelAccount, 0, len(candidates))
	candidateByAccount := make(map[int]*model.RoutingCandidate, len(candidates))
	for _, candidate := range candidates {
		account, accountErr := model.GetChannelAccountById(channel.Id, candidate.ChannelAccountID)
		if accountErr != nil || account == nil {
			continue
		}
		accounts = append(accounts, account)
		candidateByAccount[account.Id] = candidate
	}
	if len(accounts) == 0 {
		return candidates[0]
	}
	if channel.HasUpstreamAccountSyncMetadata() {
		sort.SliceStable(accounts, func(i, j int) bool {
			if accounts[i].Priority != accounts[j].Priority {
				return accounts[i].Priority > accounts[j].Priority
			}
			if accounts[i].GetWeight() != accounts[j].GetWeight() {
				return accounts[i].GetWeight() > accounts[j].GetWeight()
			}
			return accounts[i].Id < accounts[j].Id
		})
		return candidateByAccount[accounts[0].Id]
	}
	picked := pickChannelAccount(channel, accounts, usingGroup, modelName)
	if picked == nil {
		return candidates[0]
	}
	return candidateByAccount[picked.Id]
}

func pickPoolAccountRoutingCandidate(usingGroup string, modelName string, candidates []*model.RoutingCandidate) *model.RoutingCandidate {
	group, err := model.GetAccountPoolGroupById(candidates[0].PoolGroupID)
	if err != nil || group == nil {
		return candidates[0]
	}
	accounts := make([]*model.PoolAccount, 0, len(candidates))
	candidateByAccount := make(map[int]*model.RoutingCandidate, len(candidates))
	for _, candidate := range candidates {
		account, accountErr := model.GetPoolAccountById(candidate.PoolAccountID)
		if accountErr != nil || account == nil {
			continue
		}
		accounts = append(accounts, account)
		candidateByAccount[account.Id] = candidate
	}
	if len(accounts) == 0 {
		return candidates[0]
	}
	sortPoolAccountCandidates(accounts, group.Strategy)
	picked := pickPoolAccount(group, accounts, usingGroup, modelName)
	if picked == nil {
		return candidates[0]
	}
	return candidateByAccount[picked.Id]
}

func pickRoutingCandidateByEffectiveWeight(candidates []*model.RoutingCandidate) *model.RoutingCandidate {
	if len(candidates) == 0 {
		return nil
	}
	if len(candidates) == 1 {
		return candidates[0]
	}
	sortRoutingCandidatesByStableKey(candidates)
	sumWeight := 0
	for _, candidate := range candidates {
		if candidate != nil && candidate.Schedule.EffectiveWeight > 0 {
			sumWeight += candidate.Schedule.EffectiveWeight
		}
	}
	if sumWeight <= 0 {
		return candidates[rand.Intn(len(candidates))]
	}
	smoothingFactor := 1
	if sumWeight/len(candidates) < 10 {
		smoothingFactor = 100
	}
	randomWeight := rand.Intn(sumWeight * smoothingFactor)
	for _, candidate := range candidates {
		weight := candidate.Schedule.EffectiveWeight
		if weight <= 0 {
			continue
		}
		randomWeight -= weight * smoothingFactor
		if randomWeight < 0 {
			return candidate
		}
	}
	return candidates[len(candidates)-1]
}

func sortRoutingCandidatesByStableKey(candidates []*model.RoutingCandidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].CandidateKey().String() < candidates[j].CandidateKey().String()
	})
}

// AppendRoutingCandidateAdminInfo 把统一调度命中的非敏感候选信息追加到管理日志。
// 这里刻意只记录类型、ID 和合成调度值，不记录 channel_key、账号凭据或脱敏摘要，
// 既方便排查调度结果，也避免日志扩大敏感信息暴露面。
func AppendRoutingCandidateAdminInfo(c *gin.Context, adminInfo map[string]interface{}) {
	if c == nil || adminInfo == nil {
		return
	}
	value, ok := common.GetContextKey(c, constant.ContextKeyRoutingCandidate)
	if !ok || value == nil {
		return
	}
	var candidate *model.RoutingCandidate
	switch typed := value.(type) {
	case *model.RoutingCandidate:
		candidate = typed
	case model.RoutingCandidate:
		candidate = &typed
	default:
		return
	}
	if candidate == nil || candidate.ChannelID <= 0 {
		return
	}
	info := map[string]interface{}{
		"kind":               string(candidate.Kind),
		"channel_id":         candidate.ChannelID,
		"effective_priority": candidate.Schedule.EffectivePriority,
		"effective_weight":   candidate.Schedule.EffectiveWeight,
		"policy_domain":      candidate.PolicyDomain,
	}
	if candidate.MultiKeyIndex > 0 || candidate.Kind == model.RoutingCredentialKindMultiKey {
		info["multi_key_index"] = candidate.MultiKeyIndex
	}
	if candidate.ChannelAccountID > 0 {
		info["channel_account_id"] = candidate.ChannelAccountID
	}
	if candidate.PoolGroupID > 0 {
		info["pool_group_id"] = candidate.PoolGroupID
	}
	if candidate.PoolAccountID > 0 {
		info["pool_account_id"] = candidate.PoolAccountID
	}
	adminInfo["routing_candidate"] = info
}

func routingCandidateSelectionError(selectGroup string, modelName string, err error) error {
	if err != nil {
		return fmt.Errorf("获取分组 %s 下模型 %s 的统一候选失败：%w", selectGroup, modelName, err)
	}
	return fmt.Errorf("分组 %s 下模型 %s 的统一候选不存在", selectGroup, modelName)
}
