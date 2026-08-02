// Package main 提供压测专用的 OpenAI-compatible mock upstream。
//
// 该服务只用于本地压测环境，避免压测误打真实模型上游产生费用，也避免真实上游的
// 网络抖动、限流策略和模型延迟污染 NexusTok 自身的性能基线。接口保持 OpenAI
// Chat Completions 的最小兼容形态，足够覆盖非流式、SSE 流式、模型列表和健康检查。
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

const defaultModel = "gpt-loadtest"

// config 保存 mock upstream 的可调参数。
//
// 压测时通过环境变量控制响应延迟、输出长度、错误率和 429 比例。所有参数只影响
// mock 服务自身，不会写入 NexusTok 生产配置。
type config struct {
	addr          string
	model         string
	ttft          time.Duration
	tokenDelay    time.Duration
	outputTokens  int
	errorRate     float64
	rateLimitRate float64
	bodyBytes     int
}

// stats 记录 mock upstream 的基础运行指标。
//
// 这些指标用于 collect.sh 在压测后确认上游是否收到流量、是否有预期内的错误注入，
// 并和 NexusTok 的应用指标、k6 summary 放在同一份报告里对照。
type stats struct {
	startedAt           time.Time
	totalRequests       atomic.Int64
	successRequests     atomic.Int64
	failedRequests      atomic.Int64
	rateLimitedRequests atomic.Int64
	streamRequests      atomic.Int64
	nonStreamRequests   atomic.Int64
	totalLatencyMs      atomic.Int64
	maxLatencyMs        atomic.Int64
	lastError           atomic.Value
}

type chatRequest struct {
	Model    string        `json:"model"`
	Stream   bool          `json:"stream"`
	Messages []chatMessage `json:"messages"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type modelListResponse struct {
	Object string      `json:"object"`
	Data   []modelItem `json:"data"`
}

type modelItem struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

type chatResponse struct {
	ID      string          `json:"id"`
	Object  string          `json:"object"`
	Created int64           `json:"created"`
	Model   string          `json:"model"`
	Choices []chatChoice    `json:"choices"`
	Usage   chatUsage       `json:"usage"`
	Error   *openAIErrorBox `json:"error,omitempty"`
}

type chatChoice struct {
	Index        int         `json:"index"`
	Message      chatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

type chatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type streamChunk struct {
	ID      string              `json:"id"`
	Object  string              `json:"object"`
	Created int64               `json:"created"`
	Model   string              `json:"model"`
	Choices []streamChunkChoice `json:"choices"`
}

type streamChunkChoice struct {
	Index        int               `json:"index"`
	Delta        map[string]string `json:"delta"`
	FinishReason *string           `json:"finish_reason"`
}

type openAIErrorResponse struct {
	Error openAIErrorBox `json:"error"`
}

type openAIErrorBox struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code,omitempty"`
}

type metricsResponse struct {
	Model               string `json:"model"`
	StartedAt           string `json:"started_at"`
	UptimeSeconds       int64  `json:"uptime_seconds"`
	TotalRequests       int64  `json:"total_requests"`
	SuccessRequests     int64  `json:"success_requests"`
	FailedRequests      int64  `json:"failed_requests"`
	RateLimitedRequests int64  `json:"rate_limited_requests"`
	StreamRequests      int64  `json:"stream_requests"`
	NonStreamRequests   int64  `json:"non_stream_requests"`
	AvgLatencyMs        int64  `json:"avg_latency_ms"`
	MaxLatencyMs        int64  `json:"max_latency_ms"`
	LastError           string `json:"last_error,omitempty"`
}

var randomFloat = rand.Float64

func main() {
	cfg := loadConfigFromEnv()
	serverStats := &stats{startedAt: time.Now()}
	handler := newServer(cfg, serverStats)

	log.Printf("mock-openai listening on %s, model=%s", cfg.addr, cfg.model)
	if err := http.ListenAndServe(cfg.addr, handler); err != nil {
		log.Fatal(err)
	}
}

func loadConfigFromEnv() config {
	return config{
		addr:          envString("MOCK_OPENAI_ADDR", ":8080"),
		model:         envString("MOCK_OPENAI_MODEL", defaultModel),
		ttft:          time.Duration(envInt("MOCK_OPENAI_TTFT_MS", 80)) * time.Millisecond,
		tokenDelay:    time.Duration(envInt("MOCK_OPENAI_TOKEN_DELAY_MS", 20)) * time.Millisecond,
		outputTokens:  envInt("MOCK_OPENAI_OUTPUT_TOKENS", 64),
		errorRate:     clampRate(envFloat("MOCK_OPENAI_ERROR_RATE", 0)),
		rateLimitRate: clampRate(envFloat("MOCK_OPENAI_RATE_LIMIT_RATE", 0)),
		bodyBytes:     envInt("MOCK_OPENAI_BODY_BYTES", 0),
	}
}

func envString(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}

func envFloat(key string, fallback float64) float64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func clampRate(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func newServer(cfg config, serverStats *stats) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", handleHealthz)
	mux.HandleFunc("/metrics", handleMetrics(cfg, serverStats))
	mux.HandleFunc("/v1/models", handleModels(cfg))
	mux.HandleFunc("/v1/chat/completions", handleChatCompletions(cfg, serverStats))
	return mux
}

func handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func handleMetrics(cfg config, serverStats *stats) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		total := serverStats.totalRequests.Load()
		avgLatency := int64(0)
		if total > 0 {
			avgLatency = serverStats.totalLatencyMs.Load() / total
		}
		lastError := ""
		if value := serverStats.lastError.Load(); value != nil {
			lastError, _ = value.(string)
		}
		writeJSON(w, http.StatusOK, metricsResponse{
			Model:               cfg.model,
			StartedAt:           serverStats.startedAt.Format(time.RFC3339),
			UptimeSeconds:       int64(time.Since(serverStats.startedAt).Seconds()),
			TotalRequests:       total,
			SuccessRequests:     serverStats.successRequests.Load(),
			FailedRequests:      serverStats.failedRequests.Load(),
			RateLimitedRequests: serverStats.rateLimitedRequests.Load(),
			StreamRequests:      serverStats.streamRequests.Load(),
			NonStreamRequests:   serverStats.nonStreamRequests.Load(),
			AvgLatencyMs:        avgLatency,
			MaxLatencyMs:        serverStats.maxLatencyMs.Load(),
			LastError:           lastError,
		})
	}
}

func handleModels(cfg config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed", "method_not_allowed")
			return
		}
		writeJSON(w, http.StatusOK, modelListResponse{
			Object: "list",
			Data: []modelItem{{
				ID:      cfg.model,
				Object:  "model",
				Created: time.Now().Unix(),
				OwnedBy: "loadtest",
			}},
		})
	}
}

func handleChatCompletions(cfg config, serverStats *stats) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		serverStats.totalRequests.Add(1)
		defer recordLatency(serverStats, start)

		if r.Method != http.MethodPost {
			serverStats.failedRequests.Add(1)
			serverStats.lastError.Store("method not allowed")
			writeError(w, http.StatusMethodNotAllowed, "method not allowed", "method_not_allowed")
			return
		}

		var req chatRequest
		// mock upstream 是压测专用独立工具，容器镜像需保持标准库依赖，避免为了
		// 本地假上游下载主项目完整依赖图，拖慢压测环境启动。
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			serverStats.failedRequests.Add(1)
			serverStats.lastError.Store("invalid json: " + err.Error())
			writeError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error")
			return
		}
		if strings.TrimSpace(req.Model) == "" {
			req.Model = cfg.model
		}

		if shouldInject(cfg.rateLimitRate) {
			serverStats.failedRequests.Add(1)
			serverStats.rateLimitedRequests.Add(1)
			serverStats.lastError.Store("injected rate limit")
			writeError(w, http.StatusTooManyRequests, "mock upstream injected rate limit", "rate_limit_exceeded")
			return
		}
		if shouldInject(cfg.errorRate) {
			serverStats.failedRequests.Add(1)
			serverStats.lastError.Store("injected upstream error")
			writeError(w, http.StatusInternalServerError, "mock upstream injected error", "internal_error")
			return
		}

		if req.Stream {
			serverStats.streamRequests.Add(1)
			if err := writeStreamResponse(w, cfg, req.Model); err != nil {
				serverStats.failedRequests.Add(1)
				serverStats.lastError.Store(err.Error())
				return
			}
			serverStats.successRequests.Add(1)
			return
		}

		serverStats.nonStreamRequests.Add(1)
		time.Sleep(cfg.ttft + time.Duration(cfg.outputTokens)*cfg.tokenDelay)
		writeJSON(w, http.StatusOK, buildChatResponse(cfg, req.Model))
		serverStats.successRequests.Add(1)
	}
}

func shouldInject(rate float64) bool {
	return rate > 0 && randomFloat() < rate
}

func recordLatency(serverStats *stats, start time.Time) {
	latencyMs := time.Since(start).Milliseconds()
	serverStats.totalLatencyMs.Add(latencyMs)
	for {
		current := serverStats.maxLatencyMs.Load()
		if latencyMs <= current || serverStats.maxLatencyMs.CompareAndSwap(current, latencyMs) {
			return
		}
	}
}

func buildChatResponse(cfg config, model string) chatResponse {
	content := buildCompletionText(cfg.outputTokens, cfg.bodyBytes)
	promptTokens := 12
	return chatResponse{
		ID:      fmt.Sprintf("chatcmpl-loadtest-%d", time.Now().UnixNano()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []chatChoice{{
			Index:        0,
			Message:      chatMessage{Role: "assistant", Content: content},
			FinishReason: "stop",
		}},
		Usage: chatUsage{
			PromptTokens:     promptTokens,
			CompletionTokens: cfg.outputTokens,
			TotalTokens:      promptTokens + cfg.outputTokens,
		},
	}
}

func buildCompletionText(tokens int, bodyBytes int) string {
	if tokens <= 0 {
		tokens = 1
	}
	parts := make([]string, 0, tokens)
	for i := 0; i < tokens; i++ {
		parts = append(parts, fmt.Sprintf("token-%03d", i+1))
	}
	content := strings.Join(parts, " ")
	if bodyBytes > 0 {
		content += "\n" + strings.Repeat("x", bodyBytes)
	}
	return content
}

func writeStreamResponse(w http.ResponseWriter, cfg config, model string) error {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("response writer does not support flushing")
	}

	id := fmt.Sprintf("chatcmpl-loadtest-%d", time.Now().UnixNano())
	created := time.Now().Unix()
	time.Sleep(cfg.ttft)

	if err := writeSSEChunk(w, streamChunk{
		ID:      id,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   model,
		Choices: []streamChunkChoice{{
			Index: 0,
			Delta: map[string]string{"role": "assistant"},
		}},
	}); err != nil {
		return err
	}
	flusher.Flush()

	if cfg.outputTokens <= 0 {
		cfg.outputTokens = 1
	}
	for i := 0; i < cfg.outputTokens; i++ {
		if cfg.tokenDelay > 0 {
			time.Sleep(cfg.tokenDelay)
		}
		if err := writeSSEChunk(w, streamChunk{
			ID:      id,
			Object:  "chat.completion.chunk",
			Created: created,
			Model:   model,
			Choices: []streamChunkChoice{{
				Index: 0,
				Delta: map[string]string{"content": fmt.Sprintf("token-%03d ", i+1)},
			}},
		}); err != nil {
			return err
		}
		flusher.Flush()
	}

	finishReason := "stop"
	if err := writeSSEChunk(w, streamChunk{
		ID:      id,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   model,
		Choices: []streamChunkChoice{{
			Index:        0,
			Delta:        map[string]string{},
			FinishReason: &finishReason,
		}},
	}); err != nil {
		return err
	}
	if _, err := fmt.Fprint(w, "data: [DONE]\n\n"); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func writeSSEChunk(w http.ResponseWriter, chunk streamChunk) error {
	payload, err := json.Marshal(chunk)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "data: %s\n\n", payload)
	return err
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to encode response", "internal_error")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

func writeError(w http.ResponseWriter, status int, message string, code string) {
	data, err := json.Marshal(openAIErrorResponse{
		Error: openAIErrorBox{
			Message: message,
			Type:    code,
			Code:    code,
		},
	})
	if err != nil {
		http.Error(w, message, status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}
