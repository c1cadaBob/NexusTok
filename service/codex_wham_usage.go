// codex_wham_usage.go
// 本文件实现了查询 Codex WHAM（Web Hosted Application Model）使用量的功能。
// WHAM 是 OpenAI 内部的使用量统计接口，用于获取账户的 API 调用消耗情况。

package service

import (
	// 标准库
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// FetchCodexWhamUsage 调用 Codex WHAM 使用量查询接口
// 向 {baseURL}/backend-api/wham/usage 发送 GET 请求，获取指定账户的使用量信息
// 请求头包含 Authorization（Bearer Token）、chatgpt-account-id 和 originator
// 参数:
//   - ctx: 上下文，用于控制请求超时和取消
//   - client: HTTP 客户端（可配置代理）
//   - baseURL: Codex API 基础地址
//   - accessToken: OAuth 访问令牌
//   - accountID: Codex 账户 ID
// 返回值:
//   - statusCode: HTTP 响应状态码
//   - body: 响应体原始字节
//   - err: 请求失败时返回错误
func FetchCodexWhamUsage(
	ctx context.Context,
	client *http.Client,
	baseURL string,
	accessToken string,
	accountID string,
) (statusCode int, body []byte, err error) {
	if client == nil {
		return 0, nil, fmt.Errorf("nil http client")
	}
	// 去除 baseURL 末尾的斜杠，避免拼接时出现双斜杠
	bu := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if bu == "" {
		return 0, nil, fmt.Errorf("empty baseURL")
	}
	at := strings.TrimSpace(accessToken)
	aid := strings.TrimSpace(accountID)
	if at == "" {
		return 0, nil, fmt.Errorf("empty accessToken")
	}
	if aid == "" {
		return 0, nil, fmt.Errorf("empty accountID")
	}

	// 构建 WHAM 使用量查询请求
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, bu+"/backend-api/wham/usage", nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+at) // 设置 OAuth 访问令牌
	req.Header.Set("chatgpt-account-id", aid)     // 设置 Codex 账户 ID
	req.Header.Set("Accept", "application/json")
	if req.Header.Get("originator") == "" {
		req.Header.Set("originator", "codex_cli_rs") // 标识请求来源为 Codex CLI
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, body, nil
}
