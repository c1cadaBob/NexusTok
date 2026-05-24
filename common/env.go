// Package common - env.go
// 该文件提供了环境变量读取的工具函数
//
// 封装了 os.Getenv 和类型转换，提供带默认值的环境变量读取
// 支持 int、string、bool 三种类型
package common

import (
	"fmt"
	"os"
	"strconv"
)

// GetEnvOrDefault 获取整数类型的环境变量，如果不存在或解析失败则返回默认值
//
// 参数：
//   - env: 环境变量名
//   - defaultValue: 默认值
//
// 返回值：
//   - int: 环境变量的值或默认值
func GetEnvOrDefault(env string, defaultValue int) int {
	if env == "" || os.Getenv(env) == "" {
		return defaultValue
	}
	num, err := strconv.Atoi(os.Getenv(env))
	if err != nil {
		SysError(fmt.Sprintf("failed to parse %s: %s, using default value: %d", env, err.Error(), defaultValue))
		return defaultValue
	}
	return num
}

// GetEnvOrDefaultString 获取字符串类型的环境变量，如果不存在则返回默认值
//
// 参数：
//   - env: 环境变量名
//   - defaultValue: 默认值
//
// 返回值：
//   - string: 环境变量的值或默认值
func GetEnvOrDefaultString(env string, defaultValue string) string {
	if env == "" || os.Getenv(env) == "" {
		return defaultValue
	}
	return os.Getenv(env)
}

// GetEnvOrDefaultBool 获取布尔类型的环境变量，如果不存在或解析失败则返回默认值
//
// 接受的真值：1, t, T, TRUE, true, True
// 接受的假值：0, f, F, FALSE, false, False
//
// 参数：
//   - env: 环境变量名
//   - defaultValue: 默认值
//
// 返回值：
//   - bool: 环境变量的值或默认值
func GetEnvOrDefaultBool(env string, defaultValue bool) bool {
	if env == "" || os.Getenv(env) == "" {
		return defaultValue
	}
	b, err := strconv.ParseBool(os.Getenv(env))
	if err != nil {
		SysError(fmt.Sprintf("failed to parse %s: %s, using default value: %t", env, err.Error(), defaultValue))
		return defaultValue
	}
	return b
}
