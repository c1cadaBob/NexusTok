package upstreamaccount

import (
	"encoding/hex"
	"fmt"
	"math"
	"strings"

	"github.com/c1cada/NexusTok/common"
)

const (
	upstreamAccountSyncMetadataKey = "upstream_account_sync"
	// AccountAutoCheckFastSuccessDurationMs 是自动测试允许恢复同步密钥的单次成功耗时上限。
	// 自动恢复需要连续两次成功且两次都严格小于该阈值；慢成功只说明上游可达，
	// 不能证明该密钥足够健康到可以重新承接用户对话。
	AccountAutoCheckFastSuccessDurationMs int64 = 45 * 1000
)

type syncMetadata struct {
	Platform                     string                   `json:"platform,omitempty"`
	BaseURL                      string                   `json:"base_url,omitempty"`
	ManagementBaseURL            string                   `json:"management_base_url,omitempty"`
	RelayBaseURL                 string                   `json:"relay_base_url,omitempty"`
	Credentials                  *StoredCredential        `json:"credentials,omitempty"`
	ExternalID                   string                   `json:"external_id,omitempty"`
	KeyDigest                    string                   `json:"key_digest,omitempty"`
	SyncedAt                     int64                    `json:"synced_at,omitempty"`
	GroupID                      string                   `json:"group_id,omitempty"`
	GroupName                    string                   `json:"group_name,omitempty"`
	GroupRatio                   *float64                 `json:"group_ratio,omitempty"`
	ModelRatios                  map[string]float64       `json:"model_ratios,omitempty"`
	EffectiveRatio               float64                  `json:"effective_ratio,omitempty"`
	RatioConversion              float64                  `json:"ratio_conversion,omitempty"`
	RatioConversionConfig        *RatioConversionSnapshot `json:"ratio_conversion_config,omitempty"`
	QuotaLimitUSD                *float64                 `json:"quota_limit_usd,omitempty"`
	QuotaUsedUSD                 *float64                 `json:"quota_used_usd,omitempty"`
	QuotaRemainingUSD            *float64                 `json:"quota_remaining_usd,omitempty"`
	BalanceSnapshot              *AccountBalanceSnapshot  `json:"balance_snapshot,omitempty"`
	KeyModelsSyncedAt            int64                    `json:"key_models_synced_at,omitempty"`
	KeyModelsSyncSource          string                   `json:"key_models_sync_source,omitempty"`
	KeyModelsSyncError           string                   `json:"key_models_sync_error,omitempty"`
	KeyModelsManualOverride      bool                     `json:"key_models_manual_override,omitempty"`
	AutoCheckLastCheckedAt       int64                    `json:"auto_check_last_checked_at,omitempty"`
	AutoCheckLastSuccessAt       int64                    `json:"auto_check_last_success_at,omitempty"`
	AutoCheckLastDurationMS      int64                    `json:"auto_check_last_duration_ms,omitempty"`
	AutoCheckFastSuccessStreak   int                      `json:"auto_check_fast_success_streak,omitempty"`
	AutoCheckFailureCount        int                      `json:"auto_check_failure_count,omitempty"`
	AutoCheckLastError           string                   `json:"auto_check_last_error,omitempty"`
	AutoCheckLastStatus          string                   `json:"auto_check_last_status,omitempty"`
	AutoCheckDisabledByAutoCheck bool                     `json:"auto_check_disabled_by_auto_check,omitempty"`
	AutoCheckDisabledAt          int64                    `json:"auto_check_disabled_at,omitempty"`
}

// AccountSyncDisplayMetadata 是可返回给前端展示的同步账号元数据。
//
// 该结构体只包含上游密钥分组和倍率信息，不包含明文 key、key digest、external_id
// 等可用于定位或恢复凭证的敏感身份字段。controller 的账号列表和详情响应可以直接
// 展示这些字段，避免只读管理员为了查看同步成本信息而必须拥有敏感写权限。
type AccountSyncDisplayMetadata struct {
	KeyGroupID            string                   `json:"key_group_id,omitempty"`
	KeyGroupName          string                   `json:"key_group_name,omitempty"`
	GroupRatio            *float64                 `json:"group_ratio,omitempty"`
	ModelRatios           map[string]float64       `json:"model_ratios,omitempty"`
	EffectiveRatio        float64                  `json:"effective_ratio,omitempty"`
	RatioConversion       float64                  `json:"ratio_conversion,omitempty"`
	RatioConversionConfig *RatioConversionSnapshot `json:"ratio_conversion_config,omitempty"`
}

// AccountAutoCheckMetadata 是同步密钥自动连接测试需要读取的非敏感运行状态。
type AccountAutoCheckMetadata struct {
	RatioConversion     float64 `json:"ratio_conversion,omitempty"`
	EffectiveRatio      float64 `json:"effective_ratio,omitempty"`
	FailureCount        int     `json:"failure_count,omitempty"`
	LastCheckedAt       int64   `json:"last_checked_at,omitempty"`
	LastSuccessAt       int64   `json:"last_success_at,omitempty"`
	LastDurationMS      int64   `json:"last_duration_ms,omitempty"`
	FastSuccessStreak   int     `json:"fast_success_streak,omitempty"`
	LastError           string  `json:"last_error,omitempty"`
	LastStatus          string  `json:"last_status,omitempty"`
	DisabledByAutoCheck bool    `json:"disabled_by_auto_check,omitempty"`
	DisabledAt          int64   `json:"disabled_at,omitempty"`
}

// AccountBalanceSnapshot 是渠道 settings 中保存的上游账号级余额快照。
//
// Channel.balance 只保存当前余额，无法表达上游账号总用量、数据是否部分缺失、
// 以及该快照来自哪个上游接口。该结构只包含非敏感账单摘要，允许返回给管理员汇总
// 页面；密码、Cookie、access_token、refresh_token 和完整 API Key 仍然只存在
// credentials 或 ChannelAccount.Key 中，不会写入这里。
type AccountBalanceSnapshot struct {
	BalanceUSD          *float64 `json:"balance_usd,omitempty"`
	UsedUSD             *float64 `json:"used_usd,omitempty"`
	RawBalance          *float64 `json:"raw_balance,omitempty"`
	RawUsed             *float64 `json:"raw_used,omitempty"`
	QuotaPerUnit        *float64 `json:"quota_per_unit,omitempty"`
	Source              string   `json:"source,omitempty"`
	Partial             bool     `json:"partial,omitempty"`
	MissingUsedValue    bool     `json:"missing_used_value,omitempty"`
	MissingBalanceValue bool     `json:"missing_balance_value,omitempty"`
	SyncedAt            int64    `json:"synced_at,omitempty"`
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
	existingMetadata := readChannelSyncMetadata(existing)
	syncedAt := common.GetTimestamp()
	next := map[string]any{
		"platform":  snapshot.Platform,
		"base_url":  snapshotSyncMetadataBaseURL(snapshot),
		"synced_at": syncedAt,
	}
	if managementBaseURL := snapshotManagementBaseURL(snapshot); managementBaseURL != "" {
		next["management_base_url"] = managementBaseURL
	}
	if relayBaseURL := snapshotRelayBaseURL(snapshot); relayBaseURL != "" {
		next["relay_base_url"] = relayBaseURL
	}
	if snapshot.RatioConversion != nil {
		next["ratio_conversion_config"] = snapshot.RatioConversion
	} else if existingMetadata.RatioConversionConfig != nil {
		// 余额刷新和部分旧上游接口可能只返回账号余额，不重新携带倍率配置；
		// 此时必须继续保存渠道级历史配置，不能让下次展示退回因子 1。
		next["ratio_conversion_config"] = existingMetadata.RatioConversionConfig
	}
	if balanceSnapshot := buildAccountBalanceSnapshot(snapshot.Balance, syncedAt); balanceSnapshot != nil {
		next["balance_snapshot"] = balanceSnapshot
	} else if existingMetadata.BalanceSnapshot != nil {
		// 某些上游只返回密钥列表而不返回余额；此时保留上一次账号级快照，
		// 避免管理员钱包页因为一次不完整刷新突然归零或退化成旧密钥近似值。
		next["balance_snapshot"] = existingMetadata.BalanceSnapshot
	}
	if existingMetadata.Credentials != nil {
		next["credentials"] = existingMetadata.Credentials
	}
	data[upstreamAccountSyncMetadataKey] = next
	bytes, err := common.Marshal(data)
	if err != nil {
		return existing
	}
	return string(bytes)
}

func mergeChannelSyncMetadataWithCredential(existing string, snapshot *Snapshot, credential Credential) string {
	metadata := mergeChannelSyncMetadata(existing, snapshot)
	stored := snapshotStoredCredential(snapshot)
	if stored == nil {
		var err error
		stored, err = buildStoredCredential(snapshot, credential)
		if err != nil {
			common.SysLog("failed to encrypt upstream account credential metadata: " + err.Error())
		}
	}
	if stored == nil {
		return metadata
	}
	var data map[string]any
	if strings.TrimSpace(metadata) != "" {
		_ = common.UnmarshalJsonStr(metadata, &data)
	}
	if data == nil {
		data = map[string]any{}
	}
	raw, _ := data[upstreamAccountSyncMetadataKey].(map[string]any)
	if raw == nil {
		raw = map[string]any{}
	}
	if stored.ManagementBaseURL == "" {
		stored.ManagementBaseURL = snapshotManagementBaseURL(snapshot)
	}
	if stored.RelayBaseURL == "" {
		stored.RelayBaseURL = snapshotRelayBaseURL(snapshot)
	}
	raw["credentials"] = stored
	data[upstreamAccountSyncMetadataKey] = raw
	bytes, err := common.Marshal(data)
	if err != nil {
		return metadata
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
	existingMetadata := readAccountSyncMetadata(existing)
	data[upstreamAccountSyncMetadataKey] = syncMetadata{
		Platform:                     snapshot.Platform,
		BaseURL:                      snapshotSyncMetadataBaseURL(snapshot),
		ManagementBaseURL:            snapshotManagementBaseURL(snapshot),
		RelayBaseURL:                 snapshotRelayBaseURL(snapshot),
		ExternalID:                   key.ExternalID,
		KeyDigest:                    keyDigest(key.Key),
		SyncedAt:                     common.GetTimestamp(),
		GroupID:                      strings.TrimSpace(key.GroupID),
		GroupName:                    strings.TrimSpace(key.GroupName),
		GroupRatio:                   key.GroupRatio,
		ModelRatios:                  cloneModelRatios(key.ModelRatios),
		EffectiveRatio:               EffectiveKeyRatio(key),
		RatioConversion:              ConvertedKeyRatio(key),
		RatioConversionConfig:        snapshot.RatioConversion,
		QuotaLimitUSD:                finiteFloatPointer(key.QuotaLimitUSD),
		QuotaUsedUSD:                 finiteFloatPointer(key.QuotaUsedUSD),
		QuotaRemainingUSD:            finiteFloatPointer(key.QuotaRemainingUSD),
		KeyModelsSyncedAt:            existingMetadata.KeyModelsSyncedAt,
		KeyModelsSyncSource:          existingMetadata.KeyModelsSyncSource,
		KeyModelsSyncError:           existingMetadata.KeyModelsSyncError,
		KeyModelsManualOverride:      existingMetadata.KeyModelsManualOverride,
		AutoCheckLastCheckedAt:       existingMetadata.AutoCheckLastCheckedAt,
		AutoCheckLastSuccessAt:       existingMetadata.AutoCheckLastSuccessAt,
		AutoCheckLastDurationMS:      existingMetadata.AutoCheckLastDurationMS,
		AutoCheckFastSuccessStreak:   existingMetadata.AutoCheckFastSuccessStreak,
		AutoCheckFailureCount:        existingMetadata.AutoCheckFailureCount,
		AutoCheckLastError:           existingMetadata.AutoCheckLastError,
		AutoCheckLastStatus:          existingMetadata.AutoCheckLastStatus,
		AutoCheckDisabledByAutoCheck: existingMetadata.AutoCheckDisabledByAutoCheck,
		AutoCheckDisabledAt:          existingMetadata.AutoCheckDisabledAt,
	}
	bytes, err := common.Marshal(data)
	if err != nil {
		return existing
	}
	return string(bytes)
}

// ReadAccountSyncDisplayMetadata 从账号 settings 读取安全展示字段。
func ReadAccountSyncDisplayMetadata(settings string) AccountSyncDisplayMetadata {
	metadata := readAccountSyncMetadata(settings)
	if metadata.Platform == "" && metadata.BaseURL == "" && metadata.ExternalID == "" && metadata.KeyDigest == "" {
		return AccountSyncDisplayMetadata{}
	}
	return AccountSyncDisplayMetadata{
		KeyGroupID:            metadata.GroupID,
		KeyGroupName:          metadata.GroupName,
		GroupRatio:            metadata.GroupRatio,
		ModelRatios:           cloneModelRatios(metadata.ModelRatios),
		EffectiveRatio:        metadata.EffectiveRatio,
		RatioConversion:       metadata.RatioConversion,
		RatioConversionConfig: metadata.RatioConversionConfig,
	}
}

// ReadChannelAccountBalanceSnapshot 读取同步渠道保存的账号级余额快照。
//
// 汇总接口会优先使用这里记录的上游账号级 used_usd；旧渠道可能只有
// Channel.balance 和同步密钥 used_quota，因此该函数只负责安全读取快照，不做任何推断。
func ReadChannelAccountBalanceSnapshot(settings string) (AccountBalanceSnapshot, bool) {
	metadata := readChannelSyncMetadata(settings)
	if metadata.BalanceSnapshot == nil {
		return AccountBalanceSnapshot{}, false
	}
	return *metadata.BalanceSnapshot, true
}

// HasAccountSyncMetadata 判断账号 settings 是否包含上游同步身份。
//
// 控制器在允许管理员本地编辑同步账号的模型、访问组和调度权重时，需要先识别该账号
// 是否由上游账号同步流程维护。这里统一复用内部解析逻辑，避免控制器自行处理 settings
// JSON，也避免把 key_digest、external_id 等只供匹配使用的字段暴露到响应结构中。
func HasAccountSyncMetadata(settings string) bool {
	metadata := readAccountSyncMetadata(settings)
	return syncMetadataHasIdentity(metadata)
}

// syncMetadataHasIdentity 判断 metadata 是否来自上游账号同步流程。
//
// 资产回退、账号脱敏展示和刷新匹配都依赖这个判断。它不能只看 quota 字段，
// 因为管理员本地账号也可能拥有 used_quota；必须确认存在同步来源身份后，才能把
// ChannelAccount.used_quota 当作旧版上游密钥已用快照。
func syncMetadataHasIdentity(metadata syncMetadata) bool {
	return metadata.Platform != "" ||
		metadata.BaseURL != "" ||
		metadata.ManagementBaseURL != "" ||
		metadata.RelayBaseURL != "" ||
		metadata.ExternalID != "" ||
		metadata.KeyDigest != "" ||
		metadata.SyncedAt > 0
}

// SanitizeChannelSyncSettings 移除渠道 settings 中只供后端使用的上游登录凭据。
//
// 渠道列表和详情接口会把 settings 返回给前端用于回填普通配置。即使 Password 和
// Session 都是 AES-GCM 密文，也不应暴露到浏览器或导出接口里；后端内部从数据库
// 读取原始 settings 时仍可用 ReadChannelSyncCredential 解密并重新登录上游平台。
func SanitizeChannelSyncSettings(settings string) string {
	var data map[string]any
	if strings.TrimSpace(settings) == "" {
		return settings
	}
	if err := common.UnmarshalJsonStr(settings, &data); err != nil {
		return settings
	}
	raw, ok := data[upstreamAccountSyncMetadataKey]
	if !ok {
		return settings
	}
	rawBytes, err := common.Marshal(raw)
	if err != nil {
		return settings
	}
	var metadata map[string]any
	if err := common.Unmarshal(rawBytes, &metadata); err != nil {
		return settings
	}
	hasCredential := false
	credentialAuthMode := ""
	if rawCredential, ok := metadata["credentials"]; ok {
		if credentialMap, ok := rawCredential.(map[string]any); ok {
			if password, ok := credentialMap["password"].(string); ok && strings.TrimSpace(password) != "" {
				hasCredential = true
			}
			if session, ok := credentialMap["session"].(string); ok && strings.TrimSpace(session) != "" {
				hasCredential = true
			}
			if authMode, ok := credentialMap["auth_mode"].(string); ok {
				credentialAuthMode = NormalizeAuthMode(authMode)
			}
		}
	}
	delete(metadata, "credentials")
	delete(metadata, "credential_saved")
	delete(metadata, "credential_auth_mode")
	if hasCredential {
		metadata["credential_saved"] = true
		if credentialAuthMode != "" {
			// 只暴露认证方式摘要，帮助前端正确展示“账号密码”或“自动配置”。
			// 真正的 password/session 密文仍会被移除，浏览器无法用该字段恢复任何凭据。
			metadata["credential_auth_mode"] = credentialAuthMode
		}
	}
	data[upstreamAccountSyncMetadataKey] = metadata
	bytes, err := common.Marshal(data)
	if err != nil {
		return settings
	}
	return string(bytes)
}

// PreserveChannelSyncCredential 在渠道编辑保存时保留后端隐藏的上游登录凭据。
//
// 渠道详情接口返回给前端的 settings 会把 credentials 脱敏为 credential_saved，
// 前端保存普通渠道配置时只能提交这个安全副本。如果直接落库，就会把真实的加密
// password/session 覆盖成一个不可复用的展示标记，导致下次刷新时界面显示“已保存登录”，
// 但后端读不到任何可认证凭据。该函数只在 next 仍然声明同一个同步来源时，把 existing
// 中的隐藏 credentials 合并回去；如果来源被显式切换，则不跨平台或跨站点复用旧凭据。
func PreserveChannelSyncCredential(existing string, next string) string {
	nextData := map[string]any{}
	if strings.TrimSpace(next) != "" {
		if err := common.UnmarshalJsonStr(next, &nextData); err != nil {
			return next
		}
	}
	rawNextMetadata, ok := nextData[upstreamAccountSyncMetadataKey]
	if !ok {
		return next
	}
	nextMetadata, ok := syncMetadataMap(rawNextMetadata)
	if !ok {
		return next
	}

	if rawCredential, ok := nextMetadata["credentials"]; ok && syncCredentialHasSecret(rawCredential) {
		delete(nextMetadata, "credential_saved")
		nextData[upstreamAccountSyncMetadataKey] = nextMetadata
		return marshalSettingsOrFallback(nextData, next)
	}

	delete(nextMetadata, "credentials")
	delete(nextMetadata, "credential_saved")
	existingMetadata := readChannelSyncMetadata(existing)
	if existingMetadata.Credentials != nil &&
		storedCredentialHasSecret(existingMetadata.Credentials) &&
		channelSyncCredentialSourceMatches(existingMetadata, nextMetadata) {
		nextMetadata["credentials"] = existingMetadata.Credentials
	}
	nextData[upstreamAccountSyncMetadataKey] = nextMetadata
	return marshalSettingsOrFallback(nextData, next)
}

// ReadChannelSyncCredential 从渠道 settings 中读取并解密上游账号登录凭据。
func ReadChannelSyncCredential(settings string) (Credential, bool, error) {
	metadata := readChannelSyncMetadata(settings)
	if metadata.Platform == "" && metadata.BaseURL == "" && metadata.ManagementBaseURL == "" && metadata.RelayBaseURL == "" {
		return Credential{}, false, nil
	}
	if metadata.Credentials == nil {
		return Credential{}, false, nil
	}
	password := ""
	if strings.TrimSpace(metadata.Credentials.Password) != "" {
		var err error
		password, err = common.DecryptSensitiveString(metadata.Credentials.Password)
		if err != nil {
			return Credential{}, false, fmt.Errorf("解密上游账号凭据失败：%w", err)
		}
	}
	session, err := decryptAuthenticatedSession(metadata.Credentials.Session)
	if err != nil {
		return Credential{}, false, err
	}
	credential := Credential{
		Platform:          firstNonEmpty(metadata.Credentials.Platform, metadata.Platform),
		BaseURL:           firstNonEmpty(metadata.Credentials.ManagementBaseURL, metadata.Credentials.BaseURL, metadata.ManagementBaseURL, metadata.BaseURL),
		ManagementBaseURL: firstNonEmpty(metadata.Credentials.ManagementBaseURL, metadata.Credentials.BaseURL, metadata.ManagementBaseURL, metadata.BaseURL),
		RelayBaseURL:      firstNonEmpty(metadata.Credentials.RelayBaseURL, metadata.RelayBaseURL),
		Username:          metadata.Credentials.Username,
		Email:             metadata.Credentials.Email,
		AuthMode:          metadata.Credentials.AuthMode,
		Password:          password,
		Session:           session,
	}
	credential = HydrateCredentialFromSession(credential)
	if strings.TrimSpace(credential.Username) == "" && strings.TrimSpace(credential.Email) == "" && !hasReusableAuthSession(session) {
		return Credential{}, false, nil
	}
	if strings.TrimSpace(credential.Password) == "" && !hasReusableAuthSession(session) {
		return Credential{}, false, nil
	}
	return credential, true, nil
}

func syncMetadataMap(raw any) (map[string]any, bool) {
	rawBytes, err := common.Marshal(raw)
	if err != nil {
		return nil, false
	}
	var metadata map[string]any
	if err := common.Unmarshal(rawBytes, &metadata); err != nil {
		return nil, false
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	return metadata, true
}

func syncCredentialHasSecret(raw any) bool {
	rawBytes, err := common.Marshal(raw)
	if err != nil {
		return false
	}
	var credential StoredCredential
	if err := common.Unmarshal(rawBytes, &credential); err != nil {
		return false
	}
	return storedCredentialHasSecret(&credential)
}

func storedCredentialHasSecret(credential *StoredCredential) bool {
	if credential == nil {
		return false
	}
	return strings.TrimSpace(credential.Password) != "" ||
		strings.TrimSpace(credential.Session) != ""
}

func channelSyncCredentialSourceMatches(existing syncMetadata, next map[string]any) bool {
	credential := existing.Credentials
	if credential == nil {
		return false
	}
	existingPlatform := NormalizePlatform(firstNonEmpty(credential.Platform, existing.Platform))
	nextPlatform := NormalizePlatform(stringFromMetadata(next, "platform"))
	if nextPlatform != "" && existingPlatform != "" && nextPlatform != existingPlatform {
		return false
	}
	existingBaseURL := firstNonEmpty(credential.BaseURL, existing.BaseURL)
	if existingPlatform == PlatformSub2API {
		existingBaseURL = firstNonEmpty(credential.ManagementBaseURL, credential.BaseURL, existing.ManagementBaseURL, existing.BaseURL)
	}
	nextBaseURL := stringFromMetadata(next, "base_url")
	if nextPlatform == PlatformSub2API {
		nextBaseURL = firstNonEmpty(stringFromMetadata(next, "management_base_url"), nextBaseURL)
	}
	if nextBaseURL != "" && existingBaseURL != "" {
		platform := firstNonEmpty(nextPlatform, existingPlatform)
		if !sameSyncSourceBaseURL(platform, existingBaseURL, nextBaseURL) {
			return false
		}
	}
	return true
}

func stringFromMetadata(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	value, ok := metadata[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func marshalSettingsOrFallback(data map[string]any, fallback string) string {
	bytes, err := common.Marshal(data)
	if err != nil {
		return fallback
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

// MarkAccountKeyModelsManualOverride 标记同步账号的模型白名单已由管理员手动编辑。
//
// 默认自动同步不会覆盖带有该标记的 ChannelAccount.models。该函数只改写
// settings.upstream_account_sync 中的非敏感状态，不接触明文 key、token 或 Cookie。
func MarkAccountKeyModelsManualOverride(settings string) string {
	return mutateAccountSyncMetadata(settings, func(metadata map[string]any) {
		metadata["key_models_manual_override"] = true
		metadata["key_models_sync_source"] = "manual"
		metadata["key_models_sync_error"] = ""
	})
}

// AccountKeyModelsManualOverride 判断账号模型白名单是否被管理员手动覆盖过。
func AccountKeyModelsManualOverride(settings string) bool {
	return readAccountSyncMetadata(settings).KeyModelsManualOverride
}

// ReadAccountAutoCheckMetadata 读取同步密钥自动连接测试状态和倍率信息。
func ReadAccountAutoCheckMetadata(settings string) AccountAutoCheckMetadata {
	metadata := readAccountSyncMetadata(settings)
	return AccountAutoCheckMetadata{
		RatioConversion:     metadata.RatioConversion,
		EffectiveRatio:      metadata.EffectiveRatio,
		FailureCount:        metadata.AutoCheckFailureCount,
		LastCheckedAt:       metadata.AutoCheckLastCheckedAt,
		LastSuccessAt:       metadata.AutoCheckLastSuccessAt,
		LastDurationMS:      metadata.AutoCheckLastDurationMS,
		FastSuccessStreak:   metadata.AutoCheckFastSuccessStreak,
		LastError:           metadata.AutoCheckLastError,
		LastStatus:          metadata.AutoCheckLastStatus,
		DisabledByAutoCheck: metadata.AutoCheckDisabledByAutoCheck,
		DisabledAt:          metadata.AutoCheckDisabledAt,
	}
}

// ApplyAccountAutoCheckSuccess 写入同步密钥手动测试成功状态。
//
// 该函数保留给旧调用点作为兼容包装：手动测试成功代表管理员已经明确验证该密钥
// 可用，因此会立即清除自动禁用标记。后台自动测试必须调用
// ApplyAccountAutoCheckAutomaticSuccess，避免第一次成功就绕过恢复门槛。
func ApplyAccountAutoCheckSuccess(settings string) string {
	return ApplyAccountAutoCheckManualSuccess(settings, 0)
}

// ApplyAccountAutoCheckAutomaticSuccess 写入同步密钥后台自动测试成功状态。
//
// 自动测试成功只表示本次探测可达。为了避免偶发恢复造成用户请求再次失败，这里只
// 记录连续快速成功次数，不清除 auto_check_disabled_by_auto_check。真正恢复账号状态
// 由调用方在确认连续两次快速成功后，再调用 ApplyAccountAutoCheckRecoveryMarker 完成。
func ApplyAccountAutoCheckAutomaticSuccess(settings string, durationMs int64) string {
	now := common.GetTimestamp()
	durationMs = normalizeAccountAutoCheckDurationMS(durationMs)
	previous := readAccountSyncMetadata(settings)
	fastSuccessStreak := 0
	if AccountAutoCheckDurationFast(durationMs) {
		fastSuccessStreak = previous.AutoCheckFastSuccessStreak + 1
	}
	return mutateAccountSyncMetadata(settings, func(metadata map[string]any) {
		metadata["auto_check_last_checked_at"] = now
		metadata["auto_check_last_success_at"] = now
		metadata["auto_check_last_duration_ms"] = durationMs
		metadata["auto_check_fast_success_streak"] = fastSuccessStreak
		metadata["auto_check_failure_count"] = 0
		metadata["auto_check_last_error"] = ""
		metadata["auto_check_last_status"] = "success"
	})
}

// ApplyAccountAutoCheckManualSuccess 写入同步密钥手动测试成功状态。
//
// 手动测试是管理员针对指定密钥发起的显式验证，成功后应立即恢复该密钥并清除所有
// 自动禁用恢复状态；否则页面上“测试成功但仍禁用”的结果会与管理员预期相冲突。
func ApplyAccountAutoCheckManualSuccess(settings string, durationMs int64) string {
	now := common.GetTimestamp()
	durationMs = normalizeAccountAutoCheckDurationMS(durationMs)
	return mutateAccountSyncMetadata(settings, func(metadata map[string]any) {
		metadata["auto_check_last_checked_at"] = now
		metadata["auto_check_last_success_at"] = now
		metadata["auto_check_last_duration_ms"] = durationMs
		metadata["auto_check_fast_success_streak"] = 0
		metadata["auto_check_failure_count"] = 0
		metadata["auto_check_last_error"] = ""
		metadata["auto_check_last_status"] = "success"
		metadata["auto_check_disabled_by_auto_check"] = false
		metadata["auto_check_disabled_at"] = 0
	})
}

// ApplyAccountAutoCheckRecoveryMarker 清除自动检测禁用标记并保留成功测试状态。
//
// 后台自动测试达到恢复门槛后使用该函数。它不同于 ClearAccountAutoCheckDisableMarker：
// last_status 仍应保持 success，表示账号是由连通性检测恢复，而不是由管理员直接改状态。
func ApplyAccountAutoCheckRecoveryMarker(settings string) string {
	return mutateAccountSyncMetadata(settings, func(metadata map[string]any) {
		metadata["auto_check_fast_success_streak"] = 0
		metadata["auto_check_failure_count"] = 0
		metadata["auto_check_last_error"] = ""
		metadata["auto_check_disabled_by_auto_check"] = false
		metadata["auto_check_disabled_at"] = 0
	})
}

// ApplyAccountAutoCheckFailure 写入同步密钥自动连接测试失败状态。
func ApplyAccountAutoCheckFailure(settings string, failureCount int, errorText string, disabledByAutoCheck bool) string {
	now := common.GetTimestamp()
	errorText = sanitizeUpstreamAccountSyncTaskLogText(common.MaskSensitiveInfo(errorText), upstreamAccountSyncTaskLogErrorMaxRunes)
	return mutateAccountSyncMetadata(settings, func(metadata map[string]any) {
		metadata["auto_check_last_checked_at"] = now
		metadata["auto_check_fast_success_streak"] = 0
		metadata["auto_check_failure_count"] = failureCount
		metadata["auto_check_last_error"] = errorText
		metadata["auto_check_last_status"] = "failed"
		if disabledByAutoCheck {
			metadata["auto_check_disabled_by_auto_check"] = true
			if stringFromMetadata(metadata, "auto_check_disabled_at") == "" || fmt.Sprint(metadata["auto_check_disabled_at"]) == "0" {
				metadata["auto_check_disabled_at"] = now
			}
		}
	})
}

// ClearAccountAutoCheckDisableMarker 清除“由自动检测禁用”的恢复标记。
//
// 管理员手动启用或手动禁用密钥时调用该函数，避免后续自动检测把人工状态误当成
// 可自动恢复的状态。
func ClearAccountAutoCheckDisableMarker(settings string) string {
	return mutateAccountSyncMetadata(settings, func(metadata map[string]any) {
		metadata["auto_check_fast_success_streak"] = 0
		metadata["auto_check_failure_count"] = 0
		metadata["auto_check_last_error"] = ""
		metadata["auto_check_last_status"] = "manual"
		metadata["auto_check_disabled_by_auto_check"] = false
		metadata["auto_check_disabled_at"] = 0
	})
}

// AccountAutoCheckDurationFast 判断一次自动测试成功是否满足恢复用的快速成功条件。
func AccountAutoCheckDurationFast(durationMs int64) bool {
	return normalizeAccountAutoCheckDurationMS(durationMs) < AccountAutoCheckFastSuccessDurationMs
}

func normalizeAccountAutoCheckDurationMS(durationMs int64) int64 {
	if durationMs < 0 {
		return 0
	}
	return durationMs
}

func applyAccountKeyModelsSyncMetadata(settings string, key SyncedKey, manualOverride bool, usedModels string) string {
	if manualOverride {
		return MarkAccountKeyModelsManualOverride(settings)
	}
	source := strings.TrimSpace(key.KeyModelSyncSource)
	if source == "" && len(key.Models) > 0 {
		source = "snapshot"
	}
	errText := sanitizeUpstreamAccountSyncTaskLogText(common.MaskSensitiveInfo(key.KeyModelSyncError), upstreamAccountSyncTaskLogErrorMaxRunes)
	if source == "" && errText == "" {
		return settings
	}
	return mutateAccountSyncMetadata(settings, func(metadata map[string]any) {
		if source != "" && strings.TrimSpace(usedModels) != "" {
			metadata["key_models_synced_at"] = common.GetTimestamp()
			metadata["key_models_sync_source"] = source
			metadata["key_models_manual_override"] = false
		}
		if errText != "" {
			metadata["key_models_sync_error"] = errText
			if source != "" {
				metadata["key_models_sync_source"] = source
			}
		} else {
			metadata["key_models_sync_error"] = ""
		}
	})
}

func mutateAccountSyncMetadata(settings string, mutate func(map[string]any)) string {
	if mutate == nil {
		return settings
	}
	var data map[string]any
	if strings.TrimSpace(settings) != "" {
		if err := common.UnmarshalJsonStr(settings, &data); err != nil {
			return settings
		}
	}
	if data == nil {
		data = map[string]any{}
	}
	raw, ok := data[upstreamAccountSyncMetadataKey]
	if !ok {
		return settings
	}
	metadata, ok := syncMetadataMap(raw)
	if !ok {
		return settings
	}
	mutate(metadata)
	data[upstreamAccountSyncMetadataKey] = metadata
	return marshalSettingsOrFallback(data, settings)
}

func readAccountSyncMetadata(settings string) syncMetadata {
	return readSyncMetadata(settings)
}

func readChannelSyncMetadata(settings string) syncMetadata {
	return readSyncMetadata(settings)
}

func readSyncMetadata(settings string) syncMetadata {
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

func buildAccountBalanceSnapshot(balance *BalanceSnapshot, syncedAt int64) *AccountBalanceSnapshot {
	if balance == nil {
		return nil
	}
	snapshot := &AccountBalanceSnapshot{
		BalanceUSD:          finiteFloatPointer(balance.BalanceUSD),
		UsedUSD:             finiteFloatPointer(balance.UsedUSD),
		RawBalance:          finiteFloatPointer(balance.RawBalance),
		RawUsed:             finiteFloatPointer(balance.RawUsed),
		QuotaPerUnit:        finiteFloatPointer(balance.QuotaPerUnit),
		Source:              strings.TrimSpace(balance.Source),
		Partial:             balance.Partial,
		MissingUsedValue:    balance.MissingUsedValue,
		MissingBalanceValue: balance.MissingBalanceValue,
		SyncedAt:            syncedAt,
	}
	if snapshot.BalanceUSD == nil &&
		snapshot.UsedUSD == nil &&
		snapshot.RawBalance == nil &&
		snapshot.RawUsed == nil &&
		snapshot.QuotaPerUnit == nil &&
		snapshot.Source == "" &&
		!snapshot.Partial &&
		!snapshot.MissingUsedValue &&
		!snapshot.MissingBalanceValue {
		return nil
	}
	return snapshot
}

func finiteFloatPointer(value *float64) *float64 {
	if value == nil {
		return nil
	}
	if math.IsNaN(*value) || math.IsInf(*value, 0) {
		return nil
	}
	v := *value
	return &v
}

func buildStoredCredential(snapshot *Snapshot, credential Credential) (*StoredCredential, error) {
	if snapshot == nil {
		return nil, nil
	}
	if snapshot.AuthSession != nil {
		credential.Session = snapshot.AuthSession
	}
	if credential.RelayBaseURL == "" {
		credential.RelayBaseURL = snapshotRelayBaseURL(snapshot)
	}
	return buildStoredCredentialWithBase(
		firstNonEmpty(credential.Platform, snapshot.Platform),
		firstNonEmpty(snapshotManagementBaseURL(snapshot), credential.ManagementBaseURL, credential.BaseURL),
		credential,
	)
}

func snapshotStoredCredential(snapshot *Snapshot) *StoredCredential {
	if snapshot == nil || snapshot.StoredCredential == nil {
		return nil
	}
	stored := *snapshot.StoredCredential
	if stored.Platform == "" {
		stored.Platform = NormalizePlatform(snapshot.Platform)
	}
	if stored.BaseURL == "" {
		stored.BaseURL = snapshotSyncMetadataBaseURL(snapshot)
	}
	if stored.ManagementBaseURL == "" {
		stored.ManagementBaseURL = snapshotManagementBaseURL(snapshot)
	}
	if stored.RelayBaseURL == "" {
		stored.RelayBaseURL = snapshotRelayBaseURL(snapshot)
	}
	if snapshot.AuthSession != nil {
		if err := attachEncryptedAuthSessionToStoredCredential(&stored, stored.Platform, firstNonEmpty(stored.ManagementBaseURL, stored.BaseURL), snapshot.AuthSession); err != nil {
			common.SysLog("failed to encrypt upstream authenticated session: " + err.Error())
		}
	}
	if strings.TrimSpace(stored.Password) == "" && strings.TrimSpace(stored.Session) == "" {
		return nil
	}
	return &stored
}

// buildStoredCredentialWithBase 将上游账号密码和已认证登录态加密后封装成可落库的凭据元数据。
//
// 这里永远不返回明文密码或明文登录态；调用方如果希望把登录信息继续挂到预览快照
// 或 challenge，必须显式把返回值放进后端内存结构中，不能依赖前端回填。
func buildStoredCredentialWithBase(platform string, baseURL string, credential Credential) (*StoredCredential, error) {
	password := strings.TrimSpace(credential.Password)
	if password == "" && !hasReusableAuthSession(credential.Session) {
		return nil, nil
	}
	encryptedPassword := ""
	if password != "" {
		var err error
		encryptedPassword, err = common.EncryptSensitiveString(password)
		if err != nil {
			return nil, fmt.Errorf("加密上游账号凭据失败：%w", err)
		}
	}
	stored := &StoredCredential{
		Platform: NormalizePlatform(platform),
		BaseURL:  normalizeSyncMetadataBaseURL(platform, baseURL),
		ManagementBaseURL: normalizeSyncMetadataBaseURL(
			platform,
			firstNonEmpty(credential.ManagementBaseURL, baseURL),
		),
		RelayBaseURL: normalizeSyncMetadataBaseURL(platform, credential.RelayBaseURL),
		Username:     strings.TrimSpace(credential.Username),
		Email:        strings.TrimSpace(credential.Email),
		AuthMode:     NormalizeAuthMode(credential.AuthMode),
		Password:     encryptedPassword,
		ImportedAt:   credentialImportedAt(credential),
		UpdatedAt:    common.GetTimestamp(),
	}
	if stored.Platform == "" {
		stored.Platform = NormalizePlatform(platform)
	}
	if stored.BaseURL == "" {
		stored.BaseURL = normalizeSyncMetadataBaseURL(stored.Platform, baseURL)
	}
	if stored.ManagementBaseURL == "" {
		stored.ManagementBaseURL = stored.BaseURL
	}
	if err := attachEncryptedAuthSessionToStoredCredential(stored, stored.Platform, stored.ManagementBaseURL, credential.Session); err != nil {
		return nil, err
	}
	return stored, nil
}

// attachStoredCredentialFromChallenge 将已保存的加密凭据重新挂回预览快照。
//
// 只有 2FA challenge 路径会走这里：第一次登录时已经保存的凭据会继续留在后续快照里，
// 供创建或刷新流程复用，但不会再回传给浏览器。
func attachStoredCredentialFromChallenge(snapshot *Snapshot, record *AuthChallengeRecord) {
	if snapshot == nil || record == nil || record.Credential == nil {
		return
	}
	stored := *record.Credential
	if stored.Platform == "" {
		stored.Platform = NormalizePlatform(record.Platform)
	}
	if stored.BaseURL == "" {
		stored.BaseURL = normalizeSyncMetadataBaseURL(stored.Platform, record.BaseURL)
	}
	if stored.ManagementBaseURL == "" {
		stored.ManagementBaseURL = stored.BaseURL
	}
	if stored.RelayBaseURL == "" {
		stored.RelayBaseURL = record.RelayBaseURL
	}
	if snapshot.AuthSession != nil {
		if err := attachEncryptedAuthSessionToStoredCredential(&stored, stored.Platform, stored.ManagementBaseURL, snapshot.AuthSession); err != nil {
			common.SysLog("failed to encrypt upstream authenticated session from challenge: " + err.Error())
		}
	}
	stored.UpdatedAt = common.GetTimestamp()
	snapshot.StoredCredential = &stored
}

// attachStoredCredentialToChallenge 将临时登录凭据加密后挂到 2FA challenge 上。
//
// 这样 2FA 通过后仍然可以把登录信息写回普通预览快照，后续刷新时就能复用同一份
// 上游账号凭据，而不需要管理员再次手动输入。
func attachStoredCredentialToChallenge(record *AuthChallengeRecord, credential Credential) {
	if record == nil {
		return
	}
	if credential.ManagementBaseURL == "" {
		credential.ManagementBaseURL = record.BaseURL
	}
	if credential.BaseURL == "" {
		credential.BaseURL = record.BaseURL
	}
	if credential.RelayBaseURL == "" {
		credential.RelayBaseURL = record.RelayBaseURL
	}
	stored, err := buildStoredCredentialWithBase(record.Platform, record.BaseURL, credential)
	if err != nil {
		common.SysLog("failed to encrypt upstream account challenge credential: " + err.Error())
		return
	}
	record.Credential = stored
}

func attachEncryptedAuthSessionToStoredCredential(stored *StoredCredential, platform string, baseURL string, session *AuthenticatedSession) error {
	if stored == nil || !hasReusableAuthSession(session) {
		return nil
	}
	encrypted, updatedAt, err := encryptAuthenticatedSession(platform, baseURL, session)
	if err != nil {
		return err
	}
	if encrypted == "" {
		return nil
	}
	stored.Session = encrypted
	stored.SessionUpdatedAt = updatedAt
	return nil
}

func encryptAuthenticatedSession(platform string, baseURL string, session *AuthenticatedSession) (string, int64, error) {
	prepared := normalizeAuthenticatedSession(platform, baseURL, session)
	if !hasReusableAuthSession(prepared) {
		return "", 0, nil
	}
	bytes, err := common.Marshal(prepared)
	if err != nil {
		return "", 0, fmt.Errorf("序列化上游登录态失败：%w", err)
	}
	encrypted, err := common.EncryptSensitiveString(string(bytes))
	if err != nil {
		return "", 0, fmt.Errorf("加密上游登录态失败：%w", err)
	}
	return encrypted, prepared.UpdatedAt, nil
}

func decryptAuthenticatedSession(raw string) (*AuthenticatedSession, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	plain, err := common.DecryptSensitiveString(raw)
	if err != nil {
		return nil, fmt.Errorf("解密上游登录态失败：%w", err)
	}
	var session AuthenticatedSession
	if err := common.UnmarshalJsonStr(plain, &session); err != nil {
		return nil, fmt.Errorf("解析上游登录态失败：%w", err)
	}
	prepared := normalizeAuthenticatedSession(session.Platform, session.BaseURL, &session)
	if !hasReusableAuthSession(prepared) {
		return nil, nil
	}
	return prepared, nil
}

func normalizeAuthenticatedSession(platform string, baseURL string, session *AuthenticatedSession) *AuthenticatedSession {
	if session == nil {
		return nil
	}
	prepared := *session
	prepared.Platform = NormalizePlatform(firstNonEmpty(prepared.Platform, platform))
	prepared.BaseURL = normalizeSyncMetadataBaseURL(prepared.Platform, firstNonEmpty(prepared.BaseURL, baseURL))
	prepared.AuthMode = NormalizeAuthMode(prepared.AuthMode)
	if prepared.UpdatedAt <= 0 {
		prepared.UpdatedAt = common.GetTimestamp()
	}
	if prepared.ImportedAt <= 0 && prepared.AuthMode != "" && prepared.AuthMode != AuthModePassword {
		prepared.ImportedAt = prepared.UpdatedAt
	}
	return &prepared
}

func hasReusableAuthSession(session *AuthenticatedSession) bool {
	if session == nil {
		return false
	}
	switch NormalizePlatform(session.Platform) {
	case PlatformNewAPI:
		return session.NewAPI != nil &&
			(strings.TrimSpace(session.NewAPI.AccessToken) != "" || len(session.NewAPI.Cookies) > 0)
	case PlatformSub2API:
		return session.Sub2API != nil &&
			strings.TrimSpace(session.Sub2API.AccessToken) != ""
	default:
		return false
	}
}

func credentialImportedAt(credential Credential) int64 {
	if credential.Session != nil && credential.Session.ImportedAt > 0 {
		return credential.Session.ImportedAt
	}
	if NormalizeAuthMode(credential.AuthMode) != AuthModePassword {
		return common.GetTimestamp()
	}
	return 0
}

func snapshotManagementBaseURL(snapshot *Snapshot) string {
	if snapshot == nil {
		return ""
	}
	platform := NormalizePlatform(snapshot.Platform)
	if platform == PlatformSub2API {
		return normalizeSyncMetadataBaseURL(platform, firstNonEmpty(snapshot.ManagementBaseURL, snapshot.BaseURL))
	}
	return normalizeSyncMetadataBaseURL(platform, snapshot.BaseURL)
}

func snapshotRelayBaseURL(snapshot *Snapshot) string {
	if snapshot == nil {
		return ""
	}
	platform := NormalizePlatform(snapshot.Platform)
	if platform == PlatformSub2API {
		return normalizeSyncMetadataBaseURL(platform, firstNonEmpty(snapshot.RelayBaseURL, snapshot.BaseURL, snapshot.ManagementBaseURL))
	}
	return ""
}

func snapshotSyncMetadataBaseURL(snapshot *Snapshot) string {
	if snapshot == nil {
		return ""
	}
	platform := NormalizePlatform(snapshot.Platform)
	if platform == PlatformSub2API {
		return snapshotManagementBaseURL(snapshot)
	}
	return normalizeSyncMetadataBaseURL(platform, snapshot.BaseURL)
}

func authSessionMatches(session *AuthenticatedSession, platform string, baseURL string) bool {
	if !hasReusableAuthSession(session) {
		return false
	}
	normalizedPlatform := NormalizePlatform(platform)
	if NormalizePlatform(session.Platform) != normalizedPlatform {
		return false
	}
	if strings.TrimSpace(session.BaseURL) == "" {
		return true
	}
	return sameSyncSourceBaseURL(normalizedPlatform, session.BaseURL, baseURL)
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
	if NormalizePlatform(platform) == PlatformSub2API {
		if relatedSub2APIBaseURL(left, right) {
			return true
		}
	}
	return normalizeSyncMetadataBaseURL(platform, left) == normalizeSyncMetadataBaseURL(platform, right)
}

func normalizeSyncMetadataBaseURL(platform string, raw string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(raw), "/")
	if NormalizePlatform(platform) == PlatformSub2API {
		return strings.TrimRight(strings.TrimSpace(normalizeSub2APIBaseURL(trimmed)), "/")
	}
	return trimmed
}

func cloneModelRatios(values map[string]float64) map[string]float64 {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]float64, len(values))
	for modelName, ratio := range values {
		if strings.TrimSpace(modelName) == "" || ratio <= 0 {
			continue
		}
		cloned[modelName] = ratio
	}
	if len(cloned) == 0 {
		return nil
	}
	return cloned
}
