// util - image.go
// 本文件提供图片生成的辅助功能，包括根据宽高比创建白色占位图片并返回 Base64 编码。
// 主要用于图片生成接口的测试和占位场景。
package util

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/draw"
	"image/png"
)

// CreateWhiteImageBase64 根据指定的宽高比创建一个纯白色图片并返回其 Base64 编码字符串。
// 支持多种常见宽高比，包括 1:1、2:3、3:2、3:4、4:3、4:5、5:4、9:16、16:9、21:9。
// 未匹配的宽高比默认使用 1024x1024。
//
// 参数：
//   - aspectRatio: 宽高比字符串，如 "1:1"、"16:9" 等
//
// 返回值：
//   - string: PNG 图片的 Base64 编码字符串
//   - error: 图片编码失败时返回错误
func CreateWhiteImageBase64(aspectRatio string) (string, error) {
	width := 1024
	height := 1024

	switch aspectRatio {
	case "1:1":
		width = 1024
		height = 1024
	case "2:3":
		width = 832
		height = 1248
	case "3:2":
		width = 1248
		height = 832
	case "3:4":
		width = 864
		height = 1184
	case "4:3":
		width = 1184
		height = 864
	case "4:5":
		width = 896
		height = 1152
	case "5:4":
		width = 1152
		height = 896
	case "9:16":
		width = 768
		height = 1344
	case "16:9":
		width = 1344
		height = 768
	case "21:9":
		width = 1536
		height = 672
	}

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Bounds(), image.White, image.Point{}, draw.Src)

	var buf bytes.Buffer

	if err := png.Encode(&buf, img); err != nil {
		return "", err
	}

	base64String := base64.StdEncoding.EncodeToString(buf.Bytes())
	return base64String, nil
}
