// Package common - audio.go
// 该文件实现了纯 Go 的音频文件时长解析功能
//
// 设计目标：
// - 不依赖外部程序（如 ffmpeg、ffprobe），完全使用 Go 原生库解析
// - 支持主流音频格式：MP3、WAV、FLAC、M4A/MP4、OGG/Vorbis、Opus、AIFF、WebM、AAC
// - 用于 TTS（文本转语音）和 STT（语音转文本）接口的计费和验证
//
// 支持的音频格式及对应的解析库：
// - MP3: github.com/tcolgate/mp3（逐帧解码累加时长）
// - WAV: github.com/go-audio/wav（解析文件头计算时长）
// - FLAC: github.com/mewkiz/flac（解析 STREAMINFO 块）
// - M4A/MP4: github.com/abema/go-mp4（解析 mvhd box）
// - OGG/Vorbis: github.com/jfreymuth/oggvorbis（读取采样计算时长）
// - Opus: 手动解析 OGG 页面头获取 granule position
// - AIFF: github.com/go-audio/aiff（使用解码器获取时长）
// - WebM: 简化 EBML 解析（不完整，建议使用 ffprobe）
// - AAC: github.com/yapingcat/gomedia（解析 ADTS 帧）
package common

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/abema/go-mp4"
	"github.com/go-audio/aiff"
	"github.com/go-audio/wav"
	"github.com/jfreymuth/oggvorbis"
	"github.com/mewkiz/flac"
	"github.com/pkg/errors"
	"github.com/tcolgate/mp3"
	"github.com/yapingcat/gomedia/go-codec"
)

// GetAudioDuration 使用纯 Go 库获取音频文件的时长（秒）
//
// 该函数是音频时长解析的统一入口，根据文件扩展名选择对应的解析器
// 在 TTS/STT 接口的计费流程中被调用，用于确定音频的实际时长
//
// 参数：
//   - ctx: 上下文（当前未使用，预留用于超时控制）
//   - f: 可读可定位的音频文件流（io.ReadSeeker）
//   - ext: 文件扩展名（如 ".mp3"、".wav"），用于选择解析器
//
// 返回值：
//   - duration: 音频时长（秒）
//   - err: 解析错误，nil 表示成功
func GetAudioDuration(ctx context.Context, f io.ReadSeeker, ext string) (duration float64, err error) {
	SysLog(fmt.Sprintf("GetAudioDuration: ext=%s", ext))
	// 根据文件扩展名选择对应的解析器
	switch ext {
	case ".mp3":
		duration, err = getMP3Duration(f)
	case ".wav":
		duration, err = getWAVDuration(f)
	case ".flac":
		duration, err = getFLACDuration(f)
	case ".m4a", ".mp4":
		duration, err = getM4ADuration(f)
	case ".ogg", ".oga", ".opus":
		// OGG 和 Opus 使用相同的容器格式，先尝试 OGG/Vorbis 解析
		// 如果失败则尝试 Opus 解析
		duration, err = getOGGDuration(f)
		if err != nil {
			duration, err = getOpusDuration(f)
		}
	case ".aiff", ".aif", ".aifc":
		duration, err = getAIFFDuration(f)
	case ".webm":
		duration, err = getWebMDuration(f)
	case ".aac":
		duration, err = getAACDuration(f)
	default:
		return 0, fmt.Errorf("unsupported audio format: %s", ext)
	}
	SysLog(fmt.Sprintf("GetAudioDuration: duration=%f", duration))
	return duration, err
}

// getMP3Duration 解析 MP3 文件以获取时长
//
// 解析原理：
// - 逐帧解码 MP3 数据，累加每帧的时长
// - 每帧时长由比特率和采样率决定
//
// 注意事项：
// - 对于 VBR（可变比特率）MP3，这个估算可能不完全精确
// - 但通常足够用于计费目的
// - FFmpeg 会扫描整个文件获得精确值，但这里的库提供快速估算
//
// 参数：
//   - r: 音频数据读取器
//
// 返回值：
//   - float64: 音频时长（秒）
//   - error: 解析错误
func getMP3Duration(r io.Reader) (float64, error) {
	d := mp3.NewDecoder(r)
	var f mp3.Frame
	skipped := 0
	duration := 0.0

	// 逐帧解码，累加每帧时长
	for {
		if err := d.Decode(&f, &skipped); err != nil {
			if err == io.EOF {
				break // 文件读取完毕
			}
			return 0, errors.Wrap(err, "failed to decode mp3 frame")
		}
		duration += f.Duration().Seconds()
	}
	return duration, nil
}

// getWAVDuration 解析 WAV 文件头以获取时长
//
// 解析原理：
// 1. 读取 WAV 文件头，获取采样率、通道数、位深度等元数据
// 2. 定位 PCM 数据块，计算数据大小
// 3. 时长 = PCM 数据大小 / (采样率 × 通道数 × 每采样字节数)
//
// 特殊处理：
// - 如果 PCM 数据大小为 0，尝试用文件大小反推
// - WAV 文件头通常为 44 字节
//
// 参数：
//   - r: 可读可定位的音频数据流
//
// 返回值：
//   - float64: 音频时长（秒）
//   - error: 解析错误
func getWAVDuration(r io.ReadSeeker) (float64, error) {
	// 1. 强制复位指针到文件开头
	r.Seek(0, io.SeekStart)

	dec := wav.NewDecoder(r)

	// IsValidFile 会读取 fmt 块，验证 WAV 文件格式
	if !dec.IsValidFile() {
		return 0, errors.New("invalid wav file")
	}

	// 尝试寻找 data 块，定位 PCM 音频数据
	if err := dec.FwdToPCM(); err != nil {
		return 0, errors.Wrap(err, "failed to find PCM data chunk")
	}

	pcmSize := int64(dec.PCMSize)

	// 如果读出来的 PCM 大小为 0，尝试用文件大小反推
	if pcmSize == 0 {
		// 获取文件总大小
		currentPos, _ := r.Seek(0, io.SeekCurrent) // 当前通常在 data chunk header 之后
		endPos, _ := r.Seek(0, io.SeekEnd)
		fileSize := endPos

		// 恢复位置
		r.Seek(currentPos, io.SeekStart)

		// 数据区大小 ≈ 文件总大小 - 当前指针位置（即 Header 大小）
		// 注意：FwdToPCM 成功后，CurrentPos 应该刚好指向 Data 区数据的开始
		// 或者是 Data Chunk ID + Size 之后
		// WAV Header 一般 44 字节
		if fileSize > 44 {
			// 如果 FwdToPCM 成功，Reader 应该位于 data 块的数据起始处
			// 所以剩余的所有字节理论上都是音频数据
			pcmSize = fileSize - currentPos

			// 简单的兜底：如果算出来还是负数或 0，强制按文件大小-44 计算
			if pcmSize <= 0 {
				pcmSize = fileSize - 44
			}
		}
	}

	// 获取音频参数
	numChans := int64(dec.NumChans)   // 通道数（单声道=1，立体声=2）
	bitDepth := int64(dec.BitDepth)   // 位深度（8/16/24/32 bit）
	sampleRate := float64(dec.SampleRate) // 采样率（如 44100 Hz）

	// 验证参数有效性
	if sampleRate == 0 || numChans == 0 || bitDepth == 0 {
		return 0, errors.New("invalid wav header metadata")
	}

	// 计算每帧的字节数 = 通道数 × (位深度 / 8)
	bytesPerFrame := numChans * (bitDepth / 8)
	if bytesPerFrame == 0 {
		return 0, errors.New("invalid byte depth calculation")
	}

	// 计算总帧数 = PCM 数据大小 / 每帧字节数
	totalFrames := pcmSize / bytesPerFrame

	// 计算时长 = 总帧数 / 采样率
	durationSeconds := float64(totalFrames) / sampleRate
	return durationSeconds, nil
}

// getFLACDuration 解析 FLAC 文件的 STREAMINFO 块以获取时长
//
// 解析原理：
// - FLAC 文件的 STREAMINFO 块包含总采样数和采样率
// - 时长 = 总采样数 / 采样率
//
// 参数：
//   - r: 音频数据读取器
//
// 返回值：
//   - float64: 音频时长（秒）
//   - error: 解析错误
func getFLACDuration(r io.Reader) (float64, error) {
	stream, err := flac.Parse(r)
	if err != nil {
		return 0, errors.Wrap(err, "failed to parse flac stream")
	}
	defer stream.Close()

	// 时长 = 总采样数 / 采样率
	duration := float64(stream.Info.NSamples) / float64(stream.Info.SampleRate)
	return duration, nil
}

// getM4ADuration 解析 M4A/MP4 文件的 'mvhd' box 以获取时长
//
// 解析原理：
// - M4A/MP4 容器格式使用 box 结构组织数据
// - 'mvhd'（movie header）box 包含时间刻度和时长信息
// - 时长 = Duration / Timescale
//
// 参数：
//   - r: 可读可定位的音频数据流（go-mp4 库需要 ReadSeeker 接口）
//
// 返回值：
//   - float64: 音频时长（秒）
//   - error: 解析错误
func getM4ADuration(r io.ReadSeeker) (float64, error) {
	info, err := mp4.Probe(r)
	if err != nil {
		return 0, errors.Wrap(err, "failed to probe m4a/mp4 file")
	}
	// 时长 = Duration / Timescale（将时间刻度转换为秒）
	return float64(info.Duration) / float64(info.Timescale), nil
}

// getOGGDuration 解析 OGG/Vorbis 文件以获取时长
//
// 解析原理：
// - 使用 oggvorbis 库创建 reader，读取音频采样数据
// - 通过读取整个文件计算总采样数
// - 时长 = 总采样数 / 采样率
//
// 参数：
//   - r: 可读可定位的音频数据流
//
// 返回值：
//   - float64: 音频时长（秒）
//   - error: 解析错误
func getOGGDuration(r io.ReadSeeker) (float64, error) {
	// 重置 reader 到开头
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return 0, errors.Wrap(err, "failed to seek ogg file")
	}

	reader, err := oggvorbis.NewReader(r)
	if err != nil {
		return 0, errors.Wrap(err, "failed to create ogg vorbis reader")
	}

	// 获取音频参数
	channels := reader.Channels()   // 通道数
	sampleRate := reader.SampleRate() // 采样率

	// 估算方法：读取到文件结尾，累加总采样数
	var totalSamples int64
	buf := make([]float32, 4096*channels) // 缓冲区大小 = 4096 帧 × 通道数
	for {
		n, err := reader.Read(buf)
		if err == io.EOF {
			break // 文件读取完毕
		}
		if err != nil {
			return 0, errors.Wrap(err, "failed to read ogg samples")
		}
		totalSamples += int64(n / channels) // 采样数 = 读取的 float32 数 / 通道数
	}

	// 时长 = 总采样数 / 采样率
	duration := float64(totalSamples) / float64(sampleRate)
	return duration, nil
}

// getOpusDuration 解析 Opus 文件（在 OGG 容器中）以获取时长
//
// 解析原理：
// - Opus 音频通常封装在 OGG 容器格式中
// - 通过解析 OGG 页面头获取 granule position（采样位置）
// - granule position 表示到该页面为止已解码的总采样数
// - Opus 的采样率固定为 48000 Hz
// - 时长 = 最大 granule position / 48000
//
// OGG 页面结构：
// - 4 字节魔数 "OggS"
// - 1 字节版本
// - 1 字节头类型标志
// - 8 字节 granule position（小端序）
// - 4 字节序列号
// - 4 字节页面序列号
// - 4 字节 CRC 校验和
// - 1 字节段数
// - N 字节段表
//
// 参数：
//   - r: 可读可定位的音频数据流
//
// 返回值：
//   - float64: 音频时长（秒）
//   - error: 解析错误
func getOpusDuration(r io.ReadSeeker) (float64, error) {
	// 重置 reader 到开头
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return 0, errors.Wrap(err, "failed to seek opus file")
	}

	// 读取 OGG 页面头部（最小 27 字节）
	var totalGranulePos int64
	buf := make([]byte, 27)

	for {
		n, err := r.Read(buf)
		if err == io.EOF {
			break // 文件读取完毕
		}
		if err != nil {
			return 0, errors.Wrap(err, "failed to read opus/ogg page")
		}
		if n < 27 {
			break // 数据不足一个页面头
		}

		// 检查 OGG 页面标识 "OggS"
		if string(buf[0:4]) != "OggS" {
			// 不是有效的 OGG 页面头，跳过一些字节继续寻找
			if _, err := r.Seek(-26, io.SeekCurrent); err != nil {
				break
			}
			continue
		}

		// 读取 granule position（字节 6-13，小端序）
		// granule position 表示到该页面为止已解码的总采样数
		granulePos := int64(binary.LittleEndian.Uint64(buf[6:14]))
		if granulePos > totalGranulePos {
			totalGranulePos = granulePos // 记录最大的 granule position
		}

		// 读取段表大小（字节 26）
		numSegments := int(buf[26])
		segmentTable := make([]byte, numSegments)
		if _, err := io.ReadFull(r, segmentTable); err != nil {
			break
		}

		// 计算页面数据大小并跳过
		// 每个段的大小存储在段表中，页面数据大小 = 所有段大小之和
		var pageSize int
		for _, segSize := range segmentTable {
			pageSize += int(segSize)
		}
		if _, err := r.Seek(int64(pageSize), io.SeekCurrent); err != nil {
			break
		}
	}

	// Opus 的采样率固定为 48000 Hz
	// 时长 = 最大 granule position / 48000
	duration := float64(totalGranulePos) / 48000.0
	return duration, nil
}

// getAIFFDuration 解析 AIFF 文件头以获取时长
//
// 解析原理：
// - AIFF（Audio Interchange File Format）是 Apple 开发的音频格式
// - 使用 go-audio/aiff 库解码文件头，获取时长信息
//
// 参数：
//   - r: 可读可定位的音频数据流
//
// 返回值：
//   - float64: 音频时长（秒）
//   - error: 解析错误
func getAIFFDuration(r io.ReadSeeker) (float64, error) {
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return 0, errors.Wrap(err, "failed to seek aiff file")
	}

	dec := aiff.NewDecoder(r)
	if !dec.IsValidFile() {
		return 0, errors.New("invalid aiff file")
	}

	d, err := dec.Duration()
	if err != nil {
		return 0, errors.Wrap(err, "failed to get aiff duration")
	}

	return d.Seconds(), nil
}

// getWebMDuration 解析 WebM 文件以获取时长
//
// 解析原理：
// - WebM 使用 Matroska 容器格式，基于 EBML（Extensible Binary Meta Language）
// - 时长信息存储在 Duration 元素中（Element ID: 0x4489）
//
// 注意事项：
// - WebM/Matroska 文件的解析比较复杂，需要完整的 EBML 解析器
// - 当前实现是简化版本，可能不适用于所有 WebM 文件
// - 建议对于 WebM 文件使用 ffprobe 获取精确时长
//
// 参数：
//   - r: 可读可定位的音频数据流
//
// 返回值：
//   - float64: 音频时长（秒）
//   - error: 解析错误（当前实现总是返回错误）
func getWebMDuration(r io.ReadSeeker) (float64, error) {
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return 0, errors.Wrap(err, "failed to seek webm file")
	}

	// WebM/Matroska 文件的解析比较复杂
	// 这里提供一个简化的实现，读取 EBML 头部
	// 对于完整的 WebM 解析，可能需要使用专门的库

	buf := make([]byte, 8192)
	n, err := r.Read(buf)
	if err != nil && err != io.EOF {
		return 0, errors.Wrap(err, "failed to read webm file")
	}

	// 尝试查找 Duration 元素（这是一个简化的方法）
	// 实际的 WebM 解析需要完整的 EBML 解析器
	if n > 0 {
		// 检查 EBML 标识（魔数 0x1A45DFA3）
		if len(buf) >= 4 && binary.BigEndian.Uint32(buf[0:4]) == 0x1A45DFA3 {
			// 这是一个有效的 EBML 文件
			// 但完整解析需要更复杂的逻辑
			return 0, errors.New("webm duration parsing requires full EBML parser (consider using ffprobe for webm files)")
		}
	}

	return 0, errors.New("failed to parse webm file")
}

// getAACDuration 解析 AAC（ADTS 格式）文件以获取时长
//
// 解析原理：
// - AAC 音频通常使用 ADTS（Audio Data Transport Stream）封装
// - 每个 ADTS 帧包含 1024 个采样
// - 通过解析 ADTS 帧头获取采样率信息
// - 时长 = (总帧数 × 1024) / 采样率
//
// 使用 gomedia 库的 SplitAACFrame 函数分割 AAC 帧
// 使用 ConvertADTSToASC 函数解析 ADTS 头部获取音频配置
//
// 参数：
//   - r: 可读可定位的音频数据流
//
// 返回值：
//   - float64: 音频时长（秒）
//   - error: 解析错误
func getAACDuration(r io.ReadSeeker) (float64, error) {
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return 0, errors.Wrap(err, "failed to seek aac file")
	}

	// 读取整个文件内容
	data, err := io.ReadAll(r)
	if err != nil {
		return 0, errors.Wrap(err, "failed to read aac file")
	}

	var totalFrames int64
	var sampleRate int

	// 使用 gomedia 的 SplitAACFrame 函数来分割 AAC 帧
	codec.SplitAACFrame(data, func(aac []byte) {
		// 解析 ADTS 头部以获取采样率信息
		if len(aac) >= 7 {
			// 使用 ConvertADTSToASC 来获取音频配置信息
			// ASC（Audio Specific Configuration）包含采样率索引等信息
			asc, err := codec.ConvertADTSToASC(aac)
			if err == nil && sampleRate == 0 {
				// 将采样率索引转换为实际采样率
				sampleRate = codec.AACSampleIdxToSample(int(asc.Sample_freq_index))
			}
			totalFrames++
		}
	})

	if sampleRate == 0 || totalFrames == 0 {
		return 0, errors.New("no valid aac frames found")
	}

	// 每个 AAC ADTS 帧包含 1024 个采样
	totalSamples := totalFrames * 1024
	// 时长 = 总采样数 / 采样率
	duration := float64(totalSamples) / float64(sampleRate)
	return duration, nil
}
