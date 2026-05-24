// amp - response_rewriter.go
// 响应重写器，用于在模型映射场景下修改响应中的模型名称。
// 该模块拦截并修改 HTTP 响应正文，主要功能包括：
//   - 将响应中的模型名称重写为客户端请求的原始模型名
//   - 注入空的 signature 字段到 tool_use/thinking 块（Amp TUI 兼容性）
//   - 规范化工具名称大小写（Amp 模式白名单要求精确匹配）
//   - 支持流式（SSE）和非流式响应的重写
//   - 清理请求体中的无效 thinking 块签名
package amp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ResponseRewriter 包装 gin.ResponseWriter 以拦截和修改响应正文。
// 用于在使用模型映射时重写响应中的模型名称，保持 Amp 兼容的响应格式。
type ResponseRewriter struct {
	gin.ResponseWriter                // 嵌入原始响应写入器
	body             *bytes.Buffer    // 缓冲非流式响应的正文
	originalModel    string           // 客户端请求的原始模型名称（重写目标）
	isStreaming      bool             // 是否为流式响应
	suppressThinking bool             // 是否抑制 thinking 块（用于非流式响应）
}

// NewResponseRewriter 创建一个新的响应重写器，用于模型名称替换。
// 参数 originalModel 为客户端请求的原始模型名称，将用于替换响应中的实际模型名。
func NewResponseRewriter(w gin.ResponseWriter, originalModel string) *ResponseRewriter {
	return &ResponseRewriter{
		ResponseWriter: w,
		body:           &bytes.Buffer{},
		originalModel:  originalModel,
	}
}

// maxBufferedResponseBytes 是非流式响应缓冲的最大字节数（2 MiB）。
// 超过此限制将自动切换到流式模式。
const maxBufferedResponseBytes = 2 * 1024 * 1024 // 2MB safety cap

// looksLikeSSEChunk 检测数据是否看起来像 SSE（Server-Sent Events）格式。
// 通过查找 "data:" 或 "event:" 前缀来判断。
func looksLikeSSEChunk(data []byte) bool {
	for _, line := range bytes.Split(data, []byte("\n")) {
		trimmed := bytes.TrimSpace(line)
		if bytes.HasPrefix(trimmed, []byte("data:")) ||
			bytes.HasPrefix(trimmed, []byte("event:")) {
			return true
		}
	}
	return false
}

// enableStreaming 将重写器切换到流式模式。
// 切换前会将已缓冲的数据作为第一个流式块刷新到客户端。
// 参数 reason 用于日志记录切换原因。
func (rw *ResponseRewriter) enableStreaming(reason string) error {
	if rw.isStreaming {
		return nil
	}
	rw.isStreaming = true

	if rw.body != nil && rw.body.Len() > 0 {
		buf := rw.body.Bytes()
		toFlush := make([]byte, len(buf))
		copy(toFlush, buf)
		rw.body.Reset()

		if _, err := rw.ResponseWriter.Write(rw.rewriteStreamChunk(toFlush)); err != nil {
			return err
		}
		if flusher, ok := rw.ResponseWriter.(http.Flusher); ok {
			flusher.Flush()
		}
	}

	log.Debugf("amp response rewriter: switched to streaming (%s)", reason)
	return nil
}

// Write 拦截响应写入，根据响应类型选择缓冲或流式处理。
// 非流式响应：缓冲到内存，待 Flush 时统一重写。
// 流式响应：每个数据块立即重写并刷新到客户端。
// 自动检测流式模式的触发条件：Content-Type 包含 "stream"、SSE 格式启发式、缓冲区超限。
func (rw *ResponseRewriter) Write(data []byte) (int, error) {
	if !rw.isStreaming && rw.body.Len() == 0 {
		contentType := rw.Header().Get("Content-Type")
		rw.isStreaming = strings.Contains(contentType, "text/event-stream") ||
			strings.Contains(contentType, "stream")
	}

	if !rw.isStreaming {
		if looksLikeSSEChunk(data) {
			if err := rw.enableStreaming("sse heuristic"); err != nil {
				return 0, err
			}
		} else if rw.body.Len()+len(data) > maxBufferedResponseBytes {
			log.Warnf("amp response rewriter: buffer exceeded %d bytes, switching to streaming", maxBufferedResponseBytes)
			if err := rw.enableStreaming("buffer limit"); err != nil {
				return 0, err
			}
		}
	}

	if rw.isStreaming {
		rewritten := rw.rewriteStreamChunk(data)
		n, err := rw.ResponseWriter.Write(rewritten)
		if err == nil {
			if flusher, ok := rw.ResponseWriter.(http.Flusher); ok {
				flusher.Flush()
			}
		}
		return n, err
	}
	return rw.body.Write(data)
}

// Flush 将缓冲的非流式响应重写后刷新到客户端。
// 流式模式下直接调用底层 Flusher。
// 非流式模式下重写整个响应体并更新 Content-Length。
func (rw *ResponseRewriter) Flush() {
	if rw.isStreaming {
		if flusher, ok := rw.ResponseWriter.(http.Flusher); ok {
			flusher.Flush()
		}
		return
	}
	if rw.body.Len() > 0 {
		rewritten := rw.rewriteModelInResponse(rw.body.Bytes())
		// Update Content-Length to match the rewritten body size, since
		// signature injection and model name changes alter the payload length.
		rw.ResponseWriter.Header().Set("Content-Length", fmt.Sprintf("%d", len(rewritten)))
		if _, err := rw.ResponseWriter.Write(rewritten); err != nil {
			log.Warnf("amp response rewriter: failed to write rewritten response: %v", err)
		}
	}
}

// modelFieldPaths 是响应中可能包含模型名称的 JSON 字段路径列表。
// 重写器会将这些路径的值替换为客户端请求的原始模型名。
var modelFieldPaths = []string{"message.model", "model", "modelVersion", "response.model", "response.modelVersion"}

// ampCanonicalToolNames 映射工具名称到 Amp 模式白名单期望的规范大小写。
// 某些上游模型返回小写的工具名（如 "bash"），但 Amp 的大小写敏感白名单要求 "Bash"。
var ampCanonicalToolNames = map[string]string{
	"bash":  "Bash",
	"read":  "Read",
	"grep":  "Grep",
	"glob":  "glob",
	"task":  "Task",
	"check": "Check",
}

// normalizeAmpToolNames 修正 tool_use 块中的工具名称大小写。
// 处理两种格式：
//   - 非流式：content[].name
//   - 流式：content_block.name（content_block_start 事件）
func normalizeAmpToolNames(data []byte) []byte {
	// Non-streaming: content[].name in tool_use blocks
	for index, block := range gjson.GetBytes(data, "content").Array() {
		if block.Get("type").String() != "tool_use" {
			continue
		}
		name := block.Get("name").String()
		if canonical, ok := ampCanonicalToolNames[strings.ToLower(name)]; ok && name != canonical {
			path := fmt.Sprintf("content.%d.name", index)
			var err error
			data, err = sjson.SetBytes(data, path, canonical)
			if err != nil {
				log.Warnf("Amp ResponseRewriter: failed to normalize tool name %q to %q: %v", name, canonical, err)
			}
		}
	}

	// Streaming: content_block.name in content_block_start events
	if gjson.GetBytes(data, "content_block.type").String() == "tool_use" {
		name := gjson.GetBytes(data, "content_block.name").String()
		if canonical, ok := ampCanonicalToolNames[strings.ToLower(name)]; ok && name != canonical {
			var err error
			data, err = sjson.SetBytes(data, "content_block.name", canonical)
			if err != nil {
				log.Warnf("Amp ResponseRewriter: failed to normalize streaming tool name %q to %q: %v", name, canonical, err)
			}
		}
	}

	return data
}

// ensureAmpSignature 向 tool_use/thinking 块注入空的 signature 字段。
// Amp TUI 在渲染时会访问 P.signature.length，缺少该字段会导致崩溃。
// 处理两种格式：
//   - 非流式：content[].signature
//   - 流式：content_block.signature
func ensureAmpSignature(data []byte) []byte {
	for index, block := range gjson.GetBytes(data, "content").Array() {
		blockType := block.Get("type").String()
		if blockType != "tool_use" && blockType != "thinking" {
			continue
		}
		signaturePath := fmt.Sprintf("content.%d.signature", index)
		if gjson.GetBytes(data, signaturePath).Exists() {
			continue
		}
		var err error
		data, err = sjson.SetBytes(data, signaturePath, "")
		if err != nil {
			log.Warnf("Amp ResponseRewriter: failed to add empty signature to %s block: %v", blockType, err)
			break
		}
	}

	contentBlockType := gjson.GetBytes(data, "content_block.type").String()
	if (contentBlockType == "tool_use" || contentBlockType == "thinking") && !gjson.GetBytes(data, "content_block.signature").Exists() {
		var err error
		data, err = sjson.SetBytes(data, "content_block.signature", "")
		if err != nil {
			log.Warnf("Amp ResponseRewriter: failed to add empty signature to streaming %s block: %v", contentBlockType, err)
		}
	}

	return data
}

// suppressAmpThinking 在非流式响应中抑制 thinking 块。
// 当响应中同时存在 tool_use 块时，移除 thinking 块以简化 Amp TUI 的渲染。
// 仅在 suppressThinking 标志为 true 时生效。
func (rw *ResponseRewriter) suppressAmpThinking(data []byte) []byte {
	if !rw.suppressThinking {
		return data
	}
	if gjson.GetBytes(data, `content.#(type=="tool_use")`).Exists() {
		filtered := gjson.GetBytes(data, `content.#(type!="thinking")#`)
		if filtered.Exists() {
			originalCount := gjson.GetBytes(data, "content.#").Int()
			filteredCount := filtered.Get("#").Int()
			if originalCount > filteredCount {
				var err error
				data, err = sjson.SetBytes(data, "content", filtered.Value())
				if err != nil {
					log.Warnf("Amp ResponseRewriter: failed to suppress thinking blocks: %v", err)
				}
			}
		}
	}

	return data
}

// rewriteModelInResponse 对非流式响应执行完整的重写流程：
//  1. 注入空的 signature 字段（Amp TUI 兼容性）
//  2. 规范化工具名称大小写
//  3. 抑制 thinking 块（如果启用）
//  4. 将模型字段路径的值替换为原始模型名
func (rw *ResponseRewriter) rewriteModelInResponse(data []byte) []byte {
	data = ensureAmpSignature(data)
	data = normalizeAmpToolNames(data)
	data = rw.suppressAmpThinking(data)
	if len(data) == 0 {
		return data
	}

	if rw.originalModel == "" {
		return data
	}
	for _, path := range modelFieldPaths {
		if gjson.GetBytes(data, path).Exists() {
			data, _ = sjson.SetBytes(data, path, rw.originalModel)
		}
	}
	return data
}

// rewriteStreamChunk 对流式 SSE 数据块执行重写。
// 处理三种情况：
//  1. "event:" 行：向前查找配对的 "data:" 行，组合处理
//  2. 独立的 "data:" 行：直接提取 JSON 并重写
//  3. 其他行：原样传递
//
// 支持跨 chunk 分割的事件（event 和 data 在不同 chunk 中到达）。
func (rw *ResponseRewriter) rewriteStreamChunk(chunk []byte) []byte {
	lines := bytes.Split(chunk, []byte("\n"))
	var out [][]byte

	i := 0
	for i < len(lines) {
		line := lines[i]
		trimmed := bytes.TrimSpace(line)

		// Case 1: "event:" line - look ahead for its "data:" line
		if bytes.HasPrefix(trimmed, []byte("event: ")) {
			// Scan forward past blank lines to find the data: line
			dataIdx := -1
			for j := i + 1; j < len(lines); j++ {
				t := bytes.TrimSpace(lines[j])
				if len(t) == 0 {
					continue
				}
				if bytes.HasPrefix(t, []byte("data: ")) {
					dataIdx = j
				}
				break
			}

			if dataIdx >= 0 {
				// Found event+data pair - process through rewriter
				jsonData := bytes.TrimPrefix(bytes.TrimSpace(lines[dataIdx]), []byte("data: "))
				if len(jsonData) > 0 && jsonData[0] == '{' {
					rewritten := rw.rewriteStreamEvent(jsonData)
					if rewritten == nil {
						i = dataIdx + 1
						continue
					}
					// Emit event line
					out = append(out, line)
					// Emit blank lines between event and data
					for k := i + 1; k < dataIdx; k++ {
						out = append(out, lines[k])
					}
					// Emit rewritten data
					out = append(out, append([]byte("data: "), rewritten...))
					i = dataIdx + 1
					continue
				}
			}

			// No data line found (orphan event from cross-chunk split)
			// Pass it through as-is - the data will arrive in the next chunk
			out = append(out, line)
			i++
			continue
		}

		// Case 2: standalone "data:" line (no preceding event: in this chunk)
		if bytes.HasPrefix(trimmed, []byte("data: ")) {
			jsonData := bytes.TrimPrefix(trimmed, []byte("data: "))
			if len(jsonData) > 0 && jsonData[0] == '{' {
				rewritten := rw.rewriteStreamEvent(jsonData)
				if rewritten != nil {
					out = append(out, append([]byte("data: "), rewritten...))
				}
				i++
				continue
			}
		}

		// Case 3: everything else
		out = append(out, line)
		i++
	}

	return bytes.Join(out, []byte("\n"))
}

// rewriteStreamEvent 处理 SSE 流中的单个 JSON 事件。
// 执行的重写操作：
//  1. 注入空的 signature 字段
//  2. 规范化工具名称大小写
//  3. 重写模型名称
//
// 注意：流式模式不抑制 thinking 块，以避免破坏 SSE 索引对齐和 TUI 渲染。
func (rw *ResponseRewriter) rewriteStreamEvent(data []byte) []byte {
	// Inject empty signature where needed
	data = ensureAmpSignature(data)

	// Normalize tool names to canonical casing
	data = normalizeAmpToolNames(data)

	// Rewrite model name
	if rw.originalModel != "" {
		for _, path := range modelFieldPaths {
			if gjson.GetBytes(data, path).Exists() {
				data, _ = sjson.SetBytes(data, path, rw.originalModel)
			}
		}
	}

	return data
}

// SanitizeAmpRequestBody 清理发送给上游 API 的请求体。
// 执行的清理操作：
//  1. 移除 thinking 块中空/缺失/无效的 signature（API 要求有效签名）
//  2. 移除 tool_use 块中代理注入的 signature 字段（API 不接受此字段）
//
// 这些清理防止上游 API 返回 400 错误。
func SanitizeAmpRequestBody(body []byte) []byte {
	messages := gjson.GetBytes(body, "messages")
	if !messages.Exists() || !messages.IsArray() {
		return body
	}

	modified := false
	for msgIdx, msg := range messages.Array() {
		if msg.Get("role").String() != "assistant" {
			continue
		}
		content := msg.Get("content")
		if !content.Exists() || !content.IsArray() {
			continue
		}

		var keepBlocks []interface{}
		contentModified := false

		for _, block := range content.Array() {
			blockType := block.Get("type").String()
			if blockType == "thinking" {
				sig := block.Get("signature")
				if !sig.Exists() || sig.Type != gjson.String || strings.TrimSpace(sig.String()) == "" {
					contentModified = true
					continue
				}
			}

			// Use raw JSON to prevent float64 rounding of large integers in tool_use inputs
			blockRaw := []byte(block.Raw)
			if blockType == "tool_use" && block.Get("signature").Exists() {
				blockRaw, _ = sjson.DeleteBytes(blockRaw, "signature")
				contentModified = true
			}

			// sjson.SetBytes supports raw JSON strings if wrapped in gjson.Raw
			keepBlocks = append(keepBlocks, json.RawMessage(blockRaw))
		}

		if contentModified {
			contentPath := fmt.Sprintf("messages.%d.content", msgIdx)
			var err error
			if len(keepBlocks) == 0 {
				body, err = sjson.SetBytes(body, contentPath, []interface{}{})
			} else {
				body, err = sjson.SetBytes(body, contentPath, keepBlocks)
			}
			if err != nil {
				log.Warnf("Amp RequestSanitizer: failed to sanitize message %d: %v", msgIdx, err)
				continue
			}
			modified = true
		}
	}

	if modified {
		log.Debugf("Amp RequestSanitizer: sanitized request body")
	}
	return body
}
