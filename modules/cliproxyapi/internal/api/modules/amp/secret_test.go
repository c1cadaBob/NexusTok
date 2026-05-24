// amp - secret_test.go
// 多源密钥（MultiSourceSecret）和映射密钥（MappedSecretSource）的单元测试。
// 测试以下功能：
// - 多源密钥的优先级顺序：配置 > 环境变量 > 文件
// - 缓存行为：TTL 内返回缓存值、过期后重新读取、手动失效
// - 文件处理：缺失文件、无效 JSON、缺失键、空值的处理
// - 并发安全性：多协程同时读取的稳定性
// - 静态密钥源的正确行为
// - 空结果缓存：避免重复读取不存在的文件
// - 映射密钥源：根据客户端 API Key 选择对应的上游密钥
// - 重复客户端键的处理：第一个映射获胜并记录警告
package amp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	log "github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
)

// TestMultiSourceSecret_PrecedenceOrder 测试多源密钥的优先级顺序：
// 配置键 > 环境变量 AMP_API_KEY > 文件中的 apiKey@https://ampcode.com/ 键。
// 空白字符串被视为未设置，自动回退到下一个优先级。
func TestMultiSourceSecret_PrecedenceOrder(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name      string
		configKey string
		envKey    string
		fileJSON  string
		want      string
	}{
		{"config_wins", "cfg", "env", `{"apiKey@https://ampcode.com/":"file"}`, "cfg"},
		{"env_wins_when_no_cfg", "", "env", `{"apiKey@https://ampcode.com/":"file"}`, "env"},
		{"file_when_no_cfg_env", "", "", `{"apiKey@https://ampcode.com/":"file"}`, "file"},
		{"empty_cfg_trims_then_env", "   ", "env", `{"apiKey@https://ampcode.com/":"file"}`, "env"},
		{"empty_env_then_file", "", "   ", `{"apiKey@https://ampcode.com/":"file"}`, "file"},
		{"missing_file_returns_empty", "", "", "", ""},
		{"all_empty_returns_empty", "  ", "  ", `{"apiKey@https://ampcode.com/":"  "}`, ""},
	}

	for _, tc := range cases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			secretsPath := filepath.Join(tmpDir, "secrets.json")

			if tc.fileJSON != "" {
				if err := os.WriteFile(secretsPath, []byte(tc.fileJSON), 0600); err != nil {
					t.Fatal(err)
				}
			}

			t.Setenv("AMP_API_KEY", tc.envKey)

			s := NewMultiSourceSecretWithPath(tc.configKey, secretsPath, 100*time.Millisecond)
			got, err := s.Get(ctx)
			if err != nil && tc.fileJSON != "" && json.Valid([]byte(tc.fileJSON)) {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("want %q, got %q", tc.want, got)
			}
		})
	}
}

// TestMultiSourceSecret_CacheBehavior 测试缓存行为：
// - TTL 内返回缓存值（v1），即使文件已更新（v2）
// - TTL 过期后返回新值（v2）
// - InvalidateCache 强制立即重新读取（v3）
func TestMultiSourceSecret_CacheBehavior(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	p := filepath.Join(tmpDir, "secrets.json")

	// Initial value
	if err := os.WriteFile(p, []byte(`{"apiKey@https://ampcode.com/":"v1"}`), 0600); err != nil {
		t.Fatal(err)
	}

	s := NewMultiSourceSecretWithPath("", p, 50*time.Millisecond)

	// First read - should return v1
	got1, err := s.Get(ctx)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got1 != "v1" {
		t.Fatalf("expected v1, got %s", got1)
	}

	// Change file; within TTL we should still see v1 (cached)
	if err := os.WriteFile(p, []byte(`{"apiKey@https://ampcode.com/":"v2"}`), 0600); err != nil {
		t.Fatal(err)
	}
	got2, _ := s.Get(ctx)
	if got2 != "v1" {
		t.Fatalf("cache hit expected v1, got %s", got2)
	}

	// After TTL expires, should see v2
	time.Sleep(60 * time.Millisecond)
	got3, _ := s.Get(ctx)
	if got3 != "v2" {
		t.Fatalf("cache miss expected v2, got %s", got3)
	}

	// Invalidate forces re-read immediately
	if err := os.WriteFile(p, []byte(`{"apiKey@https://ampcode.com/":"v3"}`), 0600); err != nil {
		t.Fatal(err)
	}
	s.InvalidateCache()
	got4, _ := s.Get(ctx)
	if got4 != "v3" {
		t.Fatalf("invalidate expected v3, got %s", got4)
	}
}

// TestMultiSourceSecret_FileHandling 测试各种文件异常情况：
// - 缺失文件不返回错误
// - 无效 JSON 返回错误
// - JSON 中缺失目标键返回空字符串
// - 空白值返回空字符串（修剪后为空）
func TestMultiSourceSecret_FileHandling(t *testing.T) {
	ctx := context.Background()

	t.Run("missing_file_no_error", func(t *testing.T) {
		s := NewMultiSourceSecretWithPath("", "/nonexistent/path/secrets.json", 100*time.Millisecond)
		got, err := s.Get(ctx)
		if err != nil {
			t.Fatalf("expected no error for missing file, got: %v", err)
		}
		if got != "" {
			t.Fatalf("expected empty string, got %q", got)
		}
	})

	t.Run("invalid_json", func(t *testing.T) {
		tmpDir := t.TempDir()
		p := filepath.Join(tmpDir, "secrets.json")
		if err := os.WriteFile(p, []byte(`{invalid json`), 0600); err != nil {
			t.Fatal(err)
		}

		s := NewMultiSourceSecretWithPath("", p, 100*time.Millisecond)
		_, err := s.Get(ctx)
		if err == nil {
			t.Fatal("expected error for invalid JSON")
		}
	})

	t.Run("missing_key_in_json", func(t *testing.T) {
		tmpDir := t.TempDir()
		p := filepath.Join(tmpDir, "secrets.json")
		if err := os.WriteFile(p, []byte(`{"other":"value"}`), 0600); err != nil {
			t.Fatal(err)
		}

		s := NewMultiSourceSecretWithPath("", p, 100*time.Millisecond)
		got, err := s.Get(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "" {
			t.Fatalf("expected empty string for missing key, got %q", got)
		}
	})

	t.Run("empty_key_value", func(t *testing.T) {
		tmpDir := t.TempDir()
		p := filepath.Join(tmpDir, "secrets.json")
		if err := os.WriteFile(p, []byte(`{"apiKey@https://ampcode.com/":"   "}`), 0600); err != nil {
			t.Fatal(err)
		}

		s := NewMultiSourceSecretWithPath("", p, 100*time.Millisecond)
		got, _ := s.Get(ctx)
		if got != "" {
			t.Fatalf("expected empty after trim, got %q", got)
		}
	})
}

// TestMultiSourceSecret_Concurrency 测试并发安全性：
// 50 个协程同时调用 Get 100 次，验证返回值一致且无错误
func TestMultiSourceSecret_Concurrency(t *testing.T) {
	tmpDir := t.TempDir()
	p := filepath.Join(tmpDir, "secrets.json")
	if err := os.WriteFile(p, []byte(`{"apiKey@https://ampcode.com/":"concurrent"}`), 0600); err != nil {
		t.Fatal(err)
	}

	s := NewMultiSourceSecretWithPath("", p, 5*time.Second)
	ctx := context.Background()

	// Spawn many goroutines calling Get concurrently
	const goroutines = 50
	const iterations = 100

	var wg sync.WaitGroup
	errors := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				val, err := s.Get(ctx)
				if err != nil {
					errors <- err
					return
				}
				if val != "concurrent" {
					errors <- err
					return
				}
			}
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrency error: %v", err)
	}
}

// TestStaticSecretSource 测试静态密钥源的行为：
// - 返回提供的密钥
// - 修剪空白
// - 空字符串返回空
func TestStaticSecretSource(t *testing.T) {
	ctx := context.Background()

	t.Run("returns_provided_key", func(t *testing.T) {
		s := NewStaticSecretSource("test-key-123")
		got, err := s.Get(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "test-key-123" {
			t.Fatalf("want test-key-123, got %q", got)
		}
	})

	t.Run("trims_whitespace", func(t *testing.T) {
		s := NewStaticSecretSource("  test-key  ")
		got, err := s.Get(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "test-key" {
			t.Fatalf("want test-key, got %q", got)
		}
	})

	t.Run("empty_string", func(t *testing.T) {
		s := NewStaticSecretSource("")
		got, err := s.Get(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "" {
			t.Fatalf("want empty string, got %q", got)
		}
	})
}

// TestMultiSourceSecret_CacheEmptyResult 测试空结果缓存：
// 缺失文件的结果应被缓存，避免重复文件读取。
// 文件创建后，在缓存过期前仍返回空值，过期后才读取新文件。
// Test that missing file results are cached to avoid repeated file reads
func TestMultiSourceSecret_CacheEmptyResult(t *testing.T) {
	tmpDir := t.TempDir()
	p := filepath.Join(tmpDir, "nonexistent.json")

	s := NewMultiSourceSecretWithPath("", p, 100*time.Millisecond)
	ctx := context.Background()

	// First call - file doesn't exist, should cache empty result
	got1, err := s.Get(ctx)
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	if got1 != "" {
		t.Fatalf("expected empty string, got %q", got1)
	}

	// Create the file now
	if err := os.WriteFile(p, []byte(`{"apiKey@https://ampcode.com/":"new-value"}`), 0600); err != nil {
		t.Fatal(err)
	}

	// Second call - should still return empty (cached), not read the new file
	got2, _ := s.Get(ctx)
	if got2 != "" {
		t.Fatalf("cache should return empty, got %q", got2)
	}

	// After TTL expires, should see the new value
	time.Sleep(110 * time.Millisecond)
	got3, _ := s.Get(ctx)
	if got3 != "new-value" {
		t.Fatalf("after cache expiry, expected new-value, got %q", got3)
	}
}

// TestMappedSecretSource_UsesMappingFromContext 测试映射密钥源：
// 当客户端 API Key（k1）匹配映射表中的条目时，返回对应的上游密钥（u1）；
// 不匹配时回退到默认密钥源（default）。
func TestMappedSecretSource_UsesMappingFromContext(t *testing.T) {
	defaultSource := NewStaticSecretSource("default")
	s := NewMappedSecretSource(defaultSource)
	s.UpdateMappings([]config.AmpUpstreamAPIKeyEntry{
		{
			UpstreamAPIKey: "u1",
			APIKeys:        []string{"k1"},
		},
	})

	ctx := context.WithValue(context.Background(), clientAPIKeyContextKey{}, "k1")
	got, err := s.Get(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "u1" {
		t.Fatalf("want u1, got %q", got)
	}

	ctx = context.WithValue(context.Background(), clientAPIKeyContextKey{}, "k2")
	got, err = s.Get(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "default" {
		t.Fatalf("want default fallback, got %q", got)
	}
}

// TestMappedSecretSource_DuplicateClientKey_FirstWins 测试当同一客户端键出现在多个映射条目中时，
// 第一个映射条目获胜（u1 而非 u2）
func TestMappedSecretSource_DuplicateClientKey_FirstWins(t *testing.T) {
	defaultSource := NewStaticSecretSource("default")
	s := NewMappedSecretSource(defaultSource)
	s.UpdateMappings([]config.AmpUpstreamAPIKeyEntry{
		{
			UpstreamAPIKey: "u1",
			APIKeys:        []string{"k1"},
		},
		{
			UpstreamAPIKey: "u2",
			APIKeys:        []string{"k1"},
		},
	})

	ctx := context.WithValue(context.Background(), clientAPIKeyContextKey{}, "k1")
	got, err := s.Get(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "u1" {
		t.Fatalf("want u1 (first wins), got %q", got)
	}
}

// TestMappedSecretSource_DuplicateClientKey_LogsWarning 测试当同一客户端键出现在多个映射条目中时，
// 应记录警告日志
func TestMappedSecretSource_DuplicateClientKey_LogsWarning(t *testing.T) {
	hook := test.NewLocal(log.StandardLogger())
	defer hook.Reset()

	defaultSource := NewStaticSecretSource("default")
	s := NewMappedSecretSource(defaultSource)
	s.UpdateMappings([]config.AmpUpstreamAPIKeyEntry{
		{
			UpstreamAPIKey: "u1",
			APIKeys:        []string{"k1"},
		},
		{
			UpstreamAPIKey: "u2",
			APIKeys:        []string{"k1"},
		},
	})

	foundWarning := false
	for _, entry := range hook.AllEntries() {
		if entry.Level == log.WarnLevel && entry.Message == "amp upstream-api-keys: client API key appears in multiple entries; using first mapping." {
			foundWarning = true
			break
		}
	}
	if !foundWarning {
		t.Fatal("expected warning log for duplicate client key, but none was found")
	}
}
