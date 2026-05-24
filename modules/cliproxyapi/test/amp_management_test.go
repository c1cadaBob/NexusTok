// Package test - amp_management_test.go
// AmpCode 管理 API 的端到端集成测试。
// 测试覆盖了 AmpCode 配置的各个管理端点，包括：
// - 上游 URL 的增删改查
// - 上游 API 密钥的管理
// - 本地主机限制设置
// - 模型映射的增删改查（包括 PATCH 合并操作）
// - 强制模型映射开关
// - 状态持久化验证
package test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/api/handlers/management"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
)

// init 设置 Gin 为测试模式。
func init() {
	gin.SetMode(gin.TestMode)
}

// newAmpTestHandler 创建一个带有默认 AmpCode 配置的测试处理器。
// 它在临时目录中创建配置文件，并返回处理器实例和配置文件路径。
//
// 参数:
//   - t: 测试实例
//
// 返回值:
//   - *management.Handler: 测试处理器实例
//   - string: 配置文件路径
func newAmpTestHandler(t *testing.T) (*management.Handler, string) {
	t.Helper()
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	cfg := &config.Config{
		AmpCode: config.AmpCode{
			UpstreamURL:                   "https://example.com",
			UpstreamAPIKey:                "test-api-key-12345",
			RestrictManagementToLocalhost: true,
			ForceModelMappings:            false,
			ModelMappings: []config.AmpModelMapping{
				{From: "gpt-4", To: "gemini-pro"},
			},
		},
	}

	if err := os.WriteFile(configPath, []byte("port: 8080\n"), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	h := management.NewHandler(cfg, configPath, nil)
	return h, configPath
}

// setupAmpRouter 创建一个包含所有 AmpCode 管理端点的测试路由器。
// 注册的端点包括：
// - GET/PUT/DELETE /ampcode/upstream-url
// - GET/PUT/DELETE /ampcode/upstream-api-key
// - GET/PUT/PATCH/DELETE /ampcode/upstream-api-keys
// - GET/PUT /ampcode/restrict-management-to-localhost
// - GET/PUT/PATCH/DELETE /ampcode/model-mappings
// - GET/PUT /ampcode/force-model-mappings
func setupAmpRouter(h *management.Handler) *gin.Engine {
	r := gin.New()
	mgmt := r.Group("/v0/management")
	{
		mgmt.GET("/ampcode", h.GetAmpCode)
		mgmt.GET("/ampcode/upstream-url", h.GetAmpUpstreamURL)
		mgmt.PUT("/ampcode/upstream-url", h.PutAmpUpstreamURL)
		mgmt.DELETE("/ampcode/upstream-url", h.DeleteAmpUpstreamURL)
		mgmt.GET("/ampcode/upstream-api-key", h.GetAmpUpstreamAPIKey)
		mgmt.PUT("/ampcode/upstream-api-key", h.PutAmpUpstreamAPIKey)
		mgmt.DELETE("/ampcode/upstream-api-key", h.DeleteAmpUpstreamAPIKey)
		mgmt.GET("/ampcode/upstream-api-keys", h.GetAmpUpstreamAPIKeys)
		mgmt.PUT("/ampcode/upstream-api-keys", h.PutAmpUpstreamAPIKeys)
		mgmt.PATCH("/ampcode/upstream-api-keys", h.PatchAmpUpstreamAPIKeys)
		mgmt.DELETE("/ampcode/upstream-api-keys", h.DeleteAmpUpstreamAPIKeys)
		mgmt.GET("/ampcode/restrict-management-to-localhost", h.GetAmpRestrictManagementToLocalhost)
		mgmt.PUT("/ampcode/restrict-management-to-localhost", h.PutAmpRestrictManagementToLocalhost)
		mgmt.GET("/ampcode/model-mappings", h.GetAmpModelMappings)
		mgmt.PUT("/ampcode/model-mappings", h.PutAmpModelMappings)
		mgmt.PATCH("/ampcode/model-mappings", h.PatchAmpModelMappings)
		mgmt.DELETE("/ampcode/model-mappings", h.DeleteAmpModelMappings)
		mgmt.GET("/ampcode/force-model-mappings", h.GetAmpForceModelMappings)
		mgmt.PUT("/ampcode/force-model-mappings", h.PutAmpForceModelMappings)
	}
	return r
}

// TestGetAmpCode 验证 GET /v0/management/ampcode 返回完整的 AmpCode 配置。
func TestGetAmpCode(t *testing.T) {
	h, _ := newAmpTestHandler(t)
	r := setupAmpRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/v0/management/ampcode", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp map[string]config.AmpCode
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	ampcode := resp["ampcode"]
	if ampcode.UpstreamURL != "https://example.com" {
		t.Errorf("expected upstream-url %q, got %q", "https://example.com", ampcode.UpstreamURL)
	}
	if len(ampcode.ModelMappings) != 1 {
		t.Errorf("expected 1 model mapping, got %d", len(ampcode.ModelMappings))
	}
}

// TestGetAmpUpstreamURL 验证 GET /v0/management/ampcode/upstream-url 返回上游 URL。
func TestGetAmpUpstreamURL(t *testing.T) {
	h, _ := newAmpTestHandler(t)
	r := setupAmpRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/v0/management/ampcode/upstream-url", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp["upstream-url"] != "https://example.com" {
		t.Errorf("expected %q, got %q", "https://example.com", resp["upstream-url"])
	}
}

// TestPutAmpUpstreamURL 验证 PUT /v0/management/ampcode/upstream-url 更新上游 URL。
func TestPutAmpUpstreamURL(t *testing.T) {
	h, _ := newAmpTestHandler(t)
	r := setupAmpRouter(h)

	body := `{"value": "https://new-upstream.com"}`
	req := httptest.NewRequest(http.MethodPut, "/v0/management/ampcode/upstream-url", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}
}

// TestDeleteAmpUpstreamURL 验证 DELETE /v0/management/ampcode/upstream-url 清除上游 URL。
func TestDeleteAmpUpstreamURL(t *testing.T) {
	h, _ := newAmpTestHandler(t)
	r := setupAmpRouter(h)

	req := httptest.NewRequest(http.MethodDelete, "/v0/management/ampcode/upstream-url", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

// TestGetAmpUpstreamAPIKey 验证 GET /v0/management/ampcode/upstream-api-key 返回 API 密钥。
func TestGetAmpUpstreamAPIKey(t *testing.T) {
	h, _ := newAmpTestHandler(t)
	r := setupAmpRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/v0/management/ampcode/upstream-api-key", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	key := resp["upstream-api-key"].(string)
	if key != "test-api-key-12345" {
		t.Errorf("expected key %q, got %q", "test-api-key-12345", key)
	}
}

// TestPutAmpUpstreamAPIKey 验证 PUT /v0/management/ampcode/upstream-api-key 更新 API 密钥。
func TestPutAmpUpstreamAPIKey(t *testing.T) {
	h, _ := newAmpTestHandler(t)
	r := setupAmpRouter(h)

	body := `{"value": "new-secret-key"}`
	req := httptest.NewRequest(http.MethodPut, "/v0/management/ampcode/upstream-api-key", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

// TestPutAmpUpstreamAPIKeys_PersistsAndReturns 验证 PUT upstream-api-keys 持久化并返回正确结果。
func TestPutAmpUpstreamAPIKeys_PersistsAndReturns(t *testing.T) {
	h, configPath := newAmpTestHandler(t)
	r := setupAmpRouter(h)

	body := `{"value":[{"upstream-api-key":"  u1  ","api-keys":["  k1  ","","k2"]}]}`
	req := httptest.NewRequest(http.MethodPut, "/v0/management/ampcode/upstream-api-keys", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	// Verify it was persisted to disk
	loaded, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config from disk: %v", err)
	}
	if len(loaded.AmpCode.UpstreamAPIKeys) != 1 {
		t.Fatalf("expected 1 upstream-api-keys entry, got %d", len(loaded.AmpCode.UpstreamAPIKeys))
	}
	entry := loaded.AmpCode.UpstreamAPIKeys[0]
	if entry.UpstreamAPIKey != "u1" {
		t.Fatalf("expected upstream-api-key u1, got %q", entry.UpstreamAPIKey)
	}
	if len(entry.APIKeys) != 2 || entry.APIKeys[0] != "k1" || entry.APIKeys[1] != "k2" {
		t.Fatalf("expected api-keys [k1 k2], got %#v", entry.APIKeys)
	}

	// Verify it is returned by GET /ampcode
	req = httptest.NewRequest(http.MethodGet, "/v0/management/ampcode", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}
	var resp map[string]config.AmpCode
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if got := resp["ampcode"].UpstreamAPIKeys; len(got) != 1 || got[0].UpstreamAPIKey != "u1" {
		t.Fatalf("expected upstream-api-keys to be present after update, got %#v", got)
	}
}

// TestDeleteAmpUpstreamAPIKeys_ClearsAll 验证 DELETE upstream-api-keys 清除所有条目。
func TestDeleteAmpUpstreamAPIKeys_ClearsAll(t *testing.T) {
	h, _ := newAmpTestHandler(t)
	r := setupAmpRouter(h)

	// Seed with one entry
	putBody := `{"value":[{"upstream-api-key":"u1","api-keys":["k1"]}]}`
	req := httptest.NewRequest(http.MethodPut, "/v0/management/ampcode/upstream-api-keys", bytes.NewBufferString(putBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	deleteBody := `{"value":[]}`
	req = httptest.NewRequest(http.MethodDelete, "/v0/management/ampcode/upstream-api-keys", bytes.NewBufferString(deleteBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/v0/management/ampcode/upstream-api-keys", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}
	var resp map[string][]config.AmpUpstreamAPIKeyEntry
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp["upstream-api-keys"] != nil && len(resp["upstream-api-keys"]) != 0 {
		t.Fatalf("expected cleared list, got %#v", resp["upstream-api-keys"])
	}
}

// TestDeleteAmpUpstreamAPIKey 验证 DELETE /v0/management/ampcode/upstream-api-key 清除 API 密钥。
func TestDeleteAmpUpstreamAPIKey(t *testing.T) {
	h, _ := newAmpTestHandler(t)
	r := setupAmpRouter(h)

	req := httptest.NewRequest(http.MethodDelete, "/v0/management/ampcode/upstream-api-key", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

// TestGetAmpRestrictManagementToLocalhost 验证 GET 返回本地主机限制设置。
func TestGetAmpRestrictManagementToLocalhost(t *testing.T) {
	h, _ := newAmpTestHandler(t)
	r := setupAmpRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/v0/management/ampcode/restrict-management-to-localhost", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp map[string]bool
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp["restrict-management-to-localhost"] != true {
		t.Error("expected restrict-management-to-localhost to be true")
	}
}

// TestPutAmpRestrictManagementToLocalhost 验证 PUT 更新本地主机限制设置。
func TestPutAmpRestrictManagementToLocalhost(t *testing.T) {
	h, _ := newAmpTestHandler(t)
	r := setupAmpRouter(h)

	body := `{"value": false}`
	req := httptest.NewRequest(http.MethodPut, "/v0/management/ampcode/restrict-management-to-localhost", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

// TestGetAmpModelMappings 验证 GET /v0/management/ampcode/model-mappings 返回所有映射。
func TestGetAmpModelMappings(t *testing.T) {
	h, _ := newAmpTestHandler(t)
	r := setupAmpRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/v0/management/ampcode/model-mappings", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp map[string][]config.AmpModelMapping
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	mappings := resp["model-mappings"]
	if len(mappings) != 1 {
		t.Fatalf("expected 1 mapping, got %d", len(mappings))
	}
	if mappings[0].From != "gpt-4" || mappings[0].To != "gemini-pro" {
		t.Errorf("unexpected mapping: %+v", mappings[0])
	}
}

// TestPutAmpModelMappings 验证 PUT /v0/management/ampcode/model-mappings 替换所有映射。
func TestPutAmpModelMappings(t *testing.T) {
	h, _ := newAmpTestHandler(t)
	r := setupAmpRouter(h)

	body := `{"value": [{"from": "claude-3", "to": "gpt-4o"}, {"from": "gemini", "to": "claude"}]}`
	req := httptest.NewRequest(http.MethodPut, "/v0/management/ampcode/model-mappings", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}
}

// TestPatchAmpModelMappings 验证 PATCH 更新现有映射并添加新映射。
func TestPatchAmpModelMappings(t *testing.T) {
	h, _ := newAmpTestHandler(t)
	r := setupAmpRouter(h)

	body := `{"value": [{"from": "gpt-4", "to": "updated-model"}, {"from": "new-model", "to": "target"}]}`
	req := httptest.NewRequest(http.MethodPatch, "/v0/management/ampcode/model-mappings", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}
}

// TestDeleteAmpModelMappings_Specific 验证 DELETE 按 "from" 字段移除指定映射。
func TestDeleteAmpModelMappings_Specific(t *testing.T) {
	h, _ := newAmpTestHandler(t)
	r := setupAmpRouter(h)

	body := `{"value": ["gpt-4"]}`
	req := httptest.NewRequest(http.MethodDelete, "/v0/management/ampcode/model-mappings", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

// TestDeleteAmpModelMappings_All 验证 DELETE 空请求体移除所有映射。
func TestDeleteAmpModelMappings_All(t *testing.T) {
	h, _ := newAmpTestHandler(t)
	r := setupAmpRouter(h)

	req := httptest.NewRequest(http.MethodDelete, "/v0/management/ampcode/model-mappings", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

// TestGetAmpForceModelMappings 验证 GET 返回 force-model-mappings 设置。
func TestGetAmpForceModelMappings(t *testing.T) {
	h, _ := newAmpTestHandler(t)
	r := setupAmpRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/v0/management/ampcode/force-model-mappings", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp map[string]bool
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp["force-model-mappings"] != false {
		t.Error("expected force-model-mappings to be false")
	}
}

// TestPutAmpForceModelMappings 验证 PUT 更新 force-model-mappings 设置。
func TestPutAmpForceModelMappings(t *testing.T) {
	h, _ := newAmpTestHandler(t)
	r := setupAmpRouter(h)

	body := `{"value": true}`
	req := httptest.NewRequest(http.MethodPut, "/v0/management/ampcode/force-model-mappings", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

// TestPutAmpModelMappings_VerifyState 验证 PUT 替换映射后状态持久化正确。
func TestPutAmpModelMappings_VerifyState(t *testing.T) {
	h, _ := newAmpTestHandler(t)
	r := setupAmpRouter(h)

	body := `{"value": [{"from": "model-a", "to": "model-b"}, {"from": "model-c", "to": "model-d"}, {"from": "model-e", "to": "model-f"}]}`
	req := httptest.NewRequest(http.MethodPut, "/v0/management/ampcode/model-mappings", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("PUT failed: status %d, body: %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v0/management/ampcode/model-mappings", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp map[string][]config.AmpModelMapping
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	mappings := resp["model-mappings"]
	if len(mappings) != 3 {
		t.Fatalf("expected 3 mappings, got %d", len(mappings))
	}

	expected := map[string]string{"model-a": "model-b", "model-c": "model-d", "model-e": "model-f"}
	for _, m := range mappings {
		if expected[m.From] != m.To {
			t.Errorf("mapping %q -> expected %q, got %q", m.From, expected[m.From], m.To)
		}
	}
}

// TestPatchAmpModelMappings_VerifyState 验证 PATCH 正确合并映射。
func TestPatchAmpModelMappings_VerifyState(t *testing.T) {
	h, _ := newAmpTestHandler(t)
	r := setupAmpRouter(h)

	body := `{"value": [{"from": "gpt-4", "to": "updated-target"}, {"from": "new-model", "to": "new-target"}]}`
	req := httptest.NewRequest(http.MethodPatch, "/v0/management/ampcode/model-mappings", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("PATCH failed: status %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/v0/management/ampcode/model-mappings", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp map[string][]config.AmpModelMapping
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	mappings := resp["model-mappings"]
	if len(mappings) != 2 {
		t.Fatalf("expected 2 mappings (1 updated + 1 new), got %d", len(mappings))
	}

	found := make(map[string]string)
	for _, m := range mappings {
		found[m.From] = m.To
	}

	if found["gpt-4"] != "updated-target" {
		t.Errorf("gpt-4 should map to updated-target, got %q", found["gpt-4"])
	}
	if found["new-model"] != "new-target" {
		t.Errorf("new-model should map to new-target, got %q", found["new-model"])
	}
}

// TestDeleteAmpModelMappings_VerifyState 验证 DELETE 移除指定映射并保留其他映射。
func TestDeleteAmpModelMappings_VerifyState(t *testing.T) {
	h, _ := newAmpTestHandler(t)
	r := setupAmpRouter(h)

	putBody := `{"value": [{"from": "a", "to": "1"}, {"from": "b", "to": "2"}, {"from": "c", "to": "3"}]}`
	req := httptest.NewRequest(http.MethodPut, "/v0/management/ampcode/model-mappings", bytes.NewBufferString(putBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	delBody := `{"value": ["a", "c"]}`
	req = httptest.NewRequest(http.MethodDelete, "/v0/management/ampcode/model-mappings", bytes.NewBufferString(delBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("DELETE failed: status %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/v0/management/ampcode/model-mappings", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp map[string][]config.AmpModelMapping
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	mappings := resp["model-mappings"]
	if len(mappings) != 1 {
		t.Fatalf("expected 1 mapping remaining, got %d", len(mappings))
	}
	if mappings[0].From != "b" || mappings[0].To != "2" {
		t.Errorf("expected b->2, got %s->%s", mappings[0].From, mappings[0].To)
	}
}

// TestDeleteAmpModelMappings_NonExistent 验证 DELETE 不存在的映射不影响现有映射。
func TestDeleteAmpModelMappings_NonExistent(t *testing.T) {
	h, _ := newAmpTestHandler(t)
	r := setupAmpRouter(h)

	delBody := `{"value": ["non-existent-model"]}`
	req := httptest.NewRequest(http.MethodDelete, "/v0/management/ampcode/model-mappings", bytes.NewBufferString(delBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/v0/management/ampcode/model-mappings", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp map[string][]config.AmpModelMapping
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(resp["model-mappings"]) != 1 {
		t.Errorf("original mapping should remain, got %d mappings", len(resp["model-mappings"]))
	}
}

// TestPutAmpModelMappings_Empty 验证 PUT 空数组清除所有映射。
func TestPutAmpModelMappings_Empty(t *testing.T) {
	h, _ := newAmpTestHandler(t)
	r := setupAmpRouter(h)

	body := `{"value": []}`
	req := httptest.NewRequest(http.MethodPut, "/v0/management/ampcode/model-mappings", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/v0/management/ampcode/model-mappings", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp map[string][]config.AmpModelMapping
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(resp["model-mappings"]) != 0 {
		t.Errorf("expected 0 mappings, got %d", len(resp["model-mappings"]))
	}
}

// TestPutAmpUpstreamURL_VerifyState 验证 PUT 更新上游 URL 并持久化状态。
func TestPutAmpUpstreamURL_VerifyState(t *testing.T) {
	h, _ := newAmpTestHandler(t)
	r := setupAmpRouter(h)

	body := `{"value": "https://new-api.example.com"}`
	req := httptest.NewRequest(http.MethodPut, "/v0/management/ampcode/upstream-url", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("PUT failed: status %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/v0/management/ampcode/upstream-url", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if resp["upstream-url"] != "https://new-api.example.com" {
		t.Errorf("expected %q, got %q", "https://new-api.example.com", resp["upstream-url"])
	}
}

// TestDeleteAmpUpstreamURL_VerifyState 验证 DELETE 清除上游 URL。
func TestDeleteAmpUpstreamURL_VerifyState(t *testing.T) {
	h, _ := newAmpTestHandler(t)
	r := setupAmpRouter(h)

	req := httptest.NewRequest(http.MethodDelete, "/v0/management/ampcode/upstream-url", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("DELETE failed: status %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/v0/management/ampcode/upstream-url", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if resp["upstream-url"] != "" {
		t.Errorf("expected empty string, got %q", resp["upstream-url"])
	}
}

// TestPutAmpUpstreamAPIKey_VerifyState 验证 PUT 更新 API 密钥并持久化状态。
func TestPutAmpUpstreamAPIKey_VerifyState(t *testing.T) {
	h, _ := newAmpTestHandler(t)
	r := setupAmpRouter(h)

	body := `{"value": "new-secret-api-key-xyz"}`
	req := httptest.NewRequest(http.MethodPut, "/v0/management/ampcode/upstream-api-key", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("PUT failed: status %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/v0/management/ampcode/upstream-api-key", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if resp["upstream-api-key"] != "new-secret-api-key-xyz" {
		t.Errorf("expected %q, got %q", "new-secret-api-key-xyz", resp["upstream-api-key"])
	}
}

// TestDeleteAmpUpstreamAPIKey_VerifyState 验证 DELETE 清除 API 密钥。
func TestDeleteAmpUpstreamAPIKey_VerifyState(t *testing.T) {
	h, _ := newAmpTestHandler(t)
	r := setupAmpRouter(h)

	req := httptest.NewRequest(http.MethodDelete, "/v0/management/ampcode/upstream-api-key", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("DELETE failed: status %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/v0/management/ampcode/upstream-api-key", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if resp["upstream-api-key"] != "" {
		t.Errorf("expected empty string, got %q", resp["upstream-api-key"])
	}
}

// TestPutAmpRestrictManagementToLocalhost_VerifyState 验证 PUT 更新本地主机限制。
func TestPutAmpRestrictManagementToLocalhost_VerifyState(t *testing.T) {
	h, _ := newAmpTestHandler(t)
	r := setupAmpRouter(h)

	body := `{"value": false}`
	req := httptest.NewRequest(http.MethodPut, "/v0/management/ampcode/restrict-management-to-localhost", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("PUT failed: status %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/v0/management/ampcode/restrict-management-to-localhost", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp map[string]bool
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if resp["restrict-management-to-localhost"] != false {
		t.Error("expected false after update")
	}
}

// TestPutAmpForceModelMappings_VerifyState 验证 PUT 更新 force-model-mappings 设置。
func TestPutAmpForceModelMappings_VerifyState(t *testing.T) {
	h, _ := newAmpTestHandler(t)
	r := setupAmpRouter(h)

	body := `{"value": true}`
	req := httptest.NewRequest(http.MethodPut, "/v0/management/ampcode/force-model-mappings", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("PUT failed: status %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/v0/management/ampcode/force-model-mappings", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp map[string]bool
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if resp["force-model-mappings"] != true {
		t.Error("expected true after update")
	}
}

// TestPutBoolField_EmptyObject 验证 PUT 空对象返回 400 错误。
func TestPutBoolField_EmptyObject(t *testing.T) {
	h, _ := newAmpTestHandler(t)
	r := setupAmpRouter(h)

	body := `{}`
	req := httptest.NewRequest(http.MethodPut, "/v0/management/ampcode/force-model-mappings", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d for empty object, got %d", http.StatusBadRequest, w.Code)
	}
}

// TestComplexMappingsWorkflow 测试完整的工作流程：PUT、PATCH、DELETE 和 GET。
func TestComplexMappingsWorkflow(t *testing.T) {
	h, _ := newAmpTestHandler(t)
	r := setupAmpRouter(h)

	putBody := `{"value": [{"from": "m1", "to": "t1"}, {"from": "m2", "to": "t2"}, {"from": "m3", "to": "t3"}, {"from": "m4", "to": "t4"}]}`
	req := httptest.NewRequest(http.MethodPut, "/v0/management/ampcode/model-mappings", bytes.NewBufferString(putBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	patchBody := `{"value": [{"from": "m2", "to": "t2-updated"}, {"from": "m5", "to": "t5"}]}`
	req = httptest.NewRequest(http.MethodPatch, "/v0/management/ampcode/model-mappings", bytes.NewBufferString(patchBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	delBody := `{"value": ["m1", "m3"]}`
	req = httptest.NewRequest(http.MethodDelete, "/v0/management/ampcode/model-mappings", bytes.NewBufferString(delBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	req = httptest.NewRequest(http.MethodGet, "/v0/management/ampcode/model-mappings", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp map[string][]config.AmpModelMapping
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	mappings := resp["model-mappings"]
	if len(mappings) != 3 {
		t.Fatalf("expected 3 mappings (m2, m4, m5), got %d", len(mappings))
	}

	expected := map[string]string{"m2": "t2-updated", "m4": "t4", "m5": "t5"}
	found := make(map[string]string)
	for _, m := range mappings {
		found[m.From] = m.To
	}

	for from, to := range expected {
		if found[from] != to {
			t.Errorf("mapping %s: expected %q, got %q", from, to, found[from])
		}
	}
}

// TestNilHandlerGetAmpCode 验证处理器在空配置下正常工作。
func TestNilHandlerGetAmpCode(t *testing.T) {
	cfg := &config.Config{}
	h := management.NewHandler(cfg, "", nil)
	r := setupAmpRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/v0/management/ampcode", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

// TestEmptyConfigGetAmpModelMappings 验证 GET 对空配置返回空数组。
func TestEmptyConfigGetAmpModelMappings(t *testing.T) {
	cfg := &config.Config{}
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("port: 8080\n"), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	h := management.NewHandler(cfg, configPath, nil)
	r := setupAmpRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/v0/management/ampcode/model-mappings", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp map[string][]config.AmpModelMapping
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(resp["model-mappings"]) != 0 {
		t.Errorf("expected 0 mappings, got %d", len(resp["model-mappings"]))
	}
}
