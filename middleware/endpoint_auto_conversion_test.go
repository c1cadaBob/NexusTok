package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/setting/model_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPrepareEndpointAutoConversionChatToResponses(t *testing.T) {
	setupEndpointAutoConversionTestDB(t, "responses-only", map[constant.EndpointType]string{
		constant.EndpointTypeOpenAIResponse: "/v1/responses",
	})

	c, _ := endpointAutoConversionTestContext("/v1/chat/completions", `{"model":"responses-only","messages":[]}`)
	modelRequest, shouldSelect, err := getModelRequest(c)
	require.NoError(t, err)
	require.True(t, shouldSelect)

	require.True(t, prepareEndpointAutoConversion(c, modelRequest))
	conversion, ok := relaycommon.GetEndpointAutoConversion(c)
	require.True(t, ok)
	require.Equal(t, constant.EndpointTypeOpenAI, conversion.FromEndpoint)
	require.Equal(t, constant.EndpointTypeOpenAIResponse, conversion.ToEndpoint)
	require.Equal(t, "/v1/responses", relaycommon.EffectiveRequestPath(c))
}

func TestPrepareEndpointAutoConversionCanBeDisabled(t *testing.T) {
	setupEndpointAutoConversionTestDB(t, "responses-only-disabled", map[constant.EndpointType]string{
		constant.EndpointTypeOpenAIResponse: "/v1/responses",
	})

	c, _ := endpointAutoConversionTestContext("/v1/chat/completions", `{"model":"responses-only-disabled","messages":[]}`)
	c.Request.Header.Set(endpointAutoConvertDisableHeader, "true")
	modelRequest, _, err := getModelRequest(c)
	require.NoError(t, err)

	require.True(t, prepareEndpointAutoConversion(c, modelRequest))
	_, ok := relaycommon.GetEndpointAutoConversion(c)
	require.False(t, ok)
	require.Equal(t, "/v1/chat/completions", relaycommon.EffectiveRequestPath(c))
}

func TestPrepareEndpointAutoConversionCanBeDisabledByGlobalSetting(t *testing.T) {
	setupEndpointAutoConversionTestDB(t, "responses-only-global-disabled", map[constant.EndpointType]string{
		constant.EndpointTypeOpenAIResponse: "/v1/responses",
	})
	oldEnabled := model_setting.GetGlobalSettings().EndpointAutoConversionEnabled
	model_setting.GetGlobalSettings().EndpointAutoConversionEnabled = false
	t.Cleanup(func() {
		model_setting.GetGlobalSettings().EndpointAutoConversionEnabled = oldEnabled
	})

	c, _ := endpointAutoConversionTestContext("/v1/chat/completions", `{"model":"responses-only-global-disabled","messages":[]}`)
	modelRequest, _, err := getModelRequest(c)
	require.NoError(t, err)

	require.True(t, prepareEndpointAutoConversion(c, modelRequest))
	_, ok := relaycommon.GetEndpointAutoConversion(c)
	require.False(t, ok)
	require.Equal(t, "/v1/chat/completions", relaycommon.EffectiveRequestPath(c))
}

func TestPrepareEndpointAutoConversionRejectsUnsafeEndpointMismatch(t *testing.T) {
	setupEndpointAutoConversionTestDB(t, "image-only", map[constant.EndpointType]string{
		constant.EndpointTypeImageGeneration: "/v1/images/generations",
	})

	c, recorder := endpointAutoConversionTestContext("/v1/chat/completions", `{"model":"image-only","messages":[]}`)
	modelRequest, _, err := getModelRequest(c)
	require.NoError(t, err)

	require.False(t, prepareEndpointAutoConversion(c, modelRequest))
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "/v1/images/generations")
}

func setupEndpointAutoConversionTestDB(t *testing.T, modelName string, endpoints map[constant.EndpointType]string) {
	t.Helper()
	oldDB := model.DB
	db, err := gorm.Open(sqlite.Open("file:endpoint_auto_conversion_"+common.GetUUID()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.Model{}, &model.Vendor{}))
	model.DB = db
	model.InvalidatePricingCache()
	t.Cleanup(func() {
		model.DB = oldDB
		model.InvalidatePricingCache()
	})

	require.NoError(t, db.Create(&model.Channel{
		Id:     1,
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-test",
		Status: common.ChannelStatusEnabled,
		Name:   "endpoint-auto-conversion",
		Group:  "default",
		Models: modelName,
	}).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     modelName,
		ChannelId: 1,
		Enabled:   true,
	}).Error)

	endpointMap := make(map[string]interface{}, len(endpoints))
	for endpoint, path := range endpoints {
		endpointMap[string(endpoint)] = path
	}
	endpointJSON, err := common.Marshal(endpointMap)
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.Model{
		ModelName: modelName,
		Endpoints: string(endpointJSON),
		Status:    1,
		NameRule:  model.NameRuleExact,
	}).Error)
}

func endpointAutoConversionTestContext(path string, body string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, recorder
}
