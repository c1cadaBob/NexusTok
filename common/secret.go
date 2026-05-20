package common

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"strings"
)

const encryptedSecretPrefix = "enc:v1:"

func secretCipher() (cipher.AEAD, error) {
	sum := sha256.Sum256([]byte(CryptoSecret))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// EncryptSensitiveString 使用 CRYPTO_SECRET/SESSION_SECRET 派生密钥加密敏感账号凭证。
func EncryptSensitiveString(plain string) (string, error) {
	plain = strings.TrimSpace(plain)
	if plain == "" {
		return "", nil
	}
	if strings.HasPrefix(plain, encryptedSecretPrefix) {
		return plain, nil
	}
	aead, err := secretCipher()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := aead.Seal(nil, nonce, []byte(plain), nil)
	payload := append(nonce, ciphertext...)
	return encryptedSecretPrefix + base64.RawURLEncoding.EncodeToString(payload), nil
}

// DecryptSensitiveString 解密账号凭证；未加密的历史明文会原样返回以便平滑迁移。
func DecryptSensitiveString(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if !strings.HasPrefix(value, encryptedSecretPrefix) {
		return value, nil
	}
	aead, err := secretCipher()
	if err != nil {
		return "", err
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(value, encryptedSecretPrefix))
	if err != nil {
		return "", err
	}
	if len(payload) <= aead.NonceSize() {
		return "", errors.New("invalid encrypted secret payload")
	}
	nonce := payload[:aead.NonceSize()]
	ciphertext := payload[aead.NonceSize():]
	plain, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
