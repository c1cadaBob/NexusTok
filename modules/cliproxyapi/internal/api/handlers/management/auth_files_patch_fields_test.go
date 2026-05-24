// management - auth_files_patch_fields_test.go
// 认证文件字段修补端点的单元测试。
// 测试 PatchAuthFileFields 端点的以下功能：
// - 合并 headers 字段并删除空值（空白字符串和空字符串被视为删除操作）
// - headers 空映射的无操作处理（不删除现有 headers）
// - 账号分组（account_group）的持久化和列表返回
// - 多账号分组（account_groups）的支持，包括去重、修剪和清空操作
package management

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

// TestPatchAuthFileFields_MergeHeadersAndDeleteEmptyValues 测试 headers 字段的合并和删除语义：
// - 已存在的 header 值被新值覆盖（X-Old: old -> new）
// - 新的 header 被添加（X-New: v）
// - 值为空白字符串的 header 被视为删除操作（X-Remove: "  " -> 删除）
// - 值为空字符串且不存在的 header 被忽略（X-Nope: "" -> 不添加）
// 同时验证 prefix 和 proxy_url 的正确持久化，以及 metadata 中对应字段的同步更新。
func TestPatchAuthFileFields_MergeHeadersAndDeleteEmptyValues(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "")
	gin.SetMode(gin.TestMode)

	store := &memoryAuthStore{}
	manager := coreauth.NewManager(store, nil, nil)
	record := &coreauth.Auth{
		ID:       "test.json",
		FileName: "test.json",
		Provider: "claude",
		Attributes: map[string]string{
			"path":            "/tmp/test.json",
			"header:X-Old":    "old",
			"header:X-Remove": "gone",
		},
		Metadata: map[string]any{
			"type": "claude",
			"headers": map[string]any{
				"X-Old":    "old",
				"X-Remove": "gone",
			},
		},
	}
	if _, errRegister := manager.Register(context.Background(), record); errRegister != nil {
		t.Fatalf("failed to register auth record: %v", errRegister)
	}

	h := NewHandlerWithoutConfigFilePath(&config.Config{AuthDir: t.TempDir()}, manager)

	body := `{"name":"test.json","prefix":"p1","proxy_url":"http://proxy.local","headers":{"X-Old":"new","X-New":"v","X-Remove":"  ","X-Nope":""}}`
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodPatch, "/v0/management/auth-files/fields", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req
	h.PatchAuthFileFields(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	updated, ok := manager.GetByID("test.json")
	if !ok || updated == nil {
		t.Fatalf("expected auth record to exist after patch")
	}

	if updated.Prefix != "p1" {
		t.Fatalf("prefix = %q, want %q", updated.Prefix, "p1")
	}
	if updated.ProxyURL != "http://proxy.local" {
		t.Fatalf("proxy_url = %q, want %q", updated.ProxyURL, "http://proxy.local")
	}

	if updated.Metadata == nil {
		t.Fatalf("expected metadata to be non-nil")
	}
	if got, _ := updated.Metadata["prefix"].(string); got != "p1" {
		t.Fatalf("metadata.prefix = %q, want %q", got, "p1")
	}
	if got, _ := updated.Metadata["proxy_url"].(string); got != "http://proxy.local" {
		t.Fatalf("metadata.proxy_url = %q, want %q", got, "http://proxy.local")
	}

	headersMeta, ok := updated.Metadata["headers"].(map[string]any)
	if !ok {
		raw, _ := json.Marshal(updated.Metadata["headers"])
		t.Fatalf("metadata.headers = %T (%s), want map[string]any", updated.Metadata["headers"], string(raw))
	}
	if got := headersMeta["X-Old"]; got != "new" {
		t.Fatalf("metadata.headers.X-Old = %#v, want %q", got, "new")
	}
	if got := headersMeta["X-New"]; got != "v" {
		t.Fatalf("metadata.headers.X-New = %#v, want %q", got, "v")
	}
	if _, ok := headersMeta["X-Remove"]; ok {
		t.Fatalf("expected metadata.headers.X-Remove to be deleted")
	}
	if _, ok := headersMeta["X-Nope"]; ok {
		t.Fatalf("expected metadata.headers.X-Nope to be absent")
	}

	if got := updated.Attributes["header:X-Old"]; got != "new" {
		t.Fatalf("attrs header:X-Old = %q, want %q", got, "new")
	}
	if got := updated.Attributes["header:X-New"]; got != "v" {
		t.Fatalf("attrs header:X-New = %q, want %q", got, "v")
	}
	if _, ok := updated.Attributes["header:X-Remove"]; ok {
		t.Fatalf("expected attrs header:X-Remove to be deleted")
	}
	if _, ok := updated.Attributes["header:X-Nope"]; ok {
		t.Fatalf("expected attrs header:X-Nope to be absent")
	}
}

// TestPatchAuthFileFields_HeadersEmptyMapIsNoop 测试当请求中包含空的 headers 映射时，
// 现有的 headers 不会被删除。这是一种无操作（noop）场景，
// 确保空映射不会意外清除已配置的 header。
func TestPatchAuthFileFields_HeadersEmptyMapIsNoop(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "")
	gin.SetMode(gin.TestMode)

	store := &memoryAuthStore{}
	manager := coreauth.NewManager(store, nil, nil)
	record := &coreauth.Auth{
		ID:       "noop.json",
		FileName: "noop.json",
		Provider: "claude",
		Attributes: map[string]string{
			"path":         "/tmp/noop.json",
			"header:X-Kee": "1",
		},
		Metadata: map[string]any{
			"type": "claude",
			"headers": map[string]any{
				"X-Kee": "1",
			},
		},
	}
	if _, errRegister := manager.Register(context.Background(), record); errRegister != nil {
		t.Fatalf("failed to register auth record: %v", errRegister)
	}

	h := NewHandlerWithoutConfigFilePath(&config.Config{AuthDir: t.TempDir()}, manager)

	body := `{"name":"noop.json","note":"hello","headers":{}}`
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodPatch, "/v0/management/auth-files/fields", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req
	h.PatchAuthFileFields(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	updated, ok := manager.GetByID("noop.json")
	if !ok || updated == nil {
		t.Fatalf("expected auth record to exist after patch")
	}
	if got := updated.Attributes["header:X-Kee"]; got != "1" {
		t.Fatalf("attrs header:X-Kee = %q, want %q", got, "1")
	}
	headersMeta, ok := updated.Metadata["headers"].(map[string]any)
	if !ok {
		t.Fatalf("expected metadata.headers to remain a map, got %T", updated.Metadata["headers"])
	}
	if got := headersMeta["X-Kee"]; got != "1" {
		t.Fatalf("metadata.headers.X-Kee = %#v, want %q", got, "1")
	}
}

// TestPatchAuthFileFields_AccountGroupIsPersistedAndListed 测试单个账号分组（account_group）的：
// - 在 Attributes 和 Metadata 中的正确持久化
// - 通过 buildAuthFileEntry 构建的列表项中同时返回 account_group、accountGroup 和 account_groups 三种格式
func TestPatchAuthFileFields_AccountGroupIsPersistedAndListed(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "")
	gin.SetMode(gin.TestMode)

	store := &memoryAuthStore{}
	manager := coreauth.NewManager(store, nil, nil)
	record := &coreauth.Auth{
		ID:         "grouped.json",
		FileName:   "grouped.json",
		Provider:   "codex",
		Attributes: map[string]string{"path": "/tmp/grouped.json"},
		Metadata:   map[string]any{"type": "codex"},
	}
	if _, errRegister := manager.Register(context.Background(), record); errRegister != nil {
		t.Fatalf("failed to register auth record: %v", errRegister)
	}

	h := NewHandlerWithoutConfigFilePath(&config.Config{AuthDir: t.TempDir()}, manager)

	body := `{"name":"grouped.json","account_group":"production"}`
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodPatch, "/v0/management/auth-files/fields", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req
	h.PatchAuthFileFields(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	updated, ok := manager.GetByID("grouped.json")
	if !ok || updated == nil {
		t.Fatalf("expected auth record to exist after patch")
	}
	if got := updated.Attributes["account_group"]; got != "production" {
		t.Fatalf("attrs account_group = %q, want %q", got, "production")
	}
	if got, _ := updated.Metadata["account_group"].(string); got != "production" {
		t.Fatalf("metadata.account_group = %q, want %q", got, "production")
	}

	entry := h.buildAuthFileEntry(updated)
	if got, _ := entry["account_group"].(string); got != "production" {
		t.Fatalf("entry.account_group = %q, want %q", got, "production")
	}
	if got, _ := entry["accountGroup"].(string); got != "production" {
		t.Fatalf("entry.accountGroup = %q, want %q", got, "production")
	}
	if got, _ := entry["account_groups"].([]string); !reflect.DeepEqual(got, []string{"production"}) {
		t.Fatalf("entry.account_groups = %#v, want %#v", got, []string{"production"})
	}
}

// TestPatchAuthFileFields_AccountGroupsSupportMultipleGroups 测试多账号分组（account_groups）功能：
// - 支持传入分组名称数组，自动去重和修剪空白（"  testing  " -> "testing"）
// - 重复的分组名称只保留第一个（"production" 出现两次 -> 只保留一个）
// - 第一个分组自动设置为默认 account_group
// - 在 Attributes 中以换行符分隔存储，在 Metadata 中以字符串数组存储
// - 清空操作：传入空数组 [] 会清除所有分组相关字段
func TestPatchAuthFileFields_AccountGroupsSupportMultipleGroups(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "")
	gin.SetMode(gin.TestMode)

	store := &memoryAuthStore{}
	manager := coreauth.NewManager(store, nil, nil)
	record := &coreauth.Auth{
		ID:         "multi-grouped.json",
		FileName:   "multi-grouped.json",
		Provider:   "codex",
		Attributes: map[string]string{"path": "/tmp/multi-grouped.json"},
		Metadata:   map[string]any{"type": "codex"},
	}
	if _, errRegister := manager.Register(context.Background(), record); errRegister != nil {
		t.Fatalf("failed to register auth record: %v", errRegister)
	}

	h := NewHandlerWithoutConfigFilePath(&config.Config{AuthDir: t.TempDir()}, manager)

	body := `{"name":"multi-grouped.json","account_groups":["production","fallback","production","  testing  "]}`
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodPatch, "/v0/management/auth-files/fields", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req
	h.PatchAuthFileFields(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	updated, ok := manager.GetByID("multi-grouped.json")
	if !ok || updated == nil {
		t.Fatalf("expected auth record to exist after patch")
	}

	wantGroups := []string{"production", "fallback", "testing"}
	if got := updated.Attributes["account_group"]; got != "production" {
		t.Fatalf("attrs account_group = %q, want %q", got, "production")
	}
	if got := strings.Split(updated.Attributes["account_groups"], "\n"); !reflect.DeepEqual(got, wantGroups) {
		t.Fatalf("attrs account_groups = %#v, want %#v", got, wantGroups)
	}
	if got, _ := updated.Metadata["account_groups"].([]string); !reflect.DeepEqual(got, wantGroups) {
		t.Fatalf("metadata.account_groups = %#v, want %#v", got, wantGroups)
	}
	if got, _ := updated.Metadata["account_group"].(string); got != "production" {
		t.Fatalf("metadata.account_group = %q, want %q", got, "production")
	}

	entry := h.buildAuthFileEntry(updated)
	if got, _ := entry["account_groups"].([]string); !reflect.DeepEqual(got, wantGroups) {
		t.Fatalf("entry.account_groups = %#v, want %#v", got, wantGroups)
	}
	if got, _ := entry["account_group"].(string); got != "production" {
		t.Fatalf("entry.account_group = %q, want %q", got, "production")
	}

	clearBody := `{"name":"multi-grouped.json","account_groups":[]}`
	clearRec := httptest.NewRecorder()
	clearCtx, _ := gin.CreateTestContext(clearRec)
	clearReq := httptest.NewRequest(http.MethodPatch, "/v0/management/auth-files/fields", strings.NewReader(clearBody))
	clearReq.Header.Set("Content-Type", "application/json")
	clearCtx.Request = clearReq
	h.PatchAuthFileFields(clearCtx)

	if clearRec.Code != http.StatusOK {
		t.Fatalf("expected clear status %d, got %d with body %s", http.StatusOK, clearRec.Code, clearRec.Body.String())
	}

	cleared, ok := manager.GetByID("multi-grouped.json")
	if !ok || cleared == nil {
		t.Fatalf("expected auth record to exist after clearing groups")
	}
	if _, ok := cleared.Metadata["account_groups"]; ok {
		t.Fatalf("expected metadata.account_groups to be cleared")
	}
	if _, ok := cleared.Attributes["account_groups"]; ok {
		t.Fatalf("expected attrs account_groups to be cleared")
	}
}
