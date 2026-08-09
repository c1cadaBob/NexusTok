package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"

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

func TestFetchChannelAccountUpstreamModelsRejectsDisabledOrEmptyKey(t *testing.T) {
	db := setupChannelAccountMutationTestDB(t)
	channel := createChannelAccountMutationTestChannel(t, db, `{"allow_service_tier":false}`, "sk-main")
	cases := []struct {
		name    string
		account model.ChannelAccount
		want    string
	}{
		{
			name: "禁用账号",
			account: model.ChannelAccount{
				ChannelId: channel.Id,
				Name:      "disabled",
				Key:       "sk-disabled",
				Status:    common.ChannelStatusManuallyDisabled,
			},
			want: "未启用",
		},
		{
			name: "空密钥",
			account: model.ChannelAccount{
				ChannelId: channel.Id,
				Name:      "empty-key",
				Key:       " ",
				Status:    common.ChannelStatusEnabled,
			},
			want: "密钥为空",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			account := tc.account
			require.NoError(t, db.Create(&account).Error)

			ctx, recorder := createChannelAccountFetchModelsRequest(channel.Id, account.Id)
			FetchChannelAccountUpstreamModels(ctx)

			require.Equal(t, http.StatusOK, recorder.Code)
			response := decodeChannelAccountFetchModelsResponse(t, recorder)
			require.False(t, response.Success)
			require.Contains(t, response.Message, tc.want)
		})
	}
}
