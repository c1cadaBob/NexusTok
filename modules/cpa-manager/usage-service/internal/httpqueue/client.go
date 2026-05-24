// httpqueue - client.go
// HTTP 使用量队列客户端。
// 通过 HTTP GET 请求从上游 CPA 的 /v0/management/usage-queue 接口批量弹出使用量事件。
// 支持 JSON 对象和 JSON 字符串两种响应格式，自动过滤 null 值。
// 当上游返回 404/405/501 时视为不支持该接口，返回 ErrUnsupported 以便上层降级。
package httpqueue

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ErrUnsupported 表示上游不支持 HTTP 使用量队列接口。
// 当收到 404、405 或 501 响应时返回此错误，用于触发采集模式降级。
var ErrUnsupported = errors.New("http usage queue is unsupported")

// StatusError 表示 HTTP 使用量队列请求的状态码错误。
// 包含 HTTP 状态码、状态文本和响应体内容，便于上层判断错误类型。
type StatusError struct {
	StatusCode int    // HTTP 状态码
	Status     string // HTTP 状态文本（如 "401 Unauthorized"）
	Body       string // 响应体内容（截断至 1024 字节）
}

// Error 实现 error 接口，返回格式化的错误信息。
func (e *StatusError) Error() string {
	if e.Body == "" {
		return "usage queue request failed: " + e.Status
	}
	return "usage queue request failed: " + e.Status + ": " + e.Body
}

// Client 是 HTTP 使用量队列的客户端。
// 通过周期性的 GET 请求从上游拉取使用量事件。
type Client struct {
	BaseURL       string     // 上游 CPA 服务的基础 URL
	ManagementKey string     // 管理接口认证密钥（Bearer token）
	HTTPClient    *http.Client // HTTP 客户端实例
}

// New 创建一个新的 HTTP 使用量队列客户端。
// 默认请求超时为 30 秒。BaseURL 会自动去除尾部斜杠。
func New(baseURL string, managementKey string) *Client {
	return &Client{
		BaseURL:       strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		ManagementKey: strings.TrimSpace(managementKey),
		HTTPClient:    &http.Client{Timeout: 30 * time.Second},
	}
}

// Pop 从上游使用量队列中弹出指定数量的事件。
// 参数 count 指定请求的事件数量，小于等于 0 时默认为 1。
//
// 响应解析规则：
//   - JSON 数组中的 null 值和空白字符串被过滤
//   - JSON 对象（{...}）和 JSON 字符串（"..."）均被接受
//   - 字符串项经 TrimSpace 处理后非空才保留
//
// 错误处理：
//   - 404/405/501 状态码返回 ErrUnsupported
//   - 其他非 2xx 状态码返回 StatusError
func (c *Client) Pop(ctx context.Context, count int) ([]string, error) {
	if count <= 0 {
		count = 1
	}
	endpoint, err := c.endpoint(count)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if c.ManagementKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.ManagementKey)
	}

	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	// 校验 HTTP 响应状态码
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 1024))
		// 不支持的端点返回特殊错误，用于触发降级
		if isUnsupportedStatus(res.StatusCode) {
			return nil, fmt.Errorf("%w: %s", ErrUnsupported, res.Status)
		}
		return nil, &StatusError{
			StatusCode: res.StatusCode,
			Status:     res.Status,
			Body:       strings.TrimSpace(string(body)),
		}
	}

	// 解析 JSON 数组响应
	var entries []json.RawMessage
	decoder := json.NewDecoder(res.Body)
	if err := decoder.Decode(&entries); err != nil {
		return nil, err
	}

	// 过滤并规范化条目
	items := make([]string, 0, len(entries))
	for _, entry := range entries {
		trimmed := bytes.TrimSpace(entry)
		if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
			continue
		}
		// 字符串类型的条目：反序列化后检查是否非空
		if trimmed[0] == '"' {
			var text string
			if err := json.Unmarshal(trimmed, &text); err != nil {
				return nil, err
			}
			if strings.TrimSpace(text) != "" {
				items = append(items, text)
			}
			continue
		}
		// 对象类型的条目：直接使用原始 JSON
		if trimmed[0] != '{' {
			return nil, fmt.Errorf("unexpected usage queue item %s", string(trimmed))
		}
		items = append(items, string(trimmed))
	}
	return items, nil
}

// endpoint 构建使用量队列接口的完整请求 URL。
// URL 格式：{BaseURL}/v0/management/usage-queue?count={count}
// 自动处理协议前缀和尾部斜杠清理。
func (c *Client) endpoint(count int) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if base == "" {
		return "", errors.New("upstream URL is empty")
	}
	if !strings.Contains(base, "://") {
		base = "http://" + base
	}
	parsed, err := url.Parse(base + "/v0/management/usage-queue")
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	query.Set("count", strconv.Itoa(count))
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

// isUnsupportedStatus 判断 HTTP 状态码是否表示上游不支持使用量队列接口。
// 404（未找到）、405（方法不允许）、501（未实现）均视为不支持。
func isUnsupportedStatus(status int) bool {
	return status == http.StatusNotFound ||
		status == http.StatusMethodNotAllowed ||
		status == http.StatusNotImplemented
}
