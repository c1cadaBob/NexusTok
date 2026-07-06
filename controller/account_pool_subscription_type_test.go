package controller

import (
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"github.com/stretchr/testify/require"
)

func buildAccountPoolAuthFileMetadata(t *testing.T, metadata map[string]any) string {
	t.Helper()
	data, err := common.Marshal(metadata)
	require.NoError(t, err)
	return string(data)
}

func TestAccountPoolAuthFileSubscriptionTypeFromMetadata(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]any
		want     string
	}{
		{
			name: "direct plus",
			metadata: map[string]any{
				"plan_type": "plus",
			},
			want: "plus",
		},
		{
			name: "chatgpt pro alias",
			metadata: map[string]any{
				"extra": map[string]any{
					"chatgpt_plan_type": "ChatGPT Pro",
				},
			},
			want: "pro",
		},
		{
			name: "business internal plan",
			metadata: map[string]any{
				"account": map[string]any{
					"plan_type": "self_serve_business_usage_based",
				},
			},
			want: "business",
		},
		{
			name: "google free tier",
			metadata: map[string]any{
				"tier_id": "google_one_free",
			},
			want: "free",
		},
		{
			name: "k12 alias",
			metadata: map[string]any{
				"subscription_type": "education",
			},
			want: "k12",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authFile := &model.AccountPoolAuthFile{
				Provider:           "codex",
				Platform:           "codex",
				CredentialMetadata: buildAccountPoolAuthFileMetadata(t, tt.metadata),
			}

			require.Equal(t, tt.want, accountPoolAuthFileSubscriptionType(authFile))
		})
	}
}
