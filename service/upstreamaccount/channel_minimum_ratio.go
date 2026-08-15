package upstreamaccount

import (
	"math"
	"sort"
	"strings"

	"github.com/c1cada/NexusTok/model"
)

// AttachChannelMinimumRatios 为渠道列表批量附加最低换算倍率。
//
// minimum_ratio 是运行期展示字段，不落库。它从渠道内所有账号的同步元数据中读取
// ratio_conversion / effective_ratio / group_ratio 等安全展示字段，包含禁用账号，
// 因为该列表达的是成本参考信息，不代表实际路由可用性。
func AttachChannelMinimumRatios(channels []*model.Channel) error {
	return AttachChannelMinimumRatiosForModel(channels, "")
}

// AttachChannelMinimumRatiosForModel 为渠道列表按可选模型上下文附加最低换算倍率。
// modelName 为空时保持全渠道最低倍率；非空时只统计模型列表能匹配该模型的同步密钥，
// 不匹配的渠道保留 nil，供列表排序时稳定排在最后。
func AttachChannelMinimumRatiosForModel(channels []*model.Channel, modelName string) error {
	channelIDs := make([]int, 0, len(channels))
	seen := make(map[int]bool, len(channels))
	for _, channel := range channels {
		if channel == nil || channel.Id <= 0 || seen[channel.Id] {
			continue
		}
		seen[channel.Id] = true
		channelIDs = append(channelIDs, channel.Id)
	}
	if len(channelIDs) == 0 {
		return nil
	}

	var rows []struct {
		ChannelID int    `gorm:"column:channel_id"`
		Settings  string `gorm:"column:settings"`
		Models    string `gorm:"column:models"`
	}
	if err := model.DB.Model(&model.ChannelAccount{}).
		Select("channel_id", "settings", "models").
		Where("channel_id IN ?", channelIDs).
		Find(&rows).Error; err != nil {
		return err
	}

	selectedModel := strings.TrimSpace(modelName)
	minimumByChannel := make(map[int]float64, len(channelIDs))
	for _, row := range rows {
		if selectedModel != "" && !model.MatchesModelList(model.SplitCommaValues(row.Models), selectedModel) {
			continue
		}
		ratio, ok := accountMinimumRatioFromSettings(row.Settings)
		if !ok {
			continue
		}
		if current, exists := minimumByChannel[row.ChannelID]; !exists || ratio < current {
			minimumByChannel[row.ChannelID] = ratio
		}
	}

	for _, channel := range channels {
		if channel == nil {
			continue
		}
		if ratio, ok := minimumByChannel[channel.Id]; ok {
			value := ratio
			channel.MinimumRatio = &value
		} else {
			channel.MinimumRatio = nil
		}
	}

	return nil
}

// CollectChannelMinimumRatioModels 聚合渠道内同步密钥实际配置的模型列表。
// 该列表只来自 ChannelAccount.Models 和同步元数据身份判断，不读取明文 key 或凭证；
// 用于前端“最低倍率”列选择模型上下文。
func CollectChannelMinimumRatioModels(channels []*model.Channel) ([]string, error) {
	channelIDs := make([]int, 0, len(channels))
	seenChannel := make(map[int]bool, len(channels))
	for _, channel := range channels {
		if channel == nil || channel.Id <= 0 || seenChannel[channel.Id] {
			continue
		}
		seenChannel[channel.Id] = true
		channelIDs = append(channelIDs, channel.Id)
	}
	if len(channelIDs) == 0 {
		return []string{}, nil
	}

	var rows []struct {
		Settings string `gorm:"column:settings"`
		Models   string `gorm:"column:models"`
	}
	if err := model.DB.Model(&model.ChannelAccount{}).
		Select("settings", "models").
		Where("channel_id IN ?", channelIDs).
		Find(&rows).Error; err != nil {
		return nil, err
	}

	seenModel := map[string]string{}
	for _, row := range rows {
		if !HasAccountSyncMetadata(row.Settings) {
			continue
		}
		for _, modelName := range model.SplitCommaValues(row.Models) {
			key := strings.ToLower(strings.TrimSpace(modelName))
			if key == "" {
				continue
			}
			if _, exists := seenModel[key]; !exists {
				seenModel[key] = strings.TrimSpace(modelName)
			}
		}
	}

	models := make([]string, 0, len(seenModel))
	for _, modelName := range seenModel {
		models = append(models, modelName)
	}
	sort.Slice(models, func(i, j int) bool {
		left := strings.ToLower(models[i])
		right := strings.ToLower(models[j])
		if left == right {
			return models[i] < models[j]
		}
		return left < right
	})
	return models, nil
}

// SortChannelsByMinimumRatio 按最低倍率对已回填 minimum_ratio 的渠道切片排序。
//
// 无倍率数据的渠道始终排在最后；同倍率时按渠道 ID 降序保证结果稳定，避免分页时
// 因 map 或数据库默认顺序差异产生跳动。
func SortChannelsByMinimumRatio(channels []*model.Channel, desc bool) {
	sort.SliceStable(channels, func(i, j int) bool {
		left := channels[i]
		right := channels[j]
		if left == nil {
			return false
		}
		if right == nil {
			return true
		}
		leftValue := left.MinimumRatio
		rightValue := right.MinimumRatio
		if leftValue == nil && rightValue == nil {
			return left.Id > right.Id
		}
		if leftValue == nil {
			return false
		}
		if rightValue == nil {
			return true
		}
		if *leftValue == *rightValue {
			return left.Id > right.Id
		}
		if desc {
			return *leftValue > *rightValue
		}
		return *leftValue < *rightValue
	})
}

func accountMinimumRatioFromSettings(settings string) (float64, bool) {
	metadata := ReadAccountSyncDisplayMetadata(settings)

	if validMinimumRatio(metadata.RatioConversion) {
		return metadata.RatioConversion, true
	}
	if validMinimumRatio(metadata.EffectiveRatio) {
		return metadata.EffectiveRatio, true
	}
	if metadata.GroupRatio != nil {
		if validMinimumRatio(*metadata.GroupRatio) {
			return *metadata.GroupRatio, true
		}
	}

	var minimum float64
	found := false
	for _, ratio := range metadata.ModelRatios {
		if !validMinimumRatio(ratio) {
			continue
		}
		if !found {
			minimum = ratio
			found = true
			continue
		}
		if ratio < minimum {
			minimum = ratio
		}
	}
	return minimum, found
}

func validMinimumRatio(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}
