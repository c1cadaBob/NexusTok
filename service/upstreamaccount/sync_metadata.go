package upstreamaccount

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/c1cada/NexusTok/common"
)

const upstreamAccountSyncMetadataKey = "upstream_account_sync"

type syncMetadata struct {
	Platform              string                   `json:"platform,omitempty"`
	BaseURL               string                   `json:"base_url,omitempty"`
	Credentials           *StoredCredential        `json:"credentials,omitempty"`
	ExternalID            string                   `json:"external_id,omitempty"`
	KeyDigest             string                   `json:"key_digest,omitempty"`
	SyncedAt              int64                    `json:"synced_at,omitempty"`
	GroupID               string                   `json:"group_id,omitempty"`
	GroupName             string                   `json:"group_name,omitempty"`
	GroupRatio            *float64                 `json:"group_ratio,omitempty"`
	ModelRatios           map[string]float64       `json:"model_ratios,omitempty"`
	EffectiveRatio        float64                  `json:"effective_ratio,omitempty"`
	RatioConversion       float64                  `json:"ratio_conversion,omitempty"`
	RatioConversionConfig *RatioConversionSnapshot `json:"ratio_conversion_config,omitempty"`
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
	next := map[string]any{
		"platform":  snapshot.Platform,
		"base_url":  normalizeSyncMetadataBaseURL(snapshot.Platform, snapshot.BaseURL),
		"synced_at": common.GetTimestamp(),
	}
	if existingMetadata := readChannelSyncMetadata(existing); existingMetadata.Credentials != nil {
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
	data[upstreamAccountSyncMetadataKey] = syncMetadata{
		Platform:              snapshot.Platform,
		BaseURL:               snapshot.BaseURL,
		ExternalID:            key.ExternalID,
		KeyDigest:             keyDigest(key.Key),
		SyncedAt:              common.GetTimestamp(),
		GroupID:               strings.TrimSpace(key.GroupID),
		GroupName:             strings.TrimSpace(key.GroupName),
		GroupRatio:            key.GroupRatio,
		ModelRatios:           cloneModelRatios(key.ModelRatios),
		EffectiveRatio:        EffectiveKeyRatio(key),
		RatioConversion:       ConvertedKeyRatio(key),
		RatioConversionConfig: snapshot.RatioConversion,
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
	if rawCredential, ok := metadata["credentials"]; ok {
		if credentialMap, ok := rawCredential.(map[string]any); ok {
			if password, ok := credentialMap["password"].(string); ok && strings.TrimSpace(password) != "" {
				hasCredential = true
			}
			if session, ok := credentialMap["session"].(string); ok && strings.TrimSpace(session) != "" {
				hasCredential = true
			}
		}
	}
	delete(metadata, "credentials")
	delete(metadata, "credential_saved")
	if hasCredential {
		metadata["credential_saved"] = true
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
	if metadata.Platform == "" && metadata.BaseURL == "" {
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
		Platform: firstNonEmpty(metadata.Credentials.Platform, metadata.Platform),
		BaseURL:  firstNonEmpty(metadata.Credentials.BaseURL, metadata.BaseURL),
		Username: metadata.Credentials.Username,
		Email:    metadata.Credentials.Email,
		Password: password,
		Session:  session,
	}
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
	nextBaseURL := stringFromMetadata(next, "base_url")
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

func buildStoredCredential(snapshot *Snapshot, credential Credential) (*StoredCredential, error) {
	if snapshot == nil {
		return nil, nil
	}
	if snapshot.AuthSession != nil {
		credential.Session = snapshot.AuthSession
	}
	return buildStoredCredentialWithBase(
		firstNonEmpty(credential.Platform, snapshot.Platform),
		firstNonEmpty(credential.BaseURL, snapshot.BaseURL),
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
		stored.BaseURL = normalizeSyncMetadataBaseURL(stored.Platform, snapshot.BaseURL)
	}
	if snapshot.AuthSession != nil {
		if err := attachEncryptedAuthSessionToStoredCredential(&stored, stored.Platform, stored.BaseURL, snapshot.AuthSession); err != nil {
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
		Platform:  NormalizePlatform(platform),
		BaseURL:   normalizeSyncMetadataBaseURL(platform, baseURL),
		Username:  strings.TrimSpace(credential.Username),
		Email:     strings.TrimSpace(credential.Email),
		Password:  encryptedPassword,
		UpdatedAt: common.GetTimestamp(),
	}
	if stored.Platform == "" {
		stored.Platform = NormalizePlatform(platform)
	}
	if stored.BaseURL == "" {
		stored.BaseURL = normalizeSyncMetadataBaseURL(stored.Platform, baseURL)
	}
	if err := attachEncryptedAuthSessionToStoredCredential(stored, stored.Platform, stored.BaseURL, credential.Session); err != nil {
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
	if snapshot.AuthSession != nil {
		if err := attachEncryptedAuthSessionToStoredCredential(&stored, stored.Platform, stored.BaseURL, snapshot.AuthSession); err != nil {
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
	if prepared.UpdatedAt <= 0 {
		prepared.UpdatedAt = common.GetTimestamp()
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
			strings.TrimSpace(session.NewAPI.UserID) != "" &&
			len(session.NewAPI.Cookies) > 0
	case PlatformSub2API:
		return session.Sub2API != nil &&
			strings.TrimSpace(session.Sub2API.AccessToken) != ""
	default:
		return false
	}
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
