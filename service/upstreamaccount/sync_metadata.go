package upstreamaccount

import (
	"encoding/hex"
	"strings"

	"github.com/c1cada/NexusTok/common"
)

const upstreamAccountSyncMetadataKey = "upstream_account_sync"

type syncMetadata struct {
	Platform   string `json:"platform,omitempty"`
	BaseURL    string `json:"base_url,omitempty"`
	ExternalID string `json:"external_id,omitempty"`
	KeyDigest  string `json:"key_digest,omitempty"`
	SyncedAt   int64  `json:"synced_at,omitempty"`
}

func keyDigest(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	return hex.EncodeToString(common.Sha256Raw([]byte(key)))
}

func mergeChannelSyncMetadata(existing string, snapshot *Snapshot) string {
	var data map[string]any
	if strings.TrimSpace(existing) != "" {
		_ = common.UnmarshalJsonStr(existing, &data)
	}
	if data == nil {
		data = map[string]any{}
	}
	data[upstreamAccountSyncMetadataKey] = map[string]any{
		"platform":  snapshot.Platform,
		"base_url":  snapshot.BaseURL,
		"synced_at": common.GetTimestamp(),
	}
	bytes, err := common.Marshal(data)
	if err != nil {
		return existing
	}
	return string(bytes)
}

func mergeAccountSyncMetadata(existing string, snapshot *Snapshot, key SyncedKey) string {
	var data map[string]any
	if strings.TrimSpace(existing) != "" {
		_ = common.UnmarshalJsonStr(existing, &data)
	}
	if data == nil {
		data = map[string]any{}
	}
	data[upstreamAccountSyncMetadataKey] = syncMetadata{
		Platform:   snapshot.Platform,
		BaseURL:    snapshot.BaseURL,
		ExternalID: key.ExternalID,
		KeyDigest:  keyDigest(key.Key),
		SyncedAt:   common.GetTimestamp(),
	}
	bytes, err := common.Marshal(data)
	if err != nil {
		return existing
	}
	return string(bytes)
}

// PreserveAccountSyncMetadata 在账号本地 settings 被手动更新时保留同步身份。
//
// 同步账号的刷新匹配依赖 `upstream_account_sync` 中的 platform、base_url、
// external_id 和 key_digest。管理员在渠道账号编辑页保存本地配置时，前端可能只提交
// 业务 settings；若直接覆盖会丢失同步身份，下一次刷新就无法按 external_id 更新原账号，
// 只能创建新账号。该函数只在旧 settings 已有同步身份、且新 settings 未显式携带同步身份
// 时合并；如果新 settings 不是 JSON，则保持原输入，避免改变既有容错语义。
func PreserveAccountSyncMetadata(existing string, next string) string {
	var existingData map[string]any
	if strings.TrimSpace(existing) == "" {
		return next
	}
	if err := common.UnmarshalJsonStr(existing, &existingData); err != nil {
		return next
	}
	rawMetadata, ok := existingData[upstreamAccountSyncMetadataKey]
	if !ok {
		return next
	}

	nextData := map[string]any{}
	if strings.TrimSpace(next) != "" {
		if err := common.UnmarshalJsonStr(next, &nextData); err != nil {
			return next
		}
	}
	if _, ok := nextData[upstreamAccountSyncMetadataKey]; ok {
		return next
	}
	nextData[upstreamAccountSyncMetadataKey] = rawMetadata
	bytes, err := common.Marshal(nextData)
	if err != nil {
		return next
	}
	return string(bytes)
}

func readAccountSyncMetadata(settings string) syncMetadata {
	var data map[string]any
	if strings.TrimSpace(settings) == "" {
		return syncMetadata{}
	}
	if err := common.UnmarshalJsonStr(settings, &data); err != nil {
		return syncMetadata{}
	}
	raw, ok := data[upstreamAccountSyncMetadataKey]
	if !ok {
		return syncMetadata{}
	}
	bytes, err := common.Marshal(raw)
	if err != nil {
		return syncMetadata{}
	}
	var metadata syncMetadata
	if err := common.Unmarshal(bytes, &metadata); err != nil {
		return syncMetadata{}
	}
	return metadata
}

func syncIdentityKey(platform string, baseURL string, externalID string) string {
	platform = NormalizePlatform(platform)
	baseURL = normalizeSyncMetadataBaseURL(platform, baseURL)
	externalID = strings.TrimSpace(externalID)
	if platform == "" || baseURL == "" || externalID == "" {
		return ""
	}
	return platform + "|" + baseURL + "|" + externalID
}

func sameSyncSourceBaseURL(platform string, left string, right string) bool {
	return normalizeSyncMetadataBaseURL(platform, left) == normalizeSyncMetadataBaseURL(platform, right)
}

func normalizeSyncMetadataBaseURL(platform string, raw string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(raw), "/")
	if NormalizePlatform(platform) == PlatformSub2API {
		return strings.TrimRight(strings.TrimSpace(normalizeSub2APIBaseURL(trimmed)), "/")
	}
	return trimmed
}
