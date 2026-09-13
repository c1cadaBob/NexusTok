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
		case request.URL.Path == "/api/user/self":
			assert.Equal(t, "Bearer newapi-session", request.Header.Get("Authorization"))
			_, _ = writer.Write([]byte(`{"success":true,"data":{"quota":12.5,"used_quota":3}}`))
		case request.URL.Path == "/api/user/models":
			_, _ = writer.Write([]byte(`{"success":true,"data":["gpt-4o","claude-3-7-sonnet"]}`))
		case request.URL.Path == "/api/token/":
			assert.Equal(t, "1", request.URL.Query().Get("p"))
			_, _ = writer.Write([]byte(`{"success":true,"data":{"items":[{"id":7,"name":"primary","key":"sk-newapi","quota":8,"expired_time":"4102444800","models":["gpt-4o"]}]}}`))
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
	assert.Equal(t, 12.5, snapshot.Balance)
	require.Len(t, snapshot.Keys, 1)
	assert.Equal(t, "sk-newapi", snapshot.Keys[0].Secret)
	assert.Equal(t, int64(8), *snapshot.Keys[0].RemainQuota)
	assert.Equal(t, []string{"gpt-4o"}, snapshot.Keys[0].Models)
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
			ExternalID:  "key-1",
			Name:        "key",
			Secret:      "sk-test",
			Group:       "default",
			Models:      []string{"gpt-4o"},
			RemainQuota: &remaining,
		}},
	}))

	var key model.UpstreamKey
	require.NoError(t, db.Where("channel_id = ?", channel.Id).First(&key).Error)
	assert.Equal(t, model.UpstreamKeyStatusAutoDisabled, key.Status)
	assert.Equal(t, "密钥剩余额度不足", key.DisabledReason)
	assert.Equal(t, 1900, key.Weight)

	var ability model.Ability
	require.NoError(t, db.Where(&model.Ability{
		ChannelId: channel.Id,
		Group:     "default",
		Model:     "gpt-4o",
	}).First(&ability).Error)
	assert.True(t, ability.Enabled)
}

func TestPlatformSiteRequestRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte(strings.Repeat("x", upstreamSiteResponseLimit+1)))
	}))
	defer server.Close()

	session, err := newPlatformSiteSession(server.URL, nil)
	require.NoError(t, err)
	_, err = platformSiteRequest(context.Background(), session, http.MethodGet, "/", url.Values{}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "响应体超过限制")
}
