package upstreamaccount

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
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

var (
	previewConsumeMu    sync.Mutex
	previewConsumeLocks = map[string]*previewConsumeLock{}
)

type previewConsumeLock struct {
	mu   sync.Mutex
	refs int
}

// Preview 使用临时账号密码或已保存的上游凭据读取目标平台快照，并生成可由前端展示的预览结果。
func Preview(ctx context.Context, req PreviewRequest) (*PreviewResult, error) {
	req.Platform = NormalizePlatform(req.Platform)
	req.AuthMode = NormalizeAuthMode(req.AuthMode)
	if req.AuthMode == AuthModePassword && strings.TrimSpace(req.Password) == "" && req.ChannelID > 0 {
		credential, ok, err := loadChannelSyncCredential(req.ChannelID)
		if err != nil {
			return nil, err
		}
		if ok {
			req.Credential = credential
		}
	}
	var err error
	req.Credential, err = PrepareImportedCredential(req.Credential)
	if err != nil {
		return nil, err
	}
	if req.Platform == "" {
		return nil, fmt.Errorf("上游平台不能为空")
	}
	if strings.TrimSpace(req.BaseURL) == "" {
		return nil, fmt.Errorf("上游平台地址不能为空")
	}
	if credentialNeedsPassword(req.Credential) {
		return nil, fmt.Errorf("上游平台密码不能为空")
	}
	client, err := NewPlatformClient(req.Platform)
	if err != nil {
		return nil, err
	}
	switch req.Platform {
	case PlatformNewAPI:
		client := NewNewAPIClient(nil)
		snapshot, challengeRecord, err := client.BeginPreview(ctx, req.Credential)
		if err != nil {
			return nil, err
		}
		if challengeRecord != nil {
			attachStoredCredentialToChallenge(challengeRecord, req.Credential)
			challenge, err := saveAuthChallenge(*challengeRecord)
			if err != nil {
				return nil, err
			}
			return &PreviewResult{
				ExpiresAt: challenge.ExpiresAt,
				Challenge: challenge,
			}, nil
		}
		ApplyRatioConversion(snapshot, req.RatioConversion)
		syncSnapshotKeyModels(ctx, req.ChannelID, snapshot, nil)
		ApplySuggestions(snapshot)
		attachStoredCredential(snapshot, req.Credential)
		return SavePreviewSnapshot(snapshot)
	case PlatformSub2API:
		client := NewSub2APIClient(nil)
		snapshot, challengeRecord, err := client.BeginPreview(ctx, req.Credential)
		if err != nil {
			return nil, err
		}
		if challengeRecord != nil {
			attachStoredCredentialToChallenge(challengeRecord, req.Credential)
			challenge, err := saveAuthChallenge(*challengeRecord)
			if err != nil {
				return nil, err
			}
			return &PreviewResult{
				ExpiresAt: challenge.ExpiresAt,
				Challenge: challenge,
			}, nil
		}
		ApplyRatioConversion(snapshot, req.RatioConversion)
		syncSnapshotKeyModels(ctx, req.ChannelID, snapshot, nil)
		ApplySuggestions(snapshot)
		attachStoredCredential(snapshot, req.Credential)
		return SavePreviewSnapshot(snapshot)
	}
	snapshot, err := client.FetchSnapshot(ctx, req.Credential)
	if err != nil {
		return nil, err
	}
	ApplyRatioConversion(snapshot, req.RatioConversion)
	ApplySuggestions(snapshot)
	attachStoredCredential(snapshot, req.Credential)
	return SavePreviewSnapshot(snapshot)
}

// CompletePreview2FA 使用短期 challenge 完成上游平台二次验证，并生成普通预览快照。
//
// 如果第一次登录时已经保存了加密凭据，这里会把它继续挂到新的预览快照上，供后续
// 创建或刷新流程复用。
func CompletePreview2FA(ctx context.Context, req Preview2FARequest) (*PreviewResult, error) {
	if strings.TrimSpace(req.Code) == "" {
		return nil, fmt.Errorf("验证码不能为空")
	}
	record, err := consumeAuthChallenge(req.ChallengeID)
	if err != nil {
		return nil, err
	}
	switch NormalizePlatform(record.Platform) {
	case PlatformNewAPI:
		client := NewNewAPIClient(nil)
		snapshot, err := client.Complete2FA(ctx, *record, req.Code)
		if err != nil {
			return nil, err
		}
		ApplyRatioConversion(snapshot, req.RatioConversion)
		syncSnapshotKeyModels(ctx, 0, snapshot, nil)
		ApplySuggestions(snapshot)
		attachStoredCredentialFromChallenge(snapshot, record)
		return SavePreviewSnapshot(snapshot)
	case PlatformSub2API:
		client := NewSub2APIClient(nil)
		snapshot, err := client.Complete2FA(ctx, *record, req.Code)
		if err != nil {
			return nil, err
		}
		ApplyRatioConversion(snapshot, req.RatioConversion)
		syncSnapshotKeyModels(ctx, 0, snapshot, nil)
		ApplySuggestions(snapshot)
		attachStoredCredentialFromChallenge(snapshot, record)
		return SavePreviewSnapshot(snapshot)
	default:
		return nil, fmt.Errorf("不支持的二次验证平台：%s", record.Platform)
	}
}

// loadChannelSyncCredential 从数据库读取同步渠道里已保存的上游凭据。
//
// 这个函数只在前端选择“复用已保存登录”时使用，返回值仍然是普通 Credential，
// 但 Password 已经是解密后的临时内存值，不会回写给前端。
func loadChannelSyncCredential(channelID int) (Credential, bool, error) {
	if channelID <= 0 {
		return Credential{}, false, nil
	}
	var channel model.Channel
	if err := model.DB.Where("id = ?", channelID).First(&channel).Error; err != nil {
		return Credential{}, false, err
	}
	credential, ok, err := ReadChannelSyncCredential(channel.OtherSettings)
	if err != nil {
		return Credential{}, false, err
	}
	if !ok {
		return Credential{}, false, nil
	}
	return credential, true, nil
}

// SavePreviewSnapshot 保存包含完整 Key 的短期快照，并返回脱敏预览结果。
func SavePreviewSnapshot(snapshot *Snapshot) (*PreviewResult, error) {
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

func attachStoredCredential(snapshot *Snapshot, credential Credential) {
	stored, err := buildStoredCredential(snapshot, credential)
	if err != nil {
		common.SysLog("failed to store upstream account credential metadata: " + err.Error())
		return
	}
	snapshot.StoredCredential = stored
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

// ConsumePreviewRecord 读取并删除完整预览快照，保证包含完整 Key 的快照只能被创建流程使用一次。
func ConsumePreviewRecord(previewID string) (*PreviewRecord, error) {
	previewID = strings.TrimSpace(previewID)
	if previewID == "" {
		return nil, fmt.Errorf("preview_id 不能为空")
	}

	unlock := lockPreviewConsume(previewID)
	defer unlock()

	record, found, err := previewCache.GetAndDelete(previewID)
	if err != nil {
		return nil, err
	}
	if !found || record.Snapshot == nil || record.ExpiresAt < time.Now().Unix() {
		return nil, fmt.Errorf("预览快照不存在或已过期，请重新同步")
	}
	return &record, nil
}

func lockPreviewConsume(previewID string) func() {
	previewConsumeMu.Lock()
	lock := previewConsumeLocks[previewID]
	if lock == nil {
		lock = &previewConsumeLock{}
		previewConsumeLocks[previewID] = lock
	}
	lock.refs++
	previewConsumeMu.Unlock()

	lock.mu.Lock()
	return func() {
		lock.mu.Unlock()
		previewConsumeMu.Lock()
		lock.refs--
		if lock.refs == 0 && previewConsumeLocks[previewID] == lock {
			delete(previewConsumeLocks, previewID)
		}
		previewConsumeMu.Unlock()
	}
}

// sanitizeSnapshot 返回可发送到前端的快照副本。
func sanitizeSnapshot(snapshot *Snapshot) *Snapshot {
	if snapshot == nil {
		return nil
	}
	copySnapshot := *snapshot
	copySnapshot.StoredCredential = nil
	copySnapshot.AuthSession = nil
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
