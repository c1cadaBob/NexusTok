// Package channel - api_request.go
// 该文件实现了 API 请求的通用处理逻辑
//
// 核心功能：
// 1. 请求头设置：根据中继模式设置 Content-Type、Accept 等头
// 2. 请求头透传：支持从客户端透传请求头到上游
// 3. 请求头模板：支持在请求头中使用变量模板
// 4. 请求发送：发送 HTTP/WebSocket 请求到上游
// 5. 响应处理：处理上游响应，提取使用量信息
package channel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	common2 "github.com/c1cada/NexusTok/common"            // 公共工具包
	"github.com/c1cada/NexusTok/logger"                    // 日志
	"github.com/c1cada/NexusTok/relay/common"              // 中继公共
	"github.com/c1cada/NexusTok/relay/constant"            // 中继常量
	"github.com/c1cada/NexusTok/relay/helper"              // 中继辅助
	"github.com/c1cada/NexusTok/service"                   // 服务层
	"github.com/c1cada/NexusTok/setting/operation_setting" // 运营设置
	"github.com/c1cada/NexusTok/types"                     // 类型定义

	"github.com/bytedance/gopkg/util/gopool" // 协程池
	"github.com/gin-gonic/gin"               // Gin 框架
	"github.com/gorilla/websocket"           // WebSocket
)

// SetupApiRequestHeader 设置 API 请求头
// 根据中继模式设置不同的请求头：
// - 音频转录/翻译：multipart/form-data（不设置 Content-Type）
// - WebSocket：不设置请求头
// - 其他：从客户端复制 Content-Type 和 Accept
//
// 参数：
//   - info: 中继信息
//   - c: Gin 上下文
//   - req: 请求头
func SetupApiRequestHeader(info *common.RelayInfo, c *gin.Context, req *http.Header) {
	if info.RelayMode == constant.RelayModeAudioTranscription || info.RelayMode == constant.RelayModeAudioTranslation {
		// multipart/form-data
	} else if info.RelayMode == constant.RelayModeRealtime {
		// websocket
	} else {
		req.Set("Content-Type", c.Request.Header.Get("Content-Type"))
		req.Set("Accept", c.Request.Header.Get("Accept"))
		if info.IsStream && c.Request.Header.Get("Accept") == "" {
			req.Set("Accept", "text/event-stream")
		}
	}
}

// clientHeaderPlaceholderPrefix 客户端请求头占位符前缀
// 用于在请求头模板中引用客户端请求头
// 格式：{client_header:X-Custom-Header}
const clientHeaderPlaceholderPrefix = "{client_header:"

// 请求头透传模式常量
const (
	headerPassthroughAllKey        = "*"      // 透传所有请求头
	headerPassthroughRegexPrefix   = "re:"    // 正则匹配模式（旧版）
	headerPassthroughRegexPrefixV2 = "regex:" // 正则匹配模式（新版）
)

// passthroughSkipHeaderNamesLower 不允许透传的请求头列表
// 包括 hop-by-hop 头、认证头、WebSocket 握手头等
var passthroughSkipHeaderNamesLower = map[string]struct{}{
	// RFC 7230 hop-by-hop 头
	"connection":          {},
	"keep-alive":          {},
	"proxy-authenticate":  {},
	"proxy-authorization": {},
	"te":                  {},
	"trailer":             {},
	"transfer-encoding":   {},
	"upgrade":             {},

	"cookie": {},

	// 不应转发的额外请求头
	"host":            {},
	"content-length":  {},
	"accept-encoding": {},

	// 不允许通过通配符/正则透传的认证头
	"authorization":  {},
	"x-api-key":      {},
	"x-goog-api-key": {},

	// WebSocket 握手头由客户端/拨号器生成
	"sec-websocket-key":        {},
	"sec-websocket-version":    {},
	"sec-websocket-extensions": {},
}

// applyUpstreamContentLength 在转换后的上游请求体使用 BodyStorage 时回填 ContentLength。
//
// net/http.NewRequest 只能自动识别 *bytes.Reader、*bytes.Buffer 和 *strings.Reader 的长度。
// 当 handler 使用 relay/common.NewOutboundJSONBody 后，请求体会被 ReaderOnly 包装成普通
// io.Reader，此时如果不显式设置 ContentLength，上游会收到 chunked 请求。部分 provider 对
// JSON POST 要求固定 Content-Length，因此这里集中使用 RelayInfo 中记录的最终字节数。
func applyUpstreamContentLength(req *http.Request, info *common.RelayInfo) {
	if info == nil {
		return
	}
	if info.UpstreamRequestBodySize > 0 && req.ContentLength <= 0 {
		req.ContentLength = info.UpstreamRequestBodySize
	}
}

// headerPassthroughRegexCache 正则表达式缓存
// 避免重复编译相同的正则表达式
var headerPassthroughRegexCache sync.Map // map[string]*regexp.Regexp

// getHeaderPassthroughRegex 获取编译后的正则表达式
// 使用缓存避免重复编译
//
// 参数：
//   - pattern: 正则表达式模式
//
// 返回值：
//   - *regexp.Regexp: 编译后的正则表达式
//   - error: 编译错误
func getHeaderPassthroughRegex(pattern string) (*regexp.Regexp, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return nil, errors.New("empty regex pattern")
	}
	if v, ok := headerPassthroughRegexCache.Load(pattern); ok {
		if re, ok := v.(*regexp.Regexp); ok {
			return re, nil
		}
		headerPassthroughRegexCache.Delete(pattern)
	}
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	actual, _ := headerPassthroughRegexCache.LoadOrStore(pattern, compiled)
	if re, ok := actual.(*regexp.Regexp); ok {
		return re, nil
	}
	return compiled, nil
}

func IsHeaderPassthroughRuleKey(key string) bool {
	return isHeaderPassthroughRuleKey(key)
}
func isHeaderPassthroughRuleKey(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	if key == headerPassthroughAllKey {
		return true
	}
	lower := strings.ToLower(key)
	return strings.HasPrefix(lower, headerPassthroughRegexPrefix) || strings.HasPrefix(lower, headerPassthroughRegexPrefixV2)
}

func shouldSkipPassthroughHeader(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return true
	}
	lower := strings.ToLower(name)
	if _, ok := passthroughSkipHeaderNamesLower[lower]; ok {
		return true
	}
	return false
}

func applyHeaderOverridePlaceholders(template string, c *gin.Context, apiKey string) (string, bool, error) {
	trimmed := strings.TrimSpace(template)
	if strings.HasPrefix(trimmed, clientHeaderPlaceholderPrefix) {
		afterPrefix := trimmed[len(clientHeaderPlaceholderPrefix):]
		end := strings.Index(afterPrefix, "}")
		if end < 0 || end != len(afterPrefix)-1 {
			return "", false, fmt.Errorf("client_header placeholder must be the full value: %q", template)
		}

		name := strings.TrimSpace(afterPrefix[:end])
		if name == "" {
			return "", false, fmt.Errorf("client_header placeholder name is empty: %q", template)
		}
		if c == nil || c.Request == nil {
			return "", false, fmt.Errorf("missing request context for client_header placeholder")
		}
		clientHeaderValue := c.Request.Header.Get(name)
		if strings.TrimSpace(clientHeaderValue) == "" {
			return "", false, nil
		}
		// Do not interpolate {api_key} inside client-supplied content.
		return clientHeaderValue, true, nil
	}

	if strings.Contains(template, "{api_key}") {
		template = strings.ReplaceAll(template, "{api_key}", apiKey)
	}
	if strings.TrimSpace(template) == "" {
		return "", false, nil
	}
	return template, true, nil
}

// processHeaderOverride applies channel header overrides, with placeholder substitution.
// Supported placeholders:
//   - {api_key}: resolved to the channel API key
//   - {client_header:<name>}: resolved to the incoming request header value
//
// Header passthrough rules (keys only; values are ignored):
//   - "*": passthrough all incoming headers by name (excluding unsafe headers)
//   - "re:<regex>" / "regex:<regex>": passthrough headers whose names match the regex (Go regexp)
//
// Passthrough rules are applied first, then normal overrides are applied, so explicit overrides win.
func processHeaderOverride(info *common.RelayInfo, c *gin.Context) (map[string]string, error) {
	headerOverride := make(map[string]string)
	if info == nil {
		return headerOverride, nil
	}

	headerOverrideSource := common.GetEffectiveHeaderOverride(info)

	passAll := false
	var passthroughRegex []*regexp.Regexp
	if !info.IsChannelTest {
		for k := range headerOverrideSource {
			key := strings.TrimSpace(strings.ToLower(k))
			if key == "" {
				continue
			}
			if key == headerPassthroughAllKey {
				passAll = true
				continue
			}

			var pattern string
			switch {
			case strings.HasPrefix(key, headerPassthroughRegexPrefix):
				pattern = strings.TrimSpace(key[len(headerPassthroughRegexPrefix):])
			case strings.HasPrefix(key, headerPassthroughRegexPrefixV2):
				pattern = strings.TrimSpace(key[len(headerPassthroughRegexPrefixV2):])
			default:
				continue
			}

			if pattern == "" {
				return nil, types.NewError(fmt.Errorf("header passthrough regex pattern is empty: %q", k), types.ErrorCodeChannelHeaderOverrideInvalid)
			}
			compiled, err := getHeaderPassthroughRegex(pattern)
			if err != nil {
				return nil, types.NewError(err, types.ErrorCodeChannelHeaderOverrideInvalid)
			}
			passthroughRegex = append(passthroughRegex, compiled)
		}
	}

	if passAll || len(passthroughRegex) > 0 {
		if c == nil || c.Request == nil {
			return nil, types.NewError(fmt.Errorf("missing request context for header passthrough"), types.ErrorCodeChannelHeaderOverrideInvalid)
		}
		for name := range c.Request.Header {
			if shouldSkipPassthroughHeader(name) {
				continue
			}
			if !passAll {
				matched := false
				for _, re := range passthroughRegex {
					if re.MatchString(name) {
						matched = true
						break
					}
				}
				if !matched {
					continue
				}
			}
			value := strings.TrimSpace(c.Request.Header.Get(name))
			if value == "" {
				continue
			}
			headerOverride[strings.ToLower(strings.TrimSpace(name))] = value
		}
	}

	for k, v := range headerOverrideSource {
		if isHeaderPassthroughRuleKey(k) {
			continue
		}
		key := strings.TrimSpace(strings.ToLower(k))
		if key == "" {
			continue
		}

		str, ok := v.(string)
		if !ok {
			return nil, types.NewError(nil, types.ErrorCodeChannelHeaderOverrideInvalid)
		}
		if info.IsChannelTest && strings.HasPrefix(strings.TrimSpace(str), clientHeaderPlaceholderPrefix) {
			continue
		}

		value, include, err := applyHeaderOverridePlaceholders(str, c, info.ApiKey)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeChannelHeaderOverrideInvalid)
		}
		if !include {
			continue
		}

		headerOverride[key] = value
	}
	return headerOverride, nil
}

func ResolveHeaderOverride(info *common.RelayInfo, c *gin.Context) (map[string]string, error) {
	return processHeaderOverride(info, c)
}

func applyHeaderOverrideToRequest(req *http.Request, headerOverride map[string]string) {
	if req == nil {
		return
	}
	for key, value := range headerOverride {
		req.Header.Set(key, value)
		// set Host in req
		if strings.EqualFold(key, "Host") {
			req.Host = value
		}
	}
}

func DoApiRequest(a Adaptor, c *gin.Context, info *common.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	fullRequestURL, err := a.GetRequestURL(info)
	if err != nil {
		return nil, fmt.Errorf("get request url failed: %w", err)
	}
	if common2.DebugEnabled {
		println("fullRequestURL:", fullRequestURL)
	}
	req, err := http.NewRequest(c.Request.Method, fullRequestURL, requestBody)
	if err != nil {
		return nil, fmt.Errorf("new request failed: %w", err)
	}
	applyUpstreamContentLength(req, info)
	headers := req.Header
	err = a.SetupRequestHeader(c, &headers, info)
	if err != nil {
		return nil, fmt.Errorf("setup request header failed: %w", err)
	}
	// 在 SetupRequestHeader 之后应用 Header Override，确保用户设置优先级最高
	// 这样可以覆盖默认的 Authorization header 设置
	headerOverride, err := processHeaderOverride(info, c)
	if err != nil {
		return nil, err
	}
	applyHeaderOverrideToRequest(req, headerOverride)
	resp, err := doRequest(c, req, info)
	if err != nil {
		return nil, fmt.Errorf("do request failed: %w", err)
	}
	return resp, nil
}

func DoFormRequest(a Adaptor, c *gin.Context, info *common.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	fullRequestURL, err := a.GetRequestURL(info)
	if err != nil {
		return nil, fmt.Errorf("get request url failed: %w", err)
	}
	if common2.DebugEnabled {
		println("fullRequestURL:", fullRequestURL)
	}
	req, err := http.NewRequest(c.Request.Method, fullRequestURL, requestBody)
	if err != nil {
		return nil, fmt.Errorf("new request failed: %w", err)
	}
	applyUpstreamContentLength(req, info)
	// set form data
	req.Header.Set("Content-Type", c.Request.Header.Get("Content-Type"))
	headers := req.Header
	err = a.SetupRequestHeader(c, &headers, info)
	if err != nil {
		return nil, fmt.Errorf("setup request header failed: %w", err)
	}
	// 在 SetupRequestHeader 之后应用 Header Override，确保用户设置优先级最高
	// 这样可以覆盖默认的 Authorization header 设置
	headerOverride, err := processHeaderOverride(info, c)
	if err != nil {
		return nil, err
	}
	applyHeaderOverrideToRequest(req, headerOverride)
	resp, err := doRequest(c, req, info)
	if err != nil {
		return nil, fmt.Errorf("do request failed: %w", err)
	}
	return resp, nil
}

func DoWssRequest(a Adaptor, c *gin.Context, info *common.RelayInfo, requestBody io.Reader) (*websocket.Conn, error) {
	fullRequestURL, err := a.GetRequestURL(info)
	if err != nil {
		return nil, fmt.Errorf("get request url failed: %w", err)
	}
	targetHeader := http.Header{}
	err = a.SetupRequestHeader(c, &targetHeader, info)
	if err != nil {
		return nil, fmt.Errorf("setup request header failed: %w", err)
	}
	// 在 SetupRequestHeader 之后应用 Header Override，确保用户设置优先级最高
	// 这样可以覆盖默认的 Authorization header 设置
	headerOverride, err := processHeaderOverride(info, c)
	if err != nil {
		return nil, err
	}
	for key, value := range headerOverride {
		targetHeader.Set(key, value)
	}
	targetHeader.Set("Content-Type", c.Request.Header.Get("Content-Type"))
	targetConn, _, err := websocket.DefaultDialer.Dial(fullRequestURL, targetHeader)
	if err != nil {
		return nil, fmt.Errorf("dial failed to %s: %w", fullRequestURL, err)
	}
	// send request body
	//all, err := io.ReadAll(requestBody)
	//err = service.WssString(c, targetConn, string(all))
	return targetConn, nil
}

func startPingKeepAlive(c *gin.Context, pingInterval time.Duration) context.CancelFunc {
	pingerCtx, stopPinger := context.WithCancel(context.Background())

	gopool.Go(func() {
		defer func() {
			// 增加panic恢复处理
			if r := recover(); r != nil {
				if common2.DebugEnabled {
					println("SSE ping goroutine panic recovered:", fmt.Sprintf("%v", r))
				}
			}
			if common2.DebugEnabled {
				println("SSE ping goroutine stopped.")
			}
		}()

		if pingInterval <= 0 {
			pingInterval = helper.DefaultPingInterval
		}

		ticker := time.NewTicker(pingInterval)
		// 确保在任何情况下都清理ticker
		defer func() {
			ticker.Stop()
			if common2.DebugEnabled {
				println("SSE ping ticker stopped")
			}
		}()

		var pingMutex sync.Mutex
		if common2.DebugEnabled {
			println("SSE ping goroutine started")
		}

		// 增加超时控制，防止goroutine长时间运行
		maxPingDuration := 120 * time.Minute // 最大ping持续时间
		pingTimeout := time.NewTimer(maxPingDuration)
		defer pingTimeout.Stop()

		for {
			select {
			// 发送 ping 数据
			case <-ticker.C:
				if err := sendPingData(c, &pingMutex); err != nil {
					if common2.DebugEnabled {
						println("SSE ping error, stopping goroutine:", err.Error())
					}
					return
				}
			// 收到退出信号
			case <-pingerCtx.Done():
				return
			// request 结束
			case <-c.Request.Context().Done():
				return
			// 超时保护，防止goroutine无限运行
			case <-pingTimeout.C:
				if common2.DebugEnabled {
					println("SSE ping goroutine timeout, stopping")
				}
				return
			}
		}
	})

	return stopPinger
}

func sendPingData(c *gin.Context, mutex *sync.Mutex) error {
	// 增加超时控制，防止锁死等待
	done := make(chan error, 1)
	go func() {
		mutex.Lock()
		defer mutex.Unlock()

		err := helper.PingData(c)
		if err != nil {
			logger.LogError(c, "SSE ping error: "+err.Error())
			done <- err
			return
		}

		if common2.DebugEnabled {
			println("SSE ping data sent.")
		}
		done <- nil
	}()

	// 设置发送ping数据的超时时间
	select {
	case err := <-done:
		return err
	case <-time.After(10 * time.Second):
		return errors.New("SSE ping data send timeout")
	case <-c.Request.Context().Done():
		return errors.New("request context cancelled during ping")
	}
}

func DoRequest(c *gin.Context, req *http.Request, info *common.RelayInfo) (*http.Response, error) {
	return doRequest(c, req, info)
}
func doRequest(c *gin.Context, req *http.Request, info *common.RelayInfo) (*http.Response, error) {
	var client *http.Client
	var err error
	if info.ChannelSetting.Proxy != "" {
		client, err = service.NewProxyHttpClient(info.ChannelSetting.Proxy)
		if err != nil {
			return nil, fmt.Errorf("new proxy http client failed: %w", err)
		}
	} else {
		client = service.GetHttpClient()
	}

	var stopPinger context.CancelFunc
	if info.IsStream {
		helper.SetEventStreamHeaders(c)
		// 处理流式请求的 ping 保活
		generalSettings := operation_setting.GetGeneralSetting()
		if generalSettings.PingIntervalEnabled && !info.DisablePing {
			pingInterval := time.Duration(generalSettings.PingIntervalSeconds) * time.Second
			stopPinger = startPingKeepAlive(c, pingInterval)
			// 使用defer确保在任何情况下都能停止ping goroutine
			defer func() {
				if stopPinger != nil {
					stopPinger()
					if common2.DebugEnabled {
						println("SSE ping goroutine stopped by defer")
					}
				}
			}()
		}
	}

	info.SetUpstreamRequestStartTime(time.Now())
	resp, err := client.Do(req)
	if err != nil {
		logger.LogError(c, "do request failed: "+err.Error())
		return nil, types.NewError(err, types.ErrorCodeDoRequestFailed, types.ErrOptionWithHideErrMsg("upstream error: do request failed"))
	}
	if resp == nil {
		return nil, errors.New("resp is nil")
	}
	info.SetUpstreamResponseHeaderTime(time.Now())

	if upID := resp.Header.Get(common2.RequestIdKey); upID != "" {
		c.Set(common2.UpstreamRequestIdKey, upID)
	}

	_ = req.Body.Close()
	_ = c.Request.Body.Close()
	return resp, nil
}

func DoTaskApiRequest(a TaskAdaptor, c *gin.Context, info *common.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	fullRequestURL, err := a.BuildRequestURL(info)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(c.Request.Method, fullRequestURL, requestBody)
	if err != nil {
		return nil, fmt.Errorf("new request failed: %w", err)
	}
	applyUpstreamContentLength(req, info)
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(requestBody), nil
	}

	err = a.BuildRequestHeader(c, req, info)
	if err != nil {
		return nil, fmt.Errorf("setup request header failed: %w", err)
	}
	resp, err := doRequest(c, req, info)
	if err != nil {
		return nil, fmt.Errorf("do request failed: %w", err)
	}
	return resp, nil
}
