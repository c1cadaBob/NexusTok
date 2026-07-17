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
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	externalID = strings.TrimSpace(externalID)
	if platform == "" || baseURL == "" || externalID == "" {
		return ""
	}
	return platform + "|" + baseURL + "|" + externalID
}
