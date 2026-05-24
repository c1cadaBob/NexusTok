// Package helper 提供了中继层的各种辅助函数。
// 本文件负责构建计费表达式（Billing Expression）所需的请求输入数据。
// 计费表达式系统允许基于请求内容（如是否为流式、model 名称等）进行动态定价。
package helper

import (
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/pkg/billingexpr"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/gin-gonic/gin"
)

// ResolveIncomingBillingExprRequestInput 从 Gin 上下文中解析并构建计费表达式的请求输入。
// 如果 RelayInfo 中已预加载了 BillingRequestInput（例如在上游转发前缓存了请求体），
// 则直接使用预加载的数据并合并请求头；否则从 Gin 上下文中读取请求体。
//
// 参数：
//   - c: Gin 请求上下文
//   - info: 中继信息，可能包含预加载的计费请求输入
//
// 返回值：
//   - billingexpr.RequestInput: 计费表达式可用的请求输入（包含 body 和 headers）
//   - error: 解析过程中的错误
func ResolveIncomingBillingExprRequestInput(c *gin.Context, info *relaycommon.RelayInfo) (billingexpr.RequestInput, error) {
	// 优先使用预加载的请求输入（避免重复读取已消费的请求体）
	if info != nil && info.BillingRequestInput != nil {
		input := cloneRequestInput(*info.BillingRequestInput)
		// 合并 RelayInfo 中的请求头到输入中
		merged := cloneStringMap(info.RequestHeaders)
		for k, v := range input.Headers {
			merged[k] = v
		}
		input.Headers = merged
		return input, nil
	}

	// 从 Gin 上下文中读取请求体
	input := billingexpr.RequestInput{}
	if info != nil {
		input.Headers = cloneStringMap(info.RequestHeaders)
	}

	bodyBytes, err := readIncomingBillingExprBody(c)
	if err != nil {
		return billingexpr.RequestInput{}, err
	}
	input.Body = bodyBytes
	return input, nil
}

// BuildBillingExprRequestInputFromRequest 从已解析的请求对象构建计费表达式输入。
// 将请求对象序列化为 JSON 作为 body，并传入 headers。
// 适用于已有请求对象（无需从 HTTP 上下文读取）的场景。
//
// 参数：
//   - request: 已解析的请求对象（实现 dto.Request 接口），可为 nil
//   - headers: 请求头键值对映射
//
// 返回值：
//   - billingexpr.RequestInput: 构建完成的计费请求输入
//   - error: JSON 序列化错误
func BuildBillingExprRequestInputFromRequest(request dto.Request, headers map[string]string) (billingexpr.RequestInput, error) {
	input := billingexpr.RequestInput{
		Headers: cloneStringMap(headers),
	}
	if request == nil {
		return input, nil
	}

	bodyBytes, err := common.Marshal(request)
	if err != nil {
		return billingexpr.RequestInput{}, err
	}
	input.Body = bodyBytes
	return input, nil
}

// readIncomingBillingExprBody 从 Gin 上下文中读取 JSON 请求体。
// 仅当 Content-Type 为 application/json 时才读取，否则返回 nil。
// 使用 common.GetBodyStorage 获取可重复读取的请求体存储。
func readIncomingBillingExprBody(c *gin.Context) ([]byte, error) {
	if c == nil || c.Request == nil || !isJSONContentType(c.Request.Header.Get("Content-Type")) {
		return nil, nil
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil, err
	}
	return storage.Bytes()
}

// cloneRequestInput 深拷贝一个 RequestInput 对象。
// 避免修改原始对象中的 Headers map 和 Body 切片。
func cloneRequestInput(src billingexpr.RequestInput) billingexpr.RequestInput {
	input := billingexpr.RequestInput{
		Headers: cloneStringMap(src.Headers),
	}
	if len(src.Body) > 0 {
		input.Body = append([]byte(nil), src.Body...)
	}
	return input
}

// isJSONContentType 检查 Content-Type 是否为 application/json。
// 支持大小写不敏感和前后空格的处理。
func isJSONContentType(contentType string) bool {
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	return strings.HasPrefix(contentType, "application/json")
}

// cloneStringMap 深拷贝一个 string 类型的 map。
// 跳过空键的条目，返回一个新的 map 实例。
func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return map[string]string{}
	}
	dst := make(map[string]string, len(src))
	for key, value := range src {
		if strings.TrimSpace(key) == "" {
			continue
		}
		dst[key] = value
	}
	return dst
}
