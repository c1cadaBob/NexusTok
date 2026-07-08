// Package model - usedata.go
// 该文件定义了使用数据（QuotaData）数据模型及相关操作
//
// 主要结构体：
// - QuotaData：配额使用数据（柱状图数据）
//
// 核心功能：
// - 使用数据的记录和查询
// - 按用户、模型、时间范围统计使用量
// - 支持数据导出和排行榜功能
package model

import (
	"fmt"
	"sync"
	"time"

	"github.com/c1cada/NexusTok/common"
	"gorm.io/gorm"
)

// QuotaData 配额使用数据（柱状图数据）
// 用于统计用户的 API 使用量，支持按用户、模型、时间维度聚合
type QuotaData struct {
	Id        int    `json:"id"`                                                                                              // 数据 ID
	UserID    int    `json:"user_id" gorm:"index;index:idx_qdt_flow_user,priority:1"`                                         // 用户 ID
	Username  string `json:"username" gorm:"index:idx_qdt_model_user_name,priority:2;size:64;default:''"`                     // 用户名
	NodeName  string `json:"node_name" gorm:"size:128;default:'';index:idx_qdt_flow_node"`                                    // 记录该用量的节点名称
	TokenID   int    `json:"token_id" gorm:"default:0;index:idx_qdt_flow_token"`                                              // API Token ID，Root 流量账本用于定位调用入口
	UseGroup  string `json:"use_group" gorm:"size:64;default:'';index:idx_qdt_flow_group"`                                    // 实际使用的分组
	ChannelID int    `json:"channel_id" gorm:"default:0;index:idx_qdt_flow_channel"`                                          // 实际命中的渠道 ID
	ModelName string `json:"model_name" gorm:"index:idx_qdt_model_user_name,priority:1;size:64;default:''"`                   // 模型名称
	CreatedAt int64  `json:"created_at" gorm:"bigint;index:idx_qdt_created_at,priority:2;index:idx_qdt_flow_user,priority:2"` // 创建时间
	TokenUsed int    `json:"token_used" gorm:"default:0"`                                                                     // 使用的 Token 数量
	Count     int    `json:"count" gorm:"default:0"`                                                                          // 请求次数
	Quota     int    `json:"quota" gorm:"default:0"`                                                                          // 使用的配额
}

// QuotaDataLogParams 描述一次写入用量看板缓存所需的完整维度。
//
// 旧看板只按用户、模型、小时聚合；流量账本需要额外保留节点、Token、分组和渠道。
// 所有维度都落在同一张 quota_data 表中，避免新增跨数据库迁移复杂度，也让旧接口继续
// 通过 Select/Group 读取原有字段。
type QuotaDataLogParams struct {
	UserID    int
	Username  string
	NodeName  string
	TokenID   int
	UseGroup  string
	ChannelID int
	ModelName string
	Quota     int
	CreatedAt int64
	TokenUsed int
}

// FlowQuotaData 是 /api/data/flow 返回的聚合视图。
//
// Root 可以看到 node/token/channel 维度；Admin 隐藏 token 和 node；普通用户只看到
// 自己的 token/group/model 聚合，避免暴露渠道拓扑和其它用户信息。
type FlowQuotaData struct {
	UserID      int    `json:"user_id,omitempty" gorm:"column:user_id"`
	Username    string `json:"username,omitempty" gorm:"column:username"`
	NodeName    string `json:"node_name,omitempty" gorm:"column:node_name"`
	TokenID     int    `json:"token_id,omitempty" gorm:"column:token_id"`
	TokenName   string `json:"token_name,omitempty" gorm:"-"`
	UseGroup    string `json:"use_group" gorm:"column:use_group"`
	ChannelID   int    `json:"channel_id,omitempty" gorm:"column:channel_id"`
	ChannelName string `json:"channel_name,omitempty" gorm:"-"`
	ModelName   string `json:"model_name" gorm:"column:model_name"`
	TokenUsed   int    `json:"token_used" gorm:"column:token_used"`
	Count       int    `json:"count" gorm:"column:count"`
	Quota       int    `json:"quota" gorm:"column:quota"`
}

// UpdateQuotaData 定时更新使用数据（后台协程）
// 根据 DataExportInterval 配置定期将内存缓存的使用数据写入数据库
func UpdateQuotaData() {
	for {
		if common.DataExportEnabled {
			common.SysLog("正在更新数据看板数据...")
			SaveQuotaDataCache()
		}
		time.Sleep(time.Duration(common.DataExportInterval) * time.Minute)
	}
}

// CacheQuotaData 内存缓存的使用数据
// 用于批量写入数据库，减少数据库 IO
var CacheQuotaData = make(map[string]*QuotaData)

// CacheQuotaDataLock 使用数据缓存锁
var CacheQuotaDataLock = sync.Mutex{}

// logQuotaDataCache 将使用数据记录到内存缓存
// 如果缓存中已存在相同键的数据，则累加 TokenUsed、Count、Quota
//
// 参数：
//   - userId: 用户 ID
//   - username: 用户名
//   - modelName: 模型名称
//   - quota: 使用的配额
//   - createdAt: 创建时间
//   - tokenUsed: 使用的 Token 数量
func logQuotaDataCache(params QuotaDataLogParams) {
	key := fmt.Sprintf("%d-%s-%s-%d-%s-%d-%d-%s", params.UserID, params.Username, params.ModelName, params.CreatedAt, params.UseGroup, params.TokenID, params.ChannelID, params.NodeName)
	quotaData, ok := CacheQuotaData[key]
	if ok {
		quotaData.Count += 1
		quotaData.Quota += params.Quota
		quotaData.TokenUsed += params.TokenUsed
	} else {
		quotaData = &QuotaData{
			UserID:    params.UserID,
			Username:  params.Username,
			NodeName:  params.NodeName,
			TokenID:   params.TokenID,
			UseGroup:  params.UseGroup,
			ChannelID: params.ChannelID,
			ModelName: params.ModelName,
			CreatedAt: params.CreatedAt,
			Count:     1,
			Quota:     params.Quota,
			TokenUsed: params.TokenUsed,
		}
	}
	CacheQuotaData[key] = quotaData
}

// LogQuotaData 将一条消费记录折叠进小时级用量缓存。
//
// CreatedAt 会向下取整到小时；其它维度保持原值。UseGroup 为空时说明是旧调用路径或
// 非 relay 消费记录，flow 查询会忽略这类缺少分组语义的历史行，避免生成误导性流向。
func LogQuotaData(params QuotaDataLogParams) {
	// 只精确到小时
	params.CreatedAt = params.CreatedAt - (params.CreatedAt % 3600)

	CacheQuotaDataLock.Lock()
	defer CacheQuotaDataLock.Unlock()
	logQuotaDataCache(params)
}

func SaveQuotaDataCache() {
	CacheQuotaDataLock.Lock()
	defer CacheQuotaDataLock.Unlock()
	size := len(CacheQuotaData)
	// 如果缓存中有数据，就保存到数据库中
	// 1. 先查询数据库中是否有数据
	// 2. 如果有数据，就更新数据
	// 3. 如果没有数据，就插入数据
	for _, quotaData := range CacheQuotaData {
		quotaDataDB := &QuotaData{}
		DB.Table("quota_data").
			Where("user_id = ? and username = ? and model_name = ? and created_at = ? and use_group = ? and token_id = ? and channel_id = ? and node_name = ?",
				quotaData.UserID, quotaData.Username, quotaData.ModelName, quotaData.CreatedAt, quotaData.UseGroup, quotaData.TokenID, quotaData.ChannelID, quotaData.NodeName).
			First(quotaDataDB)
		if quotaDataDB.Id > 0 {
			//quotaDataDB.Count += quotaData.Count
			//quotaDataDB.Quota += quotaData.Quota
			//DB.Table("quota_data").Save(quotaDataDB)
			increaseQuotaData(quotaData)
		} else {
			DB.Table("quota_data").Create(quotaData)
		}
	}
	CacheQuotaData = make(map[string]*QuotaData)
	common.SysLog(fmt.Sprintf("保存数据看板数据成功，共保存%d条数据", size))
}

func increaseQuotaData(quotaData *QuotaData) {
	err := DB.Table("quota_data").
		Where("user_id = ? and username = ? and model_name = ? and created_at = ? and use_group = ? and token_id = ? and channel_id = ? and node_name = ?",
			quotaData.UserID, quotaData.Username, quotaData.ModelName, quotaData.CreatedAt, quotaData.UseGroup, quotaData.TokenID, quotaData.ChannelID, quotaData.NodeName).
		Updates(map[string]interface{}{
			"count":      gorm.Expr("count + ?", quotaData.Count),
			"quota":      gorm.Expr("quota + ?", quotaData.Quota),
			"token_used": gorm.Expr("token_used + ?", quotaData.TokenUsed),
		}).Error
	if err != nil {
		common.SysLog(fmt.Sprintf("increaseQuotaData error: %s", err))
	}
}

func GetQuotaDataByUsername(username string, startTime int64, endTime int64) (quotaData []*QuotaData, err error) {
	var quotaDatas []*QuotaData
	// 基础 Dashboard 只需要用户、模型和小时维度。quota_data 同时承载 Flow 的
	// node/token/group/channel 细维度，按这里聚合后可避免普通用量图表暴露路由拓扑
	// 或因为同一小时多个 token/channel 被拆成多行而重复展示。
	err = DB.Table("quota_data").
		Select("user_id, username, model_name, created_at, sum(count) as count, sum(quota) as quota, sum(token_used) as token_used").
		Where("username = ? and created_at >= ? and created_at <= ?", username, startTime, endTime).
		Group("user_id, username, model_name, created_at").
		Find(&quotaDatas).Error
	return quotaDatas, err
}

func GetQuotaDataByUserId(userId int, startTime int64, endTime int64) (quotaData []*QuotaData, err error) {
	var quotaDatas []*QuotaData
	// 与按用户名查询保持同一聚合语义。Flow 视图需要细维度时会走 GetFlowQuotaData，
	// 这里不返回 node/token/channel/use_group，避免用户自助 Dashboard 暴露内部路由。
	err = DB.Table("quota_data").
		Select("user_id, username, model_name, created_at, sum(count) as count, sum(quota) as quota, sum(token_used) as token_used").
		Where("user_id = ? and created_at >= ? and created_at <= ?", userId, startTime, endTime).
		Group("user_id, username, model_name, created_at").
		Find(&quotaDatas).Error
	return quotaDatas, err
}

func GetQuotaDataGroupByUser(startTime int64, endTime int64) (quotaData []*QuotaData, err error) {
	var quotaDatas []*QuotaData
	err = DB.Table("quota_data").
		Select("username, created_at, sum(count) as count, sum(quota) as quota, sum(token_used) as token_used").
		Where("created_at >= ? and created_at <= ?", startTime, endTime).
		Group("username, created_at").
		Find(&quotaDatas).Error
	return quotaDatas, err
}

func GetAllQuotaDates(startTime int64, endTime int64, username string) (quotaData []*QuotaData, err error) {
	if username != "" {
		return GetQuotaDataByUsername(username, startTime, endTime)
	}
	var quotaDatas []*QuotaData
	// 从quota_data表中查询数据
	// only select model_name, sum(count) as count, sum(quota) as quota, model_name, created_at from quota_data group by model_name, created_at;
	//err = DB.Table("quota_data").Where("created_at >= ? and created_at <= ?", startTime, endTime).Find(&quotaDatas).Error
	err = DB.Table("quota_data").Select("model_name, sum(count) as count, sum(quota) as quota, sum(token_used) as token_used, created_at").Where("created_at >= ? and created_at <= ?", startTime, endTime).Group("model_name, created_at").Find(&quotaDatas).Error
	return quotaDatas, err
}

// GetFlowQuotaData 按调用者角色返回流量账本聚合数据。
func GetFlowQuotaData(startTime int64, endTime int64, username string, userID int, role int) ([]*FlowQuotaData, error) {
	switch {
	case role >= common.RoleRootUser:
		return getRootFlowQuotaData(startTime, endTime, username)
	case role >= common.RoleAdminUser:
		return getAdminFlowQuotaData(startTime, endTime, username)
	default:
		return getSelfFlowQuotaData(startTime, endTime, userID)
	}
}

func flowQuotaBaseQuery(startTime int64, endTime int64) *gorm.DB {
	return DB.Table("quota_data").
		Where("use_group <> ''").
		Where("created_at >= ? and created_at <= ?", startTime, endTime)
}

func getSelfFlowQuotaData(startTime int64, endTime int64, userID int) ([]*FlowQuotaData, error) {
	rows := make([]*FlowQuotaData, 0)
	err := flowQuotaBaseQuery(startTime, endTime).
		Select("token_id, use_group, model_name, sum(count) as count, sum(quota) as quota, sum(token_used) as token_used").
		Where("user_id = ?", userID).
		Group("token_id, use_group, model_name").
		Order("quota DESC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, fillFlowTokenNames(rows)
}

func getAdminFlowQuotaData(startTime int64, endTime int64, username string) ([]*FlowQuotaData, error) {
	rows := make([]*FlowQuotaData, 0)
	query := flowQuotaBaseQuery(startTime, endTime).
		Select("user_id, username, use_group, model_name, channel_id, sum(count) as count, sum(quota) as quota, sum(token_used) as token_used")
	if username != "" {
		query = query.Where("username = ?", username)
	}
	err := query.
		Group("user_id, username, use_group, model_name, channel_id").
		Order("quota DESC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, fillFlowChannelNames(rows)
}

func getRootFlowQuotaData(startTime int64, endTime int64, username string) ([]*FlowQuotaData, error) {
	rows := make([]*FlowQuotaData, 0)
	query := flowQuotaBaseQuery(startTime, endTime).
		Select("user_id, username, node_name, token_id, use_group, model_name, channel_id, sum(count) as count, sum(quota) as quota, sum(token_used) as token_used")
	if username != "" {
		query = query.Where("username = ?", username)
	}
	err := query.
		Group("user_id, username, node_name, token_id, use_group, model_name, channel_id").
		Order("quota DESC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	if err := fillFlowTokenNames(rows); err != nil {
		return rows, err
	}
	return rows, fillFlowChannelNames(rows)
}

func fillFlowTokenNames(rows []*FlowQuotaData) error {
	tokenIDSet := make(map[int]struct{})
	tokenIDs := make([]int, 0)
	for _, row := range rows {
		if row.TokenID == 0 {
			continue
		}
		if _, ok := tokenIDSet[row.TokenID]; ok {
			continue
		}
		tokenIDSet[row.TokenID] = struct{}{}
		tokenIDs = append(tokenIDs, row.TokenID)
	}
	if len(tokenIDs) == 0 {
		return nil
	}

	var tokens []struct {
		Id   int    `gorm:"column:id"`
		Name string `gorm:"column:name"`
	}
	if err := DB.Model(&Token{}).Select("id, name").Where("id IN ?", tokenIDs).Find(&tokens).Error; err != nil {
		return err
	}
	tokenNameByID := make(map[int]string, len(tokens))
	for _, token := range tokens {
		tokenNameByID[token.Id] = token.Name
	}
	for _, row := range rows {
		if name := tokenNameByID[row.TokenID]; name != "" {
			row.TokenName = name
		}
	}
	return nil
}

func fillFlowChannelNames(rows []*FlowQuotaData) error {
	channelIDSet := make(map[int]struct{})
	channelIDs := make([]int, 0)
	for _, row := range rows {
		if row.ChannelID == 0 {
			continue
		}
		if _, ok := channelIDSet[row.ChannelID]; ok {
			continue
		}
		channelIDSet[row.ChannelID] = struct{}{}
		channelIDs = append(channelIDs, row.ChannelID)
	}
	if len(channelIDs) == 0 {
		return nil
	}

	channelNameByID := make(map[int]string, len(channelIDs))
	if common.MemoryCacheEnabled {
		for _, channelID := range channelIDs {
			if channel, err := CacheGetChannel(channelID); err == nil {
				channelNameByID[channelID] = channel.Name
			}
		}
	} else {
		var channels []struct {
			Id   int    `gorm:"column:id"`
			Name string `gorm:"column:name"`
		}
		if err := DB.Table("channels").Select("id, name").Where("id IN ?", channelIDs).Find(&channels).Error; err != nil {
			return err
		}
		for _, channel := range channels {
			channelNameByID[channel.Id] = channel.Name
		}
	}
	for _, row := range rows {
		if name := channelNameByID[row.ChannelID]; name != "" {
			row.ChannelName = name
			continue
		}
		if row.ChannelID > 0 {
			row.ChannelName = fmt.Sprintf("channel-%d", row.ChannelID)
		}
	}
	return nil
}
