// httpqueue - client_test.go
// HTTP 使用量队列客户端的单元测试。
// 测试覆盖以下场景：
//   - 从 usage-queue 接口正确读取使用量事件（支持 JSON 对象和字符串两种格式）
//   - 404 响应正确返回 ErrUnsupported 错误
//   - 认证错误（401）与不支持错误的区分
package httpqueue

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestClientPopReadsUsageQueue 验证 Pop 方法能正确解析上游响应。
// 响应包含三种类型：JSON 对象、JSON 字符串和 null 值。
// 验证 null 值被过滤，对象和字符串被正确保留。
func TestClientPopReadsUsageQueue(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v0/management/usage-queue" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.URL.Query().Get("count") != "25" {
			t.Fatalf("count = %q", r.URL.Query().Get("count"))
		}
		if r.Header.Get("Authorization") != "Bearer management-key" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"timestamp":"2026-05-06T00:00:00Z","model":"gpt-test"},
			"{\"timestamp\":\"2026-05-06T00:00:01Z\",\"model\":\"gpt-string\"}",
			null
		]`))
	}))
	t.Cleanup(upstream.Close)

	items, err := New(upstream.URL, "management-key").Pop(context.Background(), 25)
	if err != nil {
		t.Fatalf("pop: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items len = %d, want 2: %#v", len(items), items)
	}
	if !strings.Contains(items[0], `"model":"gpt-test"`) {
		t.Fatalf("object item = %s", items[0])
	}
	if !strings.Contains(items[1], `"model":"gpt-string"`) {
		t.Fatalf("string item = %s", items[1])
	}
}

// TestClientPopClassifiesUnsupportedEndpoint 验证 404 响应返回 ErrUnsupported 错误。
// 用于触发采集模式从 HTTP 降级到 RESP。
func TestClientPopClassifiesUnsupportedEndpoint(t *testing.T) {
	upstream := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(upstream.Close)

	_, err := New(upstream.URL, "management-key").Pop(context.Background(), 10)
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
}

// TestClientPopKeepsAuthErrorsDistinct 验证认证错误（401）不会被误分类为 ErrUnsupported。
// 确保 StatusError 的 StatusCode 正确记录，且与 ErrUnsupported 保持独立。
func TestClientPopKeepsAuthErrorsDistinct(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad key", http.StatusUnauthorized)
	}))
	t.Cleanup(upstream.Close)

	_, err := New(upstream.URL, "management-key").Pop(context.Background(), 10)
	var statusErr *StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("err = %T %v, want StatusError", err, err)
	}
	if statusErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d", statusErr.StatusCode)
	}
	if errors.Is(err, ErrUnsupported) {
		t.Fatalf("auth error must not be classified as unsupported")
	}
}
