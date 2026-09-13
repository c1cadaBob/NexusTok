package model

import (
	"math"
	"testing"
	"time"

	"github.com/c1cadaBob/NexusTok/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalculateUpstreamKeyWeight(t *testing.T) {
	tests := []struct {
		name            string
		ratio           float64
		expectedWeight  int
		expectedFailure bool
	}{
		{name: "free", ratio: 0, expectedWeight: 2000},
		{name: "discounted", ratio: 0.07, expectedWeight: 1930},
		{name: "one to one", ratio: 1, expectedWeight: 1000},
		{name: "expensive", ratio: 2, expectedWeight: 0},
		{name: "very expensive clamps", ratio: 3, expectedWeight: 0},
		{name: "negative rejected", ratio: -0.1, expectedFailure: true},
		{name: "nan rejected", ratio: math.NaN(), expectedFailure: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			weight, err := CalculateUpstreamKeyWeight(test.ratio)
			if test.expectedFailure {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.expectedWeight, weight)
		})
	}
}

func TestPlatformSiteCredentialEncryptionRoundTrip(t *testing.T) {
	previousSecret := common.CryptoSecret
	common.CryptoSecret = "upstream-credential-test-secret"
	t.Cleanup(func() {
		common.CryptoSecret = previousSecret
	})

	credential := PlatformSiteCredential{
		Username:    "operator@example.invalid",
		Password:    "password-value",
		AccessToken: "access-token-value",
		AdminKey:    "admin-key-value",
		Cookie:      "session-cookie-value",
	}
	ciphertext, err := EncryptPlatformSiteCredential(credential)
	require.NoError(t, err)
	assert.NotContains(t, ciphertext, credential.Password)
	assert.NotContains(t, ciphertext, credential.Cookie)

	decrypted, err := DecryptPlatformSiteCredential(ciphertext)
	require.NoError(t, err)
	assert.Equal(t, credential, decrypted)
}

func TestUpstreamKeyIsRoutable(t *testing.T) {
	now := time.Unix(1_000, 0)
	remaining := int64(100)
	key := UpstreamKey{
		Status:      UpstreamKeyStatusEnabled,
		RemainQuota: &remaining,
		ExpiresAt:   ptrTime(now.Add(time.Minute)),
	}
	assert.True(t, key.IsRoutable(now))

	key.ExpiresAt = ptrTime(now)
	assert.False(t, key.IsRoutable(now))

	key.ExpiresAt = nil
	remaining = 0
	assert.False(t, key.IsRoutable(now))
}

func ptrTime(value time.Time) *time.Time {
	return &value
}
