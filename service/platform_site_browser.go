package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/c1cadaBob/NexusTok/common"
	"github.com/gorilla/websocket"
)

const (
	platformSiteBrowserAuthEnabledEnv    = "PLATFORM_SITE_BROWSER_AUTH_ENABLED"
	platformSiteBrowserExecutableEnv     = "PLATFORM_SITE_BROWSER_EXECUTABLE"
	platformSiteBrowserHeadlessEnv       = "PLATFORM_SITE_BROWSER_HEADLESS"
	platformSiteBrowserProfileDirEnv     = "PLATFORM_SITE_BROWSER_PROFILE_DIR"
	platformSiteBrowserTimeoutSecondsEnv = "PLATFORM_SITE_BROWSER_TIMEOUT_SECONDS"
	defaultPlatformSiteBrowserTimeout    = 45 * time.Second
	defaultPlatformSiteBrowserPoll       = 250 * time.Millisecond
	maxPlatformSiteBrowserResponseBytes  = 2 << 20
)

var (
	ErrPlatformSiteBrowserDisabled    = errors.New("平台站点后台浏览器认证已禁用")
	ErrPlatformSiteBrowserUnavailable = errors.New("未找到可用的 Chromium/Chrome 浏览器")
	ErrPlatformSiteBrowserLoginFailed = errors.New("平台站点后台浏览器登录失败")
)

type PlatformSiteBrowserAuthResult struct {
	AccessToken       string
	RefreshToken      string
	TokenExpiresAt    int64
	UserID            string
	Cookie            string
	TempToken         string
	VerificationType  string
	BrowserAuthStatus string
	Pending           PlatformSitePendingContext
}

type platformSiteBrowserConfig struct {
	Enabled    bool
	Executable string
	Headless   bool
	ProfileDir string
	Timeout    time.Duration
}

type platformSiteBrowserAuthenticator interface {
	Authenticate(context.Context, string, string, string, string) (*PlatformSiteBrowserAuthResult, error)
}

var (
	platformSiteBrowserAuthenticatorMu sync.RWMutex
	platformSiteBrowserAuthenticatorFn platformSiteBrowserAuthenticator = defaultPlatformSiteBrowserAuthenticator{}
)

func setPlatformSiteBrowserAuthenticatorForTests(authenticator platformSiteBrowserAuthenticator) func() {
	platformSiteBrowserAuthenticatorMu.Lock()
	previous := platformSiteBrowserAuthenticatorFn
	platformSiteBrowserAuthenticatorFn = authenticator
	platformSiteBrowserAuthenticatorMu.Unlock()
	return func() {
		platformSiteBrowserAuthenticatorMu.Lock()
		platformSiteBrowserAuthenticatorFn = previous
		platformSiteBrowserAuthenticatorMu.Unlock()
	}
}

func authenticatePlatformSiteInBrowser(
	ctx context.Context,
	platform string,
	baseURL string,
	username string,
	password string,
) (*PlatformSiteBrowserAuthResult, error) {
	platformSiteBrowserAuthenticatorMu.RLock()
	authenticator := platformSiteBrowserAuthenticatorFn
	platformSiteBrowserAuthenticatorMu.RUnlock()
	return authenticator.Authenticate(ctx, platform, baseURL, username, password)
}

func platformSiteBrowserConfigFromEnv() platformSiteBrowserConfig {
	timeoutSeconds := common.GetEnvOrDefault(
		platformSiteBrowserTimeoutSecondsEnv,
		int(defaultPlatformSiteBrowserTimeout/time.Second),
	)
	if timeoutSeconds < 5 {
		timeoutSeconds = 5
	}
	if timeoutSeconds > 300 {
		timeoutSeconds = 300
	}
	return platformSiteBrowserConfig{
		Enabled:    common.GetEnvOrDefaultBool(platformSiteBrowserAuthEnabledEnv, true),
		Executable: strings.TrimSpace(common.GetEnvOrDefaultString(platformSiteBrowserExecutableEnv, "")),
		Headless:   common.GetEnvOrDefaultBool(platformSiteBrowserHeadlessEnv, true),
		ProfileDir: strings.TrimSpace(common.GetEnvOrDefaultString(platformSiteBrowserProfileDirEnv, "")),
		Timeout:    time.Duration(timeoutSeconds) * time.Second,
	}
}

type defaultPlatformSiteBrowserAuthenticator struct{}

func (defaultPlatformSiteBrowserAuthenticator) Authenticate(
	ctx context.Context,
	platform string,
	baseURL string,
	username string,
	password string,
) (*PlatformSiteBrowserAuthResult, error) {
	config := platformSiteBrowserConfigFromEnv()
	if !config.Enabled {
		return nil, ErrPlatformSiteBrowserDisabled
	}
	if strings.TrimSpace(username) == "" || password == "" {
		return nil, fmt.Errorf("%w: 账号密码为空", ErrPlatformSiteBrowserLoginFailed)
	}
	executable, err := findPlatformSiteBrowserExecutable(config.Executable)
	if err != nil {
		return nil, err
	}
	origin, err := platformSiteOrigin(baseURL)
	if err != nil {
		return nil, err
	}
	if err := validatePlatformSiteURL(baseURL); err != nil {
		return nil, err
	}
	if err := validatePlatformSiteURL(origin); err != nil {
		return nil, err
	}
	browserContext, cancel := context.WithTimeout(ctx, config.Timeout)
	defer cancel()

	profileDir, cleanup, err := createPlatformSiteBrowserProfile(config.ProfileDir)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	process, endpoint, err := startPlatformSiteBrowser(browserContext, executable, profileDir, config.Headless)
	if err != nil {
		return nil, err
	}
	defer func() {
		if process.Process != nil {
			_ = process.Process.Kill()
		}
		_ = process.Wait()
	}()

	client, err := newPlatformSiteCDPClient(browserContext, endpoint)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	result, err := client.Login(browserContext, platform, origin, username, password)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func findPlatformSiteBrowserExecutable(configured string) (string, error) {
	if configured != "" {
		if _, err := os.Stat(configured); err != nil {
			return "", fmt.Errorf("%w: %s", ErrPlatformSiteBrowserUnavailable, configured)
		}
		return configured, nil
	}
	for _, candidate := range []string{
		"google-chrome",
		"google-chrome-stable",
		"chromium",
		"chromium-browser",
	} {
		if path, err := exec.LookPath(candidate); err == nil {
			return path, nil
		}
	}
	return "", ErrPlatformSiteBrowserUnavailable
}

func createPlatformSiteBrowserProfile(parent string) (string, func(), error) {
	if parent != "" {
		if err := os.MkdirAll(parent, 0o700); err != nil {
			return "", func() {}, fmt.Errorf("创建浏览器 profile 目录失败: %w", err)
		}
	}
	profile, err := os.MkdirTemp(parent, "nexustok-platform-site-")
	if err != nil {
		return "", func() {}, fmt.Errorf("创建浏览器临时 profile 失败: %w", err)
	}
	return profile, func() {
		_ = os.RemoveAll(profile)
	}, nil
}

func startPlatformSiteBrowser(
	ctx context.Context,
	executable string,
	profileDir string,
	headless bool,
) (*exec.Cmd, string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, "", fmt.Errorf("分配浏览器调试端口失败: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()

	args := []string{
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-background-networking",
		"--disable-dev-shm-usage",
		"--disable-popup-blocking",
		"--disable-extensions",
		"--remote-debugging-address=127.0.0.1",
		fmt.Sprintf("--remote-debugging-port=%d", port),
		"--user-data-dir=" + profileDir,
		"--window-size=1440,1200",
		"about:blank",
	}
	if headless {
		args = append(args, "--headless=new", "--disable-gpu", "--no-sandbox")
	}
	process := exec.CommandContext(ctx, executable, args...)
	process.Stdout = io.Discard
	process.Stderr = io.Discard
	if err := process.Start(); err != nil {
		return nil, "", fmt.Errorf("启动后台浏览器失败: %w", err)
	}
	endpoint, err := waitForPlatformSiteBrowserEndpoint(ctx, port)
	if err != nil {
		if process.Process != nil {
			_ = process.Process.Kill()
		}
		_ = process.Wait()
		return nil, "", err
	}
	return process, endpoint, nil
}

func waitForPlatformSiteBrowserEndpoint(ctx context.Context, port int) (string, error) {
	target := fmt.Sprintf("http://127.0.0.1:%d/json/version", port)
	client := &http.Client{Timeout: 2 * time.Second}
	ticker := time.NewTicker(defaultPlatformSiteBrowserPoll)
	defer ticker.Stop()
	for {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err == nil {
			response, requestErr := client.Do(request)
			if requestErr == nil {
				data, readErr := io.ReadAll(io.LimitReader(response.Body, maxPlatformSiteBrowserResponseBytes))
				_ = response.Body.Close()
				if readErr == nil && response.StatusCode == http.StatusOK {
					var version struct {
						WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
					}
					if common.Unmarshal(data, &version) == nil &&
						strings.TrimSpace(version.WebSocketDebuggerURL) != "" {
						return version.WebSocketDebuggerURL, nil
					}
				}
			}
		}
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("等待后台浏览器启动超时: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

type platformSiteCDPClient struct {
	conn      *websocket.Conn
	writeMu   sync.Mutex
	nextID    int64
	sessionID string
}

func newPlatformSiteCDPClient(ctx context.Context, endpoint string) (*platformSiteCDPClient, error) {
	dialer := websocket.DefaultDialer
	conn, _, err := dialer.DialContext(ctx, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("连接后台浏览器调试接口失败: %w", err)
	}
	client := &platformSiteCDPClient{conn: conn, nextID: 1}
	result, err := client.call(ctx, "Target.createTarget", map[string]any{"url": "about:blank"}, "")
	if err != nil {
		client.Close()
		return nil, err
	}
	var created struct {
		TargetID string `json:"targetId"`
	}
	if err := common.Unmarshal(result, &created); err != nil || created.TargetID == "" {
		client.Close()
		return nil, errors.New("后台浏览器未返回页面目标")
	}
	result, err = client.call(ctx, "Target.attachToTarget", map[string]any{
		"targetId": created.TargetID,
		"flatten":  true,
	}, "")
	if err != nil {
		client.Close()
		return nil, err
	}
	var attached struct {
		SessionID string `json:"sessionId"`
	}
	if err := common.Unmarshal(result, &attached); err != nil || attached.SessionID == "" {
		client.Close()
		return nil, errors.New("后台浏览器未返回页面会话")
	}
	client.sessionID = attached.SessionID
	return client, nil
}

func (client *platformSiteCDPClient) Close() {
	if client == nil || client.conn == nil {
		return
	}
	_ = client.conn.Close()
}

func (client *platformSiteCDPClient) call(
	ctx context.Context,
	method string,
	params map[string]any,
	sessionID string,
) ([]byte, error) {
	if client == nil || client.conn == nil {
		return nil, errors.New("后台浏览器会话不可用")
	}
	client.writeMu.Lock()
	defer client.writeMu.Unlock()
	id := client.nextID
	client.nextID++
	request := map[string]any{
		"id":     id,
		"method": method,
		"params": params,
	}
	if sessionID != "" {
		request["sessionId"] = sessionID
	}
	payload, err := common.Marshal(request)
	if err != nil {
		return nil, err
	}
	if err := client.conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		return nil, err
	}
	for {
		if deadline, ok := ctx.Deadline(); ok {
			_ = client.conn.SetReadDeadline(deadline)
		}
		_, responseData, readErr := client.conn.ReadMessage()
		if readErr != nil {
			return nil, readErr
		}
		var response struct {
			ID     int64           `json:"id"`
			Error  map[string]any  `json:"error"`
			Result json.RawMessage `json:"result"`
		}
		if err := common.Unmarshal(responseData, &response); err != nil {
			continue
		}
		if response.ID != id {
			continue
		}
		if len(response.Error) > 0 {
			return nil, fmt.Errorf("浏览器协议 %s 调用失败", method)
		}
		return response.Result, nil
	}
}

func (client *platformSiteCDPClient) evaluate(ctx context.Context, expression string) (any, error) {
	result, err := client.call(ctx, "Runtime.evaluate", map[string]any{
		"expression":    expression,
		"awaitPromise":  true,
		"returnByValue": true,
		"userGesture":   true,
	}, client.sessionID)
	if err != nil {
		return nil, err
	}
	var envelope struct {
		Result struct {
			Value            any `json:"value"`
			ExceptionDetails any `json:"exceptionDetails"`
		} `json:"result"`
	}
	if err := common.Unmarshal(result, &envelope); err != nil {
		return nil, err
	}
	if envelope.Result.ExceptionDetails != nil {
		return nil, errors.New("浏览器页面脚本执行失败")
	}
	return envelope.Result.Value, nil
}

func (client *platformSiteCDPClient) Login(
	ctx context.Context,
	platform string,
	origin string,
	username string,
	password string,
) (*PlatformSiteBrowserAuthResult, error) {
	if _, err := client.call(ctx, "Page.enable", nil, client.sessionID); err != nil {
		return nil, err
	}
	if _, err := client.call(ctx, "Runtime.enable", nil, client.sessionID); err != nil {
		return nil, err
	}
	if _, err := client.call(ctx, "Network.enable", nil, client.sessionID); err != nil {
		return nil, err
	}

	paths := []string{"", "/login", "/auth/login"}
	for _, path := range paths {
		target, err := upstreamSiteURL(origin, path, nil)
		if err != nil {
			continue
		}
		if _, err := client.call(ctx, "Page.navigate", map[string]any{"url": target}, client.sessionID); err != nil {
			continue
		}
		result, err := client.waitForLoginResult(ctx, platform, username, password)
		if err != nil {
			return nil, err
		}
		if result.BrowserAuthStatus == PlatformSiteBrowserAuthSuccess ||
			result.VerificationType == PlatformSiteVerificationOTPRequired ||
			result.VerificationType == PlatformSiteVerificationManualCaptcha {
			return result, nil
		}
	}
	return nil, fmt.Errorf("%w: 页面未完成登录", ErrPlatformSiteBrowserLoginFailed)
}

func (client *platformSiteCDPClient) waitForLoginResult(
	ctx context.Context,
	platform string,
	username string,
	password string,
) (*PlatformSiteBrowserAuthResult, error) {
	ticker := time.NewTicker(defaultPlatformSiteBrowserPoll)
	defer ticker.Stop()
	submitted := false
	for {
		if !submitted {
			value, err := client.evaluate(ctx, browserFillLoginScript(username, password))
			if err != nil {
				return nil, err
			}
			if state, ok := value.(map[string]any); ok && state["submitted"] == true {
				submitted = true
			}
		}
		value, err := client.evaluate(ctx, browserReadLoginStateScript())
		if err != nil {
			return nil, err
		}
		result, err := platformSiteBrowserResult(value, platform)
		if err != nil {
			return nil, err
		}
		if result.BrowserAuthStatus == PlatformSiteBrowserAuthSuccess ||
			result.VerificationType == PlatformSiteVerificationOTPRequired ||
			result.VerificationType == PlatformSiteVerificationManualCaptcha {
			cookies, cookieErr := client.browserCookies(ctx)
			if cookieErr == nil && cookies != "" {
				result.Cookie = cookies
				if result.VerificationType == "" {
					result.BrowserAuthStatus = PlatformSiteBrowserAuthSuccess
				}
			}
			if result.VerificationType == PlatformSiteVerificationOTPRequired {
				result.Pending = PlatformSitePendingContext{
					Kind: "browser_otp",
				}
				if platform == "newapi" {
					result.Pending.Kind = "newapi_cookie"
					result.Pending.Cookies = platformSiteCookiesFromHeader(result.Cookie)
				}
			}
			return result, nil
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("后台浏览器登录超时: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func (client *platformSiteCDPClient) browserCookies(ctx context.Context) (string, error) {
	result, err := client.call(ctx, "Network.getAllCookies", nil, client.sessionID)
	if err != nil {
		return "", err
	}
	var envelope struct {
		Cookies []struct {
			Name   string `json:"name"`
			Value  string `json:"value"`
			Domain string `json:"domain"`
		} `json:"cookies"`
	}
	if err := common.Unmarshal(result, &envelope); err != nil {
		return "", err
	}
	values := make([]string, 0, len(envelope.Cookies))
	for _, cookie := range envelope.Cookies {
		if strings.TrimSpace(cookie.Name) == "" {
			continue
		}
		values = append(values, cookie.Name+"="+cookie.Value)
	}
	return strings.Join(values, "; "), nil
}

func platformSiteBrowserResult(value any, platform string) (*PlatformSiteBrowserAuthResult, error) {
	record, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("浏览器页面未返回登录状态")
	}
	result := &PlatformSiteBrowserAuthResult{
		AccessToken:       browserString(record, "access_token"),
		RefreshToken:      browserString(record, "refresh_token"),
		UserID:            browserString(record, "user_id"),
		TempToken:         browserString(record, "temp_token"),
		TokenExpiresAt:    browserInt64(record, "expires_at"),
		BrowserAuthStatus: browserString(record, "browser_auth_status"),
		VerificationType:  browserString(record, "verification_type"),
	}
	if result.VerificationType == "" && browserBool(record, "otp_required") {
		result.VerificationType = PlatformSiteVerificationOTPRequired
	}
	if result.VerificationType == "" && browserBool(record, "manual_captcha_required") {
		result.VerificationType = PlatformSiteVerificationManualCaptcha
	}
	if result.VerificationType == "" && browserBool(record, "invalid_credential") {
		result.VerificationType = PlatformSiteVerificationInvalidCredential
	}
	if result.VerificationType == "" && result.BrowserAuthStatus == "" {
		result.BrowserAuthStatus = PlatformSiteBrowserAuthPending
	}
	_ = platform
	return result, nil
}

func browserString(record map[string]any, key string) string {
	value, ok := record[key]
	if !ok {
		return ""
	}
	text, _ := value.(string)
	return strings.TrimSpace(strings.TrimPrefix(text, "Bearer "))
}

func browserBool(record map[string]any, key string) bool {
	value, _ := record[key].(bool)
	return value
}

func browserInt64(record map[string]any, key string) int64 {
	value, ok := record[key].(float64)
	if !ok || value <= 0 {
		return 0
	}
	return int64(value)
}

func platformSiteCookiesFromHeader(header string) []PlatformSitePendingCookie {
	parts := strings.Split(header, ";")
	result := make([]PlatformSitePendingCookie, 0, len(parts))
	for _, part := range parts {
		name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok || name == "" {
			continue
		}
		result = append(result, PlatformSitePendingCookie{Name: name, Value: value})
	}
	return result
}

func browserFillLoginScript(username, password string) string {
	usernameJSON, _ := common.Marshal(username)
	passwordJSON, _ := common.Marshal(password)
	return fmt.Sprintf(`(() => {
const username = %s;
const password = %s;
const visible = (element) => {
  if (!element) return false;
  const style = getComputedStyle(element);
  return style.display !== 'none' && style.visibility !== 'hidden' && element.offsetParent !== null;
};
const inputs = Array.from(document.querySelectorAll('input')).filter(visible);
const passwordInput = inputs.find((input) => input.type === 'password' ||
  /pass(word)?|密码/i.test(input.name || input.id || input.autocomplete || ''));
const userInput = inputs.find((input) => input !== passwordInput &&
  /email|user(name)?|account|login|账号|用户名/i.test(
    input.name || input.id || input.autocomplete || input.placeholder || ''
  )) || inputs.find((input) => input !== passwordInput && input.type === 'text');
if (!passwordInput || !userInput) return {submitted: false};
const setValue = (input, value) => {
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set;
  setter.call(input, value);
  input.dispatchEvent(new Event('input', {bubbles: true}));
  input.dispatchEvent(new Event('change', {bubbles: true}));
};
setValue(userInput, username);
setValue(passwordInput, password);
const form = passwordInput.form || userInput.form;
const submit = form && form.querySelector('button[type="submit"],input[type="submit"]');
if (submit) submit.click();
else if (form && form.requestSubmit) form.requestSubmit();
else passwordInput.dispatchEvent(new KeyboardEvent('keydown', {key: 'Enter', bubbles: true}));
return {submitted: true};
})()`, string(usernameJSON), string(passwordJSON))
}

func browserReadLoginStateScript() string {
	return `(() => {
const text = (value) => value == null ? '' : String(value).trim();
const names = {
  access_token: ['access_token','accessToken','auth_token','authToken','id_token','idToken','token','jwt'],
  refresh_token: ['refresh_token','refreshToken','refresh','rt'],
  user_id: ['id','user_id','userId','uid','sub'],
  expires_at: ['expires_at','token_expires_at','access_expires_at','expiresAt']
};
const nested = (value, wanted, depth = 0) => {
  if (!value || depth > 6) return '';
  if (Array.isArray(value)) {
    for (const item of value) {
      const found = nested(item, wanted, depth + 1);
      if (found) return found;
    }
    return '';
  }
  if (typeof value !== 'object') return '';
  for (const [key, child] of Object.entries(value)) {
    if (wanted.some((name) => key.toLowerCase() === name.toLowerCase()) &&
        child != null && typeof child !== 'object') {
      const found = text(child).replace(/^Bearer\s+/i, '');
      if (found) return found;
    }
    const found = nested(child, wanted, depth + 1);
    if (found) return found;
  }
  return '';
};
const readStorage = () => {
  const values = [];
  for (const storage of [localStorage, sessionStorage]) {
    try {
      for (let i = 0; i < storage.length && i < 128; i += 1) {
        const raw = storage.getItem(storage.key(i));
        if (!raw) continue;
        try { values.push(JSON.parse(raw)); } catch (_) { values.push(raw); }
      }
    } catch (_) {}
  }
  return values;
};
const values = readStorage();
let accessToken = '';
let refreshToken = '';
let userId = '';
let tempToken = '';
let expiresAt = '';
for (const value of values) {
  accessToken ||= nested(value, names.access_token);
  refreshToken ||= nested(value, names.refresh_token);
  userId ||= nested(value, names.user_id);
  tempToken ||= nested(value, ['temp_token','tempToken','flow_token','flowToken','verification_token','verificationToken']);
  expiresAt ||= nested(value, names.expires_at);
}
const body = text(document.body && document.body.innerText).slice(0, 12000).toLowerCase();
const url = text(location.href).toLowerCase();
const otpRequired = !accessToken && /2fa|two.factor|mfa|otp|one.time|verification.code|验证码|二次验证/.test(body);
const captchaRequired = !accessToken && /turnstile|captcha|verify.you.are.human|cloudflare|人机验证|图形验证码/.test(body + ' ' + url);
const invalidCredential = !accessToken && /invalid.credentials|invalid.password|incorrect.password|用户名或密码错误|账号或密码错误/.test(body);
const loginForm = !accessToken && Boolean(document.querySelector('input[type="password"]'));
return {
  access_token: accessToken,
  refresh_token: refreshToken,
  user_id: userId,
  temp_token: tempToken,
  expires_at: expiresAt,
  otp_required: otpRequired,
  manual_captcha_required: captchaRequired,
  invalid_credential: invalidCredential,
  browser_auth_status: accessToken || document.cookie ? 'success' : (loginForm ? 'pending' : 'pending')
};
})()`
}
