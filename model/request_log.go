package model

import (
	"fmt"

	"github.com/c1cada/NexusTok/common"
)

// RequestLog 请求记录，存储匹配到规则的请求和响应内容
type RequestLog struct {
	Id            int64  `json:"id" gorm:"primaryKey;autoIncrement"`
	RequestRuleId int    `json:"request_rule_id" gorm:"index"`
	RequestId     string `json:"request_id" gorm:"type:varchar(64);index"`
	UserId        int    `json:"user_id" gorm:"index"`
	TokenId       int    `json:"token_id" gorm:"index"`
	ChannelId     int    `json:"channel_id" gorm:"index"`
	ModelName     string `json:"model_name" gorm:"size:128;index"`
	RelayFormat   string `json:"relay_format" gorm:"size:64"`

	RequestBody  string `json:"request_body,omitempty" gorm:"type:text"`
	ResponseBody string `json:"response_body,omitempty" gorm:"type:text"`

	StatusCode int   `json:"status_code" gorm:"default:0"`
	Latency    int   `json:"latency" gorm:"default:0"` // 毫秒
	CreatedAt  int64 `json:"created_at" gorm:"bigint;index"`
}

// InsertRequestLog 异步写入请求记录到日志库
func InsertRequestLog(log *RequestLog) {
	if log.CreatedAt == 0 {
		log.CreatedAt = common.GetTimestamp()
	}
	err := LOG_DB.Create(log).Error
	if err != nil {
		common.SysError(fmt.Sprintf("写入请求记录失败: %v", err))
	}
}

// GetAllRequestLogs 分页获取请求记录，支持多维度过滤
func GetAllRequestLogs(offset int, limit int, ruleId int, userId int, modelName string, relayFormat string, startTime int64, endTime int64) ([]*RequestLog, int64, error) {
	db := LOG_DB.Model(&RequestLog{})
	if ruleId > 0 {
		db = db.Where("request_rule_id = ?", ruleId)
	}
	if userId > 0 {
		db = db.Where("user_id = ?", userId)
	}
	if modelName != "" {
		db = db.Where("model_name = ?", modelName)
	}
	if relayFormat != "" {
		db = db.Where("relay_format = ?", relayFormat)
	}
	if startTime > 0 {
		db = db.Where("created_at >= ?", startTime)
	}
	if endTime > 0 {
		db = db.Where("created_at <= ?", endTime)
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 列表查询不返回 body 内容（节省带宽）
	var logs []*RequestLog
	if err := db.Select("id, request_rule_id, request_id, user_id, token_id, channel_id, model_name, relay_format, status_code, latency, created_at").
		Offset(offset).Limit(limit).Order("id DESC").Find(&logs).Error; err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}

// GetRequestLogDetail 获取单条请求记录的完整内容（包含 body）
func GetRequestLogDetail(id int64) (*RequestLog, error) {
	var log RequestLog
	err := LOG_DB.First(&log, id).Error
	if err != nil {
		return nil, err
	}
	return &log, nil
}

// DeleteRequestLogsBefore 删除指定时间之前的请求记录
func DeleteRequestLogsBefore(timestamp int64) (int64, error) {
	result := LOG_DB.Where("created_at < ?", timestamp).Delete(&RequestLog{})
	return result.RowsAffected, result.Error
}

// DeleteAllRequestLogs 删除所有请求记录
func DeleteAllRequestLogs() (int64, error) {
	result := LOG_DB.Where("1 = 1").Delete(&RequestLog{})
	return result.RowsAffected, result.Error
}
