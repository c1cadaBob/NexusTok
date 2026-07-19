package controller

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/middleware"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/service"
	"github.com/c1cada/NexusTok/setting/operation_setting"
	"github.com/c1cada/NexusTok/setting/ratio_setting"
	"github.com/c1cada/NexusTok/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestShouldRetryDoesNotRetrySpecificChannelForChannelError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	c.Set("specific_channel_id", "1")
	err := types.NewError(errors.New("channel invalid key"), types.ErrorCodeChannelInvalidKey, types.ErrOptionWithStatusCode(http.StatusUnauthorized))

	require.False(t, shouldRetry(c, err, 1))
}

func TestRelayRetriesToNextChannelAfterChannelFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupRelayRetryFallbackTestState(t)

	firstUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/chat/completions", r.URL.Path)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"first channel rejected key","type":"invalid_request_error","code":"invalid_api_key"}}`))
	}))
	defer firstUpstream.Close()

	secondUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/chat/completions", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-retry-ok","object":"chat.completion","created":1,"model":"gpt-retry-relay","choices":[{"index":0,"message":{"role":"assistant","content":"fallback-ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer secondUpstream.Close()

	channelOne := createRelayRetryFallbackChannel(t, 1, "first", firstUpstream.URL, 20)
	createRelayRetryFallbackChannel(t, 2, "second", secondUpstream.URL, 10)
	model.InitChannelCache()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := []byte(`{"model":"gpt-retry-relay","messages":[{"role":"user","content":"ping"}]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	storage, err := common.CreateBodyStorage(body)
	require.NoError(t, err)
	c.Set(common.KeyBodyStorage, storage)
	defer common.CleanupBodyStorage(c)
	setRelayRetryFallbackRequestContext(c)

	// 模拟 Distribute 中间件首次选中的高优先级渠道；Relay 内部重试必须自行排除失败渠道并重选。
	require.Nil(t, middleware.SetupContextForSelectedChannel(c, channelOne, "gpt-retry-relay"))

	Relay(c, types.RelayFormatOpenAI)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "fallback-ok")
	require.Equal(t, []string{"1", "2"}, c.GetStringSlice("use_channel"))
	require.Equal(t, []int{1}, service.GetExcludedChannelIds(c))

	var firstStored model.Channel
	require.NoError(t, model.DB.First(&firstStored, 1).Error)
	require.Equal(t, common.ChannelStatusEnabled, firstStored.Status)
}

func TestRelayRetriesToNextChannelAccountAfterSyncedKeyFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupRelayRetryFallbackTestState(t)

	firstUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/chat/completions", r.URL.Path)
		require.Equal(t, "Bearer sk-account-fail", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"synced key rejected","type":"invalid_request_error","code":"invalid_api_key"}}`))
	}))
	defer firstUpstream.Close()

	secondUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/chat/completions", r.URL.Path)
		require.Equal(t, "Bearer sk-account-ok", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-account-retry-ok","object":"chat.completion","created":1,"model":"gpt-retry-relay","choices":[{"index":0,"message":{"role":"assistant","content":"account-fallback-ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer secondUpstream.Close()

	channel := createRelayRetryFallbackAccountPoolChannel(t, 1, "synced-account-channel")
	createRelayRetryFallbackChannelAccount(t, channel.Id, "first", "sk-account-fail", firstUpstream.URL, 20, 100)
	createRelayRetryFallbackChannelAccount(t, channel.Id, "second", "sk-account-ok", secondUpstream.URL, 10, 100)
	model.InitChannelCache()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := []byte(`{"model":"gpt-retry-relay","messages":[{"role":"user","content":"ping"}]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	storage, err := common.CreateBodyStorage(body)
	require.NoError(t, err)
	c.Set(common.KeyBodyStorage, storage)
	defer common.CleanupBodyStorage(c)
	setRelayRetryFallbackRequestContext(c)

	require.Nil(t, middleware.SetupContextForSelectedChannel(c, channel, "gpt-retry-relay"))

	Relay(c, types.RelayFormatOpenAI)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "account-fallback-ok")
	require.Equal(t, []string{"1", "1"}, c.GetStringSlice("use_channel"))
	require.Empty(t, service.GetExcludedChannelIds(c))
	require.Len(t, service.GetExcludedChannelAccountIds(c), 1)

	var firstStored model.ChannelAccount
	require.NoError(t, model.DB.Where("channel_id = ? AND name = ?", channel.Id, "first").First(&firstStored).Error)
	require.Equal(t, common.ChannelStatusEnabled, firstStored.Status)
	require.Contains(t, firstStored.LastError, "synced key rejected")
	require.True(t, service.GetExcludedChannelAccountIds(c)[firstStored.Id])
}

func TestPrepareRelayChannelContextFallsBackWhenInitialAccountPoolUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupRelayRetryFallbackTestState(t)

	accountPoolChannel := createRelayRetryFallbackAccountPoolChannel(t, 1, "cooling-account-channel")
	createRelayRetryFallbackCoolingChannelAccount(t, accountPoolChannel.Id, "cooling", "sk-cooling", 20, 100)
	fallbackChannel := createRelayRetryFallbackChannel(t, 2, "fallback", "https://fallback.example.test", 10)
	model.InitChannelCache()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := []byte(`{"model":"gpt-retry-relay","messages":[{"role":"user","content":"ping"}]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	storage, err := common.CreateBodyStorage(body)
	require.NoError(t, err)
	c.Set(common.KeyBodyStorage, storage)
	defer common.CleanupBodyStorage(c)
	setRelayRetryFallbackRequestContext(c)

	selected, ok := middleware.PrepareRelayChannelContext(c)

	require.True(t, ok)
	require.NotNil(t, selected)
	require.Equal(t, fallbackChannel.Id, selected.Id)
	require.Equal(t, []int{accountPoolChannel.Id}, service.GetExcludedChannelIds(c))
	require.Equal(t, fallbackChannel.Id, common.GetContextKeyInt(c, constant.ContextKeyChannelId))
}

func setupRelayRetryFallbackTestState(t *testing.T) {
	t.Helper()

	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	oldRedisEnabled := common.RedisEnabled
	oldBatchUpdateEnabled := common.BatchUpdateEnabled
	oldLogConsumeEnabled := common.LogConsumeEnabled
	oldCountToken := constant.CountToken
	oldRetryTimes := common.RetryTimes
	oldQuotaSetting := *operation_setting.GetQuotaSetting()
	oldModelRatio := ratio_setting.GetModelRatioCopy()
	oldGroupRatio := ratio_setting.GetGroupRatioCopy()
	oldModelRatioJSON, err := common.Marshal(oldModelRatio)
	require.NoError(t, err)
	oldGroupRatioJSON, err := common.Marshal(oldGroupRatio)
	require.NoError(t, err)

	common.MemoryCacheEnabled = true
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	common.LogConsumeEnabled = false
	constant.CountToken = false
	common.RetryTimes = 2
	operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = false
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-retry-relay":0}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	service.InitHttpClient()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.User{}, &model.Token{}, &model.Log{}, &model.ChannelAccount{}))
	model.DB = db
	model.LOG_DB = db

	require.NoError(t, db.Create(&model.User{
		Id:       1,
		Username: "relay-retry-user",
		Password: "not-used-in-test",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
		Quota:    1_000_000,
	}).Error)
	require.NoError(t, db.Create(&model.Token{
		Id:             1,
		UserId:         1,
		Key:            "relay-retry-token",
		Status:         common.TokenStatusEnabled,
		Name:           "relay-retry-token",
		RemainQuota:    1_000_000,
		UnlimitedQuota: true,
		Group:          "default",
	}).Error)

	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
		common.RedisEnabled = oldRedisEnabled
		common.BatchUpdateEnabled = oldBatchUpdateEnabled
		common.LogConsumeEnabled = oldLogConsumeEnabled
		constant.CountToken = oldCountToken
		common.RetryTimes = oldRetryTimes
		*operation_setting.GetQuotaSetting() = oldQuotaSetting
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(oldModelRatioJSON)))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(string(oldGroupRatioJSON)))
	})
}

func createRelayRetryFallbackChannel(t *testing.T, id int, name string, baseURL string, priority int64) *model.Channel {
	t.Helper()

	weight := uint(100)
	autoBan := 0
	channel := &model.Channel{
		Id:       id,
		Type:     constant.ChannelTypeOpenAI,
		Key:      "sk-test",
		Status:   common.ChannelStatusEnabled,
		Name:     name,
		Weight:   &weight,
		BaseURL:  &baseURL,
		Models:   "gpt-retry-relay",
		Group:    "default",
		Priority: &priority,
		AutoBan:  &autoBan,
	}
	require.NoError(t, model.DB.Create(channel).Error)
	require.NoError(t, model.DB.Create(&model.Ability{
		Group:     "default",
		Model:     "gpt-retry-relay",
		ChannelId: id,
		Enabled:   true,
		Priority:  &priority,
		Weight:    weight,
	}).Error)
	return channel
}

func createRelayRetryFallbackAccountPoolChannel(t *testing.T, id int, name string) *model.Channel {
	t.Helper()

	weight := uint(100)
	priority := int64(20)
	autoBan := 0
	channel := &model.Channel{
		Id:       id,
		Type:     constant.ChannelTypeOpenAI,
		Key:      constant.ChannelCredentialModeAccountPool,
		Status:   common.ChannelStatusEnabled,
		Name:     name,
		Weight:   &weight,
		Models:   "gpt-retry-relay",
		Group:    "default",
		Priority: &priority,
		AutoBan:  &autoBan,
		ChannelInfo: model.ChannelInfo{
			CredentialMode:     constant.ChannelCredentialModeAccountPool,
			AccountPoolEnabled: true,
			AccountPoolMode:    constant.ChannelAccountPoolModePolling,
		},
	}
	require.NoError(t, model.DB.Create(channel).Error)
	require.NoError(t, model.DB.Create(&model.Ability{
		Group:     "default",
		Model:     "gpt-retry-relay",
		ChannelId: id,
		Enabled:   true,
		Priority:  &priority,
		Weight:    weight,
	}).Error)
	return channel
}

func createRelayRetryFallbackChannelAccount(t *testing.T, channelID int, name string, key string, baseURL string, priority int64, weight int) {
	t.Helper()

	require.NoError(t, model.DB.Create(&model.ChannelAccount{
		ChannelId: channelID,
		Name:      name,
		Key:       key,
		Status:    common.ChannelStatusEnabled,
		Models:    "gpt-retry-relay",
		Group:     "default",
		BaseURL:   &baseURL,
		Priority:  priority,
		Weight:    weight,
	}).Error)
}

func createRelayRetryFallbackCoolingChannelAccount(t *testing.T, channelID int, name string, key string, priority int64, weight int) {
	t.Helper()

	require.NoError(t, model.DB.Create(&model.ChannelAccount{
		ChannelId:        channelID,
		Name:             name,
		Key:              key,
		Status:           common.ChannelStatusEnabled,
		Models:           "gpt-retry-relay",
		Group:            "default",
		Priority:         priority,
		Weight:           weight,
		RateLimitedUntil: common.GetTimestamp() + 300,
	}).Error)
}

func setRelayRetryFallbackRequestContext(c *gin.Context) {
	common.SetContextKey(c, constant.ContextKeyUserId, 1)
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
	common.SetContextKey(c, constant.ContextKeyUserQuota, 1_000_000)
	common.SetContextKey(c, constant.ContextKeyUserEmail, "relay-retry@example.test")
	common.SetContextKey(c, constant.ContextKeyTokenId, 1)
	common.SetContextKey(c, constant.ContextKeyTokenKey, "relay-retry-token")
	common.SetContextKey(c, constant.ContextKeyTokenGroup, "default")
	common.SetContextKey(c, constant.ContextKeyTokenUnlimited, true)
	common.SetContextKey(c, constant.ContextKeyTokenModelLimitEnabled, false)
	common.SetContextKey(c, constant.ContextKeyOriginalModel, "gpt-retry-relay")
	common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{})
	c.Set("token_name", "relay-retry-token")
	c.Set("username", "relay-retry-user")
}
