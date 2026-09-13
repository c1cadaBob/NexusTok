package model

import (
	"math/rand"
	"strings"
	"time"

	"github.com/c1cadaBob/NexusTok/setting/ratio_setting"
)

type upstreamRouteCandidate struct {
	channel *Channel
	key     *UpstreamKey
}

func selectChannelByUpstreamKey(channels []*Channel, group, modelName string) *Channel {
	candidates := make([]upstreamRouteCandidate, 0, len(channels))
	for _, channel := range channels {
		if channel == nil {
			continue
		}
		if channel.UpstreamKind == UpstreamKindPlatformSite {
			keys := loadRoutableUpstreamKeys(channel.Id, group, modelName)
			for _, key := range keys {
				candidates = append(candidates, upstreamRouteCandidate{channel: channel, key: key})
			}
			continue
		}
		weight := 1000
		if channel.KeyWeightOverride != nil {
			weight = max(MinUpstreamKeyWeight, min(MaxUpstreamKeyWeight, *channel.KeyWeightOverride))
		} else if channel.ConversionRatio > 0 {
			if calculated, err := CalculateUpstreamKeyWeight(channel.ConversionRatio); err == nil {
				weight = calculated
			}
		}
		candidates = append(candidates, upstreamRouteCandidate{
			channel: channel,
			key: &UpstreamKey{
				KeyPriority: channel.KeyPriority,
				Weight:      weight,
			},
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

	totalWeight := 0
	for _, candidate := range filtered {
		totalWeight += candidate.key.EffectiveWeight()
	}
	selected := filtered[rand.Intn(len(filtered))]
	if totalWeight > 0 {
		value := rand.Intn(totalWeight)
		for _, candidate := range filtered {
			value -= candidate.key.EffectiveWeight()
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

func loadRoutableUpstreamKeys(channelID int, group, modelName string) []*UpstreamKey {
	var keys []UpstreamKey
	if err := DB.Where("channel_id = ? AND status = ?", channelID, UpstreamKeyStatusEnabled).Find(&keys).Error; err != nil {
		return nil
	}
	now := time.Now()
	result := make([]*UpstreamKey, 0, len(keys))
	for index := range keys {
		key := &keys[index]
		if !key.IsRoutable(now) || !upstreamKeySupportsModel(key, group, modelName) {
			continue
		}
		credential, err := DecryptPlatformSiteCredential(key.SecretCiphertext)
		if err != nil || credential.AccessToken == "" {
			continue
		}
		key.Secret = credential.AccessToken
		result = append(result, key)
	}
	return result
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
	for _, item := range key.GetModels() {
		if item == modelName || ratio_setting.RoutingMatchModelName(item) == ratio_setting.RoutingMatchModelName(modelName) {
			return true
		}
	}
	return true
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
