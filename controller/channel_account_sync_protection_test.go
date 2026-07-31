package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type channelAccountMutationAPIResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// setupChannelAccountMutationTestDB 初始化账号池手动变更控制器测试使用的内存数据库。
func setupChannelAccountMutationTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	oldDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelAccount{}))
	model.DB = db
	t.Cleanup(func() {
		model.DB = oldDB
		model.InitChannelCache()
	})
	return db
}

// createChannelAccountMutationTestChannel 创建用于账号池手动变更测试的渠道。
func createChannelAccountMutationTestChannel(t *testing.T, db *gorm.DB, settings string, key string) model.Channel {
	t.Helper()
	channel := model.Channel{
		Type:          constant.ChannelTypeOpenAI,
		Key:           key,
		Name:          "account-mutation-test",
		Status:        common.ChannelStatusEnabled,
		Models:        "gpt-4o-mini",
		Group:         "default",
		OtherSettings: settings,
	}
	require.NoError(t, db.Create(&channel).Error)
	return channel
}

// createChannelAccountMutationRequest 构造带渠道路径参数的 Gin 测试上下文。
func createChannelAccountMutationRequest(
	t *testing.T,
	method string,
	channelID int,
	accountID int,
	body any,
) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	payload := []byte("{}")
	if body != nil {
		encoded, err := common.Marshal(body)
		require.NoError(t, err)
		payload = encoded
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		method,
		"/api/channel/"+strconv.Itoa(channelID)+"/accounts",
		bytes.NewReader(payload),
	)
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(channelID)}}
	if accountID > 0 {
		ctx.Params = append(ctx.Params, gin.Param{Key: "account_id", Value: strconv.Itoa(accountID)})
	}
	return ctx, recorder
}

// decodeChannelAccountMutationResponse 解析控制器返回的标准 API 响应。
func decodeChannelAccountMutationResponse(t *testing.T, recorder *httptest.ResponseRecorder) channelAccountMutationAPIResponse {
	t.Helper()
	var response channelAccountMutationAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}

// requireChannelAccountMutationRejected 断言同步渠道手动变更被业务规则拒绝。
func requireChannelAccountMutationRejected(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	require.Equal(t, http.StatusOK, recorder.Code)
	response := decodeChannelAccountMutationResponse(t, recorder)
	require.False(t, response.Success)
	require.Contains(t, response.Message, upstreamAccountSyncManualAccountMutationError)
}

// requireChannelAccountMutationSucceeded 断言普通渠道手动账号池变更成功。
func requireChannelAccountMutationSucceeded(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	require.Equal(t, http.StatusOK, recorder.Code)
	response := decodeChannelAccountMutationResponse(t, recorder)
	require.True(t, response.Success, response.Message)
}

// TestSyncedChannelRejectsManualAccountMutations 验证同步渠道不能手动新增、导入或删除账号。
func TestSyncedChannelRejectsManualAccountMutations(t *testing.T) {
	db := setupChannelAccountMutationTestDB(t)
	channel := createChannelAccountMutationTestChannel(
		t,
		db,
		`{"upstream_account_sync":{"platform":"new-api","base_url":"https://upstream.example","synced_at":1}}`,
		"sk-upstream-a\nsk-upstream-b",
	)
	account := model.ChannelAccount{
		ChannelId: channel.Id,
		Name:      "synced-account",
		Key:       "sk-synced",
		Status:    common.ChannelStatusEnabled,
		Models:    "gpt-4o-mini",
		Group:     "default",
	}
	require.NoError(t, db.Create(&account).Error)

	t.Run("单个新增被拒绝", func(t *testing.T) {
		ctx, recorder := createChannelAccountMutationRequest(t, http.MethodPost, channel.Id, 0, gin.H{
			"name":   "manual-create",
			"key":    "sk-manual",
			"models": "gpt-4o-mini",
			"group":  "default",
		})

		CreateChannelAccount(ctx)

		requireChannelAccountMutationRejected(t, recorder)
	})

	t.Run("批量导入被拒绝", func(t *testing.T) {
		ctx, recorder := createChannelAccountMutationRequest(t, http.MethodPost, channel.Id, 0, gin.H{
			"keys":   "sk-batch-a\nsk-batch-b",
			"models": "gpt-4o-mini",
			"group":  "default",
		})

		BatchCreateChannelAccounts(ctx)

		requireChannelAccountMutationRejected(t, recorder)
	})

	t.Run("从多 Key 导入被拒绝", func(t *testing.T) {
		ctx, recorder := createChannelAccountMutationRequest(t, http.MethodPost, channel.Id, 0, gin.H{})

		ImportMultiKeyToChannelAccounts(ctx)

		requireChannelAccountMutationRejected(t, recorder)
	})

	t.Run("删除账号被拒绝", func(t *testing.T) {
		ctx, recorder := createChannelAccountMutationRequest(t, http.MethodDelete, channel.Id, account.Id, nil)

		DeleteChannelAccount(ctx)

		requireChannelAccountMutationRejected(t, recorder)
		var count int64
		require.NoError(t, db.Model(&model.ChannelAccount{}).Where("id = ?", account.Id).Count(&count).Error)
		require.EqualValues(t, 1, count)
	})
}

// TestRegularChannelAllowsManualAccountMutations 验证普通渠道账号池手动入口保持可用。
func TestRegularChannelAllowsManualAccountMutations(t *testing.T) {
	db := setupChannelAccountMutationTestDB(t)

	t.Run("单个新增仍可用", func(t *testing.T) {
		channel := createChannelAccountMutationTestChannel(t, db, `{"allow_service_tier":false}`, "sk-main")
		ctx, recorder := createChannelAccountMutationRequest(t, http.MethodPost, channel.Id, 0, gin.H{
			"name":   "manual-create",
			"key":    "sk-manual",
			"models": "gpt-4o-mini",
			"group":  "default",
		})

		CreateChannelAccount(ctx)

		requireChannelAccountMutationSucceeded(t, recorder)
		var count int64
		require.NoError(t, db.Model(&model.ChannelAccount{}).Where("channel_id = ?", channel.Id).Count(&count).Error)
		require.EqualValues(t, 1, count)
	})

	t.Run("批量导入仍可用", func(t *testing.T) {
		channel := createChannelAccountMutationTestChannel(t, db, `{"allow_service_tier":false}`, "sk-main")
		ctx, recorder := createChannelAccountMutationRequest(t, http.MethodPost, channel.Id, 0, gin.H{
			"keys":   "sk-batch-a\nsk-batch-b",
			"models": "gpt-4o-mini",
			"group":  "default",
		})

		BatchCreateChannelAccounts(ctx)

		requireChannelAccountMutationSucceeded(t, recorder)
		var count int64
		require.NoError(t, db.Model(&model.ChannelAccount{}).Where("channel_id = ?", channel.Id).Count(&count).Error)
		require.EqualValues(t, 2, count)
	})

	t.Run("从多 Key 导入仍可用", func(t *testing.T) {
		channel := createChannelAccountMutationTestChannel(t, db, `{"allow_service_tier":false}`, "sk-import-a\nsk-import-b")
		ctx, recorder := createChannelAccountMutationRequest(t, http.MethodPost, channel.Id, 0, gin.H{})

		ImportMultiKeyToChannelAccounts(ctx)

		requireChannelAccountMutationSucceeded(t, recorder)
		var count int64
		require.NoError(t, db.Model(&model.ChannelAccount{}).Where("channel_id = ?", channel.Id).Count(&count).Error)
		require.EqualValues(t, 2, count)
	})

	t.Run("删除账号仍可用", func(t *testing.T) {
		channel := createChannelAccountMutationTestChannel(t, db, `{"allow_service_tier":false}`, "sk-main")
		account := model.ChannelAccount{
			ChannelId: channel.Id,
			Name:      "manual-delete",
			Key:       "sk-delete",
			Status:    common.ChannelStatusEnabled,
			Models:    "gpt-4o-mini",
			Group:     "default",
		}
		require.NoError(t, db.Create(&account).Error)
		ctx, recorder := createChannelAccountMutationRequest(t, http.MethodDelete, channel.Id, account.Id, nil)

		DeleteChannelAccount(ctx)

		requireChannelAccountMutationSucceeded(t, recorder)
		var count int64
		require.NoError(t, db.Model(&model.ChannelAccount{}).Where("id = ?", account.Id).Count(&count).Error)
		require.EqualValues(t, 0, count)
	})
}
