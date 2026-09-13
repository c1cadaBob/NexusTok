package common

import (
	"crypto/aes"
	"crypto/cipher"
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

const upstreamCredentialCipherVersion = "v1"

var ErrUpstreamCredentialInvalid = errors.New("upstream credential is invalid")

// EncryptUpstreamCredential protects platform credentials at rest with the
// process-wide CRYPTO_SECRET. The version prefix leaves room for key rotation.
func EncryptUpstreamCredential(plaintext string) (string, error) {
	if strings.TrimSpace(CryptoSecret) == "" {
		return "", ErrUpstreamCredentialInvalid
	}
	key := sha256.Sum256([]byte(CryptoSecret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", fmt.Errorf("create upstream credential cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create upstream credential gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(crand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate upstream credential nonce: %w", err)
	}
	ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), []byte(upstreamCredentialCipherVersion))
	return strings.Join([]string{
		upstreamCredentialCipherVersion,
		base64.RawStdEncoding.EncodeToString(nonce),
		base64.RawStdEncoding.EncodeToString(ciphertext),
	}, "."), nil
}

// DecryptUpstreamCredential decrypts a credential envelope without exposing
// secret material in errors.
func DecryptUpstreamCredential(envelope string) (string, error) {
	if strings.TrimSpace(CryptoSecret) == "" {
		return "", ErrUpstreamCredentialInvalid
	}
	parts := strings.Split(envelope, ".")
	if len(parts) != 3 || parts[0] != upstreamCredentialCipherVersion {
		return "", ErrUpstreamCredentialInvalid
	}
	nonce, err := base64.RawStdEncoding.Strict().DecodeString(parts[1])
	if err != nil {
		return "", ErrUpstreamCredentialInvalid
	}
	ciphertext, err := base64.RawStdEncoding.Strict().DecodeString(parts[2])
	if err != nil {
		return "", ErrUpstreamCredentialInvalid
	}
	key := sha256.Sum256([]byte(CryptoSecret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", ErrUpstreamCredentialInvalid
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || len(nonce) != gcm.NonceSize() {
		return "", ErrUpstreamCredentialInvalid
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, []byte(upstreamCredentialCipherVersion))
	if err != nil {
		return "", ErrUpstreamCredentialInvalid
	}
	return string(plaintext), nil
}
