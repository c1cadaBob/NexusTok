// Package codex - metadata.go
// 该文件提供 Codex 认证文件元数据的兼容性归一化工具。
//
// 旧版本 Codex 文件会把 access_token、refresh_token、id_token、email、
// account_id 等字段放在 token_data 或 token 嵌套对象中；当前运行时和大多数
// 新写入路径都更偏好顶层字段。这里把历史结构抬平，避免“导入成功、第一次请求就
// 401”这类问题在不同入口上反复出现。
package codex

import "strings"

const (
	// AccessTokenOnlyCredentialMessage 是 Codex 账号只包含 access_token 时的统一提示。
	// 这类凭据可以用于请求，但不能自动刷新；access_token 过期或上游返回 401 后，
	// 需要重新导入新的 access_token 或使用带 refresh_token 的完整 OAuth 凭据。
	AccessTokenOnlyCredentialMessage = "codex auth file is access_token-only; it can be used until the access_token expires or upstream returns unauthorized, then re-import a fresh credential"
)

var codexTokenContainers = []string{"token_data", "tokenData", "TokenData", "token", "Token"}

// NormalizeMetadata 规范化 Codex 认证文件元数据。
//
// 该函数会保留原始元数据中的其他字段，只把历史上可能嵌套在 token_data/token
// 中的核心凭据抬到顶层，必要时还会从 id_token 中补出 plan_type、email 和 account_id。
func NormalizeMetadata(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return nil
	}

	normalized := cloneMetadata(metadata)
	copyStringField(normalized, "access_token", []string{"accessToken"}, codexTokenContainers, "access_token", "accessToken")
	copyStringField(normalized, "refresh_token", []string{"refreshToken"}, codexTokenContainers, "refresh_token", "refreshToken")
	copyStringField(normalized, "id_token", []string{"idToken"}, codexTokenContainers, "id_token", "idToken")
	copyStringField(normalized, "email", nil, codexTokenContainers, "email")
	copyStringField(normalized, "account_id", []string{"accountId"}, codexTokenContainers, "account_id", "accountId")
	copyAnyField(normalized, "last_refresh", []string{"lastRefresh", "last_refreshed_at", "lastRefreshedAt"}, codexTokenContainers, "last_refresh", "lastRefresh", "last_refreshed_at", "lastRefreshedAt")
	copyAnyField(normalized, "expired", []string{"expire", "expires_at", "expiresAt", "expiry", "expires"}, codexTokenContainers, "expired", "expire", "expires_at", "expiresAt", "expiry", "expires")
	copyStringField(normalized, "plan_type", []string{"planType"}, codexTokenContainers, "plan_type", "planType")

	if planType := ExtractPlanType(normalized); planType != "" {
		normalized["plan_type"] = planType
	}
	if email := ExtractEmail(normalized); email != "" {
		normalized["email"] = email
	}
	if accountID := ExtractAccountID(normalized); accountID != "" {
		normalized["account_id"] = accountID
	}

	return normalized
}

// ExtractAccessToken 从元数据中提取 Codex 的访问令牌。
func ExtractAccessToken(metadata map[string]any) string {
	return extractCodexString(metadata, "access_token", []string{"accessToken"}, codexTokenContainers, "access_token", "accessToken")
}

// ExtractRefreshToken 从元数据中提取 Codex 的刷新令牌。
func ExtractRefreshToken(metadata map[string]any) string {
	return extractCodexString(metadata, "refresh_token", []string{"refreshToken"}, codexTokenContainers, "refresh_token", "refreshToken")
}

// IsAccessTokenOnlyCredential 判断 Codex OAuth/Bearer 类凭据是否只能依赖 access_token。
//
// 有些导出来源无法提供 refresh_token，只能提供短期 access_token。系统允许这类凭据
// 导入并参与调度，但必须标记为不可自动刷新，避免自动刷新循环把“无刷新动作”误判为成功。
// 纯 codex-api-key 文件不包含 access_token，因此不属于此类。
func IsAccessTokenOnlyCredential(metadata map[string]any) bool {
	return ExtractAccessToken(metadata) != "" && ExtractRefreshToken(metadata) == ""
}

// ExtractIDToken 从元数据中提取 Codex 的 ID 令牌。
func ExtractIDToken(metadata map[string]any) string {
	return extractCodexString(metadata, "id_token", []string{"idToken"}, codexTokenContainers, "id_token", "idToken")
}

// ExtractEmail 从元数据中提取邮箱地址。
func ExtractEmail(metadata map[string]any) string {
	if email := extractCodexString(metadata, "email", nil, codexTokenContainers, "email"); email != "" {
		return email
	}
	if claims := parseClaimsFromMetadata(metadata); claims != nil {
		return strings.TrimSpace(claims.Email)
	}
	return ""
}

// ExtractAccountID 从元数据中提取 Codex 账户 ID。
func ExtractAccountID(metadata map[string]any) string {
	if accountID := extractCodexString(metadata, "account_id", []string{"accountId"}, codexTokenContainers, "account_id", "accountId"); accountID != "" {
		return accountID
	}
	if claims := parseClaimsFromMetadata(metadata); claims != nil {
		return strings.TrimSpace(claims.GetAccountID())
	}
	return ""
}

// ExtractPlanType 从元数据中提取 Codex 账号计划类型。
func ExtractPlanType(metadata map[string]any) string {
	if planType := extractCodexString(metadata, "plan_type", []string{"planType"}, codexTokenContainers, "plan_type", "planType"); planType != "" {
		return planType
	}
	if claims := parseClaimsFromMetadata(metadata); claims != nil {
		return strings.TrimSpace(claims.CodexAuthInfo.ChatgptPlanType)
	}
	return ""
}

func cloneMetadata(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return nil
	}
	clone := make(map[string]any, len(metadata))
	for key, value := range metadata {
		clone[key] = value
	}
	return clone
}

func copyStringField(metadata map[string]any, canonical string, aliases []string, containerKeys []string, nestedKeys ...string) {
	if metadata == nil || canonical == "" {
		return
	}
	if value := extractTopLevelString(metadata, append([]string{canonical}, aliases...)...); value != "" {
		metadata[canonical] = value
		return
	}
	if value := extractNestedString(metadata, containerKeys, nestedKeys...); value != "" {
		metadata[canonical] = value
	}
}

func copyAnyField(metadata map[string]any, canonical string, aliases []string, containerKeys []string, nestedKeys ...string) {
	if metadata == nil || canonical == "" {
		return
	}
	if value, ok := extractTopLevelValue(metadata, append([]string{canonical}, aliases...)...); ok {
		metadata[canonical] = value
		return
	}
	if value, ok := extractNestedValue(metadata, containerKeys, nestedKeys...); ok {
		metadata[canonical] = value
	}
}

func extractCodexString(metadata map[string]any, canonical string, aliases []string, containerKeys []string, nestedKeys ...string) string {
	if value := extractTopLevelString(metadata, append([]string{canonical}, aliases...)...); value != "" {
		return value
	}
	return extractNestedString(metadata, containerKeys, nestedKeys...)
}

func extractTopLevelValue(metadata map[string]any, keys ...string) (any, bool) {
	if len(metadata) == 0 {
		return nil, false
	}
	for _, key := range keys {
		if key == "" {
			continue
		}
		if value, ok := metadata[key]; ok {
			return value, true
		}
	}
	return nil, false
}

func extractNestedValue(metadata map[string]any, containerKeys []string, nestedKeys ...string) (any, bool) {
	if len(metadata) == 0 {
		return nil, false
	}
	for _, containerKey := range containerKeys {
		container, ok := metadata[containerKey]
		if !ok || container == nil {
			continue
		}
		switch typed := container.(type) {
		case map[string]any:
			if value, okValue := extractTopLevelValue(typed, nestedKeys...); okValue {
				return value, true
			}
		case map[string]string:
			for _, nestedKey := range nestedKeys {
				if nestedKey == "" {
					continue
				}
				if value, okValue := typed[nestedKey]; okValue {
					return value, true
				}
			}
		}
	}
	return nil, false
}

func extractTopLevelString(metadata map[string]any, keys ...string) string {
	if len(metadata) == 0 {
		return ""
	}
	for _, key := range keys {
		if key == "" {
			continue
		}
		if value, ok := metadata[key]; ok {
			if s := stringValue(value); s != "" {
				return s
			}
		}
	}
	return ""
}

func extractNestedString(metadata map[string]any, containerKeys []string, nestedKeys ...string) string {
	if len(metadata) == 0 {
		return ""
	}
	for _, containerKey := range containerKeys {
		container, ok := metadata[containerKey]
		if !ok || container == nil {
			continue
		}
		switch typed := container.(type) {
		case map[string]any:
			if value := extractTopLevelString(typed, nestedKeys...); value != "" {
				return value
			}
		case map[string]string:
			temp := make(map[string]any, len(typed))
			for key, value := range typed {
				temp[key] = value
			}
			if value := extractTopLevelString(temp, nestedKeys...); value != "" {
				return value
			}
		}
	}
	return ""
}

func parseClaimsFromMetadata(metadata map[string]any) *JWTClaims {
	if len(metadata) == 0 {
		return nil
	}
	idToken := ExtractIDToken(metadata)
	if strings.TrimSpace(idToken) == "" {
		return nil
	}
	claims, err := ParseJWTToken(idToken)
	if err != nil || claims == nil {
		return nil
	}
	return claims
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	}
	return ""
}
