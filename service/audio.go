// audio.go 提供音频数据的解析和解码功能。
// 包括从 Base64 编码的音频数据中计算时长，以及解码带 data URI 前缀的音频数据。
// 支持 PCM16、G.711 u-law、G.711 a-law 等音频格式。
package service

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// parseAudio 从 Base64 编码的音频数据中解析音频时长。
// 根据音频格式计算采样数，再除以采样率得到时长（秒）。
//
// 支持的格式：
//   - pcm16: 16 位 PCM，24kHz 采样率，每样本 2 字节
//   - g711_ulaw / g711_alaw: G.711 编码，8kHz 采样率，每样本 1 字节
//   - 其他格式: 按 8kHz、每样本 1 字节处理
//
// 参数：
//   - audioBase64: Base64 编码的音频数据
//   - format: 音频格式标识
//
// 返回：
//   - duration: 音频时长（秒）
//   - err: Base64 解码错误
func parseAudio(audioBase64 string, format string) (duration float64, err error) {
	audioData, err := base64.StdEncoding.DecodeString(audioBase64)
	if err != nil {
		return 0, fmt.Errorf("base64 decode error: %v", err)
	}

	var samplesCount int
	var sampleRate int

	switch format {
	case "pcm16":
		samplesCount = len(audioData) / 2 // 16位 = 2字节每样本
		sampleRate = 24000                // 24kHz
	case "g711_ulaw", "g711_alaw":
		samplesCount = len(audioData) // 8位 = 1字节每样本
		sampleRate = 8000             // 8kHz
	default:
		samplesCount = len(audioData) // 8位 = 1字节每样本
		sampleRate = 8000             // 8kHz
	}

	duration = float64(samplesCount) / float64(sampleRate)
	return duration, nil
}

// DecodeBase64AudioData 解码 Base64 编码的音频数据。
// 自动检测并移除 data:audio/xxx;base64, 前缀（如果存在）。
// 验证 Base64 数据的有效性后返回纯 Base64 字符串。
//
// 参数：
//   - audioBase64: 可能带 data URI 前缀的 Base64 音频数据
//
// 返回：
//   - string: 去除前缀后的纯 Base64 字符串
//   - error: Base64 解码验证错误
func DecodeBase64AudioData(audioBase64 string) (string, error) {
	// 检查并移除 data:audio/xxx;base64, 前缀
	idx := strings.Index(audioBase64, ",")
	if idx != -1 {
		audioBase64 = audioBase64[idx+1:]
	}

	// 解码 Base64 数据
	_, err := base64.StdEncoding.DecodeString(audioBase64)
	if err != nil {
		return "", fmt.Errorf("base64 decode error: %v", err)
	}

	return audioBase64, nil
}
