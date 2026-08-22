// Package model - channel_cache.go
// 该文件实现了渠道数据的内存缓存机制
//
// 缓存数据结构：
// - group2model2channels：分组 -> 模型 -> 渠道 ID 列表的映射（仅启用的渠道）
// - channelsIDM：渠道 ID -> 渠道对象的映射（包含所有渠道）
//
// 核心功能：
// - InitChannelCache：从数据库加载渠道和能力数据，构建内存缓存
// - SyncChannelCache：定时同步渠道缓存（后台协程）
// - GetRandomSatisfiedChannel：基于优先级和权重的加权随机渠道选择
// - CacheGetChannel/CacheGetChannelInfo：从缓存获取渠道信息
// - CacheUpdateChannelStatus/CacheUpdateChannel：更新缓存中的渠道状态
//
// 缓存特点：
// - 使用读写锁（sync.RWMutex）保证并发安全
// - 支持多 Key 轮询模式的索引保留
// - 渠道按优先级降序排列，同优先级内按权重随机选择
package model

import (
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/setting/ratio_setting"
)

// group2model2channels 分组 -> 模型 -> 渠道 ID 列表的映射（仅包含启用的渠道）
var group2model2channels map[string]map[string][]int // 启用渠道映射

// channelsIDM 渠道 ID -> 渠道对象的映射（包含所有渠道，含禁用的）
var channelsIDM map[int]*Channel // 包含禁用状态在内的全量渠道

// channel2advancedCustomConfig 缓存 type 58 渠道解析后的 Advanced Custom 配置。
// 选路热路径只需要判断 request path 是否命中 route，缓存配置可以避免每次请求重复解析 JSON。
var channel2advancedCustomConfig map[int]*dto.AdvancedCustomConfig

// channelAbilitySchedules 保存 group/model/channel 维度的 Ability 调度值。
//
// channels 表只能表达渠道级 priority/weight；账号池渠道会按每个启用密钥生成 Ability，
// 同一个渠道在不同模型或分组下可能对应不同的最优密钥调度值。内存缓存选路必须读取
// 这里的 Ability 值，才能与数据库兜底路径保持一致。
var channelAbilitySchedules map[string]map[string]map[int]channelAbilitySchedule

type channelAbilitySchedule struct {
	Priority int64
	Weight   int
}

// channelSyncLock 渠道缓存的读写锁，保证并发安全
var channelSyncLock sync.RWMutex

// InitChannelCache 从数据库加载渠道和能力数据，构建内存缓存
// 如果内存缓存未启用（common.MemoryCacheEnabled=false），则跳过
// 构建过程：
// 1. 加载所有渠道到 channelsIDM 映射
// 2. 加载所有能力记录，提取分组信息
// 3. 构建 group2model2channels 映射（仅启用的渠道）
// 4. 按优先级降序排列每个模型的渠道列表
// 5. 保留多 Key 轮询模式的索引信息
func InitChannelCache() {
	if !common.MemoryCacheEnabled {
		return
	}
	newChannelId2channel := make(map[int]*Channel)
	newChannel2advancedCustomConfig := make(map[int]*dto.AdvancedCustomConfig)
	var channels []*Channel
	DB.Find(&channels)
	for _, channel := range channels {
		newChannelId2channel[channel.Id] = channel
		if channel.Type == constant.ChannelTypeAdvancedCustom {
			if config := channel.GetOtherSettings().AdvancedCustom; config != nil {
				newChannel2advancedCustomConfig[channel.Id] = config
			}
		}
	}
	newGroup2model2channels := make(map[string]map[string][]int)
	newChannelAbilitySchedules := make(map[string]map[string]map[int]channelAbilitySchedule)
	newRoutingCandidateCache := buildRoutingCandidateCache(channels)
	var abilities []*Ability
	DB.Find(&abilities)
	representedAbilityKeys := make(map[string]struct{}, len(abilities))
	for _, ability := range abilities {
		if ability == nil {
			continue
		}
		group := strings.TrimSpace(ability.Group)
		modelName := strings.TrimSpace(ability.Model)
		if group == "" || modelName == "" || ability.ChannelId <= 0 {
			continue
		}
		representedAbilityKeys[cachedAbilityKey(group, modelName, ability.ChannelId)] = struct{}{}
		channel, ok := newChannelId2channel[ability.ChannelId]
		if !ok || channel.Status != common.ChannelStatusEnabled || !ability.Enabled {
			continue
		}
		priority := int64(0)
		if ability.Priority != nil {
			priority = *ability.Priority
		}
		addCachedChannelAbility(newGroup2model2channels, newChannelAbilitySchedules, group, modelName, channel.Id, priority, int(ability.Weight))
	}
	for _, channel := range channels {
		if channel.Status != common.ChannelStatusEnabled {
			continue // 跳过禁用渠道，避免它们进入运行期选路缓存。
		}
		groups := strings.Split(channel.Group, ",")
		for _, group := range groups {
			group = strings.TrimSpace(group)
			if group == "" {
				continue
			}
			if _, ok := newGroup2model2channels[group]; !ok {
				// 渠道缓存会在启动、热更新和能力修复时重建；此时 abilities 表可能
				// 尚未包含刚创建渠道的分组，必须按渠道本身的分组懒初始化二级 map，
				// 否则会因为写入 nil map 导致进程启动失败。
				newGroup2model2channels[group] = make(map[string][]int)
			}
			models := strings.Split(channel.Models, ",")
			for _, model := range models {
				model = strings.TrimSpace(model)
				if model == "" {
					continue
				}
				if _, represented := representedAbilityKeys[cachedAbilityKey(group, model, channel.Id)]; represented {
					continue
				}
				addCachedChannelAbility(newGroup2model2channels, newChannelAbilitySchedules, group, model, channel.Id, channel.GetPriority(), channel.GetWeight())
			}
		}
	}

	// 按 Ability 或渠道调度值排序，保证缓存列表本身也保持最优候选在前。
	for group, model2channels := range newGroup2model2channels {
		for model, channels := range model2channels {
			sort.Slice(channels, func(i, j int) bool {
				left := cachedChannelScheduleFrom(newChannelAbilitySchedules, group, model, channels[i], newChannelId2channel[channels[i]])
				right := cachedChannelScheduleFrom(newChannelAbilitySchedules, group, model, channels[j], newChannelId2channel[channels[j]])
				if left.Priority != right.Priority {
					return left.Priority > right.Priority
				}
				if left.Weight != right.Weight {
					return left.Weight > right.Weight
				}
				return channels[i] < channels[j]
			})
			newGroup2model2channels[group][model] = channels
		}
	}

	channelSyncLock.Lock()
	group2model2channels = newGroup2model2channels
	for i, channel := range newChannelId2channel {
		if channel.ChannelInfo.IsMultiKey {
			channel.Keys = channel.GetKeys()
			if channel.ChannelInfo.MultiKeyMode == constant.MultiKeyModePolling {
				if oldChannel, ok := channelsIDM[i]; ok {
					// 存在旧的渠道，如果是多key且轮询，保留轮询索引信息
					if oldChannel.ChannelInfo.IsMultiKey && oldChannel.ChannelInfo.MultiKeyMode == constant.MultiKeyModePolling {
						channel.ChannelInfo.MultiKeyPollingIndex = oldChannel.ChannelInfo.MultiKeyPollingIndex
					}
				}
			}
		}
	}
	channelsIDM = newChannelId2channel
	channel2advancedCustomConfig = newChannel2advancedCustomConfig
	channelAbilitySchedules = newChannelAbilitySchedules
	routingCandidateCache = newRoutingCandidateCache
	channelSyncLock.Unlock()
	common.SysLog("channels synced from database")
}

func addCachedChannelAbility(
	groupModelChannels map[string]map[string][]int,
	schedules map[string]map[string]map[int]channelAbilitySchedule,
	group string,
	modelName string,
	channelID int,
	priority int64,
	weight int,
) {
	if group == "" || modelName == "" || channelID <= 0 {
		return
	}
	if _, ok := groupModelChannels[group]; !ok {
		groupModelChannels[group] = make(map[string][]int)
	}
	if _, ok := groupModelChannels[group][modelName]; !ok {
		groupModelChannels[group][modelName] = make([]int, 0)
	}
	for _, existingID := range groupModelChannels[group][modelName] {
		if existingID == channelID {
			setCachedChannelSchedule(schedules, group, modelName, channelID, priority, weight)
			return
		}
	}
	groupModelChannels[group][modelName] = append(groupModelChannels[group][modelName], channelID)
	setCachedChannelSchedule(schedules, group, modelName, channelID, priority, weight)
}

func setCachedChannelSchedule(schedules map[string]map[string]map[int]channelAbilitySchedule, group string, modelName string, channelID int, priority int64, weight int) {
	if _, ok := schedules[group]; !ok {
		schedules[group] = make(map[string]map[int]channelAbilitySchedule)
	}
	if _, ok := schedules[group][modelName]; !ok {
		schedules[group][modelName] = make(map[int]channelAbilitySchedule)
	}
	schedules[group][modelName][channelID] = channelAbilitySchedule{Priority: priority, Weight: weight}
}

func cachedAbilityKey(group string, modelName string, channelID int) string {
	return fmt.Sprintf("%s\x00%s\x00%d", group, modelName, channelID)
}

// SyncChannelCache 定时同步渠道缓存（后台协程）
// 每隔 frequency 秒从数据库重新加载渠道数据
func SyncChannelCache(frequency int) {
	for {
		time.Sleep(time.Duration(frequency) * time.Second)
		common.SysLog("syncing channels from database")
		InitChannelCache()
	}
}

// GetRandomSatisfiedChannel 基于优先级和权重的加权随机渠道选择
// 当内存缓存启用时从缓存选择，否则回退到数据库查询
// 选择算法：
// 1. 根据分组和模型查找可用渠道列表
// 2. 按优先级分组，retry 参数决定使用哪个优先级
// 3. 在目标优先级内，按权重进行加权随机选择
// 4. 使用平滑因子处理权重差异过小的情况
func GetRandomSatisfiedChannel(group string, model string, retry int, requestPath string) (*Channel, error) {
	return GetRandomSatisfiedChannelWithExclusions(group, model, retry, requestPath, nil)
}

// GetRandomSatisfiedChannelWithExclusions 基于优先级和权重选择渠道，并排除当前请求中已失败的渠道。
//
// excludedChannelIds 只影响当前请求的重试选路：当某个渠道在本请求中已经失败，
// 下一轮不能再次命中它，即使自动禁用还没有完成 DB 和缓存更新。没有排除项时
// 保留旧的 retry -> 优先级层级映射；存在排除项时，会从 retry 指向的优先级
// 切换为“剩余候选的最高优先级”，避免同优先级其他健康渠道被跳过。
func GetRandomSatisfiedChannelWithExclusions(group string, model string, retry int, requestPath string, excludedChannelIds []int) (*Channel, error) {
	// 未启用内存缓存时，直接使用数据库兜底路径。
	if !common.MemoryCacheEnabled {
		return GetChannelWithExclusions(group, model, retry, requestPath, excludedChannelIds)
	}

	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	// 先按原始模型名查找，找不到时在 helper 内回退到规范化模型名。
	excludedSet := intSliceToSet(excludedChannelIds)

	channels, cacheModel := getCachedCandidateChannelIDs(group, model, requestPath, excludedSet)

	if len(channels) == 0 {
		return nil, nil
	}

	if len(channels) == 1 {
		if channel, ok := channelsIDM[channels[0]]; ok {
			return materializeCachedChannel(group, cacheModel, channel), nil
		}
		return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channels[0])
	}

	uniquePriorities := make(map[int]bool)
	for _, channelId := range channels {
		if channel, ok := channelsIDM[channelId]; ok {
			uniquePriorities[int(cachedChannelSchedule(group, cacheModel, channelId, channel).Priority)] = true
		} else {
			return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channelId)
		}
	}
	var sortedUniquePriorities []int
	for priority := range uniquePriorities {
		sortedUniquePriorities = append(sortedUniquePriorities, priority)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(sortedUniquePriorities)))

	if retry >= len(uniquePriorities) {
		retry = len(uniquePriorities) - 1
	}
	targetPriority := int64(sortedUniquePriorities[selectPriorityIndexForRetry(sortedUniquePriorities, retry, len(excludedSet) > 0)])

	// 根据本次 retry 对应的优先级层级收集候选渠道。
	var sumWeight = 0
	var targetChannels []*Channel
	for _, channelId := range channels {
		if channel, ok := channelsIDM[channelId]; ok {
			schedule := cachedChannelSchedule(group, cacheModel, channelId, channel)
			if schedule.Priority == targetPriority {
				sumWeight += schedule.Weight
				targetChannels = append(targetChannels, materializeCachedChannel(group, cacheModel, channel))
			}
		} else {
			return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channelId)
		}
	}

	if len(targetChannels) == 0 {
		return nil, errors.New(fmt.Sprintf("no channel found, group: %s, model: %s, priority: %d", group, model, targetPriority))
	}

	// 平滑权重，兼容历史“权重全为 0 时等权随机”的行为。
	smoothingFactor := 1
	smoothingAdjustment := 0

	if sumWeight == 0 {
		// 所有渠道权重为 0 时，每个渠道按有效权重 100 参与随机。
		sumWeight = len(targetChannels) * 100
		smoothingAdjustment = 100
	} else if sumWeight/len(targetChannels) < 10 {
		// 平均权重过小时放大 100 倍，降低整数随机带来的偏差。
		smoothingFactor = 100
	}

	// 计算目标优先级内的总权重并执行加权随机。
	totalWeight := sumWeight * smoothingFactor
	randomWeight := rand.Intn(totalWeight)
	for _, channel := range targetChannels {
		randomWeight -= channel.GetWeight()*smoothingFactor + smoothingAdjustment
		if randomWeight < 0 {
			return channel, nil
		}
	}
	return nil, errors.New("channel not found")
}

// GetSatisfiedChannelCandidatesWithExclusions 获取现有选路规则下的候选渠道列表。
//
// 该函数只负责复用原有的分组、模型、优先级、状态、请求路径和请求级排除逻辑，
// 不执行随机选择。服务层可以在这份候选列表上叠加临时健康状态，而不会改写
// 数据库中的 priority/weight，也不会破坏账号池先换账号再换渠道的行为。
func GetSatisfiedChannelCandidatesWithExclusions(group string, model string, retry int, requestPath string, excludedChannelIds []int) ([]*Channel, error) {
	if !common.MemoryCacheEnabled {
		return getChannelCandidatesWithExclusions(group, model, retry, requestPath, excludedChannelIds)
	}

	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	excludedSet := intSliceToSet(excludedChannelIds)
	channelIds, cacheModel := getCachedCandidateChannelIDs(group, model, requestPath, excludedSet)
	if len(channelIds) == 0 {
		return nil, nil
	}

	uniquePriorities := make(map[int]bool)
	for _, channelId := range channelIds {
		channel, ok := channelsIDM[channelId]
		if !ok {
			return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channelId)
		}
		uniquePriorities[int(cachedChannelSchedule(group, cacheModel, channelId, channel).Priority)] = true
	}
	sortedUniquePriorities := make([]int, 0, len(uniquePriorities))
	for priority := range uniquePriorities {
		sortedUniquePriorities = append(sortedUniquePriorities, priority)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(sortedUniquePriorities)))
	targetPriority := int64(sortedUniquePriorities[selectPriorityIndexForRetry(sortedUniquePriorities, retry, len(excludedSet) > 0)])

	targetChannels := make([]*Channel, 0, len(channelIds))
	for _, channelId := range channelIds {
		channel := channelsIDM[channelId]
		if cachedChannelSchedule(group, cacheModel, channelId, channel).Priority == targetPriority {
			targetChannels = append(targetChannels, materializeCachedChannel(group, cacheModel, channel))
		}
	}
	if len(targetChannels) == 0 {
		return nil, fmt.Errorf("no channel found, group: %s, model: %s, priority: %d", group, model, targetPriority)
	}
	return targetChannels, nil
}

// GetAllSatisfiedChannelCandidatesWithExclusions 获取分组和模型下所有优先级的候选渠道。
//
// 该入口只给动态健康策略使用：普通选路仍由 retry 控制优先级层级。动态策略在
// 当前层级全部处于临时冷却时，可以通过它寻找尚未降级的低优先级候选，实现
// “先避开故障，再保持服务可用”的自动降级。
func GetAllSatisfiedChannelCandidatesWithExclusions(group string, model string, requestPath string, excludedChannelIds []int) ([]*Channel, error) {
	if !common.MemoryCacheEnabled {
		return getAllChannelCandidatesWithExclusions(group, model, requestPath, excludedChannelIds)
	}

	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	excludedSet := intSliceToSet(excludedChannelIds)
	channelIDs, cacheModel := getCachedCandidateChannelIDs(group, model, requestPath, excludedSet)
	if len(channelIDs) == 0 {
		return nil, nil
	}

	channels := make([]*Channel, 0, len(channelIDs))
	for _, channelID := range channelIDs {
		channel, ok := channelsIDM[channelID]
		if !ok {
			return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channelID)
		}
		channels = append(channels, materializeCachedChannel(group, cacheModel, channel))
	}
	return channels, nil
}

func getCachedCandidateChannelIDs(group string, model string, requestPath string, excludedSet map[int]struct{}) ([]int, string) {
	channelIDs := filterCandidateChannels(group2model2channels[group][model], requestPath, excludedSet)
	if len(channelIDs) > 0 {
		return channelIDs, model
	}
	normalizedModel := ratio_setting.FormatMatchingModelName(model)
	channelIDs = filterCandidateChannels(group2model2channels[group][normalizedModel], requestPath, excludedSet)
	return channelIDs, normalizedModel
}

func cachedChannelSchedule(group string, modelName string, channelID int, channel *Channel) channelAbilitySchedule {
	return cachedChannelScheduleFrom(channelAbilitySchedules, group, modelName, channelID, channel)
}

func cachedChannelScheduleFrom(schedules map[string]map[string]map[int]channelAbilitySchedule, group string, modelName string, channelID int, channel *Channel) channelAbilitySchedule {
	if schedules != nil {
		if modelSchedules, ok := schedules[group]; ok {
			if channelSchedules, ok := modelSchedules[modelName]; ok {
				if schedule, ok := channelSchedules[channelID]; ok {
					return schedule
				}
			}
		}
	}
	if channel == nil {
		return channelAbilitySchedule{}
	}
	return channelAbilitySchedule{Priority: channel.GetPriority(), Weight: channel.GetWeight()}
}

func materializeCachedChannel(group string, modelName string, channel *Channel) *Channel {
	if channel == nil {
		return nil
	}
	schedule := cachedChannelSchedule(group, modelName, channel.Id, channel)
	if schedule.Priority == channel.GetPriority() && schedule.Weight == channel.GetWeight() {
		return channel
	}
	priority := schedule.Priority
	weight := uint(0)
	if schedule.Weight > 0 {
		weight = uint(schedule.Weight)
	}
	clone := *channel
	clone.Priority = &priority
	clone.Weight = &weight
	return &clone
}

// SelectChannelByWeight 按渠道权重随机选择一个候选。
//
// 这里集中保留与旧内存选路一致的平滑权重算法，供动态健康选路复用，
// 确保新增策略不会改变同分候选的随机语义。
func SelectChannelByWeight(channels []*Channel) *Channel {
	if len(channels) == 0 {
		return nil
	}
	if len(channels) == 1 {
		return channels[0]
	}

	sumWeight := 0
	for _, channel := range channels {
		if channel != nil {
			sumWeight += channel.GetWeight()
		}
	}
	smoothingFactor := 1
	smoothingAdjustment := 0
	if sumWeight == 0 {
		sumWeight = len(channels) * 100
		smoothingAdjustment = 100
	} else if sumWeight/len(channels) < 10 {
		smoothingFactor = 100
	}

	totalWeight := sumWeight * smoothingFactor
	if totalWeight <= 0 {
		return channels[0]
	}
	randomWeight := rand.Intn(totalWeight)
	for _, channel := range channels {
		if channel == nil {
			continue
		}
		randomWeight -= channel.GetWeight()*smoothingFactor + smoothingAdjustment
		if randomWeight < 0 {
			return channel
		}
	}
	return channels[len(channels)-1]
}

// selectPriorityIndexForRetry 计算本次重试应使用的优先级索引。
//
// 没有请求级排除集时保持旧行为：retry=0 选最高优先级，retry=1 选下一档。
// 有排除集时，候选列表已经移除了失败渠道，因此应优先从剩余最高优先级开始；
// 若该优先级已被排空，sortedPriorities 自然只剩更低档位，实现自动降级。
func selectPriorityIndexForRetry(sortedPriorities []int, retry int, hasExclusions bool) int {
	if len(sortedPriorities) == 0 {
		return 0
	}
	if hasExclusions {
		return 0
	}
	if retry < 0 {
		return 0
	}
	if retry >= len(sortedPriorities) {
		return len(sortedPriorities) - 1
	}
	return retry
}

// filterCandidateChannels 过滤当前请求不可用的候选渠道。
//
// 这里会二次校验渠道状态，防止缓存列表中残留已禁用 ID；同时排除本请求已经
// 失败过的渠道，并继续应用 Advanced Custom 的 request path 约束。
func filterCandidateChannels(channels []int, requestPath string, excludedSet map[int]struct{}) []int {
	channels = filterChannelsByRequestPath(channels, requestPath)
	if len(channels) == 0 {
		return channels
	}
	filtered := make([]int, 0, len(channels))
	seen := make(map[int]struct{}, len(channels))
	for _, channelId := range channels {
		if _, exists := seen[channelId]; exists {
			continue
		}
		seen[channelId] = struct{}{}
		if _, excluded := excludedSet[channelId]; excluded {
			continue
		}
		channel, ok := channelsIDM[channelId]
		if !ok {
			// 保留未知 ID，让后续一致性检查继续按旧逻辑报错。
			filtered = append(filtered, channelId)
			continue
		}
		if channel.Status != common.ChannelStatusEnabled {
			continue
		}
		filtered = append(filtered, channelId)
	}
	return filtered
}

// intSliceToSet 将 ID 列表转换为集合，忽略无效 ID。
func intSliceToSet(ids []int) map[int]struct{} {
	set := make(map[int]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		set[id] = struct{}{}
	}
	return set
}

// filterChannelsByRequestPath 按当前请求路径过滤内存缓存中的候选渠道。
//
// 只过滤 Advanced Custom 渠道：普通渠道的能力仍由 group/model/status 决定；
// type 58 渠道必须有至少一条 incoming_path 命中 requestPath 才能参与选路。
// requestPath 为空时跳过过滤，以兼容测试、维护任务等非 relay 调用。
// 调用方必须已经持有 channelSyncLock 读锁；本函数不会修改缓存切片。
func filterChannelsByRequestPath(channels []int, requestPath string) []int {
	if requestPath == "" || len(channels) == 0 {
		return channels
	}
	filtered := make([]int, 0, len(channels))
	for _, channelId := range channels {
		channel, ok := channelsIDM[channelId]
		if !ok {
			// 保留未知 ID，让后续一致性检查继续按旧逻辑报错。
			filtered = append(filtered, channelId)
			continue
		}
		if channel.Type != constant.ChannelTypeAdvancedCustom {
			filtered = append(filtered, channelId)
			continue
		}
		if config := channel2advancedCustomConfig[channelId]; config != nil && config.SupportsPath(requestPath) {
			filtered = append(filtered, channelId)
		}
	}
	return filtered
}

// CacheGetChannel 从缓存获取渠道对象
// 内存缓存未启用时回退到数据库查询
func CacheGetChannel(id int) (*Channel, error) {
	if !common.MemoryCacheEnabled {
		return GetChannelById(id, true)
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	c, ok := channelsIDM[id]
	if !ok {
		return nil, fmt.Errorf("渠道# %d，已不存在", id)
	}
	return c, nil
}

// CacheGetChannelInfo 从缓存获取渠道信息（ChannelInfo）
// 内存缓存未启用时回退到数据库查询
func CacheGetChannelInfo(id int) (*ChannelInfo, error) {
	if !common.MemoryCacheEnabled {
		channel, err := GetChannelById(id, true)
		if err != nil {
			return nil, err
		}
		return &channel.ChannelInfo, nil
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	c, ok := channelsIDM[id]
	if !ok {
		return nil, fmt.Errorf("渠道# %d，已不存在", id)
	}
	return &c.ChannelInfo, nil
}

// CacheUpdateChannelStatus 更新缓存中渠道的状态
// 当渠道被禁用时，同时从 group2model2channels 中移除
func CacheUpdateChannelStatus(id int, status int) {
	if !common.MemoryCacheEnabled {
		return
	}
	channelSyncLock.Lock()
	defer channelSyncLock.Unlock()
	if channel, ok := channelsIDM[id]; ok {
		channel.Status = status
	}
	if status != common.ChannelStatusEnabled {
		// 从 group2model2channels 中移除该渠道。
		for group, model2channels := range group2model2channels {
			for model, channels := range model2channels {
				filtered := channels[:0]
				for _, channelId := range channels {
					if channelId != id {
						filtered = append(filtered, channelId)
					}
				}
				group2model2channels[group][model] = filtered
			}
		}
	}
}

// CacheUpdateChannel 更新缓存中的渠道对象
func CacheUpdateChannel(channel *Channel) {
	if !common.MemoryCacheEnabled {
		return
	}
	channelSyncLock.Lock()
	defer channelSyncLock.Unlock()
	if channel == nil {
		return
	}

	println("CacheUpdateChannel:", channel.Id, channel.Name, channel.Status, channel.ChannelInfo.MultiKeyPollingIndex)

	println("before:", channelsIDM[channel.Id].ChannelInfo.MultiKeyPollingIndex)
	channelsIDM[channel.Id] = channel
	println("after :", channelsIDM[channel.Id].ChannelInfo.MultiKeyPollingIndex)
}
