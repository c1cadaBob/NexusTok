// Package controller - account_pool_proxy.go
// 该文件实现了账号池管理接口的反向代理
//
// 将 NexusTok 管理员的请求转发到内部 CLIProxyAPI 管理接口
// 用于管理外部账号池服务（如独立部署的账号池 Sidecar）
//
// 代理特性：
// - 自动添加管理密钥认证
// - 移除逐跳（Hop-by-Hop）头
// - 保留原始请求方法和查询参数
// - 超时和错误处理
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

// accountPoolProxyClient 用于转发请求的 HTTP 客户端
var accountPoolProxyClient = &http.Client{}

// accountPoolHopByHopHeaders 需要移除的逐跳头
//
// 逐跳头（Hop-by-Hop Headers）是 HTTP/1.1 中定义的只对单次传输有意义的头
// 在代理转发时需要移除，参见 RFC 2616 Section 13.5.1
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

// AccountPoolManagementProxy 将 NexusTok 管理员态请求转发到内部 CLIProxyAPI 管理接口
//
// 代理流程：
// 1. 获取目标服务地址（service.AccountPoolCLIProxyURL）
// 2. 构建代理请求 URL
// 3. 复制原始请求头并添加管理密钥
// 4. 移除逐跳头
// 5. 发送代理请求
// 6. 复制响应头和响应体
//
// 参数：
//   - c: Gin 上下文
func AccountPoolManagementProxy(c *gin.Context) {
	// 获取目标服务地址
	targetBase := service.AccountPoolCLIProxyURL()

	// 构建代理请求 URL
	targetURL, err := buildAccountPoolProxyURL(targetBase, c.Param("path"), c.Request.URL.RawQuery)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"success": false,
			"message": "账号池服务地址配置错误",
		})
		return
	}

	// 创建代理请求
	proxyReq, err := http.NewRequestWithContext(c.Request.Context(), c.Request.Method, targetURL.String(), c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"success": false,
			"message": "账号池服务请求创建失败",
		})
		return
	}

	// 复制请求头
	proxyReq.ContentLength = c.Request.ContentLength
	copyAccountPoolHeaders(proxyReq.Header, c.Request.Header)
	removeAccountPoolHopByHopHeaders(proxyReq.Header)

	// 替换认证头为管理密钥
	proxyReq.Header.Del("Authorization")
	proxyReq.Header.Del("Proxy-Authorization")
	proxyReq.Header.Set("Authorization", "Bearer "+service.AccountPoolCLIProxyManagementKey())
	proxyReq.Host = targetURL.Host

	// 发送代理请求
	resp, err := accountPoolProxyClient.Do(proxyReq)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"success": false,
			"message": "账号池服务不可用",
		})
		return
	}
	defer resp.Body.Close()

	// 复制响应头
	copyAccountPoolHeaders(c.Writer.Header(), resp.Header)
	removeAccountPoolHopByHopHeaders(c.Writer.Header())

	// 设置响应状态码并复制响应体
	c.Status(resp.StatusCode)
	if _, err = io.Copy(c.Writer, resp.Body); err != nil {
		_ = c.Error(fmt.Errorf("转发账号池响应失败: %w", err))
	}
}

// buildAccountPoolProxyURL 构建代理请求的完整 URL
//
// URL 格式：{base}/v0/management{path}?{query}
//
// 参数：
//   - base: 目标服务基础 URL
//   - rawPath: 原始请求路径
//   - rawQuery: 原始查询参数
//
// 返回值：
//   - *url.URL: 构建的 URL
//   - error: URL 解析错误
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
	// 拼接管理 API 路径前缀
	parsedBase.Path = strings.TrimRight(parsedBase.Path, "/") + "/v0/management" + proxyPath
	parsedBase.RawQuery = rawQuery
	return parsedBase, nil
}

// copyAccountPoolHeaders 复制 HTTP 头
//
// 参数：
//   - dst: 目标头
//   - src: 源头
func copyAccountPoolHeaders(dst http.Header, src http.Header) {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

// removeAccountPoolHopByHopHeaders 移除逐跳头
//
// 逐跳头在代理转发时需要移除，包括：
// - Connection 头中列出的头
// - 标准逐跳头列表中的头
//
// 参数：
//   - header: 需要处理的 HTTP 头
func removeAccountPoolHopByHopHeaders(header http.Header) {
	// 移除 Connection 头中列出的头
	for _, value := range header.Values("Connection") {
		for _, token := range strings.Split(value, ",") {
			if token = strings.TrimSpace(token); token != "" {
				header.Del(token)
			}
		}
	}
	// 移除标准逐跳头
	for key := range accountPoolHopByHopHeaders {
		header.Del(key)
	}
}
