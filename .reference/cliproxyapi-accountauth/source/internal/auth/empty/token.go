// empty - token.go
// 包 empty 提供无操作的令牌存储实现。
// 当不需要认证令牌或使用 API 密钥认证（而非 OAuth 令牌）时使用此包。
package empty

// EmptyStorage 是 TokenStorage 接口的无操作实现。
// 为不需要令牌存储的场景提供空实现，
// 例如使用 API 密钥而非 OAuth 令牌进行认证时。
type EmptyStorage struct {
	// Type 表示认证提供商类型，此实现始终为 "empty"
	Type string `json:"type"`
}

// SaveTokenToFile 是始终成功的无操作实现。
// 此方法满足 TokenStorage 接口，但不执行实际的文件操作，
// 因为 empty 存储不需要持久化的令牌数据。
//
// 参数：
//   - _: 文件路径参数在此实现中被忽略
//
// 返回：
//   - error: 始终返回 nil（无错误）
func (ts *EmptyStorage) SaveTokenToFile(_ string) error {
	ts.Type = "empty"
	return nil
}
