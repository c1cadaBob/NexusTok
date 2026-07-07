// codex_wham_usage.go
// 本文件实现了查询 Codex WHAM（Web Hosted Application Model）使用量的功能。
// WHAM 是 OpenAI 内部的使用量统计接口，用于获取账户的 API 调用消耗情况。

package service

import (
	// 标准库
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/c1cada/NexusTok/common"

	"github.com/google/uuid"
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
//
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

	// 构建 WHAM 使用量查询请求
	req, err := newCodexWhamRequest(ctx, http.MethodGet, baseURL, "/backend-api/wham/usage", nil, accessToken, accountID)
	if err != nil {
		return 0, nil, err
	}
	return doCodexWhamRequest(client, req)
}

// FetchCodexWhamRateLimitResetCredits 查询 Codex 账户可用的用量重置次数。
//
// 该接口与 usage 查询共用 WHAM 鉴权头，返回内容由上游直接决定，控制器只负责
// 透传成功状态、状态码和响应体，避免在本地绑定不稳定的上游响应结构。
func FetchCodexWhamRateLimitResetCredits(
	ctx context.Context,
	client *http.Client,
	baseURL string,
	accessToken string,
	accountID string,
) (statusCode int, body []byte, err error) {
	if client == nil {
		return 0, nil, fmt.Errorf("nil http client")
	}
	req, err := newCodexWhamRequest(ctx, http.MethodGet, baseURL, "/backend-api/wham/rate-limit-reset-credits", nil, accessToken, accountID)
	if err != nil {
		return 0, nil, err
	}
	return doCodexWhamRequest(client, req)
}

// ConsumeCodexWhamRateLimitResetCredit 消耗一次 Codex 用量重置额度。
//
// 上游要求每次消费携带唯一 redeem_request_id，用于幂等和审计；这里在服务端生成，
// 不接受客户端传入，避免同一个重置请求被不同用户或脚本复用。
func ConsumeCodexWhamRateLimitResetCredit(
	ctx context.Context,
	client *http.Client,
	baseURL string,
	accessToken string,
	accountID string,
) (statusCode int, body []byte, err error) {
	if client == nil {
		return 0, nil, fmt.Errorf("nil http client")
	}
	requestBody, err := common.Marshal(map[string]string{
		"redeem_request_id": uuid.NewString(),
	})
	if err != nil {
		return 0, nil, err
	}

	req, err := newCodexWhamRequest(
		ctx,
		http.MethodPost,
		baseURL,
		"/backend-api/wham/rate-limit-reset-credits/consume",
		bytes.NewReader(requestBody),
		accessToken,
		accountID,
	)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	return doCodexWhamRequest(client, req)
}

func doCodexWhamRequest(client *http.Client, req *http.Request) (statusCode int, body []byte, err error) {
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

func newCodexWhamRequest(
	ctx context.Context,
	method string,
	baseURL string,
	path string,
	body io.Reader,
	accessToken string,
	accountID string,
) (*http.Request, error) {
	bu := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if bu == "" {
		return nil, fmt.Errorf("empty baseURL")
	}
	at := strings.TrimSpace(accessToken)
	aid := strings.TrimSpace(accountID)
	if at == "" {
		return nil, fmt.Errorf("empty accessToken")
	}
	if aid == "" {
		return nil, fmt.Errorf("empty accountID")
	}

	req, err := http.NewRequestWithContext(ctx, method, bu+path, body)
	if err != nil {
		return nil, err
	}
	setCodexWhamRequestHeaders(req, at, aid)
	return req, nil
}

func setCodexWhamRequestHeaders(req *http.Request, accessToken string, accountID string) {
	req.Header.Set("Authorization", "Bearer "+accessToken) // 设置 OAuth 访问令牌
	req.Header.Set("chatgpt-account-id", accountID)        // 设置 Codex 账户 ID
	req.Header.Set("Accept", "application/json")
	if req.Header.Get("originator") == "" {
		req.Header.Set("originator", "codex_cli_rs") // 标识请求来源为 Codex CLI
	}
}
