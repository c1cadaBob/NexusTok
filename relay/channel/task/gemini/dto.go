// gemini - dto.go
// Gemini/Veo 视频生成 API 的数据传输对象（DTO）定义。
// 定义了 Veo predictLongRunning 端点的请求和响应结构体。
// 这些结构体被 Gemini 和 Vertex 两个适配器共享使用。
package gemini

// VeoImageInput Veo 图片输入结构体，用于图生视频模式。
// 包含 Base64 编码的图片数据和 MIME 类型。
type VeoImageInput struct {
	BytesBase64Encoded string `json:"bytesBase64Encoded"` // Base64 编码的图片数据
	MimeType           string `json:"mimeType"`           // 图片 MIME 类型（如 "image/png"）
}

// VeoInstance Veo 请求实例，代表一个视频生成任务。
// 包含提示词和可选的参考图片。
type VeoInstance struct {
	Prompt string         `json:"prompt"`           // 视频生成的文本提示词
	Image  *VeoImageInput `json:"image,omitempty"`  // 可选的参考图片（用于图生视频）
	// TODO: support referenceImages (style/asset references, up to 3 images)
	// TODO: support lastFrame (first+last frame interpolation, Veo 3.1)
}

// VeoParameters Veo 生成参数，控制视频的各项属性。
type VeoParameters struct {
	SampleCount        int    `json:"sampleCount"`                  // 生成样本数量（目前固定为 1）
	DurationSeconds    int    `json:"durationSeconds,omitempty"`    // 视频时长（秒）
	AspectRatio        string `json:"aspectRatio,omitempty"`        // 宽高比（如 "16:9"、"9:16"）
	Resolution         string `json:"resolution,omitempty"`         // 分辨率（如 "720p"、"1080p"、"4k"）
	NegativePrompt     string `json:"negativePrompt,omitempty"`     // 负面提示词（不希望出现的内容）
	PersonGeneration   string `json:"personGeneration,omitempty"`   // 人物生成策略
	StorageUri         string `json:"storageUri,omitempty"`         // GCS 存储 URI（用于结果存储）
	CompressionQuality string `json:"compressionQuality,omitempty"` // 压缩质量
	ResizeMode         string `json:"resizeMode,omitempty"`         // 缩放模式
	Seed               *int   `json:"seed,omitempty"`               // 随机种子（用于可复现生成）
	GenerateAudio      *bool  `json:"generateAudio,omitempty"`      // 是否生成音频
}

// VeoRequestPayload Veo predictLongRunning 端点的顶层请求体。
// 被 Gemini 和 Vertex 两个适配器共享使用。
type VeoRequestPayload struct {
	Instances  []VeoInstance  `json:"instances"`            // 请求实例列表
	Parameters *VeoParameters `json:"parameters,omitempty"` // 生成参数
}

// submitResponse Veo 任务提交的响应结构体。
// 包含异步操作的名称（operation name）。
type submitResponse struct {
	Name string `json:"name"` // 操作名称，用于后续轮询任务状态
}

// operationVideo 操作响应中的视频数据结构。
type operationVideo struct {
	MimeType           string `json:"mimeType"`           // 视频 MIME 类型
	BytesBase64Encoded string `json:"bytesBase64Encoded"` // Base64 编码的视频数据
	Encoding           string `json:"encoding"`           // 编码格式
}

// operationResponse Veo 操作轮询的响应结构体。
// 表示一个异步操作的当前状态，包含完成状态、结果数据或错误信息。
type operationResponse struct {
	Name string `json:"name"` // 操作名称
	Done bool   `json:"done"` // 操作是否完成
	Response struct {
		Type                  string           `json:"@type"`                // 响应类型标识
		RaiMediaFilteredCount int              `json:"raiMediaFilteredCount"` // RAI 过滤的媒体数量
		Videos                []operationVideo `json:"videos"`               // 视频列表（新格式）
		BytesBase64Encoded    string           `json:"bytesBase64Encoded"`   // Base64 编码的视频（旧格式）
		Encoding              string           `json:"encoding"`             // 编码格式
		Video                 string           `json:"video"`                // Base64 编码的视频（变体格式）
		GenerateVideoResponse struct {
			GeneratedVideos []struct {
				Video struct {
					URI string `json:"uri"` // 视频下载 URI
				} `json:"video"`
			} `json:"generatedVideos"` // 生成的视频列表（URI 格式）
		} `json:"generateVideoResponse"` // 视频生成响应
	} `json:"response"` // 操作结果
	Error struct {
		Message string `json:"message"` // 错误消息
	} `json:"error"` // 错误信息
}
