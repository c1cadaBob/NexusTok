// empty - token.go
// 提供空操作的 Token 存储实现。
// 当不需要认证令牌或使用 API 密钥认证时使用此包。
package empty

// EmptyStorage 是 TokenStorage 接口的空操作实现。
// 用于不需要令牌存储的场景，如使用 API 密钥而非 OAuth 令牌进行认证。
type EmptyStorage struct {
	// Type indicates the authentication provider type, always "empty" for this implementation.
	Type string `json:"type"`
}

// SaveTokenToFile is a no-operation implementation that always succeeds.
// This method satisfies the TokenStorage interface but performs no actual file operations
// since empty storage doesn't require persistent token data.
//
// Parameters:
//   - _: The file path parameter is ignored in this implementation
//
// Returns:
//   - error: Always returns nil (no error)
func (ts *EmptyStorage) SaveTokenToFile(_ string) error {
	ts.Type = "empty"
	return nil
}
