// config - config.go
// CPA Manager 使用量采集服务的配置管理模块。
// 支持多层配置来源（优先级从高到低）：
//  1. 环境变量（如 HTTP_ADDR、CPA_UPSTREAM_URL 等）
//  2. JSON 配置文件（默认为可执行文件同目录下的 config.json）
//  3. 程序内置默认值
//
// 配置文件支持相对路径（基于配置文件所在目录解析）。
// 管理密钥支持通过环境变量、文件路径或 Docker Secret 方式提供。
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// configEnvKey 指定自定义配置文件路径的环境变量名称。
// 设置后将覆盖默认的配置文件查找逻辑。
const configEnvKey = "CPA_MANAGER_CONFIG"

// defaultConfigName 默认配置文件名，与可执行文件同目录。
const defaultConfigName = "config.json"

// defaultSecretFile Docker Secret 默认路径，用于读取管理密钥。
const defaultSecretFile = "/run/secrets/cpa_management_key"

// Config 表示应用的最终生效配置（合并了所有来源后的配置）。
// 所有字段都经过环境变量覆盖或默认值填充。
type Config struct {
	HTTPAddr       string        // HTTP 服务监听地址（如 0.0.0.0:18317）
	DBPath         string        // SQLite 数据库文件的完整路径
	CPAUpstreamURL string        // CPA 上游服务的基础 URL
	ManagementKey  string        // CPA 管理接口的认证密钥
	CollectorMode  string        // 采集模式：auto/http/resp/subscribe
	Queue          string        // 使用量队列名称（Redis 队列名）
	PopSide        string        // 队列弹出方向：left（LPOP）或 right（RPOP）
	BatchSize      int           // 每次批量拉取的事件数量
	PollInterval   time.Duration // HTTP/RESP 轮询间隔
	QueryLimit     int           // 查询事件时的最大返回数量
	PanelPath      string        // 管理面板 HTML 文件的外部路径（可选，覆盖内置面板）
	CORSOrigins    []string      // 允许的 CORS 来源列表（"*" 表示允许所有）
	TLSSkipVerify  bool          // 是否跳过 TLS 证书验证
}

// fileConfig 表示从 JSON 配置文件中反序列化的配置结构。
// 使用 json 标签定义字段名，部分字段名与最终 Config 不同（如 dataDir vs DBPath）。
type fileConfig struct {
	HTTPAddr          string   `json:"httpAddr,omitempty"`          // HTTP 监听地址
	DataDir           string   `json:"dataDir,omitempty"`           // 数据目录（DBPath 会基于此目录生成）
	DBPath            string   `json:"dbPath,omitempty"`            // 数据库文件的完整路径
	CPAUpstreamURL    string   `json:"cpaUpstreamUrl,omitempty"`    // CPA 上游 URL
	ManagementKeyFile string   `json:"managementKeyFile,omitempty"` // 管理密钥文件路径
	CollectorMode     string   `json:"collectorMode,omitempty"`     // 采集模式
	Queue             string   `json:"queue,omitempty"`             // 队列名称
	PopSide           string   `json:"popSide,omitempty"`           // 弹出方向
	BatchSize         int      `json:"batchSize,omitempty"`         // 批次大小
	PollIntervalMS    int      `json:"pollIntervalMs,omitempty"`    // 轮询间隔（毫秒）
	QueryLimit        int      `json:"queryLimit,omitempty"`        // 查询限制
	PanelPath         string   `json:"panelPath,omitempty"`         // 面板文件路径
	CORSOrigins       []string `json:"corsOrigins,omitempty"`       // CORS 来源
	TLSSkipVerify     bool     `json:"tlsSkipVerify,omitempty"`     // TLS 跳过验证
}

// Load 从配置文件和环境变量加载并合并配置。
// 加载流程：
// 1. 查找并读取 JSON 配置文件（不存在则创建默认配置）
// 2. 解析配置文件中的相对路径（基于配置文件所在目录）
// 3. 用环境变量覆盖配置文件中的值
// 4. 读取管理密钥（支持环境变量、文件路径、Docker Secret）
// 5. 返回最终生效的 Config 结构
func Load() (Config, error) {
	cfgFile, cfgDir, err := loadFileConfig()
	if err != nil {
		return Config{}, err
	}

	dataDirFallback := "/data"
	if cfgFile.DataDir != "" {
		dataDirFallback = resolveConfigPath(cfgFile.DataDir, cfgDir)
	} else if cfgDir != "" {
		dataDirFallback = resolveConfigPath("./data", cfgDir)
	}
	dataDir := env("USAGE_DATA_DIR", dataDirFallback)

	dbPathFallback := filepath.Join(dataDir, "usage.sqlite")
	if !hasEnv("USAGE_DATA_DIR") && cfgFile.DBPath != "" {
		dbPathFallback = resolveConfigPath(cfgFile.DBPath, cfgDir)
	}

	managementKeyFile := defaultSecretFile
	if cfgFile.ManagementKeyFile != "" {
		managementKeyFile = resolveConfigPath(cfgFile.ManagementKeyFile, cfgDir)
	}

	return Config{
		HTTPAddr:       env("HTTP_ADDR", stringFallback(cfgFile.HTTPAddr, "0.0.0.0:18317")),
		DBPath:         env("USAGE_DB_PATH", dbPathFallback),
		CPAUpstreamURL: env("CPA_UPSTREAM_URL", cfgFile.CPAUpstreamURL),
		ManagementKey:  readSecret("CPA_MANAGEMENT_KEY", "CPA_MANAGEMENT_KEY_FILE", managementKeyFile),
		CollectorMode:  normalizeCollectorMode(env("USAGE_COLLECTOR_MODE", stringFallback(cfgFile.CollectorMode, "auto"))),
		Queue:          env("USAGE_RESP_QUEUE", stringFallback(cfgFile.Queue, "usage")),
		PopSide:        env("USAGE_RESP_POP_SIDE", stringFallback(cfgFile.PopSide, "right")),
		BatchSize:      envInt("USAGE_BATCH_SIZE", intFallback(cfgFile.BatchSize, 100)),
		PollInterval:   time.Duration(envInt("USAGE_POLL_INTERVAL_MS", intFallback(cfgFile.PollIntervalMS, 500))) * time.Millisecond,
		QueryLimit:     envInt("USAGE_QUERY_LIMIT", intFallback(cfgFile.QueryLimit, 50000)),
		PanelPath:      env("PANEL_PATH", resolveConfigPath(cfgFile.PanelPath, cfgDir)),
		CORSOrigins:    splitCSV(env("USAGE_CORS_ORIGINS", strings.Join(sliceFallback(cfgFile.CORSOrigins, []string{"*"}), ","))),
		TLSSkipVerify:  envBool("USAGE_RESP_TLS_SKIP_VERIFY", cfgFile.TLSSkipVerify),
	}, nil
}

// loadFileConfig 查找并加载 JSON 配置文件。
// 优先使用 CPA_MANAGER_CONFIG 环境变量指定的路径；
// 否则在可执行文件同目录下查找 config.json；
// 若文件不存在且未配置关键环境变量，则创建默认配置文件。
func loadFileConfig() (fileConfig, string, error) {
	if configPath := strings.TrimSpace(os.Getenv(configEnvKey)); configPath != "" {
		return readOrCreateFileConfig(configPath)
	}

	configPath, err := executableConfigPath()
	if err != nil {
		return fileConfig{}, "", err
	}
	cfg, cfgDir, ok, err := readFileConfig(configPath)
	if err != nil || ok {
		return cfg, cfgDir, err
	}
	if hasEnv("USAGE_DATA_DIR") || hasEnv("USAGE_DB_PATH") {
		return fileConfig{}, "", nil
	}
	return createDefaultFileConfig(configPath)
}

// readOrCreateFileConfig 尝试读取指定路径的配置文件，不存在则创建默认配置。
func readOrCreateFileConfig(configPath string) (fileConfig, string, error) {
	cfg, cfgDir, ok, err := readFileConfig(configPath)
	if err != nil || ok {
		return cfg, cfgDir, err
	}
	return createDefaultFileConfig(configPath)
}

// readFileConfig 读取并解析 JSON 配置文件。
// 返回值：
//   - cfg: 解析后的文件配置
//   - cfgDir: 配置文件所在目录（用于解析相对路径）
//   - ok: 文件是否存在且解析成功
//   - err: 读取或解析错误
func readFileConfig(configPath string) (fileConfig, string, bool, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fileConfig{}, filepath.Dir(configPath), false, nil
		}
		return fileConfig{}, filepath.Dir(configPath), false, fmt.Errorf("read config %s: %w", configPath, err)
	}
	var cfg fileConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fileConfig{}, filepath.Dir(configPath), false, fmt.Errorf("parse config %s: %w", configPath, err)
	}
	return cfg, filepath.Dir(configPath), true, nil
}

// createDefaultFileConfig 创建默认配置文件。
// 默认配置只包含 HTTP 监听地址和数据目录，写入指定路径。
func createDefaultFileConfig(configPath string) (fileConfig, string, error) {
	cfg := fileConfig{
		HTTPAddr: "0.0.0.0:18317",
		DataDir:  "./data",
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fileConfig{}, "", err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return fileConfig{}, "", fmt.Errorf("create config directory %s: %w", filepath.Dir(configPath), err)
	}
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		return fileConfig{}, "", fmt.Errorf("create default config %s: %w", configPath, err)
	}
	return cfg, filepath.Dir(configPath), nil
}

// executableConfigPath 获取可执行文件同目录下的默认配置文件路径。
func executableConfigPath() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve executable path: %w", err)
	}
	return filepath.Join(filepath.Dir(executable), defaultConfigName), nil
}

// normalizeCollectorMode 规范化采集模式字符串。
// 有效值：auto、http、resp、subscribe。无效值统一返回 "auto"。
func normalizeCollectorMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "http", "resp", "subscribe":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "auto"
	}
}

// hasEnv 检查指定的环境变量是否已设置且非空。
func hasEnv(key string) bool {
	return strings.TrimSpace(os.Getenv(key)) != ""
}

// env 获取环境变量的值，未设置或为空时返回 fallback。
func env(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

// envInt 获取环境变量的整数值，未设置、无效或非正数时返回 fallback。
func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

// envBool 获取环境变量的布尔值。
// 视为 true 的值：1、true、yes、on（不区分大小写）。
// 未设置时返回 fallback。
func envBool(key string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

// stringFallback 返回非空的 value，否则返回 fallback。
func stringFallback(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

// intFallback 返回正整数的 value，否则返回 fallback。
func intFallback(value int, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}

// sliceFallback 返回非空切片的 value，否则返回 fallback。
func sliceFallback(value []string, fallback []string) []string {
	if len(value) == 0 {
		return fallback
	}
	return value
}

// resolveConfigPath 解析相对路径。
// 如果 path 是绝对路径或 baseDir 为空则原样返回，否则将 path 拼接到 baseDir 下。
func resolveConfigPath(path string, baseDir string) string {
	path = strings.TrimSpace(path)
	if path == "" || filepath.IsAbs(path) || baseDir == "" {
		return path
	}
	return filepath.Join(baseDir, path)
}

// splitCSV 将逗号分隔的字符串拆分为切片，每项去除空白。
// 空白项被过滤掉。
func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// readSecret 读取管理密钥，按优先级尝试以下来源：
// 1. 环境变量 envKey
// 2. 环境变量 fileEnvKey 指定的文件路径
// 3. 默认文件路径 defaultFile
// 读取文件时自动去除空白字符。
func readSecret(envKey string, fileEnvKey string, defaultFile string) string {
	if value := strings.TrimSpace(os.Getenv(envKey)); value != "" {
		return value
	}

	path := strings.TrimSpace(os.Getenv(fileEnvKey))
	if path == "" {
		path = defaultFile
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
