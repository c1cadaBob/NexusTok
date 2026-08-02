package modelcatalog

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/setting/ratio_setting"
	"gorm.io/gorm"
)

const (
	envCatalogWriteBack     = "MODEL_CATALOG_WRITE_BACK"
	envCatalogRepoDir       = "MODEL_CATALOG_REPO_DIR"
	envCatalogGitAutocommit = "MODEL_CATALOG_GIT_AUTOCOMMIT"
)

type selectedPricing struct {
	ModelName  string
	ProviderID string
	Update     model.ModelPricingUpdateRequest
}

// WriteBackEnabled 返回开发环境模型仓库写回开关状态。
//
// 生产环境必须保持关闭。该开关只控制 Git 工作区里的模型仓库文件写入，不会导出用户、
// Token、渠道、账号池、日志、密钥或其它运行时数据。
func WriteBackEnabled() bool {
	return common.GetEnvOrDefaultBool(envCatalogWriteBack, false)
}

// RepositoryDir 返回模型仓库源码目录。
func RepositoryDir() string {
	return normalizeRepositoryDir(common.GetEnvOrDefaultString(envCatalogRepoDir, RepositoryDefaultDir))
}

// SeedEmbeddedCatalog 将内置模型仓库补齐到数据库。
//
// 该函数适合在启动时、InitOptionMap 之后执行。它的保护边界是：
// - 只创建缺失供应商和缺失模型；
// - 对 sync_official=0 的模型完全跳过；
// - 只给没有模型级定价覆盖的模型补价格；
// - manual、upstream、builtin 或历史 options 覆盖都不覆盖。
func SeedEmbeddedCatalog() (*SeedResult, error) {
	catalog, err := LoadEmbeddedCatalog()
	if err != nil {
		return nil, err
	}
	result, err := SeedCatalog(catalog)
	if err != nil {
		return result, err
	}
	if result != nil && (result.CreatedModels > 0 || result.CreatedVendors > 0 || result.PricingUpdated > 0) {
		model.RefreshPricing()
		ratio_setting.InvalidateExposedDataCache()
	}
	return result, nil
}

// SeedCatalog 将指定 catalog 按只读补齐策略写入数据库。
func SeedCatalog(catalog *Catalog) (*SeedResult, error) {
	if catalog == nil {
		return nil, errors.New("model catalog is nil")
	}
	result := &SeedResult{
		SkippedModels: make([]string, 0),
		PricingList:   make([]string, 0),
	}
	vendorIDCache := make(map[string]int)
	vendorByOwner := buildProviderByOwner(catalog)

	for _, key := range sortedModelKeys(catalog.Models) {
		ownerID, modelName := splitCanonicalKey(key)
		def := normalizeCatalogModel(catalog.Models[key], modelName)
		modelName = strings.TrimSpace(def.ID)
		if modelName == "" || len(modelName) > 128 {
			result.SkippedModels = append(result.SkippedModels, key)
			continue
		}

		var existing model.Model
		err := model.DB.Where("model_name = ?", modelName).First(&existing).Error
		if err == nil {
			if existing.SyncOfficial == 0 {
				result.SkippedModels = append(result.SkippedModels, modelName)
			}
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return result, err
		}

		provider := vendorByOwner[ownerID]
		vendorID, err := ensureCatalogVendor(provider, vendorIDCache, result)
		if err != nil {
			return result, err
		}
		item := &model.Model{
			ModelName:    modelName,
			Description:  catalogModelDescription(provider, def),
			Icon:         catalogModelIcon(provider, def),
			Tags:         strings.Join(def.Tags, ","),
			VendorID:     vendorID,
			Status:       catalogModelStatus(def.Status),
			SyncOfficial: 1,
			NameRule:     normalizedNameRule(def.NameRule),
		}
		if err := item.Insert(); err != nil {
			return result, err
		}
		result.CreatedModels++
	}

	if err := seedCatalogPricing(catalog, result); err != nil {
		return result, err
	}
	sort.Strings(result.SkippedModels)
	sort.Strings(result.PricingList)
	return result, nil
}

func seedCatalogPricing(catalog *Catalog, result *SeedResult) error {
	selected := selectCatalogPricing(catalog)
	if len(selected) == 0 {
		return nil
	}
	modelNames := make([]string, 0, len(selected))
	for name := range selected {
		modelNames = append(modelNames, name)
	}
	sort.Strings(modelNames)

	var existing []model.Model
	if err := model.DB.Where("model_name IN ?", modelNames).Find(&existing).Error; err != nil {
		return err
	}
	existingByName := make(map[string]model.Model, len(existing))
	for _, item := range existing {
		existingByName[item.ModelName] = item
	}

	overrides := model.GetModelPricingOverrideModelSet()
	sources := model.GetModelPricingSourceCopy()
	updates := make(map[string]model.ModelPricingUpdateRequest)
	sourceUpdates := make(map[string]model.ModelPricingSource)
	for _, modelName := range modelNames {
		local, ok := existingByName[modelName]
		if !ok || local.SyncOfficial == 0 {
			result.PricingUpdated += 0
			continue
		}
		if _, hasOverride := overrides[modelName]; hasOverride {
			continue
		}
		if strings.TrimSpace(sources[modelName].Kind) != "" {
			continue
		}
		item := selected[modelName]
		updates[modelName] = item.Update
		sourceUpdates[modelName] = model.ModelPricingSource{
			Kind:      model.ModelPricingSourceBuiltin,
			Provider:  item.ProviderID,
			Source:    CatalogOriginNexusTokRepository,
			UpdatedAt: time.Now().Unix(),
		}
	}
	if len(updates) == 0 {
		return nil
	}
	if err := model.SaveModelPricingConfigBatch(updates, sourceUpdates); err != nil {
		return err
	}
	result.PricingUpdated += len(updates)
	for modelName := range updates {
		result.PricingList = append(result.PricingList, modelName)
	}
	return nil
}

func ensureCatalogVendor(provider CatalogProvider, vendorIDCache map[string]int, result *SeedResult) (int, error) {
	provider = normalizeCatalogProvider(provider, provider.ID)
	name := strings.TrimSpace(provider.Name)
	if name == "" {
		return 0, nil
	}
	if id, ok := vendorIDCache[name]; ok {
		return id, nil
	}
	var existing model.Vendor
	if err := model.DB.Where("name = ?", name).First(&existing).Error; err == nil {
		vendorIDCache[name] = existing.Id
		return existing.Id, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, err
	}
	item := &model.Vendor{
		Name:        name,
		Description: provider.Description,
		Icon:        provider.Icon,
		Status:      1,
	}
	if item.Icon == "" {
		item.Icon = providerIcon(provider.ID, provider.Name)
	}
	if err := item.Insert(); err != nil {
		return 0, err
	}
	vendorIDCache[name] = item.Id
	result.CreatedVendors++
	return item.Id, nil
}

func buildProviderByOwner(catalog *Catalog) map[string]CatalogProvider {
	result := make(map[string]CatalogProvider)
	if catalog == nil {
		return result
	}
	for providerID, provider := range catalog.Providers {
		result[providerID] = normalizeCatalogProvider(provider, providerID)
	}
	for key := range catalog.Models {
		ownerID, _ := splitCanonicalKey(key)
		if ownerID == "" {
			continue
		}
		if _, ok := result[ownerID]; !ok {
			result[ownerID] = normalizeCatalogProvider(CatalogProvider{ID: ownerID}, ownerID)
		}
	}
	return result
}

func selectCatalogPricing(catalog *Catalog) map[string]selectedPricing {
	result := make(map[string]selectedPricing)
	if catalog == nil {
		return result
	}
	providerIDs := sortedProviderKeys(catalog.Providers)
	sort.SliceStable(providerIDs, func(i, j int) bool {
		pi := providerPriority(providerIDs[i])
		pj := providerPriority(providerIDs[j])
		if pi != pj {
			return pi < pj
		}
		return providerIDs[i] < providerIDs[j]
	})
	for _, providerID := range providerIDs {
		provider := catalog.Providers[providerID]
		modelIDs := sortedModelKeys(provider.Models)
		for _, modelID := range modelIDs {
			if _, exists := result[modelID]; exists {
				continue
			}
			update, ok := buildPricingUpdate(provider.Models[modelID])
			if !ok {
				continue
			}
			result[modelID] = selectedPricing{
				ModelName:  modelID,
				ProviderID: providerID,
				Update:     update,
			}
		}
	}
	for key, def := range catalog.Models {
		_, modelID := splitCanonicalKey(key)
		if _, exists := result[modelID]; exists {
			continue
		}
		update, ok := buildPricingUpdate(def)
		if !ok {
			continue
		}
		result[modelID] = selectedPricing{
			ModelName:  modelID,
			ProviderID: def.PricingProvider(),
			Update:     update,
		}
	}
	return result
}

func buildPricingUpdate(def CatalogModel) (model.ModelPricingUpdateRequest, bool) {
	if def.Pricing != nil {
		return buildPricingUpdateFromCatalogPricing(*def.Pricing)
	}
	return buildPricingUpdateFromCatalogCost(def.Cost)
}

func buildPricingUpdateFromCatalogPricing(pricing CatalogPricing) (model.ModelPricingUpdateRequest, bool) {
	mode := strings.TrimSpace(pricing.BillingMode)
	if mode == "" {
		mode = model.ModelPricingModeRatio
	}
	update := model.ModelPricingUpdateRequest{
		BillingMode:           mode,
		ModelPrice:            cloneFloat(pricing.ModelPrice),
		ModelRatio:            cloneFloat(pricing.ModelRatio),
		InputPricePerMillion:  cloneFloat(pricing.InputPricePerMillion),
		OutputPricePerMillion: cloneFloat(pricing.OutputPricePerMillion),
		CompletionRatio:       cloneFloat(pricing.CompletionRatio),
		CacheRatio:            cloneFloat(pricing.CacheRatio),
		CreateCacheRatio:      cloneFloat(pricing.CreateCacheRatio),
		ImageRatio:            cloneFloat(pricing.ImageRatio),
		AudioRatio:            cloneFloat(pricing.AudioRatio),
		AudioCompletionRatio:  cloneFloat(pricing.AudioCompletionRatio),
	}
	if strings.TrimSpace(pricing.BillingExpr) != "" {
		update.BillingExpr = common.GetPointer(strings.TrimSpace(pricing.BillingExpr))
	}
	switch mode {
	case model.ModelPricingModeFixed:
		return update, update.ModelPrice != nil
	case model.ModelPricingModeTieredExpr:
		return update, update.BillingExpr != nil
	default:
		if update.InputPricePerMillion == nil && update.ModelRatio == nil {
			return model.ModelPricingUpdateRequest{}, false
		}
		return update, true
	}
}

func buildPricingUpdateFromCatalogCost(cost CatalogCost) (model.ModelPricingUpdateRequest, bool) {
	if cost.Input == nil || !validCost(*cost.Input) {
		return model.ModelPricingUpdateRequest{}, false
	}
	input := *cost.Input
	if input == 0 && cost.Output != nil && *cost.Output > 0 {
		return model.ModelPricingUpdateRequest{}, false
	}
	update := model.ModelPricingUpdateRequest{
		BillingMode:          model.ModelPricingModeRatio,
		InputPricePerMillion: &input,
	}
	if output := cloneValidCost(cost.Output); output != nil {
		update.OutputPricePerMillion = output
	}
	if ratio := relativeRatio(cost.CacheRead, input); ratio != nil {
		update.CacheRatio = ratio
	}
	if ratio := relativeRatio(cost.CacheWrite, input); ratio != nil {
		update.CreateCacheRatio = ratio
	}
	if ratio := relativeRatio(cost.InputAudio, input); ratio != nil {
		update.AudioRatio = ratio
	}
	if cost.OutputAudio != nil && cost.InputAudio != nil && *cost.InputAudio > 0 {
		value := roundRatio(*cost.OutputAudio / *cost.InputAudio)
		update.AudioCompletionRatio = &value
	}
	return update, true
}

// WriteBackModelFromDB 将后台创建/编辑后的模型元数据写回项目内模型仓库。
func WriteBackModelFromDB(item model.Model) error {
	if !WriteBackEnabled() {
		return nil
	}
	catalog, err := loadWritableCatalog()
	if err != nil {
		return err
	}
	provider := providerFromModel(item)
	ownerID := provider.ID
	modelDef := catalogModelFromDB(item)
	if catalog.Models == nil {
		catalog.Models = make(map[string]CatalogModel)
	}
	catalog.Models[canonicalModelKey(ownerID, modelDef.ID)] = modelDef
	if catalog.Providers == nil {
		catalog.Providers = make(map[string]CatalogProvider)
	}
	existingProvider := catalog.Providers[ownerID]
	existingProvider = mergeProvider(existingProvider, provider)
	catalog.Providers[ownerID] = existingProvider
	return WriteCatalogToRepository(RepositoryDir(), catalog)
}

// WriteBackModelPricing 将后台模型页保存的价格写回 provider model TOML。
func WriteBackModelPricing(item model.Model, req model.ModelPricingUpdateRequest) error {
	if !WriteBackEnabled() {
		return nil
	}
	catalog, err := loadWritableCatalog()
	if err != nil {
		return err
	}
	provider := providerFromModel(item)
	modelDef := catalogModelFromDB(item)
	modelDef.Pricing = catalogPricingFromUpdate(req, provider.ID)
	if req.BillingMode == model.ModelPricingModeRatio {
		modelDef.Cost = catalogCostFromUpdate(req)
	}
	if catalog.Providers == nil {
		catalog.Providers = make(map[string]CatalogProvider)
	}
	provider = mergeProvider(catalog.Providers[provider.ID], provider)
	if provider.Models == nil {
		provider.Models = make(map[string]CatalogModel)
	}
	provider.Models[modelDef.ID] = modelDef
	catalog.Providers[provider.ID] = provider
	if catalog.Models == nil {
		catalog.Models = make(map[string]CatalogModel)
	}
	catalog.Models[canonicalModelKey(provider.ID, modelDef.ID)] = modelDef
	return WriteCatalogToRepository(RepositoryDir(), catalog)
}

// WriteBackCatalog 将一个已经脱敏的标准 Catalog 写回源码仓库。
func WriteBackCatalog(catalog *Catalog) error {
	if !WriteBackEnabled() {
		return nil
	}
	if common.GetEnvOrDefaultBool(envCatalogGitAutocommit, false) {
		common.SysLog("MODEL_CATALOG_GIT_AUTOCOMMIT is reserved for future use; catalog files were written without git commit")
	}
	return WriteCatalogToRepository(RepositoryDir(), catalog)
}

func loadWritableCatalog() (*Catalog, error) {
	catalog, err := LoadRepository(RepositoryDir())
	if err == nil {
		return catalog, nil
	}
	embedded, embeddedErr := LoadEmbeddedCatalog()
	if embeddedErr == nil {
		return embedded, nil
	}
	return &Catalog{Models: map[string]CatalogModel{}, Providers: map[string]CatalogProvider{}}, nil
}

func providerFromModel(item model.Model) CatalogProvider {
	provider := CatalogProvider{ID: "local", Name: "Local", Status: "active"}
	if item.VendorID == 0 {
		return provider
	}
	vendor, err := model.GetVendorByID(item.VendorID)
	if err != nil || vendor == nil {
		return provider
	}
	provider.ID = providerIDFromName(vendor.Name)
	provider.Name = vendor.Name
	provider.Description = vendor.Description
	provider.Icon = vendor.Icon
	return provider
}

func catalogModelFromDB(item model.Model) CatalogModel {
	return CatalogModel{
		ID:          strings.TrimSpace(item.ModelName),
		Name:        strings.TrimSpace(item.ModelName),
		Description: strings.TrimSpace(item.Description),
		Icon:        strings.TrimSpace(item.Icon),
		Tags:        splitCommaList(item.Tags),
		Status:      dbModelStatus(item.Status),
		NameRule:    normalizedNameRule(item.NameRule),
		Source: CatalogSourceTrace{
			Origin:    CatalogOriginNexusTokRepository,
			UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		},
	}
}

func catalogPricingFromUpdate(req model.ModelPricingUpdateRequest, providerID string) *CatalogPricing {
	pricing := &CatalogPricing{
		BillingMode:           req.BillingMode,
		ModelPrice:            cloneFloat(req.ModelPrice),
		ModelRatio:            cloneFloat(req.ModelRatio),
		InputPricePerMillion:  cloneFloat(req.InputPricePerMillion),
		OutputPricePerMillion: cloneFloat(req.OutputPricePerMillion),
		CompletionRatio:       cloneFloat(req.CompletionRatio),
		CacheRatio:            cloneFloat(req.CacheRatio),
		CreateCacheRatio:      cloneFloat(req.CreateCacheRatio),
		ImageRatio:            cloneFloat(req.ImageRatio),
		AudioRatio:            cloneFloat(req.AudioRatio),
		AudioCompletionRatio:  cloneFloat(req.AudioCompletionRatio),
		SourceProvider:        providerID,
	}
	if req.BillingExpr != nil {
		pricing.BillingExpr = strings.TrimSpace(*req.BillingExpr)
	}
	return pricing
}

func catalogCostFromUpdate(req model.ModelPricingUpdateRequest) CatalogCost {
	cost := CatalogCost{
		Input:  cloneFloat(req.InputPricePerMillion),
		Output: cloneFloat(req.OutputPricePerMillion),
	}
	input := req.InputPricePerMillion
	if input == nil && req.ModelRatio != nil {
		value := *req.ModelRatio * 2
		input = &value
		cost.Input = &value
	}
	if input != nil {
		if req.CacheRatio != nil {
			value := *input * *req.CacheRatio
			cost.CacheRead = &value
		}
		if req.CreateCacheRatio != nil {
			value := *input * *req.CreateCacheRatio
			cost.CacheWrite = &value
		}
		if req.AudioRatio != nil {
			value := *input * *req.AudioRatio
			cost.InputAudio = &value
			if req.AudioCompletionRatio != nil {
				output := value * *req.AudioCompletionRatio
				cost.OutputAudio = &output
			}
		}
	}
	return cost
}

func mergeProvider(existing CatalogProvider, incoming CatalogProvider) CatalogProvider {
	incoming = normalizeCatalogProvider(incoming, incoming.ID)
	if existing.Models != nil {
		incoming.Models = existing.Models
	}
	if incoming.Description == "" {
		incoming.Description = existing.Description
	}
	if incoming.Icon == "" {
		incoming.Icon = existing.Icon
	}
	return incoming
}

func catalogModelDescription(provider CatalogProvider, def CatalogModel) string {
	if strings.TrimSpace(def.Description) != "" {
		return strings.TrimSpace(def.Description)
	}
	displayName := strings.TrimSpace(def.Name)
	if displayName == "" {
		displayName = strings.TrimSpace(def.ID)
	}
	if displayName == "" {
		displayName = "This model"
	}
	providerName := strings.TrimSpace(provider.Name)
	if providerName == "" {
		providerName = "NexusTok"
	}
	parts := []string{fmt.Sprintf("%s is an AI model from %s.", displayName, providerName)}
	if def.Limit.Context > 0 {
		parts = append(parts, fmt.Sprintf("Context window: %d tokens.", def.Limit.Context))
	}
	if def.Limit.Output > 0 {
		parts = append(parts, fmt.Sprintf("Max output: %d tokens.", def.Limit.Output))
	}
	return strings.Join(parts, " ")
}

func catalogModelIcon(provider CatalogProvider, def CatalogModel) string {
	if strings.TrimSpace(def.Icon) != "" {
		return strings.TrimSpace(def.Icon)
	}
	if strings.TrimSpace(provider.Icon) != "" {
		return strings.TrimSpace(provider.Icon)
	}
	return providerIcon(provider.ID, provider.Name)
}

func providerIcon(providerID, providerName string) string {
	switch strings.ToLower(strings.TrimSpace(providerID)) {
	case "openai":
		return "OpenAI.Color"
	case "anthropic":
		return "Claude.Color"
	case "google":
		return "Gemini.Color"
	case "xai":
		return "XAI.Color"
	case "moonshotai":
		return "Moonshot.Color"
	case "mistral":
		return "Mistral.Color"
	case "cohere":
		return "Cohere.Color"
	case "deepseek":
		return "DeepSeek.Color"
	case "alibaba", "alibaba-cn":
		return "Qwen.Color"
	case "openrouter":
		return "OpenRouter.Color"
	case "perplexity":
		return "Perplexity.Color"
	case "siliconflow":
		return "SiliconCloud.Color"
	case "jina":
		return "Jina.Color"
	case "aws":
		return "Aws.Color"
	case "replicate":
		return "Replicate.Color"
	}
	return strings.TrimSpace(providerName)
}

func catalogModelStatus(status string) int {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "deprecated", "disabled", "inactive":
		return 0
	default:
		return 1
	}
}

func dbModelStatus(status int) string {
	if status == 0 {
		return "disabled"
	}
	return "active"
}

func normalizedNameRule(rule int) int {
	switch rule {
	case model.NameRulePrefix, model.NameRuleContains, model.NameRuleSuffix:
		return rule
	default:
		return model.NameRuleExact
	}
}

func providerPriority(providerID string) int {
	switch strings.ToLower(strings.TrimSpace(providerID)) {
	case "openai", "anthropic", "google", "deepseek", "xai", "moonshotai", "mistral", "cohere", "alibaba", "alibaba-cn", "perplexity", "jina", "aws", "azure", "vertex-ai":
		return 0
	case "openrouter", "requesty", "vercel", "cloudflare", "siliconflow", "replicate":
		return 20
	default:
		return 10
	}
}

func providerIDFromName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	switch name {
	case "openai":
		return "openai"
	case "anthropic", "claude":
		return "anthropic"
	case "google", "gemini":
		return "google"
	case "alibaba", "qwen", "通义千问":
		return "alibaba"
	case "deepseek":
		return "deepseek"
	}
	replacer := strings.NewReplacer(" ", "-", "_", "-", "/", "-", "\\", "-", ".", "-", ":", "-")
	id := replacer.Replace(name)
	id = strings.Trim(id, "-")
	if id == "" {
		return "local"
	}
	return id
}

func splitCommaList(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return uniqueStrings(result)
}

func cloneFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneValidCost(value *float64) *float64 {
	if value == nil || !validCost(*value) {
		return nil
	}
	cloned := *value
	return &cloned
}

func validCost(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0
}

func relativeRatio(value *float64, base float64) *float64 {
	if value == nil {
		return nil
	}
	if base == 0 {
		if *value != 0 {
			return nil
		}
		zero := 0.0
		return &zero
	}
	ratio := roundRatio(*value / base)
	return &ratio
}

func roundRatio(value float64) float64 {
	return math.Round(value*1e6) / 1e6
}

// PricingProvider 返回显式定价 provider，用于内置 canonical model 直接带 pricing 的兜底场景。
func (m CatalogModel) PricingProvider() string {
	if m.Pricing == nil {
		return ""
	}
	return strings.TrimSpace(m.Pricing.SourceProvider)
}
