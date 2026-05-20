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
	ErrNoAvailableChannelAccount = errors.New("该渠道账号池无可用账号")

	channelAccountCursorMu sync.Mutex
	channelAccountCursors  = map[string]uint64{}

	channelAccountConcurrencyMu sync.Mutex
	channelAccountConcurrency   = map[int]int{}
)

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

func ExcludeChannelAccountForRequest(c *gin.Context, accountID int) {
	if c == nil || accountID <= 0 {
		return
	}
	excluded := GetExcludedChannelAccountIds(c)
	excluded[accountID] = true
	common.SetContextKey(c, constant.ContextKeyChannelAccountExcludedIds, excluded)
}

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

func channelAccountSupportsModel(account *model.ChannelAccount, channel *model.Channel, modelName string) bool {
	models := account.Models
	if strings.TrimSpace(models) == "" && channel != nil {
		models = channel.Models
	}
	modelList := splitCommaValues(models)
	if len(modelList) == 0 {
		return true
	}
	return matchesModelList(modelList, modelName)
}

func channelAccountSupportsGroup(account *model.ChannelAccount, channel *model.Channel, usingGroup string) bool {
	if strings.TrimSpace(usingGroup) == "" {
		return true
	}
	group := account.Group
	if strings.TrimSpace(group) == "" && channel != nil {
		group = channel.Group
	}
	groups := splitCommaValues(group)
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

func matchesModelList(models []string, modelName string) bool {
	modelName = strings.TrimSpace(modelName)
	canonicalModel := ratio_setting.FormatMatchingModelName(modelName)
	for _, candidate := range models {
		if candidate == "*" || candidate == modelName || candidate == canonicalModel {
			return true
		}
		normalizedCandidate := ratio_setting.FormatMatchingModelName(candidate)
		if normalizedCandidate == modelName || normalizedCandidate == canonicalModel {
			return true
		}
		if strings.HasSuffix(candidate, "*") {
			prefix := strings.TrimSuffix(candidate, "*")
			if strings.HasPrefix(modelName, prefix) || strings.HasPrefix(canonicalModel, prefix) {
				return true
			}
		}
	}
	return false
}

func splitCommaValues(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func removeChannelAccount(accounts []*model.ChannelAccount, accountID int) []*model.ChannelAccount {
	for i, account := range accounts {
		if account != nil && account.Id == accountID {
			return append(accounts[:i], accounts[i+1:]...)
		}
	}
	return accounts
}
