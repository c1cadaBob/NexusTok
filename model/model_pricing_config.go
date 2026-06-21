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

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/setting/billing_setting"
	"github.com/c1cada/NexusTok/setting/ratio_setting"
	"gorm.io/gorm"
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

// ModelPricingConfig 是模型级定价聚合响应。
// Effective 表示当前实际生效配置，Override 表示当前模型名在 options 中的直接配置。
type ModelPricingConfig struct {
	ModelID     int                `json:"model_id"`
	ModelName   string             `json:"model_name"`
	BillingMode string             `json:"billing_mode"`
	Effective   ModelPricingValues `json:"effective"`
	Override    ModelPricingValues `json:"override"`
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
}

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
	return modelPricingOptionMaps{
		modelPrice:           ratio_setting.GetModelPriceCopy(),
		modelRatio:           ratio_setting.GetModelRatioCopy(),
		completionRatio:      ratio_setting.GetCompletionRatioCopy(),
		cacheRatio:           ratio_setting.GetCacheRatioCopy(),
		createCacheRatio:     ratio_setting.GetCreateCacheRatioCopy(),
		imageRatio:           ratio_setting.GetImageRatioCopy(),
		audioRatio:           ratio_setting.GetAudioRatioCopy(),
		audioCompletionRatio: ratio_setting.GetAudioCompletionRatioCopy(),
		billingMode:          billing_setting.GetBillingModeCopy(),
		billingExpr:          billing_setting.GetBillingExprCopy(),
	}
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

	return result, nil
}

func ptr[T any](value T) *T {
	return &value
}
