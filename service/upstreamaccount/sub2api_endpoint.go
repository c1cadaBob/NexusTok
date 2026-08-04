package upstreamaccount

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/publicsuffix"
)

var sub2APIAppConfigAPIBaseURLPattern = regexp.MustCompile(`(?i)["']api_base_url["']\s*:\s*["']([^"']+)["']`)

// resolveCaptureSub2APIBaseURLs 将油猴脚本回传的地址拆分成管理地址和模型转发地址。
//
// `base_url` 和 `management_base_url` 表示登录、分组、密钥等后台管理接口所在的面板
// 地址；`api_base_url` / `relay_base_url` 表示最终模型请求使用的 OpenAI 兼容地址。
// 两类地址都要求与面板同 host 或同一可注册主域名，避免脚本把登录态转给无关站点。
func resolveCaptureSub2APIBaseURLs(record CaptureSessionRecord, req CaptureSessionCompleteRequest) (string, string, error) {
	panelBaseURL := firstNonEmpty(record.ManagementBaseURL, record.BaseURL, record.Origin)
	managementCandidate := firstNonEmpty(req.ManagementBaseURL, record.ManagementBaseURL, record.BaseURL, req.BaseURL, panelBaseURL)
	managementBaseURL, err := normalizeBaseURL(normalizeSub2APIBaseURL(managementCandidate))
	if err != nil {
		return "", "", err
	}
	if err := validateRelatedAPIBaseURL(panelBaseURL, managementBaseURL); err != nil {
		return "", "", err
	}

	relayCandidate := firstNonEmpty(req.RelayBaseURL, req.APIBaseURL)
	if relayCandidate == "" {
		return managementBaseURL, managementBaseURL, nil
	}
	relayBaseURL, err := normalizeBaseURL(normalizeSub2APIBaseURL(relayCandidate))
	if err != nil {
		return "", "", err
	}
	if err := validateRelatedAPIBaseURL(managementBaseURL, relayBaseURL); err != nil {
		return "", "", err
	}
	return managementBaseURL, relayBaseURL, nil
}

// discoverSub2APIRelayBaseURLFromPanel 尝试从 Sub2API 前端页面配置中发现模型转发端点。
//
// aiapipay.com 这类部署会把管理面板和 API 服务拆成两个域名。管理员常复制面板地址，
// 但真正的模型 relay 应请求 window.__APP_CONFIG__.api_base_url 指向的 API 域名。
// 注意：该地址不是账号同步管理接口；密钥、分组、余额仍走面板域名下的 /api/v1。
func (c *Sub2APIClient) discoverSub2APIRelayBaseURLFromPanel(ctx context.Context, panelBaseURL string) (string, bool) {
	panelBaseURL = strings.TrimSpace(normalizeSub2APIBaseURL(panelBaseURL))
	if panelBaseURL == "" {
		return "", false
	}
	normalizedPanel, err := normalizeBaseURL(panelBaseURL)
	if err != nil {
		return "", false
	}
	if isLikelyAPISubdomain(normalizedPanel) {
		return "", false
	}
	discoveryCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(discoveryCtx, http.MethodGet, normalizedPanel, nil)
	if err != nil {
		return "", false
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	client := c.httpClient
	if client == nil {
		client = (&httpClient{}).defaultClient()
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", false
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return "", false
	}
	candidate := extractSub2APIAppConfigAPIBaseURL(string(data))
	if candidate == "" {
		return "", false
	}
	normalizedCandidate, err := normalizeBaseURL(normalizeSub2APIBaseURL(candidate))
	if err != nil {
		return "", false
	}
	if err := validateRelatedAPIBaseURL(normalizedPanel, normalizedCandidate); err != nil {
		return "", false
	}
	return normalizedCandidate, true
}

// recoverSub2APIManagementBaseURLFromRelay 兼容上一版误把 api.* 模型端点保存成管理地址的历史数据。
//
// 该函数只对 `api.` 子域名做保守推断，把 `https://api.example.com` 还原为
// `https://example.com`，再用无需凭据的 `/api/v1/auth/me` 探测路由是否像 Sub2API 管理 API。
// 返回 200/401/403 均说明管理路由存在；404 或 HTML 网关错误则保持原地址并让上层给出明确提示。
func (c *Sub2APIClient) recoverSub2APIManagementBaseURLFromRelay(ctx context.Context, relayBaseURL string) (string, bool) {
	normalizedRelay, err := normalizeBaseURL(normalizeSub2APIBaseURL(relayBaseURL))
	if err != nil || !isLikelyAPISubdomain(normalizedRelay) {
		return "", false
	}
	parsed, err := url.Parse(normalizedRelay)
	if err != nil {
		return "", false
	}
	host := parsed.Hostname()
	if !strings.HasPrefix(strings.ToLower(host), "api.") {
		return "", false
	}
	parsed.Host = strings.TrimPrefix(parsed.Host, "api.")
	parsed.Path = ""
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	candidate := strings.TrimRight(parsed.String(), "/")
	if candidate == "" {
		return "", false
	}
	if c.probeSub2APIManagementEndpoint(ctx, candidate) {
		return candidate, true
	}
	return "", false
}

func (c *Sub2APIClient) probeSub2APIManagementEndpoint(ctx context.Context, candidate string) bool {
	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, strings.TrimRight(candidate, "/")+"/api/v1/auth/me", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Accept", "application/json")
	client := c.httpClient
	if client == nil {
		client = (&httpClient{}).defaultClient()
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK, http.StatusUnauthorized, http.StatusForbidden:
		return true
	default:
		return false
	}
}

func (c *httpClient) defaultClient() *http.Client {
	return &http.Client{Timeout: defaultHTTPTimeout}
}

func extractSub2APIAppConfigAPIBaseURL(html string) string {
	match := sub2APIAppConfigAPIBaseURLPattern.FindStringSubmatch(html)
	if len(match) < 2 {
		return ""
	}
	value := strings.TrimSpace(match[1])
	value = strings.ReplaceAll(value, `\/`, `/`)
	if decoded, err := url.QueryUnescape(value); err == nil && strings.HasPrefix(decoded, "http") {
		value = decoded
	}
	return value
}

func isLikelyAPISubdomain(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	return strings.HasPrefix(strings.ToLower(parsed.Hostname()), "api.")
}

func validateRelatedAPIBaseURL(panelRaw string, apiRaw string) error {
	panelURL, err := normalizeBaseURL(panelRaw)
	if err != nil {
		return err
	}
	apiURL, err := normalizeBaseURL(apiRaw)
	if err != nil {
		return err
	}
	panel, _ := url.Parse(panelURL)
	api, _ := url.Parse(apiURL)
	panelHost := normalizeHostname(panel.Hostname())
	apiHost := normalizeHostname(api.Hostname())
	if panelHost == "" || apiHost == "" {
		return fmt.Errorf("目标站 API 地址无效")
	}
	if panelHost == apiHost {
		return nil
	}
	if isLocalOrIPHost(panelHost) || isLocalOrIPHost(apiHost) {
		return fmt.Errorf("检测到跨站 API 端点，请管理员确认：%s", apiURL)
	}
	panelDomain, panelOK := registrableDomain(panelHost)
	apiDomain, apiOK := registrableDomain(apiHost)
	if panelOK && apiOK && panelDomain == apiDomain {
		return nil
	}
	return fmt.Errorf("检测到跨站 API 端点，请管理员确认：%s", apiURL)
}

func relatedSub2APIBaseURL(left string, right string) bool {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" || right == "" {
		return false
	}
	leftNormalized, err := normalizeBaseURL(normalizeSub2APIBaseURL(left))
	if err != nil {
		return false
	}
	rightNormalized, err := normalizeBaseURL(normalizeSub2APIBaseURL(right))
	if err != nil {
		return false
	}
	if leftNormalized == rightNormalized {
		return true
	}
	return validateRelatedAPIBaseURL(leftNormalized, rightNormalized) == nil
}

func normalizeHostname(host string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
}

func isLocalOrIPHost(host string) bool {
	host = normalizeHostname(host)
	return host == "localhost" || net.ParseIP(host) != nil
}

func registrableDomain(host string) (string, bool) {
	host = normalizeHostname(host)
	if host == "" || isLocalOrIPHost(host) {
		return "", false
	}
	domain, err := publicsuffix.EffectiveTLDPlusOne(host)
	if err != nil {
		return "", false
	}
	return strings.ToLower(domain), true
}
