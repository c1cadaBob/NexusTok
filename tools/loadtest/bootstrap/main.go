// Package main 提供压测环境初始化 helper。
//
// 该程序由 scripts/loadtest/bootstrap.sh 调用，负责通过 NexusTok 现有后台 API
// 幂等地完成 Root 初始化、登录、Mock OpenAI 渠道创建和压测 Token 提取。使用 Go
// helper 而不是在 shell 中解析 JSON，是为了避免要求服务器额外安装 jq，同时让 cookie
// 处理和错误提示更稳定。
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/common"
)

const (
	defaultBaseURL      = "http://127.0.0.1:3100"
	defaultRootUser     = "root"
	defaultRootPassword = "LoadtestRoot123!"
	defaultChannelName  = "loadtest-mock-openai"
	defaultTokenName    = "loadtest-token"
	defaultModel        = "gpt-loadtest"
	defaultUpstreamURL  = "http://mock-openai:8080"
	defaultStateDir     = ".loadtest"
)

type bootstrapConfig struct {
	baseURL      string
	username     string
	password     string
	channelName  string
	tokenName    string
	model        string
	upstreamURL  string
	stateDir     string
	timeout      time.Duration
	setupSelfUse bool
}

type apiClient struct {
	baseURL string
	client  *http.Client
}

type apiEnvelope struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type setupData struct {
	Status   bool `json:"status"`
	RootInit bool `json:"root_init"`
}

type channelListData struct {
	Items []channelItem `json:"items"`
	Total int           `json:"total"`
}

type channelItem struct {
	ID      int     `json:"id"`
	Name    string  `json:"name"`
	Type    int     `json:"type"`
	BaseURL *string `json:"base_url"`
	Models  string  `json:"models"`
	Group   string  `json:"group"`
	Status  int     `json:"status"`
}

type tokenListData struct {
	Items []tokenItem `json:"items"`
	Total int         `json:"total"`
}

type tokenItem struct {
	ID             int    `json:"id"`
	Name           string `json:"name"`
	Status         int    `json:"status"`
	Group          string `json:"group"`
	UnlimitedQuota bool   `json:"unlimited_quota"`
}

type tokenKeyData struct {
	Key string `json:"key"`
}

func main() {
	cfg := loadBootstrapConfig()
	if err := runBootstrap(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "loadtest bootstrap failed: %v\n", err)
		os.Exit(1)
	}
}

func loadBootstrapConfig() bootstrapConfig {
	cfg := bootstrapConfig{}
	flag.StringVar(&cfg.baseURL, "base-url", firstNonEmpty(os.Getenv("LOADTEST_BASE_URL"), os.Getenv("BASE_URL"), defaultBaseURL), "NexusTok 入口地址")
	flag.StringVar(&cfg.username, "username", firstNonEmpty(os.Getenv("LOADTEST_ROOT_USERNAME"), defaultRootUser), "Root 用户名")
	flag.StringVar(&cfg.password, "password", firstNonEmpty(os.Getenv("LOADTEST_ROOT_PASSWORD"), defaultRootPassword), "Root 密码")
	flag.StringVar(&cfg.channelName, "channel-name", firstNonEmpty(os.Getenv("LOADTEST_CHANNEL_NAME"), defaultChannelName), "压测渠道名称")
	flag.StringVar(&cfg.tokenName, "token-name", firstNonEmpty(os.Getenv("LOADTEST_TOKEN_NAME"), defaultTokenName), "压测 Token 名称")
	flag.StringVar(&cfg.model, "model", firstNonEmpty(os.Getenv("MODEL"), os.Getenv("LOADTEST_MODEL"), defaultModel), "压测模型名")
	flag.StringVar(&cfg.upstreamURL, "upstream-url", firstNonEmpty(os.Getenv("LOADTEST_UPSTREAM_URL"), defaultUpstreamURL), "Mock upstream 容器内地址")
	flag.StringVar(&cfg.stateDir, "state-dir", firstNonEmpty(os.Getenv("LOADTEST_STATE_DIR"), defaultStateDir), "压测状态文件目录")
	flag.DurationVar(&cfg.timeout, "timeout", 90*time.Second, "等待 NexusTok 可用的最长时间")
	flag.BoolVar(&cfg.setupSelfUse, "self-use", true, "首次 setup 时是否启用自用模式")
	flag.Parse()

	cfg.baseURL = normalizeBaseURL(cfg.baseURL)
	cfg.upstreamURL = strings.TrimRight(strings.TrimSpace(cfg.upstreamURL), "/")
	return cfg
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func normalizeBaseURL(value string) string {
	return strings.TrimRight(strings.TrimSpace(value), "/")
}

func runBootstrap(cfg bootstrapConfig) error {
	if cfg.baseURL == "" {
		return errors.New("base URL 不能为空")
	}
	if cfg.password == "" {
		return errors.New("Root 密码不能为空")
	}
	if err := os.MkdirAll(cfg.stateDir, 0o755); err != nil {
		return fmt.Errorf("创建状态目录失败: %w", err)
	}

	client, err := newAPIClient(cfg.baseURL)
	if err != nil {
		return err
	}

	fmt.Printf("等待 NexusTok 可用: %s\n", cfg.baseURL)
	if err := waitForStatus(client, cfg.timeout); err != nil {
		return err
	}
	if err := ensureSetup(client, cfg); err != nil {
		return err
	}
	if err := login(client, cfg); err != nil {
		return err
	}
	if err := writeCookieFile(client, cfg); err != nil {
		return err
	}
	if _, err := ensureChannel(client, cfg); err != nil {
		return err
	}
	token, err := ensureToken(client, cfg)
	if err != nil {
		return err
	}
	if err := writeStateFiles(cfg, token); err != nil {
		return err
	}

	fmt.Printf("压测环境初始化完成，Token 已写入 %s\n", filepath.Join(cfg.stateDir, "token"))
	return nil
}

func newAPIClient(baseURL string) (*apiClient, error) {
	if _, err := url.ParseRequestURI(baseURL); err != nil {
		return nil, fmt.Errorf("base URL 不合法: %w", err)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("创建 cookie jar 失败: %w", err)
	}
	return &apiClient{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 30 * time.Second,
			Jar:     jar,
		},
	}, nil
}

func waitForStatus(client *apiClient, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		var env apiEnvelope
		lastErr = client.do(http.MethodGet, "/api/status", nil, &env)
		if lastErr == nil && env.Success {
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	if lastErr != nil {
		return fmt.Errorf("等待 /api/status 超时，最后错误: %w", lastErr)
	}
	return errors.New("等待 /api/status 超时")
}

func ensureSetup(client *apiClient, cfg bootstrapConfig) error {
	var setup setupData
	if err := client.doData(http.MethodGet, "/api/setup", nil, &setup); err != nil {
		return fmt.Errorf("读取 setup 状态失败: %w", err)
	}
	if setup.Status {
		fmt.Println("系统已完成初始化，跳过 setup")
		return nil
	}

	payload := map[string]any{
		"username":           cfg.username,
		"password":           cfg.password,
		"confirmPassword":    cfg.password,
		"SelfUseModeEnabled": cfg.setupSelfUse,
		"DemoSiteEnabled":    false,
	}
	var env apiEnvelope
	if err := client.do(http.MethodPost, "/api/setup", payload, &env); err != nil {
		return fmt.Errorf("提交 setup 失败: %w", err)
	}
	if !env.Success {
		return fmt.Errorf("提交 setup 失败: %s", env.Message)
	}
	fmt.Println("系统初始化完成")
	return nil
}

func login(client *apiClient, cfg bootstrapConfig) error {
	payload := map[string]any{
		"username": cfg.username,
		"password": cfg.password,
	}
	var env apiEnvelope
	if err := client.do(http.MethodPost, "/api/user/login", payload, &env); err != nil {
		return fmt.Errorf("Root 登录失败: %w", err)
	}
	if !env.Success {
		return fmt.Errorf("Root 登录失败: %s", env.Message)
	}
	fmt.Println("Root 登录成功")
	return nil
}

func ensureChannel(client *apiClient, cfg bootstrapConfig) (*channelItem, error) {
	channels, err := listChannels(client)
	if err != nil {
		return nil, err
	}
	if existing := findChannel(channels, cfg.channelName); existing != nil {
		fmt.Printf("复用已有压测渠道: id=%d name=%s\n", existing.ID, existing.Name)
		return existing, nil
	}

	payload := map[string]any{
		"mode":           "single",
		"multi_key_mode": "polling",
		"channel": map[string]any{
			"type":         1,
			"name":         cfg.channelName,
			"key":          "mock-key",
			"base_url":     cfg.upstreamURL,
			"models":       cfg.model,
			"group":        "default",
			"status":       1,
			"auto_ban":     0,
			"channel_info": map[string]any{"credential_mode": "single_key"},
		},
	}
	var env apiEnvelope
	if err := client.do(http.MethodPost, "/api/channel/", payload, &env); err != nil {
		return nil, fmt.Errorf("创建压测渠道失败: %w", err)
	}
	if !env.Success {
		return nil, fmt.Errorf("创建压测渠道失败: %s", env.Message)
	}

	channels, err = listChannels(client)
	if err != nil {
		return nil, err
	}
	created := findChannel(channels, cfg.channelName)
	if created == nil {
		return nil, errors.New("创建压测渠道后未能在列表中找到该渠道")
	}
	fmt.Printf("创建压测渠道成功: id=%d name=%s\n", created.ID, created.Name)
	return created, nil
}

func listChannels(client *apiClient) ([]channelItem, error) {
	var data channelListData
	if err := client.doData(http.MethodGet, "/api/channel/?p=1&size=100&type=1&id_sort=true", nil, &data); err != nil {
		return nil, fmt.Errorf("获取渠道列表失败: %w", err)
	}
	return data.Items, nil
}

func findChannel(items []channelItem, name string) *channelItem {
	for i := range items {
		if items[i].Name == name {
			return &items[i]
		}
	}
	return nil
}

func ensureToken(client *apiClient, cfg bootstrapConfig) (string, error) {
	tokens, err := listTokens(client)
	if err != nil {
		return "", err
	}
	item := findToken(tokens, cfg.tokenName)
	if item == nil {
		payload := map[string]any{
			"name":            cfg.tokenName,
			"expired_time":    -1,
			"remain_quota":    0,
			"unlimited_quota": true,
			"group":           "default",
			"status":          1,
		}
		var env apiEnvelope
		if err := client.do(http.MethodPost, "/api/token/", payload, &env); err != nil {
			return "", fmt.Errorf("创建压测 Token 失败: %w", err)
		}
		if !env.Success {
			return "", fmt.Errorf("创建压测 Token 失败: %s", env.Message)
		}
		tokens, err = listTokens(client)
		if err != nil {
			return "", err
		}
		item = findToken(tokens, cfg.tokenName)
		if item == nil {
			return "", errors.New("创建压测 Token 后未能在列表中找到该 Token")
		}
		fmt.Printf("创建压测 Token 成功: id=%d name=%s\n", item.ID, item.Name)
	} else {
		fmt.Printf("复用已有压测 Token: id=%d name=%s\n", item.ID, item.Name)
	}

	var key tokenKeyData
	if err := client.doData(http.MethodPost, fmt.Sprintf("/api/token/%d/key", item.ID), nil, &key); err != nil {
		return "", fmt.Errorf("获取压测 Token 原始密钥失败: %w", err)
	}
	if strings.TrimSpace(key.Key) == "" {
		return "", errors.New("获取到的压测 Token 为空")
	}
	return key.Key, nil
}

func listTokens(client *apiClient) ([]tokenItem, error) {
	var data tokenListData
	if err := client.doData(http.MethodGet, "/api/token/?p=1&size=100", nil, &data); err != nil {
		return nil, fmt.Errorf("获取 Token 列表失败: %w", err)
	}
	return data.Items, nil
}

func findToken(items []tokenItem, name string) *tokenItem {
	for i := range items {
		if items[i].Name == name {
			return &items[i]
		}
	}
	return nil
}

func writeCookieFile(client *apiClient, cfg bootstrapConfig) error {
	cookieHeader, err := client.cookieHeader()
	if err != nil {
		return err
	}
	if cookieHeader == "" {
		return errors.New("登录后未获取到 session cookie")
	}
	path := filepath.Join(cfg.stateDir, "cookie")
	if err := os.WriteFile(path, []byte(cookieHeader+"\n"), 0o600); err != nil {
		return fmt.Errorf("写入 cookie 文件失败: %w", err)
	}
	return nil
}

func writeStateFiles(cfg bootstrapConfig, token string) error {
	files := map[string]string{
		"token":    token + "\n",
		"base_url": cfg.baseURL + "\n",
		"model":    cfg.model + "\n",
	}
	for name, value := range files {
		if err := os.WriteFile(filepath.Join(cfg.stateDir, name), []byte(value), 0o600); err != nil {
			return fmt.Errorf("写入 %s 失败: %w", name, err)
		}
	}
	return nil
}

func (c *apiClient) doData(method string, path string, payload any, data any) error {
	var env apiEnvelope
	if err := c.do(method, path, payload, &env); err != nil {
		return err
	}
	if !env.Success {
		return errors.New(env.Message)
	}
	if data == nil {
		return nil
	}
	if len(env.Data) == 0 || string(env.Data) == "null" {
		return errors.New("响应 data 为空")
	}
	if err := common.Unmarshal(env.Data, data); err != nil {
		return fmt.Errorf("解析响应 data 失败: %w", err)
	}
	return nil
}

func (c *apiClient) do(method string, path string, payload any, out *apiEnvelope) error {
	var body *bytes.Reader
	if payload != nil {
		data, err := common.Marshal(payload)
		if err != nil {
			return fmt.Errorf("编码请求体失败: %w", err)
		}
		body = bytes.NewReader(data)
	} else {
		body = bytes.NewReader(nil)
	}

	req, err := http.NewRequest(method, c.baseURL+path, body)
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s 返回 HTTP %d", method, path, resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	if err := common.DecodeJson(resp.Body, out); err != nil {
		return fmt.Errorf("解析 API 响应失败: %w", err)
	}
	return nil
}

func (c *apiClient) cookieHeader() (string, error) {
	parsed, err := url.Parse(c.baseURL)
	if err != nil {
		return "", fmt.Errorf("解析 base URL 失败: %w", err)
	}
	cookies := c.client.Jar.Cookies(parsed)
	parts := make([]string, 0, len(cookies))
	for _, cookie := range cookies {
		parts = append(parts, cookie.Name+"="+cookie.Value)
	}
	return strings.Join(parts, "; "), nil
}
