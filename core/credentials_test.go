package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCredentialStoreSetGet(t *testing.T) {
	dir := t.TempDir()
	cs := NewCredentialStore(dir)
	ctx := context.Background()

	// Set + Get
	if err := cs.Set(ctx, "llm-anthropic", "api_key", "sk-12345"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	val, err := cs.Get(ctx, "llm-anthropic", "api_key")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != "sk-12345" {
		t.Errorf("Get = %q, want sk-12345", val)
	}
}

func TestCredentialStoreDelete(t *testing.T) {
	dir := t.TempDir()
	cs := NewCredentialStore(dir)
	ctx := context.Background()

	cs.Set(ctx, "plugin-x", "token", "abc")
	cs.Set(ctx, "plugin-x", "secret", "xyz")

	if err := cs.Delete(ctx, "plugin-x", "token"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := cs.Get(ctx, "plugin-x", "token")
	if err == nil {
		t.Error("deleted credential should not be found")
	}

	// 另一个 key 仍在
	val, err := cs.Get(ctx, "plugin-x", "secret")
	if err != nil {
		t.Fatalf("Get secret: %v", err)
	}
	if val != "xyz" {
		t.Errorf("secret = %q, want xyz", val)
	}
}

func TestCredentialStoreListKeys(t *testing.T) {
	dir := t.TempDir()
	cs := NewCredentialStore(dir)
	ctx := context.Background()

	cs.Set(ctx, "plugin-y", "key1", "val1")
	cs.Set(ctx, "plugin-y", "key2", "val2")

	keys, err := cs.ListKeys(ctx, "plugin-y")
	if err != nil {
		t.Fatalf("ListKeys: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}
}

func TestCredentialStoreEnvFallback(t *testing.T) {
	dir := t.TempDir()
	cs := NewCredentialStore(dir)
	ctx := context.Background()

	// 设置环境变量
	os.Setenv("LLM_ANTHROPIC_API_KEY", "sk-from-env")
	defer os.Unsetenv("LLM_ANTHROPIC_API_KEY")

	// 无文件记录 → 回退到环境变量
	val, err := cs.Get(ctx, "llm-anthropic", "api_key")
	if err != nil {
		t.Fatalf("Get with env fallback: %v", err)
	}
	if val != "sk-from-env" {
		t.Errorf("env fallback = %q, want sk-from-env", val)
	}
}

func TestCredentialStoreNotFound(t *testing.T) {
	dir := t.TempDir()
	cs := NewCredentialStore(dir)
	ctx := context.Background()

	_, err := cs.Get(ctx, "nonexistent", "key")
	if err == nil {
		t.Error("should return error for nonexistent credential")
	}
}

func TestCredentialStorePersistence(t *testing.T) {
	dir := t.TempDir()
	cs := NewCredentialStore(dir)
	ctx := context.Background()

	cs.Set(ctx, "plugin-z", "cred", "secret-value")

	// 创建新实例加载同一目录
	cs2 := NewCredentialStore(dir)
	val, err := cs2.Get(ctx, "plugin-z", "cred")
	if err != nil {
		t.Fatalf("Get from new instance: %v", err)
	}
	if val != "secret-value" {
		t.Errorf("persistence = %q, want secret-value", val)
	}

	// 文件权限应为 0600
	info, err := os.Stat(filepath.Join(dir, "plugin-z.json"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("file perm = %o, want 0600", info.Mode().Perm())
	}
}

func TestCredEnvKey(t *testing.T) {
	cases := []struct {
		pluginName, key, want string
	}{
		{"llm-anthropic", "api_key", "LLM_ANTHROPIC_API_KEY"},
		{"tool-ssh", "password", "TOOL_SSH_PASSWORD"},
		{"dsc-notify", "token", "DSC_NOTIFY_TOKEN"},
	}
	for _, c := range cases {
		if got := credEnvKey(c.pluginName, c.key); got != c.want {
			t.Errorf("credEnvKey(%q, %q) = %q, want %q", c.pluginName, c.key, got, c.want)
		}
	}
}
