package model

import (
	"fmt"
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/setting/ratio_setting"
)

// RoutingCredentialKind 表示统一路由候选实际使用的凭证来源。
// 候选缓存只记录来源类型和非敏感 ID，不记录明文 API Key 或 OAuth 凭据。
type RoutingCredentialKind string

const (
	RoutingCredentialKindSingleKey      RoutingCredentialKind = "single_key"
	RoutingCredentialKindMultiKey       RoutingCredentialKind = "multi_key"
	RoutingCredentialKindChannelAccount RoutingCredentialKind = "channel_account"
	RoutingCredentialKindPoolAccount    RoutingCredentialKind = "pool_account"
)

// RoutingCandidateKey 是请求级排除集使用的稳定候选标识。
// 同一渠道下不同 multi-key 索引、渠道账号或全局池账号会生成不同 key，
// 因此重试可以先替换具体凭证，避免过早把整个渠道降级。
type RoutingCandidateKey struct {
	ChannelID        int                   `json:"channel_id"`
	Kind             RoutingCredentialKind `json:"kind"`
	MultiKeyIndex    int                   `json:"multi_key_index,omitempty"`
	ChannelAccountID int                   `json:"channel_account_id,omitempty"`
	PoolGroupID      int                   `json:"pool_group_id,omitempty"`
	PoolAccountID    int                   `json:"pool_account_id,omitempty"`
}

// String 返回可安全写入 Gin context 的排除键；它不包含任何明文凭据。
func (key RoutingCandidateKey) String() string {
	return fmt.Sprintf("%d:%s:%d:%d:%d:%d", key.ChannelID, key.Kind, key.MultiKeyIndex, key.ChannelAccountID, key.PoolGroupID, key.PoolAccountID)
}

// RoutingSchedule 保存渠道与凭证的四级字典序调度值。
// 比较顺序固定为：channel_priority > channel_weight > credential_priority > credential_weight。
// 这里只保存原始字段，不再提前折叠成合成分数，避免低层候选借助别的变量反超高层候选。
type RoutingSchedule struct {
	ChannelPriority    int64 `json:"channel_priority"`
	ChannelWeight      int   `json:"channel_weight"`
	CredentialPriority int64 `json:"credential_priority"`
	CredentialWeight   int   `json:"credential_weight"`
}

// NewRoutingSchedule 构建候选的四级调度值。
func NewRoutingSchedule(channelPriority int64, channelWeight int, credentialPriority int64, credentialWeight int) RoutingSchedule {
	if channelWeight < 0 {
		channelWeight = 0
	}
	if credentialWeight < 0 {
		credentialWeight = 0
	}
	return RoutingSchedule{
		ChannelPriority:    channelPriority,
		ChannelWeight:      channelWeight,
		CredentialPriority: credentialPriority,
		CredentialWeight:   credentialWeight,
	}
}

// Compare 按四级字典序比较两个调度值。
//
// 返回值含义：
//   - 1：当前调度值更高
//   - 0：两个调度值完全相同
//   - -1：当前调度值更低
func (schedule RoutingSchedule) Compare(other RoutingSchedule) int {
	if schedule.ChannelPriority != other.ChannelPriority {
		if schedule.ChannelPriority > other.ChannelPriority {
			return 1
		}
		return -1
	}
	if schedule.ChannelWeight != other.ChannelWeight {
		if schedule.ChannelWeight > other.ChannelWeight {
			return 1
		}
		return -1
	}
	if schedule.CredentialPriority != other.CredentialPriority {
		if schedule.CredentialPriority > other.CredentialPriority {
			return 1
		}
		return -1
	}
	if schedule.CredentialWeight != other.CredentialWeight {
		if schedule.CredentialWeight > other.CredentialWeight {
			return 1
		}
		return -1
	}
	return 0
}

// SameLayer 判断两个调度值是否完全相同。
func (schedule RoutingSchedule) SameLayer(other RoutingSchedule) bool {
	return schedule.Compare(other) == 0
}

// RoutingCandidate 是统一调度层使用的非敏感候选元数据。
// Group/Model 是该候选被索引的能力域；PolicyDomain/Policy 用于在同一策略域内保留
// 原有 multi-key、渠道账号池或全局账号池的调度语义。
type RoutingCandidate struct {
	ChannelID        int                   `json:"channel_id"`
	Kind             RoutingCredentialKind `json:"kind"`
	MultiKeyIndex    int                   `json:"multi_key_index,omitempty"`
	ChannelAccountID int                   `json:"channel_account_id,omitempty"`
	PoolGroupID      int                   `json:"pool_group_id,omitempty"`
	PoolAccountID    int                   `json:"pool_account_id,omitempty"`
	Group            string                `json:"group"`
	Model            string                `json:"model"`
	PolicyDomain     string                `json:"policy_domain"`
	Policy           string                `json:"policy"`
	Schedule         RoutingSchedule       `json:"schedule"`
}

// CandidateKey 返回该候选的请求级排除标识。
func (candidate *RoutingCandidate) CandidateKey() RoutingCandidateKey {
	if candidate == nil {
		return RoutingCandidateKey{}
	}
	return RoutingCandidateKey{
		ChannelID:        candidate.ChannelID,
		Kind:             candidate.Kind,
		MultiKeyIndex:    candidate.MultiKeyIndex,
		ChannelAccountID: candidate.ChannelAccountID,
		PoolGroupID:      candidate.PoolGroupID,
		PoolAccountID:    candidate.PoolAccountID,
	}
}

// Clone 返回候选副本，避免调用方修改缓存中的元数据。
func (candidate *RoutingCandidate) Clone() *RoutingCandidate {
	if candidate == nil {
		return nil
	}
	clone := *candidate
	return &clone
}

var routingCandidateCache map[string]map[string][]*RoutingCandidate

// routingAbilitySchedule 保存指定 group/model/channel 能力记录的调度值。
//
// 账号池渠道的模型级 priority/weight 可能与 channels 表不同。统一候选必须使用
// Ability 的值，否则后台调整模型路由后，账号池候选仍会按旧的渠道级值选路。
type routingAbilitySchedule struct {
	Enabled  bool
	Priority int64
	Weight   int
}

// buildRoutingCandidateCache 构建非敏感密钥级候选缓存。
// 缓存只存 ID、策略域和调度值；明文 key、OAuth credential 等敏感字段仍在 setup 阶段按 ID 读取。
func buildRoutingCandidateCache(channels []*Channel) map[string]map[string][]*RoutingCandidate {
	accountsByChannel := loadEnabledChannelAccountsByChannel()
	poolGroups, poolAccountsByGroup := loadEnabledPoolRoutingSources()
	index := buildRoutingCandidateIndex(channels, accountsByChannel, poolGroups, poolAccountsByGroup)
	return applyRoutingAbilitySchedules(index, loadRoutingAbilitySchedules())
}

// loadRoutingAbilitySchedules 读取能力表中的模型级调度配置。
//
// 渠道缓存会在启动和热刷新时重建，因此每次构建都重新读取 Ability，确保后台
// 修改模型能力的 priority/weight 后，统一密钥路由能同步使用最新配置。
func loadRoutingAbilitySchedules() map[string]map[string]map[int]routingAbilitySchedule {
	schedules := make(map[string]map[string]map[int]routingAbilitySchedule)
	var abilities []*Ability
	if err := DB.Find(&abilities).Error; err != nil {
		return schedules
	}
	for _, ability := range abilities {
		if ability == nil || strings.TrimSpace(ability.Group) == "" ||
			strings.TrimSpace(ability.Model) == "" || ability.ChannelId <= 0 {
			continue
		}
		group := strings.TrimSpace(ability.Group)
		modelName := strings.TrimSpace(ability.Model)
		if _, ok := schedules[group]; !ok {
			schedules[group] = make(map[string]map[int]routingAbilitySchedule)
		}
		if _, ok := schedules[group][modelName]; !ok {
			schedules[group][modelName] = make(map[int]routingAbilitySchedule)
		}
		priority := int64(0)
		if ability.Priority != nil {
			priority = *ability.Priority
		}
		schedules[group][modelName][ability.ChannelId] = routingAbilitySchedule{
			Enabled:  ability.Enabled,
			Priority: priority,
			Weight:   int(ability.Weight),
		}
	}
	return schedules
}

// applyRoutingAbilitySchedules 将 Ability 的模型级调度覆盖到密钥级候选。
//
// 候选生成先按渠道/账号的结构构建，再在这里按候选自身的 group/model 精确匹配
// Ability。这样无需改变账号池、多 Key 的候选展开逻辑，同时能过滤明确禁用的
// Ability；没有对应 Ability 的历史候选继续回退到渠道级字段。
func applyRoutingAbilitySchedules(
	index map[string]map[string][]*RoutingCandidate,
	schedules map[string]map[string]map[int]routingAbilitySchedule,
) map[string]map[string][]*RoutingCandidate {
	if len(index) == 0 || len(schedules) == 0 {
		return index
	}
	for group, modelCandidates := range index {
		for modelName, candidates := range modelCandidates {
			scheduled := make([]*RoutingCandidate, 0, len(candidates))
			for _, candidate := range candidates {
				if candidate == nil {
					continue
				}
				groupSchedules := schedules[group]
				modelSchedules := groupSchedules[modelName]
				schedule, exists := modelSchedules[candidate.ChannelID]
				if !exists {
					// 精确候选可能来自通配模型/分组能力；没有精确 Ability 时
					// 保留原有渠道级调度，避免历史配置突然失去候选。
					scheduled = append(scheduled, candidate)
					continue
				}
				if !schedule.Enabled {
					continue
				}
				clone := candidate.Clone()
				clone.Schedule = NewRoutingSchedule(
					schedule.Priority,
					schedule.Weight,
					candidate.Schedule.CredentialPriority,
					candidate.Schedule.CredentialWeight,
				)
				scheduled = append(scheduled, clone)
			}
			modelCandidates[modelName] = scheduled
		}
	}
	return index
}

func loadEnabledChannelAccountsByChannel() map[int][]*ChannelAccount {
	if !DB.Migrator().HasTable(&ChannelAccount{}) {
		return map[int][]*ChannelAccount{}
	}
	var accounts []*ChannelAccount
	_ = DB.Where("status = ?", common.ChannelStatusEnabled).Order("priority DESC").Order("weight DESC").Order("id ASC").Find(&accounts).Error
	result := make(map[int][]*ChannelAccount)
	for _, account := range accounts {
		if account == nil || account.ChannelId <= 0 {
			continue
		}
		result[account.ChannelId] = append(result[account.ChannelId], account)
	}
	return result
}

func loadEnabledPoolRoutingSources() (map[int]*AccountPoolGroup, map[int][]*PoolAccount) {
	if !DB.Migrator().HasTable(&AccountPoolGroup{}) || !DB.Migrator().HasTable(&PoolAccount{}) {
		return map[int]*AccountPoolGroup{}, map[int][]*PoolAccount{}
	}
	var groups []*AccountPoolGroup
	_ = DB.Where("status = ?", common.ChannelStatusEnabled).Find(&groups).Error
	groupByID := make(map[int]*AccountPoolGroup, len(groups))
	for _, group := range groups {
		if group == nil || group.Id <= 0 || !IsNativeAccountPoolGroupSource(group.Source) {
			continue
		}
		groupByID[group.Id] = group
	}

	var accounts []*PoolAccount
	_ = DB.Where("status = ? AND schedulable = ?", common.ChannelStatusEnabled, true).Order("priority DESC").Order("weight DESC").Order("id ASC").Find(&accounts).Error
	accountsByGroup := make(map[int][]*PoolAccount)
	for _, account := range accounts {
		if account == nil || account.PoolGroupId <= 0 || !account.Schedulable {
			continue
		}
		if _, ok := groupByID[account.PoolGroupId]; !ok {
			continue
		}
		accountsByGroup[account.PoolGroupId] = append(accountsByGroup[account.PoolGroupId], account)
	}
	return groupByID, accountsByGroup
}

func buildRoutingCandidateIndex(
	channels []*Channel,
	accountsByChannel map[int][]*ChannelAccount,
	poolGroups map[int]*AccountPoolGroup,
	poolAccountsByGroup map[int][]*PoolAccount,
) map[string]map[string][]*RoutingCandidate {
	index := make(map[string]map[string][]*RoutingCandidate)
	for _, channel := range channels {
		if channel == nil || channel.Id <= 0 || channel.Status != common.ChannelStatusEnabled {
			continue
		}
		switch channel.GetCredentialMode() {
		case constant.ChannelCredentialModeMultiKey:
			addMultiKeyRoutingCandidates(index, channel)
		case constant.ChannelCredentialModeAccountPool:
			addChannelAccountRoutingCandidates(index, channel, accountsByChannel[channel.Id])
			if channel.ChannelInfo.AccountPoolFallback {
				addSingleKeyRoutingCandidates(index, channel)
			}
		case constant.ChannelCredentialModeGlobalAccountPool:
			addPoolAccountRoutingCandidates(index, channel, poolGroups[channel.ChannelInfo.AccountPoolGroupId], poolAccountsByGroup[channel.ChannelInfo.AccountPoolGroupId])
			if channel.ChannelInfo.AccountPoolFallback {
				addSingleKeyRoutingCandidates(index, channel)
			}
		default:
			addSingleKeyRoutingCandidates(index, channel)
		}
	}
	return index
}

func addSingleKeyRoutingCandidates(index map[string]map[string][]*RoutingCandidate, channel *Channel) {
	candidate := &RoutingCandidate{
		ChannelID:    channel.Id,
		Kind:         RoutingCredentialKindSingleKey,
		PolicyDomain: fmt.Sprintf("channel:%d:single_key", channel.Id),
		Policy:       string(RoutingCredentialKindSingleKey),
		Schedule:     NewRoutingSchedule(channel.GetPriority(), channel.GetWeight(), 0, 0),
	}
	addRoutingCandidateForValues(index, candidate, routingIndexValues(channel.GetGroups()), routingIndexValues(channel.GetModels()))
}

func addMultiKeyRoutingCandidates(index map[string]map[string][]*RoutingCandidate, channel *Channel) {
	keys := channel.GetKeys()
	if len(keys) == 0 {
		return
	}
	for keyIndex := range keys {
		if !routingMultiKeyIndexEnabled(channel, keyIndex) {
			continue
		}
		candidate := &RoutingCandidate{
			ChannelID:     channel.Id,
			Kind:          RoutingCredentialKindMultiKey,
			MultiKeyIndex: keyIndex,
			PolicyDomain:  fmt.Sprintf("channel:%d:multi_key", channel.Id),
			Policy:        strings.TrimSpace(string(channel.ChannelInfo.MultiKeyMode)),
			Schedule:      NewRoutingSchedule(channel.GetPriority(), channel.GetWeight(), 0, 0),
		}
		if candidate.Policy == "" {
			candidate.Policy = string(constant.MultiKeyModePolling)
		}
		addRoutingCandidateForValues(index, candidate, routingIndexValues(channel.GetGroups()), routingIndexValues(channel.GetModels()))
	}
}

func addChannelAccountRoutingCandidates(index map[string]map[string][]*RoutingCandidate, channel *Channel, accounts []*ChannelAccount) {
	for _, account := range accounts {
		if account == nil || account.ChannelId != channel.Id || account.Status != common.ChannelStatusEnabled {
			continue
		}
		groups := routingChannelAccountGroupsForIndex(account, channel)
		models := routingChannelAccountModelsForIndex(account, channel)
		if len(groups) == 0 || len(models) == 0 {
			continue
		}
		candidate := &RoutingCandidate{
			ChannelID:        channel.Id,
			Kind:             RoutingCredentialKindChannelAccount,
			ChannelAccountID: account.Id,
			PolicyDomain:     fmt.Sprintf("channel:%d:channel_account", channel.Id),
			Policy:           strings.TrimSpace(channel.GetAccountPoolMode()),
			Schedule:         NewRoutingSchedule(channel.GetPriority(), channel.GetWeight(), account.Priority, positiveRoutingWeight(account.Weight)),
		}
		addRoutingCandidateForValues(index, candidate, groups, models)
	}
}

func addPoolAccountRoutingCandidates(index map[string]map[string][]*RoutingCandidate, channel *Channel, group *AccountPoolGroup, accounts []*PoolAccount) {
	if group == nil || group.Id <= 0 || group.Status != common.ChannelStatusEnabled {
		return
	}
	for _, account := range accounts {
		if account == nil || account.PoolGroupId != group.Id || account.Status != common.ChannelStatusEnabled || !account.Schedulable {
			continue
		}
		groups := routingPoolAccountGroupsForIndex(account, group, channel)
		models := routingPoolAccountModelsForIndex(account, group, channel)
		if len(groups) == 0 || len(models) == 0 {
			continue
		}
		candidate := &RoutingCandidate{
			ChannelID:     channel.Id,
			Kind:          RoutingCredentialKindPoolAccount,
			PoolGroupID:   group.Id,
			PoolAccountID: account.Id,
			PolicyDomain:  fmt.Sprintf("channel:%d:pool_group:%d", channel.Id, group.Id),
			Policy:        strings.TrimSpace(group.Strategy),
			Schedule:      NewRoutingSchedule(channel.GetPriority(), channel.GetWeight(), account.Priority, positiveRoutingWeight(account.Weight)),
		}
		addRoutingCandidateForValues(index, candidate, groups, models)
	}
}

func routingMultiKeyIndexEnabled(channel *Channel, index int) bool {
	if channel == nil || index < 0 {
		return false
	}
	if channel.ChannelInfo.MultiKeyStatusList == nil {
		return true
	}
	status, ok := channel.ChannelInfo.MultiKeyStatusList[index]
	return !ok || status == common.ChannelStatusEnabled
}

func routingChannelAccountGroupsForIndex(account *ChannelAccount, channel *Channel) []string {
	if account == nil {
		return nil
	}
	if channel != nil && channel.HasUpstreamAccountSyncMetadata() {
		return SplitCommaValues(account.AccessGroups)
	}
	if values := SplitCommaValues(account.Group); len(values) > 0 {
		return values
	}
	if channel != nil {
		return routingIndexValues(channel.GetGroups())
	}
	return []string{"*"}
}

func routingChannelAccountModelsForIndex(account *ChannelAccount, channel *Channel) []string {
	if account == nil {
		return nil
	}
	if channel != nil && channel.HasUpstreamAccountSyncMetadata() {
		return SplitCommaValues(account.Models)
	}
	if values := SplitCommaValues(account.Models); len(values) > 0 {
		return values
	}
	if channel != nil {
		return routingIndexValues(channel.GetModels())
	}
	return []string{"*"}
}

func routingPoolAccountGroupsForIndex(account *PoolAccount, group *AccountPoolGroup, channel *Channel) []string {
	if account == nil {
		return nil
	}
	if values := SplitCommaValues(account.Group); len(values) > 0 {
		return values
	}
	if group != nil {
		if values := SplitCommaValues(group.Group); len(values) > 0 {
			return values
		}
	}
	if channel != nil {
		return routingIndexValues(channel.GetGroups())
	}
	return []string{"*"}
}

func routingPoolAccountModelsForIndex(account *PoolAccount, group *AccountPoolGroup, channel *Channel) []string {
	if account == nil {
		return nil
	}
	if values := SplitCommaValues(account.Models); len(values) > 0 {
		return values
	}
	if group != nil {
		if values := SplitCommaValues(group.Models); len(values) > 0 {
			return values
		}
	}
	if channel != nil {
		return routingIndexValues(channel.GetModels())
	}
	return []string{"*"}
}

func routingIndexValues(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			result = append(result, value)
		}
	}
	if len(result) == 0 {
		return []string{"*"}
	}
	return result
}

func positiveRoutingWeight(weight int) int {
	if weight > 0 {
		return weight
	}
	return 0
}

func addRoutingCandidateForValues(index map[string]map[string][]*RoutingCandidate, candidate *RoutingCandidate, groups []string, models []string) {
	for _, group := range routingIndexValues(groups) {
		for _, modelName := range routingIndexValues(models) {
			clone := candidate.Clone()
			clone.Group = group
			clone.Model = modelName
			addRoutingCandidate(index, clone)
		}
	}
}

func addRoutingCandidate(index map[string]map[string][]*RoutingCandidate, candidate *RoutingCandidate) {
	if candidate == nil || candidate.ChannelID <= 0 {
		return
	}
	group := strings.TrimSpace(candidate.Group)
	modelName := strings.TrimSpace(candidate.Model)
	if group == "" {
		group = "*"
	}
	if modelName == "" {
		modelName = "*"
	}
	if _, ok := index[group]; !ok {
		index[group] = make(map[string][]*RoutingCandidate)
	}
	index[group][modelName] = append(index[group][modelName], candidate)
}

// GetRoutingCandidatesWithExclusions 获取当前请求可参与统一调度的密钥级候选。
// 内存缓存开启时优先使用 InitChannelCache 同步构建的非敏感候选索引；未开启缓存时，
// 从数据库按相同规则临时构建，作为兼容兜底路径。
func GetRoutingCandidatesWithExclusions(group string, modelName string, requestPath string, excludedChannelIds []int, excludedCandidateKeys map[string]bool) ([]*RoutingCandidate, error) {
	excludedChannels := intSliceToSet(excludedChannelIds)
	if common.MemoryCacheEnabled {
		channelSyncLock.RLock()
		if routingCandidateCache != nil {
			candidates := routingCandidatesFromIndexLocked(group, modelName)
			filtered := filterRoutingCandidatesLocked(candidates, group, modelName, requestPath, excludedChannels, excludedCandidateKeys)
			channelSyncLock.RUnlock()
			return filtered, nil
		}
		channelSyncLock.RUnlock()
	}

	var channels []*Channel
	if err := DB.Where("status = ?", common.ChannelStatusEnabled).Find(&channels).Error; err != nil {
		return nil, err
	}
	index := buildRoutingCandidateCache(channels)
	candidates := routingCandidatesFromIndex(index, group, modelName)
	return filterRoutingCandidatesFromDB(candidates, group, modelName, requestPath, excludedChannels, excludedCandidateKeys), nil
}

func routingCandidatesFromIndexLocked(group string, modelName string) []*RoutingCandidate {
	return routingCandidatesFromIndex(routingCandidateCache, group, modelName)
}

func routingCandidatesFromIndex(index map[string]map[string][]*RoutingCandidate, group string, modelName string) []*RoutingCandidate {
	if len(index) == 0 {
		return nil
	}
	group = strings.TrimSpace(group)
	if group == "" {
		group = "default"
	}
	modelName = strings.TrimSpace(modelName)
	normalizedModel := ratio_setting.FormatMatchingModelName(modelName)
	models := []string{modelName}
	if normalizedModel != "" && normalizedModel != modelName {
		models = append(models, normalizedModel)
	}
	models = append(models, "*")
	groups := []string{group, "*"}

	seen := map[string]struct{}{}
	result := make([]*RoutingCandidate, 0)
	for _, groupName := range groups {
		modelIndex := index[groupName]
		if len(modelIndex) == 0 {
			continue
		}
		for _, candidateModel := range models {
			for _, candidate := range modelIndex[candidateModel] {
				if candidate == nil {
					continue
				}
				// 同一个凭证可能因为精确模型、规范化模型或通配配置同时命中多个索引。
				// 选路时必须按真实候选去重，否则会把同一 key/account 的权重重复计算。
				key := candidate.CandidateKey().String()
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				result = append(result, candidate)
			}
		}
	}
	return result
}

func filterRoutingCandidatesLocked(candidates []*RoutingCandidate, group string, modelName string, requestPath string, excludedChannels map[int]struct{}, excludedCandidateKeys map[string]bool) []*RoutingCandidate {
	filtered := make([]*RoutingCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if !routingCandidateBaseAllowed(candidate, group, modelName, excludedChannels, excludedCandidateKeys) {
			continue
		}
		channel := channelsIDM[candidate.ChannelID]
		if channel == nil || channel.Status != common.ChannelStatusEnabled {
			continue
		}
		if !routingCandidateRequestPathAllowed(channel, requestPath) {
			continue
		}
		filtered = append(filtered, candidate.Clone())
	}
	return filtered
}

func filterRoutingCandidatesFromDB(candidates []*RoutingCandidate, group string, modelName string, requestPath string, excludedChannels map[int]struct{}, excludedCandidateKeys map[string]bool) []*RoutingCandidate {
	filtered := make([]*RoutingCandidate, 0, len(candidates))
	channelByID := map[int]*Channel{}
	for _, candidate := range candidates {
		if candidate == nil || candidate.ChannelID <= 0 {
			continue
		}
		if _, ok := channelByID[candidate.ChannelID]; ok {
			continue
		}
		if channel, err := GetChannelById(candidate.ChannelID, true); err == nil {
			channelByID[candidate.ChannelID] = channel
		}
	}
	for _, candidate := range candidates {
		if !routingCandidateBaseAllowed(candidate, group, modelName, excludedChannels, excludedCandidateKeys) {
			continue
		}
		channel := channelByID[candidate.ChannelID]
		if channel == nil || channel.Status != common.ChannelStatusEnabled {
			continue
		}
		if !routingCandidateRequestPathAllowed(channel, requestPath) {
			continue
		}
		filtered = append(filtered, candidate.Clone())
	}
	return filtered
}

func routingCandidateBaseAllowed(candidate *RoutingCandidate, group string, modelName string, excludedChannels map[int]struct{}, excludedCandidateKeys map[string]bool) bool {
	if candidate == nil || candidate.ChannelID <= 0 {
		return false
	}
	if _, excluded := excludedChannels[candidate.ChannelID]; excluded {
		return false
	}
	if excludedCandidateKeys != nil && excludedCandidateKeys[candidate.CandidateKey().String()] {
		return false
	}
	if candidate.Group != "*" && strings.TrimSpace(group) != "" && candidate.Group != group {
		return false
	}
	if candidate.Model != "*" && !MatchesModelList([]string{candidate.Model}, modelName) {
		return false
	}
	return true
}

func routingCandidateRequestPathAllowed(channel *Channel, requestPath string) bool {
	if channel == nil {
		return false
	}
	if requestPath == "" || channel.Type != constant.ChannelTypeAdvancedCustom {
		return true
	}
	if common.MemoryCacheEnabled {
		if config := channel2advancedCustomConfig[channel.Id]; config != nil {
			return config.SupportsPath(requestPath)
		}
		return false
	}
	config := channel.GetOtherSettings().AdvancedCustom
	return config != nil && config.SupportsPath(requestPath)
}

// RoutingCandidateDynamicAllowed 根据候选指向的凭证执行结构性过滤。
//
// 这里只保留不会改变四级字典序层级的条件：渠道/账号状态、凭证模式、模型白名单和
// 分组白名单。冷却、并发、额度和其他运行态限制留到具体候选被选中后再处理，避免
// 低层候选因为运行态状态提前抢位。
func RoutingCandidateDynamicAllowed(candidate *RoutingCandidate, channel *Channel, usingGroup string, modelName string) bool {
	if candidate == nil || channel == nil {
		return false
	}
	switch candidate.Kind {
	case RoutingCredentialKindSingleKey:
		mode := channel.GetCredentialMode()
		if mode == constant.ChannelCredentialModeSingleKey {
			return true
		}
		return channel.ChannelInfo.AccountPoolFallback && (mode == constant.ChannelCredentialModeAccountPool || mode == constant.ChannelCredentialModeGlobalAccountPool)
	case RoutingCredentialKindMultiKey:
		return channel.GetCredentialMode() == constant.ChannelCredentialModeMultiKey && routingMultiKeyIndexEnabled(channel, candidate.MultiKeyIndex)
	case RoutingCredentialKindChannelAccount:
		account, err := GetChannelAccountById(channel.Id, candidate.ChannelAccountID)
		if err != nil || account == nil {
			return false
		}
		return account.Status == common.ChannelStatusEnabled && RoutingChannelAccountSupportsModel(account, channel, modelName) && RoutingChannelAccountSupportsGroup(account, channel, usingGroup)
	case RoutingCredentialKindPoolAccount:
		group, err := GetAccountPoolGroupById(candidate.PoolGroupID)
		if err != nil || group == nil || group.Status != common.ChannelStatusEnabled || !IsNativeAccountPoolGroupSource(group.Source) {
			return false
		}
		account, err := GetPoolAccountById(candidate.PoolAccountID)
		if err != nil || account == nil {
			return false
		}
		return account.Status == common.ChannelStatusEnabled && account.Schedulable && RoutingPoolAccountSupportsModel(account, group, modelName) && RoutingPoolAccountSupportsGroup(account, group, usingGroup)
	default:
		return false
	}
}
