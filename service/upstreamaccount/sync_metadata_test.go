package upstreamaccount

import (
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/stretchr/testify/require"
)

func TestPreserveChannelSyncCredentialRestoresHiddenCredential(t *testing.T) {
	encrypted, err := common.EncryptSensitiveString("secret")
	require.NoError(t, err)

	existing := `{"upstream_account_sync":{"platform":"new-api","base_url":"https://newapi.example","credentials":{"platform":"new-api","base_url":"https://newapi.example","username":"alice","password":"` + encrypted + `"}},"allow_service_tier":false}`
	sanitized := SanitizeChannelSyncSettings(existing)

	require.Contains(t, sanitized, `"credential_saved":true`)
	require.NotContains(t, sanitized, "credentials")

	preserved := PreserveChannelSyncCredential(existing, sanitized)
	credential, ok, err := ReadChannelSyncCredential(preserved)

	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "new-api", credential.Platform)
	require.Equal(t, "https://newapi.example", credential.BaseURL)
	require.Equal(t, "alice", credential.Username)
	require.Equal(t, "secret", credential.Password)
	require.NotContains(t, preserved, "credential_saved")
}

func TestPreserveChannelSyncCredentialDoesNotCrossSyncSource(t *testing.T) {
	encrypted, err := common.EncryptSensitiveString("secret")
	require.NoError(t, err)

	existing := `{"upstream_account_sync":{"platform":"new-api","base_url":"https://newapi.example","credentials":{"platform":"new-api","base_url":"https://newapi.example","username":"alice","password":"` + encrypted + `"}}}`
	next := `{"upstream_account_sync":{"platform":"new-api","base_url":"https://other.example","credential_saved":true}}`

	preserved := PreserveChannelSyncCredential(existing, next)
	_, ok, err := ReadChannelSyncCredential(preserved)

	require.NoError(t, err)
	require.False(t, ok)
	require.NotContains(t, preserved, "credentials")
	require.NotContains(t, preserved, "credential_saved")
}

func TestSanitizeChannelSyncSettingsDropsStaleCredentialSaved(t *testing.T) {
	stale := `{"upstream_account_sync":{"platform":"new-api","base_url":"https://newapi.example","credential_saved":true},"allow_service_tier":false}`

	sanitized := SanitizeChannelSyncSettings(stale)

	require.Contains(t, sanitized, `"upstream_account_sync"`)
	require.Contains(t, sanitized, `"allow_service_tier":false`)
	require.NotContains(t, sanitized, "credential_saved")
	require.NotContains(t, sanitized, "credentials")
}
