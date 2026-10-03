package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

const (
	newAPIPasswordEncryptionPath = "/api/user/login/encryption-key"
	newAPIPasswordV2Label        = "password-v2"
	newAPIPasswordV2NonceSize    = 12
	newAPIPasswordV2KeySize      = 32
)

type newAPIPasswordEncryptionConfig struct {
	keyID     string
	publicKey *rsa.PublicKey
	enabled   bool
}

func fetchNewAPIPasswordEncryptionConfig(
	ctx context.Context,
	session *PlatformSiteSession,
) (*newAPIPasswordEncryptionConfig, error) {
	payload, err := platformSiteRequest(
		ctx,
		session,
		http.MethodGet,
		newAPIPasswordEncryptionPath,
		nil,
		nil,
	)
	if err != nil {
		return nil, err
	}

	record := firstRecord(payload)
	enabled, ok := record["enabled"].(bool)
	if !ok {
		return nil, fmt.Errorf("%w: NewAPI 密码加密开关无效", ErrPlatformSiteResponse)
	}
	if !enabled {
		return &newAPIPasswordEncryptionConfig{}, nil
	}

	keyID := firstString(record, "kid", "key_id", "keyId")
	publicKeyPEM := firstString(record, "public_key", "publicKey")
	if keyID == "" || publicKeyPEM == "" {
		return nil, fmt.Errorf("%w: NewAPI 密码加密公钥缺失", ErrPlatformSiteResponse)
	}
	publicKey, err := parseNewAPIPasswordPublicKey(publicKeyPEM)
	if err != nil {
		return nil, err
	}
	return &newAPIPasswordEncryptionConfig{
		keyID:     keyID,
		publicKey: publicKey,
		enabled:   true,
	}, nil
}

func parseNewAPIPasswordPublicKey(publicKeyPEM string) (*rsa.PublicKey, error) {
	block, rest := pem.Decode([]byte(publicKeyPEM))
	if block == nil || strings.TrimSpace(string(rest)) != "" {
		return nil, fmt.Errorf("%w: NewAPI 密码加密公钥格式无效", ErrPlatformSiteResponse)
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%w: NewAPI 密码加密公钥解析失败", ErrPlatformSiteResponse)
	}
	publicKey, ok := parsed.(*rsa.PublicKey)
	if !ok || publicKey.N == nil || publicKey.N.BitLen() < 2048 {
		return nil, fmt.Errorf("%w: NewAPI 密码加密公钥类型无效", ErrPlatformSiteResponse)
	}
	return publicKey, nil
}

func encryptNewAPIPassword(
	password string,
	config *newAPIPasswordEncryptionConfig,
) (string, error) {
	if config == nil || !config.enabled || config.publicKey == nil || config.keyID == "" {
		return "", errors.New("NewAPI 密码加密配置不可用")
	}
	plaintext := []byte(password)
	maxRSAPlaintextSize := config.publicKey.Size() - 2*sha256.Size - 2
	if len(plaintext) <= maxRSAPlaintextSize {
		ciphertext, err := rsa.EncryptOAEP(
			sha256.New(),
			rand.Reader,
			config.publicKey,
			plaintext,
			nil,
		)
		if err != nil {
			return "", fmt.Errorf("NewAPI 密码 RSA 加密失败: %w", err)
		}
		return base64.StdEncoding.EncodeToString(ciphertext), nil
	}

	secret := make([]byte, newAPIPasswordV2KeySize)
	if _, err := rand.Read(secret); err != nil {
		return "", fmt.Errorf("NewAPI 密码信封密钥生成失败: %w", err)
	}
	nonce := make([]byte, newAPIPasswordV2NonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("NewAPI 密码信封随机数生成失败: %w", err)
	}
	wrappedKey, err := rsa.EncryptOAEP(
		sha256.New(),
		rand.Reader,
		config.publicKey,
		secret,
		[]byte(newAPIPasswordV2Label),
	)
	if err != nil {
		return "", fmt.Errorf("NewAPI 密码信封密钥加密失败: %w", err)
	}
	block, err := aes.NewCipher(secret)
	if err != nil {
		return "", fmt.Errorf("NewAPI 密码信封密码创建失败: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("NewAPI 密码信封模式创建失败: %w", err)
	}
	ciphertext := gcm.Seal(
		nil,
		nonce,
		plaintext,
		[]byte(newAPIPasswordV2Label+":"+config.keyID),
	)
	return strings.Join([]string{
		"v2",
		base64.StdEncoding.EncodeToString(wrappedKey),
		base64.StdEncoding.EncodeToString(nonce),
		base64.StdEncoding.EncodeToString(ciphertext),
	}, "."), nil
}
