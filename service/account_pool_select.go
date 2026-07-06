// account_pool_select.go 实现了全局账号池的账号选择和调度逻辑。
// 包括账号候选过滤、优先级排序、负载均衡策略（轮询、最少使用、先填满、成功率优先、加权随机）、
// 并发控制（基于 Redis 或内存的信号量）、游标管理等功能。
// 是账号池请求调度的核心模块。
package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/service/accountauth"
	"github.com/c1cada/NexusTok/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

var (
	// ErrNoAvailablePoolAccount 表示账号池中没有可用的账号。
	ErrNoAvailablePoolAccount = errors.New("账号池无可用账号")
	// ErrPoolAccountGroupConcurrencyExceeded 表示账号池分组级并发槽位已满。
	ErrPoolAccountGroupConcurrencyExceeded = errors.New("账号池组并发已满")
	// ErrPoolAccountGroupRateLimitExceeded 表示账号池分组每分钟请求频率已达到上限。
	ErrPoolAccountGroupRateLimitExceeded = errors.New("账号池分组请求频率已达到上限")
	// ErrPoolAccountRateLimitExceeded 表示单个账号每分钟请求频率已达到上限。
	ErrPoolAccountRateLimitExceeded = errors.New("账号池账号请求频率已达到上限")

	poolAccountCursorMu sync.Mutex            // 游标映射表的互斥锁
	poolAccountCursors  = map[string]uint64{} // 轮询游标映射表，key 为分组+模型组合

	poolAccountConcurrencyMu sync.Mutex      // 并发计数映射表的互斥锁
	poolAccountConcurrency   = map[int]int{} // 账号并发计数映射表，key 为账号 ID

	poolAccountRateMu       sync.Mutex                       // 账号 RPM 窗口映射表的互斥锁
	poolAccountRateCounters = map[int]poolGroupRateCounter{} // 账号 RPM 窗口计数，key 为账号 ID

	poolGroupConcurrencyMu sync.Mutex      // 分组并发计数映射表的互斥锁
	poolGroupConcurrency   = map[int]int{} // 分组并发计数映射表，key 为分组 ID

	poolGroupRateMu       sync.Mutex                       // 分组 RPM 窗口映射表的互斥锁
	poolGroupRateCounters = map[int]poolGroupRateCounter{} // 分组 RPM 窗口计数，key 为分组 ID
)

type poolGroupRateCounter struct {
	window int64
	count  int
}

// SelectPoolAccount 从渠道引用的全局账号池组里选择一个可调度账号。
// 选择流程：
// 1. 获取渠道关联的账号池分组
// 2. 查询分组下所有启用且可调度的账号
// 3. 过滤掉已排除的、冷却中的、不支持当前模型或分组的账号
// 4. 按优先级排序，取最高优先级的候选集
// 5. 使用负载均衡策略从候选集中选择一个账号
// 6. 尝试预留并发槽位，失败则排除并重试
//
// 参数：
//   - c: Gin 请求上下文（用于存储排除列表和并发预留状态）
//   - channel: 渠道信息
//   - modelName: 请求的模型名称
//   - usingGroup: 使用的分组标识
//   - relayMode: Relay 模式（当前未使用）
//
// 返回：
//   - *model.AccountPoolGroup: 账号池分组信息
//   - *model.PoolAccount: 选中的账号
//   - error: 选择失败时返回错误
func SelectPoolAccount(c *gin.Context, channel *model.Channel, modelName string, usingGroup string, relayMode int) (*model.AccountPoolGroup, *model.PoolAccount, error) {
	_ = relayMode
	if channel == nil || channel.ChannelInfo.AccountPoolGroupId <= 0 {
		return nil, nil, ErrNoAvailablePoolAccount
	}
	group, err := model.GetAccountPoolGroupById(channel.ChannelInfo.AccountPoolGroupId)
	if err != nil {
		return nil, nil, err
	}
	if group == nil || group.Status != common.ChannelStatusEnabled {
		return group, nil, ErrNoAvailablePoolAccount
	}
	nowTime := time.Now()
	if reset, err := model.ResetAccountPoolGroupDailyUsageIfNeeded(group.Id, nowTime); err != nil {
		return group, nil, err
	} else if reset {
		group.DailyRequestCount = 0
		group.DailyUsedQuota = 0
		group.DailyResetTime = model.AccountPoolDailyWindowStart(nowTime)
	}
	if group.DailyRequestLimit > 0 && group.DailyRequestCount >= group.DailyRequestLimit {
		return group, nil, model.ErrAccountPoolGroupDailyRequestLimitExceeded
	}
	if group.DailyQuotaLimit > 0 && group.DailyUsedQuota >= group.DailyQuotaLimit {
		return group, nil, model.ErrAccountPoolGroupDailyQuotaLimitExceeded
	}

	usingGroup = resolveChannelAccountUsingGroup(c, usingGroup)
	now := common.GetTimestamp()
	excluded := GetExcludedPoolAccountIds(c)

	var accounts []*model.PoolAccount
	if err := model.DB.Where("pool_group_id = ? AND status = ? AND schedulable = ?", group.Id, common.ChannelStatusEnabled, true).
		Order("priority DESC").Order("id ASC").Find(&accounts).Error; err != nil {
		return group, nil, err
	}

	candidates := make([]*model.PoolAccount, 0, len(accounts))
	var lastAccountLimitErr error
	for _, account := range accounts {
		if account == nil {
			continue
		}
		if excluded[account.Id] {
			continue
		}
		if account.Status != common.ChannelStatusEnabled || !account.Schedulable {
			continue
		}
		if err := model.PoolAccountDailyLimitError(account, nowTime); err != nil {
			if markErr := model.MarkPoolAccountDailyLimitExceeded(account.Id, err, nowTime); markErr != nil {
				common.SysLog(fmt.Sprintf("failed to mark pool account daily limit exceeded: account_id=%d, error=%v", account.Id, markErr))
			}
			lastAccountLimitErr = err
			continue
		}
		if account.Unavailable || account.IsCoolingDown(now) {
			continue
		}
		if !poolAccountSupportsModel(account, group, modelName) {
			continue
		}
		if !poolAccountSupportsGroup(account, group, usingGroup) {
			continue
		}
		candidates = append(candidates, account)
	}
	if len(candidates) == 0 {
		if lastAccountLimitErr != nil {
			return group, nil, lastAccountLimitErr
		}
		return group, nil, ErrNoAvailablePoolAccount
	}
	groupReserved := false
	if group.GetMaxConcurrency() > 0 {
		if !reservePoolGroupForRequest(c, group) {
			return group, nil, ErrPoolAccountGroupConcurrencyExceeded
		}
		groupReserved = true
	}
	releaseGroupOnFailure := true
	defer func() {
		if groupReserved && releaseGroupOnFailure {
			releaseReservedPoolGroup(c)
		}
	}()

	sortPoolAccountCandidates(candidates, group.Strategy)
	topPriority := candidates[0].Priority
	topAccounts := make([]*model.PoolAccount, 0, len(candidates))
	for _, account := range candidates {
		if account.Priority != topPriority {
			break
		}
		topAccounts = append(topAccounts, account)
	}

	availableTopAccounts := append([]*model.PoolAccount(nil), topAccounts...)
	var lastAccountReserveErr error
	for len(availableTopAccounts) > 0 {
		account := pickPoolAccount(group, availableTopAccounts, usingGroup, modelName)
		if account == nil {
			if lastAccountReserveErr != nil {
				return group, nil, lastAccountReserveErr
			}
			return group, nil, ErrNoAvailablePoolAccount
		}
		if ReservePoolAccount(c, account) {
			if err := reservePoolAccountUsageLimit(account); err != nil {
				releaseReservedPoolAccount(c)
				ExcludePoolAccountForRequest(c, account.Id)
				availableTopAccounts = removePoolAccount(availableTopAccounts, account.Id)
				lastAccountReserveErr = err
				continue
			}
			if err := reservePoolGroupUsageLimit(group); err != nil {
				releasePoolAccountUsageReservation(account)
				ReleaseSelectedPoolAccount(c)
				return group, nil, err
			}
			model.TouchPoolAccount(account.Id)
			releaseGroupOnFailure = false
			return group, account, nil
		}
		ExcludePoolAccountForRequest(c, account.Id)
		availableTopAccounts = removePoolAccount(availableTopAccounts, account.Id)
	}
	if lastAccountReserveErr != nil {
		return group, nil, lastAccountReserveErr
	}
	return group, nil, ErrNoAvailablePoolAccount
}

// sortPoolAccountCandidates 对账号池候选账号进行排序。
// 排序规则：
//  1. 始终先按账号优先级降序排列，确保管理员显式配置的优先级高于策略偏好。
//  2. least_used 策略在同优先级内按已用配额升序排列。
//  3. success_rate 策略在同优先级内按带先验的成功率降序排列，避免新账号因样本不足被极端化处理。
//  4. 其它策略保持 ID 升序，供轮询和加权策略获得稳定候选顺序。
func sortPoolAccountCandidates(accounts []*model.PoolAccount, strategy string) {
	strategy = strings.TrimSpace(strategy)
	sort.SliceStable(accounts, func(i, j int) bool {
		if accounts[i].Priority != accounts[j].Priority {
			return accounts[i].Priority > accounts[j].Priority
		}
		if strategy == model.AccountPoolStrategyLeastUsed && accounts[i].UsedQuota != accounts[j].UsedQuota {
			return accounts[i].UsedQuota < accounts[j].UsedQuota
		}
		if strategy == model.AccountPoolStrategySuccessRate {
			leftScore := poolAccountSuccessRateScore(accounts[i])
			rightScore := poolAccountSuccessRateScore(accounts[j])
			if leftScore != rightScore {
				return leftScore > rightScore
			}
			leftSuccess := nonNegativePoolAccountCount(accounts[i].SuccessCount)
			rightSuccess := nonNegativePoolAccountCount(accounts[j].SuccessCount)
			if leftSuccess != rightSuccess {
				return leftSuccess > rightSuccess
			}
			leftFailed := nonNegativePoolAccountCount(accounts[i].FailedCount)
			rightFailed := nonNegativePoolAccountCount(accounts[j].FailedCount)
			if leftFailed != rightFailed {
				return leftFailed < rightFailed
			}
		}
		return accounts[i].Id < accounts[j].Id
	})
}

// poolAccountSuccessRateScore 返回账号成功率优先策略使用的带先验评分。
// 使用 (success + 1) / (success + failed + 2) 可以把没有历史请求的新账号视为 50% 的中性账号，
// 避免 0 成功/0 失败被误判为 0% 或 100%，也能在样本较少时降低偶然成功或失败的影响。
func poolAccountSuccessRateScore(account *model.PoolAccount) float64 {
	if account == nil {
		return 0
	}
	success := float64(nonNegativePoolAccountCount(account.SuccessCount))
	failed := float64(nonNegativePoolAccountCount(account.FailedCount))
	return (success + 1) / (success + failed + 2)
}

// nonNegativePoolAccountCount 兜底清理历史异常计数。
// 正常路径只会累加成功和失败次数；这里保留防御逻辑，避免人工修库产生负数后影响排序。
func nonNegativePoolAccountCount(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
}

// pickPoolAccount 根据分组的负载均衡策略从候选账号中选择一个。
// 支持的策略：
// - fill_first: 先填满，直接返回第一个账号
// - least_used: 最少使用，直接返回第一个（已按配额排序）
// - success_rate: 成功率优先，直接返回第一个（已按成功率排序）
// - round_robin: 轮询，使用游标循环选择
// - weighted: 加权轮询，根据账号权重分配
func pickPoolAccount(group *model.AccountPoolGroup, accounts []*model.PoolAccount, usingGroup string, modelName string) *model.PoolAccount {
	if len(accounts) == 0 {
		return nil
	}
	strategy := model.AccountPoolStrategyRoundRobin
	groupID := 0
	if group != nil {
		strategy = strings.TrimSpace(group.Strategy)
		groupID = group.Id
	}
	switch strategy {
	case model.AccountPoolStrategyFillFirst:
		return accounts[0]
	case model.AccountPoolStrategyLeastUsed, model.AccountPoolStrategySuccessRate:
		return accounts[0]
	}
	cursorKey := buildPoolAccountCursorKey(groupID, usingGroup, modelName)
	cursor := nextPoolAccountCursor(cursorKey)
	return pickPoolAccountByCursor(accounts, cursor, strategy == model.AccountPoolStrategyWeighted)
}

// pickPoolAccountByCursor 使用游标和加权算法从候选账号中选择一个。
// 计算所有账号的总权重，用游标对总权重取模得到偏移量，
// 然后遍历账号列表，累减权重直到偏移量为负，返回对应账号。
func pickPoolAccountByCursor(accounts []*model.PoolAccount, cursor uint64, weighted bool) *model.PoolAccount {
	totalWeight := 0
	for _, account := range accounts {
		if weighted {
			totalWeight += account.GetWeight()
		} else {
			totalWeight++
		}
	}
	if totalWeight <= 0 {
		return accounts[0]
	}
	offset := int(cursor % uint64(totalWeight))
	for _, account := range accounts {
		weight := 1
		if weighted {
			weight = account.GetWeight()
		}
		offset -= weight
		if offset < 0 {
			return account
		}
	}
	return accounts[0]
}

// buildPoolAccountCursorKey 构建轮询游标的缓存键。
// 格式：nexustok:account_pool:cursor:{groupID}:{usingGroup}:{modelName}
func buildPoolAccountCursorKey(groupID int, usingGroup string, modelName string) string {
	canonicalModel := ratio_setting.FormatMatchingModelName(strings.TrimSpace(modelName))
	if canonicalModel == "" {
		canonicalModel = strings.TrimSpace(modelName)
	}
	if strings.TrimSpace(usingGroup) == "" {
		usingGroup = "default"
	}
	return fmt.Sprintf("nexustok:account_pool:cursor:%d:%s:%s", groupID, usingGroup, canonicalModel)
}

// nextPoolAccountCursor 获取并递增轮询游标。
// 优先使用 Redis 原子递增，Redis 不可用时回退到内存计数器。
func nextPoolAccountCursor(key string) uint64 {
	if common.RedisEnabled && common.RDB != nil {
		value, err := common.RDB.Incr(context.Background(), key).Result()
		if err == nil {
			return uint64(value - 1)
		}
		common.SysLog(fmt.Sprintf("failed to update pool account cursor in redis, fallback to memory: key=%s, error=%v", key, err))
	}
	poolAccountCursorMu.Lock()
	defer poolAccountCursorMu.Unlock()
	cursor := poolAccountCursors[key]
	poolAccountCursors[key] = cursor + 1
	return cursor
}

// ExcludePoolAccountForRequest 将账号添加到当前请求的排除列表中。
// 被排除的账号在后续重试时不会被选中。
func ExcludePoolAccountForRequest(c *gin.Context, accountID int) {
	if c == nil || accountID <= 0 {
		return
	}
	excluded := GetExcludedPoolAccountIds(c)
	excluded[accountID] = true
	common.SetContextKey(c, constant.ContextKeyPoolAccountExcludedIds, excluded)
}

// GetExcludedPoolAccountIds 获取当前请求上下文中的排除账号 ID 集合。
func GetExcludedPoolAccountIds(c *gin.Context) map[int]bool {
	if c == nil {
		return map[int]bool{}
	}
	if value, ok := common.GetContextKey(c, constant.ContextKeyPoolAccountExcludedIds); ok {
		if excluded, ok := value.(map[int]bool); ok {
			return excluded
		}
	}
	return map[int]bool{}
}

// ReservePoolAccount 尝试为账号预留并发槽位。
// 如果账号没有设置最大并发限制（MaxConcurrency <= 0），直接返回成功。
// 否则通过信号量机制检查并预留一个并发槽位。
func ReservePoolAccount(c *gin.Context, account *model.PoolAccount) bool {
	if account == nil {
		return false
	}
	if account.MaxConcurrency <= 0 {
		return true
	}
	if reservePoolAccountConcurrency(account.Id, account.MaxConcurrency) {
		common.SetContextKey(c, constant.ContextKeyPoolAccountReserved, true)
		common.SetContextKey(c, constant.ContextKeyPoolAccountId, account.Id)
		return true
	}
	return false
}

// reservePoolAccountUsageLimit 在账号并发槽位已经预留后检查并预占账号级请求额度。
// 账号级 RPM、每日请求数和每日配额都只约束单个账号；如果当前账号超限，
// 调度层会排除它并继续尝试同组内其它候选账号。
func reservePoolAccountUsageLimit(account *model.PoolAccount) error {
	if account == nil {
		return nil
	}
	if err := model.CheckPoolAccountDailyQuotaLimit(account.Id); err != nil {
		if markErr := model.MarkPoolAccountDailyLimitExceeded(account.Id, err, time.Now()); markErr != nil {
			common.SysLog(fmt.Sprintf("failed to mark pool account daily quota exceeded: account_id=%d, error=%v", account.Id, markErr))
		}
		return err
	}
	if !reservePoolAccountRateLimit(account.Id, account.RateLimitRpm) {
		return ErrPoolAccountRateLimitExceeded
	}
	if err := model.ReservePoolAccountRequest(account.Id); err != nil {
		releasePoolAccountRateLimit(account.Id, account.RateLimitRpm)
		if markErr := model.MarkPoolAccountDailyLimitExceeded(account.Id, err, time.Now()); markErr != nil {
			common.SysLog(fmt.Sprintf("failed to mark pool account daily request exceeded: account_id=%d, error=%v", account.Id, markErr))
		}
		return err
	}
	return nil
}

// releasePoolAccountUsageReservation 回滚还没有进入上游调用的账号级用量预占。
// 仅在账号已成功预占、但后续分组级限制拒绝请求时调用；请求进入上游后不回滚。
func releasePoolAccountUsageReservation(account *model.PoolAccount) {
	if account == nil {
		return
	}
	model.ReleasePoolAccountRequest(account.Id)
	releasePoolAccountRateLimit(account.Id, account.RateLimitRpm)
}

// reservePoolAccountRateLimit 按自然分钟预占单账号请求频率额度。
// Redis 可用时使用分钟窗口 key 做原子计数；没有 Redis 时回退到进程内计数器。
func reservePoolAccountRateLimit(accountID int, rpm int) bool {
	if accountID <= 0 || rpm <= 0 {
		return true
	}
	window := time.Now().Unix() / 60
	if common.RedisEnabled && common.RDB != nil {
		key := fmt.Sprintf("nexustok:account_pool:account_rate:%d:%d", accountID, window)
		value, err := common.RDB.Incr(context.Background(), key).Result()
		if err == nil {
			if value == 1 {
				_ = common.RDB.Expire(context.Background(), key, 2*time.Minute).Err()
			}
			if value <= int64(rpm) {
				return true
			}
			_ = common.RDB.Decr(context.Background(), key).Err()
			return false
		}
		common.SysLog(fmt.Sprintf("failed to reserve pool account rate limit in redis, fallback to memory: account_id=%d, error=%v", accountID, err))
	}
	poolAccountRateMu.Lock()
	defer poolAccountRateMu.Unlock()
	counter := poolAccountRateCounters[accountID]
	if counter.window != window {
		counter = poolGroupRateCounter{window: window}
	}
	if counter.count >= rpm {
		poolAccountRateCounters[accountID] = counter
		return false
	}
	counter.count++
	poolAccountRateCounters[accountID] = counter
	return true
}

// releasePoolAccountRateLimit 回滚本请求刚预占的单账号 RPM 计数。
func releasePoolAccountRateLimit(accountID int, rpm int) {
	if accountID <= 0 || rpm <= 0 {
		return
	}
	window := time.Now().Unix() / 60
	if common.RedisEnabled && common.RDB != nil {
		key := fmt.Sprintf("nexustok:account_pool:account_rate:%d:%d", accountID, window)
		if _, err := common.RDB.Decr(context.Background(), key).Result(); err == nil {
			return
		}
	}
	poolAccountRateMu.Lock()
	defer poolAccountRateMu.Unlock()
	counter := poolAccountRateCounters[accountID]
	if counter.window != window || counter.count <= 1 {
		delete(poolAccountRateCounters, accountID)
		return
	}
	counter.count--
	poolAccountRateCounters[accountID] = counter
}

// reservePoolGroupForRequest 尝试为账号池分组预留一个并发槽位。
// 分组级限制作用于整个组内所有账号，用于控制一组账号同时被 Relay 占用的总量。
// 成功预留后会把分组 ID 和保留标记写入请求上下文，确保请求成功、失败或重试时能统一释放。
func reservePoolGroupForRequest(c *gin.Context, group *model.AccountPoolGroup) bool {
	if group == nil {
		return false
	}
	maxConcurrency := group.GetMaxConcurrency()
	if maxConcurrency <= 0 {
		return true
	}
	if c == nil {
		return false
	}
	if reservePoolGroupConcurrency(group.Id, maxConcurrency) {
		common.SetContextKey(c, constant.ContextKeyPoolGroupReserved, true)
		common.SetContextKey(c, constant.ContextKeyPoolGroupId, group.Id)
		return true
	}
	return false
}

// reservePoolGroupUsageLimit 在账号已经预留成功后检查并预占分组级请求额度。
// 调用时机放在账号级并发预留之后：只有真实拿到账号的请求才会消耗每日请求次数；
// 如果每日请求数已满，调用方会释放刚预留的账号和分组并发槽位。
func reservePoolGroupUsageLimit(group *model.AccountPoolGroup) error {
	if group == nil {
		return nil
	}
	if err := model.CheckAccountPoolGroupDailyQuotaLimit(group.Id); err != nil {
		return err
	}
	if !reservePoolGroupRateLimit(group.Id, group.RateLimitRpm) {
		return ErrPoolAccountGroupRateLimitExceeded
	}
	if err := model.ReserveAccountPoolGroupRequest(group.Id); err != nil {
		releasePoolGroupRateLimit(group.Id, group.RateLimitRpm)
		return err
	}
	return nil
}

// reservePoolGroupRateLimit 按自然分钟预占账号池分组的请求频率额度。
// Redis 可用时使用分钟窗口 key 做原子计数；没有 Redis 时回退到进程内计数器。
// 该限制只在请求已经拿到具体账号后消耗，避免无可用账号的尝试污染 RPM 统计。
func reservePoolGroupRateLimit(groupID int, rpm int) bool {
	if groupID <= 0 || rpm <= 0 {
		return true
	}
	window := time.Now().Unix() / 60
	if common.RedisEnabled && common.RDB != nil {
		key := fmt.Sprintf("nexustok:account_pool:group_rate:%d:%d", groupID, window)
		value, err := common.RDB.Incr(context.Background(), key).Result()
		if err == nil {
			if value == 1 {
				_ = common.RDB.Expire(context.Background(), key, 2*time.Minute).Err()
			}
			if value <= int64(rpm) {
				return true
			}
			_ = common.RDB.Decr(context.Background(), key).Err()
			return false
		}
		common.SysLog(fmt.Sprintf("failed to reserve pool group rate limit in redis, fallback to memory: group_id=%d, error=%v", groupID, err))
	}
	poolGroupRateMu.Lock()
	defer poolGroupRateMu.Unlock()
	counter := poolGroupRateCounters[groupID]
	if counter.window != window {
		counter = poolGroupRateCounter{window: window}
	}
	if counter.count >= rpm {
		poolGroupRateCounters[groupID] = counter
		return false
	}
	counter.count++
	poolGroupRateCounters[groupID] = counter
	return true
}

// releasePoolGroupRateLimit 回滚本请求刚预占的账号池分组 RPM 计数。
// 只有在 RPM 预占已经成功、但后续每日请求额度等检查失败时调用；
// 正常完成的请求不释放 RPM，因为频率限制统计的是一分钟内实际进入热路径的请求量。
func releasePoolGroupRateLimit(groupID int, rpm int) {
	if groupID <= 0 || rpm <= 0 {
		return
	}
	window := time.Now().Unix() / 60
	if common.RedisEnabled && common.RDB != nil {
		key := fmt.Sprintf("nexustok:account_pool:group_rate:%d:%d", groupID, window)
		if _, err := common.RDB.Decr(context.Background(), key).Result(); err == nil {
			return
		}
	}
	poolGroupRateMu.Lock()
	defer poolGroupRateMu.Unlock()
	counter := poolGroupRateCounters[groupID]
	if counter.window != window || counter.count <= 1 {
		delete(poolGroupRateCounters, groupID)
		return
	}
	counter.count--
	poolGroupRateCounters[groupID] = counter
}

// ReleaseSelectedPoolAccount 释放当前请求预留的账号和分组并发槽位。
// 在请求完成（成功或失败）后调用，恢复账号和分组的可用并发数。
func ReleaseSelectedPoolAccount(c *gin.Context) {
	if c == nil {
		return
	}
	releaseReservedPoolAccount(c)
	releaseReservedPoolGroup(c)
}

// releaseReservedPoolAccount 释放当前请求预留的账号并发槽位。
func releaseReservedPoolAccount(c *gin.Context) {
	if c == nil {
		return
	}
	if common.GetContextKeyBool(c, constant.ContextKeyPoolAccountReserved) {
		accountID := common.GetContextKeyInt(c, constant.ContextKeyPoolAccountId)
		if accountID > 0 {
			releasePoolAccountConcurrency(accountID)
		}
		common.SetContextKey(c, constant.ContextKeyPoolAccountReserved, false)
		common.SetContextKey(c, constant.ContextKeyPoolAccountId, 0)
	}
}

// releaseReservedPoolGroup 释放当前请求预留的账号池分组并发槽位。
// 该函数既用于请求结束释放，也用于“分组已占用但账号级并发全部失败”的回滚路径。
func releaseReservedPoolGroup(c *gin.Context) {
	if c == nil || !common.GetContextKeyBool(c, constant.ContextKeyPoolGroupReserved) {
		return
	}
	groupID := common.GetContextKeyInt(c, constant.ContextKeyPoolGroupId)
	if groupID > 0 {
		releasePoolGroupConcurrency(groupID)
	}
	common.SetContextKey(c, constant.ContextKeyPoolGroupReserved, false)
}

// reservePoolAccountConcurrency 底层并发预留实现。
// 优先使用 Redis 原子操作，Redis 不可用时回退到内存计数器。
// 使用 INCR + EXPIRE 实现带自动过期的计数器。
func reservePoolAccountConcurrency(accountID int, maxConcurrency int) bool {
	if accountID <= 0 || maxConcurrency <= 0 {
		return true
	}
	if common.RedisEnabled && common.RDB != nil {
		key := fmt.Sprintf("nexustok:account_pool:concurrency:%d", accountID)
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
		common.SysLog(fmt.Sprintf("failed to reserve pool account concurrency in redis, fallback to memory: account_id=%d, error=%v", accountID, err))
	}
	poolAccountConcurrencyMu.Lock()
	defer poolAccountConcurrencyMu.Unlock()
	if poolAccountConcurrency[accountID] >= maxConcurrency {
		return false
	}
	poolAccountConcurrency[accountID]++
	return true
}

// releasePoolAccountConcurrency 释放账号的并发槽位。
// 优先使用 Redis DECR，Redis 不可用时回退到内存计数器。
func releasePoolAccountConcurrency(accountID int) {
	if accountID <= 0 {
		return
	}
	if common.RedisEnabled && common.RDB != nil {
		key := fmt.Sprintf("nexustok:account_pool:concurrency:%d", accountID)
		if _, err := common.RDB.Decr(context.Background(), key).Result(); err == nil {
			return
		}
	}
	poolAccountConcurrencyMu.Lock()
	defer poolAccountConcurrencyMu.Unlock()
	if poolAccountConcurrency[accountID] <= 1 {
		delete(poolAccountConcurrency, accountID)
		return
	}
	poolAccountConcurrency[accountID]--
}

// reservePoolGroupConcurrency 底层分组并发预留实现。
// Redis 可用时使用带 TTL 的原子计数器；Redis 不可用时回退到进程内计数器。
func reservePoolGroupConcurrency(groupID int, maxConcurrency int) bool {
	if groupID <= 0 || maxConcurrency <= 0 {
		return true
	}
	if common.RedisEnabled && common.RDB != nil {
		key := fmt.Sprintf("nexustok:account_pool:group_concurrency:%d", groupID)
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
		common.SysLog(fmt.Sprintf("failed to reserve pool group concurrency in redis, fallback to memory: group_id=%d, error=%v", groupID, err))
	}
	poolGroupConcurrencyMu.Lock()
	defer poolGroupConcurrencyMu.Unlock()
	if poolGroupConcurrency[groupID] >= maxConcurrency {
		return false
	}
	poolGroupConcurrency[groupID]++
	return true
}

// releasePoolGroupConcurrency 释放账号池分组的并发槽位。
func releasePoolGroupConcurrency(groupID int) {
	if groupID <= 0 {
		return
	}
	if common.RedisEnabled && common.RDB != nil {
		key := fmt.Sprintf("nexustok:account_pool:group_concurrency:%d", groupID)
		if _, err := common.RDB.Decr(context.Background(), key).Result(); err == nil {
			return
		}
	}
	poolGroupConcurrencyMu.Lock()
	defer poolGroupConcurrencyMu.Unlock()
	if poolGroupConcurrency[groupID] <= 1 {
		delete(poolGroupConcurrency, groupID)
		return
	}
	poolGroupConcurrency[groupID]--
}

// BuildPoolAccountChannelKey 将账号池凭证转换成现有 adaptor 可理解的 channel key。
// 对于官方 OAuth 类型的账号，委托给对应的 Provider 构建密钥；
// 对于 API Key 类型的账号，尝试从 JSON 中提取 api_key/key/access_token 字段；
// 其他情况直接返回解密后的原始凭证。
func BuildPoolAccountChannelKey(account *model.PoolAccount) (string, error) {
	if account == nil {
		return "", ErrNoAvailablePoolAccount
	}
	if provider, ok := accountauth.DefaultManager().Provider(account.GetCredentialProvider()); ok && account.AuthType == model.AccountPoolAuthTypeOfficialOAuth {
		return provider.BuildChannelKey(account)
	}
	raw, err := account.GetDecryptedCredentials()
	if err != nil {
		return "", err
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("账号池账号凭证为空")
	}
	if account.AuthType != model.AccountPoolAuthTypeAPIKey {
		return raw, nil
	}
	var payload map[string]interface{}
	if err := common.UnmarshalJsonStr(raw, &payload); err != nil {
		return raw, nil
	}
	for _, key := range []string{"api_key", "key", "access_token"} {
		if value, ok := payload[key]; ok && value != nil {
			text := strings.TrimSpace(fmt.Sprintf("%v", value))
			if text != "" {
				return text, nil
			}
		}
	}
	return raw, nil
}

// poolAccountSupportsModel 检查账号是否支持指定的模型。
// 优先使用账号级别的模型列表，其次使用分组级别的模型列表。
// 空列表表示支持所有模型。
func poolAccountSupportsModel(account *model.PoolAccount, group *model.AccountPoolGroup, modelName string) bool {
	models := account.Models
	if strings.TrimSpace(models) == "" && group != nil {
		models = group.Models
	}
	modelList := splitCommaValues(models)
	if len(modelList) == 0 {
		return true
	}
	return matchesModelList(modelList, modelName)
}

// poolAccountSupportsGroup 检查账号是否属于指定的使用分组。
// 优先使用账号级别的分组，其次使用分组级别的分组。
// 空列表或通配符 "*" 表示属于所有分组。
func poolAccountSupportsGroup(account *model.PoolAccount, group *model.AccountPoolGroup, usingGroup string) bool {
	if strings.TrimSpace(usingGroup) == "" {
		return true
	}
	groupValue := account.Group
	if strings.TrimSpace(groupValue) == "" && group != nil {
		groupValue = group.Group
	}
	groups := splitCommaValues(groupValue)
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

// removePoolAccount 从账号列表中移除指定 ID 的账号。
// 返回移除后的新切片（不修改原切片）。
func removePoolAccount(accounts []*model.PoolAccount, accountID int) []*model.PoolAccount {
	for i, account := range accounts {
		if account != nil && account.Id == accountID {
			return append(accounts[:i], accounts[i+1:]...)
		}
	}
	return accounts
}
