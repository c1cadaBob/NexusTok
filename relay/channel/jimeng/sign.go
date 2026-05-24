// jimeng - sign.go
// 即梦（Jimeng）AI API 请求签名模块。
// 实现了基于 HMAC-SHA256 的请求签名机制，用于对即梦 API 的 HTTP 请求进行身份认证。
// 签名流程遵循火山引擎的签名规范（V4 签名算法），包括：
//   - 构建规范请求（Canonical Request）
//   - 计算签名字符串（String to Sign）
//   - 派生签名密钥（Signing Key）
//   - 生成最终签名并附加到 Authorization 请求头
package jimeng

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/logger"
	"github.com/gin-gonic/gin"
)

// SignRequestForJimeng 对即梦 API 请求进行签名，支持 http.Request 或 header+url+body 方式
//func SignRequestForJimeng(req *http.Request, accessKey, secretKey string) error {
//	var bodyBytes []byte
//	var err error
//
//	if req.Body != nil {
//		bodyBytes, err = io.ReadAll(req.Body)
//		if err != nil {
//			return fmt.Errorf("read request body failed: %w", err)
//		}
//		_ = req.Body.Close()
//		req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes)) // rewind
//	} else {
//		bodyBytes = []byte{}
//	}
//
//	return signJimengHeaders(&req.Header, req.Method, req.URL, bodyBytes, accessKey, secretKey)
//}

// HexPayloadHashKey 是 Gin 上下文中存储 payload hash 的键名。
// 用于在请求处理链中传递已计算的请求体哈希值。
const HexPayloadHashKey = "HexPayloadHash"

// SetPayloadHash 计算请求体的 SHA-256 哈希值并存储到 Gin 上下文中。
// 用于在后续的签名过程中复用已计算的 payload hash，避免重复计算。
// 参数:
//   - c: Gin 上下文
//   - req: 请求对象（将被序列化为 JSON 后计算哈希）
//
// 返回:
//   - error: JSON 序列化失败时返回错误
func SetPayloadHash(c *gin.Context, req any) error {
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	logger.LogInfo(c, fmt.Sprintf("SetPayloadHash body: %s", body))
	payloadHash := sha256.Sum256(body)
	hexPayloadHash := hex.EncodeToString(payloadHash[:])
	c.Set(HexPayloadHashKey, hexPayloadHash)
	return nil
}
// getPayloadHash 从 Gin 上下文中获取之前存储的 payload hash。
// 参数:
//   - c: Gin 上下文
//
// 返回:
//   - string: 十六进制编码的 payload hash，不存在时返回空字符串
func getPayloadHash(c *gin.Context) string {
	return c.GetString(HexPayloadHashKey)
}

// Sign 对即梦 API 的 HTTP 请求进行 HMAC-SHA256 签名。
// 签名流程（遵循火山引擎 V4 签名规范）：
//  1. 读取并计算请求体的 SHA-256 哈希
//  2. 从 apiKey 中解析 accessKey 和 secretKey（格式: "ak|sk"）
//  3. 设置 Host、X-Date、X-Content-Sha256 请求头
//  4. 构建规范请求字符串（Canonical Request）：包含 HTTP 方法、路径、查询参数、头部和 payload hash
//  5. 计算规范请求的 SHA-256 哈希
//  6. 构建签名字符串（String to Sign）
//  7. 通过 HMAC 链式派生签名密钥：secretKey -> kDate -> kRegion -> kService -> kSigning
//  8. 计算最终签名并生成 Authorization 头
//
// 参数:
//   - c: Gin 上下文
//   - req: 待签名的 HTTP 请求（函数会修改其 Header）
//   - apiKey: 即梦 API 密钥，格式为 "accessKey|secretKey"
//
// 返回:
//   - error: 签名过程中的错误（如 apiKey 格式无效、读取请求体失败等）
func Sign(c *gin.Context, req *http.Request, apiKey string) error {
	header := req.Header

	var bodyBytes []byte
	var err error

	if req.Body != nil {
		bodyBytes, err = io.ReadAll(req.Body)
		if err != nil {
			return err
		}
		_ = req.Body.Close()
		req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes)) // Rewind
	}

	payloadHash := sha256.Sum256(bodyBytes)
	hexPayloadHash := hex.EncodeToString(payloadHash[:])

	method := c.Request.Method
	u := req.URL
	keyParts := strings.Split(apiKey, "|")
	if len(keyParts) != 2 {
		return errors.New("invalid api key format for jimeng: expected 'ak|sk'")
	}
	accessKey := strings.TrimSpace(keyParts[0])
	secretKey := strings.TrimSpace(keyParts[1])
	t := time.Now().UTC()
	xDate := t.Format("20060102T150405Z")
	shortDate := t.Format("20060102")

	host := u.Host
	header.Set("Host", host)
	header.Set("X-Date", xDate)
	header.Set("X-Content-Sha256", hexPayloadHash)

	// Sort and encode query parameters to create canonical query string
	queryParams := u.Query()
	sortedKeys := make([]string, 0, len(queryParams))
	for k := range queryParams {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)
	var queryParts []string
	for _, k := range sortedKeys {
		values := queryParams[k]
		sort.Strings(values)
		for _, v := range values {
			queryParts = append(queryParts, fmt.Sprintf("%s=%s", url.QueryEscape(k), url.QueryEscape(v)))
		}
	}
	canonicalQueryString := strings.Join(queryParts, "&")

	headersToSign := map[string]string{
		"host":             host,
		"x-date":           xDate,
		"x-content-sha256": hexPayloadHash,
	}
	if header.Get("Content-Type") == "" {
		header.Set("Content-Type", "application/json")
	}
	headersToSign["content-type"] = header.Get("Content-Type")

	var signedHeaderKeys []string
	for k := range headersToSign {
		signedHeaderKeys = append(signedHeaderKeys, k)
	}
	sort.Strings(signedHeaderKeys)

	var canonicalHeaders strings.Builder
	for _, k := range signedHeaderKeys {
		canonicalHeaders.WriteString(k)
		canonicalHeaders.WriteString(":")
		canonicalHeaders.WriteString(strings.TrimSpace(headersToSign[k]))
		canonicalHeaders.WriteString("\n")
	}
	signedHeaders := strings.Join(signedHeaderKeys, ";")

	canonicalRequest := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s",
		method,
		u.Path,
		canonicalQueryString,
		canonicalHeaders.String(),
		signedHeaders,
		hexPayloadHash,
	)

	hashedCanonicalRequest := sha256.Sum256([]byte(canonicalRequest))
	hexHashedCanonicalRequest := hex.EncodeToString(hashedCanonicalRequest[:])

	region := "cn-north-1"
	serviceName := "cv"
	credentialScope := fmt.Sprintf("%s/%s/%s/request", shortDate, region, serviceName)
	stringToSign := fmt.Sprintf("HMAC-SHA256\n%s\n%s\n%s",
		xDate,
		credentialScope,
		hexHashedCanonicalRequest,
	)

	kDate := hmacSHA256([]byte(secretKey), []byte(shortDate))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(serviceName))
	kSigning := hmacSHA256(kService, []byte("request"))
	signature := hex.EncodeToString(hmacSHA256(kSigning, []byte(stringToSign)))

	authorization := fmt.Sprintf("HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		accessKey,
		credentialScope,
		signedHeaders,
		signature,
	)
	header.Set("Authorization", authorization)
	return nil
}

// hmacSHA256 使用 HMAC-SHA256 算法计算消息认证码。
// 用于火山引擎 V4 签名算法中的密钥派生和签名计算。
// 参数:
//   - key: HMAC 密钥
//   - data: 待计算的数据
//
// 返回:
//   - []byte: HMAC-SHA256 计算结果（32 字节）
func hmacSHA256(key []byte, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}
