package dto

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdvancedCustomValidateAcceptsValidRoutes(t *testing.T) {
	config := &AdvancedCustomConfig{
		Routes: []AdvancedCustomRoute{
			{
				IncomingPath: "/v1/chat/completions",
				UpstreamPath: "https://upstream.example/v1/responses",
				Converter:    AdvancedCustomConverterOpenAIChatCompletionsToOpenAIResponses,
				Auth: &AdvancedCustomRouteAuth{
					Type:  AdvancedCustomAuthTypeHeader,
					Name:  "x-api-key",
					Value: "{api_key}",
				},
			},
			{
				IncomingPath: "/v1beta/models/{model}:generateContent",
				UpstreamPath: "/v1/chat/completions",
				Converter:    AdvancedCustomConverterGeminiGenerateContentToOpenAIChatCompletions,
			},
		},
	}

	require.NoError(t, config.Validate())
	assert.Equal(t, AdvancedCustomConverterOpenAIChatCompletionsToOpenAIResponses, config.Routes[0].Converter)
}

func TestAdvancedCustomValidateDefaultsEmptyConverter(t *testing.T) {
	config := &AdvancedCustomConfig{
		Routes: []AdvancedCustomRoute{
			{
				IncomingPath: "/v1/chat/completions",
				UpstreamPath: "https://upstream.example/v1/chat/completions",
			},
		},
	}

	require.NoError(t, config.Validate())
	assert.Equal(t, AdvancedCustomConverterNone, config.Routes[0].Converter)
}

func TestAdvancedCustomValidateRejectsUnsafeRoutes(t *testing.T) {
	tests := []struct {
		name        string
		route       AdvancedCustomRoute
		wantMessage string
	}{
		{
			name: "incoming path must start with slash",
			route: AdvancedCustomRoute{
				IncomingPath: "v1/chat/completions",
				UpstreamPath: "https://upstream.example/v1/chat/completions",
			},
			wantMessage: "incoming_path must start with /",
		},
		{
			name: "incoming path must not include query",
			route: AdvancedCustomRoute{
				IncomingPath: "/v1/chat/completions?debug=1",
				UpstreamPath: "https://upstream.example/v1/chat/completions",
			},
			wantMessage: "incoming_path must not include query",
		},
		{
			name: "protocol relative upstream is rejected",
			route: AdvancedCustomRoute{
				IncomingPath: "/v1/chat/completions",
				UpstreamPath: "//upstream.example/v1/chat/completions",
			},
			wantMessage: "upstream_path must be a full URL or a path starting with /",
		},
		{
			name: "unsupported upstream scheme is rejected",
			route: AdvancedCustomRoute{
				IncomingPath: "/v1/chat/completions",
				UpstreamPath: "file:///etc/passwd",
			},
			wantMessage: "upstream_path must be a full URL or a path starting with /",
		},
		{
			name: "upstream user info is rejected",
			route: AdvancedCustomRoute{
				IncomingPath: "/v1/chat/completions",
				UpstreamPath: "https://user:pass@upstream.example/v1/chat/completions",
			},
			wantMessage: "upstream_path must not include user info",
		},
		{
			name: "converter must match incoming path",
			route: AdvancedCustomRoute{
				IncomingPath: "/v1/messages",
				UpstreamPath: "https://upstream.example/v1/responses",
				Converter:    AdvancedCustomConverterOpenAIResponsesToOpenAIChatCompletions,
			},
			wantMessage: "converter does not match incoming_path",
		},
		{
			name: "header auth name must be valid",
			route: AdvancedCustomRoute{
				IncomingPath: "/v1/chat/completions",
				UpstreamPath: "https://upstream.example/v1/chat/completions",
				Auth: &AdvancedCustomRouteAuth{
					Type:  AdvancedCustomAuthTypeHeader,
					Name:  "bad header",
					Value: "{api_key}",
				},
			},
			wantMessage: "auth.name is not a valid header name",
		},
		{
			name: "query auth value is required",
			route: AdvancedCustomRoute{
				IncomingPath: "/v1/chat/completions",
				UpstreamPath: "https://upstream.example/v1/chat/completions",
				Auth: &AdvancedCustomRouteAuth{
					Type: AdvancedCustomAuthTypeQuery,
					Name: "key",
				},
			},
			wantMessage: "auth.value is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &AdvancedCustomConfig{Routes: []AdvancedCustomRoute{tt.route}}

			err := config.Validate()

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantMessage)
		})
	}
}

func TestAdvancedCustomValidateRejectsDuplicateIncomingPath(t *testing.T) {
	config := &AdvancedCustomConfig{
		Routes: []AdvancedCustomRoute{
			{
				IncomingPath: "/v1/chat/completions",
				UpstreamPath: "https://upstream-a.example/v1/chat/completions",
			},
			{
				IncomingPath: " /v1/chat/completions ",
				UpstreamPath: "https://upstream-b.example/v1/chat/completions",
			},
		},
	}

	err := config.Validate()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "incoming_path must be unique")
}

func TestAdvancedCustomMatchPathSupportsModelPlaceholderAndGeminiStream(t *testing.T) {
	config := &AdvancedCustomConfig{
		Routes: []AdvancedCustomRoute{
			{
				IncomingPath: "/v1beta/models/{model}:generateContent",
				UpstreamPath: "https://upstream.example/v1/chat/completions",
				Converter:    AdvancedCustomConverterGeminiGenerateContentToOpenAIChatCompletions,
			},
		},
	}

	require.NoError(t, config.Validate())
	_, ok := config.MatchPath("/v1beta/models/gemini-2.5-flash:streamGenerateContent")
	assert.True(t, ok)
	_, ok = config.MatchPath("/v1beta/models/family/gemini-2.5-flash:streamGenerateContent")
	assert.False(t, ok)
}
