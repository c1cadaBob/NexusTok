package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type channelMinimumRatioListResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Items []struct {
			ID           int      `json:"id"`
			MinimumRatio *float64 `json:"minimum_ratio"`
		} `json:"items"`
		Total int `json:"total"`
	} `json:"data"`
}

func TestGetAllChannelsSortsByMinimumRatioBeforePagination(t *testing.T) {
	db := setupChannelAccountMutationTestDB(t)
	highPriority := int64(100)
	lowPriority := int64(1)

	channelA := createMinimumRatioListChannel(t, "ratio-050", &highPriority)
	channelB := createMinimumRatioListChannel(t, "ratio-empty", &highPriority)
	channelC := createMinimumRatioListChannel(t, "ratio-020", &lowPriority)
	createMinimumRatioListAccount(t, channelA.Id, 0.5)
	createMinimumRatioListAccount(t, channelC.Id, 0.2)
	require.NoError(t, db.Create(&model.ChannelAccount{
		ChannelId: channelB.Id,
		Name:      "plain-account",
		Key:       "sk-plain",
		Status:    common.ChannelStatusEnabled,
	}).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/?sort_by=minimum_ratio&sort_order=asc&p=1&page_size=2", nil)

	GetAllChannels(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response channelMinimumRatioListResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.Equal(t, 3, response.Data.Total)
	require.Len(t, response.Data.Items, 2)
	require.Equal(t, channelC.Id, response.Data.Items[0].ID)
	require.Equal(t, channelA.Id, response.Data.Items[1].ID)
	require.NotNil(t, response.Data.Items[0].MinimumRatio)
	require.InDelta(t, 0.2, *response.Data.Items[0].MinimumRatio, 0.000001)
}

func createMinimumRatioListChannel(t *testing.T, name string, priority *int64) *model.Channel {
	t.Helper()
	channel := &model.Channel{
		Type:     constant.ChannelTypeOpenAI,
		Key:      "sk-channel",
		Name:     name,
		Status:   common.ChannelStatusEnabled,
		Priority: priority,
	}
	require.NoError(t, model.DB.Create(channel).Error)
	return channel
}

func createMinimumRatioListAccount(t *testing.T, channelID int, ratio float64) {
	t.Helper()
	settingsBytes, err := common.Marshal(map[string]any{
		"upstream_account_sync": map[string]any{
			"platform":         "new-api",
			"base_url":         "https://upstream.example",
			"ratio_conversion": ratio,
			"effective_ratio":  ratio * 2,
		},
	})
	require.NoError(t, err)
	account := &model.ChannelAccount{
		ChannelId:     channelID,
		Name:          "synced-key",
		Key:           "sk-test",
		Status:        common.ChannelStatusEnabled,
		OtherSettings: string(settingsBytes),
	}
	require.NoError(t, model.DB.Create(account).Error)
}
