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
var group2model2channels map[string]map[string][]int // enabled channel

// channelsIDM 渠道 ID -> 渠道对象的映射（包含所有渠道，含禁用的）
var channelsIDM map[int]*Channel // all channels include disabled

// channel2advancedCustomConfig 缓存 type 58 渠道解析后的 Advanced Custom 配置。
// 选路热路径只需要判断 request path 是否命中 route，缓存配置可以避免每次请求重复解析 JSON。
var channel2advancedCustomConfig map[int]*dto.AdvancedCustomConfig

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
	var abilities []*Ability
	DB.Find(&abilities)
	groups := make(map[string]bool)
	for _, ability := range abilities {
		groups[ability.Group] = true
	}
	newGroup2model2channels := make(map[string]map[string][]int)
	for group := range groups {
		newGroup2model2channels[group] = make(map[string][]int)
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
				if _, ok := newGroup2model2channels[group][model]; !ok {
					newGroup2model2channels[group][model] = make([]int, 0)
				}
				newGroup2model2channels[group][model] = append(newGroup2model2channels[group][model], channel.Id)
			}
		}
	}

	// sort by priority
	for group, model2channels := range newGroup2model2channels {
		for model, channels := range model2channels {
			sort.Slice(channels, func(i, j int) bool {
				return newChannelId2channel[channels[i]].GetPriority() > newChannelId2channel[channels[j]].GetPriority()
			})
			newGroup2model2channels[group][model] = channels
		}
	}

	channelSyncLock.Lock()
	group2model2channels = newGroup2model2channels
	//channelsIDM = newChannelId2channel
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
	channelSyncLock.Unlock()
	common.SysLog("channels synced from database")
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
	// if memory cache is disabled, get channel directly from database
	if !common.MemoryCacheEnabled {
		return GetChannelWithExclusions(group, model, retry, requestPath, excludedChannelIds)
	}

	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	// First, try to find channels with the exact model name.
	excludedSet := intSliceToSet(excludedChannelIds)

	channels := filterCandidateChannels(group2model2channels[group][model], requestPath, excludedSet)

	// If no channels found, try to find channels with the normalized model name.
	if len(channels) == 0 {
		normalizedModel := ratio_setting.FormatMatchingModelName(model)
		channels = filterCandidateChannels(group2model2channels[group][normalizedModel], requestPath, excludedSet)
	}

	if len(channels) == 0 {
		return nil, nil
	}

	if len(channels) == 1 {
		if channel, ok := channelsIDM[channels[0]]; ok {
			return channel, nil
		}
		return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channels[0])
	}

	uniquePriorities := make(map[int]bool)
	for _, channelId := range channels {
		if channel, ok := channelsIDM[channelId]; ok {
			uniquePriorities[int(channel.GetPriority())] = true
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

	// get the priority for the given retry number
	var sumWeight = 0
	var targetChannels []*Channel
	for _, channelId := range channels {
		if channel, ok := channelsIDM[channelId]; ok {
			if channel.GetPriority() == targetPriority {
				sumWeight += channel.GetWeight()
				targetChannels = append(targetChannels, channel)
			}
		} else {
			return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channelId)
		}
	}

	if len(targetChannels) == 0 {
		return nil, errors.New(fmt.Sprintf("no channel found, group: %s, model: %s, priority: %d", group, model, targetPriority))
	}

	// smoothing factor and adjustment
	smoothingFactor := 1
	smoothingAdjustment := 0

	if sumWeight == 0 {
		// when all channels have weight 0, set sumWeight to the number of channels and set smoothing adjustment to 100
		// each channel's effective weight = 100
		sumWeight = len(targetChannels) * 100
		smoothingAdjustment = 100
	} else if sumWeight/len(targetChannels) < 10 {
		// when the average weight is less than 10, set smoothing factor to 100
		smoothingFactor = 100
	}

	// Calculate the total weight of all channels up to endIdx
	totalWeight := sumWeight * smoothingFactor

	// Generate a random value in the range [0, totalWeight)
	randomWeight := rand.Intn(totalWeight)

	// Find a channel based on its weight
	for _, channel := range targetChannels {
		randomWeight -= channel.GetWeight()*smoothingFactor + smoothingAdjustment
		if randomWeight < 0 {
			return channel, nil
		}
	}
	// return null if no channel is not found
	return nil, errors.New("channel not found")
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
		// delete the channel from group2model2channels
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
