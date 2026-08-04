package upstreamaccount

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/common"
)

const (
	defaultHTTPTimeout = 20 * time.Second
	maxResponseBytes   = 8 << 20
)

// httpClient 包装目标平台 HTTP 调用。
type httpClient struct {
	baseURL string
	client  *http.Client
}

type upstreamHTTPError struct {
	Method     string
	Path       string
	StatusCode int
	Body       string
}

func (e *upstreamHTTPError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("上游平台请求失败：status=%d, body=%s", e.StatusCode, e.Body)
}

// newHTTPClient 创建带 cookie jar 的 HTTP 客户端。
func newHTTPClient(baseURL string, client *http.Client) (*httpClient, error) {
	normalized, err := normalizeBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	if client == nil {
		jar, _ := cookiejar.New(nil)
		client = &http.Client{
			Timeout: defaultHTTPTimeout,
			Jar:     jar,
		}
	}
	return &httpClient{baseURL: normalized, client: client}, nil
}

// normalizeBaseURL 校验并规范化目标平台地址。
func normalizeBaseURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("上游平台地址不能为空")
	}
	u, err := url.Parse(trimmed)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("上游平台地址无效：%s", raw)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("上游平台地址仅支持 http/https：%s", raw)
	}
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawQuery = ""
	u.Fragment = ""
	return strings.TrimRight(u.String(), "/"), nil
}

// getJSON 发送 GET 请求并解码 JSON 响应。
func (c *httpClient) getJSON(ctx context.Context, path string, headers http.Header, out any) error {
	return c.doJSON(ctx, http.MethodGet, path, headers, nil, out)
}

// postJSON 发送 JSON POST 请求并解码 JSON 响应。
func (c *httpClient) postJSON(ctx context.Context, path string, headers http.Header, body any, out any) error {
	return c.doJSON(ctx, http.MethodPost, path, headers, body, out)
}

func (c *httpClient) doJSON(ctx context.Context, method string, path string, headers http.Header, body any, out any) error {
	var reader io.Reader
	if body != nil {
		payload, err := common.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.buildURL(path), reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, values := range headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, maxResponseBytes)
	data, err := io.ReadAll(limited)
	if err != nil {
		return err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return &upstreamHTTPError{
			Method:     method,
			Path:       path,
			StatusCode: resp.StatusCode,
			Body:       common.MaskSensitiveInfo(string(data)),
		}
	}
	if out == nil {
		return nil
	}
	if err := common.Unmarshal(data, out); err != nil {
		return fmt.Errorf("解析上游平台响应失败：%w", err)
	}
	return nil
}

func (c *httpClient) buildURL(path string) string {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return c.baseURL + path
}

// newAPIEnvelope 是 new-api 通用响应结构。
type newAPIEnvelope[T any] struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

// sub2APIEnvelope 是 sub2api 通用响应结构。
type sub2APIEnvelope[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

func unwrapNewAPI[T any](envelope newAPIEnvelope[T]) (T, error) {
	if !envelope.Success {
		var zero T
		if strings.TrimSpace(envelope.Message) == "" {
			envelope.Message = "new-api 返回失败"
		}
		return zero, errors.New(envelope.Message)
	}
	return envelope.Data, nil
}

func unwrapSub2API[T any](envelope sub2APIEnvelope[T]) (T, error) {
	if envelope.Code != 0 {
		var zero T
		if strings.TrimSpace(envelope.Message) == "" {
			envelope.Message = "sub2api 返回失败"
		}
		return zero, errors.New(envelope.Message)
	}
	return envelope.Data, nil
}

func maskKey(key string) string {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return ""
	}
	if len(trimmed) <= 10 {
		return "******"
	}
	return trimmed[:6] + "..." + trimmed[len(trimmed)-4:]
}

func stringValue(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	default:
		return ""
	}
}

func floatPtr(value float64) *float64 {
	return &value
}

func quotaToUSD(raw float64, quotaPerUnit float64) *float64 {
	if quotaPerUnit <= 0 {
		return nil
	}
	return floatPtr(raw / quotaPerUnit)
}

func splitModels(models string) []string {
	parts := strings.Split(models, ",")
	result := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		model := strings.TrimSpace(part)
		if model == "" {
			continue
		}
		if _, exists := seen[model]; exists {
			continue
		}
		seen[model] = struct{}{}
		result = append(result, model)
	}
	return result
}
