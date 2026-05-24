// antigravity/claude - signature_validation.go
// Claude thinking signature validation for Antigravity bypass mode.
// 本文件提供 Claude 思考签名的验证功能，用于 Antigravity 绕过模式。
//
// 签名编码检测（规范 §3）：
// Claude 签名使用单层或双层 Base64 编码。原始字符串的首字符决定编码深度：
//   - 'E' 前缀 → 单层：payload[0]==0x12, 前 6 位 = 000100 = base64 索引 4 = 'E'
//   - 'R' 前缀 → 双层：inner[0]=='E' (0x45), 前 6 位 = 010001 = base64 索引 17 = 'R'
//
// 所有有效签名在发送到 Antigravity 后端前会被规范化为 R 形式（双层 base64）。
//
// Protobuf 结构（规范 §4.1, §4.2）— 仅严格模式：
// Base64 解码后的原始字节（首字节必须为 0x12）包含嵌套的 protobuf 结构，
// 包含通道 ID、基础设施类型、版本、ECDSA 签名、模型文本等字段。
package claude

import (
	"encoding/base64"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/cache"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"google.golang.org/protobuf/encoding/protowire"
)

// maxBypassSignatureLen 是绕过模式签名的最大允许长度（32MB）。
const maxBypassSignatureLen = 32 * 1024 * 1024

// claudeSignatureTree 表示 Claude 签名的解析树结构。
// 包含编码层数、通道 ID、路由类别、基础设施类别、模式特征等维度信息。
type claudeSignatureTree struct {
	EncodingLayers      int    // 编码层数（1=单层, 2=双层）
	ChannelID           uint64 // 通道 ID（11 或 12）
	Field2              *uint64 // Field 2 值（基础设施标识）
	RoutingClass        string // 路由类别（routing_class_11, routing_class_12, unknown）
	InfrastructureClass string // 基础设施类别（infra_default, infra_aws, infra_google, infra_unknown）
	SchemaFeatures      string // 模式特征（compact_schema, extended_model_tagged_schema, unknown_schema_features）
	ModelText           string // 模型文本
	LegacyRouteHint     string // 旧版路由提示（仅 ch=11）
	HasField7           bool   // 是否存在 Field 7
}

// StripEmptySignatureThinkingBlocks 移除签名为空或不是有效 Claude 格式（必须以 'E' 或 'R' 开头，
// 去除缓存前缀后）的思考块。这些来自代理生成的响应（Antigravity/Gemini），其中不存在真正的 Claude 签名。
func StripEmptySignatureThinkingBlocks(payload []byte) []byte {
	messages := gjson.GetBytes(payload, "messages")
	if !messages.IsArray() {
		return payload
	}
	modified := false
	for i, msg := range messages.Array() {
		content := msg.Get("content")
		if !content.IsArray() {
			continue
		}
		var kept []string
		stripped := false
		for _, part := range content.Array() {
			if part.Get("type").String() == "thinking" && !hasValidClaudeSignature(part.Get("signature").String()) {
				stripped = true
				continue
			}
			kept = append(kept, part.Raw)
		}
		if stripped {
			modified = true
			if len(kept) == 0 {
				payload, _ = sjson.SetRawBytes(payload, fmt.Sprintf("messages.%d.content", i), []byte("[]"))
			} else {
				payload, _ = sjson.SetRawBytes(payload, fmt.Sprintf("messages.%d.content", i), []byte("["+strings.Join(kept, ",")+"]"))
			}
		}
	}
	if !modified {
		return payload
	}
	return payload
}

// hasValidClaudeSignature 检查签名是否看起来像真正的 Claude 思考签名：
// 非空且以 'E' 或 'R' 开头（去除可选的缓存前缀如 "modelGroup#" 后）。
func hasValidClaudeSignature(sig string) bool {
	sig = strings.TrimSpace(sig)
	if sig == "" {
		return false
	}
	if idx := strings.IndexByte(sig, '#'); idx >= 0 {
		sig = strings.TrimSpace(sig[idx+1:])
	}
	if sig == "" {
		return false
	}
	return sig[0] == 'E' || sig[0] == 'R'
}

// ValidateClaudeBypassSignatures 验证请求中所有 thinking 块的签名格式。
// 遍历 messages 数组，对每个 thinking 类型的内容块验证其签名是否为有效的 Claude 格式。
func ValidateClaudeBypassSignatures(inputRawJSON []byte) error {
	messages := gjson.GetBytes(inputRawJSON, "messages")
	if !messages.IsArray() {
		return nil
	}

	messageResults := messages.Array()
	for i := 0; i < len(messageResults); i++ {
		contentResults := messageResults[i].Get("content")
		if !contentResults.IsArray() {
			continue
		}
		parts := contentResults.Array()
		for j := 0; j < len(parts); j++ {
			part := parts[j]
			if part.Get("type").String() != "thinking" {
				continue
			}

			rawSignature := strings.TrimSpace(part.Get("signature").String())
			if rawSignature == "" {
				return fmt.Errorf("messages[%d].content[%d]: missing thinking signature", i, j)
			}

			if _, err := normalizeClaudeBypassSignature(rawSignature); err != nil {
				return fmt.Errorf("messages[%d].content[%d]: %w", i, j, err)
			}
		}
	}

	return nil
}

// normalizeClaudeBypassSignature 规范化 Claude 绕过模式签名。
// 去除缓存前缀、检查长度限制，然后根据首字符（'R' 或 'E'）进行相应的验证和规范化。
// 'E' 开头的单层签名会被编码为双层（R 形式）。
func normalizeClaudeBypassSignature(rawSignature string) (string, error) {
	sig := strings.TrimSpace(rawSignature)
	if sig == "" {
		return "", fmt.Errorf("empty signature")
	}

	if idx := strings.IndexByte(sig, '#'); idx >= 0 {
		sig = strings.TrimSpace(sig[idx+1:])
	}

	if sig == "" {
		return "", fmt.Errorf("empty signature after stripping prefix")
	}

	if len(sig) > maxBypassSignatureLen {
		return "", fmt.Errorf("signature exceeds maximum length (%d bytes)", maxBypassSignatureLen)
	}

	switch sig[0] {
	case 'R':
		if err := validateDoubleLayerSignature(sig); err != nil {
			return "", err
		}
		return sig, nil
	case 'E':
		if err := validateSingleLayerSignature(sig); err != nil {
			return "", err
		}
		return base64.StdEncoding.EncodeToString([]byte(sig)), nil
	default:
		return "", fmt.Errorf("invalid signature: expected 'E' or 'R' prefix, got %q", string(sig[0]))
	}
}

// validateDoubleLayerSignature 验证双层 Base64 编码的签名。
// 解码后检查内层是否以 'E' 开头，然后验证单层签名内容。
func validateDoubleLayerSignature(sig string) error {
	decoded, err := base64.StdEncoding.DecodeString(sig)
	if err != nil {
		return fmt.Errorf("invalid double-layer signature: base64 decode failed: %w", err)
	}
	if len(decoded) == 0 {
		return fmt.Errorf("invalid double-layer signature: empty after decode")
	}
	if decoded[0] != 'E' {
		return fmt.Errorf("invalid double-layer signature: inner does not start with 'E', got 0x%02x", decoded[0])
	}
	return validateSingleLayerSignatureContent(string(decoded), 2)
}

// validateSingleLayerSignature 验证单层 Base64 编码的签名。
func validateSingleLayerSignature(sig string) error {
	return validateSingleLayerSignatureContent(sig, 1)
}

// validateSingleLayerSignatureContent 验证单层签名内容。
// Base64 解码后检查首字节是否为 0x12（protobuf Field 2 标识）。
// 严格模式下还会检查完整的 protobuf 结构。
func validateSingleLayerSignatureContent(sig string, encodingLayers int) error {
	decoded, err := base64.StdEncoding.DecodeString(sig)
	if err != nil {
		return fmt.Errorf("invalid single-layer signature: base64 decode failed: %w", err)
	}
	if len(decoded) == 0 {
		return fmt.Errorf("invalid single-layer signature: empty after decode")
	}
	if decoded[0] != 0x12 {
		return fmt.Errorf("invalid Claude signature: expected first byte 0x12, got 0x%02x", decoded[0])
	}
	if !cache.SignatureBypassStrictMode() {
		return nil
	}
	_, err = inspectClaudeSignaturePayload(decoded, encodingLayers)
	return err
}

// inspectDoubleLayerSignature 检查双层签名的完整结构，返回解析树。
func inspectDoubleLayerSignature(sig string) (*claudeSignatureTree, error) {
	decoded, err := base64.StdEncoding.DecodeString(sig)
	if err != nil {
		return nil, fmt.Errorf("invalid double-layer signature: base64 decode failed: %w", err)
	}
	if len(decoded) == 0 {
		return nil, fmt.Errorf("invalid double-layer signature: empty after decode")
	}
	if decoded[0] != 'E' {
		return nil, fmt.Errorf("invalid double-layer signature: inner does not start with 'E', got 0x%02x", decoded[0])
	}
	return inspectSingleLayerSignatureWithLayers(string(decoded), 2)
}

// inspectSingleLayerSignature 检查单层签名的完整结构，返回解析树。
func inspectSingleLayerSignature(sig string) (*claudeSignatureTree, error) {
	return inspectSingleLayerSignatureWithLayers(sig, 1)
}

// inspectSingleLayerSignatureWithLayers 检查单层签名的完整结构，指定编码层数。
func inspectSingleLayerSignatureWithLayers(sig string, encodingLayers int) (*claudeSignatureTree, error) {
	decoded, err := base64.StdEncoding.DecodeString(sig)
	if err != nil {
		return nil, fmt.Errorf("invalid single-layer signature: base64 decode failed: %w", err)
	}
	if len(decoded) == 0 {
		return nil, fmt.Errorf("invalid single-layer signature: empty after decode")
	}
	return inspectClaudeSignaturePayload(decoded, encodingLayers)
}

// inspectClaudeSignaturePayload 检查 Claude 签名的有效载荷。
// 从顶层 protobuf 中提取 Field 2 容器，然后检查其中的通道块。
func inspectClaudeSignaturePayload(payload []byte, encodingLayers int) (*claudeSignatureTree, error) {
	if len(payload) == 0 {
		return nil, fmt.Errorf("invalid Claude signature: empty payload")
	}
	if payload[0] != 0x12 {
		return nil, fmt.Errorf("invalid Claude signature: expected first byte 0x12, got 0x%02x", payload[0])
	}
	container, err := extractBytesField(payload, 2, "top-level protobuf")
	if err != nil {
		return nil, err
	}
	channelBlock, err := extractBytesField(container, 1, "Claude Field 2 container")
	if err != nil {
		return nil, err
	}
	return inspectClaudeChannelBlock(channelBlock, encodingLayers)
}

// inspectClaudeChannelBlock 检查 Claude 通道块的 protobuf 字段。
// 提取通道 ID（Field 1）、基础设施标识（Field 2）、模型文本（Field 6）等，
// 并根据这些值确定路由类别、基础设施类别和模式特征。
func inspectClaudeChannelBlock(channelBlock []byte, encodingLayers int) (*claudeSignatureTree, error) {
	tree := &claudeSignatureTree{
		EncodingLayers:      encodingLayers,
		RoutingClass:        "unknown",
		InfrastructureClass: "infra_unknown",
		SchemaFeatures:      "unknown_schema_features",
	}
	haveChannelID := false
	hasField6 := false
	hasField7 := false

	err := walkProtobufFields(channelBlock, func(num protowire.Number, typ protowire.Type, raw []byte) error {
		switch num {
		case 1:
			if typ != protowire.VarintType {
				return fmt.Errorf("invalid Claude signature: Field 2.1.1 channel_id must be varint")
			}
			channelID, err := decodeVarintField(raw, "Field 2.1.1 channel_id")
			if err != nil {
				return err
			}
			tree.ChannelID = channelID
			haveChannelID = true
		case 2:
			if typ != protowire.VarintType {
				return fmt.Errorf("invalid Claude signature: Field 2.1.2 field2 must be varint")
			}
			field2, err := decodeVarintField(raw, "Field 2.1.2 field2")
			if err != nil {
				return err
			}
			tree.Field2 = &field2
		case 6:
			if typ != protowire.BytesType {
				return fmt.Errorf("invalid Claude signature: Field 2.1.6 model_text must be bytes")
			}
			modelBytes, err := decodeBytesField(raw, "Field 2.1.6 model_text")
			if err != nil {
				return err
			}
			if !utf8.Valid(modelBytes) {
				return fmt.Errorf("invalid Claude signature: Field 2.1.6 model_text is not valid UTF-8")
			}
			tree.ModelText = string(modelBytes)
			hasField6 = true
		case 7:
			if typ != protowire.VarintType {
				return fmt.Errorf("invalid Claude signature: Field 2.1.7 must be varint")
			}
			if _, err := decodeVarintField(raw, "Field 2.1.7"); err != nil {
				return err
			}
			hasField7 = true
			tree.HasField7 = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !haveChannelID {
		return nil, fmt.Errorf("invalid Claude signature: missing Field 2.1.1 channel_id")
	}

	switch tree.ChannelID {
	case 11:
		tree.RoutingClass = "routing_class_11"
	case 12:
		tree.RoutingClass = "routing_class_12"
	}

	if tree.Field2 == nil {
		tree.InfrastructureClass = "infra_default"
	} else {
		switch *tree.Field2 {
		case 1:
			tree.InfrastructureClass = "infra_aws"
		case 2:
			tree.InfrastructureClass = "infra_google"
		default:
			tree.InfrastructureClass = "infra_unknown"
		}
	}

	switch {
	case hasField6:
		tree.SchemaFeatures = "extended_model_tagged_schema"
	case !hasField6 && !hasField7 && len(channelBlock) >= 70 && len(channelBlock) <= 72:
		tree.SchemaFeatures = "compact_schema"
	}

	if tree.ChannelID == 11 {
		switch {
		case tree.Field2 == nil:
			tree.LegacyRouteHint = "legacy_default_group"
		case *tree.Field2 == 1:
			tree.LegacyRouteHint = "legacy_aws_group"
		case *tree.Field2 == 2 && tree.EncodingLayers == 2:
			tree.LegacyRouteHint = "legacy_vertex_direct"
		case *tree.Field2 == 2 && tree.EncodingLayers == 1:
			tree.LegacyRouteHint = "legacy_vertex_proxy"
		}
	}

	return tree, nil
}

// extractBytesField 从 protobuf 消息中提取指定字段号的字节类型字段值。
func extractBytesField(msg []byte, fieldNum protowire.Number, scope string) ([]byte, error) {
	var value []byte
	err := walkProtobufFields(msg, func(num protowire.Number, typ protowire.Type, raw []byte) error {
		if num != fieldNum {
			return nil
		}
		if typ != protowire.BytesType {
			return fmt.Errorf("invalid Claude signature: %s field %d must be bytes", scope, fieldNum)
		}
		bytesValue, err := decodeBytesField(raw, fmt.Sprintf("%s field %d", scope, fieldNum))
		if err != nil {
			return err
		}
		value = bytesValue
		return nil
	})
	if err != nil {
		return nil, err
	}
	if value == nil {
		return nil, fmt.Errorf("invalid Claude signature: missing %s field %d", scope, fieldNum)
	}
	return value, nil
}

// walkProtobufFields 遍历 protobuf 消息的所有字段，对每个字段调用 visit 回调函数。
func walkProtobufFields(msg []byte, visit func(num protowire.Number, typ protowire.Type, raw []byte) error) error {
	for offset := 0; offset < len(msg); {
		num, typ, n := protowire.ConsumeTag(msg[offset:])
		if n < 0 {
			return fmt.Errorf("invalid Claude signature: malformed protobuf tag: %w", protowire.ParseError(n))
		}
		offset += n
		valueLen := protowire.ConsumeFieldValue(num, typ, msg[offset:])
		if valueLen < 0 {
			return fmt.Errorf("invalid Claude signature: malformed protobuf field %d: %w", num, protowire.ParseError(valueLen))
		}
		fieldRaw := msg[offset : offset+valueLen]
		if err := visit(num, typ, fieldRaw); err != nil {
			return err
		}
		offset += valueLen
	}
	return nil
}

// decodeVarintField 解码 protobuf varint 类型的字段值。
func decodeVarintField(raw []byte, label string) (uint64, error) {
	value, n := protowire.ConsumeVarint(raw)
	if n < 0 {
		return 0, fmt.Errorf("invalid Claude signature: failed to decode %s: %w", label, protowire.ParseError(n))
	}
	return value, nil
}

// decodeBytesField 解码 protobuf bytes 类型的字段值。
func decodeBytesField(raw []byte, label string) ([]byte, error) {
	value, n := protowire.ConsumeBytes(raw)
	if n < 0 {
		return nil, fmt.Errorf("invalid Claude signature: failed to decode %s: %w", label, protowire.ParseError(n))
	}
	return value, nil
}
