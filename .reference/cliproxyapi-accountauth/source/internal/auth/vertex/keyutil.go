// vertex - keyutil.go
// 包 vertex 提供 Google Vertex AI 的服务账户凭证管理功能。
// 该文件实现了服务账户 JSON 的规范化和私钥清理功能，
// 确保私钥字段包含有效的 RSA PRIVATE KEY PEM 块。
package vertex

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"strings"
)

// NormalizeServiceAccountJSON 规范化给定的 JSON 编码的服务账户有效负载。
// 返回规范化后的 JSON（带有清理后的 private_key），如果规范化失败，
// 则返回原始字节和遇到的错误。
//
// 参数：
//   - raw: 原始服务账户 JSON 字节
//
// 返回：
//   - []byte: 规范化后的 JSON 字节
//   - error: 规范化失败时返回的错误
func NormalizeServiceAccountJSON(raw []byte) ([]byte, error) {
	if len(raw) == 0 {
		return raw, nil
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return raw, err
	}
	normalized, err := NormalizeServiceAccountMap(payload)
	if err != nil {
		return raw, err
	}
	out, err := json.Marshal(normalized)
	if err != nil {
		return raw, err
	}
	return out, nil
}

// NormalizeServiceAccountMap 返回给定服务账户映射的副本，
// 其中 private_key 字段经过清理，保证包含有效的 RSA PRIVATE KEY PEM 块。
//
// 参数：
//   - sa: 服务账户映射
//
// 返回：
//   - map[string]any: 规范化后的服务账户映射副本
//   - error: 规范化失败时返回的错误
func NormalizeServiceAccountMap(sa map[string]any) (map[string]any, error) {
	if sa == nil {
		return nil, fmt.Errorf("service account payload is empty")
	}
	pk, _ := sa["private_key"].(string)
	if strings.TrimSpace(pk) == "" {
		return nil, fmt.Errorf("service account missing private_key")
	}
	normalized, err := sanitizePrivateKey(pk)
	if err != nil {
		return nil, err
	}
	clone := make(map[string]any, len(sa))
	for k, v := range sa {
		clone[k] = v
	}
	clone["private_key"] = normalized
	return clone, nil
}

// sanitizePrivateKey 清理私钥字符串，确保其为有效的 PEM 格式。
// 处理行尾符、ANSI 转义序列和编码问题。
//
// 参数：
//   - raw: 原始私钥字符串
//
// 返回：
//   - string: 清理后的 PEM 格式私钥
//   - error: 清理失败时返回的错误
func sanitizePrivateKey(raw string) (string, error) {
	pk := strings.ReplaceAll(raw, "\r\n", "\n")
	pk = strings.ReplaceAll(pk, "\r", "\n")
	pk = stripANSIEscape(pk)
	pk = strings.ToValidUTF8(pk, "")
	pk = strings.TrimSpace(pk)

	normalized := pk
	if block, _ := pem.Decode([]byte(pk)); block == nil {
		// Attempt to reconstruct from the textual payload.
		if reconstructed, err := rebuildPEM(pk); err == nil {
			normalized = reconstructed
		} else {
			return "", fmt.Errorf("private_key is not valid pem: %w", err)
		}
	}

	block, _ := pem.Decode([]byte(normalized))
	if block == nil {
		return "", fmt.Errorf("private_key pem decode failed")
	}

	rsaBlock, err := ensureRSAPrivateKey(block)
	if err != nil {
		return "", err
	}
	return string(pem.EncodeToMemory(rsaBlock)), nil
}

// ensureRSAPrivateKey 确保 PEM 块包含 RSA 私钥。
// 支持 PKCS#1 和 PKCS#8 格式的私钥。
//
// 参数：
//   - block: PEM 块
//
// 返回：
//   - *pem.Block: 包含 RSA 私钥的 PEM 块
//   - error: 私钥格式不支持时返回的错误
func ensureRSAPrivateKey(block *pem.Block) (*pem.Block, error) {
	if block == nil {
		return nil, fmt.Errorf("pem block is nil")
	}

	if block.Type == "RSA PRIVATE KEY" {
		if _, err := x509.ParsePKCS1PrivateKey(block.Bytes); err != nil {
			return nil, fmt.Errorf("private_key invalid rsa: %w", err)
		}
		return block, nil
	}

	if block.Type == "PRIVATE KEY" {
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("private_key invalid pkcs8: %w", err)
		}
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("private_key is not an RSA key")
		}
		der := x509.MarshalPKCS1PrivateKey(rsaKey)
		return &pem.Block{Type: "RSA PRIVATE KEY", Bytes: der}, nil
	}

	// Attempt auto-detection: try PKCS#1 first, then PKCS#8.
	if rsaKey, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		der := x509.MarshalPKCS1PrivateKey(rsaKey)
		return &pem.Block{Type: "RSA PRIVATE KEY", Bytes: der}, nil
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if rsaKey, ok := key.(*rsa.PrivateKey); ok {
			der := x509.MarshalPKCS1PrivateKey(rsaKey)
			return &pem.Block{Type: "RSA PRIVATE KEY", Bytes: der}, nil
		}
	}
	return nil, fmt.Errorf("private_key uses unsupported format")
}

// rebuildPEM 从原始文本有效负载重建 PEM 格式。
// 提取 PEM 标记之间的 Base64 内容并重新编码。
//
// 参数：
//   - raw: 原始文本
//
// 返回：
//   - string: 重建的 PEM 格式字符串
//   - error: 重建失败时返回的错误
func rebuildPEM(raw string) (string, error) {
	kind := "PRIVATE KEY"
	if strings.Contains(raw, "RSA PRIVATE KEY") {
		kind = "RSA PRIVATE KEY"
	}
	header := "-----BEGIN " + kind + "-----"
	footer := "-----END " + kind + "-----"
	start := strings.Index(raw, header)
	end := strings.Index(raw, footer)
	if start < 0 || end <= start {
		return "", fmt.Errorf("missing pem markers")
	}
	body := raw[start+len(header) : end]
	payload := filterBase64(body)
	if payload == "" {
		return "", fmt.Errorf("private_key base64 payload empty")
	}
	der, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return "", fmt.Errorf("private_key base64 decode failed: %w", err)
	}
	block := &pem.Block{Type: kind, Bytes: der}
	return string(pem.EncodeToMemory(block)), nil
}

// filterBase64 从字符串中过滤出有效的 Base64 字符。
// 仅保留字母、数字和 Base64 特殊字符（+、/、=）。
//
// 参数：
//   - s: 输入字符串
//
// 返回：
//   - string: 过滤后的 Base64 字符串
func filterBase64(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '+' || r == '/' || r == '=':
			b.WriteRune(r)
		default:
			// skip
		}
	}
	return b.String()
}

// stripANSIEscape 从字符串中移除 ANSI 转义序列。
// 处理 OSC 序列（]...）和 CSI 序列（[...）。
//
// 参数：
//   - s: 可能包含 ANSI 转义序列的字符串
//
// 返回：
//   - string: 移除转义序列后的字符串
func stripANSIEscape(s string) string {
	in := []rune(s)
	var out []rune
	for i := 0; i < len(in); i++ {
		r := in[i]
		if r != 0x1b {
			out = append(out, r)
			continue
		}
		if i+1 >= len(in) {
			continue
		}
		next := in[i+1]
		switch next {
		case ']':
			i += 2
			for i < len(in) {
				if in[i] == 0x07 {
					break
				}
				if in[i] == 0x1b && i+1 < len(in) && in[i+1] == '\\' {
					i++
					break
				}
				i++
			}
		case '[':
			i += 2
			for i < len(in) {
				if (in[i] >= 'A' && in[i] <= 'Z') || (in[i] >= 'a' && in[i] <= 'z') {
					break
				}
				i++
			}
		default:
			// skip single ESC
		}
	}
	return string(out)
}
