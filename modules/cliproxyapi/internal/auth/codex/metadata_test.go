package codex

import "testing"

// TestNormalizeMetadata_FlattensLegacyTokenData 验证历史 token_data 结构会被抬平。
//
// 这类文件导入后如果缺少顶层 access_token，Codex 执行器会发出空 Bearer，
// 最终表现为上游 401 Unauthorized。
func TestNormalizeMetadata_FlattensLegacyTokenData(t *testing.T) {
	t.Parallel()

	metadata := map[string]any{
		"type": "codex",
		"token_data": map[string]any{
			"access_token":  "access-from-token-data",
			"refresh_token": "refresh-from-token-data",
			"id_token":      "bad.jwt.value",
			"email":         "codex@example.com",
			"account_id":    "acct_123",
			"expired":       "2030-01-01T00:00:00Z",
		},
		"lastRefresh": "2026-01-02T03:04:05Z",
		"account_groups": []any{
			"production",
		},
	}

	normalized := NormalizeMetadata(metadata)
	if got := ExtractAccessToken(normalized); got != "access-from-token-data" {
		t.Fatalf("access token = %q, want access-from-token-data", got)
	}
	if got := ExtractRefreshToken(normalized); got != "refresh-from-token-data" {
		t.Fatalf("refresh token = %q, want refresh-from-token-data", got)
	}
	if got := ExtractIDToken(normalized); got != "bad.jwt.value" {
		t.Fatalf("id token = %q, want bad.jwt.value", got)
	}
	if got := ExtractEmail(normalized); got != "codex@example.com" {
		t.Fatalf("email = %q, want codex@example.com", got)
	}
	if got := ExtractAccountID(normalized); got != "acct_123" {
		t.Fatalf("account ID = %q, want acct_123", got)
	}
	if got, _ := normalized["expired"].(string); got != "2030-01-01T00:00:00Z" {
		t.Fatalf("expired = %q, want 2030-01-01T00:00:00Z", got)
	}
	if got, _ := normalized["last_refresh"].(string); got != "2026-01-02T03:04:05Z" {
		t.Fatalf("last_refresh = %q, want 2026-01-02T03:04:05Z", got)
	}
	if _, ok := metadata["access_token"]; ok {
		t.Fatalf("NormalizeMetadata should not mutate input metadata")
	}
}

// TestNormalizeMetadata_TopLevelTokenWins 验证新格式顶层字段优先。
func TestNormalizeMetadata_TopLevelTokenWins(t *testing.T) {
	t.Parallel()

	normalized := NormalizeMetadata(map[string]any{
		"type":         "codex",
		"access_token": "top-level",
		"token_data": map[string]any{
			"access_token": "nested",
		},
	})
	if got := ExtractAccessToken(normalized); got != "top-level" {
		t.Fatalf("access token = %q, want top-level", got)
	}
}

// TestIsAccessTokenOnlyCredential 验证 Codex AT-only 判断只命中有 access_token、
// 但没有 refresh_token 的短期 Bearer 凭据。
func TestIsAccessTokenOnlyCredential(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		metadata map[string]any
		want     bool
	}{
		{
			name:     "access token without refresh token",
			metadata: map[string]any{"type": "codex", "access_token": "access-token"},
			want:     true,
		},
		{
			name:     "access token with refresh token",
			metadata: map[string]any{"type": "codex", "access_token": "access-token", "refresh_token": "refresh-token"},
			want:     false,
		},
		{
			name:     "nested legacy access token without refresh token",
			metadata: map[string]any{"type": "codex", "token_data": map[string]any{"access_token": "access-token"}},
			want:     true,
		},
		{
			name:     "api key auth",
			metadata: map[string]any{"type": "codex", "api_key": "codex-api-key"},
			want:     false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsAccessTokenOnlyCredential(tt.metadata); got != tt.want {
				t.Fatalf("IsAccessTokenOnlyCredential() = %v, want %v", got, tt.want)
			}
		})
	}
}
