package service

import (
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/stretchr/testify/require"
)

func TestParseAccountPoolAuthFileFillsCodexPlanTypeFromJWT(t *testing.T) {
	token := buildCodexTestJWT(t, map[string]any{
		codexJWTClaimPath: map[string]any{
			"chatgpt_account_id": "acc-test",
			"chatgpt_plan_type":  "pro",
		},
		"email": "codex@example.com",
	})
	contentBytes, err := common.Marshal(map[string]any{
		"provider":     "codex",
		"access_token": token,
	})
	require.NoError(t, err)

	parsed, err := ParseAccountPoolAuthFile(string(contentBytes), AccountPoolAuthFileImportOptions{})

	require.NoError(t, err)
	require.Equal(t, "pro", parsed.Metadata["plan_type"])
	require.Equal(t, "acc-test", parsed.Metadata["account_id"])
	require.Equal(t, "codex@example.com", parsed.Metadata["email"])
}
