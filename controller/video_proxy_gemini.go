// Package controller - video_proxy_gemini.go
// 该文件实现了 Gemini 和 Vertex AI 视频 URL 解析逻辑
//
// 功能包括：
// - Gemini 视频 URL 提取（从任务数据或上游响应）
// - Vertex AI 视频 URL 提取（支持 base64 编码和远程 URL）
// - API Key 附加到 URL 查询参数
// - 多层嵌套 JSON 结构解析
//
// 主要函数：
// - getGeminiVideoURL：获取 Gemini 视频 URL
// - getVertexVideoURL：获取 Vertex AI 视频 URL
// - extractGeminiVideoURLFrom*：从不同数据源提取 Gemini 视频 URL
// - extractVertexVideoURLFrom*：从不同数据源提取 Vertex AI 视频 URL
package controller

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/relay"
)

// getGeminiVideoURL 获取 Gemini 视频 URL
//
// 优先从任务数据中提取，如果不存在则调用上游 API 查询
// 参数：
//   - channel: 渠道信息
//   - task: 任务信息
//   - apiKey: Gemini API Key
//
// 返回：
//   - string: 视频 URL（已附加 API Key）
//   - error: 获取失败时返回错误
func getGeminiVideoURL(channel *model.Channel, task *model.Task, apiKey string) (string, error) {
	if channel == nil || task == nil {
		return "", fmt.Errorf("invalid channel or task")
	}

	if url := extractGeminiVideoURLFromTaskData(task); url != "" {
		return ensureAPIKey(url, apiKey), nil
	}

	baseURL := constant.ChannelBaseURLs[channel.Type]
	if channel.GetBaseURL() != "" {
		baseURL = channel.GetBaseURL()
	}

	adaptor := relay.GetTaskAdaptor(constant.TaskPlatform(strconv.Itoa(channel.Type)))
	if adaptor == nil {
		return "", fmt.Errorf("gemini task adaptor not found")
	}

	if apiKey == "" {
		return "", fmt.Errorf("api key not available for task")
	}

	proxy := channel.GetSetting().Proxy
	resp, err := adaptor.FetchTask(baseURL, apiKey, map[string]any{
		"task_id": task.GetUpstreamTaskID(),
		"action":  task.Action,
	}, proxy)
	if err != nil {
		return "", fmt.Errorf("fetch task failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read task response failed: %w", err)
	}

	taskInfo, parseErr := adaptor.ParseTaskResult(body)
	if parseErr == nil && taskInfo != nil && taskInfo.RemoteUrl != "" {
		return ensureAPIKey(taskInfo.RemoteUrl, apiKey), nil
	}

	if url := extractGeminiVideoURLFromPayload(body); url != "" {
		return ensureAPIKey(url, apiKey), nil
	}

	if parseErr != nil {
		return "", fmt.Errorf("parse task result failed: %w", parseErr)
	}

	return "", fmt.Errorf("gemini video url not found")
}

// extractGeminiVideoURLFromTaskData 从任务数据中提取 Gemini 视频 URL
func extractGeminiVideoURLFromTaskData(task *model.Task) string {
	if task == nil || len(task.Data) == 0 {
		return ""
	}
	var payload map[string]any
	if err := common.Unmarshal(task.Data, &payload); err != nil {
		return ""
	}
	return extractGeminiVideoURLFromMap(payload)
}

// extractGeminiVideoURLFromPayload 从上游响应体中提取 Gemini 视频 URL
func extractGeminiVideoURLFromPayload(body []byte) string {
	var payload map[string]any
	if err := common.Unmarshal(body, &payload); err != nil {
		return ""
	}
	return extractGeminiVideoURLFromMap(payload)
}

// extractGeminiVideoURLFromMap 从 map 结构中提取 Gemini 视频 URL
//
// 支持两种格式：
// - 顶层 uri 字段
// - response.generateVideoResponse.generatedSamples[].video.uri
func extractGeminiVideoURLFromMap(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	if uri, ok := payload["uri"].(string); ok && uri != "" {
		return uri
	}
	if resp, ok := payload["response"].(map[string]any); ok {
		if uri := extractGeminiVideoURLFromResponse(resp); uri != "" {
			return uri
		}
	}
	return ""
}

// extractGeminiVideoURLFromResponse 从 response 对象中提取 Gemini 视频 URL
//
// 支持多种格式：
// - generateVideoResponse.generatedSamples[].video.uri
// - videos[].uri
// - video 字符串
// - uri 字符串
func extractGeminiVideoURLFromResponse(resp map[string]any) string {
	if resp == nil {
		return ""
	}
	if gvr, ok := resp["generateVideoResponse"].(map[string]any); ok {
		if uri := extractGeminiVideoURLFromGeneratedSamples(gvr); uri != "" {
			return uri
		}
	}
	if videos, ok := resp["videos"].([]any); ok {
		for _, video := range videos {
			if vm, ok := video.(map[string]any); ok {
				if uri, ok := vm["uri"].(string); ok && uri != "" {
					return uri
				}
			}
		}
	}
	if uri, ok := resp["video"].(string); ok && uri != "" {
		return uri
	}
	if uri, ok := resp["uri"].(string); ok && uri != "" {
		return uri
	}
	return ""
}

// extractGeminiVideoURLFromGeneratedSamples 从 generatedSamples 数组中提取视频 URL
func extractGeminiVideoURLFromGeneratedSamples(gvr map[string]any) string {
	if gvr == nil {
		return ""
	}
	if samples, ok := gvr["generatedSamples"].([]any); ok {
		for _, sample := range samples {
			if sm, ok := sample.(map[string]any); ok {
				if video, ok := sm["video"].(map[string]any); ok {
					if uri, ok := video["uri"].(string); ok && uri != "" {
						return uri
					}
				}
			}
		}
	}
	return ""
}

// getVertexVideoURL 获取 Vertex AI 视频 URL
//
// 优先使用存储的 URL，然后尝试从任务数据提取，最后调用上游 API 查询
// 参数：
//   - channel: 渠道信息
//   - task: 任务信息
//
// 返回：
//   - string: 视频 URL（可能是 HTTP URL 或 data: URL）
//   - error: 获取失败时返回错误
func getVertexVideoURL(channel *model.Channel, task *model.Task) (string, error) {
	if channel == nil || task == nil {
		return "", fmt.Errorf("invalid channel or task")
	}
	if url := strings.TrimSpace(task.GetResultURL()); url != "" && !isTaskProxyContentURL(url, task.TaskID) {
		return url, nil
	}
	if url := extractVertexVideoURLFromTaskData(task); url != "" {
		return url, nil
	}

	baseURL := constant.ChannelBaseURLs[channel.Type]
	if channel.GetBaseURL() != "" {
		baseURL = channel.GetBaseURL()
	}

	adaptor := relay.GetTaskAdaptor(constant.TaskPlatform(strconv.Itoa(channel.Type)))
	if adaptor == nil {
		return "", fmt.Errorf("vertex task adaptor not found")
	}

	key := getVertexTaskKey(channel, task)
	if key == "" {
		return "", fmt.Errorf("vertex key not available for task")
	}

	resp, err := adaptor.FetchTask(baseURL, key, map[string]any{
		"task_id": task.GetUpstreamTaskID(),
		"action":  task.Action,
	}, channel.GetSetting().Proxy)
	if err != nil {
		return "", fmt.Errorf("fetch task failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read task response failed: %w", err)
	}

	taskInfo, parseErr := adaptor.ParseTaskResult(body)
	if parseErr == nil && taskInfo != nil && strings.TrimSpace(taskInfo.Url) != "" {
		return taskInfo.Url, nil
	}
	if url := extractVertexVideoURLFromPayload(body); url != "" {
		return url, nil
	}
	if parseErr != nil {
		return "", fmt.Errorf("parse task result failed: %w", parseErr)
	}
	return "", fmt.Errorf("vertex video url not found")
}

// isTaskProxyContentURL 检查 URL 是否为任务代理内容 URL
//
// 用于避免将代理 URL 作为最终视频 URL 返回
func isTaskProxyContentURL(url string, taskID string) bool {
	if strings.TrimSpace(url) == "" || strings.TrimSpace(taskID) == "" {
		return false
	}
	return strings.Contains(url, "/v1/videos/"+taskID+"/content")
}

// getVertexTaskKey 获取 Vertex AI 任务的认证密钥
//
// 优先使用任务存储的密钥，然后使用渠道的密钥
func getVertexTaskKey(channel *model.Channel, task *model.Task) string {
	if task != nil {
		if key := strings.TrimSpace(task.PrivateData.Key); key != "" {
			return key
		}
	}
	if channel == nil {
		return ""
	}
	keys := channel.GetKeys()
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key != "" {
			return key
		}
	}
	return strings.TrimSpace(channel.Key)
}

// extractVertexVideoURLFromTaskData 从任务数据中提取 Vertex AI 视频 URL
func extractVertexVideoURLFromTaskData(task *model.Task) string {
	if task == nil || len(task.Data) == 0 {
		return ""
	}
	return extractVertexVideoURLFromPayload(task.Data)
}

// extractVertexVideoURLFromPayload 从上游响应体中提取 Vertex AI 视频 URL
//
// 支持多种格式：
// - response.videos[].bytesBase64Encoded（转为 data: URL）
// - response.bytesBase64Encoded（转为 data: URL）
// - response.video（可能是 data: URL、HTTP URL 或 base64 数据）
func extractVertexVideoURLFromPayload(body []byte) string {
	var payload map[string]any
	if err := common.Unmarshal(body, &payload); err != nil {
		return ""
	}
	resp, ok := payload["response"].(map[string]any)
	if !ok || resp == nil {
		return ""
	}

	if videos, ok := resp["videos"].([]any); ok && len(videos) > 0 {
		if video, ok := videos[0].(map[string]any); ok && video != nil {
			if b64, _ := video["bytesBase64Encoded"].(string); strings.TrimSpace(b64) != "" {
				mime, _ := video["mimeType"].(string)
				enc, _ := video["encoding"].(string)
				return buildVideoDataURL(mime, enc, b64)
			}
		}
	}
	if b64, _ := resp["bytesBase64Encoded"].(string); strings.TrimSpace(b64) != "" {
		enc, _ := resp["encoding"].(string)
		return buildVideoDataURL("", enc, b64)
	}
	if video, _ := resp["video"].(string); strings.TrimSpace(video) != "" {
		if strings.HasPrefix(video, "data:") || strings.HasPrefix(video, "http://") || strings.HasPrefix(video, "https://") {
			return video
		}
		enc, _ := resp["encoding"].(string)
		return buildVideoDataURL("", enc, video)
	}
	return ""
}

// buildVideoDataURL 构建 data: URL 格式的视频
//
// 参数：
//   - mimeType: MIME 类型（如 video/mp4）
//   - encoding: 编码格式（如 mp4、base64）
//   - base64Data: base64 编码的视频数据
//
// 返回：
//   - string: data: URL 格式的字符串
func buildVideoDataURL(mimeType string, encoding string, base64Data string) string {
	mime := strings.TrimSpace(mimeType)
	if mime == "" {
		enc := strings.TrimSpace(encoding)
		if enc == "" {
			enc = "mp4"
		}
		if strings.Contains(enc, "/") {
			mime = enc
		} else {
			mime = "video/" + enc
		}
	}
	return "data:" + mime + ";base64," + base64Data
}

// ensureAPIKey 确保 URL 包含 API Key
//
// 如果 URL 中已包含 key= 参数，直接返回
// 否则在查询参数中添加 key={apiKey}
func ensureAPIKey(uri, key string) string {
	if key == "" || uri == "" {
		return uri
	}
	if strings.Contains(uri, "key=") {
		return uri
	}
	if strings.Contains(uri, "?") {
		return fmt.Sprintf("%s&key=%s", uri, key)
	}
	return fmt.Sprintf("%s?key=%s", uri, key)
}
