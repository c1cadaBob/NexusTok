package upstreamaccount

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/pkg/cachex"

	"github.com/samber/hot"
)

const (
	previewCacheNamespace = "upstream-account-preview"
	previewTTL            = 10 * time.Minute
)

var previewCache = cachex.NewHybridCache[PreviewRecord](cachex.HybridCacheConfig[PreviewRecord]{
	Namespace:    cachex.Namespace(previewCacheNamespace),
	Redis:        common.RDB,
	RedisCodec:   cachex.JSONCodec[PreviewRecord]{},
	RedisEnabled: func() bool { return common.RedisEnabled && common.RDB != nil },
	Memory: func() *hot.HotCache[string, PreviewRecord] {
		return hot.NewHotCache[string, PreviewRecord](hot.LRU, 256).
			WithTTL(previewTTL).
			WithJanitor().
			Build()
	},
})

// Preview 使用临时账号密码读取目标平台快照，并生成可由前端展示的预览结果。
func Preview(ctx context.Context, req PreviewRequest) (*PreviewResult, error) {
	req.Platform = NormalizePlatform(req.Platform)
	if req.Platform == "" {
		return nil, fmt.Errorf("上游平台不能为空")
	}
	if strings.TrimSpace(req.BaseURL) == "" {
		return nil, fmt.Errorf("上游平台地址不能为空")
	}
	if strings.TrimSpace(req.Password) == "" {
		return nil, fmt.Errorf("上游平台密码不能为空")
	}
	client, err := NewPlatformClient(req.Platform)
	if err != nil {
		return nil, err
	}
	snapshot, err := client.FetchSnapshot(ctx, req.Credential)
	if err != nil {
		return nil, err
	}
	id := common.GetUUID()
	expiresAt := time.Now().Add(previewTTL).Unix()
	record := PreviewRecord{
		ID:        id,
		ExpiresAt: expiresAt,
		Snapshot:  snapshot,
	}
	if err := previewCache.SetWithTTL(id, record, previewTTL); err != nil {
		return nil, fmt.Errorf("保存预览快照失败：%w", err)
	}
	return &PreviewResult{
		PreviewID: id,
		ExpiresAt: expiresAt,
		Snapshot:  sanitizeSnapshot(snapshot),
	}, nil
}

// GetPreviewRecord 读取后端保存的完整预览快照。
func GetPreviewRecord(previewID string) (*PreviewRecord, error) {
	previewID = strings.TrimSpace(previewID)
	if previewID == "" {
		return nil, fmt.Errorf("preview_id 不能为空")
	}
	record, found, err := previewCache.Get(previewID)
	if err != nil {
		return nil, err
	}
	if !found || record.Snapshot == nil || record.ExpiresAt < time.Now().Unix() {
		return nil, fmt.Errorf("预览快照不存在或已过期，请重新同步")
	}
	return &record, nil
}

// sanitizeSnapshot 返回可发送到前端的快照副本。
func sanitizeSnapshot(snapshot *Snapshot) *Snapshot {
	if snapshot == nil {
		return nil
	}
	copySnapshot := *snapshot
	copySnapshot.Groups = append([]SyncedGroup(nil), snapshot.Groups...)
	copySnapshot.Keys = make([]SyncedKey, len(snapshot.Keys))
	for i, key := range snapshot.Keys {
		if key.MaskedKey == "" {
			key.MaskedKey = maskKey(key.Key)
		}
		key.Key = ""
		copySnapshot.Keys[i] = key
	}
	if snapshot.Warnings != nil {
		copySnapshot.Warnings = append([]string(nil), snapshot.Warnings...)
	}
	return &copySnapshot
}
