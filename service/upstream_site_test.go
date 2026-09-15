package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/c1cadaBob/NexusTok/common"
	"github.com/c1cadaBob/NexusTok/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestNewAPIAdapterPasswordAuthenticationAndSnapshot(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/api/user/login":
			body, readErr := io.ReadAll(request.Body)
			require.NoError(t, readErr)
			assert.Contains(t, string(body), `"username":"operator"`)
			writer.WriteHeader(http.StatusOK)
			_, _ = writer.Write([]byte(`{"success":true,"data":{"token":"newapi-session"}}`))
		case request.URL.Path == "/api/status":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"quota_per_unit":10}}`))
		case request.URL.Path == "/api/user/self":
			assert.Equal(t, "Bearer newapi-session", request.Header.Get("Authorization"))
			_, _ = writer.Write([]byte(`{"success":true,"data":{"quota":12.5,"used_quota":3}}`))
		case request.URL.Path == "/api/user/self/groups":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"default":{"ratio":0.7,"desc":"默认组"}}}`))
		case request.URL.Path == "/api/user/models":
			_, _ = writer.Write([]byte(`{"success":true,"data":["gpt-4o","claude-3-7-sonnet"]}`))
		case request.URL.Path == "/api/token/":
			assert.Equal(t, "1", request.URL.Query().Get("p"))
			assert.Equal(t, "100", request.URL.Query().Get("page_size"))
			_, _ = writer.Write([]byte(`{"success":true,"data":{"items":[{"id":7,"name":"primary","key":"sk-newapi","group":"default","quota":8,"expired_time":"4102444800","model_limits":"gpt-4o,claude-3-7-sonnet"}]}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	adapter := NewNewAPIAdapter(server.Client())
	session, err := adapter.Authenticate(context.Background(), server.URL, model.PlatformSiteCredential{
		Username: "operator",
		Password: "synthetic-password",
	})
	require.NoError(t, err)

	snapshot, err := adapter.FetchSnapshot(context.Background(), session)
	require.NoError(t, err)
	assert.Equal(t, 1.25, snapshot.Balance)
	require.Len(t, snapshot.Keys, 1)
	assert.Equal(t, "sk-newapi", snapshot.Keys[0].Secret)
	assert.Equal(t, int64(8), *snapshot.Keys[0].RemainQuota)
	assert.Equal(t, []string{"gpt-4o", "claude-3-7-sonnet"}, snapshot.Keys[0].Models)
	assert.Equal(t, 0.7, snapshot.Keys[0].SourceConversionRatio)
}

func TestNewAPIAdapterPasswordAuthenticationAddsCompatUserHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/api/user/login":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"id":275,"username":"operator","status":1}}`))
		case request.URL.Path == "/api/user/self":
			assert.Empty(t, request.Header.Get("Authorization"))
			assert.Equal(t, "275", request.Header.Get("New-API-User"))
			assert.Equal(t, "275", request.Header.Get("X-ModelFlare-User"))
			assert.Equal(t, "275", request.Header.Get("User-id"))
			_, _ = writer.Write([]byte(`{"success":true,"data":{"quota":12.5,"used_quota":3}}`))
		case request.URL.Path == "/api/token/":
			assert.Equal(t, "275", request.Header.Get("New-API-User"))
			_, _ = writer.Write([]byte(`{"success":true,"data":{"items":[{"id":7,"name":"primary","key":"sk-newapi","models":["gpt-4o"]}]}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	adapter := NewNewAPIAdapter(server.Client())
	session, err := adapter.Authenticate(context.Background(), server.URL, model.PlatformSiteCredential{
		Username: "operator",
		Password: "synthetic-password",
	})
	require.NoError(t, err)

	snapshot, err := adapter.FetchSnapshot(context.Background(), session)
	require.NoError(t, err)
	require.Len(t, snapshot.Keys, 1)
	assert.Equal(t, []string{"gpt-4o"}, snapshot.Keys[0].Models)
}

func TestNewAPIAdapterAdminKeySkipsPasswordLogin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/api/user/login" {
			t.Fatalf("admin key authentication must not submit a password login")
		}
		if request.URL.Path == "/api/user/self" {
			assert.Equal(t, "admin-secret", request.Header.Get("x-api-key"))
			_, _ = writer.Write([]byte(`{"success":true,"data":{"quota":1}}`))
			return
		}
		http.NotFound(writer, request)
	}))
	defer server.Close()

	adapter := NewNewAPIAdapter(server.Client())
	_, err := adapter.Authenticate(context.Background(), server.URL, model.PlatformSiteCredential{
		AdminKey: "admin-secret",
	})
	require.NoError(t, err)
}

func TestNewAPIAdapterRefreshesRotatingSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/user/auth/refresh":
			assert.Equal(t, "Bearer old-access", request.Header.Get("Authorization"))
			body, readErr := io.ReadAll(request.Body)
			require.NoError(t, readErr)
			assert.Contains(t, string(body), `"refresh_token":"old-refresh"`)
			_, _ = writer.Write([]byte(`{"success":true,"data":{"access_token":"new-access","refresh_token":"new-refresh","expires_in":3600}}`))
		case "/api/user/self":
			assert.Equal(t, "Bearer new-access", request.Header.Get("Authorization"))
			_, _ = writer.Write([]byte(`{"success":true,"data":{"quota":1}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	adapter := NewNewAPIAdapter(server.Client())
	session, err := adapter.Authenticate(context.Background(), server.URL, model.PlatformSiteCredential{
		AccessToken:  "old-access",
		RefreshToken: "old-refresh",
	})
	require.NoError(t, err)
	require.NotNil(t, session.CredentialUpdate)
	assert.Equal(t, "new-access", session.CredentialUpdate.AccessToken)
	assert.Equal(t, "new-refresh", session.CredentialUpdate.RefreshToken)
	assert.Greater(t, session.CredentialUpdate.TokenExpiresAt, common.GetTimestamp())
}

func TestNewAPIAdapterRejectsInteractiveLoginVerification(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/api/user/login" {
			_, _ = writer.Write([]byte(`{"success":true,"data":{"require_2fa":true,"flow_token":"flow"}}`))
			return
		}
		http.NotFound(writer, request)
	}))
	defer server.Close()

	adapter := NewNewAPIAdapter(server.Client())
	_, err := adapter.Authenticate(context.Background(), server.URL, model.PlatformSiteCredential{
		Username: "operator",
		Password: "synthetic-password",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPlatformSiteAuth)
	assert.NotContains(t, err.Error(), "flow")
}

func TestSub2APIAdapterRefreshesRotatingSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/auth/refresh":
			assert.Equal(t, "Bearer old-access", request.Header.Get("Authorization"))
			body, readErr := io.ReadAll(request.Body)
			require.NoError(t, readErr)
			assert.Contains(t, string(body), `"refresh_token":"old-refresh"`)
			_, _ = writer.Write([]byte(`{"code":0,"data":{"access_token":"new-access","refresh_token":"new-refresh","expires_in":3600}}`))
		case "/api/v1/auth/me":
			assert.Equal(t, "Bearer new-access", request.Header.Get("Authorization"))
			_, _ = writer.Write([]byte(`{"code":0,"data":{"balance":1}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	adapter := NewSub2APIAdapter(server.Client())
	session, err := adapter.Authenticate(context.Background(), server.URL, model.PlatformSiteCredential{
		AccessToken:  "old-access",
		RefreshToken: "old-refresh",
	})
	require.NoError(t, err)
	require.NotNil(t, session.CredentialUpdate)
	assert.Equal(t, "new-access", session.CredentialUpdate.AccessToken)
	assert.Equal(t, "new-refresh", session.CredentialUpdate.RefreshToken)
}

func TestSub2APIAdapterAdminKeyReadsNestedCredentialAndPagination(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/auth/me":
			assert.Equal(t, "admin-secret", request.Header.Get("x-api-key"))
			_, _ = writer.Write([]byte(`{"data":{"balance":4,"used_quota":1}}`))
		case "/api/v1/groups/rates":
			_, _ = writer.Write([]byte(`{"data":{"default":0.5}}`))
		case "/v1/models":
			_, _ = writer.Write([]byte(`{"data":[{"id":"gpt-4o"}]}`))
		case "/api/v1/admin/accounts":
			assert.Equal(t, "apikey", request.URL.Query().Get("type"))
			assert.Equal(t, "1", request.URL.Query().Get("page"))
			assert.Equal(t, "name", request.URL.Query().Get("sort_by"))
			assert.Equal(t, "asc", request.URL.Query().Get("sort_order"))
			_, _ = writer.Write([]byte(`{"data":{"accounts":[{"id":"account-1","name":"managed","group":"default","quota":9}],"total":1,"page_size":100}}`))
		case "/api/v1/admin/accounts/data":
			_, _ = writer.Write([]byte(`{"data":{"accounts":[{"id":"account-1","credentials":{"api_key":"sk-sub2api"}}]}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	adapter := NewSub2APIAdapter(server.Client())
	session, err := adapter.Authenticate(context.Background(), server.URL, model.PlatformSiteCredential{
		AdminKey: "admin-secret",
	})
	require.NoError(t, err)

	snapshot, err := adapter.FetchSnapshot(context.Background(), session)
	require.NoError(t, err)
	assert.Equal(t, float64(4), snapshot.Balance)
	require.Len(t, snapshot.Keys, 1)
	assert.Equal(t, "sk-sub2api", snapshot.Keys[0].Secret)
	assert.Equal(t, 0.5, snapshot.Keys[0].ConversionRatio)
	assert.Equal(t, []string{"gpt-4o"}, snapshot.Keys[0].Models)
}

func TestSub2APIAdapterUsesProfileUsageGroupAliasesAndModelAliases(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/auth/login":
			_, _ = writer.Write([]byte(`{"code":0,"data":{"access_token":"sub2api-session"}}`))
		case "/api/v1/auth/me":
			assert.Equal(t, "Bearer sub2api-session", request.Header.Get("Authorization"))
			_, _ = writer.Write([]byte(`{"code":0,"data":{"id":9,"balance":0}}`))
		case "/api/v1/user/profile":
			_, _ = writer.Write([]byte(`{"code":0,"data":{"balance":6.5}}`))
		case "/api/v1/usage/dashboard/stats":
			_, _ = writer.Write([]byte(`{"code":0,"data":{"total_actual_cost":2}}`))
		case "/api/v1/groups/available":
			_, _ = writer.Write([]byte(`{"code":0,"data":[{"id":"group-1","name":"default","rate_multiplier":0.25}]}`))
		case "/api/v1/groups/rates":
			_, _ = writer.Write([]byte(`{"code":0,"data":{}}`))
		case "/api/v1/keys":
			_, _ = writer.Write([]byte(`{"code":0,"data":{"items":[{"id":"key-1","name":"primary","key":"sk-sub2api","group":{"id":"group-1","name":"default"},"model_limits":"gpt-4o,gemini-2.5-pro","quota":9,"quota_used":2}],"total":1,"page_size":100}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	adapter := NewSub2APIAdapter(server.Client())
	session, err := adapter.Authenticate(context.Background(), server.URL, model.PlatformSiteCredential{
		Username: "operator@example.com",
		Password: "synthetic-password",
	})
	require.NoError(t, err)

	snapshot, err := adapter.FetchSnapshot(context.Background(), session)
	require.NoError(t, err)
	assert.Equal(t, 6.5, snapshot.Balance)
	assert.Equal(t, int64(2), snapshot.UsedQuota)
	require.Len(t, snapshot.Keys, 1)
	assert.Equal(t, "default", snapshot.Keys[0].Group)
	assert.Equal(t, 0.25, snapshot.Keys[0].SourceConversionRatio)
	assert.Equal(t, []string{"gpt-4o", "gemini-2.5-pro"}, snapshot.Keys[0].Models)
	assert.True(t, snapshot.Keys[0].ModelsSynced)
}

func TestNewAPIAdapterReturnsUnavailableKeyWhenKeyRevealFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/user/self":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"quota":12.5}}`))
		case "/api/user/models":
			_, _ = writer.Write([]byte(`{"success":true,"data":["gpt-4o"]}`))
		case "/api/token/":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"items":[{"id":7,"name":"primary","key":"sk-****"}]}}`))
		case "/api/token/7/key":
			writer.WriteHeader(http.StatusForbidden)
			_, _ = writer.Write([]byte(`{"success":false,"message":"verification required"}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	adapter := NewNewAPIAdapter(server.Client())
	session, err := adapter.Authenticate(context.Background(), server.URL, model.PlatformSiteCredential{
		AccessToken: "session-token",
	})
	require.NoError(t, err)
	snapshot, err := adapter.FetchSnapshot(context.Background(), session)
	require.NoError(t, err)
	require.Len(t, snapshot.Keys, 1)
	assert.Equal(t, "7", snapshot.Keys[0].ExternalID)
	assert.Equal(t, upstreamKeySyncErrorSecretUnavailable, snapshot.Keys[0].SyncError)
	assert.Empty(t, snapshot.Keys[0].Secret)
}

func TestPersistPlatformSiteSnapshotIsolatesUnavailableKeys(t *testing.T) {
	previousDB := model.DB
	previousSecret := common.CryptoSecret
	common.CryptoSecret = "upstream-site-isolation-test-secret"
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Channel{},
		&model.Ability{},
		&model.PlatformSiteAccount{},
		&model.UpstreamKey{},
		&model.UpstreamKeyAbility{},
	))
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		common.CryptoSecret = previousSecret
		sqlDB, closeErr := db.DB()
		if closeErr == nil {
			_ = sqlDB.Close()
		}
	})

	priority := int64(3)
	channel := &model.Channel{
		Id:           12,
		Name:         "site",
		Status:       common.ChannelStatusEnabled,
		UpstreamKind: model.UpstreamKindPlatformSite,
		Group:        "default",
		Priority:     &priority,
	}
	require.NoError(t, db.Create(channel).Error)
	account := &model.PlatformSiteAccount{
		ChannelID:       channel.Id,
		Platform:        model.PlatformNewAPI,
		BaseURL:         "https://upstream.example",
		ConversionRatio: 0.1,
	}
	require.NoError(t, db.Create(account).Error)

	oldSecret, err := model.EncryptPlatformSiteCredential(model.PlatformSiteCredential{AccessToken: "sk-old"})
	require.NoError(t, err)
	oldKey := &model.UpstreamKey{
		ChannelID:        channel.Id,
		ExternalID:       "old-key",
		Name:             "old",
		SecretCiphertext: oldSecret,
		Models:           "gpt-4o",
		Status:           model.UpstreamKeyStatusEnabled,
	}
	require.NoError(t, db.Create(oldKey).Error)
	require.NoError(t, db.Create(&model.UpstreamKeyAbility{
		UpstreamKeyID: oldKey.ID,
		Group:         "default",
		Model:         "gpt-4o",
		Enabled:       true,
	}).Error)

	require.NoError(t, persistPlatformSiteSnapshot(context.Background(), account, PlatformSiteSnapshot{
		Balance: 7,
		Models:  []string{"gpt-4o"},
		Keys: []UpstreamKeySnapshot{
			{
				ExternalID: "old-key",
				Name:       "old",
				Models:     []string{"gpt-4o"},
				SyncError:  upstreamKeySyncErrorSecretUnavailable,
			},
			{
				ExternalID: "new-key",
				Name:       "new",
				Models:     []string{"gpt-4o"},
				SyncError:  upstreamKeySyncErrorSecretUnavailable,
			},
			{
				ExternalID:   "healthy-key",
				Name:         "healthy",
				Secret:       "sk-healthy",
				Group:        "default",
				Models:       []string{"gpt-4o"},
				ModelsSynced: true,
			},
		},
	}))

	var savedOld model.UpstreamKey
	require.NoError(t, db.Where("channel_id = ? AND external_id = ?", channel.Id, "old-key").First(&savedOld).Error)
	assert.Equal(t, model.UpstreamKeyStatusAutoDisabled, savedOld.Status)
	assert.Equal(t, upstreamKeySyncErrorSecretUnavailable, savedOld.DisabledReason)
	assert.Equal(t, "gpt-4o", savedOld.Models)
	oldCredential, err := model.DecryptPlatformSiteCredential(savedOld.SecretCiphertext)
	require.NoError(t, err)
	assert.Equal(t, "sk-old", oldCredential.AccessToken)

	var newKey model.UpstreamKey
	require.NoError(t, db.Where("channel_id = ? AND external_id = ?", channel.Id, "new-key").First(&newKey).Error)
	assert.Equal(t, model.UpstreamKeyStatusAutoDisabled, newKey.Status)
	assert.Equal(t, upstreamKeySyncErrorSecretUnavailable, newKey.DisabledReason)
	assert.False(t, newKey.ModelsSynced)
	assert.Empty(t, newKey.SecretCiphertext)

	var healthyKey model.UpstreamKey
	require.NoError(t, db.Where("channel_id = ? AND external_id = ?", channel.Id, "healthy-key").First(&healthyKey).Error)
	healthyCredential, err := model.DecryptPlatformSiteCredential(healthyKey.SecretCiphertext)
	require.NoError(t, err)
	assert.Equal(t, "sk-healthy", healthyCredential.AccessToken)

	var savedAccount model.PlatformSiteAccount
	require.NoError(t, db.Where("channel_id = ?", channel.Id).First(&savedAccount).Error)
	assert.Equal(t, 7.0, savedAccount.Balance)
}

func TestPersistPlatformSiteCredentialStoresOnlyEncryptedRotatedValues(t *testing.T) {
	previousDB := model.DB
	previousSecret := common.CryptoSecret
	common.CryptoSecret = "upstream-site-credential-update-test-secret"
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.PlatformSiteAccount{}))
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		common.CryptoSecret = previousSecret
		sqlDB, closeErr := db.DB()
		if closeErr == nil {
			_ = sqlDB.Close()
		}
	})

	account := &model.PlatformSiteAccount{
		ChannelID:            15,
		Platform:             model.PlatformSub2API,
		BaseURL:              "https://upstream.example",
		CredentialCiphertext: "old-ciphertext",
		CredentialKeyVersion: "v1",
	}
	require.NoError(t, db.Create(account).Error)
	credential := model.PlatformSiteCredential{
		AccessToken:    "rotated-access",
		RefreshToken:   "rotated-refresh",
		TokenExpiresAt: common.GetTimestamp() + 3600,
	}
	require.NoError(t, persistPlatformSiteCredential(account, credential))

	var saved model.PlatformSiteAccount
	require.NoError(t, db.First(&saved, account.ID).Error)
	assert.NotContains(t, saved.CredentialCiphertext, credential.AccessToken)
	assert.NotContains(t, saved.CredentialCiphertext, credential.RefreshToken)
	assert.NotEqual(t, "old-ciphertext", saved.CredentialCiphertext)
	decrypted, err := model.DecryptPlatformSiteCredential(saved.CredentialCiphertext)
	require.NoError(t, err)
	assert.Equal(t, credential, decrypted)
}

func TestPersistPlatformSiteSnapshotRebuildsAbilitiesAndAutoDisablesUnavailableKeys(t *testing.T) {
	previousDB := model.DB
	previousSecret := common.CryptoSecret
	common.CryptoSecret = "upstream-site-test-secret"
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Channel{},
		&model.Ability{},
		&model.PlatformSiteAccount{},
		&model.UpstreamKey{},
		&model.UpstreamKeyAbility{},
	))
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		common.CryptoSecret = previousSecret
		sqlDB, closeErr := db.DB()
		if closeErr == nil {
			_ = sqlDB.Close()
		}
	})

	priority := int64(4)
	channel := &model.Channel{
		Id:           11,
		Name:         "site",
		Status:       common.ChannelStatusEnabled,
		UpstreamKind: model.UpstreamKindPlatformSite,
		Group:        "default",
		Priority:     &priority,
	}
	require.NoError(t, db.Create(channel).Error)
	account := &model.PlatformSiteAccount{
		ChannelID:       channel.Id,
		Platform:        model.PlatformNewAPI,
		BaseURL:         "https://upstream.example",
		ConversionRatio: 0.1,
	}
	require.NoError(t, db.Create(account).Error)

	remaining := int64(0)
	require.NoError(t, persistPlatformSiteSnapshot(context.Background(), account, PlatformSiteSnapshot{
		Balance: 5,
		Models:  []string{"gpt-4o"},
		Keys: []UpstreamKeySnapshot{{
			ExternalID:   "key-1",
			Name:         "key",
			Secret:       "sk-test",
			Group:        "default",
			Models:       []string{"gpt-4o"},
			ModelsSynced: true,
			RemainQuota:  &remaining,
		}},
	}))

	var key model.UpstreamKey
	require.NoError(t, db.Where("channel_id = ?", channel.Id).First(&key).Error)
	assert.Equal(t, model.UpstreamKeyStatusAutoDisabled, key.Status)
	assert.Equal(t, "密钥剩余额度不足", key.DisabledReason)
	assert.Equal(t, 1900, key.Weight)

	var savedAccount model.PlatformSiteAccount
	require.NoError(t, db.Where("channel_id = ?", channel.Id).First(&savedAccount).Error)
	assert.Equal(t, 5.0, savedAccount.Balance)
	assert.Equal(t, int64(0), savedAccount.UsedQuota)

	var ability model.Ability
	assert.ErrorIs(t, db.Where(&model.Ability{
		ChannelId: channel.Id,
		Group:     "default",
		Model:     "gpt-4o",
	}).First(&ability).Error, gorm.ErrRecordNotFound)
	var keyAbility model.UpstreamKeyAbility
	require.NoError(t, db.Where(&model.UpstreamKeyAbility{
		UpstreamKeyID: key.ID,
		Group:         "",
		Model:         "gpt-4o",
	}).First(&keyAbility).Error)
	assert.True(t, keyAbility.Enabled)

	require.NoError(t, persistPlatformSiteSnapshot(context.Background(), account, PlatformSiteSnapshot{
		Balance: 5,
		Models:  []string{"gpt-4o"},
		Keys:    nil,
	}))

	remaining = 100
	require.NoError(t, db.Model(&model.UpstreamKey{}).
		Where("channel_id = ? AND external_id = ?", channel.Id, "key-1").
		Updates(map[string]any{
			"status":          model.UpstreamKeyStatusEnabled,
			"disabled_reason": "",
			"remain_quota":    remaining,
		}).Error)

	var missingKey model.UpstreamKey
	require.NoError(t, db.Where("channel_id = ? AND external_id = ?", channel.Id, "key-1").First(&missingKey).Error)
	require.NotZero(t, missingKey.MissingSince)
	assert.False(t, missingKey.IsRoutable(time.Now()))

	require.NoError(t, persistPlatformSiteSnapshot(context.Background(), account, PlatformSiteSnapshot{
		Balance: 5,
		Models:  []string{"gpt-4o"},
		Keys: []UpstreamKeySnapshot{{
			ExternalID:   "key-1",
			Name:         "key",
			Secret:       "sk-test",
			Group:        "default",
			Models:       []string{"gpt-4o"},
			ModelsSynced: true,
			RemainQuota:  &remaining,
		}},
	}))

	var restoredKey model.UpstreamKey
	require.NoError(t, db.Where("channel_id = ? AND external_id = ?", channel.Id, "key-1").First(&restoredKey).Error)
	assert.Equal(t, model.UpstreamKeyStatusEnabled, restoredKey.Status)
	assert.Zero(t, restoredKey.MissingSince)
	assert.True(t, restoredKey.IsRoutable(time.Now()))
}

func TestPersistPlatformSiteSnapshotPreservesManualRatioAndWeightOverrides(t *testing.T) {
	previousDB := model.DB
	previousSecret := common.CryptoSecret
	common.CryptoSecret = "upstream-site-override-test-secret"
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Channel{},
		&model.Ability{},
		&model.PlatformSiteAccount{},
		&model.UpstreamKey{},
		&model.UpstreamKeyAbility{},
	))
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		common.CryptoSecret = previousSecret
		sqlDB, closeErr := db.DB()
		if closeErr == nil {
			_ = sqlDB.Close()
		}
	})

	priority := int64(3)
	channel := &model.Channel{
		Id:           14,
		Name:         "override-site",
		Status:       common.ChannelStatusEnabled,
		UpstreamKind: model.UpstreamKindPlatformSite,
		Group:        "default",
		Priority:     &priority,
	}
	require.NoError(t, db.Create(channel).Error)
	account := &model.PlatformSiteAccount{
		ChannelID:       channel.Id,
		Platform:        model.PlatformNewAPI,
		BaseURL:         "https://upstream.example",
		ConversionRatio: 0.1,
	}
	require.NoError(t, db.Create(account).Error)

	ratioOverride := 0.42
	weightOverride := 1777
	secret, err := model.EncryptPlatformSiteCredential(model.PlatformSiteCredential{AccessToken: "sk-override"})
	require.NoError(t, err)
	key := &model.UpstreamKey{
		ChannelID:               channel.Id,
		ExternalID:              "override-key",
		SecretCiphertext:        secret,
		ConversionRatio:         ratioOverride,
		ConversionRatioOverride: &ratioOverride,
		Weight:                  1580,
		WeightOverride:          &weightOverride,
		Status:                  model.UpstreamKeyStatusEnabled,
	}
	require.NoError(t, db.Create(key).Error)

	sourceRatio := 0.7
	require.NoError(t, persistPlatformSiteSnapshot(context.Background(), account, PlatformSiteSnapshot{
		Balance: 9,
		Keys: []UpstreamKeySnapshot{{
			ExternalID:               "override-key",
			Name:                     "override",
			Secret:                   "sk-override-next",
			SourceConversionRatio:    sourceRatio,
			SourceConversionRatioSet: true,
			Models:                   []string{"gpt-4o"},
			ModelsSynced:             true,
		}},
	}))

	var saved model.UpstreamKey
	require.NoError(t, db.First(&saved, key.ID).Error)
	require.NotNil(t, saved.SourceConversionRatio)
	assert.Equal(t, sourceRatio, *saved.SourceConversionRatio)
	require.NotNil(t, saved.ConversionRatioOverride)
	assert.Equal(t, ratioOverride, *saved.ConversionRatioOverride)
	assert.Equal(t, ratioOverride, saved.ConversionRatio)
	require.NotNil(t, saved.WeightOverride)
	assert.Equal(t, weightOverride, *saved.WeightOverride)
	assert.Equal(t, weightOverride, saved.EffectiveWeight())
}

func TestPlatformSiteRequestRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte(strings.Repeat("x", upstreamSiteResponseLimit+1)))
	}))
	defer server.Close()

	session, err := newPlatformSiteSession(server.URL, nil)
	require.NoError(t, err)
	session.Client = server.Client()
	_, err = platformSiteRequest(context.Background(), session, http.MethodGet, "/", url.Values{}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "响应体超过限制")
}
