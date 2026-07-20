// Package upstreamaccount 实现上游平台账号同步能力。
//
// 该包只负责用管理员临时输入的账号密码读取目标平台快照，并归一化为
// NexusTok 创建渠道账号所需的数据结构。账号密码不在本包内持久化；完整
// API Key 只允许保存在短期预览缓存中，返回给前端的数据必须脱敏。
package upstreamaccount

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

const (
	// PlatformNewAPI 表示 new-api 平台。
	PlatformNewAPI = "new-api"
	// PlatformSub2API 表示 sub2api 平台。
	PlatformSub2API = "sub2api"
)

// Credential 是管理员发起同步时输入的临时凭证。
type Credential struct {
	Platform string `json:"platform"`
	BaseURL  string `json:"base_url"`
	Username string `json:"username"`
	Password string `json:"password,omitempty"`
	Email    string `json:"email,omitempty"`
}

// StoredCredential 是保存到渠道 settings 的上游账号登录凭据。
//
// 该结构只用于需要后台重新登录上游平台的管理操作，例如点击渠道余额刷新。
// Password 必须使用 common.EncryptSensitiveString 加密后再落库；对外返回渠道
// settings 前必须通过 SanitizeChannelSyncSettings 移除 credentials，避免泄露
// 可离线解密的敏感密文。
type StoredCredential struct {
	Platform  string `json:"platform,omitempty"`
	BaseURL   string `json:"base_url,omitempty"`
	Username  string `json:"username,omitempty"`
	Email     string `json:"email,omitempty"`
	Password  string `json:"password,omitempty"`
	UpdatedAt int64  `json:"updated_at,omitempty"`
}

// PreviewRequest 是预览接口的请求体。
type PreviewRequest struct {
	Credential
	RatioConversion RatioConversionConfig `json:"ratio_conversion,omitempty"`
}

// Preview2FARequest 是补全上游平台二次验证后的预览请求体。
type Preview2FARequest struct {
	ChallengeID     string                `json:"challenge_id"`
	Code            string                `json:"code"`
	RatioConversion RatioConversionConfig `json:"ratio_conversion,omitempty"`
}

// Snapshot 表示目标平台账号当前可见的密钥、分组、倍率和余额快照。
type Snapshot struct {
	Platform         string                   `json:"platform"`
	BaseURL          string                   `json:"base_url"`
	User             *UserSnapshot            `json:"user,omitempty"`
	Balance          *BalanceSnapshot         `json:"balance,omitempty"`
	Groups           []SyncedGroup            `json:"groups"`
	Keys             []SyncedKey              `json:"keys"`
	Rates            *RateSnapshot            `json:"rates,omitempty"`
	RatioConversion  *RatioConversionSnapshot `json:"ratio_conversion,omitempty"`
	StoredCredential *StoredCredential        `json:"-"`
	Warnings         []string                 `json:"warnings,omitempty"`
	Raw              map[string]any           `json:"raw,omitempty"`
}

// UserSnapshot 表示目标平台当前登录用户的基础信息。
type UserSnapshot struct {
	ID       string `json:"id,omitempty"`
	Username string `json:"username,omitempty"`
	Email    string `json:"email,omitempty"`
	Group    string `json:"group,omitempty"`
}

// BalanceSnapshot 表示目标平台账号维度的余额和已用额度。
type BalanceSnapshot struct {
	BalanceUSD          *float64 `json:"balance_usd,omitempty"`
	UsedUSD             *float64 `json:"used_usd,omitempty"`
	RawBalance          *float64 `json:"raw_balance,omitempty"`
	RawUsed             *float64 `json:"raw_used,omitempty"`
	QuotaPerUnit        *float64 `json:"quota_per_unit,omitempty"`
	Source              string   `json:"source,omitempty"`
	Partial             bool     `json:"partial,omitempty"`
	MissingUsedValue    bool     `json:"missing_used_value,omitempty"`
	MissingBalanceValue bool     `json:"missing_balance_value,omitempty"`
}

// SyncedGroup 表示目标平台可用分组及其倍率。
type SyncedGroup struct {
	ID          string             `json:"id,omitempty"`
	Name        string             `json:"name"`
	Platform    string             `json:"platform,omitempty"`
	Ratio       *float64           `json:"ratio,omitempty"`
	PeakRatio   *float64           `json:"peak_ratio,omitempty"`
	Description string             `json:"description,omitempty"`
	ModelRatios map[string]float64 `json:"model_ratios,omitempty"`
	Raw         map[string]any     `json:"raw,omitempty"`
}

// SyncedKey 表示目标平台中的一个 API Key。
type SyncedKey struct {
	SyncID            string             `json:"sync_id,omitempty"`
	ExternalID        string             `json:"external_id,omitempty"`
	Name              string             `json:"name,omitempty"`
	Key               string             `json:"-"`
	MaskedKey         string             `json:"masked_key,omitempty"`
	Status            int                `json:"status,omitempty"`
	GroupID           string             `json:"group_id,omitempty"`
	GroupName         string             `json:"group_name,omitempty"`
	Models            []string           `json:"models,omitempty"`
	ModelRatios       map[string]float64 `json:"model_ratios,omitempty"`
	GroupRatio        *float64           `json:"group_ratio,omitempty"`
	EffectiveRatio    float64            `json:"effective_ratio,omitempty"`
	RatioConversion   float64            `json:"ratio_conversion,omitempty"`
	QuotaLimitUSD     *float64           `json:"quota_limit_usd,omitempty"`
	QuotaUsedUSD      *float64           `json:"quota_used_usd,omitempty"`
	QuotaRemainingUSD *float64           `json:"quota_remaining_usd,omitempty"`
	Unlimited         bool               `json:"unlimited,omitempty"`
	SuggestedPriority int64              `json:"suggested_priority"`
	SuggestedWeight   int                `json:"suggested_weight"`
	Raw               map[string]any     `json:"raw,omitempty"`
}

// RateSnapshot 表示模型、缓存、分组等倍率配置。
type RateSnapshot struct {
	ModelRatios       map[string]float64 `json:"model_ratios,omitempty"`
	CompletionRatios  map[string]float64 `json:"completion_ratios,omitempty"`
	CacheRatios       map[string]float64 `json:"cache_ratios,omitempty"`
	CreateCacheRatios map[string]float64 `json:"create_cache_ratios,omitempty"`
	ModelPrices       map[string]float64 `json:"model_prices,omitempty"`
	GroupRates        map[string]float64 `json:"group_rates,omitempty"`
	Source            string             `json:"source,omitempty"`
	Partial           bool               `json:"partial,omitempty"`
}

// PreviewResult 是预览接口返回给前端的安全数据。
type PreviewResult struct {
	PreviewID string         `json:"preview_id,omitempty"`
	ExpiresAt int64          `json:"expires_at"`
	Snapshot  *Snapshot      `json:"snapshot,omitempty"`
	Challenge *AuthChallenge `json:"challenge,omitempty"`
}

// PreviewRecord 是后端短期缓存的预览记录，包含完整 key。
type PreviewRecord struct {
	ID        string    `json:"id"`
	ExpiresAt int64     `json:"expires_at"`
	Snapshot  *Snapshot `json:"snapshot"`
}

// AuthChallenge 表示需要管理员继续输入的上游平台登录挑战。
type AuthChallenge struct {
	ChallengeID string `json:"challenge_id"`
	Platform    string `json:"platform"`
	Type        string `json:"type"`
	ExpiresAt   int64  `json:"expires_at"`
	Username    string `json:"username,omitempty"`
}

// PlatformClient 定义目标平台账号同步客户端。
type PlatformClient interface {
	FetchSnapshot(ctx context.Context, credential Credential) (*Snapshot, error)
}

// NewPlatformClient 根据平台类型创建客户端。
func NewPlatformClient(platform string) (PlatformClient, error) {
	switch NormalizePlatform(platform) {
	case PlatformNewAPI:
		return NewNewAPIClient(nil), nil
	case PlatformSub2API:
		return NewSub2APIClient(nil), nil
	default:
		return nil, fmt.Errorf("不支持的上游平台：%s", platform)
	}
}

// NormalizePlatform 规范化平台名称。
func NormalizePlatform(platform string) string {
	normalized := strings.ToLower(strings.TrimSpace(platform))
	normalized = strings.ReplaceAll(normalized, "_", "-")
	return normalized
}

// ApplySuggestions 根据倍率为密钥生成默认优先级和权重建议。
//
// 规则保持保守：倍率越低优先级越高，同一倍率下权重相同。没有倍率时使用
// 中性建议，前端仍可让管理员手动覆盖。
func ApplySuggestions(snapshot *Snapshot) {
	if snapshot == nil || len(snapshot.Keys) == 0 {
		return
	}
	ApplySyncIDs(snapshot)
	ApplyExistingRatioConversion(snapshot)
	ratios := make([]float64, 0, len(snapshot.Keys))
	for i := range snapshot.Keys {
		ratio := ConvertedKeyRatio(snapshot.Keys[i])
		ratios = append(ratios, ratio)
	}
	uniqueRatios := make([]float64, 0, len(ratios))
	seen := map[float64]struct{}{}
	for _, ratio := range ratios {
		if _, ok := seen[ratio]; ok {
			continue
		}
		seen[ratio] = struct{}{}
		uniqueRatios = append(uniqueRatios, ratio)
	}
	sort.Float64s(uniqueRatios)
	ratioRank := map[float64]int64{}
	for index, ratio := range uniqueRatios {
		ratioRank[ratio] = int64(len(uniqueRatios) - index)
	}
	for i := range snapshot.Keys {
		ratio := ConvertedKeyRatio(snapshot.Keys[i])
		snapshot.Keys[i].SuggestedPriority = ratioRank[ratio]
		if ratio <= 0 {
			snapshot.Keys[i].SuggestedWeight = 100
			continue
		}
		weight := int(100 / ratio)
		if weight < 1 {
			weight = 1
		}
		if weight > 100 {
			weight = 100
		}
		snapshot.Keys[i].SuggestedWeight = weight
	}
}

// ApplySyncIDs 为每个同步密钥生成前后端一致的配置标识。
//
// 真实平台并不总是提供稳定 external_id。前端需要用该标识回传逐密钥启用、
// 优先级和权重配置；后端刷新也需要在 external_id 缺失时按同一标识匹配配置。
// 该值只用于一次预览/刷新请求内的配置关联，不写入明文 key。
func ApplySyncIDs(snapshot *Snapshot) {
	if snapshot == nil {
		return
	}
	seen := map[string]int{}
	for i := range snapshot.Keys {
		id := strings.TrimSpace(snapshot.Keys[i].ExternalID)
		if id == "" {
			id = strings.TrimSpace(snapshot.Keys[i].MaskedKey)
		}
		if id == "" {
			masked := maskKey(snapshot.Keys[i].Key)
			if masked != "" {
				snapshot.Keys[i].MaskedKey = masked
				id = masked
			}
		}
		if id == "" {
			id = fmt.Sprintf("index:%d", i)
		}
		baseID := id
		if count := seen[baseID]; count > 0 {
			id = fmt.Sprintf("%s#%d", baseID, count+1)
		}
		seen[baseID]++
		snapshot.Keys[i].SyncID = id
	}
}
