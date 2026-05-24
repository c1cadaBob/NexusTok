// amp - gemini_bridge.go
// AMP CLI Gemini 路径格式桥接处理器。
// 该模块将 AMP CLI 的非标准 Gemini API 路径格式转换为标准格式：
//   - AMP 格式：/publishers/google/models/{model}:{method}
//   - 标准格式：/models/{model}:{method}
//
// 通过从路径中提取 model:method 部分并设置为 :action 参数，
// 使标准 Gemini 处理器能够正确处理 AMP CLI 的请求。
package amp

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// createGeminiBridgeHandler 创建一个桥接处理器，将 AMP CLI 的非标准 Gemini 路径
// 重写为标准格式。
//
// 路径转换逻辑：
//  1. 从 catch-all 参数中获取完整路径
//  2. 查找 "/models/" 前缀，提取其后的 model:method 部分
//  3. 如果存在模型映射，替换为映射后的模型名
//  4. 将提取的部分设置为 :action 参数供 Gemini 处理器使用
//
// 参数 handler 应为期望 :action 参数的 Gemini 兼容处理器。
func createGeminiBridgeHandler(handler gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get the full path from the catch-all parameter
		path := c.Param("path")

		// Extract model:method from AMP CLI path format
		// Example: /publishers/google/models/gemini-3-pro-preview:streamGenerateContent
		const modelsPrefix = "/models/"
		if idx := strings.Index(path, modelsPrefix); idx >= 0 {
			// Extract everything after modelsPrefix
			actionPart := path[idx+len(modelsPrefix):]

			// Check if model was mapped by FallbackHandler
			if mappedModel, exists := c.Get(MappedModelContextKey); exists {
				if strModel, ok := mappedModel.(string); ok && strModel != "" {
					// Replace the model part in the action
					// actionPart is like "model-name:method"
					if colonIdx := strings.Index(actionPart, ":"); colonIdx > 0 {
						method := actionPart[colonIdx:] // ":method"
						actionPart = strModel + method
					}
				}
			}

			// Set this as the :action parameter that the Gemini handler expects
			c.Params = append(c.Params, gin.Param{
				Key:   "action",
				Value: actionPart,
			})

			// Call the handler
			handler(c)
			return
		}

		// If we can't parse the path, return 400
		c.JSON(400, gin.H{
			"error": "Invalid Gemini API path format",
		})
	}
}
