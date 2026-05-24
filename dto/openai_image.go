// Package dto - openai_image.go
// 该文件定义了 OpenAI 图像生成 API 的数据传输对象
//
// 主要结构体：
// - ImageRequest：图像生成请求（支持 DALL-E、GPT-Image 等模型）
// - ImageResponse：图像生成响应
// - ImageData：单个生成的图像数据
//
// 特性说明：
// - 支持自定义 JSON 序列化（UnmarshalJSON/MarshalJSON）
// - 未知字段通过 Extra map 透传（兼容各厂商扩展参数）
// - GetTokenCountMeta 根据图像尺寸和质量计算价格系数
// - N 参数不在此处处理，由 image_handler.go 或渠道适配器单独处理
package dto

import (
	"encoding/json"
	"reflect"
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/types"

	"github.com/gin-gonic/gin"
)

// ImageRequest 图像生成请求结构体
// Model：目标模型名称（如 dall-e-3、gpt-image-1 等）
// Prompt：图像生成提示词（必填）
// N：生成图像数量
// Size：图像尺寸（如 1024x1024、1024x1792 等）
// Quality：图像质量（standard/hd）
// ResponseFormat：响应格式（url/b64_json）
// Style：图像风格
// User：用户标识
// ExtraFields：额外字段（透传）
// Background/Moderation/OutputFormat/OutputCompression/PartialImages：GPT-Image 扩展参数
// Images/Mask/InputFidelity：编辑模式参数（垫图、遮罩等）
// Watermark：是否添加水印
// WatermarkEnabled/UserId/Image：智谱 4V 兼容参数
// Extra：未知字段存储（自定义 UnmarshalJSON 捕获的额外参数，序列化时不合并）
type ImageRequest struct {
	Model             string          `json:"model"`
	Prompt            string          `json:"prompt" binding:"required"`
	N                 *uint           `json:"n,omitempty"`
	Size              string          `json:"size,omitempty"`
	Quality           string          `json:"quality,omitempty"`
	ResponseFormat    string          `json:"response_format,omitempty"`
	Style             json.RawMessage `json:"style,omitempty"`
	User              json.RawMessage `json:"user,omitempty"`
	ExtraFields       json.RawMessage `json:"extra_fields,omitempty"`
	Background        json.RawMessage `json:"background,omitempty"`
	Moderation        json.RawMessage `json:"moderation,omitempty"`
	OutputFormat      json.RawMessage `json:"output_format,omitempty"`
	OutputCompression json.RawMessage `json:"output_compression,omitempty"`
	PartialImages     json.RawMessage `json:"partial_images,omitempty"`
	// Stream            bool            `json:"stream,omitempty"`
	Images        json.RawMessage `json:"images,omitempty"`
	Mask          json.RawMessage `json:"mask,omitempty"`
	InputFidelity json.RawMessage `json:"input_fidelity,omitempty"`
	Watermark     *bool           `json:"watermark,omitempty"`
	// zhipu 4v
	WatermarkEnabled json.RawMessage `json:"watermark_enabled,omitempty"`
	UserId           json.RawMessage `json:"user_id,omitempty"`
	Image            json.RawMessage `json:"image,omitempty"`
	// 用匿名参数接收额外参数
	Extra map[string]json.RawMessage `json:"-"`
}

// UnmarshalJSON 自定义 JSON 反序列化
// 将已知字段解析到结构体，未知字段存入 Extra map
// 实现对各厂商扩展参数的兼容透传
func (i *ImageRequest) UnmarshalJSON(data []byte) error {
	// 先解析成 map[string]interface{}
	var rawMap map[string]json.RawMessage
	if err := common.Unmarshal(data, &rawMap); err != nil {
		return err
	}

	// 用 struct tag 获取所有已定义字段名
	knownFields := GetJSONFieldNames(reflect.TypeOf(*i))

	// 再正常解析已定义字段
	type Alias ImageRequest
	var known Alias
	if err := common.Unmarshal(data, &known); err != nil {
		return err
	}
	*i = ImageRequest(known)

	// 提取多余字段
	i.Extra = make(map[string]json.RawMessage)
	for k, v := range rawMap {
		if _, ok := knownFields[k]; !ok {
			i.Extra[k] = v
		}
	}
	return nil
}

// MarshalJSON 自定义 JSON 序列化
// 将结构体字段序列化为平铺的 JSON 对象
// 注意：不合并 Extra 字段（避免参数污染）
func (r ImageRequest) MarshalJSON() ([]byte, error) {
	// 将已定义字段转为 map
	type Alias ImageRequest
	alias := Alias(r)
	base, err := common.Marshal(alias)
	if err != nil {
		return nil, err
	}

	var baseMap map[string]json.RawMessage
	if err := common.Unmarshal(base, &baseMap); err != nil {
		return nil, err
	}

	// 不能合并ExtraFields！！！！！！！！
	// 合并 ExtraFields
	//for k, v := range r.Extra {
	//	if _, exists := baseMap[k]; !exists {
	//		baseMap[k] = v
	//	}
	//}

	return common.Marshal(baseMap)
}

// GetJSONFieldNames 从结构体类型中提取所有已定义的 JSON 字段名
// 返回字段名到空结构体的映射，用于区分已知字段和未知字段
func GetJSONFieldNames(t reflect.Type) map[string]struct{} {
	fields := make(map[string]struct{})
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// 跳过匿名字段（例如 ExtraFields）
		if field.Anonymous {
			continue
		}

		tag := field.Tag.Get("json")
		if tag == "-" || tag == "" {
			continue
		}

		// 取逗号前字段名（排除 omitempty 等）
		name := tag
		if commaIdx := indexComma(tag); commaIdx != -1 {
			name = tag[:commaIdx]
		}
		fields[name] = struct{}{}
	}
	return fields
}

// indexComma 查找字符串中第一个逗号的位置
// 用于解析 JSON tag（分离字段名和 omitempty 等选项）
func indexComma(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			return i
		}
	}
	return -1
}

// GetTokenCountMeta 获取图像请求的 Token 计数元数据
// 根据模型类型、图像尺寸和质量计算价格系数：
// - DALL-E 模型：256x256 (0.4)、512x512 (0.45)、1024x1024 (1.0)、1024x1792/1792x1024 (2.0)
// - DALL-E 3 HD 质量：额外 2.0 倍（大尺寸为 1.5 倍）
// N 参数不在此处处理，由 image_handler.go 或渠道适配器单独计算
func (i *ImageRequest) GetTokenCountMeta() *types.TokenCountMeta {
	var sizeRatio = 1.0
	var qualityRatio = 1.0

	if strings.HasPrefix(i.Model, "dall-e") {
		// Size
		if i.Size == "256x256" {
			sizeRatio = 0.4
		} else if i.Size == "512x512" {
			sizeRatio = 0.45
		} else if i.Size == "1024x1024" {
			sizeRatio = 1
		} else if i.Size == "1024x1792" || i.Size == "1792x1024" {
			sizeRatio = 2
		}

		if i.Model == "dall-e-3" && i.Quality == "hd" {
			qualityRatio = 2.0
			if i.Size == "1024x1792" || i.Size == "1792x1024" {
				qualityRatio = 1.5
			}
		}
	}

	// n is NOT included here; it is handled via OtherRatio("n") in
	// image_handler.go (default) or channel adaptors (actual count).
	// Including n here caused double-counting for channels that also
	// set OtherRatio("n") (e.g. Ali/Bailian).
	return &types.TokenCountMeta{
		CombineText:     i.Prompt,
		MaxTokens:       1584,
		ImagePriceRatio: sizeRatio * qualityRatio,
	}
}

// IsStream 图像生成请求不支持流式输出，始终返回 false
func (i *ImageRequest) IsStream(c *gin.Context) bool {
	return false
}

// SetModelName 设置图像请求的模型名称
// 仅在 modelName 非空时更新，用于上游模型映射或路由替换
func (i *ImageRequest) SetModelName(modelName string) {
	if modelName != "" {
		i.Model = modelName
	}
}

// ImageResponse 图像生成响应
// Data：生成的图像数据列表
// Created：创建时间戳
// Metadata：扩展元数据（可选）
type ImageResponse struct {
	Data     []ImageData     `json:"data"`
	Created  int64           `json:"created"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
}
// ImageData 单个生成的图像数据
// Url：图像 URL（当 response_format 为 url 时）
// B64Json：Base64 编码的图像数据（当 response_format 为 b64_json 时）
// RevisedPrompt：模型优化后的提示词（DALL-E 3 特性）
type ImageData struct {
	Url           string `json:"url"`
	B64Json       string `json:"b64_json"`
	RevisedPrompt string `json:"revised_prompt"`
}
