package service

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/c1cada/NexusTok/common"
)

const syncedChannelAccountMetadataKey = "upstream_account_sync"

// syncedChannelAccountAutoCheckMetadata 是 service 层处理同步密钥请求结果时需要读取的最小元数据。
//
// 完整的上游账号同步元数据仍由 service/upstreamaccount 维护。这里故意只保留请求
// 成功/失败计数需要的字段，避免 service 包反向 import 上游同步包后形成循环依赖。
type syncedChannelAccountAutoCheckMetadata struct {
	RatioConversion     float64
	EffectiveRatio      float64
	FailureCount        int
	LastCheckedAt       int64
	LastSuccessAt       int64
	LastError           string
	LastStatus          string
	DisabledByAutoCheck bool
	DisabledAt          int64
}

// channelAccountHasUpstreamSyncMetadata 判断账号是否由上游账号同步流程维护。
//
// 判断条件与 upstreamaccount.HasAccountSyncMetadata 保持同一口径：必须存在同步来源
// 身份字段，而不是仅凭倍率或错误状态推断。这样同步渠道里混入的人工账号不会被
// 便宜优先调度和自动失败禁用逻辑误处理。
func channelAccountHasUpstreamSyncMetadata(settings string) bool {
	metadata, ok := syncedChannelAccountMetadataMap(settings)
	if !ok {
		return false
	}
	return strings.TrimSpace(syncedStringFromMetadata(metadata, "platform")) != "" ||
		strings.TrimSpace(syncedStringFromMetadata(metadata, "base_url")) != "" ||
		strings.TrimSpace(syncedStringFromMetadata(metadata, "management_base_url")) != "" ||
		strings.TrimSpace(syncedStringFromMetadata(metadata, "relay_base_url")) != "" ||
		strings.TrimSpace(syncedStringFromMetadata(metadata, "external_id")) != "" ||
		strings.TrimSpace(syncedStringFromMetadata(metadata, "key_digest")) != "" ||
		syncedInt64FromMetadata(metadata, "synced_at") > 0
}

// readSyncedChannelAccountAutoCheckMetadata 读取同步密钥自动检测状态。
func readSyncedChannelAccountAutoCheckMetadata(settings string) syncedChannelAccountAutoCheckMetadata {
	metadata, _ := syncedChannelAccountMetadataMap(settings)
	return syncedChannelAccountAutoCheckMetadata{
		RatioConversion:     syncedFloat64FromMetadata(metadata, "ratio_conversion"),
		EffectiveRatio:      syncedFloat64FromMetadata(metadata, "effective_ratio"),
		FailureCount:        syncedIntFromMetadata(metadata, "auto_check_failure_count"),
		LastCheckedAt:       syncedInt64FromMetadata(metadata, "auto_check_last_checked_at"),
		LastSuccessAt:       syncedInt64FromMetadata(metadata, "auto_check_last_success_at"),
		LastError:           syncedStringFromMetadata(metadata, "auto_check_last_error"),
		LastStatus:          syncedStringFromMetadata(metadata, "auto_check_last_status"),
		DisabledByAutoCheck: syncedBoolFromMetadata(metadata, "auto_check_disabled_by_auto_check"),
		DisabledAt:          syncedInt64FromMetadata(metadata, "auto_check_disabled_at"),
	}
}

// applySyncedChannelAccountAutoCheckSuccess 写入一次真实请求成功后的自动检测状态。
func applySyncedChannelAccountAutoCheckSuccess(settings string) string {
	now := common.GetTimestamp()
	return mutateSyncedChannelAccountMetadata(settings, func(metadata map[string]any) {
		metadata["auto_check_last_checked_at"] = now
		metadata["auto_check_last_success_at"] = now
		metadata["auto_check_failure_count"] = 0
		metadata["auto_check_last_error"] = ""
		metadata["auto_check_last_status"] = "success"
		metadata["auto_check_disabled_by_auto_check"] = false
		metadata["auto_check_disabled_at"] = 0
	})
}

// applySyncedChannelAccountAutoCheckFailure 写入一次真实请求失败后的自动检测状态。
func applySyncedChannelAccountAutoCheckFailure(settings string, failureCount int, errorText string, disabledByAutoCheck bool) string {
	now := common.GetTimestamp()
	errorText = syncedTruncateText(common.MaskSensitiveInfo(errorText), 240)
	return mutateSyncedChannelAccountMetadata(settings, func(metadata map[string]any) {
		metadata["auto_check_last_checked_at"] = now
		metadata["auto_check_failure_count"] = failureCount
		metadata["auto_check_last_error"] = errorText
		metadata["auto_check_last_status"] = "failed"
		if disabledByAutoCheck {
			metadata["auto_check_disabled_by_auto_check"] = true
			if syncedInt64FromMetadata(metadata, "auto_check_disabled_at") == 0 {
				metadata["auto_check_disabled_at"] = now
			}
		}
	})
}

func syncedChannelAccountMetadataMap(settings string) (map[string]any, bool) {
	if strings.TrimSpace(settings) == "" {
		return nil, false
	}
	data := map[string]any{}
	if err := common.UnmarshalJsonStr(settings, &data); err != nil {
		return nil, false
	}
	raw, ok := data[syncedChannelAccountMetadataKey]
	if !ok || raw == nil {
		return nil, false
	}
	rawBytes, err := common.Marshal(raw)
	if err != nil {
		return nil, false
	}
	metadata := map[string]any{}
	if err := common.Unmarshal(rawBytes, &metadata); err != nil {
		return nil, false
	}
	return metadata, true
}

func mutateSyncedChannelAccountMetadata(settings string, mutate func(metadata map[string]any)) string {
	data := map[string]any{}
	if strings.TrimSpace(settings) != "" {
		if err := common.UnmarshalJsonStr(settings, &data); err != nil {
			return settings
		}
	}
	metadata, _ := syncedChannelAccountMetadataMap(settings)
	if metadata == nil {
		metadata = map[string]any{}
	}
	mutate(metadata)
	data[syncedChannelAccountMetadataKey] = metadata
	bytes, err := common.Marshal(data)
	if err != nil {
		return settings
	}
	return string(bytes)
}

func syncedStringFromMetadata(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	value, ok := metadata[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
}

func syncedFloat64FromMetadata(metadata map[string]any, key string) float64 {
	if metadata == nil {
		return 0
	}
	value, ok := metadata[key]
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case string:
		parsed, _ := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return parsed
	default:
		parsed, _ := strconv.ParseFloat(strings.TrimSpace(fmt.Sprint(typed)), 64)
		return parsed
	}
}

func syncedIntFromMetadata(metadata map[string]any, key string) int {
	return int(syncedInt64FromMetadata(metadata, key))
}

func syncedInt64FromMetadata(metadata map[string]any, key string) int64 {
	if metadata == nil {
		return 0
	}
	value, ok := metadata[key]
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case int64:
		return typed
	case int:
		return int64(typed)
	case float64:
		return int64(typed)
	case float32:
		return int64(typed)
	case string:
		parsed, _ := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return parsed
	default:
		parsed, _ := strconv.ParseInt(strings.TrimSpace(fmt.Sprint(typed)), 10, 64)
		return parsed
	}
}

func syncedBoolFromMetadata(metadata map[string]any, key string) bool {
	if metadata == nil {
		return false
	}
	value, ok := metadata[key]
	if !ok || value == nil {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true") || strings.TrimSpace(typed) == "1"
	default:
		return fmt.Sprint(typed) == "1"
	}
}

func syncedTruncateText(text string, maxRunes int) string {
	text = strings.TrimSpace(text)
	if maxRunes <= 0 {
		return text
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes]) + "..."
}
