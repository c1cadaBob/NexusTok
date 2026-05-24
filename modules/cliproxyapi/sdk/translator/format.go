// translator - format.go
// 该文件定义了 Format 类型，用于标识代理内部使用的请求/响应格式。
// Format 是 string 的别名，提供了从任意字符串转换和序列化的方法。

package translator

// Format 标识代理内部使用的请求/响应格式（如 "openai"、"claude"、"gemini" 等）。
type Format string

// FromString 将任意字符串标识符转换为翻译器格式类型。
func FromString(v string) Format {
	return Format(v)
}

// String 返回格式的原始字符串标识符。
func (f Format) String() string {
	return string(f)
}
