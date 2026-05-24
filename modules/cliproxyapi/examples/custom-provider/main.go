// Package main - custom-provider 示例
// 演示如何创建自定义 AI 提供者执行器（Executor）并将其集成到 CLI Proxy API 服务器中。
// 本示例展示了以下功能：
// - 创建实现 Executor 接口的自定义执行器
// - 注册自定义翻译器用于请求/响应转换
// - 将自定义提供者与 SDK 服务器集成
// - 在模型注册表中注册自定义模型
//
// 本示例使用简单的 echo 服务（httpbin.org）作为上游 API 进行演示。
// 在实际实现中，您需要将其替换为实际的 AI 服务提供者。
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/api"
	sdkAuth "github.com/router-for-me/CLIProxyAPI/v7/sdk/auth"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	clipexec "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/logging"
	sdktr "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"
)

// 自定义提供者的常量定义。
const (
	// providerKey 是自定义提供者的唯一标识符。
	providerKey = "myprov"

	// fOpenAI 表示 OpenAI 聊天格式。
	fOpenAI = sdktr.Format("openai.chat")

	// fMyProv 表示自定义提供者的聊天格式。
	fMyProv = sdktr.Format("myprov.chat")
)

// init 注册用于演示的简单翻译器。
// 在实际实现中，您需要实现 OpenAI 格式与您的提供者格式之间的正确请求/响应转换逻辑。
func init() {
	sdktr.Register(fOpenAI, fMyProv,
		func(model string, raw []byte, stream bool) []byte { return raw },
		sdktr.ResponseTransform{
			Stream: func(ctx context.Context, model string, originalReq, translatedReq, raw []byte, param *any) [][]byte {
				return [][]byte{raw}
			},
			NonStream: func(ctx context.Context, model string, originalReq, translatedReq, raw []byte, param *any) []byte {
				return raw
			},
		},
	)
}

// MyExecutor 是用于演示的最小化提供者实现。
// 它实现了 Executor 接口，用于处理对自定义 AI 提供者的请求。
type MyExecutor struct{}

// Identifier 返回此执行器的唯一标识符。
func (MyExecutor) Identifier() string { return providerKey }

// PrepareRequest 可选地向原始 HTTP 请求注入凭证。
// 该方法在每次请求前被调用，允许执行器修改 HTTP 请求，
// 添加认证头或其他必要的修改。
//
// 参数:
//   - req: 要准备的 HTTP 请求
//   - a: 认证信息
//
// 返回值:
//   - error: 请求准备失败时返回错误
func (MyExecutor) PrepareRequest(req *http.Request, a *coreauth.Auth) error {
	if req == nil || a == nil {
		return nil
	}
	if a.Attributes != nil {
		if ak := strings.TrimSpace(a.Attributes["api_key"]); ak != "" {
			req.Header.Set("Authorization", "Bearer "+ak)
		}
	}
	return nil
}

// buildHTTPClient 根据认证信息中的代理 URL 构建 HTTP 客户端。
// 如果未配置代理，则返回默认的 HTTP 客户端。
func buildHTTPClient(a *coreauth.Auth) *http.Client {
	if a == nil || strings.TrimSpace(a.ProxyURL) == "" {
		return http.DefaultClient
	}
	u, err := url.Parse(a.ProxyURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return http.DefaultClient
	}
	return &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(u)}}
}

// upstreamEndpoint 从认证信息中获取上游 API 端点地址。
// 如果未配置，则返回默认的 echo 端点（httpbin.org/post）。
func upstreamEndpoint(a *coreauth.Auth) string {
	if a != nil && a.Attributes != nil {
		if ep := strings.TrimSpace(a.Attributes["endpoint"]); ep != "" {
			return ep
		}
	}
	// Demo echo endpoint; replace with your upstream.
	return "https://httpbin.org/post"
}

// Execute 执行同步请求，将请求负载发送到上游 API 并返回响应。
func (MyExecutor) Execute(ctx context.Context, a *coreauth.Auth, req clipexec.Request, opts clipexec.Options) (clipexec.Response, error) {
	client := buildHTTPClient(a)
	endpoint := upstreamEndpoint(a)

	httpReq, errNew := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(req.Payload))
	if errNew != nil {
		return clipexec.Response{}, errNew
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Inject credentials via PrepareRequest hook.
	if errPrep := (MyExecutor{}).PrepareRequest(httpReq, a); errPrep != nil {
		return clipexec.Response{}, errPrep
	}

	resp, errDo := client.Do(httpReq)
	if errDo != nil {
		return clipexec.Response{}, errDo
	}
	defer func() {
		if errClose := resp.Body.Close(); errClose != nil {
			fmt.Fprintf(os.Stderr, "close response body error: %v\n", errClose)
		}
	}()
	body, _ := io.ReadAll(resp.Body)
	return clipexec.Response{Payload: body}, nil
}

// HttpRequest 执行原始 HTTP 请求，注入凭证后发送到上游 API。
func (MyExecutor) HttpRequest(ctx context.Context, a *coreauth.Auth, req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, fmt.Errorf("myprov executor: request is nil")
	}
	if ctx == nil {
		ctx = req.Context()
	}
	httpReq := req.WithContext(ctx)
	if errPrep := (MyExecutor{}).PrepareRequest(httpReq, a); errPrep != nil {
		return nil, errPrep
	}
	client := buildHTTPClient(a)
	return client.Do(httpReq)
}

// CountTokens 计算请求中的 token 数量（本示例中未实现）。
func (MyExecutor) CountTokens(context.Context, *coreauth.Auth, clipexec.Request, clipexec.Options) (clipexec.Response, error) {
	return clipexec.Response{}, errors.New("count tokens not implemented")
}

// ExecuteStream 执行流式请求，返回一个包含流式数据块的通道。
func (MyExecutor) ExecuteStream(ctx context.Context, a *coreauth.Auth, req clipexec.Request, opts clipexec.Options) (*clipexec.StreamResult, error) {
	ch := make(chan clipexec.StreamChunk, 1)
	go func() {
		defer close(ch)
		ch <- clipexec.StreamChunk{Payload: []byte("data: {\"ok\":true}\n\n")}
	}()
	return &clipexec.StreamResult{Chunks: ch}, nil
}

// Refresh 刷新认证信息（本示例中直接返回原始认证信息）。
func (MyExecutor) Refresh(ctx context.Context, a *coreauth.Auth) (*coreauth.Auth, error) {
	return a, nil
}

// main 是示例程序的入口函数。
// 它加载配置、初始化认证管理器、注册自定义执行器和模型，
// 然后启动 CLI Proxy API 服务器。
func main() {
	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		panic(err)
	}

	tokenStore := sdkAuth.GetTokenStore()
	if dirSetter, ok := tokenStore.(interface{ SetBaseDir(string) }); ok {
		dirSetter.SetBaseDir(cfg.AuthDir)
	}
	core := coreauth.NewManager(tokenStore, nil, nil)
	core.RegisterExecutor(MyExecutor{})

	hooks := cliproxy.Hooks{
		OnAfterStart: func(s *cliproxy.Service) {
			// Register demo models for the custom provider so they appear in /v1/models.
			models := []*cliproxy.ModelInfo{{ID: "myprov-pro-1", Object: "model", Type: providerKey, DisplayName: "MyProv Pro 1"}}
			for _, a := range core.List() {
				if strings.EqualFold(a.Provider, providerKey) {
					cliproxy.GlobalModelRegistry().RegisterClient(a.ID, providerKey, models)
				}
			}
		},
	}

	svc, err := cliproxy.NewBuilder().
		WithConfig(cfg).
		WithConfigPath("config.yaml").
		WithCoreAuthManager(core).
		WithServerOptions(
			// Optional: add a simple middleware + custom request logger
			api.WithMiddleware(func(c *gin.Context) { c.Header("X-Example", "custom-provider"); c.Next() }),
			api.WithRequestLoggerFactory(func(cfg *config.Config, cfgPath string) logging.RequestLogger {
				return logging.NewFileRequestLoggerWithOptions(true, "logs", filepath.Dir(cfgPath), cfg.ErrorLogsMaxFiles)
			}),
		).
		WithHooks(hooks).
		Build()
	if err != nil {
		panic(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if errRun := svc.Run(ctx); errRun != nil && !errors.Is(errRun, context.Canceled) {
		panic(errRun)
	}
	_ = os.Stderr // keep os import used (demo only)
	_ = time.Second
}
