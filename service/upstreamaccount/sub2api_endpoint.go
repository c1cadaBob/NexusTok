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

// resolveCaptureAPIBaseURL 将油猴脚本回传的 API 地址转成后端实际调用地址。
//
// 回调来源仍然使用目标页面 origin 校验；api_base_url 只决定后续请求发往哪个
// Sub2API API 端点。这里要求 API 地址与面板同 host 或同一可注册主域名，避免
// 恶意页面把 access token 静默转存到完全无关的第三方站点。
func resolveCaptureAPIBaseURL(record CaptureSessionRecord, req CaptureSessionCompleteRequest) (string, error) {
	panelBaseURL := firstNonEmpty(record.BaseURL, record.Origin)
	candidate := firstNonEmpty(req.APIBaseURL, req.BaseURL, panelBaseURL)
	normalized, err := normalizeBaseURL(normalizeSub2APIBaseURL(candidate))
	if err != nil {
		return "", err
	}
	if err := validateRelatedAPIBaseURL(panelBaseURL, normalized); err != nil {
		return "", err
	}
	return normalized, nil
}

// discoverSub2APIAPIBaseURLFromPanel 尝试从 Sub2API 前端页面配置中发现实际 API 端点。
//
// aiapipay.com 这类部署会把管理面板和 API 服务拆成两个域名。管理员常复制面板地址，
// 但真正的模型 relay 和账号同步必须请求 window.__APP_CONFIG__.api_base_url 指向的
// API 域名。发现失败时返回 ok=false，调用方继续使用原地址以保持兼容。
func (c *Sub2APIClient) discoverSub2APIAPIBaseURLFromPanel(ctx context.Context, panelBaseURL string) (string, bool) {
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
