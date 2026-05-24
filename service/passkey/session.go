// session.go - Passkey 会话数据管理
// 本文件提供 WebAuthn 会话数据的存储和读取功能。
// 使用 Gin 的 session 中间件存储 WebAuthn 的 SessionData，
// 支持保存（SaveSessionData）和弹出读取（PopSessionData，读后即删）两种操作。
package passkey

import (
	"encoding/json"
	"errors"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	webauthn "github.com/go-webauthn/webauthn/webauthn"
)

// errSessionNotFound 会话不存在或已过期的错误
var errSessionNotFound = errors.New("Passkey 会话不存在或已过期")

// SaveSessionData 保存 WebAuthn 会话数据到 Gin session 中。
// data 为 nil 时删除已有的会话数据。
// 参数:
//   - c: Gin 上下文
//   - key: session 存储键名
//   - data: 待保存的 WebAuthn 会话数据，nil 表示删除
// 返回值:
//   - error: 序列化或保存失败时返回错误
func SaveSessionData(c *gin.Context, key string, data *webauthn.SessionData) error {
	session := sessions.Default(c)
	if data == nil {
		session.Delete(key)
		return session.Save()
	}
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	session.Set(key, string(payload))
	return session.Save()
}

// PopSessionData 从 Gin session 中弹出（读取并删除）WebAuthn 会话数据。
// 支持 string 和 []byte 两种存储格式的反序列化。
// 参数:
//   - c: Gin 上下文
//   - key: session 存储键名
// 返回值:
//   - *webauthn.SessionData: 读取到的会话数据
//   - error: 会话不存在、格式无效或反序列化失败时返回错误
func PopSessionData(c *gin.Context, key string) (*webauthn.SessionData, error) {
	session := sessions.Default(c)
	raw := session.Get(key)
	if raw == nil {
		return nil, errSessionNotFound
	}
	session.Delete(key)
	_ = session.Save()
	var data webauthn.SessionData
	switch value := raw.(type) {
	case string:
		if err := json.Unmarshal([]byte(value), &data); err != nil {
			return nil, err
		}
	case []byte:
		if err := json.Unmarshal(value, &data); err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("Passkey 会话格式无效")
	}
	return &data, nil
}
