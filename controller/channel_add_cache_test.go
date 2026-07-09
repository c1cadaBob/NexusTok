package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
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

// setupAddChannelCacheTestDB 初始化新增渠道缓存刷新测试使用的 SQLite 内存库。
// 该测试只需要验证渠道和 ability 写入，不依赖真实 Redis、外部上游或全局缓存状态。
func setupAddChannelCacheTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldUsingSQLite := common.UsingSQLite
	oldUsingMySQL := common.UsingMySQL
	oldUsingPostgreSQL := common.UsingPostgreSQL
	oldRedisEnabled := common.RedisEnabled

	gin.SetMode(gin.TestMode)
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.UsingSQLite = oldUsingSQLite
		common.UsingMySQL = oldUsingMySQL
		common.UsingPostgreSQL = oldUsingPostgreSQL
		common.RedisEnabled = oldRedisEnabled
	})

	return db
}

// withAddChannelCacheRefreshHooks 将新增渠道后的缓存刷新动作替换为计数函数。
// 这样可以直接断言控制器是否只在成功写库后刷新缓存，避免测试依赖全局内存缓存内容。
func withAddChannelCacheRefreshHooks(t *testing.T) (*int, *int) {
	t.Helper()

	oldRefresh := addChannelRefreshChannelCache
	oldReset := addChannelResetProxyClientCache
	refreshCount := 0
	resetCount := 0
	addChannelRefreshChannelCache = func() {
		refreshCount++
	}
	addChannelResetProxyClientCache = func() {
		resetCount++
	}
	t.Cleanup(func() {
		addChannelRefreshChannelCache = oldRefresh
		addChannelResetProxyClientCache = oldReset
	})

	return &refreshCount, &resetCount
}

// newAddChannelTestRequest 构造新增渠道测试请求，并统一使用 common.Marshal 遵守项目 JSON 约定。
func newAddChannelTestRequest(t *testing.T, payload AddChannelRequest) *http.Request {
	t.Helper()

	body, err := common.Marshal(payload)
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPost, "/api/channel/", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	return request
}

// TestAddChannelRefreshesCachesAfterSuccessfulInsert 验证新增渠道写入成功后会刷新分发缓存和代理客户端缓存。
func TestAddChannelRefreshesCachesAfterSuccessfulInsert(t *testing.T) {
	db := setupAddChannelCacheTestDB(t)
	refreshCount, resetCount := withAddChannelCacheRefreshHooks(t)

	weight := uint(1)
	priority := int64(100)
	autoBan := 0
	baseURL := "https://example.com"
	setting := "{}"
	settings := "{}"
	paramOverride := "{}"
	headerOverride := "{}"
	modelMapping := "{}"
	statusCodeMapping := "{}"

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = newAddChannelTestRequest(t, AddChannelRequest{
		Mode: "single",
		Channel: &model.Channel{
			Type:              constant.ChannelTypeOpenAI,
			Key:               "sk-codex-test",
			Status:            common.ChannelStatusEnabled,
			Name:              "codex-cache-refresh-openai",
			Weight:            &weight,
			Priority:          &priority,
			BaseURL:           &baseURL,
			Models:            "gpt-image-1",
			Group:             "default",
			AutoBan:           &autoBan,
			Setting:           &setting,
			OtherSettings:     settings,
			ParamOverride:     &paramOverride,
			HeaderOverride:    &headerOverride,
			ModelMapping:      &modelMapping,
			StatusCodeMapping: &statusCodeMapping,
		},
	})

	AddChannel(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success, response.Message)
	assert.Equal(t, 1, *refreshCount)
	assert.Equal(t, 1, *resetCount)

	var stored model.Channel
	require.NoError(t, db.First(&stored, "name = ?", "codex-cache-refresh-openai").Error)
	assert.Equal(t, "gpt-image-1", stored.Models)
	assert.Equal(t, "default", stored.Group)

	var ability model.Ability
	require.NoError(t, db.Where(&model.Ability{
		ChannelId: stored.Id,
		Model:     "gpt-image-1",
		Group:     "default",
	}).First(&ability).Error)
	assert.True(t, ability.Enabled)
}

// TestAddChannelDoesNotRefreshCachesWhenValidationFails 验证校验失败时不会误刷新缓存。
func TestAddChannelDoesNotRefreshCachesWhenValidationFails(t *testing.T) {
	refreshCount, resetCount := withAddChannelCacheRefreshHooks(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = newAddChannelTestRequest(t, AddChannelRequest{Mode: "single"})

	AddChannel(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
	assert.NotEmpty(t, response.Message)
	assert.Zero(t, *refreshCount)
	assert.Zero(t, *resetCount)
}
