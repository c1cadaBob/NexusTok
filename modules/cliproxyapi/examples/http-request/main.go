// Package main - http-request 示例
// 演示如何使用 coreauth.Manager.HttpRequest/NewHttpRequest
// 执行带有提供者凭证注入的任意 HTTP 请求。
//
// 本示例注册了一个最小化的自定义执行器，该执行器从 auth.Attributes["api_key"]
// 注入 Authorization 头，然后对 httpbin.org 执行两个请求以展示注入的头部信息。
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	clipexec "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	log "github.com/sirupsen/logrus"
)

// providerKey 是 echo 提供者的标识符。
const providerKey = "echo"

// EchoExecutor 是用于演示的最小化提供者实现。
type EchoExecutor struct{}

// Identifier 返回此执行器的唯一标识符。
func (EchoExecutor) Identifier() string { return providerKey }

// PrepareRequest 向 HTTP 请求注入认证头。
// 从认证信息的 Attributes 中提取 api_key，并设置为 Bearer token。
func (EchoExecutor) PrepareRequest(req *http.Request, auth *coreauth.Auth) error {
	if req == nil || auth == nil {
		return nil
	}
	if auth.Attributes != nil {
		if apiKey := strings.TrimSpace(auth.Attributes["api_key"]); apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
	}
	return nil
}

// HttpRequest 执行原始 HTTP 请求，注入凭证后发送。
func (EchoExecutor) HttpRequest(ctx context.Context, auth *coreauth.Auth, req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, fmt.Errorf("echo executor: request is nil")
	}
	if ctx == nil {
		ctx = req.Context()
	}
	httpReq := req.WithContext(ctx)
	if errPrep := (EchoExecutor{}).PrepareRequest(httpReq, auth); errPrep != nil {
		return nil, errPrep
	}
	return http.DefaultClient.Do(httpReq)
}

// Execute 执行同步请求（本示例中未实现）。
func (EchoExecutor) Execute(context.Context, *coreauth.Auth, clipexec.Request, clipexec.Options) (clipexec.Response, error) {
	return clipexec.Response{}, errors.New("echo executor: Execute not implemented")
}

// ExecuteStream 执行流式请求（本示例中未实现）。
func (EchoExecutor) ExecuteStream(context.Context, *coreauth.Auth, clipexec.Request, clipexec.Options) (*clipexec.StreamResult, error) {
	return nil, errors.New("echo executor: ExecuteStream not implemented")
}

// Refresh 刷新认证信息（本示例中未实现）。
func (EchoExecutor) Refresh(context.Context, *coreauth.Auth) (*coreauth.Auth, error) {
	return nil, errors.New("echo executor: Refresh not implemented")
}

// CountTokens 计算 token 数量（本示例中未实现）。
func (EchoExecutor) CountTokens(context.Context, *coreauth.Auth, clipexec.Request, clipexec.Options) (clipexec.Response, error) {
	return clipexec.Response{}, errors.New("echo executor: CountTokens not implemented")
}

// main 是示例程序的入口函数。
// 它演示了两种使用 HTTP 请求的方式：
// 1. 使用 NewHttpRequest 构建预处理后的请求，然后手动执行
// 2. 使用 HttpRequest 直接执行带有凭证注入的请求
func main() {
	log.SetLevel(log.InfoLevel)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	core := coreauth.NewManager(nil, nil, nil)
	core.RegisterExecutor(EchoExecutor{})

	auth := &coreauth.Auth{
		ID:       "demo-echo",
		Provider: providerKey,
		Attributes: map[string]string{
			"api_key": "demo-api-key",
		},
	}

	// Example 1: Build a prepared request and execute it using your own http.Client.
	reqPrepared, errReqPrepared := core.NewHttpRequest(
		ctx,
		auth,
		http.MethodGet,
		"https://httpbin.org/anything",
		nil,
		http.Header{"X-Example": []string{"prepared"}},
	)
	if errReqPrepared != nil {
		panic(errReqPrepared)
	}
	respPrepared, errDoPrepared := http.DefaultClient.Do(reqPrepared)
	if errDoPrepared != nil {
		panic(errDoPrepared)
	}
	defer func() {
		if errClose := respPrepared.Body.Close(); errClose != nil {
			log.Errorf("close response body error: %v", errClose)
		}
	}()
	bodyPrepared, errReadPrepared := io.ReadAll(respPrepared.Body)
	if errReadPrepared != nil {
		panic(errReadPrepared)
	}
	fmt.Printf("Prepared request status: %d\n%s\n\n", respPrepared.StatusCode, bodyPrepared)

	// Example 2: Execute a raw request via core.HttpRequest (auto inject + do).
	rawBody := []byte(`{"hello":"world"}`)
	rawReq, errRawReq := http.NewRequestWithContext(ctx, http.MethodPost, "https://httpbin.org/anything", bytes.NewReader(rawBody))
	if errRawReq != nil {
		panic(errRawReq)
	}
	rawReq.Header.Set("Content-Type", "application/json")
	rawReq.Header.Set("X-Example", "executed")

	respExec, errDoExec := core.HttpRequest(ctx, auth, rawReq)
	if errDoExec != nil {
		panic(errDoExec)
	}
	defer func() {
		if errClose := respExec.Body.Close(); errClose != nil {
			log.Errorf("close response body error: %v", errClose)
		}
	}()
	bodyExec, errReadExec := io.ReadAll(respExec.Body)
	if errReadExec != nil {
		panic(errReadExec)
	}
	fmt.Printf("Manager HttpRequest status: %d\n%s\n", respExec.StatusCode, bodyExec)
}
