package controller

import (
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"github.com/stretchr/testify/assert"
)

func TestChannelAccountUpdateMapOnlyUpdatesSubmittedFields(t *testing.T) {
	priority := int64(0)
	weight := 0
	maxConcurrency := 0
	req := channelAccountUpsertRequest{
		Name:           " renamed ",
		Models:         "",
		Group:          "",
		Priority:       &priority,
		Weight:         &weight,
		MaxConcurrency: &maxConcurrency,
	}

	updates := channelAccountUpdateMap(req, map[string]any{"name": req.Name})
	assert.Equal(t, map[string]interface{}{"name": "renamed"}, updates)

	updates = channelAccountUpdateMap(req, map[string]any{
		"models":          req.Models,
		"group":           req.Group,
		"priority":        priority,
		"weight":          weight,
		"max_concurrency": maxConcurrency,
	})
	assert.Equal(t, map[string]interface{}{
		"models":          "",
		"group":           "",
		"priority":        int64(0),
		"weight":          0,
		"max_concurrency": 0,
	}, updates)
}

func TestChannelAccountUpdateMapKeepsEmptyKeyFromOverwritingCredential(t *testing.T) {
	req := channelAccountUpsertRequest{Key: "   "}

	updates := channelAccountUpdateMap(req, map[string]any{"key": req.Key})

	assert.NotContains(t, updates, "key")
}

func TestChannelAccountUpdateMapUsesGormColumnForOpenAIOrganization(t *testing.T) {
	organization := "org-example"
	req := channelAccountUpsertRequest{OpenAIOrganization: &organization}

	updates := channelAccountUpdateMap(req, map[string]any{
		"openai_organization": organization,
	})

	assert.Equal(t, map[string]interface{}{
		"open_ai_organization": organization,
	}, updates)
	assert.NotContains(t, updates, "openai_organization")
}

func TestChannelAccountSensitiveChangeClassification(t *testing.T) {
	baseURL := "https://api.example.com"
	account := &model.ChannelAccount{
		Key:     "old-key",
		Status:  common.ChannelStatusEnabled,
		Models:  "gpt-5.6",
		Group:   "default",
		BaseURL: &baseURL,
	}

	t.Run("普通调度字段不需要敏感写权限", func(t *testing.T) {
		req := channelAccountUpsertRequest{
			Name:   "renamed",
			Models: "gpt-5.6,gpt-5.6-mini",
			Group:  "vip",
		}

		assert.False(t, channelAccountHasSensitiveChanges(account, req, map[string]any{
			"name":   req.Name,
			"models": req.Models,
			"group":  req.Group,
		}))
	})

	t.Run("密钥变化需要敏感写权限", func(t *testing.T) {
		req := channelAccountUpsertRequest{Key: "new-key"}

		assert.True(t, channelAccountHasSensitiveChanges(account, req, map[string]any{
			"key": req.Key,
		}))
	})

	t.Run("状态变化需要敏感写权限", func(t *testing.T) {
		disabled := common.ChannelStatusManuallyDisabled
		req := channelAccountUpsertRequest{Status: &disabled}

		assert.True(t, channelAccountHasSensitiveChanges(account, req, map[string]any{
			"status": disabled,
		}))
	})

	t.Run("清空上游地址需要敏感写权限", func(t *testing.T) {
		req := channelAccountUpsertRequest{}

		assert.True(t, channelAccountHasSensitiveChanges(account, req, map[string]any{
			"base_url": nil,
		}))
	})
}

func TestChannelAccountUnknownFieldsFailClosed(t *testing.T) {
	assert.False(t, channelAccountHasUnknownFields(map[string]any{
		"name":   "renamed",
		"models": "gpt-5.6",
	}))
	assert.True(t, channelAccountHasUnknownFields(map[string]any{
		"future_secret_field": "value",
	}))
}
