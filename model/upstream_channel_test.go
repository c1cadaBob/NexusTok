package model

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/c1cadaBob/NexusTok/common"
	"github.com/glebarez/sqlite"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
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
		{name: "maximum supported ratio clamps", ratio: MaxUpstreamConversionRatio, expectedWeight: 0},
		{name: "negative rejected", ratio: -0.1, expectedFailure: true},
		{name: "nan rejected", ratio: math.NaN(), expectedFailure: true},
		{name: "ratio above maximum rejected", ratio: MaxUpstreamConversionRatio + 1, expectedFailure: true},
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

func TestUpstreamKeyFreeWeightCannotBeOverridden(t *testing.T) {
	override := 1
	key := UpstreamKey{
		ConversionRatio: 0,
		Weight:          0,
		WeightOverride:  &override,
	}
	assert.Equal(t, MaxUpstreamKeyWeight, key.EffectiveWeight())
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

func TestMigrateUpstreamChannelDefaultsIsIdempotent(t *testing.T) {
	previousDB := DB
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Option{}))
	DB = db
	t.Cleanup(func() {
		DB = previousDB
		sqlDB, closeErr := db.DB()
		if closeErr == nil {
			_ = sqlDB.Close()
		}
	})

	legacy := &Channel{Id: 301, Name: "legacy", ConversionRatio: 0}
	explicitFree := &Channel{
		Id:              302,
		Name:            "explicit-free",
		UpstreamKind:    UpstreamKindKeyChannel,
		ConversionRatio: 0,
	}
	require.NoError(t, db.Create(legacy).Error)
	require.NoError(t, db.Create(explicitFree).Error)

	require.NoError(t, migrateUpstreamChannelDefaults())

	var migratedLegacy Channel
	require.NoError(t, db.First(&migratedLegacy, legacy.Id).Error)
	assert.Equal(t, UpstreamKindKeyChannel, migratedLegacy.UpstreamKind)
	assert.Equal(t, 1.0, migratedLegacy.ConversionRatio)

	require.NoError(t, db.Model(explicitFree).Update("conversion_ratio", 0).Error)
	require.NoError(t, migrateUpstreamChannelDefaults())

	var savedExplicitFree Channel
	require.NoError(t, db.First(&savedExplicitFree, explicitFree.Id).Error)
	assert.Equal(t, 0.0, savedExplicitFree.ConversionRatio)

	var marker Option
	require.NoError(t, db.Where("key = ?", "migration.upstream_channel_defaults.v1").First(&marker).Error)
	assert.Equal(t, "1", marker.Value)
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

func TestSelectChannelByUpstreamKeyMergesParentsAndFiltersChildren(t *testing.T) {
	previousDB := DB
	previousSecret := common.CryptoSecret
	common.CryptoSecret = "upstream-routing-test-secret"
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&Channel{},
		&Ability{},
		&PlatformSiteAccount{},
		&UpstreamKey{},
		&UpstreamKeyAbility{},
	))
	DB = db
	t.Cleanup(func() {
		DB = previousDB
		common.CryptoSecret = previousSecret
		sqlDB, closeErr := db.DB()
		if closeErr == nil {
			_ = sqlDB.Close()
		}
	})

	priority := int64(5)
	channels := []*Channel{
		{
			Id:           101,
			Name:         "site-a",
			Status:       common.ChannelStatusEnabled,
			UpstreamKind: UpstreamKindPlatformSite,
			Models:       "gpt-4o",
			Group:        "default",
			Priority:     &priority,
		},
		{
			Id:           102,
			Name:         "site-b",
			Status:       common.ChannelStatusEnabled,
			UpstreamKind: UpstreamKindPlatformSite,
			Models:       "gpt-4o",
			Group:        "default",
			Priority:     &priority,
		},
	}
	require.NoError(t, db.Create(&channels).Error)
	require.NoError(t, db.Create(&[]PlatformSiteAccount{
		{
			ChannelID:  101,
			SyncStatus: UpstreamSiteSyncSuccess,
		},
		{
			ChannelID:  102,
			SyncStatus: UpstreamSiteSyncSuccess,
		},
	}).Error)

	secretA, err := EncryptPlatformSiteCredential(PlatformSiteCredential{AccessToken: "sk-a"})
	require.NoError(t, err)
	secretB, err := EncryptPlatformSiteCredential(PlatformSiteCredential{AccessToken: "sk-b"})
	require.NoError(t, err)
	remaining := int64(100)
	keys := []UpstreamKey{
		{
			ChannelID:        101,
			ExternalID:       "a",
			SecretCiphertext: secretA,
			Models:           "gpt-4o",
			KeyPriority:      1,
			ConversionRatio:  0.1,
			Weight:           1900,
			RemainQuota:      &remaining,
			Status:           UpstreamKeyStatusEnabled,
		},
		{
			ChannelID:        102,
			ExternalID:       "b",
			SecretCiphertext: secretB,
			Models:           "claude-3-7-sonnet",
			KeyPriority:      9,
			ConversionRatio:  1,
			Weight:           1000,
			RemainQuota:      &remaining,
			Status:           UpstreamKeyStatusEnabled,
		},
	}
	require.NoError(t, db.Create(&keys).Error)
	require.NoError(t, db.Create(&UpstreamKeyAbility{
		UpstreamKeyID: keys[0].ID,
		Group:         "default",
		Model:         "gpt-4o",
		Enabled:       true,
	}).Error)
	require.NoError(t, db.Create(&UpstreamKeyAbility{
		UpstreamKeyID: keys[1].ID,
		Group:         "default",
		Model:         "claude-3-7-sonnet",
		Enabled:       true,
	}).Error)

	selected := selectChannelByUpstreamKey(channels, "default", "gpt-4o")
	require.NotNil(t, selected)
	require.NotNil(t, selected.SelectedUpstreamKey)
	assert.Equal(t, uint(keys[0].ID), selected.SelectedUpstreamKey.ID)
	assert.Equal(t, "sk-a", selected.SelectedUpstreamKey.Secret)
}

func ptrTime(value time.Time) *time.Time {
	return &value
}
