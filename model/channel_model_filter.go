package model

import "strings"

// matchesAdminModelFilter 实现渠道管理页的模型筛选语义。
// 不含通配符时保持大小写不敏感的包含匹配；含通配符时按完整模型名匹配。
func matchesAdminModelFilter(modelName, filter string) bool {
	modelName = strings.TrimSpace(modelName)
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return true
	}

	modelName = strings.ToLower(modelName)
	filter = strings.ToLower(filter)
	if !strings.ContainsAny(filter, "*?") {
		return strings.Contains(modelName, filter)
	}

	pattern := []rune(filter)
	value := []rune(modelName)
	patternIndex := 0
	valueIndex := 0
	lastStarIndex := -1
	starValueIndex := -1

	for valueIndex < len(value) {
		var patternChar rune
		if patternIndex < len(pattern) {
			patternChar = pattern[patternIndex]
		}
		if patternIndex < len(pattern) &&
			(patternChar == '?' || patternChar == value[valueIndex]) {
			patternIndex++
			valueIndex++
			continue
		}
		if patternIndex < len(pattern) && patternChar == '*' {
			lastStarIndex = patternIndex
			starValueIndex = valueIndex
			patternIndex++
			continue
		}
		if lastStarIndex < 0 {
			return false
		}

		patternIndex = lastStarIndex + 1
		starValueIndex++
		valueIndex = starValueIndex
	}

	for patternIndex < len(pattern) && pattern[patternIndex] == '*' {
		patternIndex++
	}
	return patternIndex == len(pattern)
}

// adminModelLikePattern 生成数据库候选查询使用的 LIKE 模式。
// 最终结果仍会经过 matchesAdminModelFilter，避免数据库方言差异改变语义。
func adminModelLikePattern(filter string) string {
	var builder strings.Builder
	builder.WriteByte('%')
	for _, char := range strings.ToLower(strings.TrimSpace(filter)) {
		switch char {
		case '*':
			builder.WriteByte('%')
		case '?':
			builder.WriteByte('_')
		case '!', '%', '_', '\\':
			builder.WriteByte('!')
			builder.WriteRune(char)
		default:
			builder.WriteRune(char)
		}
	}
	builder.WriteByte('%')
	return builder.String()
}

func adminModelLikeClause(column string) string {
	return "LOWER(" + column + ") LIKE LOWER(?) ESCAPE '!'"
}

func channelMatchesAdminModelFilter(
	channel *Channel,
	filter string,
	keysByChannelID map[int][]UpstreamKey,
) bool {
	if channel == nil {
		return false
	}
	for _, modelName := range channel.GetModels() {
		if matchesAdminModelFilter(modelName, filter) {
			return true
		}
	}
	for _, key := range keysByChannelID[channel.Id] {
		for _, modelName := range key.GetEffectiveModels() {
			if matchesAdminModelFilter(modelName, filter) {
				return true
			}
		}
	}
	return false
}

func filterChannelsByAdminModel(
	channels []*Channel,
	filter string,
) ([]*Channel, error) {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return channels, nil
	}

	channelIDs := make([]int, 0, len(channels))
	for _, channel := range channels {
		if channel != nil {
			channelIDs = append(channelIDs, channel.Id)
		}
	}
	keysByChannelID := make(map[int][]UpstreamKey)
	if len(channelIDs) > 0 && DB.Migrator().HasTable(&UpstreamKey{}) {
		var keys []UpstreamKey
		if err := DB.Where("channel_id IN ?", channelIDs).Find(&keys).Error; err != nil {
			return nil, err
		}
		for _, key := range keys {
			keysByChannelID[key.ChannelID] = append(
				keysByChannelID[key.ChannelID],
				key,
			)
		}
	}

	filtered := make([]*Channel, 0, len(channels))
	for _, channel := range channels {
		if channelMatchesAdminModelFilter(channel, filter, keysByChannelID) {
			filtered = append(filtered, channel)
		}
	}
	return filtered, nil
}
