package controller

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/c1cada/NexusTok/service"

	"github.com/gin-gonic/gin"
)

// accountPoolUsageProxyClient 是请求监控代理使用的共享 HTTP client。
//
// 当前代理只转发到 Docker 内部网络或本机回环地址上的 Usage Service，
// 因此复用默认 transport 即可；请求生命周期仍然受 Gin request context 控制，
// 客户端断开时上游请求也会随 context 取消。
var accountPoolUsageProxyClient = &http.Client{}

// disableAccountPoolUsageProxyCache 为请求监控代理写入强制不缓存响应头。
//
// NexusTok 的 Web 路由会先经过全局 Cache 中间件。该中间件默认给非首页路径设置
// max-age=604800，适合静态资源，但不适合 /usage-service/info 这类实时探测接口。
// 如果 Usage Service 在重启窗口内短暂不可用，502 JSON 一旦被浏览器缓存，CPAMC
// 会持续读到旧的“请求监控服务不可用”，即便 sidecar 已经恢复也无法自动消除提示。
// 因此成功响应和错误响应都必须覆盖为 no-store，并删除静态资源缓存版本头。
func disableAccountPoolUsageProxyCache(c *gin.Context) {
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate, private, max-age=0")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")
	c.Writer.Header().Del("Cache-Version")
}

// AccountPoolUsageServiceProxy 将 NexusTok 管理员态请求转发到 CPA-Manager Usage Service。
//
// CPAMC embedded 模式下浏览器只知道 NexusTok 同源地址，且不会保存 Usage Service
// 的 management key。本代理负责三件事：
//  1. 复用 NexusTok 的管理员 session 认证，避免 CPAMC 再做一次独立登录。
//  2. 将 /usage-service、/status、/v0/management/usage 等同源路径转发到内部 Usage Service。
//  3. 在服务端注入 Authorization: Bearer <management key>，避免内部密钥暴露到浏览器。
func AccountPoolUsageServiceProxy(c *gin.Context) {
	disableAccountPoolUsageProxyCache(c)

	targetBase := service.AccountPoolUsageServiceURL()
	targetURL, err := buildAccountPoolUsageProxyURL(targetBase, c.Request.URL.Path, c.Request.URL.RawQuery)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"success": false,
			"message": "请求监控服务地址配置错误",
		})
		return
	}

	proxyReq, err := http.NewRequestWithContext(c.Request.Context(), c.Request.Method, targetURL.String(), c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"success": false,
			"message": "请求监控服务请求创建失败",
		})
		return
	}

	proxyReq.ContentLength = c.Request.ContentLength
	copyAccountPoolHeaders(proxyReq.Header, c.Request.Header)
	removeAccountPoolHopByHopHeaders(proxyReq.Header)
	proxyReq.Header.Del("Authorization")
	proxyReq.Header.Del("Proxy-Authorization")
	proxyReq.Header.Set("Authorization", "Bearer "+service.AccountPoolUsageServiceManagementKey())
	proxyReq.Host = targetURL.Host

	resp, err := accountPoolUsageProxyClient.Do(proxyReq)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"success": false,
			"message": "请求监控服务不可用",
		})
		return
	}
	defer resp.Body.Close()

	copyAccountPoolHeaders(c.Writer.Header(), resp.Header)
	removeAccountPoolHopByHopHeaders(c.Writer.Header())
	disableAccountPoolUsageProxyCache(c)

	c.Status(resp.StatusCode)
	if _, err = io.Copy(c.Writer, resp.Body); err != nil {
		_ = c.Error(fmt.Errorf("转发请求监控响应失败: %w", err))
	}
}

// buildAccountPoolUsageProxyURL 构建发往 Usage Service 的完整代理 URL。
//
// 该代理保持浏览器请求路径不变，例如：
//   - /usage-service/info -> {base}/usage-service/info
//   - /v0/management/usage -> {base}/v0/management/usage
//   - /status -> {base}/status
//
// 这样 CPAMC 原有 Usage Service API 封装无需在 embedded 模式下改写端点路径，
// 只需要把 serviceBase 指向 NexusTok 同源 origin 即可。
func buildAccountPoolUsageProxyURL(base string, rawPath string, rawQuery string) (*url.URL, error) {
	parsedBase, err := url.Parse(base)
	if err != nil {
		return nil, err
	}
	if parsedBase.Scheme == "" || parsedBase.Host == "" {
		return nil, fmt.Errorf("invalid account pool usage service url: %s", base)
	}
	proxyPath := rawPath
	if proxyPath == "" {
		proxyPath = "/"
	}
	if !strings.HasPrefix(proxyPath, "/") {
		proxyPath = "/" + proxyPath
	}
	parsedBase.Path = strings.TrimRight(parsedBase.Path, "/") + proxyPath
	parsedBase.RawQuery = rawQuery
	return parsedBase, nil
}
