// 腾讯云混元 API 的请求转换和响应处理实现文件。
// 包含 OpenAI 格式与腾讯云格式的互转、流式/非流式响应处理、
// TC3-HMAC-SHA256 签名计算等核心功能。
// 参考文档：https://cloud.tencent.com/document/product/1729/97732
package tencent

import (
	"bufio"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	// 项目内部依赖
	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/dto"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/relay/helper"
	"github.com/c1cada/NexusTok/service"
	"github.com/c1cada/NexusTok/types"

	// 第三方依赖
	"github.com/gin-gonic/gin"
)

// requestOpenAI2Tencent 将 OpenAI 格式的请求转换为腾讯云混元 API 格式。
// 将消息列表中的每条消息提取 Content 和 Role，映射到腾讯云的消息格式。
// 参数:
//   - a: 腾讯云适配器实例
//   - request: OpenAI 格式的通用请求
// 返回:
//   - *TencentChatRequest: 转换后的腾讯云请求体
	messages := make([]*TencentMessage, 0, len(request.Messages))
	for i := 0; i < len(request.Messages); i++ {
		message := request.Messages[i]
		messages = append(messages, &TencentMessage{
			Content: message.StringContent(),
			Role:    message.Role,
		})
	}
	var req = TencentChatRequest{
		Stream:   request.Stream,
		Messages: messages,
		Model:    &request.Model,
	}
	if request.TopP != nil {
		req.TopP = request.TopP
	}
	req.Temperature = request.Temperature
	return &req
}

// responseTencent2OpenAI 将腾讯云混元的非流式响应转换为 OpenAI 格式。
// 提取 ID、消息内容、finish_reason 和 token 使用量。
// 参数:
//   - response: 腾讯云响应结构体
// 返回:
//   - *dto.OpenAITextResponse: OpenAI 格式的文本响应
	fullTextResponse := dto.OpenAITextResponse{
		Id:      response.Id,
		Object:  "chat.completion",
		Created: common.GetTimestamp(),
		Usage: dto.Usage{
			PromptTokens:     response.Usage.PromptTokens,
			CompletionTokens: response.Usage.CompletionTokens,
			TotalTokens:      response.Usage.TotalTokens,
		},
	}
	if len(response.Choices) > 0 {
		choice := dto.OpenAITextResponseChoice{
			Index: 0,
			Message: dto.Message{
				Role:    "assistant",
				Content: response.Choices[0].Messages.Content,
			},
			FinishReason: response.Choices[0].FinishReason,
		}
		fullTextResponse.Choices = append(fullTextResponse.Choices, choice)
	}
	return &fullTextResponse
}

// streamResponseTencent2OpenAI 将腾讯云混元的流式响应转换为 OpenAI 格式。
// 提取增量内容（delta）和 finish_reason。
// 参数:
//   - TencentResponse: 腾讯云流式响应结构体
// 返回:
//   - *dto.ChatCompletionsStreamResponse: OpenAI 格式的流式响应
	response := dto.ChatCompletionsStreamResponse{
		Object:  "chat.completion.chunk",
		Created: common.GetTimestamp(),
		Model:   "tencent-hunyuan",
	}
	if len(TencentResponse.Choices) > 0 {
		var choice dto.ChatCompletionsStreamResponseChoice
		choice.Delta.SetContentString(TencentResponse.Choices[0].Delta.Content)
		if TencentResponse.Choices[0].FinishReason == "stop" {
			choice.FinishReason = &constant.FinishReasonStop
		}
		response.Choices = append(response.Choices, choice)
	}
	return &response
}

// tencentStreamHandler 处理腾讯云混元的流式响应。
// 逐行扫描 Server-Sent Events (SSE) 数据，将每个数据块转换为 OpenAI 格式后
// 通过 EventSource 流式写入客户端。
// 参数:
//   - c: Gin 上下文
//   - info: 中继信息
//   - resp: 上游 HTTP 响应
// 返回:
//   - *dto.Usage: token 使用量统计
//   - *types.NexusTokError: 处理过程中的错误信息
	var responseText string
	scanner := bufio.NewScanner(resp.Body)
	scanner.Split(bufio.ScanLines)

	helper.SetEventStreamHeaders(c)

	for scanner.Scan() {
		data := scanner.Text()
		if len(data) < 5 || !strings.HasPrefix(data, "data:") {
			continue
		}
		data = strings.TrimPrefix(data, "data:")

		var tencentResponse TencentChatResponse
		err := common.Unmarshal([]byte(data), &tencentResponse)
		if err != nil {
			common.SysLog("error unmarshalling stream response: " + err.Error())
			continue
		}

		response := streamResponseTencent2OpenAI(&tencentResponse)
		if len(response.Choices) != 0 {
			responseText += response.Choices[0].Delta.GetContentString()
		}

		err = helper.ObjectData(c, response)
		if err != nil {
			common.SysLog(err.Error())
		}
	}

	if err := scanner.Err(); err != nil {
		common.SysLog("error reading stream: " + err.Error())
	}

	helper.Done(c)

	service.CloseResponseBodyGracefully(resp)

	return service.ResponseText2Usage(c, responseText, info.UpstreamModelName, info.GetEstimatePromptTokens()), nil
}

// tencentHandler 处理腾讯云混元的非流式响应。
// 读取完整响应体，检查错误码，转换为 OpenAI 格式后写入客户端。
// 参数:
//   - c: Gin 上下文
//   - info: 中继信息
//   - resp: 上游 HTTP 响应
// 返回:
//   - *dto.Usage: token 使用量统计
//   - *types.NexusTokError: 处理过程中的错误信息
	var tencentSb TencentChatResponseSB
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	service.CloseResponseBodyGracefully(resp)
	err = json.Unmarshal(responseBody, &tencentSb)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if tencentSb.Response.Error.Code != 0 {
		return nil, types.WithOpenAIError(types.OpenAIError{
			Message: tencentSb.Response.Error.Message,
			Code:    tencentSb.Response.Error.Code,
		}, resp.StatusCode)
	}
	fullTextResponse := responseTencent2OpenAI(&tencentSb.Response)
	jsonResponse, err := common.Marshal(fullTextResponse)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	service.IOCopyBytesGracefully(c, resp, jsonResponse)
	return &fullTextResponse.Usage, nil
}

// parseTencentConfig 解析腾讯云渠道的配置字符串。
// 格式为 "AppID|SecretId|SecretKey"，三个字段用竖线分隔。
// 参数:
//   - config: 配置字符串
// 返回:
//   - appId: 腾讯云应用 ID
//   - secretId: 腾讯云 Secret ID
//   - secretKey: 腾讯云 Secret Key
//   - err: 解析失败时返回错误
	parts := strings.Split(config, "|")
	if len(parts) != 3 {
		err = errors.New("invalid tencent config")
		return
	}
	appId, err = strconv.ParseInt(parts[0], 10, 64)
	secretId = parts[1]
	secretKey = parts[2]
	return
}

// sha256hex 计算字符串的 SHA256 哈希值并返回十六进制编码。
// 用于 TC3 签名中的请求体哈希和规范请求哈希。
// 参数:
//   - s: 待哈希的字符串
// 返回:
//   - string: SHA256 十六进制编码字符串
	b := sha256.Sum256([]byte(s))
	return hex.EncodeToString(b[:])
}

// hmacSha256 使用 HMAC-SHA256 算法计算消息的认证码。
// 用于 TC3 签名中的派生密钥计算链。
// 参数:
//   - s: 待签名的消息
//   - key: HMAC 密钥
// 返回:
//   - string: HMAC-SHA256 认证码原始字节的字符串表示
	hashed := hmac.New(sha256.New, []byte(key))
	hashed.Write([]byte(s))
	return string(hashed.Sum(nil))
}

// getTencentSign 计算腾讯云 API 的 TC3-HMAC-SHA256 签名。
// 签名流程：
// 1. 构建规范请求串（CanonicalRequest）：HTTP 方法、URI、查询串、头部、签名头、请求体哈希
// 2. 构建待签名串（StringToSign）：算法、时间戳、凭据范围、规范请求哈希
// 3. 计算签名：通过 "TC3" + SecretKey 逐层派生密钥，对待签名串进行 HMAC-SHA256
// 4. 构建 Authorization 头
// 参数:
//   - req: 腾讯云请求体
//   - adaptor: 适配器实例（包含 Action、Timestamp 等元数据）
//   - secId: 腾讯云 Secret ID
//   - secKey: 腾讯云 Secret Key
// 返回:
//   - string: Authorization 头的值
	// build canonical request string
	host := "hunyuan.tencentcloudapi.com"
	httpRequestMethod := "POST"
	canonicalURI := "/"
	canonicalQueryString := ""
	canonicalHeaders := fmt.Sprintf("content-type:%s\nhost:%s\nx-tc-action:%s\n",
		"application/json", host, strings.ToLower(adaptor.Action))
	signedHeaders := "content-type;host;x-tc-action"
	payload, _ := json.Marshal(req)
	hashedRequestPayload := sha256hex(string(payload))
	canonicalRequest := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s",
		httpRequestMethod,
		canonicalURI,
		canonicalQueryString,
		canonicalHeaders,
		signedHeaders,
		hashedRequestPayload)
	// build string to sign
	algorithm := "TC3-HMAC-SHA256"
	requestTimestamp := strconv.FormatInt(adaptor.Timestamp, 10)
	timestamp, _ := strconv.ParseInt(requestTimestamp, 10, 64)
	t := time.Unix(timestamp, 0).UTC()
	// must be the format 2006-01-02, ref to package time for more info
	date := t.Format("2006-01-02")
	credentialScope := fmt.Sprintf("%s/%s/tc3_request", date, "hunyuan")
	hashedCanonicalRequest := sha256hex(canonicalRequest)
	string2sign := fmt.Sprintf("%s\n%s\n%s\n%s",
		algorithm,
		requestTimestamp,
		credentialScope,
		hashedCanonicalRequest)

	// sign string
	secretDate := hmacSha256(date, "TC3"+secKey)
	secretService := hmacSha256("hunyuan", secretDate)
	secretKey := hmacSha256("tc3_request", secretService)
	signature := hex.EncodeToString([]byte(hmacSha256(string2sign, secretKey)))

	// build authorization
	authorization := fmt.Sprintf("%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		algorithm,
		secId,
		credentialScope,
		signedHeaders,
		signature)
	return authorization
}
