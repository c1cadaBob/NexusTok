// 包 misc - oauth.go
// 该文件提供了 OAuth2 流程的辅助功能。
// 包括随机状态生成、回调参数解析和异步提示等。
package misc

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
)

// GenerateRandomState 生成加密安全的随机 state 参数，用于防止 OAuth2 流程中的 CSRF 攻击。
//
// 返回：
//   - string: 十六进制编码的随机 state 字符串
//   - error: 随机生成失败时返回错误
func GenerateRandomState() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

// OAuthCallback 封装了解析后的 OAuth 回调参数。
type OAuthCallback struct {
	// Code 是授权码
	Code string
	// State 是防 CSRF 的 state 参数
	State string
	// Error 是 OAuth 错误码
	Error string
	// ErrorDescription 是 OAuth 错误的详细描述
	ErrorDescription string
}

// AsyncPrompt 在 goroutine 中异步运行提示函数，返回结果通道。
// 返回的通道是缓冲的（大小为 1），即使调用方放弃通道，goroutine 也能完成。
//
// 参数：
//   - promptFn: 提示函数
//   - message: 提示消息
//
// 返回：
//   - <-chan string: 输入结果通道
//   - <-chan error: 错误通道
func AsyncPrompt(promptFn func(string) (string, error), message string) (<-chan string, <-chan error) {
	inputCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		input, err := promptFn(message)
		if err != nil {
			errCh <- err
			return
		}
		inputCh <- input
	}()
	return inputCh, errCh
}

// ParseOAuthCallback 从回调 URL 中提取 OAuth 参数。
// 支持完整 URL、查询字符串、片段等多种输入格式。
// 输入为空时返回 nil。
//
// 参数：
//   - input: OAuth 回调 URL 或查询字符串
//
// 返回：
//   - *OAuthCallback: 解析后的回调参数
//   - error: 解析失败时返回错误
func ParseOAuthCallback(input string) (*OAuthCallback, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return nil, nil
	}

	candidate := trimmed
	if !strings.Contains(candidate, "://") {
		if strings.HasPrefix(candidate, "?") {
			candidate = "http://localhost" + candidate
		} else if strings.ContainsAny(candidate, "/?#") || strings.Contains(candidate, ":") {
			candidate = "http://" + candidate
		} else if strings.Contains(candidate, "=") {
			candidate = "http://localhost/?" + candidate
		} else {
			return nil, fmt.Errorf("invalid callback URL")
		}
	}

	parsedURL, err := url.Parse(candidate)
	if err != nil {
		return nil, err
	}

	query := parsedURL.Query()
	code := strings.TrimSpace(query.Get("code"))
	state := strings.TrimSpace(query.Get("state"))
	errCode := strings.TrimSpace(query.Get("error"))
	errDesc := strings.TrimSpace(query.Get("error_description"))

	if parsedURL.Fragment != "" {
		if fragQuery, errFrag := url.ParseQuery(parsedURL.Fragment); errFrag == nil {
			if code == "" {
				code = strings.TrimSpace(fragQuery.Get("code"))
			}
			if state == "" {
				state = strings.TrimSpace(fragQuery.Get("state"))
			}
			if errCode == "" {
				errCode = strings.TrimSpace(fragQuery.Get("error"))
			}
			if errDesc == "" {
				errDesc = strings.TrimSpace(fragQuery.Get("error_description"))
			}
		}
	}

	if code != "" && state == "" && strings.Contains(code, "#") {
		parts := strings.SplitN(code, "#", 2)
		code = parts[0]
		state = parts[1]
	}

	if errCode == "" && errDesc != "" {
		errCode = errDesc
		errDesc = ""
	}

	if code == "" && errCode == "" {
		return nil, fmt.Errorf("callback URL missing code")
	}

	return &OAuthCallback{
		Code:             code,
		State:            state,
		Error:            errCode,
		ErrorDescription: errDesc,
	}, nil
}
