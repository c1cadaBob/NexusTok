package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testConfig() config {
	return config{
		addr:         ":0",
		model:        defaultModel,
		ttft:         0,
		tokenDelay:   0,
		outputTokens: 3,
	}
}

func TestNonStreamChatCompletionShape(t *testing.T) {
	handler := newServer(testConfig(), &stats{startedAt: time.Now()})
	body := `{"model":"gpt-loadtest","messages":[{"role":"user","content":"ping"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp chatResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Object != "chat.completion" {
		t.Fatalf("unexpected object: %s", resp.Object)
	}
	if len(resp.Choices) != 1 || resp.Choices[0].Message.Content == "" {
		t.Fatalf("missing assistant choice: %+v", resp.Choices)
	}
	if resp.Usage.CompletionTokens != 3 || resp.Usage.TotalTokens <= resp.Usage.PromptTokens {
		t.Fatalf("unexpected usage: %+v", resp.Usage)
	}
}

func TestStreamChatCompletionOrder(t *testing.T) {
	cfg := testConfig()
	cfg.outputTokens = 2
	handler := newServer(cfg, &stats{startedAt: time.Now()})
	body := `{"model":"gpt-loadtest","stream":true,"messages":[{"role":"user","content":"ping"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	output := rec.Body.String()
	roleIndex := strings.Index(output, `"role":"assistant"`)
	firstTokenIndex := strings.Index(output, "token-001")
	doneIndex := strings.Index(output, "data: [DONE]")
	if roleIndex < 0 || firstTokenIndex < 0 || doneIndex < 0 {
		t.Fatalf("stream output missing expected chunks: %s", output)
	}
	if !(roleIndex < firstTokenIndex && firstTokenIndex < doneIndex) {
		t.Fatalf("stream chunks are out of order: %s", output)
	}
}

func TestInjectedRateLimit(t *testing.T) {
	cfg := testConfig()
	cfg.rateLimitRate = 1
	serverStats := &stats{startedAt: time.Now()}
	handler := newServer(cfg, serverStats)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-loadtest"}`))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d: %s", rec.Code, rec.Body.String())
	}
	if serverStats.rateLimitedRequests.Load() != 1 || serverStats.failedRequests.Load() != 1 {
		t.Fatalf("unexpected stats: rate_limited=%d failed=%d", serverStats.rateLimitedRequests.Load(), serverStats.failedRequests.Load())
	}
}

func TestInjectedError(t *testing.T) {
	cfg := testConfig()
	cfg.errorRate = 1
	serverStats := &stats{startedAt: time.Now()}
	handler := newServer(cfg, serverStats)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-loadtest"}`))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
	if serverStats.failedRequests.Load() != 1 {
		t.Fatalf("unexpected failed_requests: %d", serverStats.failedRequests.Load())
	}
}

func TestLoadConfigFromEnv(t *testing.T) {
	t.Setenv("MOCK_OPENAI_MODEL", "custom-model")
	t.Setenv("MOCK_OPENAI_TTFT_MS", "12")
	t.Setenv("MOCK_OPENAI_TOKEN_DELAY_MS", "7")
	t.Setenv("MOCK_OPENAI_OUTPUT_TOKENS", "9")
	t.Setenv("MOCK_OPENAI_ERROR_RATE", "2")
	t.Setenv("MOCK_OPENAI_RATE_LIMIT_RATE", "-1")
	t.Setenv("MOCK_OPENAI_BODY_BYTES", "128")

	cfg := loadConfigFromEnv()

	if cfg.model != "custom-model" {
		t.Fatalf("unexpected model: %s", cfg.model)
	}
	if cfg.ttft != 12*time.Millisecond || cfg.tokenDelay != 7*time.Millisecond {
		t.Fatalf("unexpected delay config: %+v", cfg)
	}
	if cfg.outputTokens != 9 || cfg.bodyBytes != 128 {
		t.Fatalf("unexpected size config: %+v", cfg)
	}
	if cfg.errorRate != 1 || cfg.rateLimitRate != 0 {
		t.Fatalf("rates should be clamped, got error=%f rate_limit=%f", cfg.errorRate, cfg.rateLimitRate)
	}
}
