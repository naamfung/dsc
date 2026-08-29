package core

import (
	"strings"
	"testing"
)

// TestPluginEnvPassThrough 验证插件 env = 宿主环境 + 插件自定义 env
// （互通服务 ID 改经 SetInterconnect 握手传入，不经 env）。
func TestPluginEnvPassThrough(t *testing.T) {
	m := NewManager(&ManagerConfig{})
	env := m.coreEnv(PluginEntry{Env: map[string]string{"A": "1"}})
	if !containsEnv(env, "A", "1") {
		t.Fatalf("custom env should pass through, got %v", env)
	}
	if containsEnvKey(env, "DSC_LLM_SERVICE_ID") || containsEnvKey(env, "DSC_NOTIFY_SERVICE_ID") {
		t.Fatalf("interconnect ids must not be injected via env, got %v", env)
	}
}

func containsEnv(env []string, key, val string) bool {
	for _, kv := range env {
		if kv == key+"="+val {
			return true
		}
	}
	return false
}

func containsEnvKey(env []string, key string) bool {
	for _, kv := range env {
		if strings.HasPrefix(kv, key+"=") {
			return true
		}
	}
	return false
}

// TestBuildEnvStripsSecretsForNonLLM 验证 P1-4：非 LLM 插件过滤凭据，LLM 插件保留。
func TestBuildEnvStripsSecretsForNonLLM(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-secret")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "tok-secret")
	t.Setenv("SOME_PROVIDER_SECRET", "abc")
	t.Setenv("DSC_ADMIN_TOKEN", "admin-token") // DSC_* 必须保留（webui 代理认证用）
	t.Setenv("PLAIN_VAR", "keep")

	env := buildEnv(nil, false)
	if containsEnvKey(env, "OPENAI_API_KEY") || containsEnvKey(env, "ANTHROPIC_AUTH_TOKEN") ||
		containsEnvKey(env, "SOME_PROVIDER_SECRET") {
		t.Fatalf("non-LLM env must strip secrets, got %v", env)
	}
	if !containsEnvKey(env, "DSC_ADMIN_TOKEN") || !containsEnvKey(env, "PLAIN_VAR") {
		t.Fatalf("non-LLM env must keep DSC_* and non-secret vars, got %v", env)
	}

	envLLM := buildEnv(nil, true)
	if !containsEnvKey(envLLM, "OPENAI_API_KEY") {
		t.Fatalf("LLM env must keep API key, got %v", envLLM)
	}
}
