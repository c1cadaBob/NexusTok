// gemini - billing.go
// Gemini/Veo 视频生成的计费相关工具函数。
// 负责解析视频时长、分辨率等参数，并计算对应的计费系数。
// 支持从 metadata、标准请求字段和默认值三个来源解析参数。
package gemini

import (
	"strconv"
	"strings"
)

// ParseVeoDurationSeconds 从 metadata 中解析视频时长（秒）。
// 如果 metadata 为 nil 或未指定时长，返回默认值 8 秒。
//
// 参数：
//   - metadata: 请求的元数据 map
//
// 返回：视频时长（秒），无效值时返回默认值 8。
func ParseVeoDurationSeconds(metadata map[string]any) int {
	if metadata == nil {
		return 8
	}
	v, ok := metadata["durationSeconds"]
	if !ok {
		return 8
	}
	switch n := v.(type) {
	case float64:
		if int(n) > 0 {
			return int(n)
		}
	case int:
		if n > 0 {
			return n
		}
	}
	return 8
}

// ParseVeoResolution 从 metadata 中解析视频分辨率。
// 返回小写的分辨率字符串（如 "720p"、"1080p"、"4k"）。
// 如果未指定，返回默认值 "720p"。
func ParseVeoResolution(metadata map[string]any) string {
	if metadata == nil {
		return "720p"
	}
	v, ok := metadata["resolution"]
	if !ok {
		return "720p"
	}
	if s, ok := v.(string); ok && s != "" {
		return strings.ToLower(s)
	}
	return "720p"
}

// ResolveVeoDuration 解析并返回最终有效的视频时长。
// 按优先级依次尝试：
//  1. metadata["durationSeconds"]
//  2. 标准请求的 duration 字段
//  3. 标准请求的 seconds 字段（字符串格式）
//  4. 默认值 8 秒
func ResolveVeoDuration(metadata map[string]any, stdDuration int, stdSeconds string) int {
	if metadata != nil {
		if _, exists := metadata["durationSeconds"]; exists {
			if d := ParseVeoDurationSeconds(metadata); d > 0 {
				return d
			}
		}
	}
	if stdDuration > 0 {
		return stdDuration
	}
	if s, err := strconv.Atoi(stdSeconds); err == nil && s > 0 {
		return s
	}
	return 8
}

// ResolveVeoResolution 解析并返回最终有效的视频分辨率。
// 按优先级依次尝试：
//  1. metadata["resolution"]
//  2. 标准请求的 size 字段（通过 SizeToVeoResolution 转换）
//  3. 默认值 "720p"
func ResolveVeoResolution(metadata map[string]any, stdSize string) string {
	if metadata != nil {
		if _, exists := metadata["resolution"]; exists {
			if r := ParseVeoResolution(metadata); r != "" {
				return r
			}
		}
	}
	if stdSize != "" {
		return SizeToVeoResolution(stdSize)
	}
	return "720p"
}

// SizeToVeoResolution 将 "WxH" 格式的尺寸字符串转换为 Veo 分辨率标签。
//
// 转换规则（基于较长边）：
//   - >= 3840 -> "4k"
//   - >= 1920 -> "1080p"
//   - 其他    -> "720p"
func SizeToVeoResolution(size string) string {
	parts := strings.SplitN(strings.ToLower(size), "x", 2)
	if len(parts) != 2 {
		return "720p"
	}
	w, _ := strconv.Atoi(parts[0])
	h, _ := strconv.Atoi(parts[1])
	maxDim := w
	if h > maxDim {
		maxDim = h
	}
	if maxDim >= 3840 {
		return "4k"
	}
	if maxDim >= 1920 {
		return "1080p"
	}
	return "720p"
}

// SizeToVeoAspectRatio 将 "WxH" 格式的尺寸字符串转换为 Veo 宽高比。
//
// 转换规则：
//   - 高度 > 宽度 -> "9:16"（竖屏）
//   - 其他        -> "16:9"（横屏，默认）
func SizeToVeoAspectRatio(size string) string {
	parts := strings.SplitN(strings.ToLower(size), "x", 2)
	if len(parts) != 2 {
		return "16:9"
	}
	w, _ := strconv.Atoi(parts[0])
	h, _ := strconv.Atoi(parts[1])
	if w <= 0 || h <= 0 {
		return "16:9"
	}
	if h > w {
		return "9:16"
	}
	return "16:9"
}

// VeoResolutionRatio 返回给定分辨率的计费倍率。
// 标准分辨率（720p、1080p）返回 1.0。
// 4K 分辨率根据模型返回不同的倍率（基于 Google 官方定价）：
//   - veo-3.1-fast-generate: 2.333333（$0.35 / $0.15）
//   - veo-3.1-generate: 1.5（$0.60 / $0.40）
//   - 其他模型: 1.0（不支持 4K，返回默认值）
func VeoResolutionRatio(modelName, resolution string) float64 {
	if resolution != "4k" {
		return 1.0
	}
	// 4K 倍率基于 Vertex AI 官方定价（视频+音频基础价格）
	if strings.Contains(modelName, "3.1-fast-generate") {
		return 2.333333
	}
	if strings.Contains(modelName, "3.1-generate") || strings.Contains(modelName, "3.1") {
		return 1.5
	}
	return 1.0
}
