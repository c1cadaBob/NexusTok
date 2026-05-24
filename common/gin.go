// Package common - gin.go
// 该文件提供了 Gin 框架相关的工具函数
//
// 包含的功能：
// - 请求体读取和缓存（支持内存和磁盘存储）
// - 请求体反序列化（支持 JSON、Form、Multipart）
// - 上下文键值操作（类型安全的 Get/Set）
// - API 响应封装（成功/错误/国际化）
// - Multipart 表单解析
package common

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/constant"
	"github.com/pkg/errors"

	"github.com/gin-gonic/gin"
)

const KeyRequestBody = "key_request_body" // 请求体缓存键（旧版，兼容）
const KeyBodyStorage = "key_body_storage" // 请求体存储键（新版，使用 BodyStorage）

var ErrRequestBodyTooLarge = errors.New("request body too large") // 请求体过大错误

// IsRequestBodyTooLargeError 判断错误是否为请求体过大
//
// 支持两种错误类型：
// - ErrRequestBodyTooLarge: 自定义错误
// - http.MaxBytesError: 标准库错误
//
// 参数：
//   - err: 错误对象
//
// 返回值：
//   - bool: 是否为请求体过大错误
func IsRequestBodyTooLargeError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrRequestBodyTooLarge) {
		return true
	}
	var mbe *http.MaxBytesError
	return errors.As(err, &mbe)
}

// GetRequestBody 获取请求体（支持重复读取）
//
// 读取流程：
// 1. 检查是否有 BodyStorage 缓存（新版）
// 2. 检查是否有 []byte 缓存（旧版兼容）
// 3. 从请求体读取并创建存储
//
// 返回的 io.Seeker 支持 Seek 操作，可以重复读取
//
// 参数：
//   - c: Gin 上下文
//
// 返回值：
//   - io.Seeker: 可重复读取的请求体
//   - error: 读取错误
func GetRequestBody(c *gin.Context) (io.Seeker, error) {
	// 首先检查是否有 BodyStorage 缓存
	if storage, exists := c.Get(KeyBodyStorage); exists && storage != nil {
		if bs, ok := storage.(BodyStorage); ok {
			if _, err := bs.Seek(0, io.SeekStart); err != nil {
				return nil, fmt.Errorf("failed to seek body storage: %w", err)
			}
			return bs, nil
		}
	}

	// 检查旧的缓存方式（兼容）
	cached, exists := c.Get(KeyRequestBody)
	if exists && cached != nil {
		if b, ok := cached.([]byte); ok {
			bs, err := CreateBodyStorage(b)
			if err != nil {
				return nil, err
			}
			c.Set(KeyBodyStorage, bs)
			return bs, nil
		}
	}

	// 从请求体读取
	maxMB := constant.MaxRequestBodyMB
	if maxMB <= 0 {
		maxMB = 128 // 默认 128MB
	}
	maxBytes := int64(maxMB) << 20

	contentLength := c.Request.ContentLength

	// 使用新的存储系统（自动选择内存或磁盘存储）
	storage, err := CreateBodyStorageFromReader(c.Request.Body, contentLength, maxBytes)
	_ = c.Request.Body.Close()

	if err != nil {
		if IsRequestBodyTooLargeError(err) {
			return nil, errors.Wrap(ErrRequestBodyTooLarge, fmt.Sprintf("request body exceeds %d MB", maxMB))
		}
		return nil, err
	}

	// 缓存存储对象
	c.Set(KeyBodyStorage, storage)

	return storage, nil
}

// GetBodyStorage 获取请求体存储对象（用于需要多次读取的场景）
//
// 参数：
//   - c: Gin 上下文
//
// 返回值：
//   - BodyStorage: 请求体存储对象
//   - error: 获取错误
func GetBodyStorage(c *gin.Context) (BodyStorage, error) {
	seeker, err := GetRequestBody(c)
	if err != nil {
		return nil, err
	}
	bs, ok := seeker.(BodyStorage)
	if !ok {
		return nil, errors.New("unexpected body storage type")
	}
	return bs, nil
}

// CleanupBodyStorage 清理请求体存储（应在请求结束时调用）
//
// 释放存储资源（内存或磁盘文件）
//
// 参数：
//   - c: Gin 上下文
func CleanupBodyStorage(c *gin.Context) {
	if storage, exists := c.Get(KeyBodyStorage); exists && storage != nil {
		if bs, ok := storage.(BodyStorage); ok {
			bs.Close()
		}
		c.Set(KeyBodyStorage, nil)
	}
}

// UnmarshalBodyReusable 反序列化请求体（支持重复读取）
//
// 支持的 Content-Type：
// - application/json: JSON 格式
// - application/x-www-form-urlencoded: 表单格式
// - multipart/form-data: 多部分表单格式
//
// 反序列化后会重置请求体，以便后续中间件继续读取
//
// 参数：
//   - c: Gin 上下文
//   - v: 目标结构体指针
//
// 返回值：
//   - error: 反序列化错误
func UnmarshalBodyReusable(c *gin.Context, v any) error {
	storage, err := GetBodyStorage(c)
	if err != nil {
		return err
	}
	requestBody, err := storage.Bytes()
	if err != nil {
		return err
	}
	contentType := c.Request.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "application/json") {
		err = Unmarshal(requestBody, v)
	} else if strings.Contains(contentType, gin.MIMEPOSTForm) {
		err = parseFormData(requestBody, v)
	} else if strings.Contains(contentType, gin.MIMEMultipartPOSTForm) {
		err = parseMultipartFormData(c, requestBody, v)
	} else {
		// 非 JSON 请求暂时跳过
		// TODO: 将来非 JSON 请求可能有不同的模型，需要实现
	}
	if err != nil {
		return err
	}
	// 重置请求体，以便后续中间件继续读取
	if _, seekErr := storage.Seek(0, io.SeekStart); seekErr != nil {
		return seekErr
	}
	c.Request.Body = io.NopCloser(storage)
	return nil
}

// SetContextKey 在 Gin 上下文中设置键值对
//
// 使用 constant.ContextKey 类型确保键的一致性
//
// 参数：
//   - c: Gin 上下文
//   - key: 上下文键
//   - value: 值
func SetContextKey(c *gin.Context, key constant.ContextKey, value any) {
	c.Set(string(key), value)
}

// GetContextKey 从 Gin 上下文获取键值对
//
// 参数：
//   - c: Gin 上下文
//   - key: 上下文键
//
// 返回值：
//   - any: 值
//   - bool: 是否存在
func GetContextKey(c *gin.Context, key constant.ContextKey) (any, bool) {
	return c.Get(string(key))
}

// GetContextKeyString 从 Gin 上下文获取字符串值
func GetContextKeyString(c *gin.Context, key constant.ContextKey) string {
	return c.GetString(string(key))
}

// GetContextKeyInt 从 Gin 上下文获取整数值
func GetContextKeyInt(c *gin.Context, key constant.ContextKey) int {
	return c.GetInt(string(key))
}

// GetContextKeyBool 从 Gin 上下文获取布尔值
func GetContextKeyBool(c *gin.Context, key constant.ContextKey) bool {
	return c.GetBool(string(key))
}

// GetContextKeyStringSlice 从 Gin 上下文获取字符串切片
func GetContextKeyStringSlice(c *gin.Context, key constant.ContextKey) []string {
	return c.GetStringSlice(string(key))
}

// GetContextKeyStringMap 从 Gin 上下文获取字符串映射
func GetContextKeyStringMap(c *gin.Context, key constant.ContextKey) map[string]any {
	return c.GetStringMap(string(key))
}

// GetContextKeyTime 从 Gin 上下文获取时间值
func GetContextKeyTime(c *gin.Context, key constant.ContextKey) time.Time {
	return c.GetTime(string(key))
}

// GetContextKeyType 从 Gin 上下文获取指定类型的值（泛型版本）
//
// 参数：
//   - c: Gin 上下文
//   - key: 上下文键
//
// 返回值：
//   - T: 类型化的值
//   - bool: 是否存在且类型匹配
func GetContextKeyType[T any](c *gin.Context, key constant.ContextKey) (T, bool) {
	if value, ok := c.Get(string(key)); ok {
		if v, ok := value.(T); ok {
			return v, true
		}
	}
	var t T
	return t, false
}

// ApiError 返回 API 错误响应
//
// 响应格式：{"success": false, "message": "错误信息"}
//
// 参数：
//   - c: Gin 上下文
//   - err: 错误对象
func ApiError(c *gin.Context, err error) {
	c.JSON(http.StatusOK, gin.H{
		"success": false,
		"message": err.Error(),
	})
}

// ApiErrorMsg 返回 API 错误响应（使用字符串消息）
//
// 参数：
//   - c: Gin 上下文
//   - msg: 错误消息
func ApiErrorMsg(c *gin.Context, msg string) {
	c.JSON(http.StatusOK, gin.H{
		"success": false,
		"message": msg,
	})
}

// ApiSuccess 返回 API 成功响应
//
// 响应格式：{"success": true, "message": "", "data": 数据}
//
// 参数：
//   - c: Gin 上下文
//   - data: 响应数据
func ApiSuccess(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    data,
	})
}

// ApiErrorI18n 返回国际化的 API 错误响应
//
// 根据用户的语言偏好返回翻译后的错误消息
//
// 参数：
//   - c: Gin 上下文
//   - key: i18n 消息键
//   - args: 可选的模板数据
func ApiErrorI18n(c *gin.Context, key string, args ...map[string]any) {
	msg := TranslateMessage(c, key, args...)
	c.JSON(http.StatusOK, gin.H{
		"success": false,
		"message": msg,
	})
}

// ApiSuccessI18n 返回国际化的 API 成功响应
//
// 参数：
//   - c: Gin 上下文
//   - key: i18n 消息键
//   - data: 响应数据
//   - args: 可选的模板数据
func ApiSuccessI18n(c *gin.Context, key string, data any, args ...map[string]any) {
	msg := TranslateMessage(c, key, args...)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": msg,
		"data":    data,
	})
}

// TranslateMessage 国际化消息翻译函数
//
// 该函数在 i18n 初始化时被替换为实际的翻译实现
// 默认实现返回原始 key（用于 i18n 未初始化时的降级）
var TranslateMessage func(c *gin.Context, key string, args ...map[string]any) string

func init() {
	// 默认实现：返回原始 key
	// 在 i18n 初始化时会被替换为 i18n.T
	TranslateMessage = func(c *gin.Context, key string, args ...map[string]any) string {
		c.Header("X-Translate-id", "d5e7afdfc7f03414b941f9c1e7096be9966510e7")
		return key
	}
}

// ParseMultipartFormReusable 解析 Multipart 表单（支持重复读取）
//
// 解析后会重置请求体，以便后续中间件继续读取
// 使用原始的 Content-Type 中的 boundary，避免调用者修改请求头后导致 boundary 不匹配
//
// 参数：
//   - c: Gin 上下文
//
// 返回值：
//   - *multipart.Form: 解析后的表单数据
//   - error: 解析错误
func ParseMultipartFormReusable(c *gin.Context) (*multipart.Form, error) {
	storage, err := GetBodyStorage(c)
	if err != nil {
		return nil, err
	}
	requestBody, err := storage.Bytes()
	if err != nil {
		return nil, err
	}

	// 使用原始的 Content-Type 中的 boundary，避免调用者修改请求头后导致 boundary 不匹配
	var contentType string
	if saved, ok := c.Get("_original_multipart_ct"); ok {
		contentType = saved.(string)
	} else {
		contentType = c.Request.Header.Get("Content-Type")
		c.Set("_original_multipart_ct", contentType)
	}
	boundary, err := parseBoundary(contentType)
	if err != nil {
		return nil, err
	}

	reader := multipart.NewReader(bytes.NewReader(requestBody), boundary)
	form, err := reader.ReadForm(multipartMemoryLimit())
	if err != nil {
		return nil, err
	}

	// 重置请求体
	if _, seekErr := storage.Seek(0, io.SeekStart); seekErr != nil {
		return nil, seekErr
	}
	c.Request.Body = io.NopCloser(storage)
	return form, nil
}

// processFormMap 将表单数据映射转换为目标结构体
//
// 流程：map → JSON → 目标结构体
//
// 参数：
//   - formMap: 表单数据映射
//   - v: 目标结构体指针
//
// 返回值：
//   - error: 转换错误
func processFormMap(formMap map[string]any, v any) error {
	jsonData, err := Marshal(formMap)
	if err != nil {
		return err
	}

	err = Unmarshal(jsonData, v)
	if err != nil {
		return err
	}

	return nil
}

// parseFormData 解析 URL 编码的表单数据
//
// 参数：
//   - data: 表单数据字节
//   - v: 目标结构体指针
//
// 返回值：
//   - error: 解析错误
func parseFormData(data []byte, v any) error {
	values, err := url.ParseQuery(string(data))
	if err != nil {
		return err
	}
	formMap := make(map[string]any)
	for key, vals := range values {
		if len(vals) == 1 {
			formMap[key] = vals[0]
		} else {
			formMap[key] = vals
		}
	}

	return processFormMap(formMap, v)
}

// parseMultipartFormData 解析 Multipart 表单数据
//
// 参数：
//   - c: Gin 上下文
//   - data: 表单数据字节
//   - v: 目标结构体指针
//
// 返回值：
//   - error: 解析错误
func parseMultipartFormData(c *gin.Context, data []byte, v any) error {
	var contentType string
	if saved, ok := c.Get("_original_multipart_ct"); ok {
		contentType = saved.(string)
	} else {
		contentType = c.Request.Header.Get("Content-Type")
		c.Set("_original_multipart_ct", contentType)
	}
	boundary, err := parseBoundary(contentType)
	if err != nil {
		if errors.Is(err, errBoundaryNotFound) {
			return Unmarshal(data, v) // 降级为 JSON 解析
		}
		return err
	}

	reader := multipart.NewReader(bytes.NewReader(data), boundary)
	form, err := reader.ReadForm(multipartMemoryLimit())
	if err != nil {
		return err
	}
	defer form.RemoveAll()
	formMap := make(map[string]any)
	for key, vals := range form.Value {
		if len(vals) == 1 {
			formMap[key] = vals[0]
		} else {
			formMap[key] = vals
		}
	}

	return processFormMap(formMap, v)
}

var errBoundaryNotFound = errors.New("multipart boundary not found") // 未找到 boundary 错误

// parseBoundary 从 Content-Type 头中提取 Multipart boundary
//
// 使用 mime.ParseMediaType 解析 Content-Type，提取 boundary 参数
//
// 参数：
//   - contentType: Content-Type 头的值
//
// 返回值：
//   - string: boundary 字符串
//   - error: 解析错误
func parseBoundary(contentType string) (string, error) {
	if contentType == "" {
		return "", errBoundaryNotFound
	}
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return "", err
	}
	boundary, ok := params["boundary"]
	if !ok || boundary == "" {
		return "", errBoundaryNotFound
	}
	return boundary, nil
}

// multipartMemoryLimit 返回配置的 Multipart 内存限制（字节）
//
// 超过此限制的文件会被存储到磁盘
//
// 返回值：
//   - int64: 内存限制（字节）
func multipartMemoryLimit() int64 {
	limitMB := constant.MaxFileDownloadMB
	if limitMB <= 0 {
		limitMB = 32 // 默认 32MB
	}
	return int64(limitMB) << 20
}
