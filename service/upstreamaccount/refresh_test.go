package upstreamaccount

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/setting/operation_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const upstreamLargeUsedQuota = int64(33508580000)

func setupRefreshChannelTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelAccount{}))
	model.DB = db
	model.LOG_DB = db
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
	})
	return db
}

func createRefreshTestSyncedChannel(t *testing.T, db *gorm.DB, name string, settings string) model.Channel {
	t.Helper()

	channel := model.Channel{
		Type:          constant.ChannelTypeOpenAI,
		Key:           constant.ChannelCredentialModeAccountPool,
		Name:          name,
		Status:        common.ChannelStatusEnabled,
		Models:        "gpt-4o",
		Group:         "default",
		OtherSettings: settings,
		ChannelInfo: model.ChannelInfo{
			CredentialMode:     constant.ChannelCredentialModeAccountPool,
			AccountPoolEnabled: true,
			AccountPoolMode:    constant.ChannelAccountPoolModePolling,
		},
	}
	require.NoError(t, db.Create(&channel).Error)
	return channel
}

func TestSnapshotUSDToQuotaInt64(t *testing.T) {
	normalCost := 4.75
	smallCost := 1.2
	largeCost := 67017.16
	negativeCost := -1.5
	nanCost := math.NaN()
	hugeCost := math.MaxFloat64

	require.Equal(t, int64(0), snapshotUSDToQuotaInt64(nil))
	require.Equal(t, int64(common.QuotaPerUnit*normalCost), snapshotUSDToQuotaInt64(&normalCost))
	require.Equal(t, int64(common.QuotaPerUnit*smallCost), snapshotUSDToQuotaInt64(&smallCost))
	require.Equal(t, upstreamLargeUsedQuota, snapshotUSDToQuotaInt64(&largeCost))
	require.NotEqual(t, int64(common.MaxQuota), snapshotUSDToQuotaInt64(&largeCost))
	require.Equal(t, int64(0), snapshotUSDToQuotaInt64(&negativeCost))
	require.Equal(t, int64(0), snapshotUSDToQuotaInt64(&nanCost))
	require.Equal(t, int64(math.MaxInt64), snapshotUSDToQuotaInt64(&hugeCost))
}

func TestRefreshChannelBalanceUsesStoredCredential(t *testing.T) {
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelAccount{}))
	model.DB = db
	model.LOG_DB = db
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/api/v1/") && r.URL.Path != "/api/v1/auth/login" {
			require.Equal(t, "Bearer sub2-token", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/api/v1/auth/login":
			var body map[string]string
			require.NoError(t, common.DecodeJson(r.Body, &body))
			require.Equal(t, "alice@example.com", body["email"])
			require.Equal(t, "secret", body["password"])
			_, _ = w.Write([]byte(`{"code":0,"data":{"access_token":"sub2-token","user":{"id":5,"email":"alice@example.com","balance":1}}}`))
		case "/api/v1/auth/me":
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":5,"email":"alice@example.com","balance":11}}`))
		case "/api/v1/user/profile":
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":5,"email":"alice@example.com","balance":12.5}}`))
		case "/api/v1/groups/available":
			_, _ = w.Write([]byte(`{"code":0,"data":[{"id":3,"name":"vip","platform":"openai","rate_multiplier":0.25}]}`))
		case "/api/v1/groups/rates":
			_, _ = w.Write([]byte(`{"code":0,"data":{"3":0.25}}`))
		case "/api/v1/usage/dashboard/stats":
			_, _ = w.Write([]byte(`{"code":0,"data":{"total_actual_cost":67017.16}}`))
		case "/api/v1/keys":
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"id":9,"name":"sub-key","key":"sk-sub2-full-key","status":"active","group_id":3,"group":{"id":3,"name":"vip"},"models":["gpt-4o"],"quota":20,"quota_used":3}],"total":1}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	settings := mergeChannelSyncMetadataWithCredential("", &Snapshot{
		Platform: PlatformSub2API,
		BaseURL:  server.URL,
	}, Credential{
		Platform: PlatformSub2API,
		BaseURL:  server.URL,
		Email:    "alice@example.com",
		Password: "secret",
	})
	channel := model.Channel{
		Type:               constant.ChannelTypeOpenAI,
		Key:                constant.ChannelCredentialModeAccountPool,
		Name:               "synced-sub2-channel",
		Status:             common.ChannelStatusEnabled,
		Models:             "gpt-4o",
		Group:              "default",
		Balance:            1,
		BalanceUpdatedTime: 1,
		UsedQuota:          4567,
		OtherSettings:      settings,
		ChannelInfo: model.ChannelInfo{
			CredentialMode:     constant.ChannelCredentialModeAccountPool,
			AccountPoolEnabled: true,
			AccountPoolMode:    constant.ChannelAccountPoolModePolling,
		},
	}
	require.NoError(t, db.Create(&channel).Error)

	result, err := RefreshChannelBalance(context.Background(), &channel)
	require.NoError(t, err)
	require.Equal(t, 12.5, result.Balance)
	require.Equal(t, int64(4567), result.UsedQuota)
	require.Greater(t, result.BalanceUpdatedTime, int64(1))

	var refreshed model.Channel
	require.NoError(t, db.First(&refreshed, channel.Id).Error)
	require.Equal(t, 12.5, refreshed.Balance)
	require.Equal(t, int64(4567), refreshed.UsedQuota)
	require.Greater(t, refreshed.BalanceUpdatedTime, int64(1))
	credential, ok, err := ReadChannelSyncCredential(refreshed.OtherSettings)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "secret", credential.Password)
}

func TestRefreshChannelBalanceUsesPasswordBeforeSavedSession(t *testing.T) {
	db := setupRefreshChannelTestDB(t)
	var loginCount atomic.Int32
	var authMeCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/auth/login":
			loginCount.Add(1)
			var body map[string]string
			require.NoError(t, common.DecodeJson(r.Body, &body))
			require.Equal(t, "alice@example.com", body["email"])
			require.Equal(t, "secret", body["password"])
			_, _ = w.Write([]byte(`{"code":0,"data":{"access_token":"password-token","user":{"id":5,"email":"alice@example.com","balance":1}}}`))
		case "/api/v1/auth/me":
			authMeCount.Add(1)
			require.Equal(t, "Bearer password-token", r.Header.Get("Authorization"))
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":5,"email":"alice@example.com","balance":10}}`))
		case "/api/v1/user/profile":
			require.Equal(t, "Bearer password-token", r.Header.Get("Authorization"))
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":5,"email":"alice@example.com","balance":12.5}}`))
		case "/api/v1/groups/available":
			require.Equal(t, "Bearer password-token", r.Header.Get("Authorization"))
			_, _ = w.Write([]byte(`{"code":0,"data":[{"id":3,"name":"vip","platform":"openai","rate_multiplier":0.25}]}`))
		case "/api/v1/groups/rates":
			require.Equal(t, "Bearer password-token", r.Header.Get("Authorization"))
			_, _ = w.Write([]byte(`{"code":0,"data":{"3":0.25}}`))
		case "/api/v1/usage/dashboard/stats":
			require.Equal(t, "Bearer password-token", r.Header.Get("Authorization"))
			_, _ = w.Write([]byte(`{"code":0,"data":{"total_actual_cost":2}}`))
		case "/api/v1/keys":
			require.Equal(t, "Bearer password-token", r.Header.Get("Authorization"))
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"id":9,"name":"sub-key","key":"sk-sub2-full-key","status":"active","group_id":3,"group":{"id":3,"name":"vip"},"models":["gpt-4o"],"quota":20,"quota_used":3}],"total":1}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	settings := mergeChannelSyncMetadataWithCredential(
		"",
		&Snapshot{
			Platform: PlatformSub2API,
			BaseURL:  server.URL,
			AuthSession: &AuthenticatedSession{
				Platform: PlatformSub2API,
				BaseURL:  server.URL,
				AuthMode: AuthModePassword,
				Sub2API: &Sub2APISessionData{
					AccessToken:  "saved-session-token",
					RefreshToken: "saved-refresh-token",
					ExpiresAt:    common.GetTimestamp() + 3600,
				},
			},
		},
		Credential{
			Platform: PlatformSub2API,
			BaseURL:  server.URL,
			Email:    "alice@example.com",
			AuthMode: AuthModePassword,
			Password: "secret",
		},
	)
	channel := createRefreshTestSyncedChannel(t, db, "balance-password-priority", settings)

	result, err := RefreshChannelBalance(context.Background(), &channel)
	require.NoError(t, err)
	require.Equal(t, 12.5, result.Balance)
	require.EqualValues(t, 1, loginCount.Load())
	require.EqualValues(t, 1, authMeCount.Load())
}

func TestRefreshChannelFromCredentialConsumesPreviewSnapshot(t *testing.T) {
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelAccount{}))
	model.DB = db
	model.LOG_DB = db
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
	})

	channel := model.Channel{
		Type:      constant.ChannelTypeOpenAI,
		Key:       constant.ChannelCredentialModeAccountPool,
		Name:      "synced-channel",
		Status:    common.ChannelStatusEnabled,
		Models:    "gpt-old",
		Group:     "default",
		UsedQuota: 1234,
		ChannelInfo: model.ChannelInfo{
			CredentialMode:     constant.ChannelCredentialModeAccountPool,
			AccountPoolEnabled: true,
			AccountPoolMode:    constant.ChannelAccountPoolModePolling,
		},
	}
	require.NoError(t, db.Create(&channel).Error)

	preview, err := SavePreviewSnapshot(&Snapshot{
		Platform: PlatformNewAPI,
		BaseURL:  "https://newapi.example",
		Balance:  &BalanceSnapshot{BalanceUSD: floatPtr(5), UsedUSD: floatPtr(1)},
		Keys: []SyncedKey{{
			ExternalID:        "new",
			Name:              "New Key",
			Key:               "sk-new",
			MaskedKey:         "sk-new",
			GroupName:         "default",
			Models:            []string{"gpt-4o"},
			SuggestedPriority: 1,
			SuggestedWeight:   100,
		}},
	})
	require.NoError(t, err)

	result, err := RefreshChannelFromCredential(context.Background(), RefreshRequest{
		ChannelID:         channel.Id,
		PreviewID:         preview.PreviewID,
		ApplySuggested:    true,
		DisableMissingKey: true,
	})
	require.NoError(t, err)
	require.Equal(t, 1, result.Created)
	require.Equal(t, 0, result.Updated)

	var accounts []model.ChannelAccount
	require.NoError(t, db.Find(&accounts).Error)
	require.Len(t, accounts, 1)
	require.Equal(t, "sk-new", accounts[0].Key)

	_, err = GetPreviewRecord(preview.PreviewID)
	require.Error(t, err)
}

func TestRefreshChannelFromCredentialUsesPasswordBeforeSavedSub2APISession(t *testing.T) {
	db := setupRefreshChannelTestDB(t)
	var loginCount atomic.Int32
	var refreshCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/api/v1/") && r.URL.Path != "/api/v1/auth/login" {
			require.Equal(t, "Bearer password-token", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/api/v1/auth/login":
			loginCount.Add(1)
			var body map[string]string
			require.NoError(t, common.DecodeJson(r.Body, &body))
			require.Equal(t, "alice@example.com", body["email"])
			require.Equal(t, "secret", body["password"])
			_, _ = w.Write([]byte(`{"code":0,"data":{"access_token":"password-token","refresh_token":"password-refresh","expires_in":3600,"user":{"id":5,"email":"alice@example.com","balance":1}}}`))
		case "/api/v1/auth/refresh":
			refreshCount.Add(1)
			http.Error(w, "refresh token should not be used when password is saved", http.StatusInternalServerError)
		case "/api/v1/auth/me":
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":5,"email":"alice@example.com","balance":10}}`))
		case "/api/v1/user/profile":
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":5,"email":"alice@example.com","balance":12.5}}`))
		case "/api/v1/groups/available":
			_, _ = w.Write([]byte(`{"code":0,"data":[{"id":3,"name":"vip","platform":"openai","rate_multiplier":0.25}]}`))
		case "/api/v1/groups/rates":
			_, _ = w.Write([]byte(`{"code":0,"data":{"3":0.25}}`))
		case "/api/v1/usage/dashboard/stats":
			_, _ = w.Write([]byte(`{"code":0,"data":{"total_actual_cost":1}}`))
		case "/api/v1/keys":
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"id":9,"name":"sub-key","key":"sk-sub2-password-key","status":"active","group_id":3,"group":{"id":3,"name":"vip"},"models":["gpt-4o"],"quota":20,"quota_used":3}],"total":1}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	settings := mergeChannelSyncMetadataWithCredential(
		"",
		&Snapshot{
			Platform: PlatformSub2API,
			BaseURL:  server.URL,
			AuthSession: &AuthenticatedSession{
				Platform: PlatformSub2API,
				BaseURL:  server.URL,
				AuthMode: AuthModePassword,
				Sub2API: &Sub2APISessionData{
					AccessToken:  "saved-session-token",
					RefreshToken: "saved-refresh-token",
					ExpiresAt:    common.GetTimestamp() + 3600,
				},
			},
		},
		Credential{
			Platform: PlatformSub2API,
			BaseURL:  server.URL,
			Email:    "alice@example.com",
			AuthMode: AuthModePassword,
			Password: "secret",
		},
	)
	channel := createRefreshTestSyncedChannel(t, db, "sub2-password-priority", settings)

	result, err := RefreshChannelFromCredential(context.Background(), RefreshRequest{ChannelID: channel.Id})
	require.NoError(t, err)
	require.Equal(t, 1, result.Created)
	require.EqualValues(t, 1, loginCount.Load())
	require.EqualValues(t, 0, refreshCount.Load())

	var refreshed model.Channel
	require.NoError(t, db.First(&refreshed, channel.Id).Error)
	credential, ok, err := ReadChannelSyncCredential(refreshed.OtherSettings)
	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, credential.Session)
	require.Equal(t, "password-token", credential.Session.Sub2API.AccessToken)
	require.Equal(t, "secret", credential.Password)
}

func TestRefreshChannelFromCredentialUsesPasswordBeforeSavedNewAPISession(t *testing.T) {
	db := setupRefreshChannelTestDB(t)
	var loginCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		require.NotEqual(t, "Bearer saved-newapi-token", r.Header.Get("Authorization"))
		switch r.URL.Path {
		case "/api/status":
			_, _ = w.Write([]byte(`{"success":true,"data":{"quota_per_unit":500000}}`))
		case "/api/user/login":
			loginCount.Add(1)
			var body map[string]string
			require.NoError(t, common.DecodeJson(r.Body, &body))
			require.Equal(t, "alice", body["username"])
			require.Equal(t, "secret", body["password"])
			_, _ = w.Write([]byte(`{"success":true,"data":{"id":7,"username":"alice","group":"default","quota":1000000,"used_quota":500000}}`))
		case "/api/user/self":
			require.Equal(t, "7", r.Header.Get("New-Api-User"))
			_, _ = w.Write([]byte(`{"success":true,"data":{"id":7,"username":"alice","group":"default","quota":1000000,"used_quota":500000}}`))
		case "/api/user/self/groups":
			_, _ = w.Write([]byte(`{"success":true,"data":{"default":{"ratio":1,"desc":"default"}}}`))
		case "/api/ratio_config":
			_, _ = w.Write([]byte(`{"success":true,"data":{"model_ratio":{},"completion_ratio":{},"cache_ratio":{},"create_cache_ratio":{},"model_price":{}}}`))
		case "/api/token/":
			_, _ = w.Write([]byte(`{"success":true,"data":{"items":[{"id":11,"name":"new-key","key":"sk-newapi-password-key","group":"default","status":1,"model_limits":"gpt-4o","remain_quota":1000000,"used_quota":0}],"total":1}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	settings := mergeChannelSyncMetadataWithCredential(
		"",
		&Snapshot{
			Platform: PlatformNewAPI,
			BaseURL:  server.URL,
			AuthSession: &AuthenticatedSession{
				Platform: PlatformNewAPI,
				BaseURL:  server.URL,
				AuthMode: AuthModePassword,
				NewAPI: &NewAPISessionData{
					UserID:      "7",
					AccessToken: "saved-newapi-token",
				},
			},
		},
		Credential{
			Platform: PlatformNewAPI,
			BaseURL:  server.URL,
			Username: "alice",
			AuthMode: AuthModePassword,
			Password: "secret",
		},
	)
	channel := createRefreshTestSyncedChannel(t, db, "newapi-password-priority", settings)

	result, err := RefreshChannelFromCredential(context.Background(), RefreshRequest{ChannelID: channel.Id})
	require.NoError(t, err)
	require.Equal(t, 1, result.Created)
	require.EqualValues(t, 1, loginCount.Load())
}

func TestRefreshChannelFromCredentialKeepsAccessTokenPathWithoutPassword(t *testing.T) {
	db := setupRefreshChannelTestDB(t)
	var loginCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/api/v1/") && r.URL.Path != "/api/v1/auth/login" {
			require.Equal(t, "Bearer imported-token", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/api/v1/auth/login":
			loginCount.Add(1)
			http.Error(w, "password login should not be used without a password", http.StatusInternalServerError)
		case "/api/v1/auth/me":
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":5,"email":"alice@example.com","balance":10}}`))
		case "/api/v1/user/profile":
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":5,"email":"alice@example.com","balance":12.5}}`))
		case "/api/v1/groups/available":
			_, _ = w.Write([]byte(`{"code":0,"data":[{"id":3,"name":"vip","platform":"openai","rate_multiplier":0.25}]}`))
		case "/api/v1/groups/rates":
			_, _ = w.Write([]byte(`{"code":0,"data":{"3":0.25}}`))
		case "/api/v1/usage/dashboard/stats":
			_, _ = w.Write([]byte(`{"code":0,"data":{"total_actual_cost":1}}`))
		case "/api/v1/keys":
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"id":9,"name":"sub-key","key":"sk-sub2-token-key","status":"active","group_id":3,"group":{"id":3,"name":"vip"},"models":["gpt-4o"],"quota":20,"quota_used":3}],"total":1}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	channel := createRefreshTestSyncedChannel(t, db, "sub2-token-refresh", "")
	result, err := RefreshChannelFromCredential(context.Background(), RefreshRequest{
		ChannelID: channel.Id,
		Credential: Credential{
			Platform:    PlatformSub2API,
			BaseURL:     server.URL,
			AuthMode:    AuthModeAccessToken,
			AccessToken: "imported-token",
		},
	})

	require.NoError(t, err)
	require.Equal(t, 1, result.Created)
	require.EqualValues(t, 0, loginCount.Load())
}

func TestRefreshChannelFromCredentialPasswordFailureDoesNotFallbackToSavedToken(t *testing.T) {
	db := setupRefreshChannelTestDB(t)
	var authMeCount atomic.Int32
	var refreshCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/auth/login":
			_, _ = w.Write([]byte(`{"code":1,"message":"password=secret access_token=saved-session-token sk-secret-key failed"}`))
		case "/api/v1/auth/me":
			authMeCount.Add(1)
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":5,"email":"alice@example.com","balance":10}}`))
		case "/api/v1/auth/refresh":
			refreshCount.Add(1)
			_, _ = w.Write([]byte(`{"code":0,"data":{"access_token":"refreshed-token","expires_in":3600}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	settings := mergeChannelSyncMetadataWithCredential(
		"",
		&Snapshot{
			Platform: PlatformSub2API,
			BaseURL:  server.URL,
			AuthSession: &AuthenticatedSession{
				Platform: PlatformSub2API,
				BaseURL:  server.URL,
				AuthMode: AuthModePassword,
				Sub2API: &Sub2APISessionData{
					AccessToken:  "saved-session-token",
					RefreshToken: "saved-refresh-token",
					ExpiresAt:    common.GetTimestamp() + 3600,
				},
			},
		},
		Credential{
			Platform: PlatformSub2API,
			BaseURL:  server.URL,
			Email:    "alice@example.com",
			AuthMode: AuthModePassword,
			Password: "secret",
		},
	)
	channel := createRefreshTestSyncedChannel(t, db, "sub2-password-failure", settings)

	result, err := RefreshChannelFromCredential(context.Background(), RefreshRequest{ChannelID: channel.Id})
	require.ErrorContains(t, err, "password=secret")
	require.Nil(t, result)
	require.EqualValues(t, 0, authMeCount.Load())
	require.EqualValues(t, 0, refreshCount.Load())
}

func TestRefreshChannelFromSnapshotUpdatesChannelBaseURLWhenSyncSourceMigrates(t *testing.T) {
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelAccount{}))
	model.DB = db
	model.LOG_DB = db
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
	})

	panelBaseURL := "https://aiapipay.com"
	apiBaseURL := "https://api.aiapipay.com"
	channel := model.Channel{
		Type:          constant.ChannelTypeSub2API,
		Key:           constant.ChannelCredentialModeAccountPool,
		Name:          "migrated-sub2-channel",
		Status:        common.ChannelStatusEnabled,
		BaseURL:       &panelBaseURL,
		OtherSettings: mergeChannelSyncMetadata("", &Snapshot{Platform: PlatformSub2API, BaseURL: panelBaseURL}),
		ChannelInfo: model.ChannelInfo{
			CredentialMode:     constant.ChannelCredentialModeAccountPool,
			AccountPoolEnabled: true,
			AccountPoolMode:    constant.ChannelAccountPoolModePolling,
		},
	}
	require.NoError(t, db.Create(&channel).Error)

	_, err = RefreshChannelFromSnapshot(channel.Id, &Snapshot{
		Platform:          PlatformSub2API,
		BaseURL:           apiBaseURL,
		ManagementBaseURL: panelBaseURL,
		RelayBaseURL:      apiBaseURL,
		Balance:           &BalanceSnapshot{BalanceUSD: floatPtr(2.5)},
		Keys: []SyncedKey{{
			ExternalID: "key-1",
			Name:       "api-key",
			Key:        "sk-sub2-full-key",
			Status:     common.ChannelStatusEnabled,
			Models:     []string{"gpt-4o"},
			GroupName:  "default",
		}},
	}, RefreshRequest{ChannelID: channel.Id})
	require.NoError(t, err)

	var refreshed model.Channel
	require.NoError(t, db.First(&refreshed, channel.Id).Error)
	require.NotNil(t, refreshed.BaseURL)
	require.Equal(t, apiBaseURL, *refreshed.BaseURL)
	metadata := readChannelSyncMetadata(refreshed.OtherSettings)
	require.Equal(t, panelBaseURL, metadata.BaseURL)
	require.Equal(t, panelBaseURL, metadata.ManagementBaseURL)
	require.Equal(t, apiBaseURL, metadata.RelayBaseURL)
}

func TestRefreshChannelFromSnapshotUpsertsAccountsAndDisablesMissing(t *testing.T) {
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelAccount{}))
	model.DB = db
	model.LOG_DB = db
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
	})

	channel := model.Channel{
		Type:      constant.ChannelTypeOpenAI,
		Key:       constant.ChannelCredentialModeAccountPool,
		Name:      "synced-channel",
		Status:    common.ChannelStatusEnabled,
		Models:    "gpt-old",
		Group:     "default",
		UsedQuota: 1234,
		ChannelInfo: model.ChannelInfo{
			CredentialMode:     constant.ChannelCredentialModeAccountPool,
			AccountPoolEnabled: true,
			AccountPoolMode:    constant.ChannelAccountPoolModePolling,
		},
	}
	require.NoError(t, db.Create(&channel).Error)

	oldKey := SyncedKey{
		ExternalID: "old",
		Name:       "Old Key",
		Key:        "sk-old",
		MaskedKey:  "sk-old",
		GroupName:  "default",
		Models:     []string{"gpt-old"},
	}
	existingSettings := mergeAccountSyncMetadata("", &Snapshot{
		Platform: PlatformNewAPI,
		BaseURL:  "https://newapi.example",
	}, oldKey)
	existing := model.ChannelAccount{
		ChannelId:     channel.Id,
		Name:          "Old Key",
		Key:           "sk-old",
		Status:        common.ChannelStatusEnabled,
		Models:        "gpt-old",
		Group:         "default",
		AccessGroups:  "default",
		OtherSettings: existingSettings,
	}
	missingKey := SyncedKey{
		ExternalID: "missing",
		Name:       "Missing Key",
		Key:        "sk-missing",
		MaskedKey:  "sk-missing",
		GroupName:  "default",
	}
	missing := model.ChannelAccount{
		ChannelId:    channel.Id,
		Name:         "Missing Key",
		Key:          "sk-missing",
		Status:       common.ChannelStatusEnabled,
		Models:       "gpt-old",
		Group:        "default",
		AccessGroups: "default",
		OtherSettings: mergeAccountSyncMetadata("", &Snapshot{
			Platform: PlatformNewAPI,
			BaseURL:  "https://newapi.example",
		}, missingKey),
	}
	require.NoError(t, db.Create(&existing).Error)
	require.NoError(t, db.Create(&missing).Error)

	snapshot := &Snapshot{
		Platform: PlatformNewAPI,
		BaseURL:  "https://newapi.example",
		Balance: &BalanceSnapshot{
			BalanceUSD: floatPtr(8),
			UsedUSD:    floatPtr(67017.16),
		},
		Keys: []SyncedKey{
			{
				ExternalID:        "old",
				Name:              "Old Key Renamed",
				Key:               "sk-old-rotated",
				MaskedKey:         "sk-old-rotated",
				GroupName:         "vip",
				Models:            []string{"gpt-4o"},
				QuotaUsedUSD:      floatPtr(67017.16),
				SuggestedPriority: 3,
				SuggestedWeight:   90,
			},
			{
				ExternalID:        "new",
				Name:              "New Key",
				Key:               "sk-new",
				MaskedKey:         "sk-new",
				GroupName:         "vip",
				Models:            []string{"gpt-4o-mini"},
				SuggestedPriority: 2,
				SuggestedWeight:   80,
			},
		},
	}
	result, err := RefreshChannelFromSnapshot(channel.Id, snapshot, RefreshRequest{
		ChannelID:         channel.Id,
		ApplySuggested:    true,
		DisableMissingKey: true,
	})

	require.NoError(t, err)
	require.Equal(t, 1, result.Created)
	require.Equal(t, 1, result.Updated)
	require.Equal(t, 1, result.Disabled)

	var refreshed model.Channel
	require.NoError(t, db.First(&refreshed, channel.Id).Error)
	require.Equal(t, constant.ChannelTypeNewAPI, refreshed.Type)
	require.Equal(t, "gpt-old,gpt-4o-mini", refreshed.Models)
	require.Equal(t, "default", refreshed.Group)
	require.Equal(t, float64(8), refreshed.Balance)
	require.Equal(t, int64(1234), refreshed.UsedQuota)

	var accounts []model.ChannelAccount
	require.NoError(t, db.Order("id ASC").Find(&accounts).Error)
	require.Len(t, accounts, 3)
	require.Equal(t, "Old Key Renamed", accounts[0].Name)
	require.Equal(t, "sk-old-rotated", accounts[0].Key)
	require.Equal(t, "default", accounts[0].Group)
	require.Equal(t, int64(0), accounts[0].Priority)
	require.Equal(t, 100, accounts[0].Weight)
	require.Equal(t, upstreamLargeUsedQuota, accounts[0].UsedQuota)
	require.NotEqual(t, int64(common.MaxQuota), accounts[0].UsedQuota)
	require.Equal(t, common.ChannelStatusManuallyDisabled, accounts[1].Status)
	require.Equal(t, "New Key", accounts[2].Name)
	require.Equal(t, "sk-new", accounts[2].Key)
	require.Equal(t, "vip", accounts[2].Group)

	var abilityCount int64
	require.NoError(t, db.Model(&model.Ability{}).Count(&abilityCount).Error)
	require.Equal(t, int64(2), abilityCount)
}

func TestRefreshChannelFromSnapshotSummarizesEnabledAccessGroups(t *testing.T) {
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelAccount{}))
	model.DB = db
	model.LOG_DB = db
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
	})

	channel := model.Channel{
		Type:   constant.ChannelTypeOpenAI,
		Key:    constant.ChannelCredentialModeAccountPool,
		Name:   "synced-channel",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-old,gpt-disabled",
		Group:  "old-group,disabled-group",
		ChannelInfo: model.ChannelInfo{
			CredentialMode:     constant.ChannelCredentialModeAccountPool,
			AccountPoolEnabled: true,
			AccountPoolMode:    constant.ChannelAccountPoolModePolling,
		},
	}
	require.NoError(t, db.Create(&channel).Error)

	snapshot := &Snapshot{
		Platform: PlatformNewAPI,
		BaseURL:  "https://newapi.example",
		Keys: []SyncedKey{
			{
				ExternalID:        "enabled",
				Name:              "Enabled Key",
				Key:               "sk-enabled",
				MaskedKey:         "sk-enabled",
				GroupName:         "enabled-group",
				Models:            []string{"gpt-enabled"},
				SuggestedPriority: 1,
				SuggestedWeight:   100,
			},
			{
				ExternalID:        "disabled",
				Name:              "Disabled Key",
				Key:               "sk-disabled",
				MaskedKey:         "sk-disabled",
				GroupName:         "disabled-group",
				Models:            []string{"gpt-disabled"},
				SuggestedPriority: 1,
				SuggestedWeight:   100,
			},
		},
	}

	result, err := RefreshChannelFromSnapshot(channel.Id, snapshot, RefreshRequest{
		ChannelID:      channel.Id,
		ApplySuggested: true,
		Accounts: []AccountCreateConfig{
			{ExternalID: "disabled", Enabled: boolPtr(false)},
		},
	})

	require.NoError(t, err)
	require.Equal(t, 1, result.Created)
	require.Equal(t, 0, result.Updated)

	var refreshed model.Channel
	require.NoError(t, db.First(&refreshed, channel.Id).Error)
	require.Equal(t, "gpt-enabled", refreshed.Models)
	require.Equal(t, "default", refreshed.Group)

	var abilities []model.Ability
	require.NoError(t, db.Find(&abilities).Error)
	sort.Slice(abilities, func(i, j int) bool {
		if abilities[i].Group == abilities[j].Group {
			return abilities[i].Model < abilities[j].Model
		}
		return abilities[i].Group < abilities[j].Group
	})
	require.Len(t, abilities, 1)
	require.Equal(t, "gpt-enabled", abilities[0].Model)
	require.Equal(t, "default", abilities[0].Group)
}

func TestRefreshChannelFromSnapshotPreservesLocalAccountOverrides(t *testing.T) {
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelAccount{}))
	model.DB = db
	model.LOG_DB = db
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
	})

	channel := model.Channel{
		Type:   constant.ChannelTypeOpenAI,
		Key:    constant.ChannelCredentialModeAccountPool,
		Name:   "synced-channel",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-old",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			CredentialMode:     constant.ChannelCredentialModeAccountPool,
			AccountPoolEnabled: true,
			AccountPoolMode:    constant.ChannelAccountPoolModePolling,
		},
	}
	require.NoError(t, db.Create(&channel).Error)

	oldKey := SyncedKey{
		ExternalID: "local",
		Name:       "Local Key",
		Key:        "sk-old-local",
		MaskedKey:  "sk-old-local",
		GroupName:  "default",
		Models:     []string{"gpt-old"},
	}
	localBaseURL := "https://local-overridden.example/v1"
	localOrg := "org-local"
	localSetting := `{"timeout":30}`
	localModelMapping := `{"gpt-local":"gpt-upstream"}`
	localParamOverride := `{"temperature":0}`
	localHeaderOverride := `{"X-Local":"yes"}`
	localStatusCodeMapping := `{"401":"channel_invalid_key"}`
	existing := model.ChannelAccount{
		ChannelId:          channel.Id,
		Name:               "Local Key",
		Key:                "sk-old-local",
		Status:             common.ChannelStatusEnabled,
		Models:             "gpt-old",
		Group:              "default",
		AccessGroups:       "default",
		Priority:           11,
		Weight:             22,
		UsedQuota:          3,
		BaseURL:            &localBaseURL,
		OpenAIOrganization: &localOrg,
		Other:              "local-other",
		Setting:            &localSetting,
		OtherSettings: mergeAccountSyncMetadata(`{"local_flag":true}`, &Snapshot{
			Platform: PlatformNewAPI,
			BaseURL:  "https://newapi.example",
		}, oldKey),
		ModelMapping:      &localModelMapping,
		ParamOverride:     &localParamOverride,
		HeaderOverride:    &localHeaderOverride,
		StatusCodeMapping: &localStatusCodeMapping,
		MaxConcurrency:    7,
	}
	require.NoError(t, db.Create(&existing).Error)

	snapshot := &Snapshot{
		Platform: PlatformNewAPI,
		BaseURL:  "https://newapi.example",
		Balance: &BalanceSnapshot{
			BalanceUSD: floatPtr(5),
			UsedUSD:    floatPtr(2),
		},
		Keys: []SyncedKey{
			{
				ExternalID:        "local",
				Name:              "Local Key Renamed",
				Key:               "sk-new-local",
				MaskedKey:         "sk-new-local",
				GroupName:         "vip",
				Models:            []string{"gpt-4o"},
				SuggestedPriority: 8,
				SuggestedWeight:   66,
			},
		},
	}
	result, err := RefreshChannelFromSnapshot(channel.Id, snapshot, RefreshRequest{
		ChannelID:      channel.Id,
		ApplySuggested: true,
	})

	require.NoError(t, err)
	require.Equal(t, 0, result.Created)
	require.Equal(t, 1, result.Updated)
	require.Equal(t, 0, result.Disabled)

	var refreshed model.ChannelAccount
	require.NoError(t, db.First(&refreshed, existing.Id).Error)
	require.Equal(t, "Local Key Renamed", refreshed.Name)
	require.Equal(t, "sk-new-local", refreshed.Key)
	require.Equal(t, "gpt-old", refreshed.Models)
	require.Equal(t, "default", refreshed.Group)
	require.Equal(t, int64(11), refreshed.Priority)
	require.Equal(t, 100, refreshed.Weight)
	require.Equal(t, localBaseURL, *refreshed.BaseURL)
	require.Equal(t, localOrg, *refreshed.OpenAIOrganization)
	require.Equal(t, "local-other", refreshed.Other)
	require.Equal(t, localSetting, *refreshed.Setting)
	require.Equal(t, localModelMapping, *refreshed.ModelMapping)
	require.Equal(t, localParamOverride, *refreshed.ParamOverride)
	require.Equal(t, localHeaderOverride, *refreshed.HeaderOverride)
	require.Equal(t, localStatusCodeMapping, *refreshed.StatusCodeMapping)
	require.Equal(t, 7, refreshed.MaxConcurrency)

	var settings map[string]any
	require.NoError(t, common.UnmarshalJsonStr(refreshed.OtherSettings, &settings))
	require.Equal(t, true, settings["local_flag"])
	metadata := readAccountSyncMetadata(refreshed.OtherSettings)
	require.Equal(t, PlatformNewAPI, metadata.Platform)
	require.Equal(t, "https://newapi.example", metadata.BaseURL)
	require.Equal(t, "local", metadata.ExternalID)
	require.Equal(t, keyDigest("sk-new-local"), metadata.KeyDigest)
}

func TestRefreshChannelFromSnapshotPreservesExistingRatioConversionConfig(t *testing.T) {
	db := setupRefreshChannelTestDB(t)
	channel := createRefreshTestSyncedChannel(t, db, "ratio-preserve-channel", "")

	oldKey := SyncedKey{
		ExternalID: "ratio-key",
		Name:       "Ratio Key",
		Key:        "sk-ratio-old",
		MaskedKey:  "sk-ratio-old",
		GroupName:  "default",
		Models:     []string{"gpt-4o"},
		GroupRatio: floatPtr(10),
	}
	existingSnapshot := &Snapshot{
		Platform: PlatformNewAPI,
		BaseURL:  "https://newapi.example",
		Keys:     []SyncedKey{oldKey},
	}
	ApplyRatioConversion(existingSnapshot, RatioConversionConfig{
		PaidCNY:           1,
		PlatformUSDCredit: 1,
	})
	existing := model.ChannelAccount{
		ChannelId:     channel.Id,
		Name:          "Ratio Key",
		Key:           "sk-ratio-old",
		Status:        common.ChannelStatusEnabled,
		Models:        "gpt-4o",
		Group:         "default",
		AccessGroups:  "default",
		OtherSettings: mergeAccountSyncMetadata("", existingSnapshot, existingSnapshot.Keys[0]),
	}
	require.NoError(t, db.Create(&existing).Error)

	result, err := RefreshChannelFromSnapshot(channel.Id, &Snapshot{
		Platform: PlatformNewAPI,
		BaseURL:  "https://newapi.example",
		Keys: []SyncedKey{{
			ExternalID:        "ratio-key",
			Name:              "Ratio Key Refreshed",
			Key:               "sk-ratio-new",
			MaskedKey:         "sk-ratio-new",
			GroupName:         "default",
			Models:            []string{"gpt-4o"},
			GroupRatio:        floatPtr(10),
			SuggestedPriority: 1,
			SuggestedWeight:   100,
		}},
	}, RefreshRequest{ChannelID: channel.Id})

	require.NoError(t, err)
	require.Equal(t, 1, result.Updated)

	var refreshed model.ChannelAccount
	require.NoError(t, db.First(&refreshed, existing.Id).Error)
	metadata := ReadAccountSyncDisplayMetadata(refreshed.OtherSettings)
	require.NotNil(t, metadata.RatioConversionConfig)
	require.True(t, metadata.RatioConversionConfig.Enabled)
	require.Equal(t, float64(1), metadata.RatioConversionConfig.PaidCNY)
	require.Equal(t, float64(1), metadata.RatioConversionConfig.PlatformUSDCredit)
	require.InDelta(t, 10, metadata.RatioConversion, 0.000001)
}

func TestRefreshChannelFromSnapshotPreservesChannelRatioConversionConfig(t *testing.T) {
	db := setupRefreshChannelTestDB(t)
	channel := createRefreshTestSyncedChannel(
		t,
		db,
		"channel-ratio-preserve-channel",
		`{"upstream_account_sync":{"platform":"new-api","base_url":"https://newapi.example","ratio_conversion_config":{"paid_cny":1,"platform_usd_credit":10,"enabled":true}}}`,
	)

	result, err := RefreshChannelFromSnapshot(channel.Id, &Snapshot{
		Platform: PlatformNewAPI,
		BaseURL:  "https://newapi.example",
		Keys: []SyncedKey{{
			ExternalID: "channel-ratio-key",
			Name:       "Channel Ratio Key",
			Key:        "sk-channel-ratio-key",
			MaskedKey:  "sk-channel-ratio-key",
			GroupName:  "default",
			Models:     []string{"gpt-4o"},
			GroupRatio: floatPtr(1),
		}},
	}, RefreshRequest{ChannelID: channel.Id})

	require.NoError(t, err)
	require.Equal(t, 1, result.Created)

	var refreshed model.ChannelAccount
	require.NoError(t, db.Where("channel_id = ?", channel.Id).First(&refreshed).Error)
	metadata := ReadAccountSyncDisplayMetadata(refreshed.OtherSettings)
	require.NotNil(t, metadata.RatioConversionConfig)
	require.Equal(t, float64(1), metadata.RatioConversionConfig.PaidCNY)
	require.Equal(t, float64(10), metadata.RatioConversionConfig.PlatformUSDCredit)
	require.InDelta(t, 0.1, metadata.RatioConversion, 0.000001)
}

func TestRefreshChannelFromSnapshotExplicitRatioConversionOverridesExistingConfig(t *testing.T) {
	db := setupRefreshChannelTestDB(t)
	channel := createRefreshTestSyncedChannel(t, db, "ratio-override-channel", "")

	oldKey := SyncedKey{
		ExternalID: "ratio-key",
		Name:       "Ratio Key",
		Key:        "sk-ratio-old",
		MaskedKey:  "sk-ratio-old",
		GroupName:  "default",
		Models:     []string{"gpt-4o"},
		GroupRatio: floatPtr(10),
	}
	existingSnapshot := &Snapshot{
		Platform: PlatformNewAPI,
		BaseURL:  "https://newapi.example",
		Keys:     []SyncedKey{oldKey},
	}
	ApplyRatioConversion(existingSnapshot, RatioConversionConfig{
		PaidCNY:           1,
		PlatformUSDCredit: 1,
	})
	existing := model.ChannelAccount{
		ChannelId:     channel.Id,
		Name:          "Ratio Key",
		Key:           "sk-ratio-old",
		Status:        common.ChannelStatusEnabled,
		Models:        "gpt-4o",
		Group:         "default",
		AccessGroups:  "default",
		OtherSettings: mergeAccountSyncMetadata("", existingSnapshot, existingSnapshot.Keys[0]),
	}
	require.NoError(t, db.Create(&existing).Error)

	result, err := RefreshChannelFromSnapshot(channel.Id, &Snapshot{
		Platform: PlatformNewAPI,
		BaseURL:  "https://newapi.example",
		Keys: []SyncedKey{{
			ExternalID:        "ratio-key",
			Name:              "Ratio Key Refreshed",
			Key:               "sk-ratio-new",
			MaskedKey:         "sk-ratio-new",
			GroupName:         "default",
			Models:            []string{"gpt-4o"},
			GroupRatio:        floatPtr(10),
			SuggestedPriority: 1,
			SuggestedWeight:   100,
		}},
	}, RefreshRequest{
		ChannelID: channel.Id,
		RatioConversion: RatioConversionConfig{
			PaidCNY:           2,
			PlatformUSDCredit: 4,
		},
	})

	require.NoError(t, err)
	require.Equal(t, 1, result.Updated)

	var refreshed model.ChannelAccount
	require.NoError(t, db.First(&refreshed, existing.Id).Error)
	metadata := ReadAccountSyncDisplayMetadata(refreshed.OtherSettings)
	require.NotNil(t, metadata.RatioConversionConfig)
	require.True(t, metadata.RatioConversionConfig.Enabled)
	require.Equal(t, float64(2), metadata.RatioConversionConfig.PaidCNY)
	require.Equal(t, float64(4), metadata.RatioConversionConfig.PlatformUSDCredit)
	require.InDelta(t, 5, metadata.RatioConversion, 0.000001)
}

func TestRefreshChannelFromSnapshotAppliesExplicitLocalModelsAndGroup(t *testing.T) {
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelAccount{}))
	model.DB = db
	model.LOG_DB = db
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
	})

	channel := model.Channel{
		Type:   constant.ChannelTypeOpenAI,
		Key:    constant.ChannelCredentialModeAccountPool,
		Name:   "explicit-local-channel",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-old",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			CredentialMode:     constant.ChannelCredentialModeAccountPool,
			AccountPoolEnabled: true,
			AccountPoolMode:    constant.ChannelAccountPoolModePolling,
		},
	}
	require.NoError(t, db.Create(&channel).Error)

	oldKey := SyncedKey{
		ExternalID: "explicit",
		Name:       "Explicit Key",
		Key:        "sk-explicit-old",
		MaskedKey:  "sk-explicit-old",
		GroupName:  "default",
		Models:     []string{"gpt-old"},
	}
	existing := model.ChannelAccount{
		ChannelId:     channel.Id,
		Name:          "Explicit Key",
		Key:           "sk-explicit-old",
		Status:        common.ChannelStatusEnabled,
		Models:        "gpt-old",
		Group:         "default",
		AccessGroups:  "default",
		OtherSettings: mergeAccountSyncMetadata("", &Snapshot{Platform: PlatformNewAPI, BaseURL: "https://newapi.example"}, oldKey),
	}
	require.NoError(t, db.Create(&existing).Error)

	result, err := RefreshChannelFromSnapshot(channel.Id, &Snapshot{
		Platform: PlatformNewAPI,
		BaseURL:  "https://newapi.example",
		Keys: []SyncedKey{{
			ExternalID:        "explicit",
			Name:              "Explicit Key Refreshed",
			Key:               "sk-explicit-new",
			MaskedKey:         "sk-explicit-new",
			GroupName:         "upstream-vip",
			Models:            []string{"gpt-upstream"},
			SuggestedPriority: 3,
			SuggestedWeight:   80,
		}},
	}, RefreshRequest{
		ChannelID:      channel.Id,
		ApplySuggested: false,
		Accounts: []AccountCreateConfig{{
			ExternalID: "explicit",
			Models:     strPtr("gpt-local"),
			Group:      "local-vip",
		}},
	})

	require.NoError(t, err)
	require.Equal(t, 0, result.Created)
	require.Equal(t, 1, result.Updated)

	var refreshed model.ChannelAccount
	require.NoError(t, db.First(&refreshed, existing.Id).Error)
	require.Equal(t, "gpt-local", refreshed.Models)
	require.Equal(t, "local-vip", refreshed.Group)
}

func TestRefreshChannelFromSnapshotPreservesManualKeyModelsByDefault(t *testing.T) {
	db := setupRefreshChannelTestDB(t)
	withUpstreamAccountSyncModelSetting(t, true, false)

	channel := createRefreshTestSyncedChannel(t, db, "manual-model-channel", "")
	oldKey := SyncedKey{
		ExternalID: "manual-model",
		Name:       "Manual Model",
		Key:        "sk-manual-model-old",
		Models:     []string{"gpt-old"},
	}
	existingSettings := mergeAccountSyncMetadata("", &Snapshot{Platform: PlatformNewAPI, BaseURL: "https://newapi.example"}, oldKey)
	existingSettings = MarkAccountKeyModelsManualOverride(existingSettings)
	existing := model.ChannelAccount{
		ChannelId:     channel.Id,
		Name:          "Manual Model",
		Key:           "sk-manual-model-old",
		Status:        common.ChannelStatusEnabled,
		Models:        "local-allowlist",
		Group:         "default",
		AccessGroups:  "default",
		OtherSettings: existingSettings,
	}
	require.NoError(t, db.Create(&existing).Error)

	_, err := RefreshChannelFromSnapshot(channel.Id, &Snapshot{
		Platform: PlatformNewAPI,
		BaseURL:  "https://newapi.example",
		Keys: []SyncedKey{{
			ExternalID: "manual-model",
			Name:       "Manual Model Refreshed",
			Key:        "sk-manual-model-new",
			Models:     []string{"upstream-model"},
			GroupName:  "default",
		}},
	}, RefreshRequest{ChannelID: channel.Id})

	require.NoError(t, err)
	var refreshed model.ChannelAccount
	require.NoError(t, db.First(&refreshed, existing.Id).Error)
	require.Equal(t, "local-allowlist", refreshed.Models)
	require.True(t, AccountKeyModelsManualOverride(refreshed.OtherSettings))
}

func TestRefreshChannelFromSnapshotOverwritesManualKeyModelsWhenEnabled(t *testing.T) {
	db := setupRefreshChannelTestDB(t)
	withUpstreamAccountSyncModelSetting(t, true, true)

	channel := createRefreshTestSyncedChannel(t, db, "overwrite-model-channel", "")
	oldKey := SyncedKey{
		ExternalID: "overwrite-model",
		Name:       "Overwrite Model",
		Key:        "sk-overwrite-model-old",
		Models:     []string{"gpt-old"},
	}
	existingSettings := mergeAccountSyncMetadata("", &Snapshot{Platform: PlatformNewAPI, BaseURL: "https://newapi.example"}, oldKey)
	existingSettings = MarkAccountKeyModelsManualOverride(existingSettings)
	existing := model.ChannelAccount{
		ChannelId:     channel.Id,
		Name:          "Overwrite Model",
		Key:           "sk-overwrite-model-old",
		Status:        common.ChannelStatusEnabled,
		Models:        "local-allowlist",
		Group:         "default",
		AccessGroups:  "default",
		OtherSettings: existingSettings,
	}
	require.NoError(t, db.Create(&existing).Error)

	_, err := RefreshChannelFromSnapshot(channel.Id, &Snapshot{
		Platform: PlatformNewAPI,
		BaseURL:  "https://newapi.example",
		Keys: []SyncedKey{{
			ExternalID: "overwrite-model",
			Name:       "Overwrite Model Refreshed",
			Key:        "sk-overwrite-model-new",
			Models:     []string{"upstream-model"},
			GroupName:  "default",
		}},
	}, RefreshRequest{ChannelID: channel.Id})

	require.NoError(t, err)
	var refreshed model.ChannelAccount
	require.NoError(t, db.First(&refreshed, existing.Id).Error)
	require.Equal(t, "upstream-model", refreshed.Models)
	metadata := readAccountSyncMetadata(refreshed.OtherSettings)
	require.False(t, metadata.KeyModelsManualOverride)
	require.Equal(t, keyModelSyncSourceSnapshot, metadata.KeyModelsSyncSource)
	require.NotZero(t, metadata.KeyModelsSyncedAt)
}

func TestSyncSnapshotKeyModelsFetchesMissingModelsWithTargetKey(t *testing.T) {
	db := setupRefreshChannelTestDB(t)
	withUpstreamAccountSyncModelSetting(t, true, true)
	var seenAuthorization string

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/models", r.URL.Path)
		seenAuthorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"fetched-a"},{"id":"fetched-b"}]}`))
	}))
	defer upstream.Close()

	channel := createRefreshTestSyncedChannel(t, db, "fetch-missing-model-channel", "")
	channel.BaseURL = &upstream.URL
	require.NoError(t, db.Save(&channel).Error)
	snapshot := &Snapshot{
		Platform: PlatformNewAPI,
		BaseURL:  upstream.URL,
		Keys: []SyncedKey{{
			ExternalID: "fetch-key",
			Name:       "Fetch Key",
			Key:        "sk-target-fetch",
			GroupName:  "default",
		}},
	}

	syncSnapshotKeyModels(context.Background(), channel.Id, snapshot, nil)

	require.Equal(t, []string{"fetched-a", "fetched-b"}, snapshot.Keys[0].Models)
	require.Equal(t, keyModelSyncSourceFetchModels, snapshot.Keys[0].KeyModelSyncSource)
	require.Empty(t, snapshot.Keys[0].KeyModelSyncError)
	require.Equal(t, "Bearer sk-target-fetch", seenAuthorization)
}

func withUpstreamAccountSyncModelSetting(t *testing.T, enabled bool, overwriteManual bool) {
	t.Helper()
	setting := operation_setting.GetUpstreamAccountSyncSetting()
	oldEnabled := setting.SyncKeyModelsEnabled
	oldOverwrite := setting.KeyModelSyncOverwriteManualEnabled
	setting.SyncKeyModelsEnabled = enabled
	setting.KeyModelSyncOverwriteManualEnabled = overwriteManual
	t.Cleanup(func() {
		setting.SyncKeyModelsEnabled = oldEnabled
		setting.KeyModelSyncOverwriteManualEnabled = oldOverwrite
	})
}

func TestRefreshChannelFromSnapshotKeepsManualSchedulingWhenSuggestionsDisabled(t *testing.T) {
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelAccount{}))
	model.DB = db
	model.LOG_DB = db
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
	})

	channel := model.Channel{
		Type:   constant.ChannelTypeOpenAI,
		Key:    constant.ChannelCredentialModeAccountPool,
		Name:   "manual-scheduling-channel",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-old",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			CredentialMode:     constant.ChannelCredentialModeAccountPool,
			AccountPoolEnabled: true,
			AccountPoolMode:    constant.ChannelAccountPoolModePolling,
		},
	}
	require.NoError(t, db.Create(&channel).Error)

	oldKey := SyncedKey{
		ExternalID: "manual",
		Name:       "Manual Key",
		Key:        "sk-manual-old",
		MaskedKey:  "sk-manual-old",
		GroupName:  "default",
		Models:     []string{"gpt-old"},
	}
	existing := model.ChannelAccount{
		ChannelId:     channel.Id,
		Name:          "Manual Key",
		Key:           "sk-manual-old",
		Status:        common.ChannelStatusManuallyDisabled,
		Models:        "gpt-old",
		Group:         "default",
		AccessGroups:  "default",
		Priority:      33,
		Weight:        44,
		OtherSettings: mergeAccountSyncMetadata("", &Snapshot{Platform: PlatformSub2API, BaseURL: "https://sub2api.example"}, oldKey),
	}
	require.NoError(t, db.Create(&existing).Error)

	snapshot := &Snapshot{
		Platform: PlatformSub2API,
		BaseURL:  "https://sub2api.example",
		Keys: []SyncedKey{
			{
				ExternalID:        "manual",
				Name:              "Manual Key Refreshed",
				Key:               "sk-manual-new",
				MaskedKey:         "sk-manual-new",
				GroupName:         "vip",
				Models:            []string{"gpt-4o"},
				SuggestedPriority: 8,
				SuggestedWeight:   66,
			},
		},
	}
	result, err := RefreshChannelFromSnapshot(channel.Id, snapshot, RefreshRequest{
		ChannelID:      channel.Id,
		ApplySuggested: false,
	})

	require.NoError(t, err)
	require.Equal(t, 0, result.Created)
	require.Equal(t, 1, result.Updated)

	var refreshed model.ChannelAccount
	require.NoError(t, db.First(&refreshed, existing.Id).Error)
	require.Equal(t, "Manual Key Refreshed", refreshed.Name)
	require.Equal(t, "sk-manual-new", refreshed.Key)
	require.Equal(t, int64(33), refreshed.Priority)
	require.Equal(t, 100, refreshed.Weight)
	require.Equal(t, common.ChannelStatusManuallyDisabled, refreshed.Status)
}

func TestRefreshChannelFromSnapshotPreservesAutoDisabledAccountDespiteEnabledConfig(t *testing.T) {
	db := setupRefreshChannelTestDB(t)
	channel := createRefreshTestSyncedChannel(t, db, "auto-disabled-preserve-channel", "")
	key := SyncedKey{
		ExternalID: "auto-disabled",
		Name:       "Auto Disabled",
		Key:        "sk-auto-old",
		MaskedKey:  "sk-auto-old",
		GroupName:  "default",
		Models:     []string{"gpt-old"},
	}
	settings := mergeAccountSyncMetadata("", &Snapshot{Platform: PlatformNewAPI, BaseURL: "https://newapi.example"}, key)
	settings = ApplyAccountAutoCheckFailure(settings, 2, "previous upstream failure", true)
	existing := model.ChannelAccount{
		ChannelId:         channel.Id,
		Name:              "Auto Disabled",
		Key:               "sk-auto-old",
		Status:            common.ChannelStatusAutoDisabled,
		Models:            "gpt-old",
		Group:             "default",
		AccessGroups:      "default",
		OtherSettings:     settings,
		DisabledReason:    "previous disabled reason",
		LastError:         "previous last error",
		RateLimitedUntil:  111,
		OverloadUntil:     222,
		TempDisabledUntil: 333,
	}
	require.NoError(t, db.Create(&existing).Error)

	enabled := true
	result, err := RefreshChannelFromSnapshot(channel.Id, &Snapshot{
		Platform: PlatformNewAPI,
		BaseURL:  "https://newapi.example",
		Keys: []SyncedKey{
			{
				ExternalID: "auto-disabled",
				Name:       "Auto Disabled Refreshed",
				Key:        "sk-auto-new",
				MaskedKey:  "sk-auto-new",
				GroupName:  "vip",
				Models:     []string{"gpt-new"},
			},
		},
	}, RefreshRequest{
		ChannelID: channel.Id,
		Accounts:  []AccountCreateConfig{{ExternalID: "auto-disabled", Enabled: &enabled}},
	})

	require.NoError(t, err)
	require.Equal(t, 1, result.Updated)
	var refreshed model.ChannelAccount
	require.NoError(t, db.First(&refreshed, existing.Id).Error)
	require.Equal(t, "Auto Disabled Refreshed", refreshed.Name)
	require.Equal(t, "sk-auto-new", refreshed.Key)
	require.Equal(t, common.ChannelStatusAutoDisabled, refreshed.Status)
	require.Equal(t, "previous disabled reason", refreshed.DisabledReason)
	require.Equal(t, "previous last error", refreshed.LastError)
	require.Equal(t, int64(111), refreshed.RateLimitedUntil)
	require.Equal(t, int64(222), refreshed.OverloadUntil)
	require.Equal(t, int64(333), refreshed.TempDisabledUntil)
	metadata := ReadAccountAutoCheckMetadata(refreshed.OtherSettings)
	require.True(t, metadata.DisabledByAutoCheck)
	require.Equal(t, 2, metadata.FailureCount)
}

func TestRefreshChannelFromSnapshotExplicitDisableClearsAutoRecoverMarker(t *testing.T) {
	db := setupRefreshChannelTestDB(t)
	channel := createRefreshTestSyncedChannel(t, db, "explicit-disable-marker-channel", "")
	key := SyncedKey{
		ExternalID: "explicit-disable",
		Name:       "Explicit Disable",
		Key:        "sk-explicit-old",
		MaskedKey:  "sk-explicit-old",
		GroupName:  "default",
		Models:     []string{"gpt-old"},
	}
	settings := mergeAccountSyncMetadata("", &Snapshot{Platform: PlatformNewAPI, BaseURL: "https://newapi.example"}, key)
	settings = ApplyAccountAutoCheckFailure(settings, 2, "previous upstream failure", true)
	settings = ApplyAccountAutoCheckAutomaticSuccess(settings, 10)
	existing := model.ChannelAccount{
		ChannelId:     channel.Id,
		Name:          "Explicit Disable",
		Key:           "sk-explicit-old",
		Status:        common.ChannelStatusEnabled,
		Models:        "gpt-old",
		Group:         "default",
		AccessGroups:  "default",
		OtherSettings: settings,
	}
	require.NoError(t, db.Create(&existing).Error)

	disabled := false
	result, err := RefreshChannelFromSnapshot(channel.Id, &Snapshot{
		Platform: PlatformNewAPI,
		BaseURL:  "https://newapi.example",
		Keys: []SyncedKey{
			{
				ExternalID: "explicit-disable",
				Name:       "Explicit Disable Refreshed",
				Key:        "sk-explicit-new",
				MaskedKey:  "sk-explicit-new",
				GroupName:  "default",
				Models:     []string{"gpt-new"},
			},
		},
	}, RefreshRequest{
		ChannelID: channel.Id,
		Accounts:  []AccountCreateConfig{{ExternalID: "explicit-disable", Enabled: &disabled}},
	})

	require.NoError(t, err)
	require.Equal(t, 1, result.Updated)
	var refreshed model.ChannelAccount
	require.NoError(t, db.First(&refreshed, existing.Id).Error)
	require.Equal(t, common.ChannelStatusManuallyDisabled, refreshed.Status)
	require.Equal(t, "upstream account sync disabled", refreshed.DisabledReason)
	metadata := ReadAccountAutoCheckMetadata(refreshed.OtherSettings)
	require.False(t, metadata.DisabledByAutoCheck)
	require.Equal(t, 0, metadata.FastSuccessStreak)
	require.Equal(t, "manual", metadata.LastStatus)
}

func TestRefreshChannelFromSnapshotAppliesConfigBySyncIDWhenExternalIDMissing(t *testing.T) {
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelAccount{}))
	model.DB = db
	model.LOG_DB = db
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
	})

	channel := model.Channel{
		Type:   constant.ChannelTypeOpenAI,
		Key:    constant.ChannelCredentialModeAccountPool,
		Name:   "sync-id-refresh-channel",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-4o",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			CredentialMode:     constant.ChannelCredentialModeAccountPool,
			AccountPoolEnabled: true,
			AccountPoolMode:    constant.ChannelAccountPoolModePolling,
		},
	}
	require.NoError(t, db.Create(&channel).Error)

	key := SyncedKey{
		Name:              "No External ID",
		Key:               "sk-no-external-id",
		MaskedKey:         "sk-noe...l-id",
		GroupName:         "default",
		Models:            []string{"gpt-4o"},
		SuggestedPriority: 1,
		SuggestedWeight:   100,
	}
	snapshot := &Snapshot{
		Platform: PlatformSub2API,
		BaseURL:  "https://sub2api.example",
		Keys:     []SyncedKey{key},
	}
	ApplySyncIDs(snapshot)

	result, err := RefreshChannelFromSnapshot(channel.Id, snapshot, RefreshRequest{
		ChannelID:      channel.Id,
		ApplySuggested: false,
		Accounts: []AccountCreateConfig{
			{SyncID: snapshot.Keys[0].SyncID, Priority: int64Ptr(12), Weight: intPtr(34)},
		},
	})

	require.NoError(t, err)
	require.Equal(t, 1, result.Created)

	var account model.ChannelAccount
	require.NoError(t, db.First(&account).Error)
	require.Equal(t, int64(12), account.Priority)
	require.Equal(t, 100, account.Weight)
}

func TestRefreshChannelFromSnapshotRejectsEnabledSyncedAccountWithoutModels(t *testing.T) {
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelAccount{}))
	model.DB = db
	model.LOG_DB = db
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
	})

	channel := model.Channel{
		Type:   constant.ChannelTypeOpenAI,
		Key:    constant.ChannelCredentialModeAccountPool,
		Name:   "explicit-empty-channel",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-old",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			CredentialMode:     constant.ChannelCredentialModeAccountPool,
			AccountPoolEnabled: true,
			AccountPoolMode:    constant.ChannelAccountPoolModePolling,
		},
	}
	require.NoError(t, db.Create(&channel).Error)

	key := SyncedKey{
		ExternalID: "empty",
		Name:       "Empty Key",
		Key:        "sk-empty",
		MaskedKey:  "sk-empty",
		GroupName:  "vip",
		Models:     []string{"gpt-4o"},
	}
	existing := model.ChannelAccount{
		ChannelId:     channel.Id,
		Name:          "Empty Key",
		Key:           "sk-empty",
		Status:        common.ChannelStatusEnabled,
		Models:        "gpt-old",
		Group:         "default",
		AccessGroups:  "default",
		OtherSettings: mergeAccountSyncMetadata("", &Snapshot{Platform: PlatformNewAPI, BaseURL: "https://newapi.example"}, key),
	}
	require.NoError(t, db.Create(&existing).Error)

	result, err := RefreshChannelFromSnapshot(channel.Id, &Snapshot{
		Platform: PlatformNewAPI,
		BaseURL:  "https://newapi.example",
		Keys: []SyncedKey{
			{
				ExternalID:        "empty",
				Name:              "Empty Key",
				Key:               "sk-empty-new",
				MaskedKey:         "sk-empty-new",
				GroupName:         "vip",
				Models:            []string{"gpt-4o"},
				SuggestedPriority: 5,
				SuggestedWeight:   70,
			},
		},
	}, RefreshRequest{
		ChannelID:      channel.Id,
		ApplySuggested: false,
		Accounts: []AccountCreateConfig{
			{
				ExternalID: "empty",
				Models:     strPtr(""),
				Group:      "",
				Priority:   int64Ptr(9),
				Weight:     intPtr(8),
			},
		},
	})

	require.ErrorContains(t, err, "必须配置至少一个模型")
	require.Nil(t, result)

	var refreshed model.ChannelAccount
	require.NoError(t, db.First(&refreshed, existing.Id).Error)
	require.Equal(t, "gpt-old", refreshed.Models)
	require.Equal(t, "default", refreshed.Group)
	require.Equal(t, int64(0), refreshed.Priority)
	require.Equal(t, 0, refreshed.Weight)
}

func TestRefreshChannelFromSnapshotRejectsEnabledSyncedAccountWithoutAccessGroups(t *testing.T) {
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelAccount{}))
	model.DB = db
	model.LOG_DB = db
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
	})

	channel := model.Channel{
		Type:   constant.ChannelTypeOpenAI,
		Key:    constant.ChannelCredentialModeAccountPool,
		Name:   "explicit-empty-access-groups-channel",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-old",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			CredentialMode:     constant.ChannelCredentialModeAccountPool,
			AccountPoolEnabled: true,
			AccountPoolMode:    constant.ChannelAccountPoolModePolling,
		},
	}
	require.NoError(t, db.Create(&channel).Error)

	key := SyncedKey{
		ExternalID: "empty-access-groups",
		Name:       "Empty Access Groups",
		Key:        "sk-empty-access-groups",
		MaskedKey:  "sk-empty-access-groups",
		GroupName:  "vip",
		Models:     []string{"gpt-4o"},
	}
	existing := model.ChannelAccount{
		ChannelId:     channel.Id,
		Name:          "Empty Access Groups",
		Key:           "sk-empty-access-groups",
		Status:        common.ChannelStatusEnabled,
		Models:        "gpt-old",
		Group:         "default",
		AccessGroups:  "default",
		OtherSettings: mergeAccountSyncMetadata("", &Snapshot{Platform: PlatformNewAPI, BaseURL: "https://newapi.example"}, key),
	}
	require.NoError(t, db.Create(&existing).Error)

	result, err := RefreshChannelFromSnapshot(channel.Id, &Snapshot{
		Platform: PlatformNewAPI,
		BaseURL:  "https://newapi.example",
		Keys: []SyncedKey{
			{
				ExternalID:        "empty-access-groups",
				Name:              "Empty Access Groups",
				Key:               "sk-empty-access-groups-new",
				MaskedKey:         "sk-empty-access-groups-new",
				GroupName:         "vip",
				Models:            []string{"gpt-4o"},
				SuggestedPriority: 5,
				SuggestedWeight:   70,
			},
		},
	}, RefreshRequest{
		ChannelID:      channel.Id,
		ApplySuggested: false,
		Accounts: []AccountCreateConfig{
			{
				ExternalID:   "empty-access-groups",
				AccessGroups: strPtr(""),
			},
		},
	})

	require.ErrorContains(t, err, "必须配置至少一个 NexusTok 可访问用户组")
	require.Nil(t, result)

	var refreshed model.ChannelAccount
	require.NoError(t, db.First(&refreshed, existing.Id).Error)
	require.Equal(t, "gpt-old", refreshed.Models)
	require.Equal(t, "default", refreshed.AccessGroups)
}

func TestRefreshChannelFromSnapshotAllowsDisabledSyncedAccountWithoutCapabilities(t *testing.T) {
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelAccount{}))
	model.DB = db
	model.LOG_DB = db
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
	})

	channel := model.Channel{
		Type:   constant.ChannelTypeOpenAI,
		Key:    constant.ChannelCredentialModeAccountPool,
		Name:   "disabled-empty-channel",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-old",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			CredentialMode:     constant.ChannelCredentialModeAccountPool,
			AccountPoolEnabled: true,
			AccountPoolMode:    constant.ChannelAccountPoolModePolling,
		},
	}
	require.NoError(t, db.Create(&channel).Error)

	key := SyncedKey{
		ExternalID: "disabled-empty",
		Name:       "Disabled Empty",
		Key:        "sk-disabled-empty",
		MaskedKey:  "sk-disabled-empty",
		GroupName:  "vip",
		Models:     []string{"gpt-4o"},
	}
	existing := model.ChannelAccount{
		ChannelId:     channel.Id,
		Name:          "Disabled Empty",
		Key:           "sk-disabled-empty",
		Status:        common.ChannelStatusEnabled,
		Models:        "gpt-old",
		Group:         "default",
		AccessGroups:  "default",
		OtherSettings: mergeAccountSyncMetadata("", &Snapshot{Platform: PlatformNewAPI, BaseURL: "https://newapi.example"}, key),
	}
	require.NoError(t, db.Create(&existing).Error)

	result, err := RefreshChannelFromSnapshot(channel.Id, &Snapshot{
		Platform: PlatformNewAPI,
		BaseURL:  "https://newapi.example",
		Keys: []SyncedKey{
			{
				ExternalID:        "disabled-empty",
				Name:              "Disabled Empty",
				Key:               "sk-disabled-empty-new",
				MaskedKey:         "sk-disabled-empty-new",
				GroupName:         "vip",
				Models:            []string{"gpt-4o"},
				SuggestedPriority: 5,
				SuggestedWeight:   70,
			},
		},
	}, RefreshRequest{
		ChannelID:      channel.Id,
		ApplySuggested: false,
		Accounts: []AccountCreateConfig{
			{
				ExternalID:   "disabled-empty",
				Enabled:      boolPtr(false),
				Models:       strPtr(""),
				AccessGroups: strPtr(""),
			},
		},
	})

	require.NoError(t, err)
	require.Equal(t, 0, result.Created)
	require.Equal(t, 1, result.Updated)

	var refreshed model.ChannelAccount
	require.NoError(t, db.First(&refreshed, existing.Id).Error)
	require.Equal(t, common.ChannelStatusManuallyDisabled, refreshed.Status)
	require.Equal(t, "", refreshed.Models)
	require.Equal(t, "", refreshed.AccessGroups)
}

func TestRefreshChannelFromSnapshotDisablesMissingSub2APIKeyWithLoginMetadataURL(t *testing.T) {
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelAccount{}))
	model.DB = db
	model.LOG_DB = db
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
	})

	channel := model.Channel{
		Type:   constant.ChannelTypeOpenAI,
		Key:    constant.ChannelCredentialModeAccountPool,
		Name:   "sub2-login-metadata-channel",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-4o",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			CredentialMode:     constant.ChannelCredentialModeAccountPool,
			AccountPoolEnabled: true,
			AccountPoolMode:    constant.ChannelAccountPoolModePolling,
		},
	}
	require.NoError(t, db.Create(&channel).Error)

	missingKey := SyncedKey{
		ExternalID: "missing",
		Name:       "Missing",
		Key:        "sk-missing",
		MaskedKey:  "sk-missing",
		GroupName:  "default",
	}
	account := model.ChannelAccount{
		ChannelId:    channel.Id,
		Name:         "Missing",
		Key:          "sk-missing",
		Status:       common.ChannelStatusEnabled,
		Models:       "gpt-4o",
		Group:        "default",
		AccessGroups: "default",
		OtherSettings: mergeAccountSyncMetadata("", &Snapshot{
			Platform: PlatformSub2API,
			BaseURL:  "https://sub2api.example/login",
		}, missingKey),
	}
	require.NoError(t, db.Create(&account).Error)

	result, err := RefreshChannelFromSnapshot(channel.Id, &Snapshot{
		Platform: PlatformSub2API,
		BaseURL:  "https://sub2api.example",
		Keys:     []SyncedKey{},
	}, RefreshRequest{
		ChannelID:         channel.Id,
		DisableMissingKey: true,
	})

	require.NoError(t, err)
	require.Equal(t, 1, result.Disabled)

	var refreshed model.ChannelAccount
	require.NoError(t, db.First(&refreshed, account.Id).Error)
	require.Equal(t, common.ChannelStatusManuallyDisabled, refreshed.Status)
}
