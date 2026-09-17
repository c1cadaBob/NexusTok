package model

import (
	"math/rand"
	"strings"
	"time"

	"github.com/c1cadaBob/NexusTok/common"
	"github.com/c1cadaBob/NexusTok/setting/ratio_setting"
	"github.com/c1cadaBob/NexusTok/setting/reasoning"
)

type upstreamRouteCandidate struct {
	channel *Channel
	key     *UpstreamKey
}

func buildKeyChannelRoutingKey(channel *Channel) *UpstreamKey {
	key := &UpstreamKey{
		KeyPriority:     channel.KeyPriority,
		ConversionRatio: channel.ConversionRatio,
	}
	if channel.KeyWeightOverride != nil && channel.ConversionRatio != 0 {
		override := max(MinUpstreamKeyWeight, min(MaxUpstreamKeyWeight, *channel.KeyWeightOverride))
		key.WeightOverride = &override
	}
	key.Weight = key.AutoWeight()
	return key
}

func selectChannelByUpstreamKey(channels []*Channel, group, modelName string) *Channel {
	candidates := make([]upstreamRouteCandidate, 0, len(channels))
	for _, channel := range channels {
		if channel == nil {
			continue
		}
		if channel.UpstreamKind == UpstreamKindPlatformSite {
			keys := loadRoutableUpstreamKeys(channel, group, modelName)
			for _, key := range keys {
				candidates = append(candidates, upstreamRouteCandidate{channel: channel, key: key})
			}
			continue
		}
		candidates = append(candidates, upstreamRouteCandidate{
			channel: channel,
			key:     buildKeyChannelRoutingKey(channel),
		})
	}
	if len(candidates) == 0 {
		return nil
	}

	maxPriority := candidates[0].key.KeyPriority
	for _, candidate := range candidates[1:] {
		if candidate.key.KeyPriority > maxPriority {
			maxPriority = candidate.key.KeyPriority
		}
	}
	filtered := candidates[:0]
	for _, candidate := range candidates {
		if candidate.key.KeyPriority == maxPriority {
			filtered = append(filtered, candidate)
		}
	}

	var totalWeight int64
	for _, candidate := range filtered {
		weight := candidate.key.EffectiveWeight()
		if weight <= 0 {
			continue
		}
		totalWeight += int64(weight)
	}
	selected := filtered[rand.Intn(len(filtered))]
	if totalWeight > 0 {
		value := rand.Int63n(totalWeight)
		for _, candidate := range filtered {
			value -= int64(candidate.key.EffectiveWeight())
			if value < 0 {
				selected = candidate
				break
			}
		}
	}

	channelCopy := *selected.channel
	if selected.key.Secret != "" {
		keyCopy := *selected.key
		channelCopy.SelectedUpstreamKey = &keyCopy
	}
	return &channelCopy
}

// SelectRoutableUpstreamKey 为平台站点选择一个真实可路由的子密钥。返回值是
// 填充了 SelectedUpstreamKey 的渠道副本，调用方可以继续走普通转发初始化路径。
func SelectRoutableUpstreamKey(channel *Channel, group, modelName string) *Channel {
	if channel == nil || channel.UpstreamKind != UpstreamKindPlatformSite {
		return nil
	}
	return selectChannelByUpstreamKey([]*Channel{channel}, group, modelName)
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
		if !key.ModelsSynced ||
			!key.IsRoutable(now) ||
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
		for _, item := range key.GetModels() {
			if item == modelName || ratio_setting.RoutingMatchModelName(item) == normalizedModel {
				return true
			}
		}
		return false
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
