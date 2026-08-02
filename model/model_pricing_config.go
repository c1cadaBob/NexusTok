// Package model - model_pricing_config.go
// 该文件提供模型级定价配置聚合能力。
//
// 设计目标：
// - 管理后台以“模型”为入口读写定价；
// - 底层仍复用现有 options 配置，避免数据库迁移和 relay 热路径改造；
// - 保存时集中清理互斥计费模式，防止前端直接拼多个 JSON Map 时遗漏旧值。
package model

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/setting/billing_setting"
	"github.com/c1cada/NexusTok/setting/ratio_setting"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	// ModelPricingModeRatio 表示按 token 倍率计费，是系统默认计费方式。
	ModelPricingModeRatio = billing_setting.BillingModeRatio
	// ModelPricingModeFixed 表示按请求固定价格计费，对应现有 ModelPrice 配置。
	ModelPricingModeFixed = "fixed"
	// ModelPricingModeTieredExpr 表示按表达式计费，对应现有 billing_setting。
	ModelPricingModeTieredExpr = billing_setting.BillingModeTieredExpr
)

// ModelPricingValues 表示一组可选定价字段。
// 指针字段用于区分“显式设置为 0”和“未设置/继承默认值”。
type ModelPricingValues struct {
	ModelPrice            *float64 `json:"model_price,omitempty"`
	ModelRatio            *float64 `json:"model_ratio,omitempty"`
	InputPricePerMillion  *float64 `json:"input_price_per_million,omitempty"`
	OutputPricePerMillion *float64 `json:"output_price_per_million,omitempty"`
	CompletionRatio       *float64 `json:"completion_ratio,omitempty"`
	CacheRatio            *float64 `json:"cache_ratio,omitempty"`
	CreateCacheRatio      *float64 `json:"create_cache_ratio,omitempty"`
	ImageRatio            *float64 `json:"image_ratio,omitempty"`
	AudioRatio            *float64 `json:"audio_ratio,omitempty"`
	AudioCompletionRatio  *float64 `json:"audio_completion_ratio,omitempty"`
	BillingExpr           *string  `json:"billing_expr,omitempty"`
}

// ModelPricingSource 记录某个模型当前定价的来源。
// 该元数据不参与 relay 计费热路径，只用于同步策略判断：管理员在模型页保存后
// 标记为 manual，后续自动上游同步不得覆盖；上游同步写入时标记 provider，方便
// 管理后台展示价格来自哪一个 models.dev provider。
type ModelPricingSource struct {
	Kind      string `json:"kind"`
	Provider  string `json:"provider,omitempty"`
	Source    string `json:"source,omitempty"`
	UpdatedAt int64  `json:"updated_at,omitempty"`
}

// ModelPricingConfig 是模型级定价聚合响应。
// Effective 表示当前实际生效配置，Override 表示当前模型名在 options 中的直接配置。
type ModelPricingConfig struct {
	ModelID     int                 `json:"model_id"`
	ModelName   string              `json:"model_name"`
	BillingMode string              `json:"billing_mode"`
	Effective   ModelPricingValues  `json:"effective"`
	Override    ModelPricingValues  `json:"override"`
	Source      *ModelPricingSource `json:"source,omitempty"`
}

// ModelPricingUpdateRequest 是模型级定价保存请求。
// BillingMode 决定互斥配置的清理策略；其余字段为对应模式的可选覆盖值。
type ModelPricingUpdateRequest struct {
	BillingMode           string   `json:"billing_mode"`
	ModelPrice            *float64 `json:"model_price,omitempty"`
	ModelRatio            *float64 `json:"model_ratio,omitempty"`
	InputPricePerMillion  *float64 `json:"input_price_per_million,omitempty"`
	OutputPricePerMillion *float64 `json:"output_price_per_million,omitempty"`
	CompletionRatio       *float64 `json:"completion_ratio,omitempty"`
	CacheRatio            *float64 `json:"cache_ratio,omitempty"`
	CreateCacheRatio      *float64 `json:"create_cache_ratio,omitempty"`
	ImageRatio            *float64 `json:"image_ratio,omitempty"`
	AudioRatio            *float64 `json:"audio_ratio,omitempty"`
	AudioCompletionRatio  *float64 `json:"audio_completion_ratio,omitempty"`
	BillingExpr           *string  `json:"billing_expr,omitempty"`
}

type modelPricingOptionMaps struct {
	modelPrice           map[string]float64
	modelRatio           map[string]float64
	completionRatio      map[string]float64
	cacheRatio           map[string]float64
	createCacheRatio     map[string]float64
	imageRatio           map[string]float64
	audioRatio           map[string]float64
	audioCompletionRatio map[string]float64
	billingMode          map[string]string
	billingExpr          map[string]string
	pricingSource        map[string]ModelPricingSource
}

const (
	// ModelPricingSourceOptionKey 是 options 中保存模型定价来源元数据的键名。
	ModelPricingSourceOptionKey = "ModelPricingSource"
	// ModelPricingSourceManual 表示管理员在模型编辑页手动确认过价格。
	ModelPricingSourceManual = "manual"
	// ModelPricingSourceUpstream 表示价格来自上游同步策略。
	ModelPricingSourceUpstream = "upstream"
)

// GetModelPricingConfigByID 按模型 ID 聚合模型元数据与现有定价配置。
func GetModelPricingConfigByID(id int) (*ModelPricingConfig, error) {
	var m Model
	if err := DB.First(&m, id).Error; err != nil {
		return nil, err
	}
	return BuildModelPricingConfig(m.Id, m.ModelName), nil
}

// BuildModelPricingConfig 按模型名构建定价配置。
// 该函数不访问数据库，便于控制器和单元测试复用。
func BuildModelPricingConfig(modelID int, modelName string) *ModelPricingConfig {
	maps := readModelPricingOptionMaps()
	override := buildModelPricingOverride(modelName, maps)
	effective := ModelPricingValues{}
	mode := ModelPricingModeRatio

	if expr, ok := maps.billingExpr[modelName]; ok && strings.TrimSpace(expr) != "" && maps.billingMode[modelName] == ModelPricingModeTieredExpr {
		mode = ModelPricingModeTieredExpr
		effective.BillingExpr = ptr(expr)
		effective = mergeFallbackPricingValues(effective, override)
	} else if modelPrice, ok := ratio_setting.GetModelPrice(modelName, false); ok {
		mode = ModelPricingModeFixed
		effective.ModelPrice = ptr(modelPrice)
	} else {
		modelRatio, _, _ := ratio_setting.GetModelRatio(modelName)
		completionRatio := ratio_setting.GetCompletionRatio(modelName)
		inputPrice := modelRatio * 2
		outputPrice := inputPrice * completionRatio
		effective.ModelRatio = ptr(modelRatio)
		effective.InputPricePerMillion = ptr(inputPrice)
		effective.CompletionRatio = ptr(completionRatio)
		effective.OutputPricePerMillion = ptr(outputPrice)
	}

	if mode == ModelPricingModeRatio {
		if cacheRatio, ok := ratio_setting.GetCacheRatio(modelName); ok {
			effective.CacheRatio = ptr(cacheRatio)
		}
		if createCacheRatio, ok := ratio_setting.GetCreateCacheRatio(modelName); ok {
			effective.CreateCacheRatio = ptr(createCacheRatio)
		}
		if imageRatio, ok := ratio_setting.GetImageRatio(modelName); ok {
			effective.ImageRatio = ptr(imageRatio)
		}
		if ratio_setting.ContainsAudioRatio(modelName) {
			effective.AudioRatio = ptr(ratio_setting.GetAudioRatio(modelName))
		}
		if ratio_setting.ContainsAudioCompletionRatio(modelName) {
			effective.AudioCompletionRatio = ptr(ratio_setting.GetAudioCompletionRatio(modelName))
		}
	}

	return &ModelPricingConfig{
		ModelID:     modelID,
		ModelName:   modelName,
		BillingMode: mode,
		Effective:   effective,
		Override:    override,
		Source:      modelPricingSourcePtr(maps.pricingSource[modelName]),
	}
}

// SaveModelPricingConfig 保存某个模型的定价配置。
// 保存前只修改本地副本，表达式校验通过后再统一写回 options，避免非法表达式产生部分更新。
func SaveModelPricingConfig(modelName string, req ModelPricingUpdateRequest) error {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return errors.New("模型名称不能为空")
	}
	if req.BillingMode == "" {
		req.BillingMode = ModelPricingModeRatio
	}
	if err := validateModelPricingUpdateRequest(req); err != nil {
		return err
	}

	maps := readModelPricingOptionMaps()
	clearModelPricingEntries(maps, modelName)

	switch req.BillingMode {
	case ModelPricingModeFixed:
		maps.modelPrice[modelName] = *req.ModelPrice
	case ModelPricingModeRatio:
		applyRatioPricingUpdate(maps, modelName, req)
	case ModelPricingModeTieredExpr:
		expr := strings.TrimSpace(*req.BillingExpr)
		maps.billingMode[modelName] = ModelPricingModeTieredExpr
		maps.billingExpr[modelName] = expr
		applyTieredFallbackPricingUpdate(maps, modelName, req)
	default:
		return fmt.Errorf("不支持的计费模式: %s", req.BillingMode)
	}
	maps.pricingSource[modelName] = ModelPricingSource{
		Kind:      ModelPricingSourceManual,
		UpdatedAt: timeNowUnix(),
	}

	if err := saveModelPricingOptionMaps(maps); err != nil {
		return err
	}
	RefreshPricing()
	ratio_setting.InvalidateExposedDataCache()
	return nil
}

// RenameModelPricingConfig 将旧模型名下的定价配置迁移到新模型名。
// 该函数用于模型元数据重命名后兜底搬迁 options 中的直接配置，避免孤儿定价键残留。
func RenameModelPricingConfig(oldName, newName string) error {
	return RenameModelPricingConfigWithDB(DB, oldName, newName)
}

// RenameModelPricingConfigWithDB 在指定数据库上下文中迁移模型定价键。
// 控制器在模型元数据重命名事务中调用该函数，确保“模型名更新”和“定价键迁移”
// 要么一起提交，要么一起回滚。事务提交后仍会刷新内存配置，保证 relay 热路径读到新值。
func RenameModelPricingConfigWithDB(db *gorm.DB, oldName, newName string) error {
	oldName = strings.TrimSpace(oldName)
	newName = strings.TrimSpace(newName)
	if oldName == "" || newName == "" || oldName == newName {
		return nil
	}
	if db == nil {
		db = DB
	}

	maps := readModelPricingOptionMaps()
	changed := false
	moveFloat := func(target map[string]float64) {
		if value, ok := target[oldName]; ok {
			target[newName] = value
			delete(target, oldName)
			changed = true
		}
	}
	moveString := func(target map[string]string) {
		if value, ok := target[oldName]; ok {
			target[newName] = value
			delete(target, oldName)
			changed = true
		}
	}

	moveFloat(maps.modelPrice)
	moveFloat(maps.modelRatio)
	moveFloat(maps.completionRatio)
	moveFloat(maps.cacheRatio)
	moveFloat(maps.createCacheRatio)
	moveFloat(maps.imageRatio)
	moveFloat(maps.audioRatio)
	moveFloat(maps.audioCompletionRatio)
	moveString(maps.billingMode)
	moveString(maps.billingExpr)
	if value, ok := maps.pricingSource[oldName]; ok {
		maps.pricingSource[newName] = value
		delete(maps.pricingSource, oldName)
		changed = true
	}

	if !changed {
		return nil
	}
	if err := saveModelPricingOptionMapsWithDB(db, maps); err != nil {
		return err
	}
	RefreshPricing()
	ratio_setting.InvalidateExposedDataCache()
	return nil
}

func readModelPricingOptionMaps() modelPricingOptionMaps {
	options := getPersistedModelPricingOptions()
	return modelPricingOptionMaps{
		modelPrice:           parsePersistedFloatPricingOption(options, "ModelPrice", ratio_setting.GetDefaultModelPriceMap()),
		modelRatio:           parsePersistedFloatPricingOption(options, "ModelRatio", ratio_setting.GetDefaultModelRatioMap()),
		completionRatio:      parsePersistedFloatPricingOption(options, "CompletionRatio", ratio_setting.GetDefaultCompletionRatioMap()),
		cacheRatio:           parsePersistedFloatPricingOption(options, "CacheRatio", ratio_setting.GetDefaultCacheRatioMap()),
		createCacheRatio:     parsePersistedFloatPricingOption(options, "CreateCacheRatio", ratio_setting.GetDefaultCreateCacheRatioMap()),
		imageRatio:           parsePersistedFloatPricingOption(options, "ImageRatio", ratio_setting.GetDefaultImageRatioMap()),
		audioRatio:           parsePersistedFloatPricingOption(options, "AudioRatio", ratio_setting.GetDefaultAudioRatioMap()),
		audioCompletionRatio: parsePersistedFloatPricingOption(options, "AudioCompletionRatio", ratio_setting.GetDefaultAudioCompletionRatioMap()),
		billingMode:          parsePersistedStringPricingOption(options, "billing_setting.billing_mode"),
		billingExpr:          parsePersistedStringPricingOption(options, "billing_setting.billing_expr"),
		pricingSource:        GetModelPricingSourceCopy(),
	}
}

func parsePersistedFloatPricingOption(options map[string]string, optionKey string, defaults map[string]float64) map[string]float64 {
	result := make(map[string]float64)
	raw := strings.TrimSpace(options[optionKey])
	if raw == "" {
		return result
	}
	var values map[string]float64
	if err := common.UnmarshalJsonStr(raw, &values); err != nil {
		common.SysError("failed to parse persisted pricing option " + optionKey + ": " + err.Error())
		return result
	}
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if defaultValue, ok := defaults[key]; ok && samePricingFloat(defaultValue, value) {
			continue
		}
		result[key] = value
	}
	return result
}

func parsePersistedStringPricingOption(options map[string]string, optionKey string) map[string]string {
	result := make(map[string]string)
	raw := strings.TrimSpace(options[optionKey])
	if raw == "" {
		return result
	}
	var values map[string]string
	if err := common.UnmarshalJsonStr(raw, &values); err != nil {
		common.SysError("failed to parse persisted pricing option " + optionKey + ": " + err.Error())
		return result
	}
	for key, value := range values {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		result[key] = value
	}
	return result
}

// GetModelPricingOverrideModelSet 返回所有已经存在模型级定价覆盖的模型名集合。
//
// 该集合既包含新模型页保存的定价，也包含历史系统设置里直接维护的 options。
// 上游价格同步使用它保护旧配置：没有来源元数据并不等于可以覆盖，因为升级前
// 管理员手工录入的模型价格通常不会带有 ModelPricingSource 标记。
func GetModelPricingOverrideModelSet() map[string]struct{} {
	options := getPersistedModelPricingOptions()
	result := make(map[string]struct{})
	collectFloatKeys := func(optionKey string, defaults map[string]float64) {
		var values map[string]float64
		if err := common.UnmarshalJsonStr(options[optionKey], &values); err != nil {
			if strings.TrimSpace(options[optionKey]) != "" {
				common.SysError("failed to parse persisted pricing option " + optionKey + ": " + err.Error())
			}
			return
		}
		for key, value := range values {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			defaultValue, hasDefault := defaults[key]
			if !hasDefault || !samePricingFloat(defaultValue, value) {
				result[key] = struct{}{}
			}
		}
	}
	collectBillingModeKeys := func(optionKey string) {
		var values map[string]string
		if err := common.UnmarshalJsonStr(options[optionKey], &values); err != nil {
			if strings.TrimSpace(options[optionKey]) != "" {
				common.SysError("failed to parse persisted pricing option " + optionKey + ": " + err.Error())
			}
			return
		}
		for key, value := range values {
			key = strings.TrimSpace(key)
			value = strings.TrimSpace(value)
			if key == "" || value == "" || value == ModelPricingModeRatio {
				continue
			}
			result[key] = struct{}{}
		}
	}
	collectBillingExprKeys := func(optionKey string) {
		var values map[string]string
		if err := common.UnmarshalJsonStr(options[optionKey], &values); err != nil {
			if strings.TrimSpace(options[optionKey]) != "" {
				common.SysError("failed to parse persisted pricing option " + optionKey + ": " + err.Error())
			}
			return
		}
		for key, value := range values {
			key = strings.TrimSpace(key)
			if key != "" && strings.TrimSpace(value) != "" {
				result[key] = struct{}{}
			}
		}
	}
	collectManualSourceKeys := func(optionKey string) {
		var values map[string]ModelPricingSource
		if err := common.UnmarshalJsonStr(options[optionKey], &values); err != nil {
			if strings.TrimSpace(options[optionKey]) != "" {
				common.SysError("failed to parse persisted pricing option " + optionKey + ": " + err.Error())
			}
			return
		}
		for key, value := range values {
			key = strings.TrimSpace(key)
			if key != "" && strings.TrimSpace(value.Kind) == ModelPricingSourceManual {
				result[key] = struct{}{}
			}
		}
	}

	collectFloatKeys("ModelPrice", ratio_setting.GetDefaultModelPriceMap())
	collectFloatKeys("ModelRatio", ratio_setting.GetDefaultModelRatioMap())
	collectFloatKeys("CompletionRatio", ratio_setting.GetDefaultCompletionRatioMap())
	collectFloatKeys("CacheRatio", ratio_setting.GetDefaultCacheRatioMap())
	collectFloatKeys("CreateCacheRatio", ratio_setting.GetDefaultCreateCacheRatioMap())
	collectFloatKeys("ImageRatio", ratio_setting.GetDefaultImageRatioMap())
	collectFloatKeys("AudioRatio", ratio_setting.GetDefaultAudioRatioMap())
	collectFloatKeys("AudioCompletionRatio", ratio_setting.GetDefaultAudioCompletionRatioMap())
	collectBillingModeKeys("billing_setting.billing_mode")
	collectBillingExprKeys("billing_setting.billing_expr")
	collectManualSourceKeys(ModelPricingSourceOptionKey)
	return result
}

func getPersistedModelPricingOptions() map[string]string {
	keys := []string{
		"ModelPrice",
		"ModelRatio",
		"CompletionRatio",
		"CacheRatio",
		"CreateCacheRatio",
		"ImageRatio",
		"AudioRatio",
		"AudioCompletionRatio",
		"billing_setting.billing_mode",
		"billing_setting.billing_expr",
		ModelPricingSourceOptionKey,
	}
	result := make(map[string]string, len(keys))
	if DB != nil {
		var options []Option
		if err := DB.Where(clause.IN{
			Column: clause.Column{Name: "key"},
			Values: stringSliceToAny(keys),
		}).Find(&options).Error; err == nil {
			for _, option := range options {
				result[option.Key] = option.Value
			}
			return result
		} else {
			common.SysError("failed to load persisted model pricing options: " + err.Error())
		}
	}
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	for _, key := range keys {
		result[key] = common.OptionMap[key]
	}
	return result
}

func samePricingFloat(a, b float64) bool {
	return math.Abs(a-b) < 1e-12
}

func stringSliceToAny(values []string) []interface{} {
	result := make([]interface{}, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
}

func buildModelPricingOverride(modelName string, maps modelPricingOptionMaps) ModelPricingValues {
	override := ModelPricingValues{}
	if value, ok := maps.modelPrice[modelName]; ok {
		override.ModelPrice = ptr(value)
	}
	if value, ok := maps.modelRatio[modelName]; ok {
		inputPrice := value * 2
		override.ModelRatio = ptr(value)
		override.InputPricePerMillion = ptr(inputPrice)
	}
	if value, ok := maps.completionRatio[modelName]; ok {
		override.CompletionRatio = ptr(value)
		if override.InputPricePerMillion != nil {
			outputPrice := *override.InputPricePerMillion * value
			override.OutputPricePerMillion = ptr(outputPrice)
		}
	}
	if value, ok := maps.cacheRatio[modelName]; ok {
		override.CacheRatio = ptr(value)
	}
	if value, ok := maps.createCacheRatio[modelName]; ok {
		override.CreateCacheRatio = ptr(value)
	}
	if value, ok := maps.imageRatio[modelName]; ok {
		override.ImageRatio = ptr(value)
	}
	if value, ok := maps.audioRatio[modelName]; ok {
		override.AudioRatio = ptr(value)
	}
	if value, ok := maps.audioCompletionRatio[modelName]; ok {
		override.AudioCompletionRatio = ptr(value)
	}
	if expr, ok := maps.billingExpr[modelName]; ok && strings.TrimSpace(expr) != "" && maps.billingMode[modelName] == ModelPricingModeTieredExpr {
		override.BillingExpr = ptr(expr)
	}
	return override
}

func mergeFallbackPricingValues(base ModelPricingValues, fallback ModelPricingValues) ModelPricingValues {
	if fallback.ModelPrice != nil {
		base.ModelPrice = fallback.ModelPrice
	}
	if fallback.ModelRatio != nil {
		base.ModelRatio = fallback.ModelRatio
		base.InputPricePerMillion = fallback.InputPricePerMillion
	}
	if fallback.CompletionRatio != nil {
		base.CompletionRatio = fallback.CompletionRatio
		base.OutputPricePerMillion = fallback.OutputPricePerMillion
	}
	if fallback.CacheRatio != nil {
		base.CacheRatio = fallback.CacheRatio
	}
	if fallback.CreateCacheRatio != nil {
		base.CreateCacheRatio = fallback.CreateCacheRatio
	}
	if fallback.ImageRatio != nil {
		base.ImageRatio = fallback.ImageRatio
	}
	if fallback.AudioRatio != nil {
		base.AudioRatio = fallback.AudioRatio
	}
	if fallback.AudioCompletionRatio != nil {
		base.AudioCompletionRatio = fallback.AudioCompletionRatio
	}
	return base
}

func validateModelPricingUpdateRequest(req ModelPricingUpdateRequest) error {
	switch req.BillingMode {
	case ModelPricingModeFixed:
		if req.ModelPrice == nil {
			return errors.New("固定价格模式必须填写每次请求价格")
		}
	case ModelPricingModeRatio:
		if req.InputPricePerMillion != nil && req.OutputPricePerMillion != nil && *req.InputPricePerMillion == 0 && *req.OutputPricePerMillion != 0 {
			return errors.New("倍率模式无法表达输入价格为 0 但输出价格非 0，请使用表达式计费")
		}
	case ModelPricingModeTieredExpr:
		if req.BillingExpr == nil || strings.TrimSpace(*req.BillingExpr) == "" {
			return errors.New("表达式计费模式必须填写计费表达式")
		}
		if err := billing_setting.SmokeTestExpr(strings.TrimSpace(*req.BillingExpr)); err != nil {
			return fmt.Errorf("计费表达式校验失败: %w", err)
		}
	default:
		return fmt.Errorf("不支持的计费模式: %s", req.BillingMode)
	}

	for label, value := range map[string]*float64{
		"model_price":              req.ModelPrice,
		"model_ratio":              req.ModelRatio,
		"input_price_per_million":  req.InputPricePerMillion,
		"output_price_per_million": req.OutputPricePerMillion,
		"completion_ratio":         req.CompletionRatio,
		"cache_ratio":              req.CacheRatio,
		"create_cache_ratio":       req.CreateCacheRatio,
		"image_ratio":              req.ImageRatio,
		"audio_ratio":              req.AudioRatio,
		"audio_completion_ratio":   req.AudioCompletionRatio,
	} {
		if value == nil {
			continue
		}
		if math.IsNaN(*value) || math.IsInf(*value, 0) {
			return fmt.Errorf("%s 不是有效数字", label)
		}
		if *value < 0 {
			return fmt.Errorf("%s 不能小于 0", label)
		}
	}
	return nil
}

func applyRatioPricingUpdate(maps modelPricingOptionMaps, modelName string, req ModelPricingUpdateRequest) {
	if req.InputPricePerMillion != nil {
		maps.modelRatio[modelName] = *req.InputPricePerMillion / 2
	} else if req.ModelRatio != nil {
		maps.modelRatio[modelName] = *req.ModelRatio
	}

	if req.OutputPricePerMillion != nil {
		inputPrice := effectiveInputPriceForUpdate(req)
		if inputPrice != nil {
			if *inputPrice == 0 {
				maps.completionRatio[modelName] = 0
			} else {
				maps.completionRatio[modelName] = *req.OutputPricePerMillion / *inputPrice
			}
		}
	} else if req.CompletionRatio != nil {
		maps.completionRatio[modelName] = *req.CompletionRatio
	}

	setOptionalFloat(maps.cacheRatio, modelName, req.CacheRatio)
	setOptionalFloat(maps.createCacheRatio, modelName, req.CreateCacheRatio)
	setOptionalFloat(maps.imageRatio, modelName, req.ImageRatio)
	setOptionalFloat(maps.audioRatio, modelName, req.AudioRatio)
	setOptionalFloat(maps.audioCompletionRatio, modelName, req.AudioCompletionRatio)
}

func applyTieredFallbackPricingUpdate(maps modelPricingOptionMaps, modelName string, req ModelPricingUpdateRequest) {
	if req.ModelPrice != nil {
		maps.modelPrice[modelName] = *req.ModelPrice
	}
	applyRatioPricingUpdate(maps, modelName, req)
}

func effectiveInputPriceForUpdate(req ModelPricingUpdateRequest) *float64 {
	if req.InputPricePerMillion != nil {
		return req.InputPricePerMillion
	}
	if req.ModelRatio != nil {
		inputPrice := *req.ModelRatio * 2
		return &inputPrice
	}
	return nil
}

func setOptionalFloat(target map[string]float64, key string, value *float64) {
	if value != nil {
		target[key] = *value
	}
}

func clearModelPricingEntries(maps modelPricingOptionMaps, modelName string) {
	delete(maps.modelPrice, modelName)
	delete(maps.modelRatio, modelName)
	delete(maps.completionRatio, modelName)
	delete(maps.cacheRatio, modelName)
	delete(maps.createCacheRatio, modelName)
	delete(maps.imageRatio, modelName)
	delete(maps.audioRatio, modelName)
	delete(maps.audioCompletionRatio, modelName)
	delete(maps.billingMode, modelName)
	delete(maps.billingExpr, modelName)
}

func saveModelPricingOptionMaps(maps modelPricingOptionMaps) error {
	return saveModelPricingOptionMapsWithDB(DB, maps)
}

func saveModelPricingOptionMapsWithDB(db *gorm.DB, maps modelPricingOptionMaps) error {
	options, err := buildModelPricingOptionValues(maps)
	if err != nil {
		return err
	}
	if db == nil {
		db = DB
	}

	writeOptions := func(tx *gorm.DB) error {
		for key, value := range options {
			option := Option{Key: key}
			if err := tx.FirstOrCreate(&option, Option{Key: key}).Error; err != nil {
				return err
			}
			option.Value = value
			if err := tx.Save(&option).Error; err != nil {
				return err
			}
		}
		return nil
	}

	// 先在事务内写入 options 表，避免模型定价的多个 Map 出现数据库半提交。
	// 如果调用方已经传入事务上下文，则复用同一个事务，确保模型元数据与定价键一致提交。
	if err := db.Transaction(writeOptions); err != nil {
		return err
	}

	for key, value := range options {
		if err := updateOptionMap(key, value); err != nil {
			return err
		}
	}
	return nil
}

// GetModelPricingSourceCopy 返回模型定价来源元数据的副本。
func GetModelPricingSourceCopy() map[string]ModelPricingSource {
	result := make(map[string]ModelPricingSource)
	common.OptionMapRWMutex.RLock()
	raw := common.OptionMap[ModelPricingSourceOptionKey]
	common.OptionMapRWMutex.RUnlock()
	if strings.TrimSpace(raw) == "" {
		return result
	}
	if err := common.UnmarshalJsonStr(raw, &result); err != nil {
		common.SysError("failed to parse model pricing source: " + err.Error())
		return map[string]ModelPricingSource{}
	}
	return result
}

// SetModelPricingSource 设置单个模型的定价来源元数据。
func SetModelPricingSource(modelName string, source ModelPricingSource) error {
	sources := GetModelPricingSourceCopy()
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return nil
	}
	if strings.TrimSpace(source.Kind) == "" {
		delete(sources, modelName)
	} else {
		sources[modelName] = source
	}
	return saveModelPricingSourceMap(DB, sources)
}

func saveModelPricingSourceMap(db *gorm.DB, sources map[string]ModelPricingSource) error {
	if db == nil {
		db = DB
	}
	value, err := marshalOptionValue(sources)
	if err != nil {
		return err
	}
	option := Option{Key: ModelPricingSourceOptionKey}
	if err := db.FirstOrCreate(&option, Option{Key: ModelPricingSourceOptionKey}).Error; err != nil {
		return err
	}
	option.Value = value
	if err := db.Save(&option).Error; err != nil {
		return err
	}
	common.OptionMapRWMutex.Lock()
	common.OptionMap[ModelPricingSourceOptionKey] = value
	common.OptionMapRWMutex.Unlock()
	return nil
}

// SaveModelPricingConfigBatch 批量保存多个模型的定价配置。
// 该函数专供上游价格同步使用：调用方已经完成策略选择和合法性判断，这里负责
// 一次性更新 options，减少每日同步或批量同步时的数据库写入次数。
func SaveModelPricingConfigBatch(updates map[string]ModelPricingUpdateRequest, sources map[string]ModelPricingSource) error {
	if len(updates) == 0 {
		return nil
	}
	maps := readModelPricingOptionMaps()
	for modelName, req := range updates {
		modelName = strings.TrimSpace(modelName)
		if modelName == "" {
			continue
		}
		if req.BillingMode == "" {
			req.BillingMode = ModelPricingModeRatio
		}
		if err := validateModelPricingUpdateRequest(req); err != nil {
			return fmt.Errorf("%s: %w", modelName, err)
		}
		clearModelPricingEntries(maps, modelName)
		switch req.BillingMode {
		case ModelPricingModeFixed:
			maps.modelPrice[modelName] = *req.ModelPrice
		case ModelPricingModeRatio:
			applyRatioPricingUpdate(maps, modelName, req)
		case ModelPricingModeTieredExpr:
			expr := strings.TrimSpace(*req.BillingExpr)
			maps.billingMode[modelName] = ModelPricingModeTieredExpr
			maps.billingExpr[modelName] = expr
			applyTieredFallbackPricingUpdate(maps, modelName, req)
		default:
			return fmt.Errorf("%s: 不支持的计费模式: %s", modelName, req.BillingMode)
		}
		if source, ok := sources[modelName]; ok {
			maps.pricingSource[modelName] = source
		}
	}
	if err := saveModelPricingOptionMaps(maps); err != nil {
		return err
	}
	RefreshPricing()
	ratio_setting.InvalidateExposedDataCache()
	return nil
}

func marshalOptionValue[T any](value T) (string, error) {
	bytes, err := common.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func buildModelPricingOptionValues(maps modelPricingOptionMaps) (map[string]string, error) {
	result := make(map[string]string, 10)

	floatMaps := []struct {
		key   string
		value map[string]float64
	}{
		{key: "ModelPrice", value: maps.modelPrice},
		{key: "ModelRatio", value: maps.modelRatio},
		{key: "CompletionRatio", value: maps.completionRatio},
		{key: "CacheRatio", value: maps.cacheRatio},
		{key: "CreateCacheRatio", value: maps.createCacheRatio},
		{key: "ImageRatio", value: maps.imageRatio},
		{key: "AudioRatio", value: maps.audioRatio},
		{key: "AudioCompletionRatio", value: maps.audioCompletionRatio},
	}
	for _, item := range floatMaps {
		value, err := marshalOptionValue(item.value)
		if err != nil {
			return nil, err
		}
		result[item.key] = value
	}

	stringMaps := []struct {
		key   string
		value map[string]string
	}{
		{key: "billing_setting.billing_mode", value: maps.billingMode},
		{key: "billing_setting.billing_expr", value: maps.billingExpr},
	}
	for _, item := range stringMaps {
		value, err := marshalOptionValue(item.value)
		if err != nil {
			return nil, err
		}
		result[item.key] = value
	}

	sourceValue, err := marshalOptionValue(maps.pricingSource)
	if err != nil {
		return nil, err
	}
	result[ModelPricingSourceOptionKey] = sourceValue

	return result, nil
}

func ptr[T any](value T) *T {
	return &value
}

func modelPricingSourcePtr(source ModelPricingSource) *ModelPricingSource {
	if strings.TrimSpace(source.Kind) == "" {
		return nil
	}
	return &source
}

func timeNowUnix() int64 {
	return time.Now().Unix()
}
