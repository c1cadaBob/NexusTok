package accountauth

import (
	"context"
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"github.com/stretchr/testify/require"
)

func TestCodexBuildChannelKeyNormalizesAccessTokenOnlyCredential(t *testing.T) {
	raw := `{
		"access_token": "access-token",
		"account_id": "account-id",
		"expired": 1791103379,
		"credential_mode": "access_token_only",
		"refreshable": false,
		"type": "codex"
	}`
	encrypted, err := common.EncryptSensitiveString(raw)
	require.NoError(t, err)
	account := &model.PoolAccount{
		Credentials: encrypted,
	}

	key, err := (&CodexProvider{}).BuildChannelKey(account)

	require.NoError(t, err)
	payload := map[string]any{}
	require.NoError(t, common.UnmarshalJsonStr(key, &payload))
	require.Equal(t, "access-token", payload["access_token"])
	require.Equal(t, "account-id", payload["account_id"])
	require.Equal(t, "1791103379", payload["expired"])
	require.Equal(t, "codex", payload["type"])
	require.NotContains(t, payload, "credential_mode")
	require.NotContains(t, payload, "refreshable")
}

func TestCodexRefreshReportsMissingRefreshTokenAfterFlexibleParse(t *testing.T) {
	raw := `{
		"access_token": "access-token",
		"account_id": "account-id",
		"expired": 1791103379,
		"credential_mode": "access_token_only",
		"refreshable": false,
		"type": "codex"
	}`
	encrypted, err := common.EncryptSensitiveString(raw)
	require.NoError(t, err)
	account := &model.PoolAccount{
		Credentials: encrypted,
	}

	_, err = (&CodexProvider{}).Refresh(context.Background(), account)

	require.EqualError(t, err, "refresh_token is required")
}
