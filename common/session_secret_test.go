package common

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveSessionSecretPersistsGeneratedSecret(t *testing.T) {
	t.Setenv("SESSION_SECRET", "")
	secretFile := filepath.Join(t.TempDir(), "session_secret")
	t.Setenv("SESSION_SECRET_FILE", secretFile)

	first, err := resolveSessionSecret()
	require.NoError(t, err)
	require.NotEmpty(t, first)

	second, err := resolveSessionSecret()
	require.NoError(t, err)
	assert.Equal(t, first, second)

	info, err := os.Stat(secretFile)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestResolveSessionSecretPrefersEnvironmentAndRejectsUnsafeDefault(t *testing.T) {
	secretFile := filepath.Join(t.TempDir(), "session_secret")
	require.NoError(t, os.WriteFile(secretFile, []byte("from-file\n"), 0600))
	t.Setenv("SESSION_SECRET_FILE", secretFile)
	t.Setenv("SESSION_SECRET", "from-env")

	secret, err := resolveSessionSecret()
	require.NoError(t, err)
	assert.Equal(t, "from-env", secret)

	t.Setenv("SESSION_SECRET", "random_string")
	_, err = resolveSessionSecret()
	assert.ErrorIs(t, err, errDefaultSessionSecret)
}

func TestResolveSessionSecretIgnoresEmptyFile(t *testing.T) {
	secretFile := filepath.Join(t.TempDir(), "session_secret")
	require.NoError(t, os.WriteFile(secretFile, []byte("\n"), 0600))
	t.Setenv("SESSION_SECRET", "")
	t.Setenv("SESSION_SECRET_FILE", secretFile)

	secret, err := resolveSessionSecret()
	require.NoError(t, err)
	assert.NotEmpty(t, secret)

	data, err := os.ReadFile(secretFile)
	require.NoError(t, err)
	assert.Equal(t, secret, string(data[:len(data)-1]))
}

func TestResolveCryptoSecretPriority(t *testing.T) {
	secretFile := filepath.Join(t.TempDir(), "crypto_secret")
	require.NoError(t, os.WriteFile(secretFile, []byte("from-file\n"), 0600))
	t.Setenv("CRYPTO_SECRET_FILE", secretFile)
	t.Setenv("CRYPTO_SECRET", "")

	secret, err := resolveCryptoSecret("from-session")
	require.NoError(t, err)
	assert.Equal(t, "from-file", secret)

	t.Setenv("CRYPTO_SECRET", "from-env")
	secret, err = resolveCryptoSecret("from-session")
	require.NoError(t, err)
	assert.Equal(t, "from-env", secret)

	t.Setenv("CRYPTO_SECRET", "")
	t.Setenv("CRYPTO_SECRET_FILE", "")
	secret, err = resolveCryptoSecret("from-session")
	require.NoError(t, err)
	assert.Equal(t, "from-session", secret)
}
