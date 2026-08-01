// Package model - ability.go
// 该文件定义了渠道能力（Ability）数据模型及相关操作
//
// 主要结构体：
// - Ability：渠道能力表，记录渠道支持的模型及调度参数
// - AbilityWithChannel：带渠道类型的联合查询结构体
//
// 核心功能：
// - 渠道能力的增删改查
// - 基于优先级和权重的渠道选择（加权随机）
// - 渠道能力表的修复和重建（FixAbility）
//
// 数据表设计：
// - 复合主键：group + model + channel_id
// - 支持优先级（priority）和权重（weight）调度
// - 支持标签（tag）分组管理
package model

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/setting/ratio_setting"

	"github.com/samber/lo"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Ability 渠道能力表模型
// 记录某个渠道（Channel）支持的模型及调度参数
// 复合主键：Group + Model + ChannelId
type Ability struct {
	Group     string  `json:"group" gorm:"type:varchar(64);primaryKey;autoIncrement:false"`  // 分组名称
	Model     string  `json:"model" gorm:"type:varchar(255);primaryKey;autoIncrement:false"` // 模型名称
	ChannelId int     `json:"channel_id" gorm:"primaryKey;autoIncrement:false;index"`        // 渠道 ID
	Enabled   bool    `json:"enabled"`                                                       // 是否启用
	Priority  *int64  `json:"priority" gorm:"bigint;default:0;index"`                        // 优先级（数值越大越优先）
	Weight    uint    `json:"weight" gorm:"default:0;index"`                                 // 权重（同优先级内用于加权随机选择）
	Tag       *string `json:"tag" gorm:"index"`                                              // 标签（用于批量管理）
}

// AbilityWithChannel 带渠道类型的联合查询结构体
// 用于查询能力时同时获取渠道类型信息
type AbilityWithChannel struct {
	Ability
	ChannelType int `json:"channel_type"` // 渠道类型
}

// GetAllEnableAbilityWithChannels 获取所有启用的能力记录及其渠道类型
// 联合查询 abilities 和 channels 表，返回包含渠道类型的能力列表
func GetAllEnableAbilityWithChannels() ([]AbilityWithChannel, error) {
	var abilities []AbilityWithChannel
	err := DB.Table("abilities").
		Select("abilities.*, channels.type as channel_type").
		Joins("left join channels on abilities.channel_id = channels.id").
		Where("abilities.enabled = ?", true).
		Scan(&abilities).Error
	return abilities, err
}

// GetGroupEnabledModels 获取指定分组下所有启用的模型名称列表
// 返回去重后的模型名称切片
func GetGroupEnabledModels(group string) []string {
	var models []string
	// Find distinct models
	DB.Table("abilities").Where(commonGroupCol+" = ? and enabled = ?", group, true).Distinct("model").Pluck("model", &models)
	return models
}

// GetEnabledModels 获取所有启用的模型名称列表（跨分组）
// 返回去重后的模型名称切片
func GetEnabledModels() []string {
	var models []string
	// Find distinct models
	DB.Table("abilities").Where("enabled = ?", true).Distinct("model").Pluck("model", &models)
	return models
}

// GetAllEnableAbilities 获取所有启用的能力记录
func GetAllEnableAbilities() []Ability {
	var abilities []Ability
	DB.Find(&abilities, "enabled = ?", true)
	return abilities
}

// getPriority 获取指定分组和模型在第 retry 次重试时应使用的优先级值
// 优先级按降序排列，retry=0 使用最高优先级，retry=1 使用次高优先级，以此类推
// 当 retry 超过可用优先级数量时，使用最低优先级
func getPriority(group string, model string, retry int) (int, error) {
	return getPriorityWithExclusions(group, model, retry, nil)
}

// getPriorityWithExclusions 获取过滤掉禁用渠道和请求级失败渠道后的可用优先级。
//
// 数据库兜底路径不能只看 abilities.enabled：自动禁用渠道时能力表更新可能和
// 渠道状态存在短暂不一致，因此这里显式 join channels 并要求渠道仍处于启用状态。
func getPriorityWithExclusions(group string, model string, retry int, excludedChannelIds []int) (int, error) {
	var priorities []int
	query := abilityEnabledChannelQuery().
		Select("DISTINCT(abilities.priority)").
		Where("abilities."+commonGroupCol+" = ? and abilities.model = ? and abilities.enabled = ?", group, model, true)
	query = applyExcludedChannelCondition(query, excludedChannelIds)
	err := query.
		Order("abilities.priority DESC").    // 按优先级降序排序
		Pluck("priority", &priorities).Error // Pluck用于将查询的结果直接扫描到一个切片中

	if err != nil {
		// 处理错误
		return 0, err
	}

	if len(priorities) == 0 {
		// 如果没有查询到优先级，则返回错误
		return 0, errors.New("数据库一致性被破坏")
	}

	// 确定要使用的优先级
	var priorityToUse int
	if retry >= len(priorities) {
		// 如果重试次数大于优先级数，则使用最小的优先级
		priorityToUse = priorities[len(priorities)-1]
	} else {
		priorityToUse = priorities[retry]
	}
	return priorityToUse, nil
}

// getChannelQuery 构建渠道能力查询条件
// 根据 retry 次数确定优先级，返回对应的 GORM 查询对象
// retry=0 时查询最高优先级，retry>0 时查询对应优先级
func getChannelQuery(group string, model string, retry int) (*gorm.DB, error) {
	return getChannelQueryWithExclusions(group, model, retry, nil)
}

// getChannelQueryWithExclusions 构建带渠道状态过滤和请求级排除集的能力查询。
func getChannelQueryWithExclusions(group string, model string, retry int, excludedChannelIds []int) (*gorm.DB, error) {
	maxPrioritySubQuery := abilityEnabledChannelQuery().
		Select("MAX(abilities.priority)").
		Where("abilities."+commonGroupCol+" = ? and abilities.model = ? and abilities.enabled = ?", group, model, true)
	maxPrioritySubQuery = applyExcludedChannelCondition(maxPrioritySubQuery, excludedChannelIds)

	channelQuery := abilityEnabledChannelQuery().
		Where("abilities."+commonGroupCol+" = ? and abilities.model = ? and abilities.enabled = ? and abilities.priority = (?)", group, model, true, maxPrioritySubQuery)
	channelQuery = applyExcludedChannelCondition(channelQuery, excludedChannelIds)
	if retry != 0 && len(excludedChannelIds) == 0 {
		priority, err := getPriorityWithExclusions(group, model, retry, excludedChannelIds)
		if err != nil {
			return nil, err
		}
		channelQuery = abilityEnabledChannelQuery().
			Where("abilities."+commonGroupCol+" = ? and abilities.model = ? and abilities.enabled = ? and abilities.priority = ?", group, model, true, priority)
		channelQuery = applyExcludedChannelCondition(channelQuery, excludedChannelIds)
	}
	return channelQuery, nil
}

// abilityEnabledChannelQuery 返回只包含启用渠道的 ability 查询基底。
func abilityEnabledChannelQuery() *gorm.DB {
	return DB.Model(&Ability{}).
		Joins("JOIN channels ON channels.id = abilities.channel_id").
		Where("channels.status = ?", common.ChannelStatusEnabled)
}

// applyExcludedChannelCondition 在查询中排除当前请求已经失败的渠道。
func applyExcludedChannelCondition(query *gorm.DB, excludedChannelIds []int) *gorm.DB {
	if len(excludedChannelIds) == 0 {
		return query
	}
	validIds := make([]int, 0, len(excludedChannelIds))
	seen := make(map[int]struct{}, len(excludedChannelIds))
	for _, id := range excludedChannelIds {
		if id <= 0 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		validIds = append(validIds, id)
	}
	if len(validIds) == 0 {
		return query
	}
	return query.Where("abilities.channel_id NOT IN ?", validIds)
}

// GetChannel 根据分组、模型和重试次数选择一个可用渠道
// 使用加权随机选择算法：权重越高的渠道被选中的概率越大
// 基础权重为 10，加上渠道自身的权重值
// 返回 nil 表示没有可用渠道
func GetChannel(group string, model string, retry int, requestPath string) (*Channel, error) {
	return GetChannelWithExclusions(group, model, retry, requestPath, nil)
}

// GetChannelWithExclusions 根据分组、模型和请求级排除集选择一个可用渠道。
func GetChannelWithExclusions(group string, model string, retry int, requestPath string, excludedChannelIds []int) (*Channel, error) {
	var abilities []Ability

	var err error = nil
	channelQuery, err := getChannelQueryWithExclusions(group, model, retry, requestPathNormalizedExclusions(excludedChannelIds))
	if err != nil {
		return nil, err
	}
	err = channelQuery.Order("abilities.weight DESC").Find(&abilities).Error
	if err != nil {
		return nil, err
	}
	abilities = filterAbilitiesByRequestPath(abilities, requestPath)
	channel := Channel{}
	if len(abilities) > 0 {
		// Randomly choose one
		weightSum := uint(0)
		for _, ability_ := range abilities {
			weightSum += ability_.Weight + 10
		}
		// Randomly choose one
		weight := common.GetRandomInt(int(weightSum))
		for _, ability_ := range abilities {
			weight -= int(ability_.Weight) + 10
			//log.Printf("weight: %d, ability weight: %d", weight, *ability_.Weight)
			if weight <= 0 {
				channel.Id = ability_.ChannelId
				break
			}
		}
	} else {
		return nil, nil
	}
	err = DB.First(&channel, "id = ?", channel.Id).Error
	return &channel, err
}

// getChannelCandidatesWithExclusions 获取数据库兜底路径下的候选渠道。
//
// 数据库路径继续使用原有 ability 查询来决定优先级层级，再把能力记录转换为
// 渠道对象返回给服务层评分。查询过程中不写入任何动态健康状态。
func getChannelCandidatesWithExclusions(group string, model string, retry int, requestPath string, excludedChannelIds []int) ([]*Channel, error) {
	channelQuery, err := getChannelQueryWithExclusions(group, model, retry, requestPathNormalizedExclusions(excludedChannelIds))
	if err != nil {
		return nil, err
	}

	var abilities []Ability
	if err := channelQuery.Order("abilities.weight DESC").Find(&abilities).Error; err != nil {
		return nil, err
	}
	abilities = filterAbilitiesByRequestPath(abilities, requestPath)
	if len(abilities) == 0 {
		return nil, nil
	}

	channelIDs := make([]int, 0, len(abilities))
	seen := make(map[int]struct{}, len(abilities))
	for _, ability := range abilities {
		if _, ok := seen[ability.ChannelId]; ok {
			continue
		}
		seen[ability.ChannelId] = struct{}{}
		channelIDs = append(channelIDs, ability.ChannelId)
	}

	var channels []*Channel
	if err := DB.Where("id IN ?", channelIDs).Find(&channels).Error; err != nil {
		return nil, err
	}
	channelByID := make(map[int]*Channel, len(channels))
	for _, channel := range channels {
		channelByID[channel.Id] = channel
	}
	ordered := make([]*Channel, 0, len(channelIDs))
	for _, channelID := range channelIDs {
		channel, ok := channelByID[channelID]
		if !ok {
			return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channelID)
		}
		ordered = append(ordered, channel)
	}
	return ordered, nil
}

// getAllChannelCandidatesWithExclusions 获取数据库兜底路径下所有优先级的候选渠道。
func getAllChannelCandidatesWithExclusions(group, model, requestPath string, excludedChannelIds []int) ([]*Channel, error) {
	buildQuery := func(modelName string) *gorm.DB {
		query := abilityEnabledChannelQuery().Where(
			"abilities."+commonGroupCol+" = ? and abilities.model = ? and abilities.enabled = ?",
			group, modelName, true,
		)
		return applyExcludedChannelCondition(query, requestPathNormalizedExclusions(excludedChannelIds))
	}

	var abilities []Ability
	query := buildQuery(model)
	if err := query.Order("abilities.priority DESC").Order("abilities.weight DESC").Find(&abilities).Error; err != nil {
		return nil, err
	}
	abilities = filterAbilitiesByRequestPath(abilities, requestPath)
	if len(abilities) == 0 {
		normalizedModel := ratio_setting.FormatMatchingModelName(model)
		if normalizedModel != model {
			if err := buildQuery(normalizedModel).Order("abilities.priority DESC").Order("abilities.weight DESC").Find(&abilities).Error; err != nil {
				return nil, err
			}
			abilities = filterAbilitiesByRequestPath(abilities, requestPath)
		}
	}
	if len(abilities) == 0 {
		return nil, nil
	}

	channelIDs := make([]int, 0, len(abilities))
	seen := make(map[int]struct{})
	for _, ability := range abilities {
		if _, ok := seen[ability.ChannelId]; ok {
			continue
		}
		seen[ability.ChannelId] = struct{}{}
		channelIDs = append(channelIDs, ability.ChannelId)
	}
	var channels []*Channel
	if err := DB.Where("id IN ?", channelIDs).Find(&channels).Error; err != nil {
		return nil, err
	}
	channelByID := make(map[int]*Channel, len(channels))
	for _, channel := range channels {
		channelByID[channel.Id] = channel
	}
	ordered := make([]*Channel, 0, len(channelIDs))
	for _, channelID := range channelIDs {
		channel, ok := channelByID[channelID]
		if !ok {
			return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channelID)
		}
		ordered = append(ordered, channel)
	}
	return ordered, nil
}

// requestPathNormalizedExclusions 返回去重后的渠道排除列表。
func requestPathNormalizedExclusions(excludedChannelIds []int) []int {
	if len(excludedChannelIds) == 0 {
		return nil
	}
	seen := make(map[int]struct{}, len(excludedChannelIds))
	ids := make([]int, 0, len(excludedChannelIds))
	for _, id := range excludedChannelIds {
		if id <= 0 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

// filterAbilitiesByRequestPath 按请求路径过滤数据库兜底选路得到的能力候选。
//
// 非内存缓存路径无法复用 channel2advancedCustomConfig，因此这里一次性查询候选渠道；
// 普通渠道全部保留，只有 Advanced Custom 渠道会要求 incoming_path 命中当前请求。
// 如果 requestPath 为空，则保持旧行为，避免后台维护和测试调用被意外过滤。
func filterAbilitiesByRequestPath(abilities []Ability, requestPath string) []Ability {
	if requestPath == "" || len(abilities) == 0 {
		return abilities
	}

	channelIds := make([]int, 0, len(abilities))
	seenChannelIds := make(map[int]struct{}, len(abilities))
	for _, ability := range abilities {
		if _, exists := seenChannelIds[ability.ChannelId]; exists {
			continue
		}
		seenChannelIds[ability.ChannelId] = struct{}{}
		channelIds = append(channelIds, ability.ChannelId)
	}

	var channels []*Channel
	if err := DB.Where("id IN ?", channelIds).Find(&channels).Error; err != nil {
		// 选路过滤不是数据源；查询异常时沿用旧候选，避免把临时 DB 异常放大成无渠道。
		return abilities
	}

	advancedConfigs := make(map[int]*dto.AdvancedCustomConfig)
	for _, channel := range channels {
		if channel.Type == constant.ChannelTypeAdvancedCustom {
			advancedConfigs[channel.Id] = channel.GetOtherSettings().AdvancedCustom
		}
	}

	filtered := make([]Ability, 0, len(abilities))
	for _, ability := range abilities {
		config, isAdvancedCustom := advancedConfigs[ability.ChannelId]
		if !isAdvancedCustom {
			filtered = append(filtered, ability)
			continue
		}
		if config != nil && config.SupportsPath(requestPath) {
			filtered = append(filtered, ability)
		}
	}
	return filtered
}

// AddAbilities 为渠道添加能力记录
// 将渠道支持的模型和分组组合写入能力表
// 使用 ON CONFLICT DO NOTHING 避免重复插入
// 支持分批插入（每批 50 条）以避免大事务
func (channel *Channel) AddAbilities(tx *gorm.DB) error {
	if channel.IsChannelAccountPoolEnabled() && !channel.ChannelInfo.AccountPoolFallback {
		return channel.UpdateAccountPoolAbilities(tx)
	}
	models_ := strings.Split(channel.Models, ",")
	groups_ := strings.Split(channel.Group, ",")
	abilitySet := make(map[string]struct{})
	abilities := make([]Ability, 0, len(models_))
	for _, model := range models_ {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		for _, group := range groups_ {
			group = strings.TrimSpace(group)
			if group == "" {
				continue
			}
			key := group + "|" + model
			if _, exists := abilitySet[key]; exists {
				continue
			}
			abilitySet[key] = struct{}{}
			ability := Ability{
				Group:     group,
				Model:     model,
				ChannelId: channel.Id,
				Enabled:   channel.Status == common.ChannelStatusEnabled,
				Priority:  channel.Priority,
				Weight:    uint(channel.GetWeight()),
				Tag:       channel.Tag,
			}
			abilities = append(abilities, ability)
		}
	}
	if len(abilities) == 0 {
		return nil
	}
	// choose DB or provided tx
	useDB := DB
	if tx != nil {
		useDB = tx
	}
	for _, chunk := range lo.Chunk(abilities, 50) {
		err := useDB.Clauses(clause.OnConflict{DoNothing: true}).Create(&chunk).Error
		if err != nil {
			return err
		}
	}
	return nil
}

// DeleteAbilities 删除渠道的所有能力记录
func (channel *Channel) DeleteAbilities() error {
	return DB.Where("channel_id = ?", channel.Id).Delete(&Ability{}).Error
}

// UpdateAbilities updates abilities of this channel.
// Make sure the channel is completed before calling this function.
func (channel *Channel) UpdateAbilities(tx *gorm.DB) error {
	if channel.IsChannelAccountPoolEnabled() && !channel.ChannelInfo.AccountPoolFallback {
		return channel.UpdateAccountPoolAbilities(tx)
	}
	isNewTx := false
	// 如果没有传入事务，创建新的事务
	if tx == nil {
		tx = DB.Begin()
		if tx.Error != nil {
			return tx.Error
		}
		isNewTx = true
		defer func() {
			if r := recover(); r != nil {
				tx.Rollback()
			}
		}()
	}

	// First delete all abilities of this channel
	err := tx.Where("channel_id = ?", channel.Id).Delete(&Ability{}).Error
	if err != nil {
		if isNewTx {
			tx.Rollback()
		}
		return err
	}

	// Then add new abilities
	models_ := strings.Split(channel.Models, ",")
	groups_ := strings.Split(channel.Group, ",")
	abilitySet := make(map[string]struct{})
	abilities := make([]Ability, 0, len(models_))
	for _, model := range models_ {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		for _, group := range groups_ {
			group = strings.TrimSpace(group)
			if group == "" {
				continue
			}
			key := group + "|" + model
			if _, exists := abilitySet[key]; exists {
				continue
			}
			abilitySet[key] = struct{}{}
			ability := Ability{
				Group:     group,
				Model:     model,
				ChannelId: channel.Id,
				Enabled:   channel.Status == common.ChannelStatusEnabled,
				Priority:  channel.Priority,
				Weight:    uint(channel.GetWeight()),
				Tag:       channel.Tag,
			}
			abilities = append(abilities, ability)
		}
	}

	if len(abilities) > 0 {
		for _, chunk := range lo.Chunk(abilities, 50) {
			err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&chunk).Error
			if err != nil {
				if isNewTx {
					tx.Rollback()
				}
				return err
			}
		}
	}

	// 如果是新创建的事务，需要提交
	if isNewTx {
		return tx.Commit().Error
	}

	return nil
}

// SyncChannelAccountPoolCapabilities 基于渠道内启用账号重建渠道顶层能力和能力表。
//
// 上游账号同步渠道允许每个 key 维护不同的模型、分组和启停状态。普通
// Channel.UpdateAbilities 只能根据渠道顶层 models/group 做笛卡尔积，无法表达
// “账号 A 只支持 default/gpt-a，账号 B 只支持 vip/gpt-b” 这类真实组合。同步渠道
// 必须以最终启用账号为准生成 Ability，否则路由层会把不存在可用账号的组合选中，
// 直到账号池选择阶段才失败并触发额外降级。
func SyncChannelAccountPoolCapabilities(channelID int, tx *gorm.DB) error {
	isNewTx := false
	if tx == nil {
		tx = DB.Begin()
		if tx.Error != nil {
			return tx.Error
		}
		isNewTx = true
		defer func() {
			if r := recover(); r != nil {
				tx.Rollback()
			}
		}()
	}

	var channel Channel
	if err := tx.Where("id = ?", channelID).First(&channel).Error; err != nil {
		if isNewTx {
			tx.Rollback()
		}
		return err
	}
	if !channel.IsChannelAccountPoolEnabled() || channel.ChannelInfo.AccountPoolFallback {
		if isNewTx {
			return tx.Commit().Error
		}
		return nil
	}
	if err := channel.syncAccountPoolCapabilities(tx); err != nil {
		if isNewTx {
			tx.Rollback()
		}
		return err
	}
	if isNewTx {
		return tx.Commit().Error
	}
	return nil
}

// UpdateAccountPoolAbilities 只重建账号池渠道的 Ability，不修改渠道 models/group。
//
// 该方法供 AddAbilities/UpdateAbilities/FixAbility 使用，保持历史渠道字段不被能力
// 修复任务意外改写；需要同时同步渠道顶层 models/group 时应调用
// SyncChannelAccountPoolCapabilities。
func (channel *Channel) UpdateAccountPoolAbilities(tx *gorm.DB) error {
	isNewTx := false
	if tx == nil {
		tx = DB.Begin()
		if tx.Error != nil {
			return tx.Error
		}
		isNewTx = true
		defer func() {
			if r := recover(); r != nil {
				tx.Rollback()
			}
		}()
	}
	abilities, _, _, err := channel.buildAccountPoolAbilities(tx)
	if err != nil {
		if isNewTx {
			tx.Rollback()
		}
		return err
	}
	if err := replaceChannelAbilities(tx, channel.Id, abilities); err != nil {
		if isNewTx {
			tx.Rollback()
		}
		return err
	}
	if isNewTx {
		return tx.Commit().Error
	}
	return nil
}

func (channel *Channel) syncAccountPoolCapabilities(tx *gorm.DB) error {
	abilities, models, groups, err := channel.buildAccountPoolAbilities(tx)
	if err != nil {
		return err
	}
	if channel.HasUpstreamAccountSyncMetadata() {
		// 上游账号同步渠道中 ChannelAccount.group 表示“密钥分组”，只用于展示和成本
		// 区分；NexusTok 下游用户的可用分组必须始终来自 Channel.group。刷新账号池
		// 能力时只更新模型集合，避免把上游密钥分组回写为渠道分组。
		groups = firstNonEmptyString(strings.TrimSpace(channel.Group), "default")
		if err := tx.Model(channel).Select("models", "group").Updates(map[string]any{
			"models": models,
			"group":  groups,
		}).Error; err != nil {
			return err
		}
		channel.Models = models
		channel.Group = groups
		return replaceChannelAbilities(tx, channel.Id, abilities)
	}
	if strings.TrimSpace(groups) == "" {
		groups = firstNonEmptyString(strings.TrimSpace(channel.Group), "default")
	}
	if err := tx.Model(channel).Select("models", "group").Updates(map[string]any{
		"models": models,
		"group":  groups,
	}).Error; err != nil {
		return err
	}
	channel.Models = models
	channel.Group = groups
	return replaceChannelAbilities(tx, channel.Id, abilities)
}

func (channel *Channel) buildAccountPoolAbilities(tx *gorm.DB) ([]Ability, string, string, error) {
	var accounts []ChannelAccount
	if err := tx.Where("channel_id = ? AND status = ?", channel.Id, common.ChannelStatusEnabled).Order("priority DESC").Order("id ASC").Find(&accounts).Error; err != nil {
		return nil, "", "", err
	}
	abilities := make([]Ability, 0)
	abilitySet := map[string]struct{}{}
	modelSet := map[string]struct{}{}
	groupSet := map[string]struct{}{}
	models := make([]string, 0)
	groups := make([]string, 0)
	useChannelGroups := channel.HasUpstreamAccountSyncMetadata()
	channelGroups := splitAbilityValues(firstNonEmptyString(channel.Group, "default"))
	for _, account := range accounts {
		accountModels := splitAbilityValues(firstNonEmptyString(account.Models, channel.Models))
		accountGroups := splitAbilityValues(firstNonEmptyString(account.Group, channel.Group))
		if useChannelGroups {
			// 同步渠道的账号分组是上游密钥分组，不能参与 NexusTok 用户分组路由；
			// 但账号模型仍表示该 key 的真实支持范围，需要继续逐账号生成能力。
			accountGroups = channelGroups
		}
		for _, modelName := range accountModels {
			if _, ok := modelSet[modelName]; !ok {
				modelSet[modelName] = struct{}{}
				models = append(models, modelName)
			}
			for _, groupName := range accountGroups {
				if _, ok := groupSet[groupName]; !ok {
					groupSet[groupName] = struct{}{}
					groups = append(groups, groupName)
				}
				abilityKey := groupName + "|" + modelName
				if _, exists := abilitySet[abilityKey]; exists {
					continue
				}
				abilitySet[abilityKey] = struct{}{}
				abilities = append(abilities, Ability{
					Group:     groupName,
					Model:     modelName,
					ChannelId: channel.Id,
					Enabled:   channel.Status == common.ChannelStatusEnabled,
					Priority:  channel.Priority,
					Weight:    uint(channel.GetWeight()),
					Tag:       channel.Tag,
				})
			}
		}
	}
	return abilities, strings.Join(models, ","), strings.Join(groups, ","), nil
}

func replaceChannelAbilities(tx *gorm.DB, channelID int, abilities []Ability) error {
	if err := tx.Where("channel_id = ?", channelID).Delete(&Ability{}).Error; err != nil {
		return err
	}
	if len(abilities) == 0 {
		return nil
	}
	for _, chunk := range lo.Chunk(abilities, 50) {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&chunk).Error; err != nil {
			return err
		}
	}
	return nil
}

func splitAbilityValues(value string) []string {
	parts := strings.Split(strings.TrimSpace(value), ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			values = append(values, part)
		}
	}
	return values
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// UpdateAbilityStatus 更新指定渠道所有能力的启用状态
func UpdateAbilityStatus(channelId int, status bool) error {
	return DB.Model(&Ability{}).Where("channel_id = ?", channelId).Select("enabled").Update("enabled", status).Error
}

// UpdateAbilityStatusByTag 按标签批量更新能力的启用状态
func UpdateAbilityStatusByTag(tag string, status bool) error {
	return DB.Model(&Ability{}).Where("tag = ?", tag).Select("enabled").Update("enabled", status).Error
}

// UpdateAbilityByTag 按标签更新能力的属性（标签、优先级、权重）
func UpdateAbilityByTag(tag string, newTag *string, priority *int64, weight *uint) error {
	ability := Ability{}
	if newTag != nil {
		ability.Tag = newTag
	}
	if priority != nil {
		ability.Priority = priority
	}
	if weight != nil {
		ability.Weight = *weight
	}
	return DB.Model(&Ability{}).Where("tag = ?", tag).Updates(ability).Error
}

// fixLock 修复操作的互斥锁，防止并发修复导致数据不一致
var fixLock = sync.Mutex{}

// FixAbility 修复渠道能力表
// 清空能力表后根据所有渠道的配置重新生成能力记录
// 使用互斥锁保证同一时间只有一个修复任务运行
// 返回值：成功数、失败数、错误
func FixAbility() (int, int, error) {
	lock := fixLock.TryLock()
	if !lock {
		return 0, 0, errors.New("已经有一个修复任务在运行中，请稍后再试")
	}
	defer fixLock.Unlock()

	// truncate abilities table
	if common.UsingSQLite {
		err := DB.Exec("DELETE FROM abilities").Error
		if err != nil {
			common.SysLog(fmt.Sprintf("Delete abilities failed: %s", err.Error()))
			return 0, 0, err
		}
	} else {
		err := DB.Exec("TRUNCATE TABLE abilities").Error
		if err != nil {
			common.SysLog(fmt.Sprintf("Truncate abilities failed: %s", err.Error()))
			return 0, 0, err
		}
	}
	var channels []*Channel
	// Find all channels
	err := DB.Model(&Channel{}).Find(&channels).Error
	if err != nil {
		return 0, 0, err
	}
	if len(channels) == 0 {
		return 0, 0, nil
	}
	successCount := 0
	failCount := 0
	for _, chunk := range lo.Chunk(channels, 50) {
		ids := lo.Map(chunk, func(c *Channel, _ int) int { return c.Id })
		// Delete all abilities of this channel
		err = DB.Where("channel_id IN ?", ids).Delete(&Ability{}).Error
		if err != nil {
			common.SysLog(fmt.Sprintf("Delete abilities failed: %s", err.Error()))
			failCount += len(chunk)
			continue
		}
		// Then add new abilities
		for _, channel := range chunk {
			err = channel.AddAbilities(nil)
			if err != nil {
				common.SysLog(fmt.Sprintf("Add abilities for channel %d failed: %s", channel.Id, err.Error()))
				failCount++
			} else {
				successCount++
			}
		}
	}
	InitChannelCache()
	return successCount, failCount, nil
}
