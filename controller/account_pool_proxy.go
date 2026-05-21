package controller

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

var accountPoolProxyClient = &http.Client{}

var accountPoolHopByHopHeaders = map[string]struct{}{
	"Connection":          {},
	"Keep-Alive":          {},
	"Proxy-Authenticate":  {},
	"Proxy-Authorization": {},
	"Te":                  {},
	"Trailer":             {},
	"Transfer-Encoding":   {},
	"Upgrade":             {},
}

// AccountPoolManagementProxy 将 NexusTok 管理员态请求转发到内部 CLIProxyAPI 管理接口。
func AccountPoolManagementProxy(c *gin.Context) {
	targetBase := strings.TrimRight(strings.TrimSpace(os.Getenv("ACCOUNT_POOL_CLI_PROXY_URL")), "/")
	if targetBase == "" {
		targetBase = "http://127.0.0.1:8317"
	}

	targetURL, err := buildAccountPoolProxyURL(targetBase, c.Param("path"), c.Request.URL.RawQuery)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"success": false,
			"message": "账号池服务地址配置错误",
		})
		return
	}

	proxyReq, err := http.NewRequestWithContext(c.Request.Context(), c.Request.Method, targetURL.String(), c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"success": false,
			"message": "账号池服务请求创建失败",
		})
		return
	}
	proxyReq.ContentLength = c.Request.ContentLength
	copyAccountPoolHeaders(proxyReq.Header, c.Request.Header)
	removeAccountPoolHopByHopHeaders(proxyReq.Header)
	proxyReq.Header.Del("Authorization")
	proxyReq.Header.Del("Proxy-Authorization")
	proxyReq.Header.Set("Authorization", "Bearer "+accountPoolManagementKey())
	proxyReq.Host = targetURL.Host

	resp, err := accountPoolProxyClient.Do(proxyReq)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"success": false,
			"message": "账号池服务不可用",
		})
		return
	}
	defer resp.Body.Close()

	copyAccountPoolHeaders(c.Writer.Header(), resp.Header)
	removeAccountPoolHopByHopHeaders(c.Writer.Header())
	c.Status(resp.StatusCode)
	if _, err = io.Copy(c.Writer, resp.Body); err != nil {
		_ = c.Error(fmt.Errorf("转发账号池响应失败: %w", err))
	}
}

func buildAccountPoolProxyURL(base string, rawPath string, rawQuery string) (*url.URL, error) {
	parsedBase, err := url.Parse(base)
	if err != nil {
		return nil, err
	}
	if parsedBase.Scheme == "" || parsedBase.Host == "" {
		return nil, fmt.Errorf("invalid account pool proxy url: %s", base)
	}
	proxyPath := rawPath
	if proxyPath == "" {
		proxyPath = "/"
	}
	if !strings.HasPrefix(proxyPath, "/") {
		proxyPath = "/" + proxyPath
	}
	parsedBase.Path = strings.TrimRight(parsedBase.Path, "/") + "/v0/management" + proxyPath
	parsedBase.RawQuery = rawQuery
	return parsedBase, nil
}

func accountPoolManagementKey() string {
	key := strings.TrimSpace(os.Getenv("ACCOUNT_POOL_CLI_PROXY_MANAGEMENT_KEY"))
	if key == "" {
		key = strings.TrimSpace(os.Getenv("ACCOUNT_POOL_MANAGEMENT_KEY"))
	}
	if key == "" {
		key = "nexustok-account-pool-local"
	}
	return key
}

func copyAccountPoolHeaders(dst http.Header, src http.Header) {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func removeAccountPoolHopByHopHeaders(header http.Header) {
	for _, value := range header.Values("Connection") {
		for _, token := range strings.Split(value, ",") {
			if token = strings.TrimSpace(token); token != "" {
				header.Del(token)
			}
		}
	}
	for key := range accountPoolHopByHopHeaders {
		header.Del(key)
	}
}
