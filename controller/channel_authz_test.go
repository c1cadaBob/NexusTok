package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestChannelHasSensitiveChanges(t *testing.T) {
	baseURL := "https://api.example.com"
	headerOverride := `{"Authorization":"Bearer {api_key}"}`
	origin := &model.Channel{
		Type:           constant.ChannelTypeOpenAI,
		Key:            "old-key",
		BaseURL:        &baseURL,
		HeaderOverride: &headerOverride,
		Models:         "gpt-4o",
		Group:          "default",
	}

	t.Run("普通路由字段不需要敏感写权限", func(t *testing.T) {
		updated := PatchChannel{Channel: *origin}
		updated.Models = "gpt-4o,gpt-4o-mini"
		updated.Group = "vip"

		assert.False(t, channelHasSensitiveChanges(&updated, origin, map[string]any{
			"models": updated.Models,
			"group":  updated.Group,
		}))
	})

	t.Run("密钥变化需要敏感写权限", func(t *testing.T) {
		updated := PatchChannel{Channel: *origin}
		updated.Key = "new-key"

		assert.True(t, channelHasSensitiveChanges(&updated, origin, map[string]any{"key": updated.Key}))
	})

	t.Run("BaseURL 变化需要敏感写权限", func(t *testing.T) {
		updated := PatchChannel{Channel: *origin}
		newBaseURL := "https://upstream.example.com"
		updated.BaseURL = &newBaseURL

		assert.True(t, channelHasSensitiveChanges(&updated, origin, map[string]any{"base_url": newBaseURL}))
	})

	t.Run("请求头覆盖变化需要敏感写权限", func(t *testing.T) {
		updated := PatchChannel{Channel: *origin}
		newHeaderOverride := `{"X-Key":"{api_key}"}`
		updated.HeaderOverride = &newHeaderOverride

		assert.True(t, channelHasSensitiveChanges(&updated, origin, map[string]any{"header_override": newHeaderOverride}))
	})

	t.Run("可选字符串 nil 和空字符串按未设置处理", func(t *testing.T) {
		originWithoutOptional := &model.Channel{
			Type:   constant.ChannelTypeOpenAI,
			Key:    "old-key",
			Models: "gpt-4o",
			Group:  "default",
		}
		updated := PatchChannel{Channel: *originWithoutOptional}
		empty := ""
		updated.BaseURL = &empty
		updated.OpenAIOrganization = &empty
		updated.ParamOverride = &empty

		assert.False(t, channelHasSensitiveChanges(&updated, originWithoutOptional, map[string]any{
			"base_url":            "",
			"openai_organization": "",
			"param_override":      "",
		}))
	})

	t.Run("ChannelInfo 仅缺失运行时字段不需要敏感写权限", func(t *testing.T) {
		originWithInfo := *origin
		originWithInfo.ChannelInfo = model.ChannelInfo{
			IsMultiKey:           true,
			MultiKeySize:         3,
			MultiKeyStatusList:   map[int]int{1: common.ChannelStatusManuallyDisabled},
			MultiKeyPollingIndex: 2,
			MultiKeyMode:         constant.MultiKeyModeRandom,
			CredentialMode:       constant.ChannelCredentialModeMultiKey,
		}
		updated := PatchChannel{Channel: originWithInfo}
		updated.ChannelInfo.MultiKeySize = 0
		updated.ChannelInfo.MultiKeyStatusList = nil
		updated.ChannelInfo.MultiKeyPollingIndex = 0

		assert.False(t, channelHasSensitiveChanges(&updated, &originWithInfo, map[string]any{
			"channel_info": map[string]any{
				"is_multi_key":    true,
				"credential_mode": constant.ChannelCredentialModeMultiKey,
				"multi_key_mode":  constant.MultiKeyModeRandom,
			},
		}))
	})

	t.Run("ChannelInfo 历史空凭证模式按前端规则归一", func(t *testing.T) {
		originWithInfo := *origin
		originWithInfo.ChannelInfo = model.ChannelInfo{
			IsMultiKey:   true,
			MultiKeyMode: "",
		}
		updated := PatchChannel{Channel: originWithInfo}
		updated.ChannelInfo.CredentialMode = constant.ChannelCredentialModeMultiKey
		updated.ChannelInfo.MultiKeyMode = constant.MultiKeyModeRandom

		assert.False(t, channelHasSensitiveChanges(&updated, &originWithInfo, map[string]any{
			"channel_info": map[string]any{
				"is_multi_key":    true,
				"credential_mode": constant.ChannelCredentialModeMultiKey,
				"multi_key_mode":  constant.MultiKeyModeRandom,
			},
		}))
	})

	t.Run("ChannelInfo 凭证模式变化需要敏感写权限", func(t *testing.T) {
		originWithInfo := *origin
		originWithInfo.ChannelInfo = model.ChannelInfo{
			CredentialMode: constant.ChannelCredentialModeSingleKey,
		}
		updated := PatchChannel{Channel: originWithInfo}
		updated.ChannelInfo.CredentialMode = constant.ChannelCredentialModeGlobalAccountPool
		updated.ChannelInfo.AccountPoolGroupId = 9

		assert.True(t, channelHasSensitiveChanges(&updated, &originWithInfo, map[string]any{
			"channel_info": map[string]any{
				"credential_mode":       constant.ChannelCredentialModeGlobalAccountPool,
				"account_pool_group_id": 9,
			},
		}))
	})

	t.Run("未知字段默认按敏感字段处理", func(t *testing.T) {
		updated := PatchChannel{Channel: *origin}

		assert.True(t, channelHasSensitiveChanges(&updated, origin, map[string]any{"future_secret_field": "x"}))
	})

	t.Run("状态字段暂归操作类字段", func(t *testing.T) {
		updated := PatchChannel{Channel: *origin}
		updated.Status = common.ChannelStatusManuallyDisabled

		assert.False(t, channelHasSensitiveChanges(&updated, origin, map[string]any{"status": updated.Status}))
	})

	t.Run("只读统计字段不触发敏感写", func(t *testing.T) {
		updated := PatchChannel{Channel: *origin}
		updated.Balance = 99
		updated.UsedQuota = 100
		updated.ResponseTime = 200

		assert.False(t, channelHasSensitiveChanges(&updated, origin, map[string]any{
			"balance":       updated.Balance,
			"used_quota":    updated.UsedQuota,
			"response_time": updated.ResponseTime,
		}))
	})
}

func TestClearChannelReadOnlyFields(t *testing.T) {
	channel := PatchChannel{Channel: model.Channel{
		CreatedTime:        11,
		TestTime:           22,
		ResponseTime:       33,
		Balance:            44.5,
		BalanceUpdatedTime: 55,
		UsedQuota:          66,
		Models:             "gpt-4o",
		Group:              "default",
		ChannelAccountStats: map[string]int64{
			"enabled": 1,
		},
	}}

	clearChannelReadOnlyFields(&channel, map[string]any{
		"created_time":          channel.CreatedTime,
		"test_time":             channel.TestTime,
		"response_time":         channel.ResponseTime,
		"balance":               channel.Balance,
		"balance_updated_time":  channel.BalanceUpdatedTime,
		"used_quota":            channel.UsedQuota,
		"channel_account_stats": channel.ChannelAccountStats,
		"models":                channel.Models,
		"group":                 channel.Group,
	})

	assert.Zero(t, channel.CreatedTime)
	assert.Zero(t, channel.TestTime)
	assert.Zero(t, channel.ResponseTime)
	assert.Zero(t, channel.Balance)
	assert.Zero(t, channel.BalanceUpdatedTime)
	assert.Zero(t, channel.UsedQuota)
	assert.Nil(t, channel.ChannelAccountStats)
	assert.Equal(t, "gpt-4o", channel.Models)
	assert.Equal(t, "default", channel.Group)
}

func TestUpdateChannelRequiresRootForSensitiveFields(t *testing.T) {
	oldDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() {
		model.DB = oldDB
	})
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))

	channel := &model.Channel{
		Type:   constant.ChannelTypeOpenAI,
		Key:    "old-key",
		Status: common.ChannelStatusEnabled,
		Name:   "openai",
		Models: "gpt-4o",
		Group:  "default",
	}
	require.NoError(t, model.DB.Create(channel).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("role", common.RoleAdminUser)
	ctx.Request = httptest.NewRequest(
		http.MethodPut,
		"/api/channel/",
		bytes.NewBufferString(`{"id":1,"name":"openai","models":"gpt-4o","group":"default","key":"new-key"}`),
	)
	ctx.Request.Header.Set("Content-Type", "application/json")

	UpdateChannel(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)

	var stored model.Channel
	require.NoError(t, model.DB.First(&stored, channel.Id).Error)
	assert.Equal(t, "old-key", stored.Key)
}

func TestUpdateChannelAllowsAdminForNonSensitiveFields(t *testing.T) {
	oldDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() {
		model.DB = oldDB
	})
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))

	priority := int64(1)
	channel := &model.Channel{
		Type:     constant.ChannelTypeOpenAI,
		Key:      "old-key",
		Status:   common.ChannelStatusEnabled,
		Name:     "openai",
		Models:   "gpt-4o",
		Group:    "default",
		Priority: &priority,
	}
	require.NoError(t, channel.Insert())

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("role", common.RoleAdminUser)
	ctx.Request = httptest.NewRequest(
		http.MethodPut,
		"/api/channel/",
		bytes.NewBufferString(`{"id":1,"models":"gpt-4o,gpt-4o-mini","group":"default,vip","priority":5}`),
	)
	ctx.Request.Header.Set("Content-Type", "application/json")

	UpdateChannel(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)

	var stored model.Channel
	require.NoError(t, model.DB.First(&stored, channel.Id).Error)
	assert.Equal(t, "old-key", stored.Key)
	assert.Equal(t, "gpt-4o,gpt-4o-mini", stored.Models)
	assert.Equal(t, "default,vip", stored.Group)
	require.NotNil(t, stored.Priority)
	assert.Equal(t, int64(5), *stored.Priority)
}

func TestUpdateChannelRejectsStatusField(t *testing.T) {
	setupChannelStatusControllerTestDB(t)
	channel := createChannelStatusTestChannel(t, "status-reject", common.ChannelStatusEnabled)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("role", common.RoleRootUser)
	ctx.Request = httptest.NewRequest(
		http.MethodPut,
		"/api/channel/",
		bytes.NewBufferString(`{"id":`+strconv.Itoa(channel.Id)+`,"status":2}`),
	)
	ctx.Request.Header.Set("Content-Type", "application/json")

	UpdateChannel(ctx)

	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)

	var stored model.Channel
	require.NoError(t, model.DB.First(&stored, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusEnabled, stored.Status)
}

func TestUpdateChannelStatusDisablesChannelAndAbilities(t *testing.T) {
	setupChannelStatusControllerTestDB(t)
	channel := createChannelStatusTestChannel(t, "status-single", common.ChannelStatusEnabled)

	ctx, recorder := newChannelStatusContext(t, channel.Id, gin.H{
		"status": common.ChannelStatusManuallyDisabled,
	})
	UpdateChannelStatus(ctx)

	response := decodeChannelStatusResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	require.True(t, response.Data)

	var stored model.Channel
	require.NoError(t, model.DB.First(&stored, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusManuallyDisabled, stored.Status)

	var ability model.Ability
	require.NoError(t, model.DB.Where("channel_id = ? AND model = ?", channel.Id, "gpt-4o").First(&ability).Error)
	assert.False(t, ability.Enabled)
}

func TestUpdateChannelStatusRejectsAutoDisabledStatus(t *testing.T) {
	setupChannelStatusControllerTestDB(t)
	channel := createChannelStatusTestChannel(t, "status-invalid", common.ChannelStatusEnabled)

	ctx, recorder := newChannelStatusContext(t, channel.Id, gin.H{
		"status": common.ChannelStatusAutoDisabled,
	})
	UpdateChannelStatus(ctx)

	response := decodeChannelStatusResponse(t, recorder)
	assert.False(t, response.Success)

	var stored model.Channel
	require.NoError(t, model.DB.First(&stored, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusEnabled, stored.Status)
}

func TestBatchUpdateChannelStatusCountsChangedChannels(t *testing.T) {
	setupChannelStatusControllerTestDB(t)
	channelA := createChannelStatusTestChannel(t, "status-batch-a", common.ChannelStatusEnabled)
	channelB := createChannelStatusTestChannel(t, "status-batch-b", common.ChannelStatusEnabled)
	channelC := createChannelStatusTestChannel(t, "status-batch-c", common.ChannelStatusManuallyDisabled)

	ctx, recorder := newChannelStatusBatchContext(t, gin.H{
		"ids":    []int{channelA.Id, channelB.Id, channelC.Id},
		"status": common.ChannelStatusManuallyDisabled,
	})
	BatchUpdateChannelStatus(ctx)

	response := decodeChannelStatusBatchResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	assert.Equal(t, 2, response.Data)

	for _, channel := range []*model.Channel{channelA, channelB, channelC} {
		var stored model.Channel
		require.NoError(t, model.DB.First(&stored, channel.Id).Error)
		assert.Equal(t, common.ChannelStatusManuallyDisabled, stored.Status)
	}
}

type channelStatusAPIResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    bool   `json:"data"`
}

type channelStatusBatchAPIResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    int    `json:"data"`
}

func setupChannelStatusControllerTestDB(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.Log{}, &model.User{}))
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
		common.RedisEnabled = oldRedisEnabled
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
}

func createChannelStatusTestChannel(t *testing.T, name string, status int) *model.Channel {
	t.Helper()
	channel := &model.Channel{
		Type:   constant.ChannelTypeOpenAI,
		Key:    "test-key",
		Status: status,
		Name:   name,
		Models: "gpt-4o",
		Group:  "default",
	}
	require.NoError(t, channel.Insert())
	return channel
}

func newChannelStatusContext(t *testing.T, channelID int, body any) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	payload, err := common.Marshal(body)
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/"+strconv.Itoa(channelID)+"/status", bytes.NewReader(payload))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(channelID)}}
	ctx.Set("username", "channel-status-tester")
	ctx.Set("id", 1)
	return ctx, recorder
}

func newChannelStatusBatchContext(t *testing.T, body any) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	payload, err := common.Marshal(body)
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/status/batch", bytes.NewReader(payload))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("username", "channel-status-tester")
	ctx.Set("id", 1)
	return ctx, recorder
}

func decodeChannelStatusResponse(t *testing.T, recorder *httptest.ResponseRecorder) channelStatusAPIResponse {
	t.Helper()
	var response channelStatusAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}

func decodeChannelStatusBatchResponse(t *testing.T, recorder *httptest.ResponseRecorder) channelStatusBatchAPIResponse {
	t.Helper()
	var response channelStatusBatchAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}

func TestChannelFieldsAreClassified(t *testing.T) {
	classified := func(name string) bool {
		if _, ok := channelSensitiveFields[name]; ok {
			return true
		}
		if _, ok := channelNonSensitiveFields[name]; ok {
			return true
		}
		if _, ok := channelOperationalFields[name]; ok {
			return true
		}
		_, ok := channelReadOnlyFields[name]
		return ok
	}

	var collect func(rt reflect.Type) []string
	collect = func(rt reflect.Type) []string {
		var names []string
		for i := 0; i < rt.NumField(); i++ {
			field := rt.Field(i)
			if field.Anonymous && field.Type.Kind() == reflect.Struct {
				names = append(names, collect(field.Type)...)
				continue
			}
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if name == "" || name == "-" {
				continue
			}
			names = append(names, name)
		}
		return names
	}

	for _, name := range collect(reflect.TypeOf(PatchChannel{})) {
		assert.Truef(t, classified(name),
			"渠道字段 %q 尚未分类；请在 channel_authz.go 中加入敏感、非敏感、操作或只读字段集合", name)
	}
}
