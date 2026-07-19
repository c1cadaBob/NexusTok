package common

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/c1cada/NexusTok/constant"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestGetFullRequestURLNormalizesSlashes 验证渠道 base_url 带尾斜杠时不会拼出双斜杠路径。
func TestGetFullRequestURLNormalizesSlashes(t *testing.T) {
	tests := []struct {
		name        string
		baseURL     string
		requestURL  string
		channelType int
		want        string
	}{
		{
			name:        "openai compatible base url with trailing slash",
			baseURL:     "http://example.test/",
			requestURL:  "/v1/chat/completions",
			channelType: constant.ChannelTypeOpenAI,
			want:        "http://example.test/v1/chat/completions",
		},
		{
			name:        "request path without leading slash",
			baseURL:     "http://example.test///",
			requestURL:  "v1/chat/completions?stream=false",
			channelType: constant.ChannelTypeOpenAI,
			want:        "http://example.test/v1/chat/completions?stream=false",
		},
		{
			name:        "cloudflare gateway still removes openai v1 prefix",
			baseURL:     "https://gateway.ai.cloudflare.com/v1/account/gateway/",
			requestURL:  "/v1/chat/completions",
			channelType: constant.ChannelTypeOpenAI,
			want:        "https://gateway.ai.cloudflare.com/v1/account/gateway/chat/completions",
		},
		{
			name:        "cloudflare gateway still removes azure deployments prefix",
			baseURL:     "https://gateway.ai.cloudflare.com/v1/account/gateway/",
			requestURL:  "/openai/deployments/gpt-4o/chat/completions?api-version=2024-10-21",
			channelType: constant.ChannelTypeAzure,
			want:        "https://gateway.ai.cloudflare.com/v1/account/gateway/gpt-4o/chat/completions?api-version=2024-10-21",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, GetFullRequestURL(tt.baseURL, tt.requestURL, tt.channelType))
		})
	}
}

// TestValidateMultipartDirectNormalizesImageField 验证直连视频 JSON 的单图 image 字段会参与图生视频动作判断。
func TestValidateMultipartDirectNormalizesImageField(t *testing.T) {
	gin.SetMode(gin.TestMode)

	c, info := newTaskValidationContextForTest(t, `{"model":"wan2.7-i2v","prompt":"animate","image":" https://example.com/first.png "}`)

	taskErr := ValidateMultipartDirect(c, info)

	require.Nil(t, taskErr)
	storedReq, err := GetTaskRequest(c)
	require.NoError(t, err)
	require.Equal(t, []string{"https://example.com/first.png"}, storedReq.Images)
	require.Equal(t, constant.TaskActionGenerate, info.Action)
}

// TestTaskDurationBounds 验证用户可控视频时长不会越过任务计费倍率边界。
func TestTaskDurationBounds(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{
			name:    "huge duration is rejected",
			body:    `{"model":"sora-2","prompt":"a cat","duration":9999999999}`,
			wantErr: true,
		},
		{
			name:    "huge seconds string is rejected",
			body:    `{"model":"sora-2","prompt":"a cat","seconds":"9999999999"}`,
			wantErr: true,
		},
		{
			name:    "negative duration is rejected",
			body:    `{"model":"sora-2","prompt":"a cat","duration":-8}`,
			wantErr: true,
		},
		{
			name: "normal duration is accepted",
			body: `{"model":"sora-2","prompt":"a cat","seconds":"8"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name+" direct", func(t *testing.T) {
			c, info := newTaskValidationContextForTest(t, tt.body)
			taskErr := ValidateMultipartDirect(c, info)
			if tt.wantErr {
				require.NotNil(t, taskErr)
				require.Equal(t, "invalid_seconds", taskErr.Code)
				return
			}
			require.Nil(t, taskErr)
		})

		t.Run(tt.name+" basic", func(t *testing.T) {
			c, info := newTaskValidationContextForTest(t, tt.body)
			taskErr := ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate)
			if tt.wantErr {
				require.NotNil(t, taskErr)
				require.Equal(t, "invalid_seconds", taskErr.Code)
				return
			}
			require.Nil(t, taskErr)
		})
	}
}

// newTaskValidationContextForTest 构造带 JSON 请求体的任务校验上下文。
func newTaskValidationContextForTest(t *testing.T, body string) (*gin.Context, *RelayInfo) {
	t.Helper()

	request := httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = request
	return c, &RelayInfo{TaskRelayInfo: &TaskRelayInfo{}}
}
