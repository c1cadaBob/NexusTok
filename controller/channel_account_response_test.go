package controller

import (
	"net/http/httptest"
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestChannelAccountResponseHidesSensitiveConfigForReadOnlyAdmin(t *testing.T) {
	previousDB := model.DB
	model.DB = nil
	t.Cleanup(func() {
		model.DB = previousDB
	})

	ctx := channelAccountResponseTestContext(common.RoleAdminUser)
	response := channelAccountResponseForContext(ctx, channelAccountResponseFixture())

	require.Equal(t, "sk-t**********7890", response["key"])
	require.Equal(t, "default", response["group"])
	require.Contains(t, response, "rate_limited_until")

	for _, key := range []string{
		"base_url",
		"openai_organization",
		"other",
		"setting",
		"settings",
		"model_mapping",
		"param_override",
		"header_override",
		"status_code_mapping",
		"last_error",
	} {
		require.NotContains(t, response, key)
	}
}

func TestChannelAccountResponseIncludesSensitiveConfigForRoot(t *testing.T) {
	previousDB := model.DB
	model.DB = nil
	t.Cleanup(func() {
		model.DB = previousDB
	})

	ctx := channelAccountResponseTestContext(common.RoleRootUser)
	response := channelAccountResponseForContext(ctx, channelAccountResponseFixture())

	require.Equal(t, "sk-t**********7890", response["key"])
	require.Equal(t, "org-example", derefStringFromResponse(t, response, "openai_organization"))
	require.Equal(t, "https://upstream.example.com", derefStringFromResponse(t, response, "base_url"))
	require.Equal(t, `{"client":"upstream"}`, derefStringFromResponse(t, response, "model_mapping"))
	require.Equal(t, `{"Authorization":"Bearer internal"}`, derefStringFromResponse(t, response, "header_override"))
	require.Equal(t, "raw upstream error with private endpoint", response["last_error"])
}

func channelAccountResponseTestContext(role int) *gin.Context {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 1)
	ctx.Set("role", role)
	return ctx
}

func channelAccountResponseFixture() *model.ChannelAccount {
	baseURL := "https://upstream.example.com"
	organization := "org-example"
	setting := `{"provider":"secret"}`
	modelMapping := `{"client":"upstream"}`
	paramOverride := `{"temperature":0}`
	headerOverride := `{"Authorization":"Bearer internal"}`
	statusCodeMapping := `{"429":500}`

	return &model.ChannelAccount{
		Id:                 7,
		ChannelId:          1,
		Name:               "primary",
		Key:                "sk-test-1234567890",
		Status:             common.ChannelStatusEnabled,
		Models:             "gpt-5.6-sol",
		Group:              "default",
		Priority:           10,
		Weight:             2,
		LastUsedTime:       1700000000,
		UsedQuota:          123,
		BaseURL:            &baseURL,
		OpenAIOrganization: &organization,
		Other:              "region-private",
		Setting:            &setting,
		OtherSettings:      `{"allow":"internal"}`,
		ModelMapping:       &modelMapping,
		ParamOverride:      &paramOverride,
		HeaderOverride:     &headerOverride,
		StatusCodeMapping:  &statusCodeMapping,
		RateLimitedUntil:   1700000100,
		OverloadUntil:      1700000200,
		TempDisabledUntil:  1700000300,
		DisabledReason:     "manual maintenance",
		LastError:          "raw upstream error with private endpoint",
		MaxConcurrency:     4,
		CreatedTime:        1699999900,
	}
}

func derefStringFromResponse(t *testing.T, response gin.H, key string) string {
	t.Helper()
	value, ok := response[key]
	require.True(t, ok)
	ptr, ok := value.(*string)
	require.True(t, ok)
	require.NotNil(t, ptr)
	return *ptr
}
