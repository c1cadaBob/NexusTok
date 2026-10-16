package service

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/c1cadaBob/NexusTok/common"
	"github.com/c1cadaBob/NexusTok/logger"

	"github.com/gin-gonic/gin"
)

func CloseResponseBodyGracefully(httpResponse *http.Response) {
	if httpResponse == nil || httpResponse.Body == nil {
		return
	}
	err := httpResponse.Body.Close()
	if err != nil {
		common.SysError("failed to close response body: " + err.Error())
	}
}

const upstreamRequestIDSourceContextKey = "nexustok.upstream_request_id_source"

// CaptureUpstreamRequestID captures the upstream request ID while preferring
// the NexusTok-compatible header over standard request ID headers.
func CaptureUpstreamRequestID(c *gin.Context, headers http.Header) {
	if c == nil || headers == nil {
		return
	}
	if requestID := upstreamHeaderValue(headers, common.RequestIdKey); requestID != "" {
		c.Set(common.UpstreamRequestIdKey, requestID)
		c.Set(upstreamRequestIDSourceContextKey, "custom")
		return
	}
	if source, exists := c.Get(upstreamRequestIDSourceContextKey); exists && source == "custom" {
		return
	}
	for _, headerName := range []string{"X-Request-ID", "X-Request-Id"} {
		if requestID := upstreamHeaderValue(headers, headerName); requestID != "" {
			c.Set(common.UpstreamRequestIdKey, requestID)
			c.Set(upstreamRequestIDSourceContextKey, "standard")
			return
		}
	}
}

func upstreamHeaderValue(headers http.Header, name string) string {
	if value := strings.TrimSpace(headers.Get(name)); value != "" {
		return value
	}
	for key, values := range headers {
		if strings.EqualFold(key, name) && len(values) > 0 {
			return strings.TrimSpace(values[0])
		}
	}
	return ""
}

// ShouldCopyUpstreamHeader checks whether a given upstream response header
// should be copied to the client response. It returns false for Content-Length
// and request ID headers so the local instance ID remains authoritative.
func ShouldCopyUpstreamHeader(c *gin.Context, k string, v []string) bool {
	if strings.EqualFold(k, "Content-Length") {
		return false
	}
	if strings.EqualFold(k, common.RequestIdKey) ||
		strings.EqualFold(k, "X-Request-ID") ||
		strings.EqualFold(k, "X-Request-Id") {
		if c != nil && len(v) > 0 {
			CaptureUpstreamRequestID(c, http.Header{k: v})
		}
		return false
	}
	return true
}

func IOCopyBytesGracefully(c *gin.Context, src *http.Response, data []byte) {
	if c.Writer == nil {
		return
	}

	body := io.NopCloser(bytes.NewBuffer(data))

	// We shouldn't set the header before we parse the response body, because the parse part may fail.
	// And then we will have to send an error response, but in this case, the header has already been set.
	// So the httpClient will be confused by the response.
	// For example, Postman will report error, and we cannot check the response at all.
	if src != nil {
		CaptureUpstreamRequestID(c, src.Header)
		for k, v := range src.Header {
			if !ShouldCopyUpstreamHeader(c, k, v) {
				continue
			}
			c.Writer.Header().Set(k, v[0])
		}
	}

	// set Content-Length header manually BEFORE calling WriteHeader
	c.Writer.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))

	// Write header with status code (this sends the headers)
	if src != nil {
		c.Writer.WriteHeader(src.StatusCode)
	} else {
		c.Writer.WriteHeader(http.StatusOK)
	}

	_, err := io.Copy(c.Writer, body)
	if err != nil {
		logger.LogError(c, fmt.Sprintf("failed to copy response body: %s", err.Error()))
	}
	c.Writer.Flush()
}
