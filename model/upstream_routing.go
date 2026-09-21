package model

import (
	"math/rand"
	"strings"
	"time"

	"github.com/c1cadaBob/NexusTok/common"
	"github.com/c1cadaBob/NexusTok/logger"
	"github.com/c1cadaBob/NexusTok/setting/ratio_setting"
	"github.com/c1cadaBob/NexusTok/setting/reasoning"
)

type routingKeyRouteCandidate struct {
	channel *Channel
	key     *RoutingKeySelection
}

func buildKeyChannelRoutingKey(channel *Channel) *UpstreamKey {
	key := &UpstreamKey{}
	if channel == nil {
		return key
	}
	key.KeyPriority = channel.KeyPriority
	key.ConversionRatio = channel.ConversionRatio
	if channel.KeyWeightOverride != nil && channel.ConversionRatio != 0 {
		override := max(MinUpstreamKeyWeight, min(MaxUpstreamKeyWeight, *channel.KeyWeightOverride))
		key.WeightOverride = &override
	}
	return key
}

func channelKeySelection(channel *Channel, key *ChannelKey) *RoutingKeySelection {
	if channel == nil || key == nil {
		return nil
	}
	priority := normalizeRoutingKeyPriority(key.KeyPriority)
	return &RoutingKeySelection{
		KeyID:             key.RoutingKeyID,
		ChannelID:         channel.Id,
		Source:            RoutingKeySourceKeyChannel,
		SourceRefID:       key.ID,
		Secret:            key.Secret,
		Name:              MaskTokenKey(key.Secret),
		KeyPriority:       priority,
		EffectivePriority: channel.GetPriority()*100 + priority,
		Weight:            key.EffectiveWeight(),
		AutoWeight:        key.AutoWeight(),
		ConversionRatio:   key.ConversionRatio,
		ChannelKeyID:      key.ID,
		MultiKeyIndex:     key.KeyIndex,
	}
}

func upstreamKeySelection(channel *Channel, key *UpstreamKey) *RoutingKeySelection {
	if channel == nil || key == nil {
		return nil
	}
	priority := normalizeRoutingKeyPriority(key.KeyPriority)
	return &RoutingKeySelection{
		KeyID:             key.RoutingKeyID,
		ChannelID:         channel.Id,
		Source:            RoutingKeySourcePlatformSite,
		SourceRefID:       key.ID,
		Secret:            key.Secret,
		Name:              key.Name,
		KeyPriority:       priority,
		EffectivePriority: channel.GetPriority()*100 + priority,
		Weight:            key.EffectiveWeight(),
		AutoWeight:        key.AutoWeight(),
		ConversionRatio:   key.ConversionRatio,
		UpstreamKeyID:     key.ID,
	}
}

func loadRoutableChannelKeys(channel *Channel, group, modelName string) []*RoutingKeySelection {
	if channel == nil || channel.Status != common.ChannelStatusEnabled ||
		channel.UpstreamKind == UpstreamKindPlatformSite {
		return nil
	}
	if strings.TrimSpace(group) != "" && !channelSupportsGroup(channel, group) {
		return nil
	}
	if strings.TrimSpace(modelName) != "" && !channelSupportsModel(channel, modelName) {
		return nil
	}
	keys, err := LoadChannelKeys(nil, channel.Id, false)
	if err != nil {
		logger.LogWarn(nil, "load channel keys failed: channel_id=%d error=%v", channel.Id, err)
		return nil
	}
	if len(keys) == 0 && strings.TrimSpace(channel.Key) != "" {
		if err := SyncChannelKeysFromLegacyField(nil, channel); err == nil {
			keys, _ = LoadChannelKeys(nil, channel.Id, false)
		}
	}
	result := make([]*RoutingKeySelection, 0, len(keys))
	for index := range keys {
		key := &keys[index]
		if key.Status != common.ChannelStatusEnabled || key.RoutingKeyID == 0 {
			continue
		}
		if err := key.LoadSecret(); err != nil {
			continue
		}
		if !RoutingKeyHealthAllowsRouting(key.RoutingKeyID, key.Status, UpstreamAvailabilityRoutable, time.Now()) {
			continue
		}
		result = append(result, channelKeySelection(channel, key))
	}
	return result
}

func routingCandidatesForChannels(channels []*Channel, group, modelName string) []routingKeyRouteCandidate {
	candidates := make([]routingKeyRouteCandidate, 0, len(channels))
	for _, channel := range channels {
		if channel == nil {
			continue
		}
		if channel.UpstreamKind == UpstreamKindPlatformSite {
			keys := loadRoutableUpstreamKeys(channel, group, modelName)
			for _, key := range keys {
				selection := upstreamKeySelection(channel, key)
				if selection != nil && selection.KeyID != 0 {
					candidates = append(candidates, routingKeyRouteCandidate{channel: channel, key: selection})
				}
			}
			continue
		}
		for _, key := range loadRoutableChannelKeys(channel, group, modelName) {
			if key != nil && key.KeyID != 0 {
				candidates = append(candidates, routingKeyRouteCandidate{channel: channel, key: key})
			}
		}
	}
	return candidates
}

func selectChannelByRoutingKey(channels []*Channel, group, modelName string, retry int) *Channel {
	candidates := routingCandidatesForChannels(channels, group, modelName)
	if len(candidates) == 0 {
		return nil
	}
	priorities := sortedUniqueEffectivePriorities(candidates)
	if retry < 0 {
		retry = 0
	}
	if retry >= len(priorities) {
		return nil
	}
	targetPriority := priorities[retry]
	filtered := make([]routingKeyRouteCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.key.EffectivePriority == targetPriority {
			filtered = append(filtered, candidate)
		}
	}

	var totalWeight int64
	for _, candidate := range filtered {
		weight := candidate.key.Weight
		if weight <= 0 {
			continue
		}
		totalWeight += int64(weight)
	}
	selected := filtered[rand.Intn(len(filtered))]
	if totalWeight > 0 {
		value := rand.Int63n(totalWeight)
		for _, candidate := range filtered {
			value -= int64(candidate.key.Weight)
			if value < 0 {
				selected = candidate
				break
			}
		}
	} else {
		logZeroWeightRoutingFallback(selected.channel.Id, targetPriority)
	}

	channelCopy := *selected.channel
	keyCopy := *selected.key
	channelCopy.SelectedRoutingKey = &keyCopy
	if keyCopy.Source == RoutingKeySourcePlatformSite && keyCopy.UpstreamKeyID != 0 {
		upstreamKey := &UpstreamKey{
			ID:                keyCopy.UpstreamKeyID,
			RoutingKeyID:      keyCopy.KeyID,
			ChannelID:         keyCopy.ChannelID,
			Name:              keyCopy.Name,
			Secret:            keyCopy.Secret,
			KeyPriority:       keyCopy.KeyPriority,
			ConversionRatio:   keyCopy.ConversionRatio,
			Weight:            keyCopy.Weight,
			SecretFingerprint: "",
		}
		channelCopy.SelectedUpstreamKey = upstreamKey
	}
	return &channelCopy
}

func selectChannelByUpstreamKey(channels []*Channel, group, modelName string) *Channel {
	return selectChannelByRoutingKey(channels, group, modelName, 0)
}

// SelectRoutableUpstreamKey 为平台站点选择一个真实可路由的子密钥。返回值是
// 填充了 SelectedUpstreamKey 的渠道副本，调用方可以继续走普通转发初始化路径。
func SelectRoutableUpstreamKey(channel *Channel, group, modelName string) *Channel {
	if channel == nil || channel.UpstreamKind != UpstreamKindPlatformSite {
		return nil
	}
	return selectChannelByRoutingKey([]*Channel{channel}, group, modelName, 0)
}

func SelectRoutableKey(channel *Channel, group, modelName string) *Channel {
	if channel == nil {
		return nil
	}
	return selectChannelByRoutingKey([]*Channel{channel}, group, modelName, 0)
}

func loadRoutableUpstreamKeys(channel *Channel, group, modelName string) []*UpstreamKey {
	if channel == nil || channel.Status != common.ChannelStatusEnabled ||
		channel.UpstreamKind != UpstreamKindPlatformSite {
		return nil
	}
	var account PlatformSiteAccount
	if err := DB.Select("sync_status", "last_sync_at").
		Where("channel_id = ?", channel.Id).
		First(&account).Error; err != nil || !PlatformSiteSnapshotUsable(&account) {
		return nil
	}
	var keys []UpstreamKey
	if err := DB.Where("channel_id = ? AND status = ?", channel.Id, UpstreamKeyStatusEnabled).Find(&keys).Error; err != nil {
		return nil
	}
	now := time.Now()
	upstreamModelName := resolveChannelUpstreamModelName(channel, modelName)
	result := make([]*UpstreamKey, 0, len(keys))
	for index := range keys {
		key := &keys[index]
		if key.RoutingKeyID == 0 {
			if err := EnsureRoutingKeyForUpstreamKey(nil, key); err != nil {
				continue
			}
		}
		availabilityReason := key.AvailabilityReason(now)
		if !key.ModelsSynced ||
			availabilityReason != UpstreamAvailabilityRoutable ||
			!RoutingKeyHealthAllowsRouting(key.RoutingKeyID, key.Status, availabilityReason, now) ||
			!upstreamKeySupportsModel(key, group, upstreamModelName) {
			continue
		}
		if err := key.LoadSecret(); err != nil {
			continue
		}
		result = append(result, key)
	}
	return result
}

func parseChannelModelMappingForRouting(channel *Channel) map[string]string {
	if channel == nil || strings.TrimSpace(channel.GetModelMapping()) == "" || channel.GetModelMapping() == "{}" {
		return nil
	}
	parsed := make(map[string]string)
	if err := common.UnmarshalJsonStr(channel.GetModelMapping(), &parsed); err != nil {
		return nil
	}
	normalized := make(map[string]string, len(parsed))
	for source, target := range parsed {
		source = strings.TrimSpace(source)
		target = strings.TrimSpace(target)
		if source != "" && target != "" {
			normalized[source] = target
		}
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func resolveChannelUpstreamModelName(channel *Channel, modelName string) string {
	modelMap := parseChannelModelMappingForRouting(channel)
	if len(modelMap) == 0 || strings.TrimSpace(modelName) == "" {
		return modelName
	}
	currentModel := modelName
	visitedModels := map[string]bool{currentModel: true}
	for {
		mappedModel, exists := modelMap[currentModel]
		baseModel := reasoning.BaseModelName(currentModel)
		if (!exists || mappedModel == "") && baseModel != currentModel {
			mappedModel, exists = modelMap[baseModel]
		}
		if !exists || mappedModel == "" {
			return currentModel
		}
		if visitedModels[mappedModel] {
			return currentModel
		}
		visitedModels[mappedModel] = true
		currentModel = mappedModel
	}
}

func platformSiteRoutingModels(channel *Channel) []string {
	if channel == nil {
		return nil
	}
	models := uniqueModelNames(channel.GetModels())
	if channel.UpstreamKind != UpstreamKindPlatformSite {
		return models
	}
	modelSet := make(map[string]struct{}, len(models))
	normalizedModelSet := make(map[string]struct{}, len(models))
	for _, modelName := range models {
		modelSet[modelName] = struct{}{}
		normalizedModelSet[ratio_setting.RoutingMatchModelName(modelName)] = struct{}{}
	}
	for source := range parseChannelModelMappingForRouting(channel) {
		target := resolveChannelUpstreamModelName(channel, source)
		if _, ok := modelSet[target]; ok {
			models = append(models, source)
			continue
		}
		if _, ok := normalizedModelSet[ratio_setting.RoutingMatchModelName(target)]; ok {
			models = append(models, source)
		}
	}
	return uniqueModelNames(models)
}

func upstreamKeySupportsModel(key *UpstreamKey, group, modelName string) bool {
	if key == nil {
		return false
	}
	if strings.TrimSpace(modelName) == "" {
		return true
	}
	if !key.AllowsModel(modelName) {
		return false
	}
	var abilities []UpstreamKeyAbility
	if err := DB.Where("upstream_key_id = ? AND enabled = ?", key.ID, true).Find(&abilities).Error; err == nil && len(abilities) > 0 {
		normalizedModel := ratio_setting.RoutingMatchModelName(modelName)
		for _, ability := range abilities {
			if ability.Group != "" && ability.Group != group {
				continue
			}
			if ability.Model == modelName || ratio_setting.RoutingMatchModelName(ability.Model) == normalizedModel {
				return true
			}
		}
		return false
	}
	if key.ModelsSynced {
		normalizedModel := ratio_setting.RoutingMatchModelName(modelName)
		for _, item := range key.GetEffectiveModels() {
			if item == modelName || ratio_setting.RoutingMatchModelName(item) == normalizedModel {
				return true
			}
		}
		return false
	}
	return false
}

func channelSupportsModel(channel *Channel, modelName string) bool {
	if channel == nil {
		return false
	}
	normalizedModel := ratio_setting.RoutingMatchModelName(modelName)
	for _, item := range channel.GetModels() {
		if item == modelName || ratio_setting.RoutingMatchModelName(item) == normalizedModel {
			return true
		}
	}
	return false
}

func upstreamChannelCandidatesByPriority(channels []*Channel, priority int64) []*Channel {
	result := make([]*Channel, 0, len(channels))
	for _, channel := range channels {
		if channel != nil && channel.GetPriority() == priority {
			result = append(result, channel)
		}
	}
	return result
}

func channelSupportsGroup(channel *Channel, group string) bool {
	for item := range strings.SplitSeq(channel.Group, ",") {
		if strings.TrimSpace(item) == group {
			return true
		}
	}
	return false
}
