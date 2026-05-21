package accountauth

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
)

const (
	codexProvider                = "codex"
	codexOAuthClientID           = "app_EMoamEEZ73f0CkXaXp7hrann"
	codexOAuthAuthorizeURL       = "https://auth.openai.com/oauth/authorize"
	codexOAuthTokenURL           = "https://auth.openai.com/oauth/token"
	codexOAuthRedirectURI        = "http://localhost:1455/auth/callback"
	codexDeviceRedirectURI       = "https://auth.openai.com/deviceauth/callback"
	codexDeviceUserCodeURL       = "https://auth.openai.com/api/accounts/deviceauth/usercode"
	codexDeviceTokenURL          = "https://auth.openai.com/api/accounts/deviceauth/token"
	codexDeviceVerificationURL   = "https://auth.openai.com/codex/device"
	codexOAuthScope              = "openid profile email offline_access"
	codexJWTClaimPath            = "https://api.openai.com/auth"
	codexDeviceDefaultInterval   = 5 * time.Second
	codexDeviceSessionExpiration = 15 * time.Minute
)

type CodexProvider struct{}

type codexOAuthKey struct {
	IDToken      string `json:"id_token,omitempty"`
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	AccountID    string `json:"account_id,omitempty"`
	LastRefresh  string `json:"last_refresh,omitempty"`
	Email        string `json:"email,omitempty"`
	Type         string `json:"type,omitempty"`
	Expired      string `json:"expired,omitempty"`
}

type codexTokenResult struct {
	IDToken      string
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
}

func init() {
	RegisterProvider(&CodexProvider{})
}

func (p *CodexProvider) Name() string {
	return codexProvider
}

func (p *CodexProvider) DisplayName() string {
	return "Codex"
}

func (p *CodexProvider) SupportsOAuth() bool {
	return true
}

func (p *CodexProvider) SupportsDevice() bool {
	return true
}

func (p *CodexProvider) RefreshLead() *time.Duration {
	lead := 5 * 24 * time.Hour
	return &lead
}

func (p *CodexProvider) StartOAuth(ctx context.Context, group *model.AccountPoolGroup, req LoginStartRequest) (*LoginStartResult, error) {
	_ = ctx
	_ = group
	state, err := randomHex(16)
	if err != nil {
		return nil, err
	}
	verifier, challenge, err := generateCodexPKCEPair()
	if err != nil {
		return nil, err
	}
	authorizeURL, err := buildCodexAuthorizeURL(state, challenge)
	if err != nil {
		return nil, err
	}
	session, err := SaveLoginSession(&LoginSession{
		Provider:     p.Name(),
		Mode:         "oauth",
		PoolGroupID:  req.PoolGroupID,
		Name:         strings.TrimSpace(req.Name),
		Options:      req.Options,
		State:        state,
		Verifier:     verifier,
		Challenge:    challenge,
		AuthorizeURL: authorizeURL,
		ExpiresAt:    time.Now().Add(defaultLoginSessionTTL),
	})
	if err != nil {
		return nil, err
	}
	return &LoginStartResult{
		SessionID:    session.SessionID,
		Provider:     p.Name(),
		Mode:         "oauth",
		AuthorizeURL: authorizeURL,
		ExpiresAt:    session.ExpiresAt.Unix(),
	}, nil
}

func (p *CodexProvider) CompleteOAuth(ctx context.Context, group *model.AccountPoolGroup, req LoginCompleteRequest) (*AccountCredential, error) {
	_ = group
	code, state, err := parseOAuthCallbackInput(req.Input)
	if err != nil {
		return nil, err
	}
	session, ok := GetLoginSession(req.SessionID)
	if !ok {
		session, ok = FindOAuthLoginSession(p.Name(), req.PoolGroupID, state)
	}
	if !ok || session == nil {
		return nil, fmt.Errorf("oauth flow not started or session expired")
	}
	if session.Status == LoginSessionCancelled {
		return nil, fmt.Errorf("oauth flow cancelled")
	}
	if state != session.State {
		return nil, fmt.Errorf("state mismatch")
	}
	proxy := strings.TrimSpace(req.Options.Proxy)
	if proxy == "" {
		proxy = strings.TrimSpace(session.Options.Proxy)
	}
	token, err := exchangeCodexAuthorizationCode(ctx, code, session.Verifier, codexOAuthRedirectURI, proxy)
	if err != nil {
		session.Status = LoginSessionFailed
		session.StatusMessage = err.Error()
		UpdateLoginSession(session)
		return nil, err
	}
	credential, err := p.buildCredential(req.Name, proxy, token)
	if err != nil {
		return nil, err
	}
	session.Status = LoginSessionCompleted
	session.Account = credential
	UpdateLoginSession(session)
	return credential, nil
}

func (p *CodexProvider) StartDevice(ctx context.Context, group *model.AccountPoolGroup, req LoginStartRequest) (*LoginStartResult, error) {
	_ = group
	proxyURL := strings.TrimSpace(req.Options.Proxy)
	client, err := httpClientWithProxy(proxyURL)
	if err != nil {
		return nil, err
	}
	userCode, err := requestCodexDeviceUserCode(ctx, client)
	if err != nil {
		return nil, err
	}
	userCodeText := strings.TrimSpace(userCode.UserCode)
	if userCodeText == "" {
		userCodeText = strings.TrimSpace(userCode.UserCodeAlt)
	}
	if strings.TrimSpace(userCode.DeviceAuthID) == "" || userCodeText == "" {
		return nil, fmt.Errorf("codex device flow did not return required fields")
	}
	interval := parseCodexDevicePollInterval(userCode.Interval)
	session, err := SaveLoginSession(&LoginSession{
		Provider:        p.Name(),
		Mode:            "device",
		PoolGroupID:     req.PoolGroupID,
		Name:            strings.TrimSpace(req.Name),
		Options:         req.Options,
		DeviceAuthID:    strings.TrimSpace(userCode.DeviceAuthID),
		UserCode:        userCodeText,
		VerificationURL: codexDeviceVerificationURL,
		ExpiresAt:       time.Now().Add(codexDeviceSessionExpiration),
		PollInterval:    interval,
	})
	if err != nil {
		return nil, err
	}
	return &LoginStartResult{
		SessionID:       session.SessionID,
		Provider:        p.Name(),
		Mode:            "device",
		VerificationURL: codexDeviceVerificationURL,
		UserCode:        userCodeText,
		ExpiresAt:       session.ExpiresAt.Unix(),
		PollInterval:    int64(interval.Seconds()),
	}, nil
}

func (p *CodexProvider) CompleteDevice(ctx context.Context, group *model.AccountPoolGroup, req LoginCompleteRequest) (*AccountCredential, error) {
	_ = group
	session, ok := GetLoginSession(req.SessionID)
	if !ok || session == nil {
		return nil, fmt.Errorf("device flow not started or session expired")
	}
	if session.Provider != p.Name() || session.Mode != "device" {
		return nil, fmt.Errorf("login session is not a codex device flow")
	}
	if session.Status == LoginSessionCancelled {
		return nil, fmt.Errorf("device flow cancelled")
	}
	proxyURL := strings.TrimSpace(req.Options.Proxy)
	if proxyURL == "" {
		proxyURL = strings.TrimSpace(session.Options.Proxy)
	}
	client, err := httpClientWithProxy(proxyURL)
	if err != nil {
		return nil, err
	}
	deviceToken, err := pollCodexDeviceToken(ctx, client, session.DeviceAuthID, session.UserCode, session.PollInterval, session.ExpiresAt)
	if err != nil {
		session.Status = LoginSessionFailed
		session.StatusMessage = err.Error()
		UpdateLoginSession(session)
		return nil, err
	}
	token, err := exchangeCodexAuthorizationCodeWithPKCE(ctx, deviceToken.AuthorizationCode, deviceToken.CodeVerifier, codexDeviceRedirectURI, proxyURL)
	if err != nil {
		session.Status = LoginSessionFailed
		session.StatusMessage = err.Error()
		UpdateLoginSession(session)
		return nil, err
	}
	credential, err := p.buildCredential(req.Name, proxyURL, token)
	if err != nil {
		return nil, err
	}
	session.Status = LoginSessionCompleted
	session.Account = credential
	UpdateLoginSession(session)
	return credential, nil
}

func (p *CodexProvider) Refresh(ctx context.Context, account *model.PoolAccount) (*AccountCredential, error) {
	if account == nil {
		return nil, fmt.Errorf("account is required")
	}
	raw, err := account.GetDecryptedCredentials()
	if err != nil {
		return nil, err
	}
	var oauthKey codexOAuthKey
	if err := common.UnmarshalJsonStr(raw, &oauthKey); err != nil {
		return nil, fmt.Errorf("codex oauth credential is invalid")
	}
	if strings.TrimSpace(oauthKey.RefreshToken) == "" {
		return nil, fmt.Errorf("refresh_token is required")
	}
	token, err := refreshCodexOAuthToken(ctx, oauthKey.RefreshToken, account.Proxy)
	if err != nil {
		return nil, err
	}
	return p.buildCredential(account.GetCredentialLabel(), account.Proxy, token)
}

func (p *CodexProvider) BuildChannelKey(account *model.PoolAccount) (string, error) {
	if account == nil {
		return "", fmt.Errorf("account is required")
	}
	raw, err := account.GetDecryptedCredentials()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(raw) == "" {
		return "", fmt.Errorf("codex account credential is empty")
	}
	return raw, nil
}

func (p *CodexProvider) Summarize(raw string) string {
	return model.NormalizeAccountPoolCredentialSummary(raw)
}

func (p *CodexProvider) buildCredential(name string, proxy string, token *codexTokenResult) (*AccountCredential, error) {
	if token == nil {
		return nil, fmt.Errorf("token result is empty")
	}
	accountID, ok := extractCodexAccountIDFromJWT(token.AccessToken)
	if !ok {
		return nil, fmt.Errorf("failed to extract account_id from access_token")
	}
	email, _ := extractEmailFromJWT(token.AccessToken)
	now := time.Now()
	oauthKey := codexOAuthKey{
		IDToken:      token.IDToken,
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		AccountID:    accountID,
		LastRefresh:  now.Format(time.RFC3339),
		Expired:      token.ExpiresAt.Format(time.RFC3339),
		Email:        email,
		Type:         codexProvider,
	}
	data, err := common.Marshal(oauthKey)
	if err != nil {
		return nil, err
	}
	label := strings.TrimSpace(name)
	if label == "" {
		label = email
	}
	if label == "" {
		label = accountID
	}
	metadata := map[string]any{
		"email":      email,
		"account_id": accountID,
		"expired":    token.ExpiresAt.Format(time.RFC3339),
	}
	attrs := map[string]string{
		"account_id": accountID,
	}
	return &AccountCredential{
		Provider:        codexProvider,
		AuthType:        model.AccountPoolAuthTypeOfficialOAuth,
		Label:           label,
		Credentials:     string(data),
		Summary:         model.NormalizeAccountPoolCredentialSummary(string(data)),
		Metadata:        metadata,
		Attributes:      attrs,
		ExpiresAt:       token.ExpiresAt,
		LastRefreshedAt: now,
		NextRefreshAt:   nextCodexRefreshAt(now, token.ExpiresAt),
	}, nil
}

func nextCodexRefreshAt(now time.Time, expiresAt time.Time) time.Time {
	if expiresAt.IsZero() {
		return now.Add(10 * time.Minute)
	}
	next := expiresAt.Add(-5 * time.Minute)
	minNext := now.Add(time.Minute)
	if next.Before(minNext) {
		return minNext
	}
	return next
}

type codexDeviceUserCodeResponse struct {
	DeviceAuthID string `json:"device_auth_id"`
	UserCode     string `json:"user_code"`
	UserCodeAlt  string `json:"usercode"`
	Interval     any    `json:"interval"`
}

type codexDeviceTokenResponse struct {
	AuthorizationCode string `json:"authorization_code"`
	CodeVerifier      string `json:"code_verifier"`
	CodeChallenge     string `json:"code_challenge"`
}

func requestCodexDeviceUserCode(ctx context.Context, client *http.Client) (*codexDeviceUserCodeResponse, error) {
	body, err := common.Marshal(map[string]string{"client_id": codexOAuthClientID})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, codexDeviceUserCodeURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to request codex device code: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("codex device code request failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	var parsed codexDeviceUserCodeResponse
	if err := common.Unmarshal(respBody, &parsed); err != nil {
		return nil, err
	}
	return &parsed, nil
}

func pollCodexDeviceToken(ctx context.Context, client *http.Client, deviceAuthID string, userCode string, interval time.Duration, expiresAt time.Time) (*codexDeviceTokenResponse, error) {
	if interval <= 0 {
		interval = codexDeviceDefaultInterval
	}
	if expiresAt.IsZero() {
		expiresAt = time.Now().Add(codexDeviceSessionExpiration)
	}
	for {
		if time.Now().After(expiresAt) {
			return nil, fmt.Errorf("codex device authentication timed out")
		}
		body, err := common.Marshal(map[string]string{
			"device_auth_id": deviceAuthID,
			"user_code":      userCode,
		})
		if err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, codexDeviceTokenURL, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to poll codex device token: %w", err)
		}
		respBody, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			var parsed codexDeviceTokenResponse
			if err := common.Unmarshal(respBody, &parsed); err != nil {
				return nil, err
			}
			if strings.TrimSpace(parsed.AuthorizationCode) == "" || strings.TrimSpace(parsed.CodeVerifier) == "" {
				return nil, fmt.Errorf("codex device token response missing required fields")
			}
			return &parsed, nil
		}
		if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusNotFound {
			return nil, fmt.Errorf("codex device token polling failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
	}
}

func exchangeCodexAuthorizationCode(ctx context.Context, code string, verifier string, redirectURI string, proxyURL string) (*codexTokenResult, error) {
	return exchangeCodexAuthorizationCodeWithPKCE(ctx, code, verifier, redirectURI, proxyURL)
}

func exchangeCodexAuthorizationCodeWithPKCE(ctx context.Context, code string, verifier string, redirectURI string, proxyURL string) (*codexTokenResult, error) {
	if strings.TrimSpace(code) == "" {
		return nil, errors.New("empty authorization code")
	}
	if strings.TrimSpace(verifier) == "" {
		return nil, errors.New("empty code_verifier")
	}
	client, err := httpClientWithProxy(proxyURL)
	if err != nil {
		return nil, err
	}
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", codexOAuthClientID)
	form.Set("code", strings.TrimSpace(code))
	form.Set("code_verifier", strings.TrimSpace(verifier))
	form.Set("redirect_uri", strings.TrimSpace(redirectURI))
	return requestCodexToken(ctx, client, form, "codex oauth code exchange failed")
}

func refreshCodexOAuthToken(ctx context.Context, refreshToken string, proxyURL string) (*codexTokenResult, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return nil, errors.New("empty refresh_token")
	}
	client, err := httpClientWithProxy(proxyURL)
	if err != nil {
		return nil, err
	}
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", strings.TrimSpace(refreshToken))
	form.Set("client_id", codexOAuthClientID)
	form.Set("scope", "openid profile email")
	return requestCodexToken(ctx, client, form, "codex oauth refresh failed")
}

func requestCodexToken(ctx context.Context, client *http.Client, form url.Values, errorPrefix string) (*codexTokenResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, codexOAuthTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var payload struct {
		IDToken      string `json:"id_token"`
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := common.DecodeJson(resp.Body, &payload); err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s: status=%d", errorPrefix, resp.StatusCode)
	}
	if strings.TrimSpace(payload.AccessToken) == "" || strings.TrimSpace(payload.RefreshToken) == "" || payload.ExpiresIn <= 0 {
		return nil, errors.New("codex oauth token response missing fields")
	}
	return &codexTokenResult{
		IDToken:      strings.TrimSpace(payload.IDToken),
		AccessToken:  strings.TrimSpace(payload.AccessToken),
		RefreshToken: strings.TrimSpace(payload.RefreshToken),
		ExpiresAt:    time.Now().Add(time.Duration(payload.ExpiresIn) * time.Second),
	}, nil
}

func buildCodexAuthorizeURL(state string, challenge string) (string, error) {
	u, err := url.Parse(codexOAuthAuthorizeURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", codexOAuthClientID)
	q.Set("redirect_uri", codexOAuthRedirectURI)
	q.Set("scope", codexOAuthScope)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("state", state)
	q.Set("id_token_add_organizations", "true")
	q.Set("codex_cli_simplified_flow", "true")
	q.Set("originator", "codex_cli_rs")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func generateCodexPKCEPair() (string, string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	verifier := base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

func parseOAuthCallbackInput(input string) (string, string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", fmt.Errorf("callback input is required")
	}
	if strings.Contains(input, "://") {
		u, err := url.Parse(input)
		if err != nil {
			return "", "", err
		}
		code := strings.TrimSpace(u.Query().Get("code"))
		state := strings.TrimSpace(u.Query().Get("state"))
		if code == "" || state == "" {
			return "", "", fmt.Errorf("callback url missing code or state")
		}
		return code, state, nil
	}
	values, err := url.ParseQuery(input)
	if err == nil {
		code := strings.TrimSpace(values.Get("code"))
		state := strings.TrimSpace(values.Get("state"))
		if code != "" && state != "" {
			return code, state, nil
		}
	}
	return "", "", fmt.Errorf("callback input missing code or state")
}

func parseCodexDevicePollInterval(raw any) time.Duration {
	switch value := raw.(type) {
	case string:
		seconds, err := strconv.Atoi(strings.TrimSpace(value))
		if err == nil && seconds > 0 {
			return time.Duration(seconds) * time.Second
		}
	case float64:
		if value > 0 {
			return time.Duration(value) * time.Second
		}
	case int:
		if value > 0 {
			return time.Duration(value) * time.Second
		}
	}
	return codexDeviceDefaultInterval
}

func extractCodexAccountIDFromJWT(token string) (string, bool) {
	claims, ok := decodeJWTClaims(token)
	if !ok {
		return "", false
	}
	raw, ok := claims[codexJWTClaimPath]
	if !ok {
		return "", false
	}
	obj, ok := raw.(map[string]any)
	if !ok {
		return "", false
	}
	value, ok := obj["chatgpt_account_id"].(string)
	if !ok {
		return "", false
	}
	value = strings.TrimSpace(value)
	return value, value != ""
}

func extractEmailFromJWT(token string) (string, bool) {
	claims, ok := decodeJWTClaims(token)
	if !ok {
		return "", false
	}
	value, ok := claims["email"].(string)
	if !ok {
		return "", false
	}
	value = strings.TrimSpace(value)
	return value, value != ""
}

func decodeJWTClaims(token string) (map[string]any, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, false
	}
	payloadRaw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, false
	}
	var claims map[string]any
	if err := common.Unmarshal(payloadRaw, &claims); err != nil {
		return nil, false
	}
	return claims, true
}
