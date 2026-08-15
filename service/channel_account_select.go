// channel_account_select.go 实现了渠道账号池的账号选择和调度逻辑。
// 与全局账号池（account_pool_select.go）类似，但作用于渠道级别的账号池。
// 包括账号候选过滤、优先级排序、负载均衡策略（轮询、加权随机）、
// 并发控制、游标管理等功能。
package service

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

var (
	// ErrNoAvailableChannelAccount 表示渠道账号池中没有可用的账号。
	ErrNoAvailableChannelAccount = errors.New("该渠道账号池无可用账号")

	channelAccountCursorMu sync.Mutex            // 游标映射表的互斥锁
	channelAccountCursors  = map[string]uint64{} // 轮询游标映射表

	channelAccountConcurrencyMu sync.Mutex      // 并发计数映射表的互斥锁
	channelAccountConcurrency   = map[int]int{} // 账号并发计数映射表
)

// SelectChannelAccount 从渠道账号池中选择一个可用账号。
// 选择流程与 SelectPoolAccount 类似：
// 1. 查询渠道下所有启用的账号
// 2. 过滤掉已排除的、冷却中的、不支持当前模型或分组的账号
// 3. 按优先级排序，取最高优先级的候选集
// 4. 使用负载均衡策略从候选集中选择一个账号
// 5. 尝试预留并发槽位，失败则排除并重试
//
// 参数：
//   - c: Gin 请求上下文
//   - channel: 渠道信息
//   - modelName: 请求的模型名称
//   - usingGroup: 使用的分组标识
//   - relayMode: Relay 模式（当前未使用）
//
// 返回：
//   - *model.ChannelAccount: 选中的渠道账号
//   - error: 选择失败时返回错误
func SelectChannelAccount(c *gin.Context, channel *model.Channel, modelName string, usingGroup string, relayMode int) (*model.ChannelAccount, error) {
	_ = relayMode
	if channel == nil {
		return nil, ErrNoAvailableChannelAccount
	}
	usingGroup = resolveChannelAccountUsingGroup(c, usingGroup)
	now := common.GetTimestamp()
	excluded := GetExcludedChannelAccountIds(c)

	var accounts []*model.ChannelAccount
	if err := model.DB.Where("channel_id = ? AND status = ?", channel.Id, common.ChannelStatusEnabled).Order("priority DESC").Order("id ASC").Find(&accounts).Error; err != nil {
		return nil, err
	}

	candidates := make([]*model.ChannelAccount, 0, len(accounts))
	for _, account := range accounts {
		if account == nil {
			continue
		}
		if excluded[account.Id] {
			continue
		}
		if account.Status != common.ChannelStatusEnabled || account.IsCoolingDown(now) {
			continue
		}
		if !channelAccountSupportsModel(account, channel, modelName) {
			continue
		}
		if !channelAccountSupportsGroup(account, channel, usingGroup) {
			continue
		}
		candidates = append(candidates, account)
	}
	if len(candidates) == 0 {
		return nil, ErrNoAvailableChannelAccount
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Priority == candidates[j].Priority {
			return candidates[i].Id < candidates[j].Id
		}
		return candidates[i].Priority > candidates[j].Priority
	})

	topPriority := candidates[0].Priority
	topAccounts := make([]*model.ChannelAccount, 0, len(candidates))
	for _, account := range candidates {
		if account.Priority != topPriority {
			break
		}
		topAccounts = append(topAccounts, account)
	}

	availableTopAccounts := append([]*model.ChannelAccount(nil), topAccounts...)
	for len(availableTopAccounts) > 0 {
		account := pickChannelAccount(channel, availableTopAccounts, usingGroup, modelName)
		if account == nil {
			return nil, ErrNoAvailableChannelAccount
		}
		if ReserveChannelAccount(c, account) {
			model.TouchChannelAccount(account.Id)
			return account, nil
		}
		ExcludeChannelAccountForRequest(c, account.Id)
		availableTopAccounts = removeChannelAccount(availableTopAccounts, account.Id)
	}
	return nil, ErrNoAvailableChannelAccount
}

// SelectSpecificChannelAccount 校验并选择管理员指定的渠道账号。
//
// 模型测试需要能够固定命中某个上游同步密钥，但仍必须经过和普通账号池相同的
// 模型、用户组、启停、冷却和并发校验，避免测试接口绕过真实路由约束。
func SelectSpecificChannelAccount(
	c *gin.Context,
	channel *model.Channel,
	modelName string,
	usingGroup string,
	accountID int,
	relayMode int,
) (*model.ChannelAccount, error) {
	return selectSpecificChannelAccount(c, channel, modelName, usingGroup, accountID, relayMode, false)
}

// SelectSpecificChannelAccountForTest 校验并选择管理员手动测试指定的同步密钥。
//
// 该入口只允许测试接口使用。它允许手动禁用、自动禁用或处于临时冷却中的同步密钥
// 被指定测试，以便管理员确认上游恢复后重新启用；但仍然校验渠道归属、模型白名单、
// NexusTok 访问组、完整凭据和并发限制，避免测试路径绕过账号池的基本安全边界。
func SelectSpecificChannelAccountForTest(
	c *gin.Context,
	channel *model.Channel,
	modelName string,
	usingGroup string,
	accountID int,
	relayMode int,
) (*model.ChannelAccount, error) {
	return selectSpecificChannelAccount(c, channel, modelName, usingGroup, accountID, relayMode, true)
}

func selectSpecificChannelAccount(
	c *gin.Context,
	channel *model.Channel,
	modelName string,
	usingGroup string,
	accountID int,
	relayMode int,
	allowDisabledForTest bool,
) (*model.ChannelAccount, error) {
	_ = relayMode
	if channel == nil || accountID <= 0 {
		return nil, ErrNoAvailableChannelAccount
	}
	usingGroup = resolveChannelAccountUsingGroup(c, usingGroup)

	var account model.ChannelAccount
	if err := model.DB.Where("channel_id = ? AND id = ?", channel.Id, accountID).First(&account).Error; err != nil {
		return nil, fmt.Errorf("指定的上游密钥不属于当前渠道")
	}
	if !allowDisabledForTest && account.Status != common.ChannelStatusEnabled {
		return nil, fmt.Errorf("指定的上游密钥未启用")
	}
	if !allowDisabledForTest && account.IsCoolingDown(common.GetTimestamp()) {
		return nil, fmt.Errorf("指定的上游密钥当前处于冷却或临时禁用状态")
	}
	if strings.TrimSpace(account.Key) == "" {
		return nil, fmt.Errorf("指定的上游密钥缺少完整凭据")
	}
	if channel.HasUpstreamAccountSyncMetadata() && strings.TrimSpace(account.Models) == "" {
		return nil, fmt.Errorf("指定的上游密钥未配置可路由模型")
	}
	if !channelAccountSupportsModel(&account, channel, modelName) {
		return nil, fmt.Errorf("指定的上游密钥不支持模型 %s", strings.TrimSpace(modelName))
	}
	if !channelAccountSupportsGroup(&account, channel, usingGroup) {
		return nil, fmt.Errorf("指定的上游密钥不允许当前用户组 %s", usingGroup)
	}
	if !ReserveChannelAccount(c, &account) {
		return nil, fmt.Errorf("指定的上游密钥已达到并发上限")
	}
	model.TouchChannelAccount(account.Id)
	return &account, nil
}

// ExcludeChannelAccountForRequest 将渠道账号添加到当前请求的排除列表中。
// 被排除的账号在后续重试时不会被选中。
func ExcludeChannelAccountForRequest(c *gin.Context, accountID int) {
	if c == nil || accountID <= 0 {
		return
	}
	excluded := GetExcludedChannelAccountIds(c)
	excluded[accountID] = true
	common.SetContextKey(c, constant.ContextKeyChannelAccountExcludedIds, excluded)
}

// GetExcludedChannelAccountIds 获取当前请求上下文中的排除渠道账号 ID 集合。
func GetExcludedChannelAccountIds(c *gin.Context) map[int]bool {
	if c == nil {
		return map[int]bool{}
	}
	if value, ok := common.GetContextKey(c, constant.ContextKeyChannelAccountExcludedIds); ok {
		if excluded, ok := value.(map[int]bool); ok {
			return excluded
		}
	}
	return map[int]bool{}
}

// ReserveChannelAccount 尝试为渠道账号预留并发槽位。
// 如果账号没有设置最大并发限制，直接返回成功。
func ReserveChannelAccount(c *gin.Context, account *model.ChannelAccount) bool {
	if account == nil {
		return false
	}
	if account.MaxConcurrency <= 0 {
		return true
	}
	if reserveChannelAccountConcurrency(account.Id, account.MaxConcurrency) {
		common.SetContextKey(c, constant.ContextKeyChannelAccountReserved, true)
		common.SetContextKey(c, constant.ContextKeyChannelAccountId, account.Id)
		return true
	}
	return false
}

// ReleaseSelectedChannelAccount 释放当前请求预留的渠道账号并发槽位。
func ReleaseSelectedChannelAccount(c *gin.Context) {
	if c == nil {
		return
	}
	if !common.GetContextKeyBool(c, constant.ContextKeyChannelAccountReserved) {
		return
	}
	accountID := common.GetContextKeyInt(c, constant.ContextKeyChannelAccountId)
	if accountID <= 0 {
		common.SetContextKey(c, constant.ContextKeyChannelAccountReserved, false)
		return
	}
	releaseChannelAccountConcurrency(accountID)
	common.SetContextKey(c, constant.ContextKeyChannelAccountReserved, false)
}

// pickChannelAccount 根据渠道的负载均衡模式从候选账号中选择一个。
// Random 模式使用加权随机选择，其他模式使用加权轮询选择。
func pickChannelAccount(channel *model.Channel, accounts []*model.ChannelAccount, usingGroup string, modelName string) *model.ChannelAccount {
	if len(accounts) == 0 {
		return nil
	}
	if channel.GetAccountPoolMode() == constant.ChannelAccountPoolModeRandom {
		return pickWeightedRandomChannelAccount(accounts)
	}
	cursorKey := buildChannelAccountCursorKey(channel.Id, usingGroup, modelName)
	cursor := nextChannelAccountCursor(cursorKey)
	return pickWeightedChannelAccount(accounts, cursor)
}

// pickWeightedRandomChannelAccount 使用加权随机算法从候选账号中选择一个。
// 权重越大的账号被选中的概率越高。
func pickWeightedRandomChannelAccount(accounts []*model.ChannelAccount) *model.ChannelAccount {
	totalWeight := 0
	for _, account := range accounts {
		totalWeight += account.GetWeight()
	}
	if totalWeight <= 0 {
		return accounts[0]
	}
	cursor := uint64(rand.Intn(totalWeight))
	return pickWeightedChannelAccount(accounts, cursor)
}

// pickWeightedChannelAccount 使用游标和加权算法从候选账号中选择一个。
// 计算所有账号的总权重，用游标对总权重取模得到偏移量，
// 然后遍历账号列表，累减权重直到偏移量为负。
func pickWeightedChannelAccount(accounts []*model.ChannelAccount, cursor uint64) *model.ChannelAccount {
	totalWeight := 0
	for _, account := range accounts {
		totalWeight += account.GetWeight()
	}
	if totalWeight <= 0 {
		return accounts[0]
	}
	offset := int(cursor % uint64(totalWeight))
	for _, account := range accounts {
		offset -= account.GetWeight()
		if offset < 0 {
			return account
		}
	}
	return accounts[0]
}

// buildChannelAccountCursorKey 构建渠道账号轮询游标的缓存键。
// 格式：nexustok:channel_account_pool:cursor:{channelID}:{usingGroup}:{modelName}
func buildChannelAccountCursorKey(channelID int, usingGroup string, modelName string) string {
	canonicalModel := ratio_setting.FormatMatchingModelName(strings.TrimSpace(modelName))
	if canonicalModel == "" {
		canonicalModel = strings.TrimSpace(modelName)
	}
	if strings.TrimSpace(usingGroup) == "" {
		usingGroup = "default"
	}
	return fmt.Sprintf("nexustok:channel_account_pool:cursor:%d:%s:%s", channelID, usingGroup, canonicalModel)
}

// nextChannelAccountCursor 获取并递增渠道账号轮询游标。
// 优先使用 Redis 原子递增，Redis 不可用时回退到内存计数器。
func nextChannelAccountCursor(key string) uint64 {
	if common.RedisEnabled && common.RDB != nil {
		value, err := common.RDB.Incr(context.Background(), key).Result()
		if err == nil {
			return uint64(value - 1)
		}
		common.SysLog(fmt.Sprintf("failed to update channel account cursor in redis, fallback to memory: key=%s, error=%v", key, err))
	}
	channelAccountCursorMu.Lock()
	defer channelAccountCursorMu.Unlock()
	cursor := channelAccountCursors[key]
	channelAccountCursors[key] = cursor + 1
	return cursor
}

// reserveChannelAccountConcurrency 底层渠道账号并发预留实现。
// 优先使用 Redis 原子操作，Redis 不可用时回退到内存计数器。
func reserveChannelAccountConcurrency(accountID int, maxConcurrency int) bool {
	if accountID <= 0 || maxConcurrency <= 0 {
		return true
	}
	if common.RedisEnabled && common.RDB != nil {
		key := fmt.Sprintf("nexustok:channel_account_pool:concurrency:%d", accountID)
		value, err := common.RDB.Incr(context.Background(), key).Result()
		if err == nil {
			if value == 1 {
				_ = common.RDB.Expire(context.Background(), key, 10*time.Minute).Err()
			}
			if value <= int64(maxConcurrency) {
				return true
			}
			_ = common.RDB.Decr(context.Background(), key).Err()
			return false
		}
		common.SysLog(fmt.Sprintf("failed to reserve channel account concurrency in redis, fallback to memory: account_id=%d, error=%v", accountID, err))
	}
	channelAccountConcurrencyMu.Lock()
	defer channelAccountConcurrencyMu.Unlock()
	if channelAccountConcurrency[accountID] >= maxConcurrency {
		return false
	}
	channelAccountConcurrency[accountID]++
	return true
}

// releaseChannelAccountConcurrency 释放渠道账号的并发槽位。
func releaseChannelAccountConcurrency(accountID int) {
	if accountID <= 0 {
		return
	}
	if common.RedisEnabled && common.RDB != nil {
		key := fmt.Sprintf("nexustok:channel_account_pool:concurrency:%d", accountID)
		if _, err := common.RDB.Decr(context.Background(), key).Result(); err == nil {
			return
		}
	}
	channelAccountConcurrencyMu.Lock()
	defer channelAccountConcurrencyMu.Unlock()
	if channelAccountConcurrency[accountID] <= 1 {
		delete(channelAccountConcurrency, accountID)
		return
	}
	channelAccountConcurrency[accountID]--
}

// resolveChannelAccountUsingGroup 解析当前请求使用的分组标识。
// 优先级：上下文中的自动分组 > 参数指定的分组 > 上下文中的使用分组。
func resolveChannelAccountUsingGroup(c *gin.Context, usingGroup string) string {
	if c != nil {
		if autoGroup := common.GetContextKeyString(c, constant.ContextKeyAutoGroup); autoGroup != "" {
			return autoGroup
		}
		if usingGroup == "" {
			usingGroup = common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
		}
	}
	return strings.TrimSpace(usingGroup)
}

// channelAccountSupportsModel 检查渠道账号是否支持指定的模型。
//
// 普通账号池继续兼容旧语义：账号模型为空时回退渠道模型，最终仍为空表示不限模型。
// 上游同步账号的模型列表是管理员确认后的显式白名单，空值表示该同步密钥暂不参与
// 任何模型路由，不能再回退到渠道聚合模型，否则“清空模型”会被错误解释成“支持全部”。
func channelAccountSupportsModel(account *model.ChannelAccount, channel *model.Channel, modelName string) bool {
	models := account.Models
	if channel != nil && channel.HasUpstreamAccountSyncMetadata() {
		if strings.TrimSpace(models) == "" {
			return false
		}
	} else if strings.TrimSpace(models) == "" && channel != nil {
		models = channel.Models
	}
	modelList := model.SplitCommaValues(models)
	if len(modelList) == 0 {
		return true
	}
	return model.MatchesModelList(modelList, modelName)
}

// channelAccountSupportsGroup 检查渠道账号是否属于指定的使用分组。
//
// 历史账号池渠道中，ChannelAccount.group 表示 NexusTok 用户分组，可以优先于渠道
// 分组做精细过滤。上游账号同步渠道中，同一字段改为上游密钥分组，只用于展示和成本
// 区分；此时必须只按 Channel.group 判断用户可用分组，否则下游用户会被上游 key 的
// 分组名称错误拦截，导致同步渠道无法按渠道分组正常路由。
func channelAccountSupportsGroup(account *model.ChannelAccount, channel *model.Channel, usingGroup string) bool {
	if strings.TrimSpace(usingGroup) == "" {
		return true
	}
	group := ""
	if channel != nil && channel.HasUpstreamAccountSyncMetadata() {
		// 上游同步账号的 Group 是平台内部的密钥分组，不能当作 NexusTok
		// 下游用户组；空 access_groups 代表该密钥不允许任何用户组。
		if account == nil {
			return false
		}
		if strings.TrimSpace(account.AccessGroups) == "" {
			return false
		}
		group = account.AccessGroups
	} else {
		group = account.Group
		if strings.TrimSpace(group) == "" && channel != nil {
			group = channel.Group
		}
	}
	groups := model.SplitCommaValues(group)
	if len(groups) == 0 {
		return true
	}
	for _, candidate := range groups {
		if candidate == "*" || candidate == usingGroup {
			return true
		}
	}
	return false
}

// removeChannelAccount 从渠道账号列表中移除指定 ID 的账号。
// 返回移除后的新切片（不修改原切片）。
func removeChannelAccount(accounts []*model.ChannelAccount, accountID int) []*model.ChannelAccount {
	for i, account := range accounts {
		if account != nil && account.Id == accountID {
			return append(accounts[:i], accounts[i+1:]...)
		}
	}
	return accounts
}
