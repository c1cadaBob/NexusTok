// image.go - 图片解码与处理工具
// 本文件提供图片数据的解码和处理功能。
// 包括：Base64 图片数据解码、Base64 文件数据解析（支持 data: URI 前缀）、
// URL 图片下载与解码、多格式图片配置信息获取（JPEG、PNG、GIF、WebP、HEIF/HEIC）。
// 支持渐进式读取策略，逐步增加读取量直到成功解码。
package service

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"

	"golang.org/x/image/webp"
)

// DecodeBase64ImageData 解码 Base64 编码的图片数据。
// 自动去除 data: URI 前缀，解码后获取图片配置信息（宽高、格式）。
// 返回值:
//   - image.Config: 图片配置信息（包含宽高）
//   - string: 图片格式（如 "jpeg", "png", "gif" 等）
//   - string: 清理后的纯 Base64 字符串
//   - error: 解码过程中的错误
func DecodeBase64ImageData(base64String string) (image.Config, string, string, error) {
	// 去除base64数据的URL前缀（如果有）
	if idx := strings.Index(base64String, ","); idx != -1 {
		base64String = base64String[idx+1:]
	}

	if len(base64String) == 0 {
		return image.Config{}, "", "", errors.New("base64 string is empty")
	}

	// 将base64字符串解码为字节切片
	decodedData, err := base64.StdEncoding.DecodeString(base64String)
	if err != nil {
		fmt.Println("Error: Failed to decode base64 string")
		return image.Config{}, "", "", fmt.Errorf("failed to decode base64 string: %s", err.Error())
	}

	// 创建一个bytes.Buffer用于存储解码后的数据
	reader := bytes.NewReader(decodedData)
	config, format, err := getImageConfig(reader)
	return config, format, base64String, err
}

// DecodeBase64FileData 解码 Base64 编码的文件数据。
// 自动解析 data: URI 前缀中的 MIME 类型信息。
// 如果无法从 URI 前缀中提取 MIME 类型，则回退到图片解码方式获取类型。
// 参数:
//   - base64String: Base64 编码的文件数据（可能包含 data: 前缀）
// 返回值:
//   - string: 文件的 MIME 类型
//   - string: 清理后的纯 Base64 字符串
//   - error: 解码过程中的错误
func DecodeBase64FileData(base64String string) (string, string, error) {
	var mimeType string
	var idx int
	idx = strings.Index(base64String, ",")
	if idx == -1 {
		_, file_type, base64, err := DecodeBase64ImageData(base64String)
		return "image/" + file_type, base64, err
	}
	mimeType = base64String[:idx]
	base64String = base64String[idx+1:]
	idx = strings.Index(mimeType, ";")
	if idx == -1 {
		_, file_type, base64, err := DecodeBase64ImageData(base64String)
		return "image/" + file_type, base64, err
	}
	mimeType = mimeType[:idx]
	idx = strings.Index(mimeType, ":")
	if idx == -1 {
		_, file_type, base64, err := DecodeBase64ImageData(base64String)
		return "image/" + file_type, base64, err
	}
	mimeType = mimeType[idx+1:]
	return mimeType, base64String, nil
}

// GetImageFromUrl 从 URL 获取图片的 MIME 类型和 Base64 编码数据。
// 会下载图片内容并限制大小，自动检测 Content-Type，支持 application/octet-stream 类型的回退解析。
// 参数:
//   - url: 图片的 URL 地址
// 返回值:
//   - mimeType: 图片的 MIME 类型
//   - data: Base64 编码的图片数据
//   - err: 下载或解析过程中的错误
func GetImageFromUrl(url string) (mimeType string, data string, err error) {
	resp, err := DoDownloadRequest(url)
	if err != nil {
		return "", "", fmt.Errorf("failed to download image: %w", err)
	}
	defer resp.Body.Close()

	// Check HTTP status code
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("failed to download image: HTTP %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/octet-stream" && !strings.HasPrefix(contentType, "image/") {
		return "", "", fmt.Errorf("invalid content type: %s, required image/*", contentType)
	}
	maxImageSize := int64(constant.MaxFileDownloadMB * 1024 * 1024)

	// Check Content-Length if available
	if resp.ContentLength > maxImageSize {
		return "", "", fmt.Errorf("image size %d exceeds maximum allowed size of %d bytes", resp.ContentLength, maxImageSize)
	}

	// Use LimitReader to prevent reading oversized images
	limitReader := io.LimitReader(resp.Body, maxImageSize)
	buffer := &bytes.Buffer{}

	written, err := io.Copy(buffer, limitReader)
	if err != nil {
		return "", "", fmt.Errorf("failed to read image data: %w", err)
	}
	if written >= maxImageSize {
		return "", "", fmt.Errorf("image size exceeds maximum allowed size of %d bytes", maxImageSize)
	}

	data = base64.StdEncoding.EncodeToString(buffer.Bytes())
	mimeType = contentType

	// Handle application/octet-stream type
	if mimeType == "application/octet-stream" {
		_, format, _, err := DecodeBase64ImageData(data)
		if err != nil {
			return "", "", err
		}
		mimeType = "image/" + format
	}

	return mimeType, data, nil
}

// DecodeUrlImageData 从 URL 下载图片并解码其配置信息（宽高、格式）。
// 采用渐进式读取策略，按 8KB、24KB、64KB 逐步增加读取量，
// 直到成功解码图片配置信息或读取失败。
// 参数:
//   - imageUrl: 图片的 URL 地址
// 返回值:
//   - image.Config: 图片配置信息（包含宽高）
//   - string: 图片格式
//   - error: 下载或解码过程中的错误
func DecodeUrlImageData(imageUrl string) (image.Config, string, error) {
	response, err := DoDownloadRequest(imageUrl)
	if err != nil {
		common.SysLog(fmt.Sprintf("fail to get image from url: %s", err.Error()))
		return image.Config{}, "", err
	}
	defer response.Body.Close()

	if response.StatusCode != 200 {
		err = errors.New(fmt.Sprintf("fail to get image from url: %s", response.Status))
		return image.Config{}, "", err
	}

	mimeType := response.Header.Get("Content-Type")

	if mimeType != "application/octet-stream" && !strings.HasPrefix(mimeType, "image/") {
		return image.Config{}, "", fmt.Errorf("invalid content type: %s, required image/*", mimeType)
	}

	var readData []byte
	for _, limit := range []int64{1024 * 8, 1024 * 24, 1024 * 64} {
		common.SysLog(fmt.Sprintf("try to decode image config with limit: %d", limit))

		// 从response.Body读取更多的数据直到达到当前的限制
		additionalData := make([]byte, limit-int64(len(readData)))
		n, _ := io.ReadFull(response.Body, additionalData)
		readData = append(readData, additionalData[:n]...)

		// 使用io.MultiReader组合已经读取的数据和response.Body
		limitReader := io.MultiReader(bytes.NewReader(readData), response.Body)

		var config image.Config
		var format string
		config, format, err = getImageConfig(limitReader)
		if err == nil {
			return config, format, nil
		}
	}

	return image.Config{}, "", err // 返回最后一个错误
}

// getImageConfig 从 io.Reader 中解码图片配置信息。
// 支持多种图片格式：标准库格式（JPEG、PNG、GIF）、WebP、HEIF/HEIC。
// 按优先级依次尝试不同解码器，返回第一个成功的解码结果。
// 参数:
//   - reader: 包含图片数据的 io.Reader
// 返回值:
//   - image.Config: 图片配置信息（包含宽高）
//   - string: 图片格式
//   - error: 解码过程中的错误
func getImageConfig(reader io.Reader) (image.Config, string, error) {
	// Read all data so we can retry with different decoders
	data, readErr := io.ReadAll(reader)
	if readErr != nil {
		return image.Config{}, "", fmt.Errorf("failed to read image data: %w", readErr)
	}

	// 读取图片的头部信息来获取图片尺寸
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err == nil {
		return config, format, nil
	}
	common.SysLog(fmt.Sprintf("fail to decode image config(gif, jpg, png): %s", err.Error()))

	config, err = webp.DecodeConfig(bytes.NewReader(data))
	if err == nil {
		return config, "webp", nil
	}
	common.SysLog(fmt.Sprintf("fail to decode image config(webp): %s", err.Error()))

	// Try HEIF/HEIC: parse ISOBMFF ispe box for dimensions
	if heifMime := detectHEIF(data); heifMime != "" {
		formatName := "heif"
		if heifMime == "image/heic" {
			formatName = "heic"
		}
		if w, h, ok := parseHEIFDimensions(data); ok {
			return image.Config{Width: w, Height: h}, formatName, nil
		}
		return image.Config{}, "", fmt.Errorf("failed to decode HEIF/HEIC image dimensions")
	}

	return image.Config{}, "", err
}
