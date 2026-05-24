// httpapi - server.go
// HTTP API 服务模块，提供 CPA Manager 使用量采集服务的 RESTful API。
// 主要功能：
//   - 健康检查和服务信息查询
//   - 采集器状态和管理配置的读写
//   - 初始连接设置（setup）
//   - 使用量数据的查询、导出和导入
//   - 模型价格管理（增删改查、从 LiteLLM 同步）
//   - API Key 别名管理（CRUD）
//   - 模型列表代理（转发到上游 CPA）
//   - 管理面板（内嵌 HTML 或外部文件）
//   - CORS 跨域支持
//   - 管理接口代理（转发到上游 CPA）
package httpapi

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/seakee/cpa-manager/usage-service/internal/collector"
	"github.com/seakee/cpa-manager/usage-service/internal/config"
	"github.com/seakee/cpa-manager/usage-service/internal/store"
	"github.com/seakee/cpa-manager/usage-service/internal/usage"
)

// embeddedPanel 是内嵌的管理面板 HTML 文件。
// 通过 go:embed 指令在编译时嵌入，作为外部文件不存在时的回退。
//
//go:embed web/management.html
var embeddedPanel embed.FS

// Server 是 HTTP API 服务的核心结构。
// 封装了配置、存储层、采集管理器和启动时间。
type Server struct {
	cfg       config.Config    // 应用配置
	store     *store.Store     // 数据存储层
	collector *collector.Manager // 采集管理器
	startedAt int64            // 服务启动时间戳（毫秒）
}

// setupSource 表示配置来源类型。
type setupSource string

// serviceID 服务标识符，用于健康检查和信息接口。
const serviceID = "cpa-manager"

// 配置来源常量定义
const (
	setupSourceNone setupSource = ""     // 未配置
	setupSourceEnv  setupSource = "env"  // 环境变量
	setupSourceDB   setupSource = "db"   // 数据库
)

// maxUsageImportBytes 导入请求体的最大大小限制（64MB）。
const maxUsageImportBytes int64 = 64 * 1024 * 1024

// modelPriceSyncSource 模型价格同步的数据源标识。
const modelPriceSyncSource = "litellm"

// modelPriceSyncURL LiteLLM 模型价格数据的远程 URL。
var modelPriceSyncURL = "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json"

// setupRequest 表示初始连接设置的请求结构。
type setupRequest struct {
	CPAUpstreamURL               string `json:"cpaBaseUrl"`                    // CPA 上游 URL
	ManagementKey                string `json:"managementKey"`                 // 管理密钥
	CollectorMode                string `json:"collectorMode"`                 // 采集模式
	Queue                        string `json:"queue"`                         // 队列名称
	PopSide                      string `json:"popSide"`                       // 弹出方向
	BatchSize                    int    `json:"batchSize"`                     // 批次大小
	PollIntervalMS               int    `json:"pollIntervalMs"`                // 轮询间隔（毫秒）
	QueryLimit                   int    `json:"queryLimit"`                    // 查询限制
	TLSSkipVerify                bool   `json:"tlsSkipVerify"`                 // TLS 跳过验证
	EnsureUsageStatisticsEnabled *bool  `json:"ensureUsageStatisticsEnabled"`  // 是否确保上游启用使用量统计
	RequestMonitoringEnabled     *bool  `json:"requestMonitoringEnabled"`      // 是否启用请求监控
}

// managerConfigResponse 表示管理配置查询的响应结构。
type managerConfigResponse struct {
	Config   store.ManagerConfig `json:"config"`             // 管理配置
	Source   string              `json:"source"`             // 配置来源（env/db）
	CPAUsage *cpaUsageConfig     `json:"cpaUsage,omitempty"` // 上游 CPA 的使用量配置
}

// cpaUsageConfig 表示上游 CPA 的使用量相关配置。
type cpaUsageConfig struct {
	UsageStatisticsEnabled          bool `json:"usageStatisticsEnabled"`          // 使用量统计是否启用
	RedisUsageQueueRetentionSeconds int  `json:"redisUsageQueueRetentionSeconds"` // Redis 使用量队列保留秒数
	RetentionSourceDefault          bool `json:"retentionSourceDefault"`          // 保留时间是否为默认值
}

// modelPricesRequest 表示模型价格保存的请求结构。
type modelPricesRequest struct {
	Prices map[string]store.ModelPrice `json:"prices"` // 模型价格映射
}

// modelPricesSyncRequest 表示模型价格同步的请求结构。
type modelPricesSyncRequest struct {
	Models []string `json:"models"` // 需要同步的模型列表（为空则同步全部）
}

// apiKeyAliasesRequest 表示 API Key 别名批量操作的请求结构。
type apiKeyAliasesRequest struct {
	Items              []store.APIKeyAlias `json:"items"`                        // 别名列表
	ActiveAPIKeyHashes []string            `json:"activeApiKeyHashes,omitempty"` // 当前活跃的 API Key 哈希集合
}

// New 创建一个新的 HTTP API 服务器实例。
func New(cfg config.Config, store *store.Store, collector *collector.Manager) *Server {
	return &Server{
		cfg:       cfg,
		store:     store,
		collector: collector,
		startedAt: time.Now().UnixMilli(),
	}
}

// Handler 注册所有 HTTP 路由并返回 http.Handler。
// 路由表：
//   - GET /health: 健康检查
//   - GET /status: 采集器状态查询（需认证）
//   - GET /usage-service/info: 服务信息
//   - GET/PUT /usage-service/config: 管理配置读写（需认证）
//   - POST /setup: 初始连接设置
//   - GET /management.html: 管理面板页面
//   - /: 根路由，分发到子路由或代理
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.withCORS(s.handleHealth))
	mux.HandleFunc("/status", s.withCORS(s.handleStatus))
	mux.HandleFunc("/usage-service/info", s.withCORS(s.handleInfo))
	mux.HandleFunc("/usage-service/config", s.withCORS(s.handleManagerConfig))
	mux.HandleFunc("/setup", s.withCORS(s.handleSetup))
	mux.HandleFunc("/management.html", s.handlePanel)
	mux.HandleFunc("/", s.handleRoot)
	return mux
}

// handleRoot 是根路由处理器，根据路径分发请求到对应的子处理器。
// 路由优先级：
// 1. OPTIONS 预检请求
// 2. /v0/management/model-prices* -> 模型价格管理
// 3. /v0/management/api-key-aliases* -> API Key 别名管理
// 4. /v0/management/usage* -> 使用量数据管理
// 5. /v0/management/* -> 代理到上游 CPA
// 6. /v1/models 或 /models -> 模型列表代理
// 7. / -> 重定向到管理面板
// 8. 其他 -> 404
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		s.writeCORS(w, r)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/v0/management/model-prices") {
		s.withCORS(s.handleModelPrices)(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/v0/management/api-key-aliases") {
		s.withCORS(s.handleAPIKeyAliases)(w, r)
		return
	}
	cleanUsagePath := strings.TrimRight(r.URL.Path, "/")
	if cleanUsagePath == "/v0/management/usage" || strings.HasPrefix(cleanUsagePath, "/v0/management/usage/") {
		s.withCORS(s.handleUsage)(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/v0/management/") {
		s.withCORS(s.handleProxy)(w, r)
		return
	}
	if isModelListProxyPath(r.URL.Path) {
		s.withCORS(s.handleModelListProxy)(w, r)
		return
	}
	if r.URL.Path == "/" {
		http.Redirect(w, r, "/management.html", http.StatusTemporaryRedirect)
		return
	}
	http.NotFound(w, r)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "service": serviceID})
}

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	setup, ok, err := s.resolveSetup(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"service":    serviceID,
		"mode":       "embedded",
		"startedAt":  s.startedAt,
		"configured": ok && setup.CPAUpstreamURL != "" && setup.ManagementKey != "",
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	if !s.authorizeIfConfigured(w, r) {
		return
	}
	events, deadLetters, err := s.store.Counts(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	status := s.collector.Status()
	status.DeadLetters = deadLetters
	writeJSON(w, http.StatusOK, map[string]any{
		"service":     serviceID,
		"dbPath":      s.cfg.DBPath,
		"events":      events,
		"deadLetters": deadLetters,
		"collector":   status,
	})
}

func (s *Server) handleManagerConfig(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeIfConfigured(w, r) {
		return
	}

	switch r.Method {
	case http.MethodGet:
		cfg, source, _, err := s.resolveManagerConfigWithSource(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		var cpaUsage *cpaUsageConfig
		if cfg.CPAConnection.CPABaseURL != "" && cfg.CPAConnection.ManagementKey != "" {
			if usageCfg, err := fetchCPAUsageConfig(
				r.Context(),
				cfg.CPAConnection.CPABaseURL,
				cfg.CPAConnection.ManagementKey,
			); err == nil {
				cpaUsage = &usageCfg
			}
		}
		writeJSON(w, http.StatusOK, managerConfigResponse{
			Config:   cfg,
			Source:   string(source),
			CPAUsage: cpaUsage,
		})
	case http.MethodPut:
		var req struct {
			Config store.ManagerConfig `json:"config"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		current, source, _, err := s.resolveManagerConfigWithSource(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		next := s.mergeSubmittedManagerConfig(current, req.Config)
		if source == setupSourceEnv && managerConfigConnectionDiffers(current, next) {
			writeError(w, http.StatusConflict, errors.New("connection setup is managed by environment variables"))
			return
		}
		if next.CPAConnection.CPABaseURL != "" || next.CPAConnection.ManagementKey != "" {
			if next.CPAConnection.CPABaseURL == "" || next.CPAConnection.ManagementKey == "" {
				writeError(w, http.StatusBadRequest, errors.New("cpaBaseUrl and managementKey are required"))
				return
			}
			if err := validateManagementAPI(
				r.Context(),
				next.CPAConnection.CPABaseURL,
				next.CPAConnection.ManagementKey,
			); err != nil {
				writeError(w, http.StatusBadGateway, err)
				return
			}
			if managerCollectorEnabled(next) {
				if err := validateCollectorAgainstCPA(r.Context(), next); err != nil {
					writeError(w, http.StatusBadRequest, err)
					return
				}
				if err := setCPAUsageStatisticsEnabled(
					r.Context(),
					next.CPAConnection.CPABaseURL,
					next.CPAConnection.ManagementKey,
					true,
				); err != nil {
					writeError(w, http.StatusBadGateway, err)
					return
				}
			}
		} else if managerCollectorEnabled(next) {
			writeError(w, http.StatusBadRequest, errors.New("cpaBaseUrl and managementKey are required when request monitoring is enabled"))
			return
		}
		if next.CPAConnection.CPABaseURL == "" || next.CPAConnection.ManagementKey == "" {
			if err := s.store.SaveManagerConfig(r.Context(), next); err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			s.collector.Stop()
			writeJSON(w, http.StatusOK, managerConfigResponse{
				Config: next,
				Source: string(setupSourceDB),
			})
			return
		}
		if err := s.store.SaveManagerConfig(r.Context(), next); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		setup := setupFromManagerConfig(next)
		if err := s.store.SaveSetup(r.Context(), setup); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if managerCollectorEnabled(next) {
			s.collector.Start(context.Background(), runtimeConfigFromManagerConfig(next))
		} else {
			s.collector.Stop()
		}
		writeJSON(w, http.StatusOK, managerConfigResponse{
			Config: next,
			Source: string(setupSourceDB),
		})
	default:
		methodNotAllowed(w)
	}
}

// handleSetup 处理初始连接设置请求。
// 验证管理密钥、连接到上游 CPA、保存配置并可选地启动采集器。
// 安全策略：
//   - 已有设置且来源为环境变量时拒绝修改
//   - 更换上游 URL 时需要通过现有管理密钥认证
//   - 同一上游更换密钥时需要验证新密钥的有效性
func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var req setupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req.CPAUpstreamURL = normalizeBaseURL(req.CPAUpstreamURL)
	req.ManagementKey = strings.TrimSpace(req.ManagementKey)
	req.CollectorMode = collectorMode(req.CollectorMode)
	if req.Queue == "" {
		req.Queue = s.cfg.Queue
	}
	if req.PopSide == "" {
		req.PopSide = s.cfg.PopSide
	}
	req.PopSide = normalizePopSide(req.PopSide, s.cfg.PopSide)
	req.BatchSize = positiveOrDefault(req.BatchSize, s.cfg.BatchSize, 100)
	req.PollIntervalMS = positiveOrDefault(req.PollIntervalMS, int(s.cfg.PollInterval/time.Millisecond), 500)
	req.QueryLimit = positiveOrDefault(req.QueryLimit, s.cfg.QueryLimit, 50000)
	requestMonitoringEnabled := setupRequestMonitoringEnabled(req)
	if req.CPAUpstreamURL == "" || req.ManagementKey == "" {
		writeError(w, http.StatusBadRequest, errors.New("cpaBaseUrl and managementKey are required"))
		return
	}
	managementAPIValidated := false
	if existing, source, ok, err := s.resolveSetupWithSource(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	} else if source == setupSourceEnv && setupDiffers(existing, req) {
		writeError(w, http.StatusConflict, errors.New("setup is managed by environment variables"))
		return
	} else if ok && existing.ManagementKey != "" &&
		!authMatches(r, existing.ManagementKey) &&
		req.ManagementKey != existing.ManagementKey {
		if normalizeBaseURL(existing.CPAUpstreamURL) != req.CPAUpstreamURL {
			writeError(w, http.StatusUnauthorized, errors.New("invalid management key for existing setup"))
			return
		}
		if err := validateManagementAPI(r.Context(), req.CPAUpstreamURL, req.ManagementKey); err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		managementAPIValidated = true
	}
	if !managementAPIValidated {
		if err := validateManagementAPI(r.Context(), req.CPAUpstreamURL, req.ManagementKey); err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
	}
	managerCfg := s.defaultManagerConfig()
	if existingManagerCfg, _, ok, err := s.resolveManagerConfigWithSource(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	} else if ok {
		managerCfg = existingManagerCfg
	}
	managerCfg.CPAConnection.CPABaseURL = req.CPAUpstreamURL
	managerCfg.CPAConnection.ManagementKey = req.ManagementKey
	managerCfg.Collector.Enabled = boolPtr(requestMonitoringEnabled)
	managerCfg.Collector.CollectorMode = req.CollectorMode
	managerCfg.Collector.Queue = req.Queue
	managerCfg.Collector.PopSide = req.PopSide
	managerCfg.Collector.BatchSize = req.BatchSize
	managerCfg.Collector.PollIntervalMS = req.PollIntervalMS
	managerCfg.Collector.QueryLimit = req.QueryLimit
	managerCfg.Collector.TLSSkipVerify = req.TLSSkipVerify
	if requestMonitoringEnabled {
		if err := validateCollectorAgainstCPA(r.Context(), managerCfg); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
	}
	ensureUsageStatisticsEnabled := requestMonitoringEnabled
	if req.EnsureUsageStatisticsEnabled != nil {
		ensureUsageStatisticsEnabled = requestMonitoringEnabled && *req.EnsureUsageStatisticsEnabled
	}
	if ensureUsageStatisticsEnabled {
		if err := setCPAUsageStatisticsEnabled(r.Context(), req.CPAUpstreamURL, req.ManagementKey, true); err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
	}
	setup := store.Setup{
		CPAUpstreamURL: req.CPAUpstreamURL,
		ManagementKey:  req.ManagementKey,
		Queue:          req.Queue,
		PopSide:        req.PopSide,
	}
	if err := s.store.SaveSetup(r.Context(), setup); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if err := s.store.SaveManagerConfig(r.Context(), managerCfg); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if requestMonitoringEnabled {
		s.collector.Start(context.Background(), runtimeConfigFromManagerConfig(managerCfg))
	} else {
		s.collector.Stop()
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "upstream": setup.CPAUpstreamURL})
}

// handleModelPrices 处理模型价格的 CRUD 和同步操作。
//   - GET /v0/management/model-prices: 获取所有模型价格
//   - PUT /v0/management/model-prices: 批量保存模型价格
//   - POST /v0/management/model-prices/sync: 从 LiteLLM 同步模型价格
func (s *Server) handleModelPrices(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeIfConfigured(w, r) {
		return
	}

	path := strings.TrimRight(r.URL.Path, "/")
	switch {
	case path == "/v0/management/model-prices" && r.Method == http.MethodGet:
		prices, err := s.store.LoadModelPrices(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"prices": prices})
	case path == "/v0/management/model-prices" && r.Method == http.MethodPut:
		var req modelPricesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if req.Prices == nil {
			writeError(w, http.StatusBadRequest, errors.New("prices are required"))
			return
		}
		if err := s.store.SaveModelPrices(r.Context(), req.Prices); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		prices, err := s.store.LoadModelPrices(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"prices": prices})
	case path == "/v0/management/model-prices/sync" && r.Method == http.MethodPost:
		var req modelPricesSyncRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		remotePrices, skipped, err := fetchLiteLLMModelPrices(r.Context())
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		selectedPrices := selectModelPrices(remotePrices, req.Models)
		result, err := s.store.UpsertSyncedModelPrices(r.Context(), selectedPrices)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		prices, err := s.store.LoadModelPrices(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"source":   modelPriceSyncSource,
			"imported": result.Imported,
			"skipped":  result.Skipped + skipped,
			"prices":   prices,
		})
	default:
		methodNotAllowed(w)
	}
}

// handleAPIKeyAliases 处理 API Key 别名的 CRUD 操作。
//   - GET /v0/management/api-key-aliases: 获取所有别名
//   - PUT /v0/management/api-key-aliases: 批量创建/更新别名
//   - DELETE /v0/management/api-key-aliases/{hash}: 删除指定别名
func (s *Server) handleAPIKeyAliases(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeIfConfigured(w, r) {
		return
	}

	path := strings.TrimRight(r.URL.Path, "/")
	const basePath = "/v0/management/api-key-aliases"
	switch {
	case path == basePath && r.Method == http.MethodGet:
		aliases, err := s.store.LoadAPIKeyAliases(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": aliases})
	case path == basePath && r.Method == http.MethodPut:
		var req apiKeyAliasesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if req.Items == nil {
			writeError(w, http.StatusBadRequest, errors.New("api key aliases are required"))
			return
		}
		if err := s.store.UpsertAPIKeyAliases(r.Context(), req.Items, req.ActiveAPIKeyHashes); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		aliases, err := s.store.LoadAPIKeyAliases(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": aliases})
	case strings.HasPrefix(path, basePath+"/") && r.Method == http.MethodDelete:
		apiKeyHash := strings.TrimPrefix(path, basePath+"/")
		if err := s.store.DeleteAPIKeyAlias(r.Context(), apiKeyHash); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		methodNotAllowed(w)
	}
}

// fetchLiteLLMModelPrices 从 LiteLLM 远程数据源获取模型价格数据。
// 返回模型价格映射和跳过的条目数。
// 价格从 "per token" 转换为 "per million tokens"。
func fetchLiteLLMModelPrices(ctx context.Context) (map[string]store.ModelPrice, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelPriceSyncURL, nil)
	if err != nil {
		return nil, 0, err
	}
	client := &http.Client{Timeout: 30 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, 0, errors.New("model price sync failed: " + res.Status)
	}

	var payload map[string]json.RawMessage
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return nil, 0, err
	}

	prices := map[string]store.ModelPrice{}
	skipped := 0
	for model, raw := range payload {
		if model == "" || model == "sample_spec" {
			skipped++
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal(raw, &entry); err != nil {
			skipped++
			continue
		}

		prompt, hasPrompt := readFloat(entry, "input_cost_per_token")
		completion, hasCompletion := readFloat(entry, "output_cost_per_token")
		cache, hasCache := readFloat(entry, "cache_read_input_token_cost")
		if !hasCache {
			cache, hasCache = readFloat(entry, "cache_read_cost_per_token")
		}
		if !hasPrompt && !hasCompletion {
			skipped++
			continue
		}
		if !hasPrompt {
			prompt = 0
		}
		if !hasCompletion {
			completion = 0
		}
		if !hasCache {
			cache = prompt
		}

		prices[model] = store.ModelPrice{
			Prompt:        prompt * 1_000_000,
			Completion:    completion * 1_000_000,
			Cache:         cache * 1_000_000,
			Source:        modelPriceSyncSource,
			SourceModelID: model,
			RawJSON:       string(raw),
		}
	}
	return prices, skipped, nil
}

// selectModelPrices 从价格映射中选择指定模型的价格。
// 如果 models 为空则返回全部价格。
// 支持精确匹配和后缀匹配（如 "gpt-4o" 可匹配 "openai/gpt-4o"）。
func selectModelPrices(prices map[string]store.ModelPrice, models []string) map[string]store.ModelPrice {
	wanted := make([]string, 0, len(models))
	seen := map[string]struct{}{}
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		wanted = append(wanted, model)
	}
	if len(wanted) == 0 {
		return prices
	}

	selected := map[string]store.ModelPrice{}
	for _, model := range wanted {
		if price, ok := prices[model]; ok {
			selected[model] = price
			continue
		}
		if price, ok := findSuffixModelPrice(prices, model); ok {
			selected[model] = price
		}
	}
	return selected
}

func findSuffixModelPrice(prices map[string]store.ModelPrice, model string) (store.ModelPrice, bool) {
	suffix := "/" + model
	var match store.ModelPrice
	matchedKey := ""
	for key, price := range prices {
		if !strings.HasSuffix(key, suffix) {
			continue
		}
		if matchedKey == "" || len(key) < len(matchedKey) {
			matchedKey = key
			match = price
		}
	}
	return match, matchedKey != ""
}

func readFloat(entry map[string]any, key string) (float64, bool) {
	value, ok := entry[key]
	if !ok || value == nil {
		return 0, false
	}
	switch typed := value.(type) {
	case float64:
		return typed, true
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

// handleUsage 处理使用量数据的查询和导入。
//   - GET /v0/management/usage: 获取最近的使用量事件（按端点和模型聚合）
//   - GET /v0/management/usage/export: 导出为 JSONL 格式
//   - POST /v0/management/usage/import: 导入使用量数据
func (s *Server) handleUsage(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeIfConfigured(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		if strings.HasSuffix(r.URL.Path, "/export") {
			s.handleUsageExport(w, r)
			return
		}
		events, err := s.store.RecentEvents(r.Context(), s.cfg.QueryLimit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, usage.BuildPayload(events))
	case http.MethodPost:
		if strings.HasSuffix(r.URL.Path, "/import") {
			s.handleUsageImport(w, r)
			return
		}
		methodNotAllowed(w)
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleUsageExport(w http.ResponseWriter, r *http.Request) {
	data, err := s.store.ExportJSONL(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Content-Disposition", `attachment; filename="usage-events.jsonl"`)
	_, _ = w.Write(data)
}

func (s *Server) handleUsageImport(w http.ResponseWriter, r *http.Request) {
	body := http.MaxBytesReader(w, r.Body, maxUsageImportBytes)
	data, err := io.ReadAll(body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeError(w, http.StatusRequestEntityTooLarge, err)
			return
		}
		writeError(w, http.StatusBadRequest, err)
		return
	}

	parsed, err := usage.ParseImportPayload(data)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error":       err.Error(),
			"format":      parsed.Format,
			"failed":      parsed.Failed,
			"unsupported": parsed.Unsupported,
			"warnings":    parsed.Warnings,
		})
		return
	}

	result, err := s.store.InsertEvents(r.Context(), parsed.Events)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"format":      parsed.Format,
		"added":       result.Inserted,
		"skipped":     result.Skipped,
		"total":       len(parsed.Events),
		"failed":      parsed.Failed,
		"unsupported": parsed.Unsupported,
		"warnings":    parsed.Warnings,
	})
}

func isModelListProxyPath(path string) bool {
	cleaned := strings.TrimRight(path, "/")
	return cleaned == "/v1/models" || cleaned == "/models"
}

func (s *Server) handleModelListProxy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	setup, ok, err := s.resolveSetup(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if !ok {
		writeError(w, http.StatusPreconditionRequired, errors.New("usage service is not configured"))
		return
	}
	target, err := url.Parse(setup.CPAUpstreamURL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.Host = target.Host
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		writeError(w, http.StatusBadGateway, err)
	}
	proxy.ServeHTTP(w, r)
}

func (s *Server) handleProxy(w http.ResponseWriter, r *http.Request) {
	setup, ok, err := s.resolveSetup(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if !ok {
		writeError(w, http.StatusPreconditionRequired, errors.New("usage service is not configured"))
		return
	}
	if !authMatches(r, setup.ManagementKey) {
		writeError(w, http.StatusUnauthorized, errors.New("invalid management key"))
		return
	}
	target, err := url.Parse(setup.CPAUpstreamURL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.Host = target.Host
		req.Header.Set("Authorization", "Bearer "+setup.ManagementKey)
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		writeError(w, http.StatusBadGateway, err)
	}
	proxy.ServeHTTP(w, r)
}

func (s *Server) handlePanel(w http.ResponseWriter, r *http.Request) {
	if s.cfg.PanelPath != "" {
		if file, err := os.Open(s.cfg.PanelPath); err == nil {
			defer file.Close()
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.Copy(w, file)
			return
		}
	}
	data, err := embeddedPanel.ReadFile("web/management.html")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", mime.TypeByExtension(".html"))
	_, _ = w.Write(data)
}

func (s *Server) resolveSetup(ctx context.Context) (store.Setup, bool, error) {
	setup, _, ok, err := s.resolveSetupWithSource(ctx)
	return setup, ok, err
}

func (s *Server) resolveSetupWithSource(ctx context.Context) (store.Setup, setupSource, bool, error) {
	if s.cfg.CPAUpstreamURL != "" && s.cfg.ManagementKey != "" {
		return store.Setup{
			CPAUpstreamURL: normalizeBaseURL(s.cfg.CPAUpstreamURL),
			ManagementKey:  s.cfg.ManagementKey,
			Queue:          s.cfg.Queue,
			PopSide:        s.cfg.PopSide,
		}, setupSourceEnv, true, nil
	}
	if managerCfg, _, ok, err := s.resolveManagerConfigWithSource(ctx); err != nil {
		return store.Setup{}, setupSourceNone, false, err
	} else if ok && managerCfg.CPAConnection.CPABaseURL != "" && managerCfg.CPAConnection.ManagementKey != "" {
		return setupFromManagerConfig(managerCfg), setupSourceDB, true, nil
	}
	setup, ok, err := s.store.LoadSetup(ctx)
	if !ok || err != nil {
		return setup, setupSourceNone, ok, err
	}
	return setup, setupSourceDB, true, nil
}

func (s *Server) resolveManagerConfigWithSource(ctx context.Context) (store.ManagerConfig, setupSource, bool, error) {
	cfg := s.defaultManagerConfig()
	source := setupSourceNone
	found := false

	if saved, ok, err := s.store.LoadManagerConfig(ctx); err != nil {
		return cfg, source, false, err
	} else if ok {
		cfg = s.mergeSubmittedManagerConfig(cfg, saved)
		source = setupSourceDB
		found = true
	}

	if setup, ok, err := s.store.LoadSetup(ctx); err != nil {
		return cfg, source, false, err
	} else if ok && cfg.CPAConnection.CPABaseURL == "" && cfg.CPAConnection.ManagementKey == "" {
		cfg.CPAConnection.CPABaseURL = normalizeBaseURL(setup.CPAUpstreamURL)
		cfg.CPAConnection.ManagementKey = setup.ManagementKey
		cfg.Collector.Queue = valueOr(setup.Queue, cfg.Collector.Queue)
		cfg.Collector.PopSide = normalizePopSide(setup.PopSide, cfg.Collector.PopSide)
		source = setupSourceDB
		found = true
	}

	if s.cfg.CPAUpstreamURL != "" && s.cfg.ManagementKey != "" {
		cfg.CPAConnection.CPABaseURL = normalizeBaseURL(s.cfg.CPAUpstreamURL)
		cfg.CPAConnection.ManagementKey = s.cfg.ManagementKey
		cfg.Collector.CollectorMode = collectorMode(s.cfg.CollectorMode)
		cfg.Collector.Queue = valueOr(s.cfg.Queue, cfg.Collector.Queue)
		cfg.Collector.PopSide = normalizePopSide(s.cfg.PopSide, cfg.Collector.PopSide)
		cfg.Collector.BatchSize = positiveOrDefault(s.cfg.BatchSize, cfg.Collector.BatchSize, 100)
		cfg.Collector.PollIntervalMS = positiveOrDefault(int(s.cfg.PollInterval/time.Millisecond), cfg.Collector.PollIntervalMS, 500)
		cfg.Collector.QueryLimit = positiveOrDefault(s.cfg.QueryLimit, cfg.Collector.QueryLimit, 50000)
		cfg.Collector.TLSSkipVerify = s.cfg.TLSSkipVerify
		source = setupSourceEnv
		found = true
	}

	return cfg, source, found, nil
}

func setupDiffers(existing store.Setup, req setupRequest) bool {
	return normalizeBaseURL(existing.CPAUpstreamURL) != req.CPAUpstreamURL ||
		existing.ManagementKey != req.ManagementKey ||
		existing.Queue != req.Queue ||
		existing.PopSide != req.PopSide
}

func setupFromManagerConfig(cfg store.ManagerConfig) store.Setup {
	return store.Setup{
		CPAUpstreamURL: cfg.CPAConnection.CPABaseURL,
		ManagementKey:  cfg.CPAConnection.ManagementKey,
		Queue:          cfg.Collector.Queue,
		PopSide:        cfg.Collector.PopSide,
	}
}

func runtimeConfigFromManagerConfig(cfg store.ManagerConfig) collector.RuntimeConfig {
	return collector.RuntimeConfig{
		CPAUpstreamURL: cfg.CPAConnection.CPABaseURL,
		ManagementKey:  cfg.CPAConnection.ManagementKey,
		CollectorMode:  cfg.Collector.CollectorMode,
		Queue:          cfg.Collector.Queue,
		PopSide:        cfg.Collector.PopSide,
		BatchSize:      cfg.Collector.BatchSize,
		PollInterval:   time.Duration(cfg.Collector.PollIntervalMS) * time.Millisecond,
		TLSSkipVerify:  cfg.Collector.TLSSkipVerify,
	}
}

func (s *Server) defaultManagerConfig() store.ManagerConfig {
	pollIntervalMS := int(s.cfg.PollInterval / time.Millisecond)
	return store.ManagerConfig{
		Collector: store.ManagerCollectorConfig{
			Enabled:        boolPtr(true),
			CollectorMode:  collectorMode(s.cfg.CollectorMode),
			Queue:          valueOr(s.cfg.Queue, "usage"),
			PopSide:        normalizePopSide(s.cfg.PopSide, "right"),
			BatchSize:      positiveOrDefault(s.cfg.BatchSize, 100, 100),
			PollIntervalMS: positiveOrDefault(pollIntervalMS, 500, 500),
			QueryLimit:     positiveOrDefault(s.cfg.QueryLimit, 50000, 50000),
			TLSSkipVerify:  s.cfg.TLSSkipVerify,
		},
	}
}

func (s *Server) mergeSubmittedManagerConfig(base store.ManagerConfig, submitted store.ManagerConfig) store.ManagerConfig {
	next := base

	if submitted.CPAConnection.CPABaseURL != "" || submitted.CPAConnection.ManagementKey != "" {
		next.CPAConnection.CPABaseURL = normalizeBaseURL(submitted.CPAConnection.CPABaseURL)
		next.CPAConnection.ManagementKey = strings.TrimSpace(submitted.CPAConnection.ManagementKey)
	}

	if submitted.Collector.Enabled != nil {
		next.Collector.Enabled = boolPtr(*submitted.Collector.Enabled)
	}
	next.Collector.CollectorMode = collectorMode(valueOr(submitted.Collector.CollectorMode, next.Collector.CollectorMode))
	next.Collector.Queue = valueOr(strings.TrimSpace(submitted.Collector.Queue), next.Collector.Queue)
	next.Collector.PopSide = normalizePopSide(submitted.Collector.PopSide, next.Collector.PopSide)
	next.Collector.BatchSize = positiveOrDefault(submitted.Collector.BatchSize, next.Collector.BatchSize, 100)
	next.Collector.PollIntervalMS = positiveOrDefault(submitted.Collector.PollIntervalMS, next.Collector.PollIntervalMS, 500)
	next.Collector.QueryLimit = positiveOrDefault(submitted.Collector.QueryLimit, next.Collector.QueryLimit, 50000)
	next.Collector.TLSSkipVerify = submitted.Collector.TLSSkipVerify

	next.ExternalUsageService.Enabled = submitted.ExternalUsageService.Enabled
	next.ExternalUsageService.ServiceBase = normalizeBaseURL(submitted.ExternalUsageService.ServiceBase)
	if !next.ExternalUsageService.Enabled {
		next.ExternalUsageService.ServiceBase = ""
	}

	return next
}

func managerConfigConnectionDiffers(left store.ManagerConfig, right store.ManagerConfig) bool {
	return normalizeBaseURL(left.CPAConnection.CPABaseURL) != normalizeBaseURL(right.CPAConnection.CPABaseURL) ||
		left.CPAConnection.ManagementKey != right.CPAConnection.ManagementKey ||
		managerCollectorEnabled(left) != managerCollectorEnabled(right) ||
		left.Collector.CollectorMode != right.Collector.CollectorMode ||
		left.Collector.Queue != right.Collector.Queue ||
		left.Collector.PopSide != right.Collector.PopSide ||
		left.Collector.BatchSize != right.Collector.BatchSize ||
		left.Collector.PollIntervalMS != right.Collector.PollIntervalMS ||
		left.Collector.TLSSkipVerify != right.Collector.TLSSkipVerify
}

func positiveOrDefault(value int, fallback int, hardDefault int) int {
	if value > 0 {
		return value
	}
	if fallback > 0 {
		return fallback
	}
	return hardDefault
}

func valueOr(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func normalizePopSide(value string, fallback string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "left", "right":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		if strings.ToLower(strings.TrimSpace(fallback)) == "left" {
			return "left"
		}
		return "right"
	}
}

func collectorMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "http", "resp", "subscribe":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "auto"
	}
}

func boolPtr(value bool) *bool {
	return &value
}

func managerCollectorEnabled(cfg store.ManagerConfig) bool {
	return cfg.Collector.Enabled == nil || *cfg.Collector.Enabled
}

func setupRequestMonitoringEnabled(req setupRequest) bool {
	if req.RequestMonitoringEnabled == nil {
		return true
	}
	return *req.RequestMonitoringEnabled
}

// authorizeIfConfigured 在已配置管理密钥时验证请求的 Authorization 头。
// 未配置管理密钥时允许所有请求通过。返回 true 表示授权通过。
func (s *Server) authorizeIfConfigured(w http.ResponseWriter, r *http.Request) bool {
	setup, ok, err := s.resolveSetup(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return false
	}
	if !ok || setup.ManagementKey == "" {
		return true
	}
	if authMatches(r, setup.ManagementKey) {
		return true
	}
	writeError(w, http.StatusUnauthorized, errors.New("invalid management key"))
	return false
}

// authMatches 验证请求的 Authorization 头是否匹配管理密钥。
// 要求 "Bearer {managementKey}" 格式。
func authMatches(r *http.Request, managementKey string) bool {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if header == "" || managementKey == "" {
		return false
	}
	const prefix = "Bearer "
	if len(header) < len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return false
	}
	return strings.TrimSpace(header[len(prefix):]) == managementKey
}

// withCORS 为处理器添加 CORS 支持的中间件。
// 处理 OPTIONS 预检请求并设置 CORS 响应头。
func (s *Server) withCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.writeCORS(w, r)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

// writeCORS 设置 CORS 响应头。
// 根据请求的 Origin 和配置的允许来源列表决定是否允许跨域。
func (s *Server) writeCORS(w http.ResponseWriter, r *http.Request) {
	if len(s.cfg.CORSOrigins) == 0 {
		return
	}
	origin := r.Header.Get("Origin")
	allowed := s.cfg.CORSOrigins[0]
	for _, candidate := range s.cfg.CORSOrigins {
		if candidate == "*" || candidate == origin {
			allowed = candidate
			break
		}
	}
	if allowed == "*" {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	} else if origin != "" && allowed == origin {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
	}
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
}

// validateCollectorAgainstCPA 验证采集器配置与上游 CPA 的兼容性。
// 检查 pollIntervalMs 是否超过 CPA 的 Redis 使用量队列保留时间。
func validateCollectorAgainstCPA(ctx context.Context, cfg store.ManagerConfig) error {
	usageCfg, err := fetchCPAUsageConfig(ctx, cfg.CPAConnection.CPABaseURL, cfg.CPAConnection.ManagementKey)
	if err != nil {
		return err
	}
	retentionMS := usageCfg.RedisUsageQueueRetentionSeconds * 1000
	if retentionMS <= 0 {
		return errors.New("CPA redis-usage-queue-retention-seconds must be greater than 0")
	}
	if cfg.Collector.PollIntervalMS > retentionMS {
		return fmt.Errorf(
			"pollIntervalMs must be less than or equal to CPA redis-usage-queue-retention-seconds (%d seconds)",
			usageCfg.RedisUsageQueueRetentionSeconds,
		)
	}
	return nil
}

// validateManagementAPI 验证上游 CPA 管理接口的可访问性和密钥有效性。
// 通过调用 /v0/management/config 接口验证。
func validateManagementAPI(ctx context.Context, baseURL string, key string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/v0/management/config", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	client := &http.Client{Timeout: 15 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 200 && res.StatusCode < 300 {
		return nil
	}
	return errors.New("management API validation failed: " + res.Status)
}

func fetchCPAUsageConfig(ctx context.Context, baseURL string, key string) (cpaUsageConfig, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, normalizeBaseURL(baseURL)+"/v0/management/config", nil)
	if err != nil {
		return cpaUsageConfig{}, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	client := &http.Client{Timeout: 15 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return cpaUsageConfig{}, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return cpaUsageConfig{}, errors.New("management API config request failed: " + res.Status)
	}

	var raw map[string]any
	if err := json.NewDecoder(res.Body).Decode(&raw); err != nil {
		return cpaUsageConfig{}, err
	}
	usageEnabled := readBoolField(raw, "usage-statistics-enabled", "usageStatisticsEnabled")
	retention, hasRetention := readIntField(raw, "redis-usage-queue-retention-seconds", "redisUsageQueueRetentionSeconds")
	if !hasRetention {
		retention = 60
	}
	return cpaUsageConfig{
		UsageStatisticsEnabled:          usageEnabled,
		RedisUsageQueueRetentionSeconds: retention,
		RetentionSourceDefault:          !hasRetention,
	}, nil
}

func setCPAUsageStatisticsEnabled(ctx context.Context, baseURL string, key string, enabled bool) error {
	payload := map[string]bool{"value": enabled}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPut,
		normalizeBaseURL(baseURL)+"/v0/management/usage-statistics-enabled",
		strings.NewReader(string(data)),
	)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 15 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 200 && res.StatusCode < 300 {
		return nil
	}
	return errors.New("enable CPA usage statistics failed: " + res.Status)
}

func readBoolField(raw map[string]any, keys ...string) bool {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case bool:
			return typed
		case string:
			normalized := strings.ToLower(strings.TrimSpace(typed))
			return normalized == "1" || normalized == "true" || normalized == "yes" || normalized == "on"
		}
	}
	return false
}

func readIntField(raw map[string]any, keys ...string) (int, bool) {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case float64:
			return int(typed), true
		case int:
			return typed, true
		case json.Number:
			parsed, err := strconv.Atoi(typed.String())
			return parsed, err == nil
		case string:
			parsed, err := strconv.Atoi(strings.TrimSpace(typed))
			return parsed, err == nil
		}
	}
	return 0, false
}

// normalizeBaseURL 规范化上游 URL。
// 自动补全协议前缀，去除尾部斜杠和多余的 /v0/management 路径。
func normalizeBaseURL(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if !strings.Contains(value, "://") {
		value = "http://" + value
	}
	value = strings.TrimRight(value, "/")
	value = strings.TrimSuffix(value, "/v0/management")
	value = strings.TrimSuffix(value, "/v0")
	return value
}

// writeJSON 写入 JSON 格式的 HTTP 响应。
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// writeError 写入包含错误信息和错误码的 JSON 响应。
func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"error": err.Error(), "code": usageServiceErrorCode(err)})
}

// methodNotAllowed 返回 405 Method Not Allowed 错误响应。
func methodNotAllowed(w http.ResponseWriter) {
	writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
}

// usageServiceErrorCode 根据错误消息内容生成结构化的错误码。
// 用于前端根据错误码进行国际化和错误处理。
func usageServiceErrorCode(err error) string {
	message := err.Error()
	switch {
	case strings.Contains(message, "connection setup is managed by environment variables"):
		return "connection_env_managed"
	case strings.Contains(message, "cpaBaseUrl and managementKey are required when request monitoring is enabled"):
		return "cpa_connection_required_for_monitoring"
	case strings.Contains(message, "cpaBaseUrl and managementKey are required"):
		return "cpa_connection_required"
	case strings.Contains(message, "setup is managed by environment variables"):
		return "setup_env_managed"
	case strings.Contains(message, "invalid management key for existing setup"):
		return "invalid_existing_management_key"
	case strings.Contains(message, "invalid management key"):
		return "invalid_management_key"
	case strings.Contains(message, "usage service is not configured"):
		return "usage_service_not_configured"
	case strings.Contains(message, "CPA redis-usage-queue-retention-seconds must be greater than 0"):
		return "cpa_usage_retention_invalid"
	case strings.Contains(message, "pollIntervalMs must be less than or equal"):
		return "poll_interval_exceeds_retention"
	case strings.Contains(message, "management API validation failed"):
		return "management_api_validation_failed"
	case strings.Contains(message, "management API config request failed"):
		return "management_api_config_failed"
	case strings.Contains(message, "enable CPA usage statistics failed"):
		return "enable_cpa_usage_statistics_failed"
	case strings.Contains(message, "prices are required"):
		return "prices_required"
	case strings.Contains(message, "api key aliases are required"):
		return "api_key_aliases_required"
	case strings.Contains(message, "api key alias already exists"):
		return "api_key_alias_duplicate"
	case strings.Contains(message, "model price sync failed"):
		return "model_price_sync_failed"
	case strings.Contains(message, "method not allowed"):
		return "method_not_allowed"
	default:
		return "request_failed"
	}
}
