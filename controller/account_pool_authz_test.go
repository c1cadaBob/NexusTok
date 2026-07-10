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
	"github.com/c1cada/NexusTok/model"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAccountPoolGroupHasSensitiveChanges(t *testing.T) {
	modelMapping := `{"gpt-4o":"codex-mini"}`
	origin := &model.AccountPoolGroup{
		Id:           1,
		Name:         "codex-default",
		Platform:     "codex",
		AuthType:     model.AccountPoolAuthTypeAPIKey,
		Status:       common.ChannelStatusEnabled,
		Models:       "gpt-4o",
		Group:        "default",
		ModelMapping: &modelMapping,
		Settings:     `{"retry_interval":30}`,
	}

	t.Run("普通分组字段不需要敏感写权限", func(t *testing.T) {
		maxConcurrency := 2
		req := accountPoolGroupUpsertRequest{
			Name:           "codex-renamed",
			Models:         "gpt-4o,gpt-4o-mini",
			Group:          "default,vip",
			MaxConcurrency: &maxConcurrency,
		}

		assert.False(t, accountPoolGroupHasSensitiveChanges(req, origin, map[string]any{
			"name":            req.Name,
			"models":          req.Models,
			"group":           req.Group,
			"max_concurrency": maxConcurrency,
		}))
	})

	t.Run("平台变化需要敏感写权限", func(t *testing.T) {
		req := accountPoolGroupUpsertRequest{Platform: "openai"}

		assert.True(t, accountPoolGroupHasSensitiveChanges(req, origin, map[string]any{
			"platform": req.Platform,
		}))
	})

	t.Run("认证类型变化需要敏感写权限", func(t *testing.T) {
		req := accountPoolGroupUpsertRequest{AuthType: model.AccountPoolAuthTypeOfficialOAuth}

		assert.True(t, accountPoolGroupHasSensitiveChanges(req, origin, map[string]any{
			"auth_type": req.AuthType,
		}))
	})

	t.Run("模型映射变化需要敏感写权限", func(t *testing.T) {
		nextMapping := `{"gpt-4o":"codex-pro"}`
		req := accountPoolGroupUpsertRequest{ModelMapping: &nextMapping}

		assert.True(t, accountPoolGroupHasSensitiveChanges(req, origin, map[string]any{
			"model_mapping": nextMapping,
		}))
	})

	t.Run("settings 变化需要敏感写权限", func(t *testing.T) {
		req := accountPoolGroupUpsertRequest{Settings: `{"retry_interval":10}`}

		assert.True(t, accountPoolGroupHasSensitiveChanges(req, origin, map[string]any{
			"settings": req.Settings,
		}))
	})

	t.Run("legacy max_concurrency 清理不被误判为 settings 敏感变更", func(t *testing.T) {
		zero := 0
		legacyOrigin := *origin
		legacyOrigin.Settings = `{"max_concurrency":2,"retry_interval":30}`
		req := accountPoolGroupUpsertRequest{
			Settings:       legacyOrigin.Settings,
			MaxConcurrency: &zero,
		}

		assert.False(t, accountPoolGroupHasSensitiveChanges(req, &legacyOrigin, map[string]any{
			"settings":        req.Settings,
			"max_concurrency": zero,
		}))
	})

	t.Run("未知字段默认按敏感字段处理", func(t *testing.T) {
		req := accountPoolGroupUpsertRequest{Name: origin.Name}

		assert.True(t, accountPoolGroupHasSensitiveChanges(req, origin, map[string]any{
			"future_secret_field": "x",
		}))
	})

	t.Run("只读响应字段不触发敏感写", func(t *testing.T) {
		req := accountPoolGroupUpsertRequest{Name: origin.Name}

		assert.False(t, accountPoolGroupHasSensitiveChanges(req, origin, map[string]any{
			"id":                origin.Id,
			"used_quota":        int64(100),
			"daily_limit_state": map[string]any{"limited": false},
			"stats":             map[string]any{"enabled": 1},
		}))
	})
}

func TestCompleteAccountPoolGroupUpdateRequestKeepsOmittedTextFields(t *testing.T) {
	origin := &model.AccountPoolGroup{
		Models:   "gpt-4o",
		Group:    "default",
		Settings: `{"retry_interval":30}`,
	}
	req := accountPoolGroupUpsertRequest{Name: "renamed"}

	completeAccountPoolGroupUpdateRequest(&req, origin, map[string]any{"name": req.Name})

	assert.Equal(t, origin.Models, req.Models)
	assert.Equal(t, origin.Group, req.Group)
	assert.Equal(t, origin.Settings, req.Settings)
}

func TestUpdateAccountPoolGroupRequiresSensitiveWriteForSensitiveFields(t *testing.T) {
	setupAccountPoolGroupAuthzTestDB(t)

	group := &model.AccountPoolGroup{
		Name:     "codex-default",
		Platform: "codex",
		AuthType: model.AccountPoolAuthTypeAPIKey,
		Status:   common.ChannelStatusEnabled,
		Models:   "gpt-4o",
		Group:    "default",
	}
	require.NoError(t, model.DB.Create(group).Error)

	recorder := performUpdateAccountPoolGroupRequest(t, group.Id, common.RoleAdminUser, `{"platform":"openai","name":"codex-default"}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)

	var stored model.AccountPoolGroup
	require.NoError(t, model.DB.First(&stored, group.Id).Error)
	assert.Equal(t, "codex", stored.Platform)
	assert.Equal(t, "codex-default", stored.Name)
}

func TestUpdateAccountPoolGroupAllowsAdminForNonSensitiveFields(t *testing.T) {
	setupAccountPoolGroupAuthzTestDB(t)

	group := &model.AccountPoolGroup{
		Name:           "codex-default",
		Platform:       "codex",
		AuthType:       model.AccountPoolAuthTypeAPIKey,
		Status:         common.ChannelStatusEnabled,
		Models:         "gpt-4o",
		Group:          "default",
		MaxConcurrency: 1,
	}
	require.NoError(t, model.DB.Create(group).Error)

	recorder := performUpdateAccountPoolGroupRequest(
		t,
		group.Id,
		common.RoleAdminUser,
		`{"name":"codex-vip","models":"gpt-4o,gpt-4o-mini","group":"default,vip","max_concurrency":3}`,
	)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)

	var stored model.AccountPoolGroup
	require.NoError(t, model.DB.First(&stored, group.Id).Error)
	assert.Equal(t, "codex", stored.Platform)
	assert.Equal(t, model.AccountPoolAuthTypeAPIKey, stored.AuthType)
	assert.Equal(t, "codex-vip", stored.Name)
	assert.Equal(t, "gpt-4o,gpt-4o-mini", stored.Models)
	assert.Equal(t, "default,vip", stored.Group)
	assert.Equal(t, 3, stored.MaxConcurrency)
}

func TestAccountPoolGroupUpsertFieldsAreClassified(t *testing.T) {
	classified := func(name string) bool {
		if _, ok := accountPoolGroupSensitiveFields[name]; ok {
			return true
		}
		if _, ok := accountPoolGroupNonSensitiveFields[name]; ok {
			return true
		}
		_, ok := accountPoolGroupReadOnlyFields[name]
		return ok
	}

	rt := reflect.TypeOf(accountPoolGroupUpsertRequest{})
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		if name == "" || name == "-" {
			continue
		}
		assert.Truef(t, classified(name),
			"账号池分组字段 %q 尚未分类；请在 account_pool_authz.go 中加入敏感、非敏感或只读字段集合", name)
	}
}

func setupAccountPoolGroupAuthzTestDB(t *testing.T) {
	t.Helper()
	oldDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() {
		model.DB = oldDB
	})
	require.NoError(t, db.AutoMigrate(&model.AccountPoolGroup{}))
}

func performUpdateAccountPoolGroupRequest(t *testing.T, groupID int, role int, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("role", role)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(groupID)}}
	ctx.Request = httptest.NewRequest(
		http.MethodPut,
		"/api/account-pool/groups/"+strconv.Itoa(groupID),
		bytes.NewBufferString(body),
	)
	ctx.Request.Header.Set("Content-Type", "application/json")

	UpdateAccountPoolGroup(ctx)
	return recorder
}
