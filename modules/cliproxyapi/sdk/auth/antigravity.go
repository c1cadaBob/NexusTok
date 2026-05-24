// auth - antigravity.go
// 本文件实现了 Antigravity 提供商的 OAuth 登录认证流程。
// Antigravity 是基于 Google OAuth 的认证提供者，与 Gemini 共享部分基础设施。
// 认证流程通过本地 OAuth 回调服务器接收授权码，交换令牌后获取用户邮箱和 GCP 项目 ID。
package auth

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/auth/antigravity"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/browser"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/misc"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/util"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	log "github.com/sirupsen/logrus"
)

// AntigravityAuthenticator 实现了 Antigravity 提供商的 OAuth 登录流程。
// Antigravity 基于 Google OAuth，使用与 Gemini 类似的认证机制。
type AntigravityAuthenticator struct{}

// NewAntigravityAuthenticator 创建一个新的 Antigravity 认证器实例。
func NewAntigravityAuthenticator() Authenticator { return &AntigravityAuthenticator{} }

// Provider 返回该认证器对应的提供商标识 "antigravity"。
func (AntigravityAuthenticator) Provider() string { return "antigravity" }

// RefreshLead 返回令牌到期前应提前执行刷新的时间间隔。
// Antigravity 令牌应在到期前 5 分钟开始刷新。
func (AntigravityAuthenticator) RefreshLead() *time.Duration {
	return new(5 * time.Minute)
}

// Login 执行 Antigravity 账号的完整 OAuth 登录流程。
// 流程概述：
//  1. 生成随机 state 参数
//  2. 启动本地 OAuth 回调 HTTP 服务器
//  3. 构建授权 URL 并尝试打开浏览器
//  4. 等待 OAuth 回调（支持自动回调和手动粘贴两种模式）
//  5. 验证 state 参数，交换授权码获取令牌
//  6. 获取用户邮箱和 GCP 项目 ID
//  7. 构建并返回认证记录
//
// 参数说明：
//   - ctx: 上下文，用于控制请求超时和取消
//   - cfg: 全局配置，不能为 nil
//   - opts: 登录选项，可为 nil 使用默认值
func (AntigravityAuthenticator) Login(ctx context.Context, cfg *config.Config, opts *LoginOptions) (*coreauth.Auth, error) {
	if cfg == nil {
		return nil, fmt.Errorf("cliproxy auth: configuration is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if opts == nil {
		opts = &LoginOptions{}
	}

	// 确定回调端口
	callbackPort := antigravity.CallbackPort
	if opts.CallbackPort > 0 {
		callbackPort = opts.CallbackPort
	}

	// 创建 Antigravity 认证服务
	authSvc := antigravity.NewAntigravityAuth(cfg, nil)

	// 生成随机 state 参数
	state, err := misc.GenerateRandomState()
	if err != nil {
		return nil, fmt.Errorf("antigravity: failed to generate state: %w", err)
	}

	// 启动本地 OAuth 回调 HTTP 服务器
	srv, port, cbChan, errServer := startAntigravityCallbackServer(callbackPort)
	if errServer != nil {
		return nil, fmt.Errorf("antigravity: failed to start callback server: %w", errServer)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	// 构建重定向 URI 和授权 URL
	redirectURI := fmt.Sprintf("http://localhost:%d/oauth-callback", port)
	authURL := authSvc.BuildAuthURL(state, redirectURI)

	// 根据 NoBrowser 选项决定是否自动打开浏览器
	if !opts.NoBrowser {
		fmt.Println("Opening browser for antigravity authentication")
		if !browser.IsAvailable() {
			log.Warn("No browser available; please open the URL manually")
			util.PrintSSHTunnelInstructions(port)
			fmt.Printf("Visit the following URL to continue authentication:\n%s\n", authURL)
		} else if errOpen := browser.OpenURL(authURL); errOpen != nil {
			log.Warnf("Failed to open browser automatically: %v", errOpen)
			util.PrintSSHTunnelInstructions(port)
			fmt.Printf("Visit the following URL to continue authentication:\n%s\n", authURL)
		}
	} else {
		util.PrintSSHTunnelInstructions(port)
		fmt.Printf("Visit the following URL to continue authentication:\n%s\n", authURL)
	}

	fmt.Println("Waiting for antigravity authentication callback...")

	var cbRes callbackResult
	// 认证总超时定时器（5 分钟）
	timeoutTimer := time.NewTimer(5 * time.Minute)
	defer timeoutTimer.Stop()

	// 手动输入提示定时器（15 秒后提示）
	var manualPromptTimer *time.Timer
	var manualPromptC <-chan time.Time
	if opts.Prompt != nil {
		manualPromptTimer = time.NewTimer(15 * time.Second)
		manualPromptC = manualPromptTimer.C
		defer manualPromptTimer.Stop()
	}

	var manualInputCh <-chan string
	var manualInputErrCh <-chan error

	// 等待 OAuth 回调结果的主循环
waitForCallback:
	for {
		select {
		case res := <-cbChan:
			// 收到自动回调结果
			cbRes = res
			break waitForCallback
		case <-manualPromptC:
			// 超时后提示用户手动输入
			manualPromptC = nil
			if manualPromptTimer != nil {
				manualPromptTimer.Stop()
			}
			// 再次检查是否有自动回调到达
			select {
			case res := <-cbChan:
				cbRes = res
				break waitForCallback
			default:
			}
			// 异步提示用户手动粘贴回调 URL
			manualInputCh, manualInputErrCh = misc.AsyncPrompt(opts.Prompt, "Paste the antigravity callback URL (or press Enter to keep waiting): ")
			continue
		case input := <-manualInputCh:
			// 收到用户手动粘贴的回调 URL
			manualInputCh = nil
			manualInputErrCh = nil
			parsed, errParse := misc.ParseOAuthCallback(input)
			if errParse != nil {
				return nil, errParse
			}
			if parsed == nil {
				continue
			}
			cbRes = callbackResult{
				Code:  parsed.Code,
				State: parsed.State,
				Error: parsed.Error,
			}
			break waitForCallback
		case errManual := <-manualInputErrCh:
			// 手动输入过程中发生错误
			return nil, errManual
		case <-timeoutTimer.C:
			// 认证超时
			return nil, fmt.Errorf("antigravity: authentication timed out")
		}
	}

	// 检查 OAuth 回调是否返回错误
	if cbRes.Error != "" {
		return nil, fmt.Errorf("antigravity: authentication failed: %s", cbRes.Error)
	}
	// 验证 state 参数
	if cbRes.State != state {
		return nil, fmt.Errorf("antigravity: invalid state")
	}
	// 检查授权码是否为空
	if cbRes.Code == "" {
		return nil, fmt.Errorf("antigravity: missing authorization code")
	}

	// 使用授权码交换令牌
	tokenResp, errToken := authSvc.ExchangeCodeForTokens(ctx, cbRes.Code, redirectURI)
	if errToken != nil {
		return nil, fmt.Errorf("antigravity: token exchange failed: %w", errToken)
	}

	// 验证访问令牌不为空
	accessToken := strings.TrimSpace(tokenResp.AccessToken)
	if accessToken == "" {
		return nil, fmt.Errorf("antigravity: token exchange returned empty access token")
	}

	// 获取用户邮箱
	email, errInfo := authSvc.FetchUserInfo(ctx, accessToken)
	if errInfo != nil {
		return nil, fmt.Errorf("antigravity: fetch user info failed: %w", errInfo)
	}
	email = strings.TrimSpace(email)
	if email == "" {
		return nil, fmt.Errorf("antigravity: empty email returned from user info")
	}

	// 通过 loadCodeAssist 获取 GCP 项目 ID（与 Gemini CLI 相同的方式）
	projectID := ""
	if accessToken != "" {
		fetchedProjectID, errProject := authSvc.FetchProjectID(ctx, accessToken)
		if errProject != nil {
			log.Warnf("antigravity: failed to fetch project ID: %v", errProject)
		} else {
			projectID = fetchedProjectID
			log.Infof("antigravity: obtained project ID %s", projectID)
		}
	}

	// 构建元数据
	now := time.Now()
	metadata := map[string]any{
		"type":          "antigravity",
		"access_token":  tokenResp.AccessToken,
		"refresh_token": tokenResp.RefreshToken,
		"expires_in":    tokenResp.ExpiresIn,
		"timestamp":     now.UnixMilli(),
		"expired":       now.Add(time.Duration(tokenResp.ExpiresIn) * time.Second).Format(time.RFC3339),
	}
	if email != "" {
		metadata["email"] = email
	}
	if projectID != "" {
		metadata["project_id"] = projectID
	}

	// 构建认证文件名和标签
	fileName := antigravity.CredentialFileName(email)
	label := email
	if label == "" {
		label = "antigravity"
	}

	fmt.Println("Antigravity authentication successful")
	if projectID != "" {
		fmt.Printf("Using GCP project: %s\n", projectID)
	}
	// 返回认证记录
	return &coreauth.Auth{
		ID:       fileName,
		Provider: "antigravity",
		FileName: fileName,
		Label:    label,
		Metadata: metadata,
	}, nil
}

// callbackResult 封装 OAuth 回调的解析结果。
// 在 xAI 和 Antigravity 认证器中共享使用。
type callbackResult struct {
	// Code 是 OAuth 授权码。
	Code string
	// Error 是 OAuth 回调返回的错误信息。
	Error string
	// State 是 OAuth state 参数，用于防止 CSRF 攻击。
	State string
}

// startAntigravityCallbackServer 启动一个本地 HTTP 服务器用于接收 Antigravity OAuth 回调。
// 服务器监听指定端口（或自动分配可用端口），在收到回调请求后通过通道传递结果。
// 回调路径为 "/oauth-callback"。
// 参数说明：
//   - port: 监听端口号，<= 0 时使用默认端口
//
// 返回值：
//   - *http.Server: HTTP 服务器实例
//   - int: 实际监听的端口号
//   - <-chan callbackResult: 回调结果通道
//   - error: 启动错误
func startAntigravityCallbackServer(port int) (*http.Server, int, <-chan callbackResult, error) {
	if port <= 0 {
		port = antigravity.CallbackPort
	}
	addr := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, 0, nil, err
	}
	// 获取实际分配的端口号
	port = listener.Addr().(*net.TCPAddr).Port
	resultCh := make(chan callbackResult, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/oauth-callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		res := callbackResult{
			Code:  strings.TrimSpace(q.Get("code")),
			Error: strings.TrimSpace(q.Get("error")),
			State: strings.TrimSpace(q.Get("state")),
		}
		resultCh <- res
		if res.Code != "" && res.Error == "" {
			_, _ = w.Write([]byte("<h1>Login successful</h1><p>You can close this window.</p>"))
		} else {
			_, _ = w.Write([]byte("<h1>Login failed</h1><p>Please check the CLI output.</p>"))
		}
	})

	srv := &http.Server{Handler: mux}
	go func() {
		if errServe := srv.Serve(listener); errServe != nil && !strings.Contains(errServe.Error(), "Server closed") {
			log.Warnf("antigravity callback server error: %v", errServe)
		}
	}()

	return srv, port, resultCh, nil
}

// FetchAntigravityProjectID 为外部调用者暴露 GCP 项目 ID 发现功能。
// 该函数通过访问令牌查询 Antigravity 服务以获取关联的 GCP 项目 ID。
// 参数说明：
//   - ctx: 上下文
//   - accessToken: Google OAuth 访问令牌
//   - httpClient: HTTP 客户端，可为 nil 使用默认客户端
//
// 返回值为 GCP 项目 ID 字符串，或错误信息。
func FetchAntigravityProjectID(ctx context.Context, accessToken string, httpClient *http.Client) (string, error) {
	cfg := &config.Config{}
	authSvc := antigravity.NewAntigravityAuth(cfg, httpClient)
	return authSvc.FetchProjectID(ctx, accessToken)
}
