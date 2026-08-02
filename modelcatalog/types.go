// Package modelcatalog 提供 NexusTok 项目内模型仓库的解析、生成、写回和启动补齐能力。
//
// 这个包刻意只描述通用模型目录数据：供应商、模型元数据、公开价格参数和来源信息。
// 它不会保存渠道、账号池、用户、Token、日志、密钥或部署拓扑，避免把开发服运行时数据
// 混入 Git 仓库和发布镜像。
package modelcatalog

const (
	// RepositoryDefaultDir 是项目内模型仓库的默认源码目录。
	RepositoryDefaultDir = "modelcatalog/repository"

	// CatalogOriginNexusTokRepository 表示数据来自 NexusTok 项目内模型仓库。
	CatalogOriginNexusTokRepository = "nexustok_repository"
	// CatalogOriginModelsDevWeb 表示数据来自 models.dev 官网 catalog。
	CatalogOriginModelsDevWeb = "models_dev_web"
	// CatalogOriginModelsDevGitHub 表示数据来自 models.dev GitHub TOML 仓库。
	CatalogOriginModelsDevGitHub = "models_dev_github"
	// CatalogOriginNexusTokEmbedded 表示数据来自二进制内置的 NexusTok 仓库兜底。
	CatalogOriginNexusTokEmbedded = "nexustok_embedded"

	// FallbackStageGitHub 表示 models.dev 官网失败后降级到了 GitHub 仓库。
	FallbackStageGitHub = "github"
	// FallbackStageEmbedded 表示外部来源均失败后降级到了内置仓库。
	FallbackStageEmbedded = "embedded"
)

// Catalog 是 NexusTok 标准模型目录。
//
// JSON 结构兼容 models.dev 的 catalog.json：Models 保存 canonical 模型，
// Providers 保存 provider 元数据和 provider 维度价格。Manifest 只用于本项目内置
// 仓库版本追踪，旧解析器会自然忽略该字段。
type Catalog struct {
	Models    map[string]CatalogModel    `json:"models"`
	Providers map[string]CatalogProvider `json:"providers"`
	Manifest  CatalogManifest            `json:"manifest,omitempty"`
}

// CatalogManifest 记录生成文件的版本、hash 和统计信息。
type CatalogManifest struct {
	Name          string `json:"name" toml:"name"`
	Version       string `json:"version" toml:"version"`
	Hash          string `json:"hash" toml:"hash"`
	GeneratedAt   string `json:"generated_at" toml:"generated_at"`
	ModelCount    int    `json:"model_count" toml:"model_count"`
	ProviderCount int    `json:"provider_count" toml:"provider_count"`
}

// CatalogModel 描述 canonical 模型或 provider 目录下的模型。
//
// Cost 兼容 models.dev 的 provider model 价格结构；Pricing 是 NexusTok 扩展结构，
// 用于表达 fixed/tiered_expr 这类无法仅靠 $/1M token cost 表达的本地计费模式。
type CatalogModel struct {
	ID               string             `json:"id" toml:"id"`
	Name             string             `json:"name,omitempty" toml:"name,omitempty"`
	Description      string             `json:"description,omitempty" toml:"description,omitempty"`
	Family           string             `json:"family,omitempty" toml:"family,omitempty"`
	Status           string             `json:"status,omitempty" toml:"status,omitempty"`
	Icon             string             `json:"icon,omitempty" toml:"icon,omitempty"`
	Tags             []string           `json:"tags,omitempty" toml:"tags,omitempty"`
	Endpoints        []string           `json:"endpoints,omitempty" toml:"endpoints,omitempty"`
	NameRule         int                `json:"name_rule,omitempty" toml:"name_rule,omitempty"`
	Attachment       bool               `json:"attachment,omitempty" toml:"attachment,omitempty"`
	Reasoning        bool               `json:"reasoning,omitempty" toml:"reasoning,omitempty"`
	Temperature      *bool              `json:"temperature,omitempty" toml:"temperature,omitempty"`
	ToolCall         bool               `json:"tool_call,omitempty" toml:"tool_call,omitempty"`
	StructuredOutput bool               `json:"structured_output,omitempty" toml:"structured_output,omitempty"`
	OpenWeights      bool               `json:"open_weights,omitempty" toml:"open_weights,omitempty"`
	Knowledge        string             `json:"knowledge,omitempty" toml:"knowledge,omitempty"`
	ReleaseDate      string             `json:"release_date,omitempty" toml:"release_date,omitempty"`
	LastUpdated      string             `json:"last_updated,omitempty" toml:"last_updated,omitempty"`
	Limit            CatalogLimit       `json:"limit,omitempty" toml:"limit,omitempty"`
	Modalities       CatalogModalities  `json:"modalities,omitempty" toml:"modalities,omitempty"`
	Cost             CatalogCost        `json:"cost,omitempty" toml:"cost,omitempty"`
	Pricing          *CatalogPricing    `json:"pricing,omitempty" toml:"pricing,omitempty"`
	Source           CatalogSourceTrace `json:"source,omitempty" toml:"source,omitempty"`
}

// CatalogProvider 描述模型供应商及其 provider 维度模型价格。
type CatalogProvider struct {
	ID          string                  `json:"id" toml:"id"`
	Name        string                  `json:"name,omitempty" toml:"name,omitempty"`
	Description string                  `json:"description,omitempty" toml:"description,omitempty"`
	Icon        string                  `json:"icon,omitempty" toml:"icon,omitempty"`
	Doc         string                  `json:"doc,omitempty" toml:"doc,omitempty"`
	Status      string                  `json:"status,omitempty" toml:"status,omitempty"`
	Models      map[string]CatalogModel `json:"models,omitempty" toml:"-"`
}

// CatalogCost 使用 models.dev 约定的美元 / 1M token 价格。
type CatalogCost struct {
	Input       *float64 `json:"input,omitempty" toml:"input,omitempty"`
	Output      *float64 `json:"output,omitempty" toml:"output,omitempty"`
	CacheRead   *float64 `json:"cache_read,omitempty" toml:"cache_read,omitempty"`
	CacheWrite  *float64 `json:"cache_write,omitempty" toml:"cache_write,omitempty"`
	InputAudio  *float64 `json:"input_audio,omitempty" toml:"input_audio,omitempty"`
	OutputAudio *float64 `json:"output_audio,omitempty" toml:"output_audio,omitempty"`
}

// CatalogPricing 是 NexusTok 的本地计费扩展。
type CatalogPricing struct {
	BillingMode           string   `json:"billing_mode,omitempty" toml:"billing_mode,omitempty"`
	ModelPrice            *float64 `json:"model_price,omitempty" toml:"model_price,omitempty"`
	ModelRatio            *float64 `json:"model_ratio,omitempty" toml:"model_ratio,omitempty"`
	InputPricePerMillion  *float64 `json:"input_price_per_million,omitempty" toml:"input_price_per_million,omitempty"`
	OutputPricePerMillion *float64 `json:"output_price_per_million,omitempty" toml:"output_price_per_million,omitempty"`
	CompletionRatio       *float64 `json:"completion_ratio,omitempty" toml:"completion_ratio,omitempty"`
	CacheRatio            *float64 `json:"cache_ratio,omitempty" toml:"cache_ratio,omitempty"`
	CreateCacheRatio      *float64 `json:"create_cache_ratio,omitempty" toml:"create_cache_ratio,omitempty"`
	ImageRatio            *float64 `json:"image_ratio,omitempty" toml:"image_ratio,omitempty"`
	AudioRatio            *float64 `json:"audio_ratio,omitempty" toml:"audio_ratio,omitempty"`
	AudioCompletionRatio  *float64 `json:"audio_completion_ratio,omitempty" toml:"audio_completion_ratio,omitempty"`
	BillingExpr           string   `json:"billing_expr,omitempty" toml:"billing_expr,omitempty"`
	SourceProvider        string   `json:"source_provider,omitempty" toml:"source_provider,omitempty"`
}

// CatalogModalities 描述模型输入输出模态。
type CatalogModalities struct {
	Input  []string `json:"input,omitempty" toml:"input,omitempty"`
	Output []string `json:"output,omitempty" toml:"output,omitempty"`
}

// CatalogLimit 描述模型上下文和输出限制。
type CatalogLimit struct {
	Context int64 `json:"context,omitempty" toml:"context,omitempty"`
	Input   int64 `json:"input,omitempty" toml:"input,omitempty"`
	Output  int64 `json:"output,omitempty" toml:"output,omitempty"`
}

// CatalogSourceTrace 记录模型仓库条目的来源，便于审查某个条目是手工维护还是同步写回。
type CatalogSourceTrace struct {
	Origin      string `json:"origin,omitempty" toml:"origin,omitempty"`
	Provider    string `json:"provider,omitempty" toml:"provider,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty" toml:"updated_at,omitempty"`
	Description string `json:"description,omitempty" toml:"description,omitempty"`
}

// SeedResult 描述启动时内置仓库补齐的结果。
type SeedResult struct {
	CreatedModels  int      `json:"created_models"`
	CreatedVendors int      `json:"created_vendors"`
	PricingUpdated int      `json:"pricing_updated"`
	SkippedModels  []string `json:"skipped_models,omitempty"`
	PricingList    []string `json:"pricing_list,omitempty"`
}
