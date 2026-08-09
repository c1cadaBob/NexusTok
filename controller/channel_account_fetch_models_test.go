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
	"github.com/c1cada/NexusTok/service/upstreamaccount"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type channelAccountFetchModelsAPIResponse struct {
	Success bool     `json:"success"`
	Message string   `json:"message"`
	Data    []string `json:"data"`
}

// createChannelAccountFetchModelsRequest 构造账号级模型获取接口的 Gin 测试上下文。
func createChannelAccountFetchModelsRequest(channelID int, accountID int) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/"+strconv.Itoa(channelID)+"/accounts/"+strconv.Itoa(accountID)+"/fetch_models", nil)
	ctx.Params = gin.Params{
		{Key: "id", Value: strconv.Itoa(channelID)},
		{Key: "account_id", Value: strconv.Itoa(accountID)},
	}
	return ctx, recorder
}

// createPreviewKeyFetchModelsRequest 构造未保存同步密钥模型获取接口的 Gin 测试上下文。
func createPreviewKeyFetchModelsRequest(t *testing.T, previewID string, body any) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	payload, err := common.Marshal(body)
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/upstream-account/preview/"+previewID+"/key-models", bytes.NewReader(payload))
	ctx.Params = gin.Params{{Key: "preview_id", Value: previewID}}
	return ctx, recorder
}

func decodeChannelAccountFetchModelsResponse(t *testing.T, recorder *httptest.ResponseRecorder) channelAccountFetchModelsAPIResponse {
	t.Helper()
	var response channelAccountFetchModelsAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}

func TestFetchChannelAccountUpstreamModelsUsesSelectedAccountCredential(t *testing.T) {
	db := setupChannelAccountMutationTestDB(t)
	var seenAuthorization string

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/models", r.URL.Path)
		seenAuthorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-5.5"},{"id":"claude-3.7"}]}`))
	}))
	defer upstream.Close()

	channelBaseURL := "https://wrong-channel.example"
	channel := model.Channel{
		Type:    constant.ChannelTypeOpenAI,
		Key:     "sk-channel",
		Name:    "account-fetch-models",
		Status:  common.ChannelStatusEnabled,
		BaseURL: &channelBaseURL,
	}
	require.NoError(t, db.Create(&channel).Error)

	accountBaseURL := upstream.URL
	account := model.ChannelAccount{
		ChannelId: channel.Id,
		Name:      "selected",
		Key:       "sk-account",
		Status:    common.ChannelStatusEnabled,
		BaseURL:   &accountBaseURL,
	}
	require.NoError(t, db.Create(&account).Error)

	ctx, recorder := createChannelAccountFetchModelsRequest(channel.Id, account.Id)
	FetchChannelAccountUpstreamModels(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	response := decodeChannelAccountFetchModelsResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	require.Equal(t, []string{"gpt-5.5", "claude-3.7"}, response.Data)
	require.Equal(t, "Bearer sk-account", seenAuthorization)
	require.NotContains(t, recorder.Body.String(), "sk-account")
}

func TestFetchChannelAccountUpstreamModelsRejectsInvalidAccount(t *testing.T) {
	db := setupChannelAccountMutationTestDB(t)
	channel := createChannelAccountMutationTestChannel(t, db, `{"allow_service_tier":false}`, "sk-main")
	otherChannel := createChannelAccountMutationTestChannel(t, db, `{"allow_service_tier":false}`, "sk-other")
	otherAccount := model.ChannelAccount{
		ChannelId: otherChannel.Id,
		Name:      "other",
		Key:       "sk-other-account",
		Status:    common.ChannelStatusEnabled,
	}
	require.NoError(t, db.Create(&otherAccount).Error)

	ctx, recorder := createChannelAccountFetchModelsRequest(channel.Id, otherAccount.Id)
	FetchChannelAccountUpstreamModels(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	response := decodeChannelAccountFetchModelsResponse(t, recorder)
	require.False(t, response.Success)
	require.NotEmpty(t, response.Message)
}

func TestFetchChannelAccountUpstreamModelsAllowsDisabledAccountForConfigFetch(t *testing.T) {
	db := setupChannelAccountMutationTestDB(t)
	var seenAuthorization string

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/models", r.URL.Path)
		seenAuthorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"disabled-key-model"}]}`))
	}))
	defer upstream.Close()

	channelBaseURL := upstream.URL
	channel := createChannelAccountMutationTestChannel(t, db, `{"allow_service_tier":false}`, "sk-main")
	channel.BaseURL = &channelBaseURL
	require.NoError(t, db.Save(&channel).Error)
	account := model.ChannelAccount{
		ChannelId: channel.Id,
		Name:      "disabled",
		Key:       "sk-disabled",
		Status:    common.ChannelStatusManuallyDisabled,
	}
	require.NoError(t, db.Create(&account).Error)

	ctx, recorder := createChannelAccountFetchModelsRequest(channel.Id, account.Id)
	FetchChannelAccountUpstreamModels(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	response := decodeChannelAccountFetchModelsResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	require.Equal(t, []string{"disabled-key-model"}, response.Data)
	require.Equal(t, "Bearer sk-disabled", seenAuthorization)
	require.NotContains(t, recorder.Body.String(), "sk-disabled")
}

func TestFetchChannelAccountUpstreamModelsRejectsEmptyKey(t *testing.T) {
	db := setupChannelAccountMutationTestDB(t)
	channel := createChannelAccountMutationTestChannel(t, db, `{"allow_service_tier":false}`, "sk-main")
	account := model.ChannelAccount{
		ChannelId: channel.Id,
		Name:      "empty-key",
		Key:       " ",
		Status:    common.ChannelStatusEnabled,
	}
	require.NoError(t, db.Create(&account).Error)

	ctx, recorder := createChannelAccountFetchModelsRequest(channel.Id, account.Id)
	FetchChannelAccountUpstreamModels(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	response := decodeChannelAccountFetchModelsResponse(t, recorder)
	require.False(t, response.Success)
	require.Contains(t, response.Message, "密钥为空")
}

func TestFetchUpstreamPreviewKeyModelsUsesPreviewCachedSecret(t *testing.T) {
	var seenAuthorization string

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/models", r.URL.Path)
		seenAuthorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"preview-gpt"},{"id":"preview-claude"}]}`))
	}))
	defer upstream.Close()

	preview, err := upstreamaccount.SavePreviewSnapshot(&upstreamaccount.Snapshot{
		Platform: upstreamaccount.PlatformNewAPI,
		BaseURL:  upstream.URL,
		Keys: []upstreamaccount.SyncedKey{{
			ExternalID: "key-a",
			Name:       "preview-key",
			Key:        "sk-preview-secret",
			MaskedKey:  "sk-pre...cret",
		}},
	})
	require.NoError(t, err)

	ctx, recorder := createPreviewKeyFetchModelsRequest(t, preview.PreviewID, gin.H{
		"sync_id":    "key-a",
		"masked_key": "sk-pre...cret",
		"index":      0,
	})
	FetchUpstreamPreviewKeyModels(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	response := decodeChannelAccountFetchModelsResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	require.Equal(t, []string{"preview-gpt", "preview-claude"}, response.Data)
	require.Equal(t, "Bearer sk-preview-secret", seenAuthorization)
	require.NotContains(t, recorder.Body.String(), "sk-preview-secret")
}

func TestFetchUpstreamPreviewKeyModelsRejectsMismatchedKey(t *testing.T) {
	preview, err := upstreamaccount.SavePreviewSnapshot(&upstreamaccount.Snapshot{
		Platform: upstreamaccount.PlatformNewAPI,
		BaseURL:  "https://upstream.example",
		Keys: []upstreamaccount.SyncedKey{{
			ExternalID: "key-a",
			Key:        "sk-preview-secret",
			MaskedKey:  "sk-pre...cret",
		}},
	})
	require.NoError(t, err)

	ctx, recorder := createPreviewKeyFetchModelsRequest(t, preview.PreviewID, gin.H{
		"sync_id":    "key-b",
		"masked_key": "sk-pre...cret",
		"index":      0,
	})
	FetchUpstreamPreviewKeyModels(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	response := decodeChannelAccountFetchModelsResponse(t, recorder)
	require.False(t, response.Success)
	require.Contains(t, response.Message, "预览密钥")
	require.NotContains(t, recorder.Body.String(), "sk-preview-secret")
}
