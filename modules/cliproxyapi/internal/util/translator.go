// Package util 提供 CLI Proxy API 服务器的工具函数。
// 包括 JSON 操作、代理配置等应用中常用的辅助函数。
package util

import (
	"bytes"
	"fmt"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Walk 递归遍历 JSON 结构，查找指定字段的所有出现位置。
// 将找到的路径添加到 paths 切片中。使用点表示法构建路径。
//
// 参数：
//   - value: 要遍历的 gjson.Result 对象
//   - path: 当前在 JSON 结构中的路径（根节点为空字符串）
//   - field: 要搜索的字段名
//   - paths: 用于存储找到路径的切片指针
func Walk(value gjson.Result, path, field string, paths *[]string) {
	switch value.Type {
	case gjson.JSON:
		// For JSON objects and arrays, iterate through each child
		value.ForEach(func(key, val gjson.Result) bool {
			var childPath string
			// Escape special characters for gjson/sjson path syntax
			// . -> \.
			// * -> \*
			// ? -> \?
			keyStr := key.String()
			safeKey := escapeGJSONPathKey(keyStr)

			if path == "" {
				childPath = safeKey
			} else {
				childPath = path + "." + safeKey
			}
			if keyStr == field {
				*paths = append(*paths, childPath)
			}
			Walk(val, childPath, field, paths)
			return true
		})
	case gjson.String, gjson.Number, gjson.True, gjson.False, gjson.Null:
		// Terminal types - no further traversal needed
	}
}

// RenameKey 重命名 JSON 字符串中的键，将旧键路径的值移动到新键路径，然后删除旧键路径。
//
// 参数：
//   - jsonStr: 要修改的 JSON 字符串
//   - oldKeyPath: 要重命名的键的点表示法路径
//   - newKeyPath: 值要移动到的点表示法路径
//
// 返回：
//   - string: 键重命名后的修改 JSON 字符串
//   - error: 操作失败时的错误
//
// 该函数分两步执行重命名：1. 在新键路径设置值；2. 删除旧键路径。
func RenameKey(jsonStr, oldKeyPath, newKeyPath string) (string, error) {
	value := gjson.Get(jsonStr, oldKeyPath)

	if !value.Exists() {
		return "", fmt.Errorf("old key '%s' does not exist", oldKeyPath)
	}

	interimJSON, errSet := sjson.SetRawBytes([]byte(jsonStr), newKeyPath, []byte(value.Raw))
	if errSet != nil {
		return "", fmt.Errorf("failed to set new key '%s': %w", newKeyPath, errSet)
	}

	finalJSON, errDelete := sjson.DeleteBytes(interimJSON, oldKeyPath)
	if errDelete != nil {
		return "", fmt.Errorf("failed to delete old key '%s': %w", oldKeyPath, errDelete)
	}

	return string(finalJSON), nil
}

// FixJSON 将使用单引号的非标准 JSON 转换为符合 RFC 8259 的标准 JSON，
// 将单引号字符串转换为双引号字符串并正确转义。
//
// 规则：
//   - 已有的双引号 JSON 字符串保持不变
//   - 单引号字符串转换为双引号字符串
//   - 转换后的字符串中双引号会被转义 (\")
//   - 常见反斜杠转义 (\n, \r, \t, \b, \f, \\) 保留
//   - 单引号字符串中的 \' 变为字面量 '
//   - 单引号字符串中的 Unicode 转义 (\uXXXX) 转发保留
func FixJSON(input string) string {
	var out bytes.Buffer

	inDouble := false
	inSingle := false
	escaped := false // applies within the current string state

	// Helper to write a rune, escaping double quotes when inside a converted
	// single-quoted string (which becomes a double-quoted string in output).
	writeConverted := func(r rune) {
		if r == '"' {
			out.WriteByte('\\')
			out.WriteByte('"')
			return
		}
		out.WriteRune(r)
	}

	runes := []rune(input)
	for i := 0; i < len(runes); i++ {
		r := runes[i]

		if inDouble {
			out.WriteRune(r)
			if escaped {
				// end of escape sequence in a standard JSON string
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if r == '"' {
				inDouble = false
			}
			continue
		}

		if inSingle {
			if escaped {
				// Handle common escape sequences after a backslash within a
				// single-quoted string
				escaped = false
				switch r {
				case 'n', 'r', 't', 'b', 'f', '/', '"':
					// Keep the backslash and the character (except for '"' which
					// rarely appears, but if it does, keep as \" to remain valid)
					out.WriteByte('\\')
					out.WriteRune(r)
				case '\\':
					out.WriteByte('\\')
					out.WriteByte('\\')
				case '\'':
					// \' inside single-quoted becomes a literal '
					out.WriteRune('\'')
				case 'u':
					// Forward \uXXXX if possible
					out.WriteByte('\\')
					out.WriteByte('u')
					// Copy up to next 4 hex digits if present
					for k := 0; k < 4 && i+1 < len(runes); k++ {
						peek := runes[i+1]
						// simple hex check
						if (peek >= '0' && peek <= '9') || (peek >= 'a' && peek <= 'f') || (peek >= 'A' && peek <= 'F') {
							out.WriteRune(peek)
							i++
						} else {
							break
						}
					}
				default:
					// Unknown escape: preserve the backslash and the char
					out.WriteByte('\\')
					out.WriteRune(r)
				}
				continue
			}

			if r == '\\' { // start escape sequence
				escaped = true
				continue
			}
			if r == '\'' { // end of single-quoted string
				out.WriteByte('"')
				inSingle = false
				continue
			}
			// regular char inside converted string; escape double quotes
			writeConverted(r)
			continue
		}

		// Outside any string
		if r == '"' {
			inDouble = true
			out.WriteRune(r)
			continue
		}
		if r == '\'' { // start of non-standard single-quoted string
			inSingle = true
			out.WriteByte('"')
			continue
		}
		out.WriteRune(r)
	}

	// If input ended while still inside a single-quoted string, close it to
	// produce the best-effort valid JSON.
	if inSingle {
		out.WriteByte('"')
	}

	return out.String()
}

// CanonicalToolName 将工具名称规范化为小写形式，去除前导下划线和空白。
func CanonicalToolName(name string) string {
	canonical := strings.TrimSpace(name)
	canonical = strings.TrimLeft(canonical, "_")
	return strings.ToLower(canonical)
}

// ToolNameMapFromClaudeRequest 从 Claude 请求中提取规范名称到原始名称的映射表。
// 用于为需要严格工具名称匹配的客户端（如 Claude Code）恢复精确的工具名大小写。
func ToolNameMapFromClaudeRequest(rawJSON []byte) map[string]string {
	if len(rawJSON) == 0 || !gjson.ValidBytes(rawJSON) {
		return nil
	}

	tools := gjson.GetBytes(rawJSON, "tools")
	if !tools.Exists() || !tools.IsArray() {
		return nil
	}

	toolResults := tools.Array()
	out := make(map[string]string, len(toolResults))
	tools.ForEach(func(_, tool gjson.Result) bool {
		name := strings.TrimSpace(tool.Get("name").String())
		if name == "" {
			name = strings.TrimSpace(tool.Get("function.name").String())
		}
		if name == "" {
			return true
		}
		key := CanonicalToolName(name)
		if key == "" {
			return true
		}
		if _, exists := out[key]; !exists {
			out[key] = name
		}
		return true
	})

	if len(out) == 0 {
		return nil
	}
	return out
}

// MapToolName 根据工具名称映射表查找并返回映射后的工具名称。
// 如果映射表中没有对应的映射，则返回原始名称。
func MapToolName(toolNameMap map[string]string, name string) string {
	if name == "" || toolNameMap == nil {
		return name
	}
	if mapped, ok := toolNameMap[CanonicalToolName(name)]; ok && mapped != "" {
		return mapped
	}
	return name
}

// SanitizedToolNameMap 从 Claude 请求工具中构建净化名称到原始名称的映射表。
// 用于在代理通过 SanitizeFunctionName 为 Gemini/Vertex API 兼容性净化工具名后，
// 为客户端（如 Claude Code）恢复精确的工具名。仅包含净化后名称发生变化的条目。
func SanitizedToolNameMap(rawJSON []byte) map[string]string {
	if len(rawJSON) == 0 || !gjson.ValidBytes(rawJSON) {
		return nil
	}

	tools := gjson.GetBytes(rawJSON, "tools")
	if !tools.Exists() || !tools.IsArray() {
		return nil
	}

	out := make(map[string]string)
	tools.ForEach(func(_, tool gjson.Result) bool {
		name := strings.TrimSpace(tool.Get("name").String())
		if name == "" {
			return true
		}
		sanitized := SanitizeFunctionName(name)
		if sanitized == name {
			return true
		}
		if _, exists := out[sanitized]; !exists {
			out[sanitized] = name
		} else {
			log.Warnf("sanitized tool name collision: %q and %q both map to %q, keeping first", out[sanitized], name, sanitized)
		}
		return true
	})

	if len(out) == 0 {
		return nil
	}
	return out
}

// RestoreSanitizedToolName 在映射表中查找已净化的函数名并返回原始的客户端名称。
// 如果没有对应的映射，则返回净化后的名称不变。
func RestoreSanitizedToolName(toolNameMap map[string]string, sanitizedName string) string {
	if sanitizedName == "" || toolNameMap == nil {
		return sanitizedName
	}
	if original, ok := toolNameMap[sanitizedName]; ok {
		return original
	}
	return sanitizedName
}
