package common

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultContainerSessionSecretFile = "/data/session_secret"
	defaultLocalSessionSecretFile     = "data/session_secret"
	generatedSessionSecretBytes       = 48
)

var errDefaultSessionSecret = errors.New("SESSION_SECRET uses the unsafe default value")

func resolveSessionSecret() (string, error) {
	if secret := strings.TrimSpace(os.Getenv("SESSION_SECRET")); secret != "" {
		if secret == "random_string" {
			return "", errDefaultSessionSecret
		}
		return secret, nil
	}
	return resolveOrCreateSecretFile("SESSION_SECRET_FILE", getSessionSecretFile())
}

func resolveCryptoSecret(sessionSecret string) (string, error) {
	if secret := strings.TrimSpace(os.Getenv("CRYPTO_SECRET")); secret != "" {
		return secret, nil
	}
	if secretFile := strings.TrimSpace(os.Getenv("CRYPTO_SECRET_FILE")); secretFile != "" {
		return resolveOrCreateSecretFile("CRYPTO_SECRET_FILE", secretFile)
	}
	return sessionSecret, nil
}

func resolveOrCreateSecretFile(envName, secretFile string) (string, error) {
	if data, err := os.ReadFile(secretFile); err == nil {
		if secret := strings.TrimSpace(string(data)); secret != "" {
			return secret, nil
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("read %s file %q: %w", envName, secretFile, err)
	}

	secret, err := generateSessionSecret()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(secretFile), 0700); err != nil {
		return "", fmt.Errorf("create %s directory %q: %w", envName, filepath.Dir(secretFile), err)
	}
	if err := os.WriteFile(secretFile, []byte(secret+"\n"), 0600); err != nil {
		return "", fmt.Errorf("write %s file %q: %w", envName, secretFile, err)
	}
	return secret, nil
}

func getSessionSecretFile() string {
	if secretFile := strings.TrimSpace(os.Getenv("SESSION_SECRET_FILE")); secretFile != "" {
		return secretFile
	}
	if IsRunningInContainer() {
		return defaultContainerSessionSecretFile
	}
	return defaultLocalSessionSecretFile
}

func generateSessionSecret() (string, error) {
	buffer := make([]byte, generatedSessionSecretBytes)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate session secret: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}
