// Package model - account_pool.go
// 该文件定义了账号池（Account Pool）数据模型及相关操作
//
// 主要结构体：
// - AccountPoolGroup：账号池分组，定义一组账号的公共配置和调度策略
// - PoolAccount：池账号，存储单个账号的凭据和状态信息
// - AccountPoolAuthFile：账号池认证文件，保存 JSON 凭据原文和文件级调度配置
//
// 常量定义：
// - 认证类型（AuthType）：api_key、official_oauth、cookie、service_account、custom_json
// - 分组来源（Source）：native（原生）、cliproxyapi（CLI 代理 API）
// - 调度策略（Strategy）：round_robin（轮询）、weighted（加权）、fill_first（优先填满）、least_used（最少使用）
//
// 核心功能：
// - 账号池分组和账号的增删改查
// - 账号凭据的脱敏摘要生成
// - 账号状态管理（启用/禁用/冷却中）
// - 账号使用统计（配额、请求数、成功/失败计数）
package model

import (
	"fmt"
	"strings"

	"github.com/c1cada/NexusTok/common"
	"gorm.io/gorm"
)

const (
	// AccountPoolAuthTypeAPIKey API Key 认证方式
	AccountPoolAuthTypeAPIKey = "api_key"
	// AccountPoolAuthTypeOfficialOAuth 官方 OAuth 认证方式
	AccountPoolAuthTypeOfficialOAuth = "official_oauth"
	// AccountPoolAuthTypeCookie Cookie 认证方式
	AccountPoolAuthTypeCookie = "cookie"
	// AccountPoolAuthTypeServiceAccount 服务账号认证方式
	AccountPoolAuthTypeServiceAccount = "service_account"
	// AccountPoolAuthTypeCustomJSON 自定义 JSON 认证方式
	AccountPoolAuthTypeCustomJSON = "custom_json"

	// AccountPoolGroupSourceNative 原生分组（手动创建）
	AccountPoolGroupSourceNative = "native"
	// AccountPoolGroupSourceCLIProxyAPI CLI 代理 API 来源
	AccountPoolGroupSourceCLIProxyAPI = "cliproxyapi"

	// AccountPoolStrategyRoundRobin 轮询调度策略
	AccountPoolStrategyRoundRobin = "round_robin"
	// AccountPoolStrategyWeighted 加权调度策略
	AccountPoolStrategyWeighted = "weighted"
	// AccountPoolStrategyFillFirst 优先填满调度策略
	AccountPoolStrategyFillFirst = "fill_first"
	// AccountPoolStrategyLeastUsed 最少使用调度策略
	AccountPoolStrategyLeastUsed = "least_used"

	// AccountPoolAuthFileFormatNative 原生 JSON 认证文件格式
	AccountPoolAuthFileFormatNative = "native"
	// AccountPoolAuthFileFormatSub2 sub2 导出的 JSON 包装格式
	AccountPoolAuthFileFormatSub2 = "sub2"
	// AccountPoolAuthFileFormatNewAPI NewAPI 导出的 JSON 包装格式
	AccountPoolAuthFileFormatNewAPI = "newapi"
)

// AccountPoolGroup 账号池分组模型
// 定义一组账号的公共配置，包括平台、认证类型、调度策略等
type AccountPoolGroup struct {
	Id           int     `json:"id"`                                                                          // 分组 ID
	Name         string  `json:"name" gorm:"type:varchar(255);index;not null"`                                // 分组名称
	Platform     string  `json:"platform" gorm:"type:varchar(64);index;not null"`                             // 平台标识（如 openai、claude）
	AuthType     string  `json:"auth_type" gorm:"type:varchar(64);index;not null"`                            // 认证类型
	Source       string  `json:"source" gorm:"type:varchar(64);default:'native';index"`                       // 分组来源
	ExternalKey  string  `json:"external_group_key" gorm:"column:external_group_key;type:varchar(255);index"` // 外部分组标识
	Status       int     `json:"status" gorm:"default:1;index"`                                               // 状态（1=启用，2=禁用）
	Strategy     string  `json:"strategy" gorm:"type:varchar(64);default:'round_robin'"`                      // 调度策略
	Models       string  `json:"models" gorm:"type:text"`                                                     // 支持的模型列表（逗号分隔）
	Group        string  `json:"group" gorm:"column:group;type:varchar(255);index"`                           // 关联的渠道分组
	ModelMapping *string `json:"model_mapping" gorm:"type:text"`                                              // 模型映射
	Settings     string  `json:"settings" gorm:"type:text"`                                                   // 额外设置（JSON）
	CreatedTime  int64   `json:"created_time" gorm:"bigint"`                                                  // 创建时间
	UpdatedTime  int64   `json:"updated_time" gorm:"bigint"`                                                  // 更新时间

	Stats map[string]int64 `json:"stats,omitempty" gorm:"-"` // 统计信息（非持久化，运行时附加）
}

// PoolAccount 池账号模型
// 存储单个账号的凭据、状态和使用统计信息
type PoolAccount struct {
	Id                 int     `json:"id"`                                                                  // 账号 ID
	PoolGroupId        int     `json:"pool_group_id" gorm:"index;not null"`                                 // 所属分组 ID
	Name               string  `json:"name" gorm:"type:varchar(255);index;not null"`                        // 账号名称
	Platform           string  `json:"platform" gorm:"type:varchar(64);index;not null"`                     // 平台标识
	AuthType           string  `json:"auth_type" gorm:"type:varchar(64);index;not null"`                    // 认证类型
	Credentials        string  `json:"credentials" gorm:"type:text;not null"`                               // 加密存储的凭据
	CredentialSummary  string  `json:"credential_summary" gorm:"type:text"`                                 // 凭据摘要（脱敏）
	CredentialProvider string  `json:"credential_provider" gorm:"type:varchar(64);index"`                   // 凭据提供方
	CredentialLabel    string  `json:"credential_label" gorm:"type:varchar(255)"`                           // 凭据标签
	CredentialMetadata string  `json:"credential_metadata" gorm:"type:text"`                                // 凭据元数据（JSON）
	CredentialAttrs    string  `json:"credential_attributes" gorm:"column:credential_attributes;type:text"` // 凭据属性（JSON）
	Status             int     `json:"status" gorm:"default:1;index"`                                       // 状态
	StatusMessage      string  `json:"status_message" gorm:"type:text"`                                     // 状态说明
	Schedulable        bool    `json:"schedulable" gorm:"default:true;index"`                               // 是否可调度
	Unavailable        bool    `json:"unavailable" gorm:"default:false;index"`                              // 是否不可用
	Models             string  `json:"models" gorm:"type:text"`                                             // 支持的模型
	Group              string  `json:"group" gorm:"column:group;type:varchar(255);index"`                   // 关联的渠道分组
	Priority           int64   `json:"priority" gorm:"bigint;default:0;index"`                              // 优先级
	Weight             int     `json:"weight" gorm:"default:1;index"`                                       // 权重
	MaxConcurrency     int     `json:"max_concurrency" gorm:"default:0"`                                    // 最大并发数（0=不限）
	Proxy              string  `json:"proxy" gorm:"type:text"`                                              // 代理地址
	BaseURL            *string `json:"base_url" gorm:"column:base_url;default:''"`                          // 自定义 API 基础 URL
	OpenAIOrganization *string `json:"openai_organization"`                                                 // OpenAI 组织 ID
	Other              string  `json:"other"`                                                               // 其他配置
	Setting            *string `json:"setting" gorm:"type:text"`                                            // 账号级设置
	OtherSettings      string  `json:"settings" gorm:"column:settings;type:text"`                           // 额外设置
	ModelMapping       *string `json:"model_mapping" gorm:"type:text"`                                      // 账号级模型映射
	ParamOverride      *string `json:"param_override" gorm:"type:text"`                                     // 参数覆盖
	HeaderOverride     *string `json:"header_override" gorm:"type:text"`                                    // 请求头覆盖
	StatusCodeMapping  *string `json:"status_code_mapping" gorm:"type:varchar(1024);default:''"`            // 状态码映射
	LastUsedTime       int64   `json:"last_used_time" gorm:"bigint;default:0;index"`                        // 最后使用时间
	UsedQuota          int64   `json:"used_quota" gorm:"bigint;default:0"`                                  // 已用配额
	RateLimitedUntil   int64   `json:"rate_limited_until" gorm:"bigint;default:0;index"`                    // 限流截止时间
	OverloadUntil      int64   `json:"overload_until" gorm:"bigint;default:0;index"`                        // 过载截止时间
	TempDisabledUntil  int64   `json:"temp_disabled_until" gorm:"bigint;default:0;index"`                   // 临时禁用截止时间
	DisabledReason     string  `json:"disabled_reason" gorm:"type:text"`                                    // 禁用原因
	LastError          string  `json:"last_error" gorm:"type:text"`                                         // 最近错误信息
	QuotaSnapshot      string  `json:"quota_snapshot" gorm:"type:text"`                                     // 配额快照
	ModelStates        string  `json:"model_states" gorm:"type:text"`                                       // 模型状态（JSON）
	LastCheckedTime    int64   `json:"last_checked_time" gorm:"bigint;default:0;index"`                     // 最后人工检测时间
	LastRefreshedTime  int64   `json:"last_refreshed_time" gorm:"bigint;default:0;index"`                   // 最后刷新时间
	NextRefreshTime    int64   `json:"next_refresh_time" gorm:"bigint;default:0;index"`                     // 下次刷新时间
	NextRetryTime      int64   `json:"next_retry_time" gorm:"bigint;default:0;index"`                       // 下次重试时间
	SuccessCount       int64   `json:"success_count" gorm:"bigint;default:0"`                               // 成功请求数
	FailedCount        int64   `json:"failed_count" gorm:"bigint;default:0"`                                // 失败请求数
	RecentRequests     string  `json:"recent_requests" gorm:"type:text"`                                    // 最近请求记录
	CreatedTime        int64   `json:"created_time" gorm:"bigint"`                                          // 创建时间
	UpdatedTime        int64   `json:"updated_time" gorm:"bigint"`                                          // 更新时间
}

// AccountPoolAuthFile 账号池认证文件模型。
// 该模型把“导入的 JSON 文件”提升为一等管理对象：原文加密保存，列表和调度只使用
// 脱敏摘要及文件级配置。PoolAccount 仍是热路径实际调度对象，PoolAccountId 用来
// 关联由该文件生成的账号，后续编辑分组、代理、优先级时可同步更新到调度层。
type AccountPoolAuthFile struct {
	Id                 int     `json:"id"`                                                                  // 认证文件 ID
	Name               string  `json:"name" gorm:"type:varchar(255);index;not null"`                        // 文件显示名称
	SourcePlatform     string  `json:"source_platform" gorm:"type:varchar(64);index"`                       // 来源平台，如 sub2、newapi、native
	Format             string  `json:"format" gorm:"type:varchar(64);default:'native';index"`               // 解析格式标识
	Provider           string  `json:"provider" gorm:"type:varchar(64);index;not null"`                     // 凭据提供方，如 codex、xai
	Platform           string  `json:"platform" gorm:"type:varchar(64);index;not null"`                     // 本地调度平台，通常与 Provider 一致
	AuthType           string  `json:"auth_type" gorm:"type:varchar(64);index;not null"`                    // 认证类型
	PoolGroupId        int     `json:"pool_group_id" gorm:"index;not null"`                                 // 关联账号池分组
	PoolAccountId      int     `json:"pool_account_id" gorm:"index"`                                        // 由该文件生成的池账号
	Status             int     `json:"status" gorm:"default:1;index"`                                       // 状态（1=启用，2=禁用）
	FileDigest         string  `json:"file_digest" gorm:"type:varchar(64);uniqueIndex"`                     // 原始 JSON 内容 SHA256，用于去重
	EncryptedContent   string  `json:"-" gorm:"type:text;not null"`                                         // 加密后的认证文件原文
	CredentialSummary  string  `json:"credential_summary" gorm:"type:text"`                                 // 凭据摘要（脱敏）
	CredentialMetadata string  `json:"credential_metadata" gorm:"type:text"`                                // 解析出的元数据 JSON
	CredentialAttrs    string  `json:"credential_attributes" gorm:"column:credential_attributes;type:text"` // 解析出的属性 JSON
	AccountGroups      string  `json:"account_groups" gorm:"type:text"`                                     // 文件级调用分组，逗号分隔
	Models             string  `json:"models" gorm:"type:text"`                                             // 文件级模型限制，逗号分隔
	Proxy              string  `json:"proxy" gorm:"type:text"`                                              // 文件级代理
	BaseURL            *string `json:"base_url" gorm:"column:base_url;default:''"`                          // 文件级基础 URL 覆盖
	Priority           int64   `json:"priority" gorm:"bigint;default:0;index"`                              // 文件级优先级
	Weight             int     `json:"weight" gorm:"default:1;index"`                                       // 文件级权重
	MaxConcurrency     int     `json:"max_concurrency" gorm:"default:0"`                                    // 文件级最大并发数（0=不限）
	LastImportedTime   int64   `json:"last_imported_time" gorm:"bigint;default:0;index"`                    // 最近导入时间
	CreatedTime        int64   `json:"created_time" gorm:"bigint"`                                          // 创建时间
	UpdatedTime        int64   `json:"updated_time" gorm:"bigint"`                                          // 更新时间
}

// PoolAccountUsageLog 记录原生账号池账号在 Relay 热路径中的一次使用结果。
//
// 该表刻意使用普通列而不是复用通用日志里的 JSON admin_info：
// 1. 账号池管理页需要按账号池组、账号、渠道、模型、请求 ID 和成功状态稳定筛选；
// 2. SQLite、MySQL、PostgreSQL 对 JSON 查询能力和索引能力差异较大，普通列更容易跨库兼容；
// 3. 日志库可能与主业务库分离，因此这里保存账号、分组、渠道名称快照，查询时不依赖跨库关联。
type PoolAccountUsageLog struct {
	Id                  int    `json:"id"`
	CreatedAt           int64  `json:"created_at" gorm:"bigint;index:idx_pool_account_usage_created_at"`
	PoolGroupId         int    `json:"pool_group_id" gorm:"index:idx_pool_account_usage_group"`
	PoolGroupName       string `json:"pool_group_name" gorm:"type:varchar(255)"`
	PoolAccountId       int    `json:"pool_account_id" gorm:"index:idx_pool_account_usage_account"`
	PoolAccountName     string `json:"pool_account_name" gorm:"type:varchar(255)"`
	PoolAccountAuthType string `json:"pool_account_auth_type" gorm:"type:varchar(64);index"`
	ChannelId           int    `json:"channel_id" gorm:"index:idx_pool_account_usage_channel"`
	ChannelName         string `json:"channel_name" gorm:"type:varchar(255)"`
	ModelName           string `json:"model_name" gorm:"type:varchar(255);index"`
	UserId              int    `json:"user_id" gorm:"index"`
	Username            string `json:"username" gorm:"type:varchar(255);index"`
	TokenId             int    `json:"token_id" gorm:"index"`
	TokenName           string `json:"token_name" gorm:"type:varchar(255)"`
	Group               string `json:"group" gorm:"column:group;type:varchar(255);index"`
	Quota               int    `json:"quota" gorm:"default:0"`
	PromptTokens        int    `json:"prompt_tokens" gorm:"default:0"`
	CompletionTokens    int    `json:"completion_tokens" gorm:"default:0"`
	UseTime             int    `json:"use_time" gorm:"default:0"`
	IsStream            bool   `json:"is_stream" gorm:"default:false"`
	Success             bool   `json:"success" gorm:"index"`
	StatusCode          int    `json:"status_code" gorm:"default:0;index"`
	ErrorCode           string `json:"error_code" gorm:"type:varchar(128);index"`
	ErrorMessage        string `json:"error_message" gorm:"type:text"`
	RequestId           string `json:"request_id,omitempty" gorm:"type:varchar(64);index:idx_pool_account_usage_request_id"`
	UpstreamRequestId   string `json:"upstream_request_id,omitempty" gorm:"type:varchar(128);index:idx_pool_account_usage_upstream_request_id"`
	RetryIndex          int    `json:"retry_index" gorm:"default:0"`
}

// PoolAccountUsageLogRecord 描述一次账号池使用日志写入请求。
// 调用方只传递当前热路径已经持有的快照值，避免日志写入再反查主库并放大请求延迟。
type PoolAccountUsageLogRecord struct {
	PoolGroupId         int
	PoolGroupName       string
	PoolAccountId       int
	PoolAccountName     string
	PoolAccountAuthType string
	ChannelId           int
	ChannelName         string
	ModelName           string
	UserId              int
	Username            string
	TokenId             int
	TokenName           string
	Group               string
	Quota               int
	PromptTokens        int
	CompletionTokens    int
	UseTime             int
	IsStream            bool
	Success             bool
	StatusCode          int
	ErrorCode           string
	ErrorMessage        string
	RequestId           string
	UpstreamRequestId   string
	RetryIndex          int
}

// PoolAccountUsageLogFilter 是账号池使用日志的分页筛选条件。
// 仅使用普通等值和 LIKE 条件，保证三类数据库行为一致。
type PoolAccountUsageLogFilter struct {
	PoolGroupId       int
	PoolAccountId     int
	ChannelId         int
	UserId            int
	Success           *bool
	StartTimestamp    int64
	EndTimestamp      int64
	ModelName         string
	RequestId         string
	UpstreamRequestId string
	Search            string
	StartIdx          int
	Limit             int
}

// BeforeCreate GORM 钩子：创建前自动设置时间和规范化字段
func (group *AccountPoolGroup) BeforeCreate(tx *gorm.DB) error {
	_ = tx
	now := common.GetTimestamp()
	if group.CreatedTime == 0 {
		group.CreatedTime = now
	}
	group.UpdatedTime = now
	group.normalize()
	return nil
}

// BeforeUpdate GORM 钩子：更新前自动设置更新时间和规范化字段
func (group *AccountPoolGroup) BeforeUpdate(tx *gorm.DB) error {
	_ = tx
	group.UpdatedTime = common.GetTimestamp()
	group.normalize()
	return nil
}

// normalize 规范化分组字段（小写化、去空格、设置默认值）
func (group *AccountPoolGroup) normalize() {
	if group.Status == 0 {
		group.Status = common.ChannelStatusEnabled
	}
	group.Platform = strings.ToLower(strings.TrimSpace(group.Platform))
	group.AuthType = strings.ToLower(strings.TrimSpace(group.AuthType))
	if group.AuthType == "" {
		group.AuthType = AccountPoolAuthTypeAPIKey
	}
	group.Source = strings.ToLower(strings.TrimSpace(group.Source))
	if group.Source == "" {
		group.Source = AccountPoolGroupSourceNative
	}
	group.ExternalKey = strings.TrimSpace(group.ExternalKey)
	group.Strategy = strings.ToLower(strings.TrimSpace(group.Strategy))
	if group.Strategy == "" {
		group.Strategy = AccountPoolStrategyRoundRobin
	}
}

// BeforeCreate GORM 钩子：创建前自动设置时间和规范化字段
func (account *PoolAccount) BeforeCreate(tx *gorm.DB) error {
	_ = tx
	now := common.GetTimestamp()
	if account.CreatedTime == 0 {
		account.CreatedTime = now
	}
	account.UpdatedTime = now
	account.normalize()
	return nil
}

// BeforeUpdate GORM 钩子：更新前自动设置更新时间和规范化字段
func (account *PoolAccount) BeforeUpdate(tx *gorm.DB) error {
	_ = tx
	account.UpdatedTime = common.GetTimestamp()
	account.normalize()
	return nil
}

// BeforeCreate GORM 钩子：创建认证文件前设置时间戳并规范化字段。
func (authFile *AccountPoolAuthFile) BeforeCreate(tx *gorm.DB) error {
	_ = tx
	now := common.GetTimestamp()
	if authFile.CreatedTime == 0 {
		authFile.CreatedTime = now
	}
	authFile.UpdatedTime = now
	if authFile.LastImportedTime == 0 {
		authFile.LastImportedTime = now
	}
	authFile.normalize()
	return nil
}

// BeforeUpdate GORM 钩子：更新认证文件前刷新时间戳并规范化字段。
func (authFile *AccountPoolAuthFile) BeforeUpdate(tx *gorm.DB) error {
	_ = tx
	authFile.UpdatedTime = common.GetTimestamp()
	authFile.normalize()
	return nil
}

// normalize 规范化认证文件字段，保证列表筛选和调度字段稳定。
func (authFile *AccountPoolAuthFile) normalize() {
	if authFile.Status == 0 {
		authFile.Status = common.ChannelStatusEnabled
	}
	authFile.Name = strings.TrimSpace(authFile.Name)
	authFile.SourcePlatform = strings.ToLower(strings.TrimSpace(authFile.SourcePlatform))
	authFile.Format = strings.ToLower(strings.TrimSpace(authFile.Format))
	if authFile.Format == "" {
		authFile.Format = AccountPoolAuthFileFormatNative
	}
	authFile.Provider = strings.ToLower(strings.TrimSpace(authFile.Provider))
	authFile.Platform = strings.ToLower(strings.TrimSpace(authFile.Platform))
	if authFile.Platform == "" {
		authFile.Platform = authFile.Provider
	}
	authFile.AuthType = strings.ToLower(strings.TrimSpace(authFile.AuthType))
	if authFile.AuthType == "" {
		authFile.AuthType = AccountPoolAuthTypeCustomJSON
	}
	if authFile.Weight <= 0 {
		authFile.Weight = 1
	}
	if authFile.MaxConcurrency < 0 {
		authFile.MaxConcurrency = 0
	}
	authFile.FileDigest = strings.TrimSpace(authFile.FileDigest)
	authFile.AccountGroups = normalizeAccountPoolCSV(authFile.AccountGroups)
	authFile.Models = normalizeAccountPoolCSV(authFile.Models)
	authFile.Proxy = strings.TrimSpace(authFile.Proxy)
}

// normalize 规范化账号字段（小写化、去空格、设置默认值）
func (account *PoolAccount) normalize() {
	if account.Status == 0 {
		account.Status = common.ChannelStatusEnabled
	}
	if account.Weight <= 0 {
		account.Weight = 1
	}
	account.Platform = strings.ToLower(strings.TrimSpace(account.Platform))
	account.AuthType = strings.ToLower(strings.TrimSpace(account.AuthType))
	if account.AuthType == "" {
		account.AuthType = AccountPoolAuthTypeAPIKey
	}
	account.CredentialProvider = strings.ToLower(strings.TrimSpace(account.CredentialProvider))
	if account.CredentialProvider == "" {
		account.CredentialProvider = account.Platform
	}
	account.CredentialLabel = strings.TrimSpace(account.CredentialLabel)
}

// GetWeight 获取账号权重，最小为 1
func (account *PoolAccount) GetWeight() int {
	if account == nil || account.Weight <= 0 {
		return 1
	}
	return account.Weight
}

// IsCoolingDown 判断账号是否处于冷却状态
// 包含限流、过载、临时禁用、重试等待四种冷却条件
func (account *PoolAccount) IsCoolingDown(now int64) bool {
	if account == nil {
		return true
	}
	return account.RateLimitedUntil > now || account.OverloadUntil > now || account.TempDisabledUntil > now || account.NextRetryTime > now
}

// GetDecryptedCredentials 解密并返回账号凭据
func (account *PoolAccount) GetDecryptedCredentials() (string, error) {
	if account == nil {
		return "", nil
	}
	return common.DecryptSensitiveString(account.Credentials)
}

// GetCredentialProvider 获取凭据提供方名称，未设置时回退到平台名
func (account *PoolAccount) GetCredentialProvider() string {
	if account == nil {
		return ""
	}
	provider := strings.TrimSpace(account.CredentialProvider)
	if provider == "" {
		provider = account.Platform
	}
	return strings.ToLower(strings.TrimSpace(provider))
}

// GetCredentialLabel 获取凭据标签，未设置时回退到账号名称
func (account *PoolAccount) GetCredentialLabel() string {
	if account == nil {
		return ""
	}
	label := strings.TrimSpace(account.CredentialLabel)
	if label == "" {
		label = strings.TrimSpace(account.Name)
	}
	return label
}

// GetBaseURL 获取账号的自定义 API 基础 URL，未设置时返回默认值
func (account *PoolAccount) GetBaseURL(defaultBaseURL string) string {
	if account != nil && account.BaseURL != nil && strings.TrimSpace(*account.BaseURL) != "" {
		return *account.BaseURL
	}
	return defaultBaseURL
}

// GetModelMapping 获取账号的模型映射，未设置时返回默认值
func (account *PoolAccount) GetModelMapping(defaultMapping string) string {
	if account != nil && account.ModelMapping != nil && strings.TrimSpace(*account.ModelMapping) != "" {
		return *account.ModelMapping
	}
	return defaultMapping
}

// GetStatusCodeMapping 获取账号的状态码映射，未设置时返回默认值
func (account *PoolAccount) GetStatusCodeMapping(defaultMapping string) string {
	if account != nil && account.StatusCodeMapping != nil && strings.TrimSpace(*account.StatusCodeMapping) != "" {
		return *account.StatusCodeMapping
	}
	return defaultMapping
}

// GetSetting 获取账号的自定义设置，未设置时返回默认值
func (account *PoolAccount) GetSetting(defaultSetting string) string {
	if account != nil && account.Setting != nil && strings.TrimSpace(*account.Setting) != "" {
		return *account.Setting
	}
	return defaultSetting
}

// GetOtherSettings 获取账号的额外设置，未设置时返回默认值
func (account *PoolAccount) GetOtherSettings(defaultSettings string) string {
	if account != nil && strings.TrimSpace(account.OtherSettings) != "" {
		return account.OtherSettings
	}
	return defaultSettings
}

// GetParamOverride 获取账号的参数覆盖配置，未设置时返回默认值
func (account *PoolAccount) GetParamOverride(defaultOverride *string) *string {
	if account != nil && account.ParamOverride != nil && strings.TrimSpace(*account.ParamOverride) != "" {
		return account.ParamOverride
	}
	return defaultOverride
}

// GetHeaderOverride 获取账号的请求头覆盖配置，未设置时返回默认值
func (account *PoolAccount) GetHeaderOverride(defaultOverride *string) *string {
	if account != nil && account.HeaderOverride != nil && strings.TrimSpace(*account.HeaderOverride) != "" {
		return account.HeaderOverride
	}
	return defaultOverride
}

// NormalizeAccountPoolCredentialSummary 规范化凭据摘要
// 将凭据中的敏感字段（token、key）脱敏后生成摘要
func NormalizeAccountPoolCredentialSummary(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	summary := map[string]string{}
	var obj map[string]interface{}
	if err := common.UnmarshalJsonStr(raw, &obj); err == nil && len(obj) > 0 {
		for _, key := range []string{"api_key", "access_token", "refresh_token", "account_id", "email", "project_id", "organization_id"} {
			if value, ok := obj[key]; ok && value != nil {
				text := strings.TrimSpace(fmt.Sprintf("%v", value))
				if text == "" {
					continue
				}
				if strings.Contains(key, "token") || strings.Contains(key, "key") {
					text = MaskTokenKey(text)
				}
				summary[key] = text
			}
		}
		if len(summary) > 0 {
			data, err := common.Marshal(summary)
			if err == nil {
				return string(data)
			}
		}
	}
	return MaskTokenKey(raw)
}

// GetAccountPoolGroupById 根据 ID 获取账号池分组
func GetAccountPoolGroupById(groupID int) (*AccountPoolGroup, error) {
	group := &AccountPoolGroup{}
	err := DB.Where("id = ?", groupID).First(group).Error
	return group, err
}

// GetAccountPoolGroups 分页查询账号池分组列表
// 支持按状态筛选和关键词搜索（名称、平台、认证类型、模型）
func GetAccountPoolGroups(page int, pageSize int, status int, search string) ([]*AccountPoolGroup, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	query := DB.Model(&AccountPoolGroup{})
	if status > 0 {
		query = query.Where("status = ?", status)
	}
	if strings.TrimSpace(search) != "" {
		like := "%" + strings.TrimSpace(search) + "%"
		query = query.Where("name LIKE ? OR platform LIKE ? OR auth_type LIKE ? OR models LIKE ?", like, like, like, like)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	groups := []*AccountPoolGroup{}
	err := query.Order("id DESC").Limit(pageSize).Offset((page - 1) * pageSize).Find(&groups).Error
	return groups, total, err
}

// AttachAccountPoolGroupStats 为分组列表附加账号统计信息
// 统计每个分组下的账号总数、启用数、禁用数、冷却中数
func AttachAccountPoolGroupStats(groups []*AccountPoolGroup) {
	groupIDs := make([]int, 0, len(groups))
	for _, group := range groups {
		if group != nil {
			groupIDs = append(groupIDs, group.Id)
		}
	}
	stats, err := CountPoolAccountsByGroupIDs(groupIDs)
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to count account pool group stats: %v", err))
		return
	}
	for _, group := range groups {
		if group == nil {
			continue
		}
		group.Stats = stats[group.Id]
	}
}

// CountPoolAccountsByGroupIDs 批量统计多个分组的账号状态
// 返回 map[groupId]map[statusKey]count 的嵌套映射
func CountPoolAccountsByGroupIDs(groupIDs []int) (map[int]map[string]int64, error) {
	result := make(map[int]map[string]int64)
	uniqueIDs := make([]int, 0, len(groupIDs))
	seen := map[int]bool{}
	for _, id := range groupIDs {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		uniqueIDs = append(uniqueIDs, id)
		result[id] = newPoolAccountStats()
	}
	if len(uniqueIDs) == 0 {
		return result, nil
	}
	now := common.GetTimestamp()
	var accounts []PoolAccount
	if err := DB.Select("pool_group_id", "status", "schedulable", "unavailable", "rate_limited_until", "overload_until", "temp_disabled_until", "next_retry_time").Where("pool_group_id IN ?", uniqueIDs).Find(&accounts).Error; err != nil {
		return result, err
	}
	for _, account := range accounts {
		stats := result[account.PoolGroupId]
		if stats == nil {
			stats = newPoolAccountStats()
			result[account.PoolGroupId] = stats
		}
		stats["total"]++
		if account.Status == common.ChannelStatusEnabled && account.Schedulable && !account.Unavailable {
			stats["enabled"]++
			if account.IsCoolingDown(now) {
				stats["cooldown"]++
			}
		} else {
			stats["disabled"]++
		}
	}
	return result, nil
}

// newPoolAccountStats 创建空的池账号统计映射
func newPoolAccountStats() map[string]int64 {
	return map[string]int64{
		"total":    0,
		"enabled":  0,
		"disabled": 0,
		"cooldown": 0,
	}
}

// GetPoolAccountById 根据 ID 获取池账号
func GetPoolAccountById(accountID int) (*PoolAccount, error) {
	account := &PoolAccount{}
	err := DB.Where("id = ?", accountID).First(account).Error
	return account, err
}

// GetAccountPoolAuthFileById 根据 ID 获取认证文件。
func GetAccountPoolAuthFileById(authFileID int) (*AccountPoolAuthFile, error) {
	authFile := &AccountPoolAuthFile{}
	err := DB.Where("id = ?", authFileID).First(authFile).Error
	return authFile, err
}

// GetAccountPoolAuthFiles 分页查询原生账号池认证文件。
// 支持按状态、账号池分组、提供方和关键词筛选，便于后台把“文件”和“生成账号”
// 分开管理，同时仍能定位到实际调度对象。
func GetAccountPoolAuthFiles(page int, pageSize int, status int, poolGroupID int, provider string, search string) ([]*AccountPoolAuthFile, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	query := DB.Model(&AccountPoolAuthFile{})
	if status > 0 {
		query = query.Where("status = ?", status)
	}
	if poolGroupID > 0 {
		query = query.Where("pool_group_id = ?", poolGroupID)
	}
	if strings.TrimSpace(provider) != "" {
		query = query.Where("provider = ? OR platform = ?", strings.ToLower(strings.TrimSpace(provider)), strings.ToLower(strings.TrimSpace(provider)))
	}
	if strings.TrimSpace(search) != "" {
		like := "%" + strings.TrimSpace(search) + "%"
		query = query.Where("name LIKE ? OR provider LIKE ? OR platform LIKE ? OR source_platform LIKE ? OR credential_summary LIKE ? OR account_groups LIKE ?", like, like, like, like, like, like)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	authFiles := []*AccountPoolAuthFile{}
	err := query.Order("id DESC").Limit(pageSize).Offset((page - 1) * pageSize).Find(&authFiles).Error
	return authFiles, total, err
}

// GetPoolAccounts 分页查询池账号列表
// 支持按状态筛选和关键词搜索（名称、凭据摘要、模型）
func GetPoolAccounts(groupID int, page int, pageSize int, status int, search string) ([]*PoolAccount, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	query := DB.Model(&PoolAccount{}).Where("pool_group_id = ?", groupID)
	if status > 0 {
		query = query.Where("status = ?", status)
	}
	if strings.TrimSpace(search) != "" {
		like := "%" + strings.TrimSpace(search) + "%"
		query = query.Where("name LIKE ? OR credential_summary LIKE ? OR models LIKE ?", like, like, like)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	accounts := []*PoolAccount{}
	err := query.Order("priority DESC").Order("id DESC").Limit(pageSize).Offset((page - 1) * pageSize).Find(&accounts).Error
	return accounts, total, err
}

func normalizeAccountPoolCSV(value string) string {
	parts := splitAccountPoolCSV(value)
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ",")
}

func splitAccountPoolCSV(value string) []string {
	seen := map[string]struct{}{}
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '\n' || r == ';'
	})
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.Join(strings.Fields(strings.TrimSpace(part)), " ")
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}

// UpdatePoolAccountStatus 更新池账号状态
// 启用时自动清除限流、过载、临时禁用等状态
func UpdatePoolAccountStatus(accountID int, status int, reason string, schedulable *bool) error {
	updates := map[string]interface{}{
		"status":          status,
		"disabled_reason": reason,
		"status_message":  reason,
	}
	if status == common.ChannelStatusEnabled {
		updates["rate_limited_until"] = 0
		updates["overload_until"] = 0
		updates["temp_disabled_until"] = 0
		updates["next_retry_time"] = 0
		updates["unavailable"] = false
		updates["last_error"] = ""
		updates["status_message"] = ""
		if schedulable == nil {
			updates["schedulable"] = true
		}
	}
	if schedulable != nil {
		updates["schedulable"] = *schedulable
	}
	return DB.Model(&PoolAccount{}).Where("id = ?", accountID).Updates(updates).Error
}

// UpdatePoolAccountErrorState 更新池账号的错误状态字段
func UpdatePoolAccountErrorState(accountID int, updates map[string]interface{}) error {
	if accountID <= 0 || len(updates) == 0 {
		return nil
	}
	return DB.Model(&PoolAccount{}).Where("id = ?", accountID).Updates(updates).Error
}

// TouchPoolAccount 更新池账号的最后使用时间
func TouchPoolAccount(accountID int) {
	if accountID <= 0 {
		return
	}
	if err := DB.Model(&PoolAccount{}).Where("id = ?", accountID).Update("last_used_time", common.GetTimestamp()).Error; err != nil {
		common.SysLog(fmt.Sprintf("failed to update pool account last_used_time: account_id=%d, error=%v", accountID, err))
	}
}

// AddPoolAccountUsedQuota 增加池账号的已用配额
func AddPoolAccountUsedQuota(accountID int, quota int64) {
	if accountID <= 0 || quota <= 0 {
		return
	}
	if err := DB.Model(&PoolAccount{}).Where("id = ?", accountID).Update("used_quota", gorm.Expr("used_quota + ?", quota)).Error; err != nil {
		common.SysLog(fmt.Sprintf("failed to update pool account used_quota: account_id=%d, quota=%d, error=%v", accountID, quota, err))
	}
}

// RecordPoolAccountRequest 记录池账号的请求结果
// 成功时增加成功计数并清除不可用状态，失败时增加失败计数
func RecordPoolAccountRequest(accountID int, success bool, recentRequests string) {
	if accountID <= 0 {
		return
	}
	updates := map[string]interface{}{
		"recent_requests": recentRequests,
	}
	if success {
		updates["success_count"] = gorm.Expr("success_count + ?", 1)
		updates["unavailable"] = false
		updates["status_message"] = ""
	} else {
		updates["failed_count"] = gorm.Expr("failed_count + ?", 1)
	}
	if err := DB.Model(&PoolAccount{}).Where("id = ?", accountID).Updates(updates).Error; err != nil {
		common.SysLog(fmt.Sprintf("failed to update pool account request runtime: account_id=%d, error=%v", accountID, err))
	}
}

// RecordPoolAccountUsageLog 写入账号池账号的一次使用日志。
// 写入失败不应影响 Relay 主流程，因此只记录系统日志并返回。
func RecordPoolAccountUsageLog(record PoolAccountUsageLogRecord) {
	if record.PoolAccountId <= 0 {
		return
	}
	logDB := LOG_DB
	if logDB == nil {
		logDB = DB
	}
	if logDB == nil {
		return
	}
	log := &PoolAccountUsageLog{
		CreatedAt:           common.GetTimestamp(),
		PoolGroupId:         record.PoolGroupId,
		PoolGroupName:       record.PoolGroupName,
		PoolAccountId:       record.PoolAccountId,
		PoolAccountName:     record.PoolAccountName,
		PoolAccountAuthType: record.PoolAccountAuthType,
		ChannelId:           record.ChannelId,
		ChannelName:         record.ChannelName,
		ModelName:           record.ModelName,
		UserId:              record.UserId,
		Username:            record.Username,
		TokenId:             record.TokenId,
		TokenName:           record.TokenName,
		Group:               record.Group,
		Quota:               record.Quota,
		PromptTokens:        record.PromptTokens,
		CompletionTokens:    record.CompletionTokens,
		UseTime:             record.UseTime,
		IsStream:            record.IsStream,
		Success:             record.Success,
		StatusCode:          record.StatusCode,
		ErrorCode:           strings.TrimSpace(record.ErrorCode),
		ErrorMessage:        strings.TrimSpace(record.ErrorMessage),
		RequestId:           strings.TrimSpace(record.RequestId),
		UpstreamRequestId:   strings.TrimSpace(record.UpstreamRequestId),
		RetryIndex:          record.RetryIndex,
	}
	if err := logDB.Create(log).Error; err != nil {
		common.SysLog(fmt.Sprintf("failed to record pool account usage log: account_id=%d, error=%v", record.PoolAccountId, err))
	}
}

// GetPoolAccountUsageLogs 分页查询账号池使用日志。
// 管理员页面直接消费该接口，避免从通用日志的 JSON other 字段反向解析账号池信息。
func GetPoolAccountUsageLogs(filter PoolAccountUsageLogFilter) ([]*PoolAccountUsageLog, int64, error) {
	logDB := LOG_DB
	if logDB == nil {
		logDB = DB
	}
	if logDB == nil {
		return nil, 0, gorm.ErrInvalidDB
	}
	if filter.Limit <= 0 {
		filter.Limit = 20
	}
	if filter.Limit > 100 {
		filter.Limit = 100
	}
	query := logDB.Model(&PoolAccountUsageLog{})
	if filter.PoolGroupId > 0 {
		query = query.Where("pool_group_id = ?", filter.PoolGroupId)
	}
	if filter.PoolAccountId > 0 {
		query = query.Where("pool_account_id = ?", filter.PoolAccountId)
	}
	if filter.ChannelId > 0 {
		query = query.Where("channel_id = ?", filter.ChannelId)
	}
	if filter.UserId > 0 {
		query = query.Where("user_id = ?", filter.UserId)
	}
	if filter.Success != nil {
		query = query.Where("success = ?", *filter.Success)
	}
	if filter.StartTimestamp > 0 {
		query = query.Where("created_at >= ?", filter.StartTimestamp)
	}
	if filter.EndTimestamp > 0 {
		query = query.Where("created_at <= ?", filter.EndTimestamp)
	}
	if strings.TrimSpace(filter.ModelName) != "" {
		query = query.Where("model_name = ?", strings.TrimSpace(filter.ModelName))
	}
	if strings.TrimSpace(filter.RequestId) != "" {
		query = query.Where("request_id = ?", strings.TrimSpace(filter.RequestId))
	}
	if strings.TrimSpace(filter.UpstreamRequestId) != "" {
		query = query.Where("upstream_request_id = ?", strings.TrimSpace(filter.UpstreamRequestId))
	}
	if strings.TrimSpace(filter.Search) != "" {
		like := "%" + strings.TrimSpace(filter.Search) + "%"
		query = query.Where("pool_group_name LIKE ? OR pool_account_name LIKE ? OR channel_name LIKE ? OR model_name LIKE ? OR username LIKE ? OR token_name LIKE ? OR error_message LIKE ?", like, like, like, like, like, like, like)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	logs := []*PoolAccountUsageLog{}
	err := query.Order("id DESC").Limit(filter.Limit).Offset(filter.StartIdx).Find(&logs).Error
	return logs, total, err
}
