// Package common - pyro.go
// 该文件实现了 Pyroscope 持续性能分析的集成
//
// Pyroscope 是一个开源的持续性能分析平台
// 可以实时分析 CPU、内存、Goroutine、Mutex、Block 等性能指标
//
// 环境变量配置：
// - PYROSCOPE_URL: Pyroscope 服务器地址（不设置则不启用）
// - PYROSCOPE_APP_NAME: 应用名称（默认 "nexustok"）
// - PYROSCOPE_BASIC_AUTH_USER: Basic Auth 用户名
// - PYROSCOPE_BASIC_AUTH_PASSWORD: Basic Auth 密码
// - HOSTNAME: 主机名（用于标签，默认 "nexustok"）
// - PYROSCOPE_MUTEX_RATE: Mutex 采样率（默认 5）
// - PYROSCOPE_BLOCK_RATE: Block 采样率（默认 5）
package common

import (
	"runtime"

	"github.com/grafana/pyroscope-go"
)

// StartPyroScope 启动 Pyroscope 持续性能分析
//
// 初始化流程：
// 1. 从环境变量读取配置
// 2. 如果未配置 PYROSCOPE_URL，直接返回（不启用）
// 3. 设置 Mutex 和 Block 的采样率
// 4. 启动 Pyroscope 客户端
//
// 采集的性能指标：
// - CPU 使用率
// - 内存分配（对象数和空间）
// - 内存使用（对象数和空间）
// - Goroutine 数量
// - Mutex 争用（次数和时长）
// - Block 等待（次数和时长）
//
// 返回值：
//   - error: 启动错误
func StartPyroScope() error {
	pyroscopeUrl := GetEnvOrDefaultString("PYROSCOPE_URL", "")
	if pyroscopeUrl == "" {
		return nil // 未配置 Pyroscope URL，不启用
	}

	pyroscopeAppName := GetEnvOrDefaultString("PYROSCOPE_APP_NAME", "nexustok")
	pyroscopeBasicAuthUser := GetEnvOrDefaultString("PYROSCOPE_BASIC_AUTH_USER", "")
	pyroscopeBasicAuthPassword := GetEnvOrDefaultString("PYROSCOPE_BASIC_AUTH_PASSWORD", "")
	pyroscopeHostname := GetEnvOrDefaultString("HOSTNAME", "nexustok")

	mutexRate := GetEnvOrDefault("PYROSCOPE_MUTEX_RATE", 5)
	blockRate := GetEnvOrDefault("PYROSCOPE_BLOCK_RATE", 5)

	// 设置 Mutex 和 Block 的采样率
	runtime.SetMutexProfileFraction(mutexRate)
	runtime.SetBlockProfileRate(blockRate)

	_, err := pyroscope.Start(pyroscope.Config{
		ApplicationName: pyroscopeAppName,

		ServerAddress:     pyroscopeUrl,
		BasicAuthUser:     pyroscopeBasicAuthUser,
		BasicAuthPassword: pyroscopeBasicAuthPassword,

		Logger: nil,

		Tags: map[string]string{"hostname": pyroscopeHostname},

		// 采集的性能指标类型
		ProfileTypes: []pyroscope.ProfileType{
			pyroscope.ProfileCPU,            // CPU 使用率
			pyroscope.ProfileAllocObjects,   // 内存分配对象数
			pyroscope.ProfileAllocSpace,     // 内存分配空间
			pyroscope.ProfileInuseObjects,   // 内存使用对象数
			pyroscope.ProfileInuseSpace,     // 内存使用空间
			pyroscope.ProfileGoroutines,     // Goroutine 数量
			pyroscope.ProfileMutexCount,     // Mutex 争用次数
			pyroscope.ProfileMutexDuration,  // Mutex 争用时长
			pyroscope.ProfileBlockCount,     // Block 等待次数
			pyroscope.ProfileBlockDuration,  // Block 等待时长
		},
	})
	if err != nil {
		return err
	}
	return nil
}
