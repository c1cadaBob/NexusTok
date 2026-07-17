package upstreamaccount

import (
	"fmt"
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"
	"gorm.io/gorm"
)

// CreateRequest 表示根据预览快照创建渠道及渠道账号的请求。
type CreateRequest struct {
	PreviewID         string                `json:"preview_id"`
	Channel           ChannelCreateConfig   `json:"channel"`
	Accounts          []AccountCreateConfig `json:"accounts"`
	ApplySuggested    bool                  `json:"apply_suggested"`
	DisableMissingKey bool                  `json:"disable_missing_key"`
}

// ChannelCreateConfig 是预览创建时允许用户配置的渠道字段。
type ChannelCreateConfig struct {
	Name              string  `json:"name"`
	Type              int     `json:"type"`
	BaseURL           *string `json:"base_url"`
	Models            string  `json:"models"`
	Group             string  `json:"group"`
	Priority          *int64  `json:"priority"`
	Weight            *uint   `json:"weight"`
	TestModel         *string `json:"test_model"`
	AutoBan           *int    `json:"auto_ban"`
	Status            int     `json:"status"`
	Tag               *string `json:"tag"`
	Remark            *string `json:"remark"`
	Setting           *string `json:"setting"`
	ParamOverride     *string `json:"param_override"`
	HeaderOverride    *string `json:"header_override"`
	StatusCodeMapping *string `json:"status_code_mapping"`
	Other             string  `json:"other"`
	OtherSettings     string  `json:"settings"`
}

// AccountCreateConfig 是单个同步密钥在 NexusTok 中的配置覆盖。
type AccountCreateConfig struct {
	ExternalID         string  `json:"external_id"`
	Name               string  `json:"name"`
	Enabled            *bool   `json:"enabled"`
	Models             string  `json:"models"`
	Group              string  `json:"group"`
	Priority           *int64  `json:"priority"`
	Weight             *int    `json:"weight"`
	BaseURL            *string `json:"base_url"`
	OpenAIOrganization *string `json:"openai_organization"`
	Other              string  `json:"other"`
	Setting            *string `json:"setting"`
	OtherSettings      string  `json:"settings"`
	ModelMapping       *string `json:"model_mapping"`
	ParamOverride      *string `json:"param_override"`
	HeaderOverride     *string `json:"header_override"`
	StatusCodeMapping  *string `json:"status_code_mapping"`
	MaxConcurrency     int     `json:"max_concurrency"`
}

// CreateResult 表示创建结果。
type CreateResult struct {
	ChannelID int `json:"channel_id"`
	Created   int `json:"created"`
	Skipped   int `json:"skipped"`
}

// CreateFromPreview 根据短期预览快照创建渠道和渠道内账号池。
func CreateFromPreview(req CreateRequest) (*CreateResult, error) {
	record, err := GetPreviewRecord(req.PreviewID)
	if err != nil {
		return nil, err
	}
	if record.Snapshot == nil {
		return nil, fmt.Errorf("预览快照为空，请重新同步")
	}
	channel, accounts, err := buildChannelAndAccounts(record.Snapshot, req)
	if err != nil {
		return nil, err
	}
	created := 0
	skipped := 0
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(channel).Error; err != nil {
			return err
		}
		if err := channel.AddAbilities(tx); err != nil {
			return err
		}
		for i := range accounts {
			accounts[i].ChannelId = channel.Id
		}
		if len(accounts) > 0 {
			if err := tx.Create(&accounts).Error; err != nil {
				return err
			}
			created = len(accounts)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	model.InitChannelCache()
	return &CreateResult{ChannelID: channel.Id, Created: created, Skipped: skipped}, nil
}

func buildChannelAndAccounts(snapshot *Snapshot, req CreateRequest) (*model.Channel, []model.ChannelAccount, error) {
	if strings.TrimSpace(req.Channel.Name) == "" {
		return nil, nil, fmt.Errorf("渠道名称不能为空")
	}
	if req.Channel.Type <= 0 {
		return nil, nil, fmt.Errorf("渠道类型不能为空")
	}
	models := strings.TrimSpace(req.Channel.Models)
	if models == "" {
		models = inferModelsFromKeys(snapshot.Keys)
	}
	if models == "" {
		return nil, nil, fmt.Errorf("模型不能为空，请在预览结果中选择或手动填写模型")
	}
	group := strings.TrimSpace(req.Channel.Group)
	if group == "" {
		group = inferGroupFromKeys(snapshot.Keys)
	}
	if group == "" {
		group = "default"
	}
	status := req.Channel.Status
	if status <= 0 {
		status = common.ChannelStatusEnabled
	}
	priority := req.Channel.Priority
	if priority == nil {
		value := int64(0)
		priority = &value
	}
	weight := req.Channel.Weight
	if weight == nil {
		value := uint(0)
		weight = &value
	}
	autoBan := req.Channel.AutoBan
	if autoBan == nil {
		value := 1
		autoBan = &value
	}
	channel := &model.Channel{
		Type:               req.Channel.Type,
		Key:                constant.ChannelCredentialModeAccountPool,
		Name:               strings.TrimSpace(req.Channel.Name),
		Weight:             weight,
		CreatedTime:        common.GetTimestamp(),
		BaseURL:            req.Channel.BaseURL,
		Balance:            balanceValue(snapshot.Balance),
		BalanceUpdatedTime: common.GetTimestamp(),
		Models:             models,
		Group:              group,
		UsedQuota:          usedQuotaValue(snapshot.Balance),
		Priority:           priority,
		AutoBan:            autoBan,
		Status:             status,
		TestModel:          req.Channel.TestModel,
		Tag:                req.Channel.Tag,
		Remark:             req.Channel.Remark,
		Setting:            req.Channel.Setting,
		ParamOverride:      req.Channel.ParamOverride,
		HeaderOverride:     req.Channel.HeaderOverride,
		StatusCodeMapping:  req.Channel.StatusCodeMapping,
		Other:              req.Channel.Other,
		OtherSettings:      mergeSyncMetadata(req.Channel.OtherSettings, snapshot),
		ChannelInfo: model.ChannelInfo{
			CredentialMode:      constant.ChannelCredentialModeAccountPool,
			AccountPoolEnabled:  true,
			AccountPoolMode:     constant.ChannelAccountPoolModePolling,
			AccountPoolFallback: false,
			IsMultiKey:          false,
		},
	}
	accounts, err := buildAccounts(snapshot, req, models, group)
	if err != nil {
		return nil, nil, err
	}
	return channel, accounts, nil
}

func buildAccounts(snapshot *Snapshot, req CreateRequest, defaultModels string, defaultGroup string) ([]model.ChannelAccount, error) {
	configs := map[string]AccountCreateConfig{}
	for _, config := range req.Accounts {
		if strings.TrimSpace(config.ExternalID) == "" {
			continue
		}
		configs[config.ExternalID] = config
	}
	accounts := make([]model.ChannelAccount, 0, len(snapshot.Keys))
	for _, key := range snapshot.Keys {
		if strings.TrimSpace(key.Key) == "" {
			return nil, fmt.Errorf("预览快照中的密钥 %s 缺少完整 key，请重新同步", key.Name)
		}
		config := configs[key.ExternalID]
		if config.Enabled != nil && !*config.Enabled {
			continue
		}
		status := common.ChannelStatusEnabled
		if key.Status > 0 && key.Status != common.ChannelStatusEnabled {
			status = key.Status
		}
		name := strings.TrimSpace(config.Name)
		if name == "" {
			name = strings.TrimSpace(key.Name)
		}
		if name == "" {
			name = key.MaskedKey
		}
		models := strings.TrimSpace(config.Models)
		if models == "" {
			models = strings.Join(key.Models, ",")
		}
		if models == "" {
			models = defaultModels
		}
		group := strings.TrimSpace(config.Group)
		if group == "" {
			group = firstNonEmpty(key.GroupName, key.GroupID, defaultGroup)
		}
		priority := int64(0)
		if req.ApplySuggested {
			priority = key.SuggestedPriority
		}
		if config.Priority != nil {
			priority = *config.Priority
		}
		weight := 0
		if req.ApplySuggested {
			weight = key.SuggestedWeight
		}
		if config.Weight != nil {
			weight = *config.Weight
		}
		accounts = append(accounts, model.ChannelAccount{
			Name:               name,
			Key:                key.Key,
			Status:             status,
			Models:             models,
			Group:              group,
			Priority:           priority,
			Weight:             weight,
			UsedQuota:          quotaToInt64(key.QuotaUsedUSD),
			BaseURL:            config.BaseURL,
			OpenAIOrganization: config.OpenAIOrganization,
			Other:              config.Other,
			Setting:            config.Setting,
			OtherSettings:      config.OtherSettings,
			ModelMapping:       config.ModelMapping,
			ParamOverride:      config.ParamOverride,
			HeaderOverride:     config.HeaderOverride,
			StatusCodeMapping:  config.StatusCodeMapping,
			MaxConcurrency:     config.MaxConcurrency,
		})
	}
	if len(accounts) == 0 {
		return nil, fmt.Errorf("没有可创建的同步密钥")
	}
	return accounts, nil
}

func inferModelsFromKeys(keys []SyncedKey) string {
	seen := map[string]struct{}{}
	result := make([]string, 0)
	for _, key := range keys {
		for _, modelName := range key.Models {
			modelName = strings.TrimSpace(modelName)
			if modelName == "" {
				continue
			}
			if _, ok := seen[modelName]; ok {
				continue
			}
			seen[modelName] = struct{}{}
			result = append(result, modelName)
		}
	}
	return strings.Join(result, ",")
}

func inferGroupFromKeys(keys []SyncedKey) string {
	seen := map[string]struct{}{}
	result := make([]string, 0)
	for _, key := range keys {
		group := strings.TrimSpace(firstNonEmpty(key.GroupName, key.GroupID))
		if group == "" {
			continue
		}
		if _, ok := seen[group]; ok {
			continue
		}
		seen[group] = struct{}{}
		result = append(result, group)
	}
	return strings.Join(result, ",")
}

func balanceValue(balance *BalanceSnapshot) float64 {
	if balance != nil && balance.BalanceUSD != nil {
		return *balance.BalanceUSD
	}
	return 0
}

func usedQuotaValue(balance *BalanceSnapshot) int64 {
	if balance != nil && balance.UsedUSD != nil {
		return int64(*balance.UsedUSD)
	}
	return 0
}

func quotaToInt64(value *float64) int64 {
	if value == nil {
		return 0
	}
	return int64(*value)
}

func mergeSyncMetadata(existing string, snapshot *Snapshot) string {
	var data map[string]any
	if strings.TrimSpace(existing) != "" {
		_ = common.UnmarshalJsonStr(existing, &data)
	}
	if data == nil {
		data = map[string]any{}
	}
	data["upstream_account_sync"] = map[string]any{
		"platform":  snapshot.Platform,
		"base_url":  snapshot.BaseURL,
		"synced_at": common.GetTimestamp(),
	}
	bytes, err := common.Marshal(data)
	if err != nil {
		return existing
	}
	return string(bytes)
}
