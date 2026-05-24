// Package common - str.go
// 该文件提供了字符串处理相关的工具函数
//
// 包含的功能：
// - 字符串默认值处理
// - 随机字符串生成
// - JSON 和 Map 转换
// - 字符串包含检查
// - 零拷贝字符串/字节切片转换
// - Base64 编码
// - 敏感信息脱敏（URL、IP、邮箱、API Key）
package common

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unsafe"

	"github.com/samber/lo"
)

var (
	maskURLPattern    = regexp.MustCompile(`(http|https)://[^\s/$.?#].[^\s]*`)  // URL 匹配模式
	maskDomainPattern = regexp.MustCompile(`\b(?:[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}\b`) // 域名匹配模式
	maskIPPattern     = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)      // IPv4 地址匹配模式
	// maskApiKeyPattern 匹配 api_key:xxx 模式，用于脱敏 API Key
	maskApiKeyPattern = regexp.MustCompile(`(['"]?)api_key:([^\s'"]+)(['"]?)`)
)

// GetStringIfEmpty 如果字符串为空则返回默认值
//
// 参数：
//   - str: 要检查的字符串
//   - defaultValue: 默认值
//
// 返回值：
//   - string: 原字符串或默认值
func GetStringIfEmpty(str string, defaultValue string) string {
	if str == "" {
		return defaultValue
	}
	return str
}

// GetRandomString 生成指定长度的随机字母数字字符串
//
// 使用 lo.RandomString 生成，字符集为字母+数字
//
// 参数：
//   - length: 字符串长度
//
// 返回值：
//   - string: 随机字符串
func GetRandomString(length int) string {
	if length <= 0 {
		return ""
	}
	return lo.RandomString(length, lo.AlphanumericCharset)
}

// MapToJsonStr 将 map 转换为 JSON 字符串
//
// 参数：
//   - m: 要转换的 map
//
// 返回值：
//   - string: JSON 字符串（失败返回空字符串）
func MapToJsonStr(m map[string]interface{}) string {
	bytes, err := json.Marshal(m)
	if err != nil {
		return ""
	}
	return string(bytes)
}

// StrToMap 将 JSON 字符串转换为 map
//
// 参数：
//   - str: JSON 字符串
//
// 返回值：
//   - map[string]interface{}: 解析后的 map
//   - error: 解析错误
func StrToMap(str string) (map[string]interface{}, error) {
	m := make(map[string]interface{})
	err := Unmarshal([]byte(str), &m)
	if err != nil {
		return nil, err
	}
	return m, nil
}

// StrToJsonArray 将 JSON 字符串转换为数组
//
// 参数：
//   - str: JSON 字符串
//
// 返回值：
//   - []interface{}: 解析后的数组
//   - error: 解析错误
func StrToJsonArray(str string) ([]interface{}, error) {
	var js []interface{}
	err := json.Unmarshal([]byte(str), &js)
	if err != nil {
		return nil, err
	}
	return js, nil
}

// IsJsonArray 判断字符串是否为有效的 JSON 数组
func IsJsonArray(str string) bool {
	var js []interface{}
	return json.Unmarshal([]byte(str), &js) == nil
}

// IsJsonObject 判断字符串是否为有效的 JSON 对象
func IsJsonObject(str string) bool {
	var js map[string]interface{}
	return json.Unmarshal([]byte(str), &js) == nil
}

// String2Int 将字符串转换为整数（失败返回 0）
func String2Int(str string) int {
	num, err := strconv.Atoi(str)
	if err != nil {
		return 0
	}
	return num
}

// StringsContains 检查字符串切片中是否包含指定字符串
func StringsContains(strs []string, str string) bool {
	for _, s := range strs {
		if s == str {
			return true
		}
	}
	return false
}

// StringToByteSlice 零拷贝将字符串转换为字节切片
//
// 警告：返回的字节切片是只读的，追加操作会 panic
// 这是一个性能优化函数，避免不必要的内存分配
//
// 参数：
//   - s: 字符串
//
// 返回值：
//   - []byte: 字节切片（只读）
func StringToByteSlice(s string) []byte {
	tmp1 := (*[2]uintptr)(unsafe.Pointer(&s))
	tmp2 := [3]uintptr{tmp1[0], tmp1[1], tmp1[1]}
	return *(*[]byte)(unsafe.Pointer(&tmp2))
}

// EncodeBase64 将字符串编码为 Base64
func EncodeBase64(str string) string {
	return base64.StdEncoding.EncodeToString([]byte(str))
}

// GetJsonString 将对象序列化为 JSON 字符串（失败返回空字符串）
func GetJsonString(data any) string {
	if data == nil {
		return ""
	}
	b, _ := json.Marshal(data)
	return string(b)
}

// NormalizeBillingPreference 规范化扣费策略
//
// 有效的值：subscription_first, wallet_first, subscription_only, wallet_only
// 其他值默认为 subscription_first
func NormalizeBillingPreference(pref string) string {
	switch strings.TrimSpace(pref) {
	case "subscription_first", "wallet_first", "subscription_only", "wallet_only":
		return strings.TrimSpace(pref)
	default:
		return "subscription_first"
	}
}

// MaskEmail 脱敏邮箱地址，防止日志中泄露 PII
//
// 示例：user@example.com → ***@example.com
//
// 参数：
//   - email: 邮箱地址
//
// 返回值：
//   - string: 脱敏后的邮箱
func MaskEmail(email string) string {
	if email == "" {
		return "***masked***"
	}

	atIndex := strings.Index(email, "@")
	if atIndex == -1 {
		return "***masked***"
	}

	return "***@" + email[atIndex+1:]
}

// maskHostTail 返回域名中应该保留的尾部部分
//
// 对于可能的国家代码顶级域名（如 co.uk, com.cn），保留 2 部分
// 否则只保留顶级域名
//
// 参数：
//   - parts: 域名分割后的部分
//
// 返回值：
//   - []string: 应该保留的尾部部分
func maskHostTail(parts []string) []string {
	if len(parts) < 2 {
		return parts
	}
	lastPart := parts[len(parts)-1]
	secondLastPart := parts[len(parts)-2]
	if len(lastPart) == 2 && len(secondLastPart) <= 3 {
		// 可能是国家代码顶级域名，如 co.uk, com.cn
		return []string{secondLastPart, lastPart}
	}
	return []string{lastPart}
}

// maskHostForURL 对 URL 中的主机名进行脱敏
//
// 将子域名替换为 ***，保留顶级域名
// 示例：api.openai.com → ***.com, sub.domain.co.uk → ***.co.uk
//
// 参数：
//   - host: 主机名
//
// 返回值：
//   - string: 脱敏后的主机名
func maskHostForURL(host string) string {
	parts := strings.Split(host, ".")
	if len(parts) < 2 {
		return "***"
	}
	tail := maskHostTail(parts)
	return "***." + strings.Join(tail, ".")
}

// maskHostForPlainDomain 对纯域名进行脱敏
//
// 用 *** 替换每个子域名部分，保留顶级域名
// 示例：openai.com → ***.com, api.openai.com → ***.***.com
//
// 参数：
//   - domain: 域名
//
// 返回值：
//   - string: 脱敏后的域名
func maskHostForPlainDomain(domain string) string {
	parts := strings.Split(domain, ".")
	if len(parts) < 2 {
		return domain
	}
	tail := maskHostTail(parts)
	numStars := len(parts) - len(tail)
	if numStars < 1 {
		numStars = 1
	}
	stars := strings.TrimSuffix(strings.Repeat("***.", numStars), ".")
	return stars + "." + strings.Join(tail, ".")
}

// MaskSensitiveInfo 对字符串中的敏感信息进行脱敏
//
// 脱敏范围：
// - URL：http://example.com → http://***.com
// - 域名：openai.com → ***.com
// - IP 地址：192.168.1.1 → ***.***.***.***
// - API Key：api_key:xxx → api_key:***
//
// 参数：
//   - str: 包含敏感信息的字符串
//
// 返回值：
//   - string: 脱敏后的字符串
func MaskSensitiveInfo(str string) string {
	// 脱敏 URL
	str = maskURLPattern.ReplaceAllStringFunc(str, func(urlStr string) string {
		u, err := url.Parse(urlStr)
		if err != nil {
			return urlStr
		}

		host := u.Host
		if host == "" {
			return urlStr
		}

		maskedHost := maskHostForURL(host)

		result := u.Scheme + "://" + maskedHost

		// 脱敏路径
		if u.Path != "" && u.Path != "/" {
			pathParts := strings.Split(strings.Trim(u.Path, "/"), "/")
			maskedPathParts := make([]string, len(pathParts))
			for i := range pathParts {
				if pathParts[i] != "" {
					maskedPathParts[i] = "***"
				}
			}
			if len(maskedPathParts) > 0 {
				result += "/" + strings.Join(maskedPathParts, "/")
			}
		} else if u.Path == "/" {
			result += "/"
		}

		// 脱敏查询参数
		if u.RawQuery != "" {
			values, err := url.ParseQuery(u.RawQuery)
			if err != nil {
				result += "?***"
			} else {
				maskedParams := make([]string, 0, len(values))
				for key := range values {
					maskedParams = append(maskedParams, key+"=***")
				}
				if len(maskedParams) > 0 {
					result += "?" + strings.Join(maskedParams, "&")
				}
			}
		}

		return result
	})

	// 脱敏纯域名（无协议前缀）
	str = maskDomainPattern.ReplaceAllStringFunc(str, func(domain string) string {
		return maskHostForPlainDomain(domain)
	})

	// 脱敏 IP 地址
	str = maskIPPattern.ReplaceAllString(str, "***.***.***.***")

	// 脱敏 API Key
	str = maskApiKeyPattern.ReplaceAllString(str, "${1}api_key:***${3}")

	return str
}
