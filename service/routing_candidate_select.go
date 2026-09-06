package service

import (
	"errors"
	"fmt"
	"math"
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
// 选择流程：先收集当前 group/model/path 下的结构性候选，再按前三层调度值找出最高层；
// 同层候选都具备真实转换倍率时，优先选择倍率最低的同步密钥。只要混入没有成本元
// 数据的手动渠道、Multi-Key 或历史账号，就回退既有权重策略，避免未知成本被错误当作
// 高成本而从候选集中剔除。健康度、策略域和同层 tie-break 只在最终候选集合内生效。
// 旧的 CacheGetRandomSatisfiedChannel 保留为兜底路径。
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
	usable, err := usableRoutingCandidatesInGroup(param, usingGroup)
	if err != nil || len(usable) == 0 {
		return nil, err
	}
	return selectRoutingCandidateByStrategy(usingGroup, param.ModelName, usable), nil
}

// SelectRoutingCandidateForChannel 只在指定渠道的剩余密钥级候选中选择。
//
// 该函数仅用于新鲜渠道亲和命中的首次尝试：亲和规则把首次候选限制在已绑定的
// channel_id 内，但仍复用统一候选的结构性过滤、动态过滤和成本排序。真实上游
// 失败后不再调用本函数，而是排除失败候选后回到 SelectRoutingCandidate 执行全渠道
// 选择，避免同渠道备用凭证优先于其他渠道的更低成本候选。
func SelectRoutingCandidateForChannel(param *RetryParam, channelID int) (*model.RoutingCandidate, string, error) {
	if param == nil {
		return nil, "", errors.New("retry param is nil")
	}
	if channelID <= 0 {
		return nil, param.TokenGroup, nil
	}
	usingGroup := param.TokenGroup
	if actualGroup := RoutingGroupFromContext(param.Ctx); actualGroup != "" && actualGroup != "auto" {
		usingGroup = actualGroup
	}
	if usingGroup == "" || usingGroup == "auto" {
		return nil, usingGroup, nil
	}
	usable, err := usableRoutingCandidatesInGroup(param, usingGroup)
	if err != nil || len(usable) == 0 {
		return nil, usingGroup, err
	}
	sameChannel := make([]*model.RoutingCandidate, 0, len(usable))
	for _, candidate := range usable {
		if candidate != nil && candidate.ChannelID == channelID {
			sameChannel = append(sameChannel, candidate)
		}
	}
	if len(sameChannel) == 0 {
		return nil, usingGroup, nil
	}
	return selectRoutingCandidateByStrategy(usingGroup, param.ModelName, sameChannel), usingGroup, nil
}

// SelectRoutingCandidateForAffinity 恢复并校验缓存绑定的具体候选。
// 亲和首次尝试不参与随机、成本或账号池策略重选：只有 CandidateKey 完全一致且当前
// 仍满足 group/model/path、启用状态与健康约束时才返回；否则调用方直接进入全渠道选择。
func SelectRoutingCandidateForAffinity(param *RetryParam, binding *model.RoutingCandidate) (*model.RoutingCandidate, string, string, error) {
	if param == nil {
		return nil, "", "invalid_binding", errors.New("retry param is nil")
	}
	if binding == nil || !binding.CandidateKey().Valid() {
		return nil, param.TokenGroup, "invalid_binding", nil
	}
	groups := []string{param.TokenGroup}
	if param.TokenGroup == "auto" {
		userGroup := common.GetContextKeyString(param.Ctx, constant.ContextKeyUserGroup)
		groups = GetUserAutoGroup(userGroup)
	}
	for _, usingGroup := range groups {
		if usingGroup == "" || usingGroup == "auto" {
			continue
		}
		candidate, reason, err := findAffinityRoutingCandidateInGroup(param, usingGroup, binding.CandidateKey())
		if err != nil {
			return nil, usingGroup, "candidate_lookup_error", err
		}
		if candidate == nil {
			if reason != "candidate_not_supported" {
				return nil, usingGroup, reason, nil
			}
			continue
		}
		if param.TokenGroup == "auto" {
			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroup, usingGroup)
		}
		return candidate, usingGroup, "", nil
	}
	return nil, param.TokenGroup, "candidate_not_supported", nil
}

func findAffinityRoutingCandidateInGroup(param *RetryParam, usingGroup string, bindingKey model.RoutingCandidateKey) (*model.RoutingCandidate, string, error) {
	if excluded := GetExcludedRoutingCandidateKeys(param.Ctx); excluded[bindingKey.String()] {
		return nil, "candidate_excluded", nil
	}
	candidates, err := model.GetRoutingCandidatesWithExclusions(usingGroup, param.ModelName, param.RequestPath, nil, nil)
	if err != nil {
		return nil, "candidate_lookup_error", err
	}
	var matched *model.RoutingCandidate
	for _, candidate := range candidates {
		if candidate != nil && candidate.CandidateKey() == bindingKey {
			matched = candidate
			break
		}
	}
	if matched == nil {
		return nil, "candidate_not_supported", nil
	}
	channel, err := model.CacheGetChannel(matched.ChannelID)
	if err != nil || channel == nil || channel.Status != common.ChannelStatusEnabled {
		return nil, "channel_disabled", nil
	}
	if !model.RoutingCandidateDynamicAllowed(matched, channel, usingGroup, param.ModelName) {
		return nil, "candidate_unavailable", nil
	}
	if !IsChannelRoutingHealthy(usingGroup, param.ModelName, matched.ChannelID) {
		return nil, "channel_health_cooldown", nil
	}
	if !IsRoutingCandidateTTFTHealthy(usingGroup, param.ModelName, matched) {
		return nil, "candidate_ttft_cooldown", nil
	}
	selected := matched.Clone()
	selected.SelectionReason = "channel_affinity"
	return selected, "", nil
}

func usableRoutingCandidatesInGroup(param *RetryParam, usingGroup string) ([]*model.RoutingCandidate, error) {
	excludedChannels := GetExcludedChannelIds(param.Ctx)
	excludedCandidates := GetExcludedRoutingCandidateKeys(param.Ctx)
	candidates, err := model.GetRoutingCandidatesWithExclusions(usingGroup, param.ModelName, param.RequestPath, excludedChannels, excludedCandidates)
	if err != nil || len(candidates) == 0 {
		return nil, err
	}
	usable := make([]*model.RoutingCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate == nil {
			continue
		}
		channel, channelErr := model.CacheGetChannel(candidate.ChannelID)
		if channelErr != nil || channel == nil || channel.Status != common.ChannelStatusEnabled {
			continue
		}
		if !model.RoutingCandidateDynamicAllowed(candidate, channel, usingGroup, param.ModelName) {
			continue
		}
		usable = append(usable, candidate)
	}
	if len(usable) == 0 {
		return nil, nil
	}
	return usable, nil
}

func selectRoutingCandidateByStrategy(usingGroup string, modelName string, candidates []*model.RoutingCandidate) *model.RoutingCandidate {
	// 首包冷却属于动态可用性过滤，不是静态成本或优先级层级。这里先排除已有
	// 明确慢首包样本的候选，保留旧行为：高优先级渠道持续慢时，健康渠道仍可接管；
	// 如果所有候选都处于冷却期，则回退到完整集合，避免观测异常导致无渠道可用。
	ttftHealthy := bestRoutingTTFTCandidates(usingGroup, modelName, candidates)
	if len(ttftHealthy) == 0 {
		ttftHealthy = candidates
	}

	layerCandidates := bestRoutingPriorityLayerCandidates(ttftHealthy)
	if len(layerCandidates) == 0 {
		return nil
	}
	costCandidates := bestRoutingConvertedRatioCandidates(layerCandidates)
	if len(costCandidates) == 0 {
		return nil
	}
	weightCandidates := bestRoutingCredentialWeightCandidates(costCandidates)
	if len(weightCandidates) == 0 {
		return nil
	}

	// 转换倍率相同或缺失时，密钥权重仍然是稳定的兼容排序层；经过静态排序
	// 后再应用渠道健康状态，避免同层中的失败渠道反复被选中。
	healthy := make([]*model.RoutingCandidate, 0, len(weightCandidates))
	for _, candidate := range weightCandidates {
		if candidate != nil && IsChannelRoutingHealthy(usingGroup, modelName, candidate.ChannelID) {
			healthy = append(healthy, candidate)
		}
	}
	if len(healthy) == 0 {
		healthy = leastDegradedRoutingCandidates(usingGroup, modelName, weightCandidates)
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
	if len(healthy) == 0 {
		return nil
	}

	domainWinners := pickRoutingPolicyDomainCandidates(usingGroup, modelName, healthy)
	selected := pickRoutingCandidateByLayerWeight(domainWinners)
	if selected == nil {
		return nil
	}
	if routingCandidateHasValidConvertedRatio(selected) {
		selected.SelectionReason = "lowest_converted_ratio"
	} else {
		selected.SelectionReason = "weight_fallback"
	}
	return selected
}

// bestRoutingTTFTCandidates 过滤处于首包冷却期的密钥级候选。
// 渠道账号和全局账号池按具体账号统计；普通单 Key、multi-key 候选使用渠道级统计。
func bestRoutingTTFTCandidates(group, modelName string, candidates []*model.RoutingCandidate) []*model.RoutingCandidate {
	result := make([]*model.RoutingCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate != nil && IsRoutingCandidateTTFTHealthy(group, modelName, candidate) {
			result = append(result, candidate)
		}
	}
	return result
}

func bestRoutingPriorityLayerCandidates(candidates []*model.RoutingCandidate) []*model.RoutingCandidate {
	var best *model.RoutingCandidate
	for _, candidate := range candidates {
		if candidate == nil {
			continue
		}
		if best == nil || candidate.Schedule.ComparePriorityLayer(best.Schedule) > 0 {
			best = candidate
		}
	}
	if best == nil {
		return nil
	}
	result := make([]*model.RoutingCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate != nil && candidate.Schedule.SamePriorityLayer(best.Schedule) {
			result = append(result, candidate)
		}
	}
	return result
}

func bestRoutingConvertedRatioCandidates(candidates []*model.RoutingCandidate) []*model.RoutingCandidate {
	if len(candidates) == 0 {
		return nil
	}
	minRatio := math.MaxFloat64
	for _, candidate := range candidates {
		if routingCandidateHasValidConvertedRatio(candidate) && candidate.ConvertedRatio < minRatio {
			minRatio = candidate.ConvertedRatio
		}
	}
	if minRatio == math.MaxFloat64 {
		// 整个优先级层都没有成本数据时才回退历史权重策略。只要同层存在已知倍率，
		// 缺失元数据的候选就不能反过来压过已知低成本候选。
		return candidates
	}
	const epsilon = 1e-12
	result := make([]*model.RoutingCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if routingCandidateHasValidConvertedRatio(candidate) && math.Abs(candidate.ConvertedRatio-minRatio) <= epsilon {
			result = append(result, candidate)
		}
	}
	return result
}

// bestRoutingCredentialWeightCandidates 在优先级与转换倍率都相同或缺失时，保留
// 密钥权重最高的候选。这样手动渠道、multi-key 和没有成本元数据的历史账号仍沿用
// 原有权重策略；同步账号的低转换倍率则已经在上一层优先胜出。
func bestRoutingCredentialWeightCandidates(candidates []*model.RoutingCandidate) []*model.RoutingCandidate {
	bestWeight := -1
	for _, candidate := range candidates {
		if candidate == nil {
			continue
		}
		if candidate.Schedule.CredentialWeight > bestWeight {
			bestWeight = candidate.Schedule.CredentialWeight
		}
	}
	if bestWeight < 0 {
		return nil
	}
	result := make([]*model.RoutingCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate != nil && candidate.Schedule.CredentialWeight == bestWeight {
			result = append(result, candidate)
		}
	}
	return result
}

func routingCandidateHasValidConvertedRatio(candidate *model.RoutingCandidate) bool {
	return candidate != nil &&
		candidate.HasConvertedRatio &&
		candidate.ConvertedRatio > 0 &&
		!math.IsNaN(candidate.ConvertedRatio) &&
		!math.IsInf(candidate.ConvertedRatio, 0)
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
		if candidate == nil {
			continue
		}
		if weight := routingCandidateScheduleWeight(candidate); weight > bestWeight {
			bestWeight = weight
		}
	}
	result := make([]*model.RoutingCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate != nil && routingCandidateScheduleWeight(candidate) == bestWeight {
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
		return pickRoutingCandidateByLayerWeight(candidates)
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

func pickRoutingCandidateByLayerWeight(candidates []*model.RoutingCandidate) *model.RoutingCandidate {
	if len(candidates) == 0 {
		return nil
	}
	if len(candidates) == 1 {
		return candidates[0]
	}
	sortRoutingCandidatesByStableKey(candidates)
	sumWeight := 0
	for _, candidate := range candidates {
		if candidate == nil {
			continue
		}
		if weight := routingCandidateScheduleWeight(candidate); weight > 0 {
			sumWeight += weight
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
		weight := routingCandidateScheduleWeight(candidate)
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

func routingCandidateScheduleWeight(candidate *model.RoutingCandidate) int {
	if candidate == nil {
		return 0
	}
	weight := candidate.Schedule.ChannelWeight + candidate.Schedule.CredentialWeight
	if weight < 0 {
		return 0
	}
	return weight
}

func sortRoutingCandidatesByStableKey(candidates []*model.RoutingCandidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].CandidateKey().String() < candidates[j].CandidateKey().String()
	})
}

// AppendRoutingCandidateAdminInfo 把统一调度命中的非敏感候选信息追加到管理日志。
// 这里刻意只记录类型、ID 和四级原始调度值，不记录 channel_key、账号凭据或脱敏摘要，
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
		"kind":                string(candidate.Kind),
		"candidate_id":        candidate.CandidateKey().String(),
		"channel_id":          candidate.ChannelID,
		"ability_priority":    candidate.Schedule.ChannelPriority,
		"ability_weight":      candidate.Schedule.ChannelWeight,
		"channel_priority":    candidate.Schedule.ChannelPriority,
		"channel_weight":      candidate.Schedule.ChannelWeight,
		"credential_priority": candidate.Schedule.CredentialPriority,
		"credential_weight":   candidate.Schedule.CredentialWeight,
		"cost_missing":        !routingCandidateHasValidConvertedRatio(candidate),
		"selection_reason":    candidate.SelectionReason,
		"policy_domain":       candidate.PolicyDomain,
	}
	if candidate.HasConvertedRatio {
		info["has_converted_ratio"] = true
		info["converted_ratio"] = candidate.ConvertedRatio
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
