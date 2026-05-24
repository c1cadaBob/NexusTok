// Package relay - websocket.go
// 本文件实现了 WebSocket（实时通信）请求的中继处理逻辑。
// WssHelper 负责建立与上游 AI 服务的 WebSocket 连接，
// 并通过适配器处理双向通信，最终完成使用量统计和计费结算。
package relay

import (
	"fmt"

	"github.com/c1cada/NexusTok/dto"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/service"
	"github.com/c1cada/NexusTok/types"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// WssHelper 是 WebSocket 实时通信请求的中继处理函数。
// 处理流程：
//  1. 初始化渠道元数据。
//  2. 获取并初始化对应 API 类型的适配器。
//  3. 通过适配器发送请求，建立与上游的 WebSocket 连接。
//  4. 将上游 WebSocket 连接存储到 info.TargetWs，并在处理完成后关闭。
//  5. 通过适配器处理双向通信（DoResponse）。
//  6. 调用 PostWssConsumeQuota 进行 WebSocket 使用量计费。
//
// 参数：
//   - c: Gin 上下文
//   - info: 中继信息，包含客户端 WebSocket 连接（ClientWs）
//
// 返回值：
//   - newAPIError: 处理过程中的错误，成功时为 nil
func WssHelper(c *gin.Context, info *relaycommon.RelayInfo) (newAPIError *types.NexusTokError) {
	info.InitChannelMeta(c)

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)
	//var requestBody io.Reader
	//firstWssRequest, _ := c.Get("first_wss_request")
	//requestBody = bytes.NewBuffer(firstWssRequest.([]byte))

	statusCodeMappingStr := c.GetString("status_code_mapping")
	resp, err := adaptor.DoRequest(c, info, nil)
	if err != nil {
		return types.NewError(err, types.ErrorCodeDoRequestFailed)
	}

	if resp != nil {
		info.TargetWs = resp.(*websocket.Conn)
		defer info.TargetWs.Close()
	}

	usage, newAPIError := adaptor.DoResponse(c, nil, info)
	if newAPIError != nil {
		// reset status code 重置状态码
		service.ResetStatusCode(newAPIError, statusCodeMappingStr)
		return newAPIError
	}
	service.PostWssConsumeQuota(c, info, info.UpstreamModelName, usage.(*dto.RealtimeUsage), "")
	return nil
}
