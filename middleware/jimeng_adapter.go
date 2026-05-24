// Package middleware - jimeng_adapter.go
// 该文件实现了即梦（Jimeng）API 请求适配器中间件
//
// 功能说明：
// - 将即梦官方 API 请求格式转换为 NexusTok 统一视频生成请求格式
// - 支持文生视频和图生视频两种模式
// - 支持任务状态查询（CVSync2AsyncGetResult 操作）
//
// 请求转换流程：
// 1. 从查询参数获取 Action（操作类型）
// 2. 解析原始请求体，提取 req_key（模型）和 prompt（提示词）
// 3. 构造统一格式的请求体（model、prompt、metadata）
// 4. 根据 Action 类型重写请求路径：
//    - 生成请求：/v1/video/generations
//    - 查询请求：/v1/video/generations/{task_id}
package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	relayconstant "github.com/c1cada/NexusTok/relay/constant"
	"github.com/gin-gonic/gin"
)

// JimengRequestConvert 即梦 API 请求适配器中间件工厂函数
// 创建并返回一个 Gin 中间件，用于将即梦 API 请求转换为统一视频生成格式
//
// 支持的操作：
// - 生成请求：将即梦的 req_key + prompt 转换为统一的 model + prompt + metadata
// - CVSync2AsyncGetResult：查询任务状态，重写路径为 /v1/video/generations/{task_id}
//
// 转换后的请求体格式：
//
//	{
//	  "model": "<req_key>",
//	  "prompt": "<prompt>",
//	  "metadata": { ...原始请求参数 }
//	}
func JimengRequestConvert() func(c *gin.Context) {
	return func(c *gin.Context) {
		action := c.Query("Action")
		if action == "" {
			abortWithOpenAiMessage(c, http.StatusBadRequest, "Action query parameter is required")
			return
		}

		// Handle Jimeng official API request
		var originalReq map[string]interface{}
		if err := common.UnmarshalBodyReusable(c, &originalReq); err != nil {
			abortWithOpenAiMessage(c, http.StatusBadRequest, "Invalid request body")
			return
		}
		model, _ := originalReq["req_key"].(string)
		prompt, _ := originalReq["prompt"].(string)

		unifiedReq := map[string]interface{}{
			"model":    model,
			"prompt":   prompt,
			"metadata": originalReq,
		}

		jsonData, err := json.Marshal(unifiedReq)
		if err != nil {
			abortWithOpenAiMessage(c, http.StatusInternalServerError, "Failed to marshal request body")
			return
		}

		// Update request body
		c.Request.Body = io.NopCloser(bytes.NewBuffer(jsonData))
		c.Set(common.KeyRequestBody, jsonData)

		if image, ok := originalReq["image"]; !ok || image == "" {
			c.Set("action", constant.TaskActionTextGenerate)
		}

		c.Request.URL.Path = "/v1/video/generations"

		if action == "CVSync2AsyncGetResult" {
			taskId, ok := originalReq["task_id"].(string)
			if !ok || taskId == "" {
				abortWithOpenAiMessage(c, http.StatusBadRequest, "task_id is required for CVSync2AsyncGetResult")
				return
			}
			c.Request.URL.Path = "/v1/video/generations/" + taskId
			c.Request.Method = http.MethodGet
			c.Set("task_id", taskId)
			c.Set("relay_mode", relayconstant.RelayModeVideoFetchByID)
		}
		c.Next()
	}
}
