package model

import (
	"encoding/json"
	"errors"

	"github.com/c1cadaBob/NexusTok/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	PlatformSiteUsageCostSourceUpstream  = "upstream"
	PlatformSiteUsageCostSourceEstimated = "estimated"

	PlatformSiteUsageCostFallbackReasonKey = "upstream_cost_fallback_reason"
	PlatformSiteUsageCostFallbackStatusKey = "upstream_cost_fallback_status"
	PlatformSiteUsageCostFallbackCodeKey   = "upstream_cost_fallback_code"

	PlatformSiteUsageCostFallbackRequestError         = "request_error"
	PlatformSiteUsageCostFallbackAuthenticationFailed = "authentication_failed"
	PlatformSiteUsageCostFallbackPermissionDenied     = "permission_denied"
	PlatformSiteUsageCostFallbackHTTPFailed           = "http_request_failed"
	PlatformSiteUsageCostFallbackEmptyResult          = "empty_result"
	PlatformSiteUsageCostFallbackRequestIDNoMatch     = "request_id_no_match"
	PlatformSiteUsageCostFallbackModelTimeNoMatch     = "model_or_time_no_match"
	PlatformSiteUsageCostFallbackAmbiguousMatch       = "ambiguous_match"
	PlatformSiteUsageCostFallbackInvalidRecord        = "invalid_cost_record"

	PlatformSiteUsageCostWindowComplete = "complete"
	PlatformSiteUsageCostWindowRetry    = "retry"
)

type PlatformSiteUsageCost struct {
	ID                uint   `json:"id" gorm:"primaryKey"`
	RequestID         string `json:"request_id" gorm:"type:varchar(64);not null;uniqueIndex"`
	ChannelID         int    `json:"channel_id" gorm:"not null;index"`
	UpstreamRequestID string `json:"upstream_request_id,omitempty" gorm:"type:varchar(128);index"`
	Cost              int64  `json:"cost" gorm:"bigint;not null"`
	Source            string `json:"source" gorm:"type:varchar(16);not null;index"`
	UpdatedAt         int64  `json:"updated_at" gorm:"bigint"`
}

type PlatformSiteUsageCostWindow struct {
	ID          uint   `json:"id" gorm:"primaryKey"`
	ChannelID   int    `json:"channel_id" gorm:"not null;uniqueIndex:idx_platform_usage_cost_window,priority:1"`
	WindowStart int64  `json:"window_start" gorm:"bigint;not null;uniqueIndex:idx_platform_usage_cost_window,priority:2"`
	Attempts    int    `json:"attempts" gorm:"not null"`
	NextRetryAt int64  `json:"next_retry_at" gorm:"bigint;index"`
	Status      string `json:"status" gorm:"type:varchar(16);not null;index"`
	UpdatedAt   int64  `json:"updated_at" gorm:"bigint"`
}

func FindPlatformSiteUsageCost(requestID string) (*PlatformSiteUsageCost, error) {
	if requestID == "" {
		return nil, nil
	}
	var cost PlatformSiteUsageCost
	err := DB.Where("request_id = ?", requestID).Take(&cost).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &cost, err
}

func UpsertPlatformSiteUsageCost(cost PlatformSiteUsageCost) error {
	if cost.RequestID == "" || cost.ChannelID <= 0 ||
		cost.Cost < 0 ||
		(cost.Source != PlatformSiteUsageCostSourceUpstream &&
			cost.Source != PlatformSiteUsageCostSourceEstimated) {
		return errors.New("平台站点成本记录无效")
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		var existing PlatformSiteUsageCost
		err := lockForUpdate(tx).Where("request_id = ?", cost.RequestID).Take(&existing).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return tx.Create(&cost).Error
		}
		if existing.Source == PlatformSiteUsageCostSourceUpstream &&
			cost.Source != PlatformSiteUsageCostSourceUpstream {
			return nil
		}
		cost.ID = existing.ID
		return tx.Model(&existing).Updates(map[string]any{
			"channel_id":          cost.ChannelID,
			"upstream_request_id": cost.UpstreamRequestID,
			"cost":                cost.Cost,
			"source":              cost.Source,
			"updated_at":          cost.UpdatedAt,
		}).Error
	})
}

func GetPlatformSiteUsageCosts(requestIDs []string) (map[string]PlatformSiteUsageCost, error) {
	result := make(map[string]PlatformSiteUsageCost, len(requestIDs))
	if len(requestIDs) == 0 {
		return result, nil
	}
	var costs []PlatformSiteUsageCost
	if err := DB.Where("request_id IN ?", requestIDs).Find(&costs).Error; err != nil {
		return nil, err
	}
	for i := range costs {
		result[costs[i].RequestID] = costs[i]
	}
	return result, nil
}

func GetPlatformSiteUsageCostAccount(channelID int) (*PlatformSiteAccount, error) {
	if channelID <= 0 {
		return nil, nil
	}
	var account PlatformSiteAccount
	err := DB.Where("channel_id = ?", channelID).Take(&account).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &account, err
}

func AttachPlatformSiteUsageCosts(logs []*Log) error {
	requestIDs := make([]string, 0, len(logs))
	for _, log := range logs {
		if log != nil &&
			(log.Type == LogTypeConsume || log.Type == LogTypeError) &&
			log.RequestId != "" {
			requestIDs = append(requestIDs, log.RequestId)
		}
	}
	costs, err := GetPlatformSiteUsageCosts(requestIDs)
	if err != nil || len(costs) == 0 {
		return err
	}

	for _, log := range logs {
		if log == nil ||
			(log.Type != LogTypeConsume && log.Type != LogTypeError) ||
			log.RequestId == "" {
			continue
		}
		cost, ok := costs[log.RequestId]
		if !ok || cost.ChannelID != log.ChannelId {
			continue
		}
		other := make(map[string]json.RawMessage)
		if log.Other != "" {
			if err := common.UnmarshalJsonStr(log.Other, &other); err != nil || other == nil {
				continue
			}
		}
		adminInfo := make(map[string]json.RawMessage)
		if rawAdminInfo, ok := other[logOtherAdminInfoKey]; ok {
			if err := common.Unmarshal(rawAdminInfo, &adminInfo); err != nil {
				continue
			}
		}
		if adminInfo == nil {
			adminInfo = make(map[string]json.RawMessage)
		}
		costValue, err := common.Marshal(cost.Cost)
		if err != nil {
			return err
		}
		sourceValue, err := common.Marshal(cost.Source)
		if err != nil {
			return err
		}
		adminInfo["upstream_cost"] = costValue
		adminInfo["upstream_cost_source"] = sourceValue
		if cost.Source == PlatformSiteUsageCostSourceUpstream {
			delete(adminInfo, PlatformSiteUsageCostFallbackReasonKey)
			delete(adminInfo, PlatformSiteUsageCostFallbackStatusKey)
			delete(adminInfo, PlatformSiteUsageCostFallbackCodeKey)
		}
		adminInfoJSON, err := common.Marshal(adminInfo)
		if err != nil {
			return err
		}
		other[logOtherAdminInfoKey] = adminInfoJSON
		data, err := common.Marshal(other)
		if err != nil {
			return err
		}
		log.Other = string(data)
	}
	return nil
}

func ListPlatformSiteUsageCostLogs(channelID int, start, end int64) ([]Log, error) {
	var logs []Log
	err := LOG_DB.Where(
		"type = ? AND channel_id = ? AND created_at >= ? AND created_at < ?",
		LogTypeConsume,
		channelID,
		start,
		end,
	).Order("created_at ASC, request_id ASC").Find(&logs).Error
	return logs, err
}

func ListPlatformSiteUsageCostMatchingLogs(channelID int, start, end int64) ([]Log, error) {
	var logs []Log
	err := LOG_DB.Where(
		"type = ? AND channel_id = ? AND created_at >= ? AND created_at <= ?",
		LogTypeConsume,
		channelID,
		start-2,
		end+2,
	).Order("created_at ASC, request_id ASC").Find(&logs).Error
	return logs, err
}

func ListPlatformSiteUsageCostAccounts() ([]PlatformSiteAccount, error) {
	var accounts []PlatformSiteAccount
	err := DB.Where(
		"platform IN ? AND (disabled_at IS NULL OR disabled_at = 0)",
		[]string{PlatformNewAPI, PlatformSub2API},
	).Order("channel_id ASC").Find(&accounts).Error
	return accounts, err
}

func GetPlatformSiteUsageCostWindow(channelID int, windowStart int64) (*PlatformSiteUsageCostWindow, error) {
	var window PlatformSiteUsageCostWindow
	err := DB.Where("channel_id = ? AND window_start = ?", channelID, windowStart).Take(&window).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &window, err
}

func GetLatestPlatformSiteUsageCostWindow(channelID int) (*PlatformSiteUsageCostWindow, error) {
	var window PlatformSiteUsageCostWindow
	err := DB.Where("channel_id = ?", channelID).
		Order("window_start DESC").Take(&window).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &window, err
}

func ListDuePlatformSiteUsageCostWindows(channelID int, now int64, limit int) ([]PlatformSiteUsageCostWindow, error) {
	var windows []PlatformSiteUsageCostWindow
	err := DB.Where("channel_id = ? AND status = ? AND next_retry_at > 0 AND next_retry_at <= ?",
		channelID,
		PlatformSiteUsageCostWindowRetry, now).
		Order("window_start ASC, channel_id ASC").Limit(limit).Find(&windows).Error
	return windows, err
}

func SavePlatformSiteUsageCostWindow(window *PlatformSiteUsageCostWindow, now int64, retryAfter int64) error {
	if window == nil || window.ChannelID <= 0 || window.WindowStart < 0 {
		return errors.New("平台站点成本窗口无效")
	}
	window.Attempts++
	window.UpdatedAt = now
	window.NextRetryAt = 0
	window.Status = PlatformSiteUsageCostWindowComplete
	if retryAfter > 0 {
		window.NextRetryAt = now + retryAfter
		window.Status = PlatformSiteUsageCostWindowRetry
	}
	return DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "channel_id"}, {Name: "window_start"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"attempts",
			"next_retry_at",
			"status",
			"updated_at",
		}),
	}).Create(window).Error
}
