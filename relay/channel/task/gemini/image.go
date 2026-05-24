// gemini - image.go
// Veo 图片输入处理工具函数。
// 负责从 multipart 表单或字符串中提取和解析图片输入，
// 用于 Veo 的图生视频（Image-to-Video）功能。
package gemini

import (
	"encoding/base64"
	"io"
	"net/http"
	"strings"

	"github.com/c1cada/NexusTok/constant"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/gin-gonic/gin"
)

// maxVeoImageSize 最大允许的图片大小（20 MB）。
const maxVeoImageSize = 20 * 1024 * 1024 // 20 MB

// ExtractMultipartImage 从 multipart 表单上传中提取参考图片。
// 读取名为 "input_reference" 的文件字段，转换为 VeoImageInput。
// 如果没有文件或文件过大，返回 nil。
//
// 参数：
//   - c: Gin 上下文
//   - info: 中继信息（会设置 Action 为 TaskActionGenerate）
//
// 返回：VeoImageInput 指针，如果没有图片则返回 nil。
func ExtractMultipartImage(c *gin.Context, info *relaycommon.RelayInfo) *VeoImageInput {
	mf, err := c.MultipartForm()
	if err != nil {
		return nil
	}
	files, exists := mf.File["input_reference"]
	if !exists || len(files) == 0 {
		return nil
	}
	fh := files[0]
	// 检查文件大小限制
	if fh.Size > maxVeoImageSize {
		return nil
	}
	file, err := fh.Open()
	if err != nil {
		return nil
	}
	defer file.Close()

	fileBytes, err := io.ReadAll(file)
	if err != nil {
		return nil
	}

	// 检测 MIME 类型
	mimeType := fh.Header.Get("Content-Type")
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = http.DetectContentType(fileBytes)
	}

	// 设置任务动作为图片生成
	info.Action = constant.TaskActionGenerate
	return &VeoImageInput{
		BytesBase64Encoded: base64.StdEncoding.EncodeToString(fileBytes),
		MimeType:           mimeType,
	}
}

// ParseImageInput 解析图片字符串为 VeoImageInput。
// 支持两种格式：
//   - Data URI：data:image/png;base64,iVBOR...
//   - 纯 Base64 编码字符串
//
// 参数：
//   - imageStr: 图片字符串（Data URI 或 Base64）
//
// 返回：VeoImageInput 指针，如果输入为空或无效则返回 nil。
// TODO: 支持 HTTP URL 图片下载并转换为 Base64
func ParseImageInput(imageStr string) *VeoImageInput {
	imageStr = strings.TrimSpace(imageStr)
	if imageStr == "" {
		return nil
	}

	// 处理 Data URI 格式
	if strings.HasPrefix(imageStr, "data:") {
		return parseDataURI(imageStr)
	}

	// 处理纯 Base64 格式
	raw, err := base64.StdEncoding.DecodeString(imageStr)
	if err != nil {
		return nil
	}
	return &VeoImageInput{
		BytesBase64Encoded: imageStr,
		MimeType:           http.DetectContentType(raw),
	}
}

// parseDataURI 解析 Data URI 格式的图片。
// 格式：data:{mimeType};base64,{base64Data}
//
// 参数：
//   - uri: Data URI 字符串
//
// 返回：VeoImageInput 指针，如果格式无效则返回 nil。
func parseDataURI(uri string) *VeoImageInput {
	// 格式：data:image/png;base64,iVBOR...
	rest := uri[len("data:"):]
	idx := strings.Index(rest, ",")
	if idx < 0 {
		return nil
	}
	meta := rest[:idx]  // "image/png;base64"
	b64 := rest[idx+1:] // "iVBOR..."
	if b64 == "" {
		return nil
	}

	// 解析 MIME 类型
	mimeType := "application/octet-stream"
	parts := strings.SplitN(meta, ";", 2)
	if len(parts) >= 1 && parts[0] != "" {
		mimeType = parts[0]
	}

	return &VeoImageInput{
		BytesBase64Encoded: b64,
		MimeType:           mimeType,
	}
}
