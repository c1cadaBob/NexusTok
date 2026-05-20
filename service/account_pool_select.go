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
	"github.com/c1cada/NexusTok/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

var (
	ErrNoAvailablePoolAccount = errors.New("账号池无可用账号")

	poolAccountCursorMu sync.Mutex
	poolAccountCursors  = map[string]uint64{}

	poolAccountConcurrencyMu sync.Mutex
	poolAccountConcurrency   = map[int]int{}
)

// SelectPoolAccount 从渠道引用的全局账号池组里选择一个可调度账号。
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

	usingGroup = resolveChannelAccountUsingGroup(c, usingGroup)
	now := common.GetTimestamp()
	excluded := GetExcludedPoolAccountIds(c)

	var accounts []*model.PoolAccount
	if err := model.DB.Where("pool_group_id = ? AND status = ? AND schedulable = ?", group.Id, common.ChannelStatusEnabled, true).
		Order("priority DESC").Order("id ASC").Find(&accounts).Error; err != nil {
		return group, nil, err
	}

	candidates := make([]*model.PoolAccount, 0, len(accounts))
	for _, account := range accounts {
		if account == nil {
			continue
		}
		if excluded[account.Id] {
			continue
		}
		if account.Status != common.ChannelStatusEnabled || !account.Schedulable || account.IsCoolingDown(now) {
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
		return group, nil, ErrNoAvailablePoolAccount
	}

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
	for len(availableTopAccounts) > 0 {
		account := pickPoolAccount(group, availableTopAccounts, usingGroup, modelName)
		if account == nil {
			return group, nil, ErrNoAvailablePoolAccount
		}
		if ReservePoolAccount(c, account) {
			model.TouchPoolAccount(account.Id)
			return group, account, nil
		}
		ExcludePoolAccountForRequest(c, account.Id)
		availableTopAccounts = removePoolAccount(availableTopAccounts, account.Id)
	}
	return group, nil, ErrNoAvailablePoolAccount
}

func sortPoolAccountCandidates(accounts []*model.PoolAccount, strategy string) {
	strategy = strings.TrimSpace(strategy)
	sort.SliceStable(accounts, func(i, j int) bool {
		if accounts[i].Priority != accounts[j].Priority {
			return accounts[i].Priority > accounts[j].Priority
		}
		if strategy == model.AccountPoolStrategyLeastUsed && accounts[i].UsedQuota != accounts[j].UsedQuota {
			return accounts[i].UsedQuota < accounts[j].UsedQuota
		}
		return accounts[i].Id < accounts[j].Id
	})
}

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
	case model.AccountPoolStrategyLeastUsed:
		return accounts[0]
	}
	cursorKey := buildPoolAccountCursorKey(groupID, usingGroup, modelName)
	cursor := nextPoolAccountCursor(cursorKey)
	return pickPoolAccountByCursor(accounts, cursor, strategy == model.AccountPoolStrategyWeighted)
}

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

func ExcludePoolAccountForRequest(c *gin.Context, accountID int) {
	if c == nil || accountID <= 0 {
		return
	}
	excluded := GetExcludedPoolAccountIds(c)
	excluded[accountID] = true
	common.SetContextKey(c, constant.ContextKeyPoolAccountExcludedIds, excluded)
}

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

func ReleaseSelectedPoolAccount(c *gin.Context) {
	if c == nil {
		return
	}
	if !common.GetContextKeyBool(c, constant.ContextKeyPoolAccountReserved) {
		return
	}
	accountID := common.GetContextKeyInt(c, constant.ContextKeyPoolAccountId)
	if accountID <= 0 {
		common.SetContextKey(c, constant.ContextKeyPoolAccountReserved, false)
		return
	}
	releasePoolAccountConcurrency(accountID)
	common.SetContextKey(c, constant.ContextKeyPoolAccountReserved, false)
}

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

// BuildPoolAccountChannelKey 将账号池凭证转换成现有 adaptor 可理解的 channel key。
func BuildPoolAccountChannelKey(account *model.PoolAccount) (string, error) {
	if account == nil {
		return "", ErrNoAvailablePoolAccount
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

func removePoolAccount(accounts []*model.PoolAccount, accountID int) []*model.PoolAccount {
	for i, account := range accounts {
		if account != nil && account.Id == accountID {
			return append(accounts[:i], accounts[i+1:]...)
		}
	}
	return accounts
}
