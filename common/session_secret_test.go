package common

// 本文件测试会话密钥（Session Secret）的解析与管理逻辑。
// 测试范围包括：密钥的自动生成与持久化、环境变量优先级、不安全默认值的拒绝，
// 以及会话最大存活时间（Session MaxAge）的解析。

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestResolveSessionSecretPersistsGeneratedSecret 测试会话密钥的自动生成和持久化行为。
// 预期行为：首次调用时自动生成密钥并写入文件，后续调用应返回相同的密钥，
// 且密钥文件权限必须为 0600（仅所有者可读写）。
func TestResolveSessionSecretPersistsGeneratedSecret(t *testing.T) {
	t.Setenv("SESSION_SECRET", "")
	secretFile := filepath.Join(t.TempDir(), "session_secret")
	t.Setenv("SESSION_SECRET_FILE", secretFile)

	// 首次调用：应自动生成密钥
	first, err := resolveSessionSecret()
	if err != nil {
		t.Fatalf("resolveSessionSecret() error = %v", err)
	}
	// 断言：生成的密钥不能为空
	if first == "" {
		t.Fatal("resolveSessionSecret() returned empty secret")
	}

	// 第二次调用：应返回相同的密钥（持久化验证）
	second, err := resolveSessionSecret()
	if err != nil {
		t.Fatalf("resolveSessionSecret() second call error = %v", err)
	}
	// 断言：两次调用返回的密钥必须一致，证明持久化生效
	if first != second {
		t.Fatalf("expected persisted secret %q, got %q", first, second)
	}

	// 验证密钥文件的权限设置
	info, err := os.Stat(secretFile)
	if err != nil {
		t.Fatalf("stat secret file: %v", err)
	}
	// 断言：密钥文件权限必须为 0600，防止未授权访问
	if info.Mode().Perm() != 0600 {
		t.Fatalf("secret file mode = %v, want 0600", info.Mode().Perm())
	}
}

// TestResolveSessionSecretPrefersEnv 测试环境变量的优先级高于文件。
// 当同时设置了 SESSION_SECRET 环境变量和 SESSION_SECRET_FILE 时，
// 应优先使用环境变量的值。
func TestResolveSessionSecretPrefersEnv(t *testing.T) {
	t.Setenv("SESSION_SECRET", "from-env")
	secretFile := filepath.Join(t.TempDir(), "session_secret")
	t.Setenv("SESSION_SECRET_FILE", secretFile)

	// 写入一个不同的密钥到文件中
	if err := os.WriteFile(secretFile, []byte("from-file\n"), 0600); err != nil {
		t.Fatalf("write secret file: %v", err)
	}

	secret, err := resolveSessionSecret()
	if err != nil {
		t.Fatalf("resolveSessionSecret() error = %v", err)
	}
	// 断言：应返回环境变量中的值，而非文件中的值
	if secret != "from-env" {
		t.Fatalf("secret = %q, want env secret", secret)
	}
}

// TestResolveSessionSecretRejectsDefaultValue 测试拒绝不安全的默认密钥值。
// 当 SESSION_SECRET 被设置为常见的不安全默认值 "random_string" 时，
// 应返回 errDefaultSessionSecret 错误以强制用户设置安全密钥。
func TestResolveSessionSecretRejectsDefaultValue(t *testing.T) {
	t.Setenv("SESSION_SECRET", "random_string")
	t.Setenv("SESSION_SECRET_FILE", filepath.Join(t.TempDir(), "session_secret"))

	_, err := resolveSessionSecret()
	// 断言：应拒绝不安全的默认密钥值
	if !errors.Is(err, errDefaultSessionSecret) {
		t.Fatalf("resolveSessionSecret() error = %v, want errDefaultSessionSecret", err)
	}
}

// TestResolveSessionMaxAge 测试会话最大存活时间的解析逻辑。
// 验证正常数值的解析以及无效值（负数）时回退到默认值的行为。
func TestResolveSessionMaxAge(t *testing.T) {
	t.Setenv("SESSION_MAX_AGE", "7776000")
	// 断言：正常数值应被正确解析（7776000 秒 = 90 天）
	if got := resolveSessionMaxAge(10); got != 7776000 {
		t.Fatalf("resolveSessionMaxAge() = %d, want 7776000", got)
	}

	t.Setenv("SESSION_MAX_AGE", "-1")
	// 断言：负数等无效值应回退到传入的默认值（10）
	if got := resolveSessionMaxAge(10); got != 10 {
		t.Fatalf("resolveSessionMaxAge() with invalid value = %d, want default", got)
	}
}
