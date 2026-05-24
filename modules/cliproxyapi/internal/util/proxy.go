// Package util 提供 CLI Proxy API 服务器的工具函数。
// 包括代理配置、HTTP 客户端设置、日志级别管理等应用中常用的辅助函数。
package util

import (
	"net/http"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/proxyutil"
	log "github.com/sirupsen/logrus"
)

// SetProxy 根据配置为提供的 HTTP 客户端设置代理。
// 支持 SOCKS5、HTTP 和 HTTPS 代理。修改客户端的传输层以通过配置的代理服务器路由请求。
//
// 参数：
//   - cfg: 包含代理 URL 的 SDK 配置
//   - httpClient: 要配置代理的 HTTP 客户端
//
// 返回：
//   - *http.Client: 配置了代理的 HTTP 客户端
func SetProxy(cfg *config.SDKConfig, httpClient *http.Client) *http.Client {
	if cfg == nil || httpClient == nil {
		return httpClient
	}

	transport, _, errBuild := proxyutil.BuildHTTPTransport(cfg.ProxyURL)
	if errBuild != nil {
		log.Errorf("%v", errBuild)
	}
	if transport != nil {
		httpClient.Transport = transport
	}
	return httpClient
}
