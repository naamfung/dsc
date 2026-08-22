package plugin

import (
	"strings"
	"testing"
)

// TestPluginEnvPassThrough 验证插件 env = 宿主环境 + 插件自定义 env
// （互通服务 ID 改经 SetInterconnect 握手传入，不经 env）。
func TestPluginEnvPassThrough(t *testing.T) {
	m := NewManager(&ManagerConfig{})
	env := m.pluginEnv(PluginEntry{Env: map[string]string{"A": "1"}})
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
