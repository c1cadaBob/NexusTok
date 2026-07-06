package service

import (
	"encoding/base64"
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/stretchr/testify/require"
)

func buildCodexTestJWT(t *testing.T, payload map[string]any) string {
	t.Helper()
	headerBytes, err := common.Marshal(map[string]any{"alg": "none", "typ": "JWT"})
	require.NoError(t, err)
	payloadBytes, err := common.Marshal(payload)
	require.NoError(t, err)
	return base64.RawURLEncoding.EncodeToString(headerBytes) + "." +
		base64.RawURLEncoding.EncodeToString(payloadBytes) + ".signature"
}

func TestExtractCodexPlanTypeFromJWT(t *testing.T) {
	token := buildCodexTestJWT(t, map[string]any{
		codexJWTClaimPath: map[string]any{
			"chatgpt_plan_type": "plus",
		},
	})

	planType, ok := ExtractCodexPlanTypeFromJWT(token)

	require.True(t, ok)
	require.Equal(t, "plus", planType)
}
