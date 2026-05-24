// gemini - gemini_auth.go
// 包 gemini 提供 Google Gemini AI 服务的认证和令牌管理功能。
// 该文件实现了完整的 OAuth2 认证流程，包括通过 Web 授权获取令牌、
// 存储令牌以及在令牌过期时刷新令牌等功能。
package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/auth/codex"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/browser"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/misc"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/util"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/proxyutil"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// Gemini OAuth 配置常量。
const (
	// ClientID 是 Google OAuth2 应用的客户端标识符
	ClientID = "REDACTED_GOOGLE_OAUTH_CLIENT_ID"
	// ClientSecret 是 Google OAuth2 应用的客户端密钥
	ClientSecret = "REDACTED_GOOGLE_OAUTH_CLIENT_SECRET"
	// DefaultCallbackPort 是默认的 OAuth 回调端口
	DefaultCallbackPort = 8085
)

// Scopes 定义了 Gemini 认证所需的 OAuth 权限范围。
var Scopes = []string{
	"https://www.googleapis.com/auth/cloud-platform",
	"https://www.googleapis.com/auth/userinfo.email",
	"https://www.googleapis.com/auth/userinfo.profile",
}

// GeminiAuth 提供处理 Gemini OAuth2 认证流程的方法。
// 封装了获取、存储和刷新 Google Gemini AI 服务认证令牌的逻辑。
type GeminiAuth struct {
}

// WebLoginOptions 自定义交互式 OAuth 流程的行为。
type WebLoginOptions struct {
	// NoBrowser 指示是否不自动打开浏览器
	NoBrowser bool
	// CallbackPort 是 OAuth 回调服务器监听的端口
	CallbackPort int
	// Prompt 是用于手动输入回调 URL 的提示函数
	Prompt func(string) (string, error)
}

// NewGeminiAuth 创建一个新的 GeminiAuth 实例。
//
// 返回：
//   - *GeminiAuth: 新的 GeminiAuth 实例
func NewGeminiAuth() *GeminiAuth {
	return &GeminiAuth{}
}

// GetAuthenticatedClient 配置并返回一个准备好进行认证 API 调用的 HTTP 客户端。
// 管理整个 OAuth2 流程，包括处理代理、加载现有令牌、
// 在必要时启动新的 Web OAuth 流程以及刷新令牌。
//
// 参数：
//   - ctx: HTTP 客户端的上下文
//   - ts: 包含认证令牌的 Gemini 令牌存储
//   - cfg: 包含代理设置的配置
//   - opts: 可选参数，用于自定义浏览器和提示行为
//
// 返回：
//   - *http.Client: 配置了认证的 HTTP 客户端
//   - error: 客户端配置失败时返回的错误
func (g *GeminiAuth) GetAuthenticatedClient(ctx context.Context, ts *GeminiTokenStorage, cfg *config.Config, opts *WebLoginOptions) (*http.Client, error) {
	callbackPort := DefaultCallbackPort
	if opts != nil && opts.CallbackPort > 0 {
		callbackPort = opts.CallbackPort
	}
	callbackURL := fmt.Sprintf("http://localhost:%d/oauth2callback", callbackPort)

	transport, _, errBuild := proxyutil.BuildHTTPTransport(cfg.ProxyURL)
	if errBuild != nil {
		log.Errorf("%v", errBuild)
	} else if transport != nil {
		proxyClient := &http.Client{Transport: transport}
		ctx = context.WithValue(ctx, oauth2.HTTPClient, proxyClient)
	}

	var err error

	// Configure the OAuth2 client.
	conf := &oauth2.Config{
		ClientID:     ClientID,
		ClientSecret: ClientSecret,
		RedirectURL:  callbackURL, // This will be used by the local server.
		Scopes:       Scopes,
		Endpoint:     google.Endpoint,
	}

	var token *oauth2.Token

	// If no token is found in storage, initiate the web-based OAuth flow.
	if ts.Token == nil {
		fmt.Printf("Could not load token from file, starting OAuth flow.\n")
		token, err = g.getTokenFromWeb(ctx, conf, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to get token from web: %w", err)
		}
		// After getting a new token, create a new token storage object with user info.
		newTs, errCreateTokenStorage := g.createTokenStorage(ctx, conf, token, ts.ProjectID)
		if errCreateTokenStorage != nil {
			log.Errorf("Warning: failed to create token storage: %v", errCreateTokenStorage)
			return nil, errCreateTokenStorage
		}
		*ts = *newTs
	}

	// Unmarshal the stored token into an oauth2.Token object.
	tsToken, _ := json.Marshal(ts.Token)
	if err = json.Unmarshal(tsToken, &token); err != nil {
		return nil, fmt.Errorf("failed to unmarshal token: %w", err)
	}

	// Return an HTTP client that automatically handles token refreshing.
	return conf.Client(ctx, token), nil
}

// createTokenStorage 创建一个新的 GeminiTokenStorage 对象。
// 使用提供的令牌获取用户的电子邮件，并填充存储结构。
//
// 参数：
//   - ctx: HTTP 请求的上下文
//   - config: OAuth2 配置
//   - token: 用于认证的 OAuth2 令牌
//   - projectID: 与此令牌关联的 Google Cloud 项目 ID
//
// 返回：
//   - *GeminiTokenStorage: 包含用户信息的新令牌存储对象
//   - error: 令牌存储创建失败时返回的错误
func (g *GeminiAuth) createTokenStorage(ctx context.Context, config *oauth2.Config, token *oauth2.Token, projectID string) (*GeminiTokenStorage, error) {
	httpClient := config.Client(ctx, token)
	req, err := http.NewRequestWithContext(ctx, "GET", "https://www.googleapis.com/oauth2/v1/userinfo?alt=json", nil)
	if err != nil {
		return nil, fmt.Errorf("could not get user info: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token.AccessToken))

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() {
		if err = resp.Body.Close(); err != nil {
			log.Printf("warn: failed to close response body: %v", err)
		}
	}()

	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("get user info request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	emailResult := gjson.GetBytes(bodyBytes, "email")
	if emailResult.Exists() && emailResult.Type == gjson.String {
		fmt.Printf("Authenticated user email: %s\n", emailResult.String())
	} else {
		fmt.Println("Failed to get user email from token")
	}

	var ifToken map[string]any
	jsonData, _ := json.Marshal(token)
	err = json.Unmarshal(jsonData, &ifToken)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal token: %w", err)
	}

	ifToken["token_uri"] = "https://oauth2.googleapis.com/token"
	ifToken["client_id"] = ClientID
	ifToken["client_secret"] = ClientSecret
	ifToken["scopes"] = Scopes
	ifToken["universe_domain"] = "googleapis.com"

	ts := GeminiTokenStorage{
		Token:     ifToken,
		ProjectID: projectID,
		Email:     emailResult.String(),
	}

	return &ts, nil
}

// getTokenFromWeb 启动基于 Web 的 OAuth2 授权流程。
// 启动本地 HTTP 服务器以监听来自 Google 认证服务器的回调，
// 打开用户的浏览器到授权 URL，并将收到的授权码交换为访问令牌。
//
// 参数：
//   - ctx: HTTP 客户端的上下文
//   - config: OAuth2 配置
//   - opts: 可选参数，用于自定义浏览器和提示行为
//
// 返回：
//   - *oauth2.Token: 从授权流程中获取的 OAuth2 令牌
//   - error: 令牌获取失败时返回的错误
func (g *GeminiAuth) getTokenFromWeb(ctx context.Context, config *oauth2.Config, opts *WebLoginOptions) (*oauth2.Token, error) {
	callbackPort := DefaultCallbackPort
	if opts != nil && opts.CallbackPort > 0 {
		callbackPort = opts.CallbackPort
	}
	callbackURL := fmt.Sprintf("http://localhost:%d/oauth2callback", callbackPort)

	// Use a channel to pass the authorization code from the HTTP handler to the main function.
	codeChan := make(chan string, 1)
	errChan := make(chan error, 1)

	// Create a new HTTP server with its own multiplexer.
	mux := http.NewServeMux()
	server := &http.Server{Addr: fmt.Sprintf(":%d", callbackPort), Handler: mux}
	config.RedirectURL = callbackURL

	mux.HandleFunc("/oauth2callback", func(w http.ResponseWriter, r *http.Request) {
		if err := r.URL.Query().Get("error"); err != "" {
			_, _ = fmt.Fprintf(w, "Authentication failed: %s", err)
			select {
			case errChan <- fmt.Errorf("authentication failed via callback: %s", err):
			default:
			}
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			_, _ = fmt.Fprint(w, "Authentication failed: code not found.")
			select {
			case errChan <- fmt.Errorf("code not found in callback"):
			default:
			}
			return
		}
		_, _ = fmt.Fprint(w, "<html><body><h1>Authentication successful!</h1><p>You can close this window.</p></body></html>")
		select {
		case codeChan <- code:
		default:
		}
	})

	// Start the server in a goroutine.
	go func() {
		if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			log.Errorf("ListenAndServe(): %v", err)
			select {
			case errChan <- err:
			default:
			}
		}
	}()

	// Open the authorization URL in the user's browser.
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline, oauth2.SetAuthURLParam("prompt", "consent"))

	noBrowser := false
	if opts != nil {
		noBrowser = opts.NoBrowser
	}

	if !noBrowser {
		fmt.Println("Opening browser for authentication...")

		// Check if browser is available
		if !browser.IsAvailable() {
			log.Warn("No browser available on this system")
			util.PrintSSHTunnelInstructions(callbackPort)
			fmt.Printf("Please manually open this URL in your browser:\n\n%s\n", authURL)
		} else {
			if err := browser.OpenURL(authURL); err != nil {
				authErr := codex.NewAuthenticationError(codex.ErrBrowserOpenFailed, err)
				log.Warn(codex.GetUserFriendlyMessage(authErr))
				util.PrintSSHTunnelInstructions(callbackPort)
				fmt.Printf("Please manually open this URL in your browser:\n\n%s\n", authURL)

				// Log platform info for debugging
				platformInfo := browser.GetPlatformInfo()
				log.Debugf("Browser platform info: %+v", platformInfo)
			} else {
				log.Debug("Browser opened successfully")
			}
		}
	} else {
		util.PrintSSHTunnelInstructions(callbackPort)
		fmt.Printf("Please open this URL in your browser:\n\n%s\n", authURL)
	}

	fmt.Println("Waiting for authentication callback...")

	// Wait for the authorization code or an error.
	var authCode string
	timeoutTimer := time.NewTimer(5 * time.Minute)
	defer timeoutTimer.Stop()

	var manualPromptTimer *time.Timer
	var manualPromptC <-chan time.Time
	if opts != nil && opts.Prompt != nil {
		manualPromptTimer = time.NewTimer(15 * time.Second)
		manualPromptC = manualPromptTimer.C
		defer manualPromptTimer.Stop()
	}

	var manualInputCh <-chan string
	var manualInputErrCh <-chan error

waitForCallback:
	for {
		select {
		case code := <-codeChan:
			authCode = code
			break waitForCallback
		case err := <-errChan:
			return nil, err
		case <-manualPromptC:
			manualPromptC = nil
			if manualPromptTimer != nil {
				manualPromptTimer.Stop()
			}
			select {
			case code := <-codeChan:
				authCode = code
				break waitForCallback
			case err := <-errChan:
				return nil, err
			default:
			}
			manualInputCh, manualInputErrCh = misc.AsyncPrompt(opts.Prompt, "Paste the Gemini callback URL (or press Enter to keep waiting): ")
			continue
		case input := <-manualInputCh:
			manualInputCh = nil
			manualInputErrCh = nil
			parsed, errParse := misc.ParseOAuthCallback(input)
			if errParse != nil {
				return nil, errParse
			}
			if parsed == nil {
				continue
			}
			if parsed.Error != "" {
				return nil, fmt.Errorf("authentication failed via callback: %s", parsed.Error)
			}
			if parsed.Code == "" {
				return nil, fmt.Errorf("code not found in callback")
			}
			authCode = parsed.Code
			break waitForCallback
		case errManual := <-manualInputErrCh:
			return nil, errManual
		case <-timeoutTimer.C:
			return nil, fmt.Errorf("oauth flow timed out")
		}
	}

	// Shutdown the server.
	if err := server.Shutdown(ctx); err != nil {
		log.Errorf("Failed to shut down server: %v", err)
	}

	// Exchange the authorization code for a token.
	token, err := config.Exchange(ctx, authCode)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange token: %w", err)
	}

	fmt.Println("Authentication successful.")
	return token, nil
}
