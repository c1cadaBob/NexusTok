// Package main - fetch_antigravity_models.go
// 该命令行工具用于从 Antigravity API 动态获取可用的模型列表，
// 并将结果保存为 JSON 文件，供离线检查或静态模型定义使用。
//
// 用法:
//
//	go run ./cmd/fetch_antigravity_models [flags]
//
// 参数:
//
//	--auths-dir <path>  存放认证 JSON 文件的目录（默认: "auths"）
//	--output    <path>  输出 JSON 文件路径（默认: "antigravity_models.json"）
//	--pretty            是否格式化输出 JSON（默认: true）
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/logging"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/misc"
	sdkauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/auth"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/proxyutil"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

// Antigravity API 的基础 URL 常量，包含生产环境、日常环境和沙箱环境的地址。
// 模型列表的请求路径为 /v1internal:fetchAvailableModels。
const (
	// antigravityBaseURLDaily 是 Antigravity 日常云代码服务的 API 基础地址。
	antigravityBaseURLDaily = "https://daily-cloudcode-pa.googleapis.com"
	// antigravitySandboxBaseURLDaily 是 Antigravity 沙箱环境的日常云代码服务 API 基础地址。
	antigravitySandboxBaseURLDaily = "https://daily-cloudcode-pa.sandbox.googleapis.com"
	// antigravityBaseURLProd 是 Antigravity 生产环境的云代码服务 API 基础地址。
	antigravityBaseURLProd = "https://cloudcode-pa.googleapis.com"
	// antigravityModelsPath 是获取可用模型列表的 API 路径。
	antigravityModelsPath = "/v1internal:fetchAvailableModels"
)

// init 初始化共享日志记录器，并设置日志级别为 Info。
func init() {
	logging.SetupBaseLogger()
	log.SetLevel(log.InfoLevel)
}

// modelOutput 是模型列表的输出包装结构，包含获取到的模型数组及元数据。
type modelOutput struct {
	// Models 是从 Antigravity API 获取的模型条目列表。
	Models []modelEntry `json:"models"`
}

// modelEntry 包含静态模型定义所需的精简字段信息。
type modelEntry struct {
	// ID 是模型的唯一标识符。
	ID string `json:"id"`
	// Object 是对象类型，固定为 "model"。
	Object string `json:"object"`
	// OwnedBy 是模型的拥有者/提供者标识。
	OwnedBy string `json:"owned_by"`
	// Type 是模型的类型分类。
	Type string `json:"type"`
	// DisplayName 是模型的显示名称。
	DisplayName string `json:"display_name"`
	// Name 是模型的内部名称。
	Name string `json:"name"`
	// Description 是模型的描述信息。
	Description string `json:"description"`
	// ContextLength 是模型支持的最大上下文长度（token 数）。
	ContextLength int `json:"context_length,omitempty"`
	// MaxCompletionTokens 是模型支持的最大输出 token 数。
	MaxCompletionTokens int `json:"max_completion_tokens,omitempty"`
}

// main 是程序入口函数，负责解析命令行参数、加载认证文件、
// 调用 Antigravity API 获取模型列表，并将结果保存为 JSON 文件。
func main() {
	// 命令行参数变量。
	var authsDir string
	var outputPath string
	var pretty bool

	flag.StringVar(&authsDir, "auths-dir", "auths", "Directory containing auth JSON files")
	flag.StringVar(&outputPath, "output", "antigravity_models.json", "Output JSON file path")
	flag.BoolVar(&pretty, "pretty", true, "Pretty-print the output JSON")
	flag.Parse()

	// Resolve relative paths against the working directory.
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot get working directory: %v\n", err)
		os.Exit(1)
	}
	if !filepath.IsAbs(authsDir) {
		authsDir = filepath.Join(wd, authsDir)
	}
	if !filepath.IsAbs(outputPath) {
		outputPath = filepath.Join(wd, outputPath)
	}

	fmt.Printf("Scanning auth files in: %s\n", authsDir)

	// Load all auth records from the directory.
	fileStore := sdkauth.NewFileTokenStore()
	fileStore.SetBaseDir(authsDir)

	ctx := context.Background()
	auths, err := fileStore.List(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to list auth files: %v\n", err)
		os.Exit(1)
	}
	if len(auths) == 0 {
		fmt.Fprintf(os.Stderr, "error: no auth files found in %s\n", authsDir)
		os.Exit(1)
	}

	// Find the first enabled antigravity auth.
	var chosen *coreauth.Auth
	for _, a := range auths {
		if a == nil || a.Disabled {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(a.Provider), "antigravity") {
			chosen = a
			break
		}
	}
	if chosen == nil {
		fmt.Fprintf(os.Stderr, "error: no enabled antigravity auth found in %s\n", authsDir)
		os.Exit(1)
	}

	fmt.Printf("Using auth: id=%s label=%s\n", chosen.ID, chosen.Label)

	// Fetch models from the upstream Antigravity API.
	fmt.Println("Fetching Antigravity model list from upstream...")

	fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	models := fetchModels(fetchCtx, chosen)
	if len(models) == 0 {
		fmt.Fprintln(os.Stderr, "warning: no models returned (API may be unavailable or token expired)")
	} else {
		fmt.Printf("Fetched %d models.\n", len(models))
	}

	// Build the output payload.
	out := modelOutput{
		Models: models,
	}

	// Marshal to JSON.
	var raw []byte
	if pretty {
		raw, err = json.MarshalIndent(out, "", "  ")
	} else {
		raw, err = json.Marshal(out)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to marshal JSON: %v\n", err)
		os.Exit(1)
	}

	if err = os.WriteFile(outputPath, raw, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to write output file %s: %v\n", outputPath, err)
		os.Exit(1)
	}

	fmt.Printf("Model list saved to: %s\n", outputPath)
}

// fetchModels 从 Antigravity API 获取可用模型列表。
// 它会依次尝试生产环境、日常环境和沙箱环境的 API 地址。
// 使用认证信息中的 access_token 进行身份验证。
//
// 参数:
//   - ctx: 上下文，用于控制请求超时
//   - auth: 认证信息，包含 access_token 和可选的 project_id
//
// 返回值:
//   - []modelEntry: 获取到的模型条目列表，失败时返回 nil
func fetchModels(ctx context.Context, auth *coreauth.Auth) []modelEntry {
	accessToken := metaStringValue(auth.Metadata, "access_token")
	if accessToken == "" {
		fmt.Fprintln(os.Stderr, "error: no access token found in auth")
		return nil
	}

	baseURLs := []string{antigravityBaseURLProd, antigravityBaseURLDaily, antigravitySandboxBaseURLDaily}

	for _, baseURL := range baseURLs {
		modelsURL := baseURL + antigravityModelsPath

		var payload []byte
		if auth != nil && auth.Metadata != nil {
			if pid, ok := auth.Metadata["project_id"].(string); ok && strings.TrimSpace(pid) != "" {
				payload = []byte(fmt.Sprintf(`{"project": "%s"}`, strings.TrimSpace(pid)))
			}
		}
		if len(payload) == 0 {
			payload = []byte(`{}`)
		}

		httpReq, errReq := http.NewRequestWithContext(ctx, http.MethodPost, modelsURL, strings.NewReader(string(payload)))
		if errReq != nil {
			continue
		}
		httpReq.Close = true
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", "Bearer "+accessToken)
		httpReq.Header.Set("User-Agent", misc.AntigravityUserAgent())

		httpClient := &http.Client{Timeout: 30 * time.Second}
		if transport, _, errProxy := proxyutil.BuildHTTPTransport(auth.ProxyURL); errProxy == nil && transport != nil {
			httpClient.Transport = transport
		}
		httpResp, errDo := httpClient.Do(httpReq)
		if errDo != nil {
			continue
		}

		bodyBytes, errRead := io.ReadAll(httpResp.Body)
		httpResp.Body.Close()
		if errRead != nil {
			continue
		}

		if httpResp.StatusCode < http.StatusOK || httpResp.StatusCode >= http.StatusMultipleChoices {
			continue
		}

		result := gjson.GetBytes(bodyBytes, "models")
		if !result.Exists() {
			continue
		}

		var models []modelEntry

		for originalName, modelData := range result.Map() {
			modelID := strings.TrimSpace(originalName)
			if modelID == "" {
				continue
			}
			// Skip internal/experimental models
			switch modelID {
			case "chat_20706", "chat_23310", "tab_flash_lite_preview", "tab_jump_flash_lite_preview", "gemini-2.5-flash-thinking", "gemini-2.5-pro":
				continue
			}

			displayName := modelData.Get("displayName").String()
			if displayName == "" {
				displayName = modelID
			}

			entry := modelEntry{
				ID:          modelID,
				Object:      "model",
				OwnedBy:     "antigravity",
				Type:        "antigravity",
				DisplayName: displayName,
				Name:        modelID,
				Description: displayName,
			}

			if maxTok := modelData.Get("maxTokens").Int(); maxTok > 0 {
				entry.ContextLength = int(maxTok)
			}
			if maxOut := modelData.Get("maxOutputTokens").Int(); maxOut > 0 {
				entry.MaxCompletionTokens = int(maxOut)
			}

			models = append(models, entry)
		}

		return models
	}

	return nil
}

// metaStringValue 从元数据 map 中提取指定键的字符串值。
// 如果 map 为 nil 或键不存在，返回空字符串。
//
// 参数:
//   - m: 元数据 map
//   - key: 要查找的键名
//
// 返回值:
//   - string: 键对应的字符串值，不存在时返回空字符串
func metaStringValue(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	default:
		return ""
	}
}
