// amp - proxy.go
// Amp 上游反向代理实现。
// 该模块创建并配置 httputil.ReverseProxy 实例，用于将请求代理到 Amp 上游服务。
// 主要功能包括：
//   - 自动注入上游 API 密钥（替换客户端的认证信息）
//   - 清理代理和浏览器指纹头部
//   - 移除与客户端密钥匹配的查询参数凭据
//   - 自动检测并解压未标记 Content-Encoding 的 gzip 响应
//   - 客户端取消时的静默错误处理
package amp

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/misc"
	log "github.com/sirupsen/logrus"
)

// removeQueryValuesMatching 从请求 URL 中移除指定键中值等于 match 的查询参数。
// 如果移除后该键没有剩余值，则完全删除该键。用于防止客户端凭据泄露到上游。
func removeQueryValuesMatching(req *http.Request, key string, match string) {
	if req == nil || req.URL == nil || match == "" {
		return
	}

	q := req.URL.Query()
	values, ok := q[key]
	if !ok || len(values) == 0 {
		return
	}

	kept := make([]string, 0, len(values))
	for _, v := range values {
		if v == match {
			continue
		}
		kept = append(kept, v)
	}

	if len(kept) == 0 {
		q.Del(key)
	} else {
		q[key] = kept
	}
	req.URL.RawQuery = q.Encode()
}

// readCloser 包装一个 reader 和一个独立的 closer。
// 用于在恢复预读取字节的同时保持原始 body 的 Close 行为。
type readCloser struct {
	r io.Reader // 读取器，包含预读取的字节和原始 body
	c io.Closer // 关闭器，指向原始 body 以确保正确的资源释放
}

// Read 从包装的读取器中读取数据。
func (rc *readCloser) Read(p []byte) (int, error) { return rc.r.Read(p) }

// Close 关闭原始的 closer。
func (rc *readCloser) Close() error { return rc.c.Close() }

// createReverseProxy 创建一个用于 Amp 上游的反向代理处理器。
// 配置内容包括：
//   - Director: 注入上游 API 密钥，清理客户端认证信息和代理头部
//   - ModifyResponse: 自动检测并解压未标记的 gzip 响应
//   - ErrorHandler: 静默处理客户端取消，记录其他代理错误
//
// 参数 upstreamURL 为上游服务地址，secretSource 用于获取上游 API 密钥。
func createReverseProxy(upstreamURL string, secretSource SecretSource) (*httputil.ReverseProxy, error) {
	parsed, err := url.Parse(upstreamURL)
	if err != nil {
		return nil, fmt.Errorf("invalid amp upstream url: %w", err)
	}

	proxy := httputil.NewSingleHostReverseProxy(parsed)
	originalDirector := proxy.Director

	// Modify outgoing requests to inject API key and fix routing
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = parsed.Host

		// Remove client's Authorization header - it was only used for CLI Proxy API authentication
		// We will set our own Authorization using the configured upstream-api-key
		req.Header.Del("Authorization")
		req.Header.Del("X-Api-Key")
		req.Header.Del("X-Goog-Api-Key")

		// Remove proxy, client identity, and browser fingerprint headers
		misc.ScrubProxyAndFingerprintHeaders(req)

		// Remove query-based credentials if they match the authenticated client API key.
		// This prevents leaking client auth material to the Amp upstream while avoiding
		// breaking unrelated upstream query parameters.
		clientKey := getClientAPIKeyFromContext(req.Context())
		removeQueryValuesMatching(req, "key", clientKey)
		removeQueryValuesMatching(req, "auth_token", clientKey)

		// Preserve correlation headers for debugging
		if req.Header.Get("X-Request-ID") == "" {
			// Could generate one here if needed
		}

		// Note: We do NOT filter Anthropic-Beta headers in the proxy path
		// Users going through ampcode.com proxy are paying for the service and should get all features
		// including 1M context window (context-1m-2025-08-07)

		// Inject API key from secret source (only uses upstream-api-key from config)
		if key, err := secretSource.Get(req.Context()); err == nil && key != "" {
			req.Header.Set("X-Api-Key", key)
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", key))
		} else if err != nil {
			log.Warnf("amp secret source error (continuing without auth): %v", err)
		}
	}

	// Modify incoming responses to handle gzip without Content-Encoding
	// This addresses the same issue as inline handler gzip handling, but at the proxy level
	proxy.ModifyResponse = func(resp *http.Response) error {
		// Skip if already marked as gzip (Content-Encoding set)
		if resp.Header.Get("Content-Encoding") != "" {
			return nil
		}

		// Skip streaming responses (SSE, chunked)
		if isStreamingResponse(resp) {
			return nil
		}

		// Save reference to original upstream body for proper cleanup
		originalBody := resp.Body

		// Peek at first 2 bytes to detect gzip magic bytes
		header := make([]byte, 2)
		n, _ := io.ReadFull(originalBody, header)

		// Check for gzip magic bytes (0x1f 0x8b)
		// If n < 2, we didn't get enough bytes, so it's not gzip
		if n >= 2 && header[0] == 0x1f && header[1] == 0x8b {
			// It's gzip - read the rest of the body
			rest, err := io.ReadAll(originalBody)
			if err != nil {
				// Restore what we read and return original body (preserve Close behavior)
				resp.Body = &readCloser{
					r: io.MultiReader(bytes.NewReader(header[:n]), originalBody),
					c: originalBody,
				}
				return nil
			}

			// Reconstruct complete gzipped data
			gzippedData := append(header[:n], rest...)

			// Decompress
			gzipReader, err := gzip.NewReader(bytes.NewReader(gzippedData))
			if err != nil {
				log.Warnf("amp proxy: gzip header detected but decompress failed: %v", err)
				// Close original body and return in-memory copy
				_ = originalBody.Close()
				resp.Body = io.NopCloser(bytes.NewReader(gzippedData))
				return nil
			}

			decompressed, err := io.ReadAll(gzipReader)
			_ = gzipReader.Close()
			if err != nil {
				log.Warnf("amp proxy: gzip decompress error: %v", err)
				// Close original body and return in-memory copy
				_ = originalBody.Close()
				resp.Body = io.NopCloser(bytes.NewReader(gzippedData))
				return nil
			}

			// Close original body since we're replacing with in-memory decompressed content
			_ = originalBody.Close()

			// Replace body with decompressed content
			resp.Body = io.NopCloser(bytes.NewReader(decompressed))
			resp.ContentLength = int64(len(decompressed))

			// Update headers to reflect decompressed state
			resp.Header.Del("Content-Encoding")                                          // No longer compressed
			resp.Header.Del("Content-Length")                                            // Remove stale compressed length
			resp.Header.Set("Content-Length", strconv.FormatInt(resp.ContentLength, 10)) // Set decompressed length

			log.Debugf("amp proxy: decompressed gzip response (%d -> %d bytes)", len(gzippedData), len(decompressed))
		} else {
			// Not gzip - restore peeked bytes while preserving Close behavior
			// Handle edge cases: n might be 0, 1, or 2 depending on EOF
			resp.Body = &readCloser{
				r: io.MultiReader(bytes.NewReader(header[:n]), originalBody),
				c: originalBody,
			}
		}

		return nil
	}

	// Error handler for proxy failures
	proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, err error) {
		// Client-side cancellations are common during polling; suppress logging in this case
		if errors.Is(err, context.Canceled) {
			return
		}
		log.Errorf("amp upstream proxy error for %s %s: %v", req.Method, req.URL.Path, err)
		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusBadGateway)
		_, _ = rw.Write([]byte(`{"error":"amp_upstream_proxy_error","message":"Failed to reach Amp upstream"}`))
	}

	return proxy, nil
}

// isStreamingResponse 检测响应是否为流式响应（仅 SSE）。
// 注意：仅将 text/event-stream 视为真正的流式响应。
// 分块传输编码（chunked）是传输层细节，不代表不能解压完整响应。
// 许多 JSON API 使用分块编码传输普通响应。
func isStreamingResponse(resp *http.Response) bool {
	contentType := resp.Header.Get("Content-Type")

	// Only Server-Sent Events are true streaming responses
	if strings.Contains(contentType, "text/event-stream") {
		return true
	}

	return false
}

// proxyHandler 将 httputil.ReverseProxy 转换为 gin.HandlerFunc。
func proxyHandler(proxy *httputil.ReverseProxy) gin.HandlerFunc {
	return func(c *gin.Context) {
		proxy.ServeHTTP(c.Writer, c.Request)
	}
}

// filterBetaFeatures 从逗号分隔的 beta 功能列表中移除指定功能。
// 用于从 Anthropic-Beta 头部中过滤特定的 beta 特性。
func filterBetaFeatures(header, featureToRemove string) string {
	features := strings.Split(header, ",")
	filtered := make([]string, 0, len(features))

	for _, feature := range features {
		trimmed := strings.TrimSpace(feature)
		if trimmed != "" && trimmed != featureToRemove {
			filtered = append(filtered, trimmed)
		}
	}

	return strings.Join(filtered, ",")
}
