package service

import (
	"os"
	"strings"
)

const (
	defaultAccountPoolUsageServiceURL = "http://127.0.0.1:18317" // CPA-Manager Usage Service 的本机默认地址
)

// AccountPoolUsageServiceURL 返回 NexusTok 后端访问 CPA-Manager Usage Service 的内部地址。
//
// 这个地址只供 NexusTok 服务端使用，浏览器不会直接读取该环境变量。这样可以把
// Usage Service 暴露在 Docker 内部网络中，由 NexusTok 统一做管理员会话校验、
// 请求转发和管理密钥注入，避免账号池请求监控模块重新引入独立登录流程。
func AccountPoolUsageServiceURL() string {
	value := strings.TrimRight(strings.TrimSpace(os.Getenv("ACCOUNT_POOL_USAGE_SERVICE_URL")), "/")
	if value == "" {
		value = defaultAccountPoolUsageServiceURL
	}
	return value
}

// AccountPoolUsageServiceManagementKey 返回访问 Usage Service 管理接口所需的内部密钥。
//
// 优先级说明：
//  1. ACCOUNT_POOL_USAGE_SERVICE_MANAGEMENT_KEY：允许 Usage Service 与 CLIProxyAPI 使用不同密钥。
//  2. ACCOUNT_POOL_MANAGEMENT_KEY：本地与 Docker 编排中的统一账号池管理密钥。
//  3. AccountPoolCLIProxyManagementKey：保持与既有 CLIProxyAPI sidecar 默认值一致。
//
// 前端 embedded 模式不会保存或发送这个密钥；所有需要 Authorization 的请求都由
// NexusTok 后端代理在转发时注入，确保内部凭证不会成为浏览器状态的一部分。
func AccountPoolUsageServiceManagementKey() string {
	key := strings.TrimSpace(os.Getenv("ACCOUNT_POOL_USAGE_SERVICE_MANAGEMENT_KEY"))
	if key == "" {
		key = strings.TrimSpace(os.Getenv("ACCOUNT_POOL_MANAGEMENT_KEY"))
	}
	if key == "" {
		key = AccountPoolCLIProxyManagementKey()
	}
	return key
}
