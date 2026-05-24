// helps - logging_helpers_test.go
// 日志辅助函数的单元测试。
// 测试 RecordAPIResponseMetadata 在请求日志禁用时仍能正确存储响应头，
// 并验证存储的是头的副本而非引用（后续修改不影响已存储的值）。
package helps

import (
	"context"
	"net/http"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/logging"
)

// TestRecordAPIResponseMetadataStoresHeadersWhenRequestLogDisabled 测试在请求日志禁用时，
// RecordAPIResponseMetadata 仍能正确存储响应头。
// 验证存储的是头的副本：后续对原始头的修改不影响已存储的值。
func TestRecordAPIResponseMetadataStoresHeadersWhenRequestLogDisabled(t *testing.T) {
	ctx := logging.WithResponseHeadersHolder(context.Background())
	headers := http.Header{}
	headers.Add("X-Upstream-Request-Id", "upstream-req-1")

	RecordAPIResponseMetadata(ctx, &config.Config{}, http.StatusOK, headers)
	headers.Set("X-Upstream-Request-Id", "mutated")

	got := logging.GetResponseHeaders(ctx)
	if got.Get("X-Upstream-Request-Id") != "upstream-req-1" {
		t.Fatalf("response header = %q, want %q", got.Get("X-Upstream-Request-Id"), "upstream-req-1")
	}
}
