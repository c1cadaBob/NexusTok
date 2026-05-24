// Package vertex 实现 Google Vertex AI 渠道的服务账号认证
// 通过 JWT 交换获取 OAuth2 访问令牌
package vertex

import (
	"crypto/rsa"      // RSA 加密
	"crypto/x509"     // X.509 证书解析
	"encoding/json"   // JSON 编解码
	"encoding/pem"    // PEM 格式解析
	"errors"          // 错误处理
	"net/http"        // HTTP 客户端
	"net/url"         // URL 编码
	"strings"         // 字符串操作

	relaycommon "github.com/c1cada/NexusTok/relay/common" // 中继通用类型
	"github.com/c1cada/NexusTok/service"                  // 服务层

	"github.com/bytedance/gopkg/cache/asynccache" // 异步缓存
	"github.com/golang-jwt/jwt/v5"                // JWT 库

	"fmt"  // 格式化输出
	"time" // 时间处理
)

// Credentials Google Cloud 服务账号凭证结构体
type Credentials struct {
	ProjectID    string `json:"project_id"`    // 项目 ID
	PrivateKeyID string `json:"private_key_id"` // 私钥 ID
	PrivateKey   string `json:"private_key"`    // 私钥（PEM 格式）
	ClientEmail  string `json:"client_email"`   // 客户端邮箱
	ClientID     string `json:"client_id"`      // 客户端 ID
}

// Cache 访问令牌缓存
// 使用异步缓存，35 分钟刷新，30 分钟过期
var Cache = asynccache.NewAsyncCache(asynccache.Options{
	RefreshDuration: time.Minute * 35,
	EnableExpire:    true,
	ExpireDuration:  time.Minute * 30,
	Fetcher: func(key string) (interface{}, error) {
		return nil, errors.New("not found")
	},
})

// getAccessToken 获取访问令牌
// 先从缓存获取，缓存未命中则创建新的 JWT 并交换访问令牌
//
// 参数：
//   - a: Vertex AI 适配器
//   - info: 中继信息
//
// 返回值：
//   - string: 访问令牌
//   - error: 获取过程中的错误
func getAccessToken(a *Adaptor, info *relaycommon.RelayInfo) (string, error) {
	var cacheKey string
	if info.ChannelIsMultiKey {
		cacheKey = fmt.Sprintf("access-token-%d-%d", info.ChannelId, info.ChannelMultiKeyIndex)
	} else {
		cacheKey = fmt.Sprintf("access-token-%d", info.ChannelId)
	}
	val, err := Cache.Get(cacheKey)
	if err == nil {
		return val.(string), nil
	}

	// 创建签名 JWT
	signedJWT, err := createSignedJWT(a.AccountCredentials.ClientEmail, a.AccountCredentials.PrivateKey)
	if err != nil {
		return "", fmt.Errorf("failed to create signed JWT: %w", err)
	}
	// 用 JWT 交换访问令牌
	newToken, err := exchangeJwtForAccessToken(signedJWT, info)
	if err != nil {
		return "", fmt.Errorf("failed to exchange JWT for access token: %w", err)
	}
	if err := Cache.SetDefault(cacheKey, newToken); err {
		return newToken, nil
	}
	return newToken, nil
}

// createSignedJWT 创建签名的 JWT
// 使用服务账号的私钥对 JWT 进行 RSA 签名
//
// 参数：
//   - email: 服务账号邮箱
//   - privateKeyPEM: PEM 格式的私钥
//
// 返回值：
//   - string: 签名后的 JWT 字符串
//   - error: 签名过程中的错误
func createSignedJWT(email, privateKeyPEM string) (string, error) {

	// 清理 PEM 格式
	privateKeyPEM = strings.ReplaceAll(privateKeyPEM, "-----BEGIN PRIVATE KEY-----", "")
	privateKeyPEM = strings.ReplaceAll(privateKeyPEM, "-----END PRIVATE KEY-----", "")
	privateKeyPEM = strings.ReplaceAll(privateKeyPEM, "\r", "")
	privateKeyPEM = strings.ReplaceAll(privateKeyPEM, "\n", "")
	privateKeyPEM = strings.ReplaceAll(privateKeyPEM, "\\n", "")

	block, _ := pem.Decode([]byte("-----BEGIN PRIVATE KEY-----\n" + privateKeyPEM + "\n-----END PRIVATE KEY-----"))
	if block == nil {
		return "", fmt.Errorf("failed to parse PEM block containing the private key")
	}

	privateKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return "", err
	}

	rsaPrivateKey, ok := privateKey.(*rsa.PrivateKey)
	if !ok {
		return "", fmt.Errorf("not an RSA private key")
	}

	// 构建 JWT 声明
	now := time.Now()
	claims := jwt.MapClaims{
		"iss":   email,                                          // 签发者
		"scope": "https://www.googleapis.com/auth/cloud-platform", // 权限范围
		"aud":   "https://www.googleapis.com/oauth2/v4/token",   // 受众
		"exp":   now.Add(time.Minute * 35).Unix(),               // 过期时间
		"iat":   now.Unix(),                                      // 签发时间
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signedToken, err := token.SignedString(rsaPrivateKey)
	if err != nil {
		return "", err
	}

	return signedToken, nil
}

// exchangeJwtForAccessToken 用 JWT 交换访问令牌
// 向 Google OAuth2 端点发送 JWT，获取访问令牌
func exchangeJwtForAccessToken(signedJWT string, info *relaycommon.RelayInfo) (string, error) {

	authURL := "https://www.googleapis.com/oauth2/v4/token"
	data := url.Values{}
	data.Set("grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer")
	data.Set("assertion", signedJWT)

	var client *http.Client
	var err error
	if info.ChannelSetting.Proxy != "" {
		client, err = service.NewProxyHttpClient(info.ChannelSetting.Proxy)
		if err != nil {
			return "", fmt.Errorf("new proxy http client failed: %w", err)
		}
	} else {
		client = service.GetHttpClient()
	}

	resp, err := client.PostForm(authURL, data)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if accessToken, ok := result["access_token"].(string); ok {
		return accessToken, nil
	}

	return "", fmt.Errorf("failed to get access token: %v", result)
}

// AcquireAccessToken 获取访问令牌（外部接口）
// 创建 JWT 并交换访问令牌，支持代理配置
//
// 参数：
//   - creds: 服务账号凭证
//   - proxy: 代理地址（可选）
//
// 返回值：
//   - string: 访问令牌
//   - error: 获取过程中的错误
func AcquireAccessToken(creds Credentials, proxy string) (string, error) {
	signedJWT, err := createSignedJWT(creds.ClientEmail, creds.PrivateKey)
	if err != nil {
		return "", fmt.Errorf("failed to create signed JWT: %w", err)
	}
	return exchangeJwtForAccessTokenWithProxy(signedJWT, proxy)
}

// exchangeJwtForAccessTokenWithProxy 用 JWT 交换访问令牌（支持代理）
func exchangeJwtForAccessTokenWithProxy(signedJWT string, proxy string) (string, error) {
	authURL := "https://www.googleapis.com/oauth2/v4/token"
	data := url.Values{}
	data.Set("grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer")
	data.Set("assertion", signedJWT)

	var client *http.Client
	var err error
	if proxy != "" {
		client, err = service.NewProxyHttpClient(proxy)
		if err != nil {
			return "", fmt.Errorf("new proxy http client failed: %w", err)
		}
	} else {
		client = service.GetHttpClient()
	}

	resp, err := client.PostForm(authURL, data)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if accessToken, ok := result["access_token"].(string); ok {
		return accessToken, nil
	}
	return "", fmt.Errorf("failed to get access token: %v", result)
}
