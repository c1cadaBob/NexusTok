package common

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveSessionSecretPersistsGeneratedSecret(t *testing.T) {
	t.Setenv("SESSION_SECRET", "")
	secretFile := filepath.Join(t.TempDir(), "session_secret")
	t.Setenv("SESSION_SECRET_FILE", secretFile)

	first, err := resolveSessionSecret()
	if err != nil {
		t.Fatalf("resolveSessionSecret() error = %v", err)
	}
	if first == "" {
		t.Fatal("resolveSessionSecret() returned empty secret")
	}

	second, err := resolveSessionSecret()
	if err != nil {
		t.Fatalf("resolveSessionSecret() second call error = %v", err)
	}
	if first != second {
		t.Fatalf("expected persisted secret %q, got %q", first, second)
	}

	info, err := os.Stat(secretFile)
	if err != nil {
		t.Fatalf("stat secret file: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("secret file mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestResolveSessionSecretPrefersEnv(t *testing.T) {
	t.Setenv("SESSION_SECRET", "from-env")
	secretFile := filepath.Join(t.TempDir(), "session_secret")
	t.Setenv("SESSION_SECRET_FILE", secretFile)

	if err := os.WriteFile(secretFile, []byte("from-file\n"), 0600); err != nil {
		t.Fatalf("write secret file: %v", err)
	}

	secret, err := resolveSessionSecret()
	if err != nil {
		t.Fatalf("resolveSessionSecret() error = %v", err)
	}
	if secret != "from-env" {
		t.Fatalf("secret = %q, want env secret", secret)
	}
}

func TestResolveSessionSecretRejectsDefaultValue(t *testing.T) {
	t.Setenv("SESSION_SECRET", "random_string")
	t.Setenv("SESSION_SECRET_FILE", filepath.Join(t.TempDir(), "session_secret"))

	_, err := resolveSessionSecret()
	if !errors.Is(err, errDefaultSessionSecret) {
		t.Fatalf("resolveSessionSecret() error = %v, want errDefaultSessionSecret", err)
	}
}

func TestResolveSessionMaxAge(t *testing.T) {
	t.Setenv("SESSION_MAX_AGE", "7776000")
	if got := resolveSessionMaxAge(10); got != 7776000 {
		t.Fatalf("resolveSessionMaxAge() = %d, want 7776000", got)
	}

	t.Setenv("SESSION_MAX_AGE", "-1")
	if got := resolveSessionMaxAge(10); got != 10 {
		t.Fatalf("resolveSessionMaxAge() with invalid value = %d, want default", got)
	}
}
