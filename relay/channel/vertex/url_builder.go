// Package vertex 实现 Google Vertex AI 渠道的 URL 构建工具
package vertex

import (
	"fmt"     // 格式化输出
	"strings" // 字符串操作
)

// API 版本常量
const (
	DefaultAPIVersion    = "v1"      // 默认 API 版本
	OpenSourceAPIVersion = "v1beta1" // 开源模型 API 版本
	PublisherGoogle      = "google"  // Google 发布者
	PublisherAnthropic   = "anthropic" // Anthropic 发布者
)

// normalizeVertexBaseURL 标准化 Vertex AI 基础 URL
// 去除首尾空格和尾部斜杠
func normalizeVertexBaseURL(baseURL string) string {
	return strings.TrimRight(strings.TrimSpace(baseURL), "/")
}

// normalizeVertexRegion 标准化区域名称
// 空区域默认为 "global"
func normalizeVertexRegion(region string) string {
	region = strings.TrimSpace(region)
	if region == "" {
		return "global"
	}
	return region
}

// appendVertexAPIVersion 在 URL 末尾追加 API 版本
// 避免重复追加已存在的版本号
func appendVertexAPIVersion(baseURL, version string) string {
	version = strings.Trim(strings.TrimSpace(version), "/")
	if version == "" {
		return baseURL
	}
	if strings.HasSuffix(baseURL, "/"+version) {
		return baseURL
	}
	return baseURL + "/" + version
}

// BuildAPIBaseURL 构建 Vertex AI API 基础 URL
// 根据项目 ID 和区域构建完整的 API 端点地址
//
// 参数：
//   - baseURL: 自定义基础 URL（可选）
//   - version: API 版本
//   - projectID: Google Cloud 项目 ID
//   - region: 区域名称
//
// 返回值：
//   - string: 完整的 API 基础 URL
func BuildAPIBaseURL(baseURL, version, projectID, region string) string {
	if normalized := normalizeVertexBaseURL(baseURL); normalized != "" {
		normalized = appendVertexAPIVersion(normalized, version)

		region = normalizeVertexRegion(region)
		if strings.TrimSpace(projectID) != "" {
			normalized = fmt.Sprintf("%s/projects/%s/locations/%s", normalized, projectID, region)
		}
		return normalized
	}

	region = normalizeVertexRegion(region)
	if strings.TrimSpace(projectID) == "" {
		if region == "global" {
			return fmt.Sprintf("https://aiplatform.googleapis.com/%s", version)
		}
		return fmt.Sprintf("https://%s-aiplatform.googleapis.com/%s", region, version)
	}

	if region == "global" {
		return fmt.Sprintf("https://aiplatform.googleapis.com/%s/projects/%s/locations/global", version, projectID)
	}
	return fmt.Sprintf("https://%s-aiplatform.googleapis.com/%s/projects/%s/locations/%s", region, version, projectID, region)
}

// BuildPublisherModelURL 构建发布者模型 URL
// 用于调用特定发布者（Google/Anthropic）的模型
func BuildPublisherModelURL(baseURL, version, projectID, region, publisher, modelName, action string) string {
	return fmt.Sprintf(
		"%s/publishers/%s/models/%s:%s",
		BuildAPIBaseURL(baseURL, version, projectID, region),
		publisher,
		modelName,
		action,
	)
}

// BuildGoogleModelURL 构建 Google 模型 URL
func BuildGoogleModelURL(baseURL, version, projectID, region, modelName, action string) string {
	return BuildPublisherModelURL(baseURL, version, projectID, region, PublisherGoogle, modelName, action)
}

// BuildAnthropicModelURL 构建 Anthropic 模型 URL
func BuildAnthropicModelURL(baseURL, version, projectID, region, modelName, action string) string {
	return BuildPublisherModelURL(baseURL, version, projectID, region, PublisherAnthropic, modelName, action)
}

// BuildOpenSourceChatCompletionsURL 构建开源模型聊天完成 URL
// 用于调用 Meta Llama 等开源模型
func BuildOpenSourceChatCompletionsURL(baseURL, projectID, region string) string {
	return fmt.Sprintf(
		"%s/endpoints/openapi/chat/completions",
		BuildAPIBaseURL(baseURL, OpenSourceAPIVersion, projectID, region),
	)
}
