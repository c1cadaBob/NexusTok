package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/service/upstreamaccount"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupUpstreamAccountSyncAuditTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldUsingSQLite := common.UsingSQLite
	oldUsingMySQL := common.UsingMySQL
	oldUsingPostgreSQL := common.UsingPostgreSQL
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	oldRedisEnabled := common.RedisEnabled

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.MemoryCacheEnabled = false
	common.RedisEnabled = false

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.Channel{},
		&model.Ability{},
		&model.ChannelAccount{},
		&model.Log{},
	))
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.Create(&model.User{
		Id:       1,
		Username: "root",
		Role:     common.RoleRootUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
	}).Error)

	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.UsingSQLite = oldUsingSQLite
		common.UsingMySQL = oldUsingMySQL
		common.UsingPostgreSQL = oldUsingPostgreSQL
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
		common.RedisEnabled = oldRedisEnabled
	})
	return db
}

func createUpstreamAccountSyncAuditChannel(t *testing.T, db *gorm.DB, name string) model.Channel {
	t.Helper()

	channel := model.Channel{
		Type:   constant.ChannelTypeOpenAI,
		Key:    constant.ChannelCredentialModeAccountPool,
		Name:   name,
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
	return channel
}

func newUpstreamAccountSyncAuditContext(t *testing.T, channelID int, body any) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()

	payload, err := common.Marshal(body)
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/"+strconv.Itoa(channelID)+"/upstream-account/refresh", bytes.NewReader(payload))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(channelID)}}
	ctx.Set("id", 1)
	ctx.Set("username", "root")
	ctx.Set("role", common.RoleRootUser)
	return ctx, recorder
}

func lastUpstreamAccountSyncAuditLog(t *testing.T) (model.Log, map[string]interface{}) {
	t.Helper()

	var log model.Log
	require.NoError(t, model.LOG_DB.Where("type = ?", model.LogTypeManage).Order("id desc").First(&log).Error)
	other, err := common.StrToMap(log.Other)
	require.NoError(t, err)
	return log, other
}

func TestRefreshUpstreamAccountChannelWritesSuccessAudit(t *testing.T) {
	db := setupUpstreamAccountSyncAuditTestDB(t)
	channel := createUpstreamAccountSyncAuditChannel(t, db, "manual-sync-channel")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/api/v1/") && r.URL.Path != "/api/v1/auth/login" {
			require.Equal(t, "Bearer manual-token", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/api/v1/auth/login":
			var body map[string]string
			require.NoError(t, common.DecodeJson(r.Body, &body))
			require.Equal(t, "alice@example.com", body["email"])
			require.Equal(t, "secret", body["password"])
			_, _ = w.Write([]byte(`{"code":0,"data":{"access_token":"manual-token","refresh_token":"manual-refresh","expires_in":3600,"user":{"id":5,"email":"alice@example.com","balance":1}}}`))
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
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"id":9,"name":"manual-key","key":"sk-manual-audit-key","status":"active","group_id":3,"group":{"id":3,"name":"vip"},"models":["gpt-4o"],"quota":20,"quota_used":3}],"total":1}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ctx, recorder := newUpstreamAccountSyncAuditContext(t, channel.Id, upstreamaccount.RefreshRequest{
		Credential: upstreamaccount.Credential{
			Platform: upstreamaccount.PlatformSub2API,
			BaseURL:  server.URL,
			Email:    "alice@example.com",
			AuthMode: upstreamaccount.AuthModePassword,
			Password: "secret",
		},
		ApplySuggested:    true,
		DisableMissingKey: true,
	})

	RefreshUpstreamAccountChannel(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.True(t, common.GetContextKeyBool(ctx, constant.ContextKeyAuditLogged))
	var count int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("type = ?", model.LogTypeManage).Count(&count).Error)
	require.EqualValues(t, 1, count)

	log, other := lastUpstreamAccountSyncAuditLog(t)
	require.Equal(t, "root", log.Username)
	op := other["op"].(map[string]interface{})
	require.Equal(t, "channel.upstream_account_sync_refresh", op["action"])
	params := op["params"].(map[string]interface{})
	require.Equal(t, "manual-sync-channel", params["name"])
	require.Equal(t, "sub2api", params["platform"])
	require.Equal(t, true, params["success"])
	require.EqualValues(t, 1, params["created"])
	auditInfo := other["audit_info"].(map[string]interface{})
	require.Equal(t, "manual", auditInfo["source"])
	require.Equal(t, true, auditInfo["success"])
	require.EqualValues(t, 1, auditInfo["created"])
	require.NotContains(t, log.Other, "secret")
	require.NotContains(t, log.Other, "manual-token")
	require.NotContains(t, log.Other, "sk-manual-audit-key")
}

func TestRefreshUpstreamAccountChannelWritesFailureAuditWithoutSecrets(t *testing.T) {
	db := setupUpstreamAccountSyncAuditTestDB(t)
	channel := createUpstreamAccountSyncAuditChannel(t, db, "manual-sync-failed-channel")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/auth/login":
			_, _ = w.Write([]byte(`{"code":1,"message":"password=secret access_token=manual-token refresh_token=manual-refresh sk-sensitive-audit-key failed"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ctx, recorder := newUpstreamAccountSyncAuditContext(t, channel.Id, upstreamaccount.RefreshRequest{
		Credential: upstreamaccount.Credential{
			Platform: upstreamaccount.PlatformSub2API,
			BaseURL:  server.URL,
			Email:    "alice@example.com",
			AuthMode: upstreamaccount.AuthModePassword,
			Password: "secret",
		},
	})

	RefreshUpstreamAccountChannel(ctx)

	require.True(t, recorder.Code >= http.StatusBadRequest || recorder.Code == http.StatusOK)
	require.True(t, common.GetContextKeyBool(ctx, constant.ContextKeyAuditLogged))
	log, other := lastUpstreamAccountSyncAuditLog(t)
	op := other["op"].(map[string]interface{})
	require.Equal(t, "channel.upstream_account_sync_refresh", op["action"])
	params := op["params"].(map[string]interface{})
	require.Equal(t, false, params["success"])
	require.Contains(t, params["error"], "password=[redacted]")
	auditInfo := other["audit_info"].(map[string]interface{})
	require.Equal(t, false, auditInfo["success"])
	require.Contains(t, auditInfo["error"], "password=[redacted]")
	require.NotContains(t, log.Other, "secret")
	require.NotContains(t, log.Other, "manual-token")
	require.NotContains(t, log.Other, "manual-refresh")
	require.NotContains(t, log.Other, "sk-sensitive-audit-key")
}
