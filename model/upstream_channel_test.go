package model

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/c1cadaBob/NexusTok/common"
	"github.com/glebarez/sqlite"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

type upstreamCompatibilityLegacyChannel struct {
	Id       int    `gorm:"primaryKey"`
	Name     string `gorm:"size:255"`
	Key      string `gorm:"type:text"`
	Models   string `gorm:"type:text"`
	Group    string `gorm:"size:64"`
	Status   int
	Priority *int64 `gorm:"bigint"`
}

type upstreamCompatibilityLegacyUpstreamKey struct {
	ID                 uint   `gorm:"primaryKey"`
	UpstreamChannelID  int    `gorm:"bigint"`
	ExternalKeyID      string `gorm:"size:255"`
	CredentialEnvelope string `gorm:"type:text"`
}

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

func TestCalculatePlatformKeyConversionRatio(t *testing.T) {
	tests := []struct {
		name            string
		siteRatio       float64
		sourceRatio     float64
		expected        float64
		expectedFailure bool
	}{
		{name: "site ratio only", siteRatio: 0.1, sourceRatio: 1, expected: 0.1},
		{name: "source ratio multiplies site ratio", siteRatio: 0.1, sourceRatio: 0.7, expected: 0.07},
		{name: "free source ratio", siteRatio: 1, sourceRatio: 0, expected: 0},
		{name: "product over limit rejected", siteRatio: 1000, sourceRatio: 2, expectedFailure: true},
		{name: "negative source ratio rejected", siteRatio: 1, sourceRatio: -0.1, expectedFailure: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ratio, err := CalculatePlatformKeyConversionRatio(test.siteRatio, test.sourceRatio)
			if test.expectedFailure {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.InDelta(t, test.expected, ratio, 1e-12)
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

func TestKeyChannelRoutingKeyPreservesConfiguredRatioAndWeight(t *testing.T) {
	override := 321
	channel := &Channel{
		KeyPriority:       7,
		ConversionRatio:   1,
		KeyWeightOverride: &override,
	}

	routingKey := buildKeyChannelRoutingKey(channel)

	require.Equal(t, int64(7), routingKey.KeyPriority)
	require.Equal(t, 1.0, routingKey.ConversionRatio)
	require.Equal(t, 321, routingKey.EffectiveWeight())
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
	previousMainType := common.MainDatabaseType()
	previousLogType := common.LogDatabaseType()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Option{}))
	DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	initCol()
	t.Cleanup(func() {
		DB = previousDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		initCol()
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
	require.NoError(t, db.Where(commonKeyCol+" = ?", "migration.upstream_channel_defaults.v1").First(&marker).Error)
	assert.Equal(t, "1", marker.Value)
}

func TestMigrateUpstreamKeyDefaultsInitializesMissingSourceRatioOnce(t *testing.T) {
	previousDB := DB
	previousMainType := common.MainDatabaseType()
	previousLogType := common.LogDatabaseType()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&UpstreamKey{}, &Option{}))
	DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	initCol()
	t.Cleanup(func() {
		DB = previousDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		initCol()
		sqlDB, closeErr := db.DB()
		if closeErr == nil {
			_ = sqlDB.Close()
		}
	})

	require.NoError(t, db.Create(&UpstreamKey{
		ChannelID:        1,
		ExternalID:       "legacy",
		SecretCiphertext: "ciphertext",
	}).Error)
	freeRatio := 0.0
	require.NoError(t, db.Create(&UpstreamKey{
		ChannelID:             1,
		ExternalID:            "free",
		SecretCiphertext:      "ciphertext",
		SourceConversionRatio: &freeRatio,
	}).Error)

	require.NoError(t, migrateUpstreamKeyDefaults())

	var legacy, free UpstreamKey
	require.NoError(t, db.Where("external_id = ?", "legacy").First(&legacy).Error)
	require.NoError(t, db.Where("external_id = ?", "free").First(&free).Error)
	require.NotNil(t, legacy.SourceConversionRatio)
	assert.Equal(t, 1.0, *legacy.SourceConversionRatio)
	require.NotNil(t, free.SourceConversionRatio)
	assert.Equal(t, 0.0, *free.SourceConversionRatio)

	require.NoError(t, db.Model(&legacy).Update("source_conversion_ratio", 0.5).Error)
	require.NoError(t, migrateUpstreamKeyDefaults())
	require.NoError(t, db.First(&legacy, legacy.ID).Error)
	assert.Equal(t, 0.5, *legacy.SourceConversionRatio)
}

func TestUpstreamChannelDatabaseCompatibility(t *testing.T) {
	tests := []struct {
		name      string
		env       string
		dbType    common.DatabaseType
		dialector func(string) gorm.Dialector
		dsn       func(*testing.T) string
	}{
		{
			name:   "sqlite",
			dbType: common.DatabaseTypeSQLite,
			dialector: func(dsn string) gorm.Dialector {
				return sqlite.Open(dsn)
			},
			dsn: func(t *testing.T) string {
				t.Helper()
				return filepath.Join(t.TempDir(), "upstream-compatibility.db")
			},
		},
		{
			name:   "mysql",
			env:    "TEST_MYSQL_DSN",
			dbType: common.DatabaseTypeMySQL,
			dialector: func(dsn string) gorm.Dialector {
				return mysql.Open(dsn)
			},
		},
		{
			name:   "postgres",
			env:    "TEST_POSTGRES_DSN",
			dbType: common.DatabaseTypePostgreSQL,
			dialector: func(dsn string) gorm.Dialector {
				return postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dsn := ""
			if test.dsn != nil {
				dsn = test.dsn(t)
			} else {
				dsn = strings.TrimSpace(os.Getenv(test.env))
				if dsn == "" {
					t.Skip(test.env + " is not configured")
				}
			}

			prefix := fmt.Sprintf("upstream_%d_", time.Now().UnixNano())
			db, err := gorm.Open(test.dialector(dsn), &gorm.Config{
				NamingStrategy: schema.NamingStrategy{TablePrefix: prefix},
			})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { _ = sqlDB.Close() })
			t.Cleanup(func() {
				_ = db.Migrator().DropTable(
					&UpstreamKeyAbility{},
					&UpstreamKey{},
					&PlatformSiteAccount{},
					&Ability{},
					&Channel{},
					&Option{},
				)
			})

			previousDB := DB
			previousMainType := common.MainDatabaseType()
			previousLogType := common.LogDatabaseType()
			DB = db
			common.SetMainDatabaseType(test.dbType)
			initCol()
			t.Cleanup(func() {
				DB = previousDB
				common.SetDatabaseTypes(previousMainType, previousLogType)
				initCol()
			})

			require.NoError(t, db.Table(prefix+"channels").AutoMigrate(&upstreamCompatibilityLegacyChannel{}))
			require.NoError(t, db.Table(prefix+"channels").Create(&upstreamCompatibilityLegacyChannel{
				Id:     41,
				Name:   "legacy channel",
				Key:    "legacy-key",
				Models: "gpt-legacy",
				Group:  "default",
				Status: common.ChannelStatusEnabled,
			}).Error)
			require.NoError(t, db.Table(prefix+"upstream_keys").AutoMigrate(&upstreamCompatibilityLegacyUpstreamKey{}))
			require.NoError(t, db.Table(prefix+"upstream_keys").Create(&upstreamCompatibilityLegacyUpstreamKey{
				UpstreamChannelID:  41,
				ExternalKeyID:      "legacy-external",
				CredentialEnvelope: "legacy-envelope",
			}).Error)

			for range 2 {
				require.NoError(t, archiveIncompatibleUpstreamKeysTable())
				require.NoError(t, db.AutoMigrate(
					&Channel{},
					&PlatformSiteAccount{},
					&UpstreamKey{},
					&UpstreamKeyAbility{},
					&Ability{},
					&Option{},
				))
				require.NoError(t, migrateUpstreamChannelDefaults())
			}
			assert.True(t, db.Migrator().HasColumn(&UpstreamKey{}, "LastUsedAt"))

			var migratedLegacy Channel
			require.NoError(t, db.First(&migratedLegacy, "id = ?", 41).Error)
			assert.Equal(t, UpstreamKindKeyChannel, migratedLegacy.UpstreamKind)
			assert.Equal(t, 1.0, migratedLegacy.ConversionRatio)

			tables, err := db.Migrator().GetTables()
			require.NoError(t, err)
			archivedTables := make([]string, 0, 1)
			for _, table := range tables {
				if strings.HasPrefix(table, prefix+"upstream_keys_legacy_") {
					archivedTables = append(archivedTables, table)
				}
			}
			require.Len(t, archivedTables, 1)
			t.Cleanup(func() { _ = db.Migrator().DropTable(archivedTables[0]) })
			var archivedCount int64
			require.NoError(t, db.Table(archivedTables[0]).Count(&archivedCount).Error)
			assert.EqualValues(t, 1, archivedCount)

			var currentKeyCount int64
			require.NoError(t, db.Model(&UpstreamKey{}).Count(&currentKeyCount).Error)
			assert.Zero(t, currentKeyCount)

			priority := int64(9)
			channel := &Channel{
				Name:         "parent-site",
				Key:          "parent-placeholder",
				Group:        "default",
				Status:       common.ChannelStatusEnabled,
				Priority:     &priority,
				UpstreamKind: UpstreamKindPlatformSite,
			}
			require.NoError(t, db.Create(channel).Error)
			account := &PlatformSiteAccount{
				ChannelID:       channel.Id,
				Platform:        PlatformNewAPI,
				BaseURL:         "https://upstream.example",
				AuthType:        UpstreamAuthAccessToken,
				ConversionRatio: 0.1,
			}
			require.NoError(t, db.Create(account).Error)
			secret, err := EncryptPlatformSiteCredential(PlatformSiteCredential{AccessToken: "fixture-db-compatibility-secret"})
			require.NoError(t, err)
			remaining := int64(100)
			key := &UpstreamKey{
				ChannelID:        channel.Id,
				ExternalID:       "external-1",
				Name:             "child-searchable",
				SecretCiphertext: secret,
				Models:           "gpt-4o-mini",
				KeyPriority:      3,
				ConversionRatio:  0.1,
				Weight:           1900,
				RemainQuota:      &remaining,
				Status:           UpstreamKeyStatusEnabled,
			}
			require.NoError(t, db.Create(key).Error)
			require.NoError(t, UpdateUpstreamKeyLastUsed(key.ID, 1_800_000_000))
			var keyWithLastUsed UpstreamKey
			require.NoError(t, db.First(&keyWithLastUsed, key.ID).Error)
			assert.EqualValues(t, 1_800_000_000, keyWithLastUsed.LastUsedAt)
			require.NoError(t, db.Create(&UpstreamKeyAbility{
				UpstreamKeyID: key.ID,
				Group:         "default",
				Model:         "gpt-4o-mini",
				Enabled:       true,
			}).Error)

			duplicateKey := *key
			duplicateKey.ID = 0
			assert.Error(t, db.Create(&duplicateKey).Error)
			assert.Error(t, db.Create(&UpstreamKeyAbility{
				UpstreamKeyID: key.ID,
				Group:         "default",
				Model:         "gpt-4o-mini",
				Enabled:       true,
			}).Error)

			found, err := SearchChannels("child-searchable", "default", "gpt-4o-mini", true)
			require.NoError(t, err)
			require.Len(t, found, 1)
			assert.Equal(t, channel.Id, found[0].Id)

			missingGroup, err := SearchChannels("child-searchable", "vip", "gpt-4o-mini", true)
			require.NoError(t, err)
			assert.Empty(t, missingGroup)

			require.NoError(t, DeleteUpstreamData(db, []int{channel.Id}))
			var accountCount int64
			require.NoError(t, db.Model(&PlatformSiteAccount{}).Where("channel_id = ?", channel.Id).Count(&accountCount).Error)
			assert.Zero(t, accountCount)
			var keyCount int64
			require.NoError(t, db.Model(&UpstreamKey{}).Where("channel_id = ?", channel.Id).Count(&keyCount).Error)
			assert.Zero(t, keyCount)
			var abilityCount int64
			require.NoError(t, db.Model(&UpstreamKeyAbility{}).Where("upstream_key_id = ?", key.ID).Count(&abilityCount).Error)
			assert.Zero(t, abilityCount)
		})
	}
}

func TestUpstreamKeyIsRoutable(t *testing.T) {
	now := time.Unix(1_000, 0)
	remaining := int64(100)
	key := UpstreamKey{
		Status:       UpstreamKeyStatusEnabled,
		ModelsSynced: true,
		RemainQuota:  &remaining,
		ExpiresAt:    ptrTime(now.Add(time.Minute)),
	}
	assert.True(t, key.IsRoutable(now))

	key.ExpiresAt = ptrTime(now)
	assert.False(t, key.IsRoutable(now))

	key.ExpiresAt = nil
	remaining = 0
	assert.False(t, key.IsRoutable(now))

	remaining = 100
	key.MissingSince = now.Unix()
	assert.False(t, key.IsRoutable(now))
}

func TestGetRoutableUpstreamKeyByIDLoadsSecretAndFiltersModels(t *testing.T) {
	previousDB := DB
	previousSecret := common.CryptoSecret
	common.CryptoSecret = "upstream-key-by-id-test-secret"
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&Channel{},
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

	channel := &Channel{
		Id:           701,
		Name:         "platform-site",
		Status:       common.ChannelStatusEnabled,
		UpstreamKind: UpstreamKindPlatformSite,
		Group:        "default",
	}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&PlatformSiteAccount{
		ChannelID:  channel.Id,
		Platform:   PlatformNewAPI,
		BaseURL:    "https://upstream.example",
		AuthType:   UpstreamAuthAccessToken,
		SyncStatus: UpstreamSiteSyncSuccess,
	}).Error)
	secret, err := EncryptPlatformSiteCredential(PlatformSiteCredential{AccessToken: "sk-routable"})
	require.NoError(t, err)
	remaining := int64(100)
	key := &UpstreamKey{
		ChannelID:        channel.Id,
		ExternalID:       "external-routable",
		SecretCiphertext: secret,
		Models:           "gpt-allowed",
		KeyPriority:      3,
		ConversionRatio:  0.2,
		Weight:           1800,
		RemainQuota:      &remaining,
		Status:           UpstreamKeyStatusEnabled,
		ModelsSynced:     true,
	}
	require.NoError(t, db.Create(key).Error)
	require.NoError(t, db.Create(&UpstreamKeyAbility{
		UpstreamKeyID: key.ID,
		Group:         "default",
		Model:         "gpt-allowed",
		Enabled:       true,
	}).Error)

	selected, err := GetRoutableUpstreamKeyByID(channel.Id, key.ID, "default", "gpt-allowed", time.Now())
	require.NoError(t, err)
	assert.Equal(t, key.ID, selected.ID)
	assert.Equal(t, "sk-routable", selected.Secret)

	_, err = GetRoutableUpstreamKeyByID(channel.Id, key.ID, "default", "gpt-blocked", time.Now())
	require.Error(t, err)

	_, err = GetRoutableUpstreamKeyByID(channel.Id+1, key.ID, "default", "gpt-allowed", time.Now())
	require.Error(t, err)
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
			ModelsSynced:     true,
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
			ModelsSynced:     true,
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

func TestPlatformSiteRoutingUsesModelMappingWithChildAbilities(t *testing.T) {
	previousDB := DB
	previousSecret := common.CryptoSecret
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	previousGroup2Model2Channels := group2model2channels
	previousChannelsIDM := channelsIDM
	previousAdvancedCustomConfig := channel2advancedCustomConfig
	common.CryptoSecret = "upstream-routing-model-mapping-test-secret"
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
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		channelSyncLock.Lock()
		group2model2channels = previousGroup2Model2Channels
		channelsIDM = previousChannelsIDM
		channel2advancedCustomConfig = previousAdvancedCustomConfig
		channelSyncLock.Unlock()
		sqlDB, closeErr := db.DB()
		if closeErr == nil {
			_ = sqlDB.Close()
		}
	})

	mappingBytes, err := common.Marshal(map[string]string{"alias-model": "gpt-real"})
	require.NoError(t, err)
	mapping := string(mappingBytes)
	priority := int64(7)
	channel := &Channel{
		Id:           401,
		Name:         "mapped-platform",
		Status:       common.ChannelStatusEnabled,
		UpstreamKind: UpstreamKindPlatformSite,
		Models:       "gpt-real",
		Group:        "default",
		Priority:     &priority,
		ModelMapping: &mapping,
	}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&PlatformSiteAccount{
		ChannelID:  channel.Id,
		Platform:   PlatformNewAPI,
		BaseURL:    "https://upstream.example",
		AuthType:   UpstreamAuthAccessToken,
		SyncStatus: UpstreamSiteSyncSuccess,
	}).Error)
	secret, err := EncryptPlatformSiteCredential(PlatformSiteCredential{AccessToken: "sk-real"})
	require.NoError(t, err)
	remaining := int64(100)
	key := &UpstreamKey{
		ChannelID:        channel.Id,
		ExternalID:       "mapped-key",
		SecretCiphertext: secret,
		Models:           "gpt-real",
		KeyPriority:      2,
		ConversionRatio:  0.1,
		Weight:           1900,
		RemainQuota:      &remaining,
		Status:           UpstreamKeyStatusEnabled,
		ModelsSynced:     true,
	}
	require.NoError(t, db.Create(key).Error)
	require.NoError(t, db.Create(&UpstreamKeyAbility{
		UpstreamKeyID: key.ID,
		Model:         "gpt-real",
		Enabled:       true,
	}).Error)

	selected := selectChannelByUpstreamKey([]*Channel{channel}, "default", "alias-model")
	require.NotNil(t, selected)
	require.NotNil(t, selected.SelectedUpstreamKey)
	assert.Equal(t, key.ID, selected.SelectedUpstreamKey.ID)
	assert.Equal(t, "sk-real", selected.SelectedUpstreamKey.Secret)

	common.MemoryCacheEnabled = false
	assert.True(t, IsChannelEnabledForGroupModel("default", "alias-model", channel.Id))
	assert.False(t, IsChannelEnabledForGroupModel("default", "blocked-model", channel.Id))

	common.MemoryCacheEnabled = true
	InitChannelCache()
	cached, err := GetRandomSatisfiedChannel("default", "alias-model", 0, nil)
	require.NoError(t, err)
	require.NotNil(t, cached)
	require.NotNil(t, cached.SelectedUpstreamKey)
	assert.Equal(t, channel.Id, cached.Id)
	assert.Equal(t, key.ID, cached.SelectedUpstreamKey.ID)

	assert.Contains(t, GetGroupEnabledModels("default"), "alias-model")
}

func TestSelectChannelByUpstreamKeyMergesOfficialKeyChannelWithPlatformKey(t *testing.T) {
	previousDB := DB
	previousSecret := common.CryptoSecret
	common.CryptoSecret = "upstream-routing-official-key-test-secret"
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

	priority := int64(6)
	officialKeyPriority := int64(4)
	platformKeyPriority := int64(4)
	official := &Channel{
		Id:              201,
		Name:            "official",
		Type:            1,
		Key:             "sk-official",
		Status:          common.ChannelStatusEnabled,
		UpstreamKind:    UpstreamKindKeyChannel,
		Models:          "gpt-4o",
		Group:           "default",
		Priority:        &priority,
		KeyPriority:     officialKeyPriority,
		ConversionRatio: 1,
	}
	platform := &Channel{
		Id:           202,
		Name:         "platform",
		Status:       common.ChannelStatusEnabled,
		UpstreamKind: UpstreamKindPlatformSite,
		Models:       "gpt-4o",
		Group:        "default",
		Priority:     &priority,
	}
	require.NoError(t, db.Create(official).Error)
	require.NoError(t, db.Create(platform).Error)
	require.NoError(t, db.Create(&PlatformSiteAccount{
		ChannelID:  platform.Id,
		SyncStatus: UpstreamSiteSyncSuccess,
	}).Error)

	secret, err := EncryptPlatformSiteCredential(PlatformSiteCredential{AccessToken: "sk-platform"})
	require.NoError(t, err)
	remaining := int64(100)
	require.NoError(t, db.Create(&UpstreamKey{
		ChannelID:        platform.Id,
		ExternalID:       "platform-key",
		SecretCiphertext: secret,
		Models:           "gpt-4o",
		KeyPriority:      platformKeyPriority,
		ConversionRatio:  0,
		Weight:           2000,
		RemainQuota:      &remaining,
		Status:           UpstreamKeyStatusEnabled,
		ModelsSynced:     true,
	}).Error)

	for range 10 {
		selected := selectChannelByUpstreamKey([]*Channel{official, platform}, "default", "gpt-4o")
		require.NotNil(t, selected)
		if selected.Id == official.Id {
			require.Empty(t, selected.SelectedUpstreamKey)
		} else {
			require.NotNil(t, selected.SelectedUpstreamKey)
			assert.Equal(t, "sk-platform", selected.SelectedUpstreamKey.Secret)
		}
	}
}

func TestSelectChannelByUpstreamKeyFallsBackWhenAllWeightsAreZero(t *testing.T) {
	previousDB := DB
	previousSecret := common.CryptoSecret
	common.CryptoSecret = "upstream-routing-zero-weight-test-secret"
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&Channel{},
		&PlatformSiteAccount{},
		&UpstreamKey{},
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

	priority := int64(1)
	channels := []*Channel{
		{
			Id:              301,
			Name:            "zero-a",
			Status:          common.ChannelStatusEnabled,
			UpstreamKind:    UpstreamKindKeyChannel,
			Key:             "sk-a",
			Models:          "gpt-4o",
			Group:           "default",
			Priority:        &priority,
			KeyPriority:     1,
			ConversionRatio: 2,
		},
		{
			Id:              302,
			Name:            "zero-b",
			Status:          common.ChannelStatusEnabled,
			UpstreamKind:    UpstreamKindKeyChannel,
			Key:             "sk-b",
			Models:          "gpt-4o",
			Group:           "default",
			Priority:        &priority,
			KeyPriority:     1,
			ConversionRatio: 3,
		},
	}

	for range 10 {
		selected := selectChannelByUpstreamKey(channels, "default", "gpt-4o")
		require.NotNil(t, selected)
		assert.Contains(t, []int{301, 302}, selected.Id)
	}
}

func ptrTime(value time.Time) *time.Time {
	return &value
}
