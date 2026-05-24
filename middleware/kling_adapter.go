// Package middleware - kling_adapter.go
// 该文件实现了可灵（Kling）API 请求适配器中间件
//
// 功能说明：
// - 将可灵官方 API 请求格式转换为 NexusTok 统一视频生成请求格式
// - 支持文生视频和图生视频两种模式
// - 同时支持 model_name 和 model 两种模型字段名
//
// 请求转换流程：
// 1. 解析原始请求体
// 2. 提取 model_name（或 model）和 prompt
// 3. 构造统一格式的请求体（model、prompt、metadata）
// 4. 重写请求路径为 /v1/video/generations
// 5. 根据是否有 image 参数设置 action（文生视频/图生视频）
package middleware

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"

	"github.com/gin-gonic/gin"
)

// KlingRequestConvert 可灵 API 请求适配器中间件工厂函数
// 创建并返回一个 Gin 中间件，用于将可灵 API 请求转换为统一视频生成格式
//
// 支持的模型字段：
// - model_name：可灵原始字段名
// - model：备选字段名
//
// 转换后的请求体格式：
//
//	{
//	  "model": "<model_name 或 model>",
//	  "prompt": "<prompt>",
//	  "metadata": { ...原始请求参数 }
//	}
//
// 注意：如果请求体解析失败，中间件会跳过转换，直接传递原始请求
func KlingRequestConvert() func(c *gin.Context) {
	return func(c *gin.Context) {
		var originalReq map[string]interface{}
		if err := common.UnmarshalBodyReusable(c, &originalReq); err != nil {
			c.Next()
			return
		}

		// Support both model_name and model fields
		model, _ := originalReq["model_name"].(string)
		if model == "" {
			model, _ = originalReq["model"].(string)
		}
		prompt, _ := originalReq["prompt"].(string)

		unifiedReq := map[string]interface{}{
			"model":    model,
			"prompt":   prompt,
			"metadata": originalReq,
		}

		jsonData, err := json.Marshal(unifiedReq)
		if err != nil {
			c.Next()
			return
		}

		// Rewrite request body and path
		c.Request.Body = io.NopCloser(bytes.NewBuffer(jsonData))
		c.Request.URL.Path = "/v1/video/generations"
		if image, ok := originalReq["image"]; !ok || image == "" {
			c.Set("action", constant.TaskActionTextGenerate)
		}

		// We have to reset the request body for the next handlers
		c.Set(common.KeyRequestBody, jsonData)
		c.Next()
	}
}
