// responses - gemini_openai-responses_response.go
// Gemini 的 OpenAI Responses 响应转换器。
// 负责将 Gemini API 的 SSE 流式响应和非流式响应转换为 OpenAI Responses 格式。
// 实现了完整的 Responses 流式事件状态机，包括：
// 1. 文本聚合与分段输出（output_text.delta/done、content_part.done）
// 2. 推理内容处理（reasoning_summary_text.delta/done）
// 3. 函数调用事件生成（function_call_arguments.delta/done）
// 4. 响应生命周期事件（response.created、response.in_progress、response.completed）
// 5. 用量元数据映射（usage.input_tokens、output_tokens、cached_tokens 等）
package responses

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	translatorcommon "github.com/router-for-me/CLIProxyAPI/v7/internal/translator/common"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/util"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// geminiToResponsesState 保存 Gemini 到 OpenAI Responses 流式转换过程中的状态。
// 用于在多次 ConvertGeminiResponseToOpenAIResponses 调用之间维护转换状态。
type geminiToResponsesState struct {
	// Seq 事件序列号，每发射一个 SSE 事件自增 1
	Seq int
	// ResponseID 响应唯一标识符，从 Gemini 的 responseId 派生或合成
	ResponseID string
	// CreatedAt 响应创建时间的 Unix 时间戳
	CreatedAt int64
	// Started 标记是否已发射 response.created 和 response.in_progress 事件
	Started bool

	// message aggregation 消息聚合相关字段
	// MsgOpened 标记是否已开始输出助手消息
	MsgOpened bool
	// MsgClosed 标记助手消息是否已完成输出
	MsgClosed bool
	// MsgIndex 当前消息在 output 数组中的索引
	MsgIndex int
	// CurrentMsgID 当前消息的唯一标识符
	CurrentMsgID string
	// TextBuf 累积的完整文本内容（用于最终聚合）
	TextBuf strings.Builder
	// ItemTextBuf 当前输出项的文本缓冲区
	ItemTextBuf strings.Builder

	// reasoning aggregation 推理内容聚合相关字段
	// ReasoningOpened 标记是否已开始输出推理内容
	ReasoningOpened bool
	// ReasoningIndex 推理内容在 output 数组中的索引
	ReasoningIndex int
	// ReasoningItemID 推理输出项的唯一标识符
	ReasoningItemID string
	// ReasoningEnc 推理内容的加密签名
	ReasoningEnc string
	// ReasoningBuf 累积的推理文本内容
	ReasoningBuf strings.Builder
	// ReasoningClosed 标记推理内容是否已完成输出
	ReasoningClosed bool

	// function call aggregation 函数调用聚合相关字段（按 output_index 索引）
	// NextIndex 下一个输出项的索引
	NextIndex int
	// FuncArgsBuf 各函数调用的参数缓冲区，键为 output_index
	FuncArgsBuf map[int]*strings.Builder
	// FuncNames 各函数调用的函数名，键为 output_index
	FuncNames map[int]string
	// FuncCallIDs 各函数调用的唯一标识符，键为 output_index
	FuncCallIDs map[int]string
	// FuncDone 各函数调用是否已完成，键为 output_index
	FuncDone map[int]bool
	// SanitizedNameMap 工具名称清理映射表，用于还原被清理过的函数名称
	SanitizedNameMap map[string]string
}

// responseIDCounter 提供进程级别的唯一响应标识符计数器。
// 使用原子操作确保在并发场景下的线程安全性。
var responseIDCounter uint64

// funcCallIDCounter 提供进程级别的唯一函数调用标识符计数器。
var funcCallIDCounter uint64

// pickRequestJSON 选择最佳可用的请求 JSON 数据。
// 优先使用原始请求数据，如果不可用则使用转换后的请求数据。
func pickRequestJSON(originalRequestRawJSON, requestRawJSON []byte) []byte {
	if len(originalRequestRawJSON) > 0 && gjson.ValidBytes(originalRequestRawJSON) {
		return originalRequestRawJSON
	}
	if len(requestRawJSON) > 0 && gjson.ValidBytes(requestRawJSON) {
		return requestRawJSON
	}
	return nil
}

// unwrapRequestRoot 从请求 JSON 中提取实际的请求根对象。
// 如果存在 "request" 子对象且包含关键字段，则返回该子对象；否则返回原始根对象。
// 用于处理 Gemini CLI 等带有额外封装层的请求格式。
func unwrapRequestRoot(root gjson.Result) gjson.Result {
	req := root.Get("request")
	if !req.Exists() {
		return root
	}
	if req.Get("model").Exists() || req.Get("input").Exists() || req.Get("instructions").Exists() {
		return req
	}
	return root
}

// unwrapGeminiResponseRoot 从响应 JSON 中提取实际的 Gemini 响应根对象。
// Vertex 风格的 Gemini 响应会将实际数据封装在 "response" 对象中。
// 如果检测到 candidates、responseId 或 usageMetadata 字段，则返回内部对象。
func unwrapGeminiResponseRoot(root gjson.Result) gjson.Result {
	resp := root.Get("response")
	if !resp.Exists() {
		return root
	}
	// Vertex-style Gemini responses wrap the actual payload in a "response" object.
	if resp.Get("candidates").Exists() || resp.Get("responseId").Exists() || resp.Get("usageMetadata").Exists() {
		return resp
	}
	return root
}

// emitEvent 将事件名称和载荷组合为 SSE 格式的事件数据。
func emitEvent(event string, payload []byte) []byte {
	return translatorcommon.SSEEventData(event, payload)
}

// ConvertGeminiResponseToOpenAIResponses 将 Gemini SSE 流式响应转换为 OpenAI Responses SSE 事件。
//
// 该函数实现了一个完整的状态机，将 Gemini 的流式响应逐步转换为 OpenAI Responses 格式的
// SSE 事件序列。状态通过 param 参数在多次调用之间保持。
//
// 事件生成顺序：
// 1. response.created + response.in_progress（首次调用时）
// 2. 推理内容事件（如果存在 thinking 内容）
// 3. 文本内容事件（output_text.delta/done）
// 4. 函数调用事件（function_call_arguments.delta/done）
// 5. response.completed（当检测到 finishReason 时）
//
// 参数：
//   - ctx: 请求上下文（当前实现中未使用）
//   - modelName: 模型名称
//   - originalRequestRawJSON: 原始请求的 JSON 数据
//   - requestRawJSON: 经过转换的请求 JSON 数据
//   - rawJSON: Gemini 格式的原始响应 JSON 数据
//   - param: 用于在多次调用之间保持状态的参数指针
//
// 返回值：
//   - [][]byte: OpenAI Responses 格式的 SSE 事件数据切片
func ConvertGeminiResponseToOpenAIResponses(_ context.Context, modelName string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) [][]byte {
	if *param == nil {
		*param = &geminiToResponsesState{
			FuncArgsBuf:      make(map[int]*strings.Builder),
			FuncNames:        make(map[int]string),
			FuncCallIDs:      make(map[int]string),
			FuncDone:         make(map[int]bool),
			SanitizedNameMap: util.SanitizedToolNameMap(originalRequestRawJSON),
		}
	}
	st := (*param).(*geminiToResponsesState)
	if st.FuncArgsBuf == nil {
		st.FuncArgsBuf = make(map[int]*strings.Builder)
	}
	if st.FuncNames == nil {
		st.FuncNames = make(map[int]string)
	}
	if st.FuncCallIDs == nil {
		st.FuncCallIDs = make(map[int]string)
	}
	if st.FuncDone == nil {
		st.FuncDone = make(map[int]bool)
	}
	if st.SanitizedNameMap == nil {
		st.SanitizedNameMap = util.SanitizedToolNameMap(originalRequestRawJSON)
	}

	if bytes.HasPrefix(rawJSON, []byte("data:")) {
		rawJSON = bytes.TrimSpace(rawJSON[5:])
	}

	rawJSON = bytes.TrimSpace(rawJSON)
	if len(rawJSON) == 0 || bytes.Equal(rawJSON, []byte("[DONE]")) {
		return [][]byte{}
	}

	root := gjson.ParseBytes(rawJSON)
	if !root.Exists() {
		return [][]byte{}
	}
	root = unwrapGeminiResponseRoot(root)

	var out [][]byte
	nextSeq := func() int { st.Seq++; return st.Seq }

	// Helper to finalize reasoning summary events in correct order.
	// It emits response.reasoning_summary_text.done followed by
	// response.reasoning_summary_part.done exactly once.
	finalizeReasoning := func() {
		if !st.ReasoningOpened || st.ReasoningClosed {
			return
		}
		full := st.ReasoningBuf.String()
		textDone := []byte(`{"type":"response.reasoning_summary_text.done","sequence_number":0,"item_id":"","output_index":0,"summary_index":0,"text":""}`)
		textDone, _ = sjson.SetBytes(textDone, "sequence_number", nextSeq())
		textDone, _ = sjson.SetBytes(textDone, "item_id", st.ReasoningItemID)
		textDone, _ = sjson.SetBytes(textDone, "output_index", st.ReasoningIndex)
		textDone, _ = sjson.SetBytes(textDone, "text", full)
		out = append(out, emitEvent("response.reasoning_summary_text.done", textDone))

		partDone := []byte(`{"type":"response.reasoning_summary_part.done","sequence_number":0,"item_id":"","output_index":0,"summary_index":0,"part":{"type":"summary_text","text":""}}`)
		partDone, _ = sjson.SetBytes(partDone, "sequence_number", nextSeq())
		partDone, _ = sjson.SetBytes(partDone, "item_id", st.ReasoningItemID)
		partDone, _ = sjson.SetBytes(partDone, "output_index", st.ReasoningIndex)
		partDone, _ = sjson.SetBytes(partDone, "part.text", full)
		out = append(out, emitEvent("response.reasoning_summary_part.done", partDone))

		itemDone := []byte(`{"type":"response.output_item.done","sequence_number":0,"output_index":0,"item":{"id":"","type":"reasoning","encrypted_content":"","summary":[{"type":"summary_text","text":""}]}}`)
		itemDone, _ = sjson.SetBytes(itemDone, "sequence_number", nextSeq())
		itemDone, _ = sjson.SetBytes(itemDone, "item.id", st.ReasoningItemID)
		itemDone, _ = sjson.SetBytes(itemDone, "output_index", st.ReasoningIndex)
		itemDone, _ = sjson.SetBytes(itemDone, "item.encrypted_content", st.ReasoningEnc)
		itemDone, _ = sjson.SetBytes(itemDone, "item.summary.0.text", full)
		out = append(out, emitEvent("response.output_item.done", itemDone))

		st.ReasoningClosed = true
	}

	// Helper to finalize the assistant message in correct order.
	// It emits response.output_text.done, response.content_part.done,
	// and response.output_item.done exactly once.
	finalizeMessage := func() {
		if !st.MsgOpened || st.MsgClosed {
			return
		}
		fullText := st.ItemTextBuf.String()
		done := []byte(`{"type":"response.output_text.done","sequence_number":0,"item_id":"","output_index":0,"content_index":0,"text":"","logprobs":[]}`)
		done, _ = sjson.SetBytes(done, "sequence_number", nextSeq())
		done, _ = sjson.SetBytes(done, "item_id", st.CurrentMsgID)
		done, _ = sjson.SetBytes(done, "output_index", st.MsgIndex)
		done, _ = sjson.SetBytes(done, "text", fullText)
		out = append(out, emitEvent("response.output_text.done", done))
		partDone := []byte(`{"type":"response.content_part.done","sequence_number":0,"item_id":"","output_index":0,"content_index":0,"part":{"type":"output_text","annotations":[],"logprobs":[],"text":""}}`)
		partDone, _ = sjson.SetBytes(partDone, "sequence_number", nextSeq())
		partDone, _ = sjson.SetBytes(partDone, "item_id", st.CurrentMsgID)
		partDone, _ = sjson.SetBytes(partDone, "output_index", st.MsgIndex)
		partDone, _ = sjson.SetBytes(partDone, "part.text", fullText)
		out = append(out, emitEvent("response.content_part.done", partDone))
		final := []byte(`{"type":"response.output_item.done","sequence_number":0,"output_index":0,"item":{"id":"","type":"message","status":"completed","content":[{"type":"output_text","text":""}],"role":"assistant"}}`)
		final, _ = sjson.SetBytes(final, "sequence_number", nextSeq())
		final, _ = sjson.SetBytes(final, "output_index", st.MsgIndex)
		final, _ = sjson.SetBytes(final, "item.id", st.CurrentMsgID)
		final, _ = sjson.SetBytes(final, "item.content.0.text", fullText)
		out = append(out, emitEvent("response.output_item.done", final))

		st.MsgClosed = true
	}

	// Initialize per-response fields and emit created/in_progress once
	if !st.Started {
		st.ResponseID = root.Get("responseId").String()
		if st.ResponseID == "" {
			st.ResponseID = fmt.Sprintf("resp_%x_%d", time.Now().UnixNano(), atomic.AddUint64(&responseIDCounter, 1))
		}
		if !strings.HasPrefix(st.ResponseID, "resp_") {
			st.ResponseID = fmt.Sprintf("resp_%s", st.ResponseID)
		}
		if v := root.Get("createTime"); v.Exists() {
			if t, errParseCreateTime := time.Parse(time.RFC3339Nano, v.String()); errParseCreateTime == nil {
				st.CreatedAt = t.Unix()
			}
		}
		if st.CreatedAt == 0 {
			st.CreatedAt = time.Now().Unix()
		}

		created := []byte(`{"type":"response.created","sequence_number":0,"response":{"id":"","object":"response","created_at":0,"status":"in_progress","background":false,"error":null,"output":[]}}`)
		created, _ = sjson.SetBytes(created, "sequence_number", nextSeq())
		created, _ = sjson.SetBytes(created, "response.id", st.ResponseID)
		created, _ = sjson.SetBytes(created, "response.created_at", st.CreatedAt)
		out = append(out, emitEvent("response.created", created))

		inprog := []byte(`{"type":"response.in_progress","sequence_number":0,"response":{"id":"","object":"response","created_at":0,"status":"in_progress"}}`)
		inprog, _ = sjson.SetBytes(inprog, "sequence_number", nextSeq())
		inprog, _ = sjson.SetBytes(inprog, "response.id", st.ResponseID)
		inprog, _ = sjson.SetBytes(inprog, "response.created_at", st.CreatedAt)
		out = append(out, emitEvent("response.in_progress", inprog))

		st.Started = true
		st.NextIndex = 0
	}

	// Handle parts (text/thought/functionCall)
	if parts := root.Get("candidates.0.content.parts"); parts.Exists() && parts.IsArray() {
		parts.ForEach(func(_, part gjson.Result) bool {
			// Reasoning text
			if part.Get("thought").Bool() {
				if st.ReasoningClosed {
					// Ignore any late thought chunks after reasoning is finalized.
					return true
				}
				if sig := part.Get("thoughtSignature"); sig.Exists() && sig.String() != "" && sig.String() != geminiResponsesThoughtSignature {
					st.ReasoningEnc = sig.String()
				} else if sig = part.Get("thought_signature"); sig.Exists() && sig.String() != "" && sig.String() != geminiResponsesThoughtSignature {
					st.ReasoningEnc = sig.String()
				}
				if !st.ReasoningOpened {
					st.ReasoningOpened = true
					st.ReasoningIndex = st.NextIndex
					st.NextIndex++
					st.ReasoningItemID = fmt.Sprintf("rs_%s_%d", st.ResponseID, st.ReasoningIndex)
					item := []byte(`{"type":"response.output_item.added","sequence_number":0,"output_index":0,"item":{"id":"","type":"reasoning","status":"in_progress","encrypted_content":"","summary":[]}}`)
					item, _ = sjson.SetBytes(item, "sequence_number", nextSeq())
					item, _ = sjson.SetBytes(item, "output_index", st.ReasoningIndex)
					item, _ = sjson.SetBytes(item, "item.id", st.ReasoningItemID)
					item, _ = sjson.SetBytes(item, "item.encrypted_content", st.ReasoningEnc)
					out = append(out, emitEvent("response.output_item.added", item))
					partAdded := []byte(`{"type":"response.reasoning_summary_part.added","sequence_number":0,"item_id":"","output_index":0,"summary_index":0,"part":{"type":"summary_text","text":""}}`)
					partAdded, _ = sjson.SetBytes(partAdded, "sequence_number", nextSeq())
					partAdded, _ = sjson.SetBytes(partAdded, "item_id", st.ReasoningItemID)
					partAdded, _ = sjson.SetBytes(partAdded, "output_index", st.ReasoningIndex)
					out = append(out, emitEvent("response.reasoning_summary_part.added", partAdded))
				}
				if t := part.Get("text"); t.Exists() && t.String() != "" {
					st.ReasoningBuf.WriteString(t.String())
					msg := []byte(`{"type":"response.reasoning_summary_text.delta","sequence_number":0,"item_id":"","output_index":0,"summary_index":0,"delta":""}`)
					msg, _ = sjson.SetBytes(msg, "sequence_number", nextSeq())
					msg, _ = sjson.SetBytes(msg, "item_id", st.ReasoningItemID)
					msg, _ = sjson.SetBytes(msg, "output_index", st.ReasoningIndex)
					msg, _ = sjson.SetBytes(msg, "delta", t.String())
					out = append(out, emitEvent("response.reasoning_summary_text.delta", msg))
				}
				return true
			}

			// Assistant visible text
			if t := part.Get("text"); t.Exists() && t.String() != "" {
				// Before emitting non-reasoning outputs, finalize reasoning if open.
				finalizeReasoning()
				if !st.MsgOpened {
					st.MsgOpened = true
					st.MsgIndex = st.NextIndex
					st.NextIndex++
					st.CurrentMsgID = fmt.Sprintf("msg_%s_0", st.ResponseID)
					item := []byte(`{"type":"response.output_item.added","sequence_number":0,"output_index":0,"item":{"id":"","type":"message","status":"in_progress","content":[],"role":"assistant"}}`)
					item, _ = sjson.SetBytes(item, "sequence_number", nextSeq())
					item, _ = sjson.SetBytes(item, "output_index", st.MsgIndex)
					item, _ = sjson.SetBytes(item, "item.id", st.CurrentMsgID)
					out = append(out, emitEvent("response.output_item.added", item))
					partAdded := []byte(`{"type":"response.content_part.added","sequence_number":0,"item_id":"","output_index":0,"content_index":0,"part":{"type":"output_text","annotations":[],"logprobs":[],"text":""}}`)
					partAdded, _ = sjson.SetBytes(partAdded, "sequence_number", nextSeq())
					partAdded, _ = sjson.SetBytes(partAdded, "item_id", st.CurrentMsgID)
					partAdded, _ = sjson.SetBytes(partAdded, "output_index", st.MsgIndex)
					out = append(out, emitEvent("response.content_part.added", partAdded))
					st.ItemTextBuf.Reset()
				}
				st.TextBuf.WriteString(t.String())
				st.ItemTextBuf.WriteString(t.String())
				msg := []byte(`{"type":"response.output_text.delta","sequence_number":0,"item_id":"","output_index":0,"content_index":0,"delta":"","logprobs":[]}`)
				msg, _ = sjson.SetBytes(msg, "sequence_number", nextSeq())
				msg, _ = sjson.SetBytes(msg, "item_id", st.CurrentMsgID)
				msg, _ = sjson.SetBytes(msg, "output_index", st.MsgIndex)
				msg, _ = sjson.SetBytes(msg, "delta", t.String())
				out = append(out, emitEvent("response.output_text.delta", msg))
				return true
			}

			// Function call
			if fc := part.Get("functionCall"); fc.Exists() {
				// Before emitting function-call outputs, finalize reasoning and the message (if open).
				// Responses streaming requires message done events before the next output_item.added.
				finalizeReasoning()
				finalizeMessage()
				name := util.RestoreSanitizedToolName(st.SanitizedNameMap, fc.Get("name").String())
				idx := st.NextIndex
				st.NextIndex++
				// Ensure buffers
				if st.FuncArgsBuf[idx] == nil {
					st.FuncArgsBuf[idx] = &strings.Builder{}
				}
				if st.FuncCallIDs[idx] == "" {
					st.FuncCallIDs[idx] = fmt.Sprintf("call_%d_%d", time.Now().UnixNano(), atomic.AddUint64(&funcCallIDCounter, 1))
				}
				st.FuncNames[idx] = name

				argsJSON := "{}"
				if args := fc.Get("args"); args.Exists() {
					argsJSON = args.Raw
				}
				if st.FuncArgsBuf[idx].Len() == 0 && argsJSON != "" {
					st.FuncArgsBuf[idx].WriteString(argsJSON)
				}

				// Emit item.added for function call
				item := []byte(`{"type":"response.output_item.added","sequence_number":0,"output_index":0,"item":{"id":"","type":"function_call","status":"in_progress","arguments":"","call_id":"","name":""}}`)
				item, _ = sjson.SetBytes(item, "sequence_number", nextSeq())
				item, _ = sjson.SetBytes(item, "output_index", idx)
				item, _ = sjson.SetBytes(item, "item.id", fmt.Sprintf("fc_%s", st.FuncCallIDs[idx]))
				item, _ = sjson.SetBytes(item, "item.call_id", st.FuncCallIDs[idx])
				item, _ = sjson.SetBytes(item, "item.name", name)
				out = append(out, emitEvent("response.output_item.added", item))

				// Emit arguments delta (full args in one chunk).
				// When Gemini omits args, emit "{}" to keep Responses streaming event order consistent.
				if argsJSON != "" {
					ad := []byte(`{"type":"response.function_call_arguments.delta","sequence_number":0,"item_id":"","output_index":0,"delta":""}`)
					ad, _ = sjson.SetBytes(ad, "sequence_number", nextSeq())
					ad, _ = sjson.SetBytes(ad, "item_id", fmt.Sprintf("fc_%s", st.FuncCallIDs[idx]))
					ad, _ = sjson.SetBytes(ad, "output_index", idx)
					ad, _ = sjson.SetBytes(ad, "delta", argsJSON)
					out = append(out, emitEvent("response.function_call_arguments.delta", ad))
				}

				// Gemini emits the full function call payload at once, so we can finalize it immediately.
				if !st.FuncDone[idx] {
					fcDone := []byte(`{"type":"response.function_call_arguments.done","sequence_number":0,"item_id":"","output_index":0,"arguments":""}`)
					fcDone, _ = sjson.SetBytes(fcDone, "sequence_number", nextSeq())
					fcDone, _ = sjson.SetBytes(fcDone, "item_id", fmt.Sprintf("fc_%s", st.FuncCallIDs[idx]))
					fcDone, _ = sjson.SetBytes(fcDone, "output_index", idx)
					fcDone, _ = sjson.SetBytes(fcDone, "arguments", argsJSON)
					out = append(out, emitEvent("response.function_call_arguments.done", fcDone))

					itemDone := []byte(`{"type":"response.output_item.done","sequence_number":0,"output_index":0,"item":{"id":"","type":"function_call","status":"completed","arguments":"","call_id":"","name":""}}`)
					itemDone, _ = sjson.SetBytes(itemDone, "sequence_number", nextSeq())
					itemDone, _ = sjson.SetBytes(itemDone, "output_index", idx)
					itemDone, _ = sjson.SetBytes(itemDone, "item.id", fmt.Sprintf("fc_%s", st.FuncCallIDs[idx]))
					itemDone, _ = sjson.SetBytes(itemDone, "item.arguments", argsJSON)
					itemDone, _ = sjson.SetBytes(itemDone, "item.call_id", st.FuncCallIDs[idx])
					itemDone, _ = sjson.SetBytes(itemDone, "item.name", st.FuncNames[idx])
					out = append(out, emitEvent("response.output_item.done", itemDone))

					st.FuncDone[idx] = true
				}

				return true
			}

			return true
		})
	}

	// Finalization on finishReason
	if fr := root.Get("candidates.0.finishReason"); fr.Exists() && fr.String() != "" {
		// Finalize reasoning first to keep ordering tight with last delta
		finalizeReasoning()
		finalizeMessage()

		// Close function calls
		if len(st.FuncArgsBuf) > 0 {
			// sort indices (small N); avoid extra imports
			idxs := make([]int, 0, len(st.FuncArgsBuf))
			for idx := range st.FuncArgsBuf {
				idxs = append(idxs, idx)
			}
			for i := 0; i < len(idxs); i++ {
				for j := i + 1; j < len(idxs); j++ {
					if idxs[j] < idxs[i] {
						idxs[i], idxs[j] = idxs[j], idxs[i]
					}
				}
			}
			for _, idx := range idxs {
				if st.FuncDone[idx] {
					continue
				}
				args := "{}"
				if b := st.FuncArgsBuf[idx]; b != nil && b.Len() > 0 {
					args = b.String()
				}
				fcDone := []byte(`{"type":"response.function_call_arguments.done","sequence_number":0,"item_id":"","output_index":0,"arguments":""}`)
				fcDone, _ = sjson.SetBytes(fcDone, "sequence_number", nextSeq())
				fcDone, _ = sjson.SetBytes(fcDone, "item_id", fmt.Sprintf("fc_%s", st.FuncCallIDs[idx]))
				fcDone, _ = sjson.SetBytes(fcDone, "output_index", idx)
				fcDone, _ = sjson.SetBytes(fcDone, "arguments", args)
				out = append(out, emitEvent("response.function_call_arguments.done", fcDone))

				itemDone := []byte(`{"type":"response.output_item.done","sequence_number":0,"output_index":0,"item":{"id":"","type":"function_call","status":"completed","arguments":"","call_id":"","name":""}}`)
				itemDone, _ = sjson.SetBytes(itemDone, "sequence_number", nextSeq())
				itemDone, _ = sjson.SetBytes(itemDone, "output_index", idx)
				itemDone, _ = sjson.SetBytes(itemDone, "item.id", fmt.Sprintf("fc_%s", st.FuncCallIDs[idx]))
				itemDone, _ = sjson.SetBytes(itemDone, "item.arguments", args)
				itemDone, _ = sjson.SetBytes(itemDone, "item.call_id", st.FuncCallIDs[idx])
				itemDone, _ = sjson.SetBytes(itemDone, "item.name", st.FuncNames[idx])
				out = append(out, emitEvent("response.output_item.done", itemDone))

				st.FuncDone[idx] = true
			}
		}

		// Reasoning already finalized above if present

		// Build response.completed with aggregated outputs and request echo fields
		completed := []byte(`{"type":"response.completed","sequence_number":0,"response":{"id":"","object":"response","created_at":0,"status":"completed","background":false,"error":null}}`)
		completed, _ = sjson.SetBytes(completed, "sequence_number", nextSeq())
		completed, _ = sjson.SetBytes(completed, "response.id", st.ResponseID)
		completed, _ = sjson.SetBytes(completed, "response.created_at", st.CreatedAt)

		if reqJSON := pickRequestJSON(originalRequestRawJSON, requestRawJSON); len(reqJSON) > 0 {
			req := unwrapRequestRoot(gjson.ParseBytes(reqJSON))
			if v := req.Get("instructions"); v.Exists() {
				completed, _ = sjson.SetBytes(completed, "response.instructions", v.String())
			}
			if v := req.Get("max_output_tokens"); v.Exists() {
				completed, _ = sjson.SetBytes(completed, "response.max_output_tokens", v.Int())
			}
			if v := req.Get("max_tool_calls"); v.Exists() {
				completed, _ = sjson.SetBytes(completed, "response.max_tool_calls", v.Int())
			}
			if v := req.Get("model"); v.Exists() {
				completed, _ = sjson.SetBytes(completed, "response.model", v.String())
			}
			if v := req.Get("parallel_tool_calls"); v.Exists() {
				completed, _ = sjson.SetBytes(completed, "response.parallel_tool_calls", v.Bool())
			}
			if v := req.Get("previous_response_id"); v.Exists() {
				completed, _ = sjson.SetBytes(completed, "response.previous_response_id", v.String())
			}
			if v := req.Get("prompt_cache_key"); v.Exists() {
				completed, _ = sjson.SetBytes(completed, "response.prompt_cache_key", v.String())
			}
			if v := req.Get("reasoning"); v.Exists() {
				completed, _ = sjson.SetBytes(completed, "response.reasoning", v.Value())
			}
			if v := req.Get("safety_identifier"); v.Exists() {
				completed, _ = sjson.SetBytes(completed, "response.safety_identifier", v.String())
			}
			if v := req.Get("service_tier"); v.Exists() {
				completed, _ = sjson.SetBytes(completed, "response.service_tier", v.String())
			}
			if v := req.Get("store"); v.Exists() {
				completed, _ = sjson.SetBytes(completed, "response.store", v.Bool())
			}
			if v := req.Get("temperature"); v.Exists() {
				completed, _ = sjson.SetBytes(completed, "response.temperature", v.Float())
			}
			if v := req.Get("text"); v.Exists() {
				completed, _ = sjson.SetBytes(completed, "response.text", v.Value())
			}
			if v := req.Get("tool_choice"); v.Exists() {
				completed, _ = sjson.SetBytes(completed, "response.tool_choice", v.Value())
			}
			if v := req.Get("tools"); v.Exists() {
				completed, _ = sjson.SetBytes(completed, "response.tools", v.Value())
			}
			if v := req.Get("top_logprobs"); v.Exists() {
				completed, _ = sjson.SetBytes(completed, "response.top_logprobs", v.Int())
			}
			if v := req.Get("top_p"); v.Exists() {
				completed, _ = sjson.SetBytes(completed, "response.top_p", v.Float())
			}
			if v := req.Get("truncation"); v.Exists() {
				completed, _ = sjson.SetBytes(completed, "response.truncation", v.String())
			}
			if v := req.Get("user"); v.Exists() {
				completed, _ = sjson.SetBytes(completed, "response.user", v.Value())
			}
			if v := req.Get("metadata"); v.Exists() {
				completed, _ = sjson.SetBytes(completed, "response.metadata", v.Value())
			}
		}

		// Compose outputs in output_index order.
		outputsWrapper := []byte(`{"arr":[]}`)
		for idx := 0; idx < st.NextIndex; idx++ {
			if st.ReasoningOpened && idx == st.ReasoningIndex {
				item := []byte(`{"id":"","type":"reasoning","encrypted_content":"","summary":[{"type":"summary_text","text":""}]}`)
				item, _ = sjson.SetBytes(item, "id", st.ReasoningItemID)
				item, _ = sjson.SetBytes(item, "encrypted_content", st.ReasoningEnc)
				item, _ = sjson.SetBytes(item, "summary.0.text", st.ReasoningBuf.String())
				outputsWrapper, _ = sjson.SetRawBytes(outputsWrapper, "arr.-1", item)
				continue
			}
			if st.MsgOpened && idx == st.MsgIndex {
				item := []byte(`{"id":"","type":"message","status":"completed","content":[{"type":"output_text","annotations":[],"logprobs":[],"text":""}],"role":"assistant"}`)
				item, _ = sjson.SetBytes(item, "id", st.CurrentMsgID)
				item, _ = sjson.SetBytes(item, "content.0.text", st.TextBuf.String())
				outputsWrapper, _ = sjson.SetRawBytes(outputsWrapper, "arr.-1", item)
				continue
			}

			if callID, ok := st.FuncCallIDs[idx]; ok && callID != "" {
				args := "{}"
				if b := st.FuncArgsBuf[idx]; b != nil && b.Len() > 0 {
					args = b.String()
				}
				item := []byte(`{"id":"","type":"function_call","status":"completed","arguments":"","call_id":"","name":""}`)
				item, _ = sjson.SetBytes(item, "id", fmt.Sprintf("fc_%s", callID))
				item, _ = sjson.SetBytes(item, "arguments", args)
				item, _ = sjson.SetBytes(item, "call_id", callID)
				item, _ = sjson.SetBytes(item, "name", st.FuncNames[idx])
				outputsWrapper, _ = sjson.SetRawBytes(outputsWrapper, "arr.-1", item)
			}
		}
		if gjson.GetBytes(outputsWrapper, "arr.#").Int() > 0 {
			completed, _ = sjson.SetRawBytes(completed, "response.output", []byte(gjson.GetBytes(outputsWrapper, "arr").Raw))
		}

		// usage mapping
		if um := root.Get("usageMetadata"); um.Exists() {
			// input tokens = prompt only (thoughts go to output)
			input := um.Get("promptTokenCount").Int()
			completed, _ = sjson.SetBytes(completed, "response.usage.input_tokens", input)
			// cached token details: align with OpenAI "cached_tokens" semantics.
			completed, _ = sjson.SetBytes(completed, "response.usage.input_tokens_details.cached_tokens", um.Get("cachedContentTokenCount").Int())
			// output tokens
			if v := um.Get("candidatesTokenCount"); v.Exists() {
				completed, _ = sjson.SetBytes(completed, "response.usage.output_tokens", v.Int())
			} else {
				completed, _ = sjson.SetBytes(completed, "response.usage.output_tokens", 0)
			}
			if v := um.Get("thoughtsTokenCount"); v.Exists() {
				completed, _ = sjson.SetBytes(completed, "response.usage.output_tokens_details.reasoning_tokens", v.Int())
			} else {
				completed, _ = sjson.SetBytes(completed, "response.usage.output_tokens_details.reasoning_tokens", 0)
			}
			if v := um.Get("totalTokenCount"); v.Exists() {
				completed, _ = sjson.SetBytes(completed, "response.usage.total_tokens", v.Int())
			} else {
				completed, _ = sjson.SetBytes(completed, "response.usage.total_tokens", 0)
			}
		}

		out = append(out, emitEvent("response.completed", completed))
	}

	return out
}

// ConvertGeminiResponseToOpenAIResponsesNonStream 将 Gemini 的非流式响应聚合为单个 OpenAI Responses JSON 对象。
//
// 该函数处理完整的 Gemini 响应，将所有内容（推理、文本、函数调用）聚合到
// 一个符合 OpenAI Responses 格式的 JSON 对象中。
//
// 参数：
//   - ctx: 请求上下文（当前实现中未使用）
//   - modelName: 模型名称（当前实现中未使用）
//   - originalRequestRawJSON: 原始请求的 JSON 数据
//   - requestRawJSON: 经过转换的请求 JSON 数据
//   - rawJSON: Gemini 格式的原始响应 JSON 数据
//   - _: 未使用的参数指针
//
// 返回值：
//   - []byte: OpenAI Responses 格式的完整 JSON 响应数据
func ConvertGeminiResponseToOpenAIResponsesNonStream(_ context.Context, _ string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, _ *any) []byte {
	root := gjson.ParseBytes(rawJSON)
	root = unwrapGeminiResponseRoot(root)
	sanitizedNameMap := util.SanitizedToolNameMap(originalRequestRawJSON)

	// Base response scaffold
	resp := []byte(`{"id":"","object":"response","created_at":0,"status":"completed","background":false,"error":null,"incomplete_details":null}`)

	// id: prefer provider responseId, otherwise synthesize
	id := root.Get("responseId").String()
	if id == "" {
		id = fmt.Sprintf("resp_%x_%d", time.Now().UnixNano(), atomic.AddUint64(&responseIDCounter, 1))
	}
	// Normalize to response-style id (prefix resp_ if missing)
	if !strings.HasPrefix(id, "resp_") {
		id = fmt.Sprintf("resp_%s", id)
	}
	resp, _ = sjson.SetBytes(resp, "id", id)

	// created_at: map from createTime if available
	createdAt := time.Now().Unix()
	if v := root.Get("createTime"); v.Exists() {
		if t, errParseCreateTime := time.Parse(time.RFC3339Nano, v.String()); errParseCreateTime == nil {
			createdAt = t.Unix()
		}
	}
	resp, _ = sjson.SetBytes(resp, "created_at", createdAt)

	// Echo request fields when present; fallback model from response modelVersion
	if reqJSON := pickRequestJSON(originalRequestRawJSON, requestRawJSON); len(reqJSON) > 0 {
		req := unwrapRequestRoot(gjson.ParseBytes(reqJSON))
		if v := req.Get("instructions"); v.Exists() {
			resp, _ = sjson.SetBytes(resp, "instructions", v.String())
		}
		if v := req.Get("max_output_tokens"); v.Exists() {
			resp, _ = sjson.SetBytes(resp, "max_output_tokens", v.Int())
		}
		if v := req.Get("max_tool_calls"); v.Exists() {
			resp, _ = sjson.SetBytes(resp, "max_tool_calls", v.Int())
		}
		if v := req.Get("model"); v.Exists() {
			resp, _ = sjson.SetBytes(resp, "model", v.String())
		} else if v = root.Get("modelVersion"); v.Exists() {
			resp, _ = sjson.SetBytes(resp, "model", v.String())
		}
		if v := req.Get("parallel_tool_calls"); v.Exists() {
			resp, _ = sjson.SetBytes(resp, "parallel_tool_calls", v.Bool())
		}
		if v := req.Get("previous_response_id"); v.Exists() {
			resp, _ = sjson.SetBytes(resp, "previous_response_id", v.String())
		}
		if v := req.Get("prompt_cache_key"); v.Exists() {
			resp, _ = sjson.SetBytes(resp, "prompt_cache_key", v.String())
		}
		if v := req.Get("reasoning"); v.Exists() {
			resp, _ = sjson.SetBytes(resp, "reasoning", v.Value())
		}
		if v := req.Get("safety_identifier"); v.Exists() {
			resp, _ = sjson.SetBytes(resp, "safety_identifier", v.String())
		}
		if v := req.Get("service_tier"); v.Exists() {
			resp, _ = sjson.SetBytes(resp, "service_tier", v.String())
		}
		if v := req.Get("store"); v.Exists() {
			resp, _ = sjson.SetBytes(resp, "store", v.Bool())
		}
		if v := req.Get("temperature"); v.Exists() {
			resp, _ = sjson.SetBytes(resp, "temperature", v.Float())
		}
		if v := req.Get("text"); v.Exists() {
			resp, _ = sjson.SetBytes(resp, "text", v.Value())
		}
		if v := req.Get("tool_choice"); v.Exists() {
			resp, _ = sjson.SetBytes(resp, "tool_choice", v.Value())
		}
		if v := req.Get("tools"); v.Exists() {
			resp, _ = sjson.SetBytes(resp, "tools", v.Value())
		}
		if v := req.Get("top_logprobs"); v.Exists() {
			resp, _ = sjson.SetBytes(resp, "top_logprobs", v.Int())
		}
		if v := req.Get("top_p"); v.Exists() {
			resp, _ = sjson.SetBytes(resp, "top_p", v.Float())
		}
		if v := req.Get("truncation"); v.Exists() {
			resp, _ = sjson.SetBytes(resp, "truncation", v.String())
		}
		if v := req.Get("user"); v.Exists() {
			resp, _ = sjson.SetBytes(resp, "user", v.Value())
		}
		if v := req.Get("metadata"); v.Exists() {
			resp, _ = sjson.SetBytes(resp, "metadata", v.Value())
		}
	} else if v := root.Get("modelVersion"); v.Exists() {
		resp, _ = sjson.SetBytes(resp, "model", v.String())
	}

	// Build outputs from candidates[0].content.parts
	var reasoningText strings.Builder
	var reasoningEncrypted string
	var messageText strings.Builder
	var haveMessage bool

	haveOutput := false
	ensureOutput := func() {
		if haveOutput {
			return
		}
		resp, _ = sjson.SetRawBytes(resp, "output", []byte("[]"))
		haveOutput = true
	}
	appendOutput := func(itemJSON []byte) {
		ensureOutput()
		resp, _ = sjson.SetRawBytes(resp, "output.-1", itemJSON)
	}

	if parts := root.Get("candidates.0.content.parts"); parts.Exists() && parts.IsArray() {
		parts.ForEach(func(_, p gjson.Result) bool {
			if p.Get("thought").Bool() {
				if t := p.Get("text"); t.Exists() {
					reasoningText.WriteString(t.String())
				}
				if sig := p.Get("thoughtSignature"); sig.Exists() && sig.String() != "" {
					reasoningEncrypted = sig.String()
				}
				return true
			}
			if t := p.Get("text"); t.Exists() && t.String() != "" {
				messageText.WriteString(t.String())
				haveMessage = true
				return true
			}
			if fc := p.Get("functionCall"); fc.Exists() {
				name := util.RestoreSanitizedToolName(sanitizedNameMap, fc.Get("name").String())
				args := fc.Get("args")
				callID := fmt.Sprintf("call_%x_%d", time.Now().UnixNano(), atomic.AddUint64(&funcCallIDCounter, 1))
				itemJSON := []byte(`{"id":"","type":"function_call","status":"completed","arguments":"","call_id":"","name":""}`)
				itemJSON, _ = sjson.SetBytes(itemJSON, "id", fmt.Sprintf("fc_%s", callID))
				itemJSON, _ = sjson.SetBytes(itemJSON, "call_id", callID)
				itemJSON, _ = sjson.SetBytes(itemJSON, "name", name)
				argsStr := ""
				if args.Exists() {
					argsStr = args.Raw
				}
				itemJSON, _ = sjson.SetBytes(itemJSON, "arguments", argsStr)
				appendOutput(itemJSON)
				return true
			}
			return true
		})
	}

	// Reasoning output item
	if reasoningText.Len() > 0 || reasoningEncrypted != "" {
		rid := strings.TrimPrefix(id, "resp_")
		itemJSON := []byte(`{"id":"","type":"reasoning","encrypted_content":""}`)
		itemJSON, _ = sjson.SetBytes(itemJSON, "id", fmt.Sprintf("rs_%s", rid))
		itemJSON, _ = sjson.SetBytes(itemJSON, "encrypted_content", reasoningEncrypted)
		if reasoningText.Len() > 0 {
			summaryJSON := []byte(`{"type":"summary_text","text":""}`)
			summaryJSON, _ = sjson.SetBytes(summaryJSON, "text", reasoningText.String())
			itemJSON, _ = sjson.SetRawBytes(itemJSON, "summary", []byte(`[]`))
			itemJSON, _ = sjson.SetRawBytes(itemJSON, "summary.-1", summaryJSON)
		}
		appendOutput(itemJSON)
	}

	// Assistant message output item
	if haveMessage {
		itemJSON := []byte(`{"id":"","type":"message","status":"completed","content":[{"type":"output_text","annotations":[],"logprobs":[],"text":""}],"role":"assistant"}`)
		itemJSON, _ = sjson.SetBytes(itemJSON, "id", fmt.Sprintf("msg_%s_0", strings.TrimPrefix(id, "resp_")))
		itemJSON, _ = sjson.SetBytes(itemJSON, "content.0.text", messageText.String())
		appendOutput(itemJSON)
	}

	// usage mapping
	if um := root.Get("usageMetadata"); um.Exists() {
		// input tokens = prompt only (thoughts go to output)
		input := um.Get("promptTokenCount").Int()
		resp, _ = sjson.SetBytes(resp, "usage.input_tokens", input)
		// cached token details: align with OpenAI "cached_tokens" semantics.
		resp, _ = sjson.SetBytes(resp, "usage.input_tokens_details.cached_tokens", um.Get("cachedContentTokenCount").Int())
		// output tokens
		if v := um.Get("candidatesTokenCount"); v.Exists() {
			resp, _ = sjson.SetBytes(resp, "usage.output_tokens", v.Int())
		}
		if v := um.Get("thoughtsTokenCount"); v.Exists() {
			resp, _ = sjson.SetBytes(resp, "usage.output_tokens_details.reasoning_tokens", v.Int())
		}
		if v := um.Get("totalTokenCount"); v.Exists() {
			resp, _ = sjson.SetBytes(resp, "usage.total_tokens", v.Int())
		}
	}

	return resp
}
