package upstreamaccount

import (
	"fmt"
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
)

// PreviewKeyModelFetchRequest 是未保存同步密钥“从上游获取模型”使用的安全定位参数。
//
// 前端只能提交 sync_id、external_id、masked_key 和 index 这类脱敏标识，不能提交明文
// API Key。后端会在短期 preview cache 中读取完整快照，并要求所有非空定位字段同时
// 匹配同一个 key，避免管理员页面过期、行顺序变化或重复 masked_key 时误取其他密钥。
type PreviewKeyModelFetchRequest struct {
	SyncID     string `json:"sync_id,omitempty"`
	ExternalID string `json:"external_id,omitempty"`
	MaskedKey  string `json:"masked_key,omitempty"`
	Index      *int   `json:"index,omitempty"`
}

// BuildChannelForPreviewKeyModelFetch 根据 preview cache 中的完整 key 构造一次性渠道副本。
//
// 该函数只服务于配置期模型获取：它不会消费 preview、不会写数据库，也不会把明文 key
// 返回给调用方。返回的 Channel 仅携带当前 key 和快照中的模型调用地址，随后由
// controller 复用现有 provider 模型列表获取逻辑。
func BuildChannelForPreviewKeyModelFetch(previewID string, req PreviewKeyModelFetchRequest) (*model.Channel, error) {
	record, err := GetPreviewRecord(previewID)
	if err != nil {
		return nil, err
	}
	snapshot := record.Snapshot
	if snapshot == nil {
		return nil, fmt.Errorf("预览快照不存在或已过期，请重新同步")
	}
	ApplySyncIDs(snapshot)
	key, err := findPreviewKeyForModelFetch(snapshot.Keys, req)
	if err != nil {
		return nil, err
	}
	fullKey := strings.TrimSpace(key.Key)
	if fullKey == "" {
		return nil, fmt.Errorf("预览密钥缺少完整 key，请重新同步上游账号")
	}

	baseURL := normalizeSyncMetadataBaseURL(
		snapshot.Platform,
		firstNonEmpty(snapshotRelayBaseURL(snapshot), snapshot.BaseURL),
	)
	channel := &model.Channel{
		Type:   resolveSyncedChannelType(snapshot, 0),
		Key:    fullKey,
		Name:   firstNonEmpty(key.Name, key.MaskedKey, "preview-key-model-fetch"),
		Status: common.ChannelStatusEnabled,
		Group:  "default",
		Models: strings.Join(key.Models, ","),
	}
	if baseURL != "" {
		channel.BaseURL = common.GetPointer(baseURL)
	}
	return channel, nil
}

// findPreviewKeyForModelFetch 根据脱敏定位字段从缓存快照中找出唯一密钥。
//
// index 是前端当前行的强约束；sync_id、external_id 和 masked_key 是防止行错位的
// 交叉校验。所有已提交的字段都必须同时匹配，同一请求不会在匹配失败时猜测其他 key。
func findPreviewKeyForModelFetch(keys []SyncedKey, req PreviewKeyModelFetchRequest) (SyncedKey, error) {
	if strings.TrimSpace(req.SyncID) == "" &&
		strings.TrimSpace(req.ExternalID) == "" &&
		strings.TrimSpace(req.MaskedKey) == "" &&
		req.Index == nil {
		return SyncedKey{}, fmt.Errorf("预览密钥定位参数不能为空")
	}
	if req.Index != nil && (*req.Index < 0 || *req.Index >= len(keys)) {
		return SyncedKey{}, fmt.Errorf("预览密钥索引无效，请重新同步")
	}

	var matched []SyncedKey
	for index, key := range keys {
		if !previewKeyMatchesLocator(key, index, req) {
			continue
		}
		matched = append(matched, key)
	}
	if len(matched) == 0 {
		return SyncedKey{}, fmt.Errorf("预览密钥不存在或已过期，请重新同步")
	}
	if len(matched) > 1 {
		return SyncedKey{}, fmt.Errorf("预览密钥定位不唯一，请重新同步")
	}
	return matched[0], nil
}

// previewKeyMatchesLocator 判断单个 key 是否满足本次安全定位请求。
func previewKeyMatchesLocator(key SyncedKey, index int, req PreviewKeyModelFetchRequest) bool {
	if req.Index != nil && *req.Index != index {
		return false
	}
	if expected := strings.TrimSpace(req.SyncID); expected != "" && strings.TrimSpace(key.SyncID) != expected {
		return false
	}
	if expected := strings.TrimSpace(req.ExternalID); expected != "" && strings.TrimSpace(key.ExternalID) != expected {
		return false
	}
	if expected := strings.TrimSpace(req.MaskedKey); expected != "" && strings.TrimSpace(key.MaskedKey) != expected {
		return false
	}
	return true
}
