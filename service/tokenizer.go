// 本文件 (tokenizer.go) 提供 Token 编码器的管理和 Token 数量计算功能。
// 负责初始化默认 Token 编码器、按模型名称缓存编码器实例、
// 以及计算给定文本的 Token 数量。
// 使用 tiktoken-go 库实现，支持 OpenAI 系列模型的 Token 计数。
package service

import (
	"sync" // 互斥锁，保护编码器缓存的并发访问

	"github.com/c1cada/NexusTok/common" // 项目公共工具包
	"github.com/tiktoken-go/tokenizer"   // Token 编码器接口定义
	"github.com/tiktoken-go/tokenizer/codec" // 具体编码算法实现（如 cl100k_base）
)

// defaultTokenEncoder 默认的 Token 编码器，使用 cl100k_base 编码算法。
// 初始化后不再改变，作为未知模型的兜底编码器。
var defaultTokenEncoder tokenizer.Codec

// tokenEncoderMap 按模型名称缓存的 Token 编码器映射表。
// 初始化完成后不再增长，避免重复创建编码器。
var tokenEncoderMap = make(map[string]tokenizer.Codec)

// tokenEncoderMutex 保护 tokenEncoderMap 的并发读写锁。
// 使用读写锁以支持高并发场景下的并行读取。
var tokenEncoderMutex sync.RWMutex

// InitTokenEncoders 初始化默认 Token 编码器。
// 该函数应在应用启动时调用，使用 cl100k_base 编码算法
// （OpenAI GPT-3.5/GPT-4 系列模型使用的编码方式）。
func InitTokenEncoders() {
	common.SysLog("initializing token encoders")
	defaultTokenEncoder = codec.NewCl100kBase()
	common.SysLog("token encoders initialized")
}

// getTokenEncoder 根据模型名称获取对应的 Token 编码器。
// 采用 double-check locking 模式确保线程安全：
// 1. 先用读锁尝试从缓存获取
// 2. 缓存未命中则升级为写锁
// 3. 写锁内再次检查缓存（防止并发创建）
// 4. 仍不存在则创建新编码器并缓存
//
// 如果指定模型没有对应的编码器，则缓存并返回默认编码器，
// 以避免对该模型反复尝试创建编码器。
// 参数:
//   - model: 模型名称（如 "gpt-4", "gpt-3.5-turbo" 等）
// 返回值:
//   - tokenizer.Codec: 该模型对应的 Token 编码器
func getTokenEncoder(model string) tokenizer.Codec {
	// 第一步：使用读锁尝试从缓存获取
	tokenEncoderMutex.RLock()
	if encoder, exists := tokenEncoderMap[model]; exists {
		tokenEncoderMutex.RUnlock()
		return encoder
	}
	tokenEncoderMutex.RUnlock()

	// 第二步：缓存未命中，升级为写锁创建新编码器
	tokenEncoderMutex.Lock()
	defer tokenEncoderMutex.Unlock()

	// 第三步：double-check，防止其他 goroutine 已创建
	if encoder, exists := tokenEncoderMap[model]; exists {
		return encoder
	}

	// 第四步：为指定模型创建新编码器
	modelCodec, err := tokenizer.ForModel(tokenizer.Model(model))
	if err != nil {
		// 创建失败时，缓存默认编码器以避免重复失败
		tokenEncoderMap[model] = defaultTokenEncoder
		return defaultTokenEncoder
	}

	// 缓存新创建的编码器
	tokenEncoderMap[model] = modelCodec
	return modelCodec
}

// getTokenNum 使用指定的 Token 编码器计算文本的 Token 数量。
// 空文本直接返回 0，避免不必要的编码操作。
// 参数:
//   - tokenEncoder: Token 编码器实例
//   - text: 需要计算 Token 数量的文本
// 返回值:
//   - int: 文本包含的 Token 数量
func getTokenNum(tokenEncoder tokenizer.Codec, text string) int {
	if text == "" {
		return 0
	}
	tkm, _ := tokenEncoder.Count(text)
	return tkm
}
