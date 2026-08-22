package plugin

import (
	"strings"
	"testing"

	"dsc/plugin/llmclient"
	"dsc/plugin/notify"
)

// TestPluginEnvInjectsLLMServiceID 验证互通注入：聚合 LLM 服务 ID 经 env
// 注入插件进程，且不污染插件自定义 env。
func TestPluginEnvInjectsLLMServiceID(t *testing.T) {
	m := NewManager(&ManagerConfig{})

	// 未就绪时注入 env 不含 DSC_LLM_SERVICE_ID，但保留自定义项
	env := m.pluginEnv(PluginEntry{Env: map[string]string{"A": "1"}})
	if !containsEnv(env, "A", "1") {
		t.Fatalf("custom env should pass through, got %v", env)
	}
	if containsEnvKey(env, "DSC_LLM_SERVICE_ID") {
		t.Fatalf("no LLM service should not inject, got %v", env)
	}

	// 就绪后注入 ID，且原 entry.Env 不被污染
	m.agentLLMServiceID = 42
	m.pluginNotifyServiceID = 7
	entryEnv := map[string]string{"A": "1"}
	env = m.pluginEnv(PluginEntry{Env: entryEnv})
	if !containsEnv(env, llmclient.EnvServiceID, "42") {
		t.Fatalf("should inject LLM service id, got %v", env)
	}
	if !containsEnv(env, notify.EnvServiceID, "7") {
		t.Fatalf("should inject notify service id, got %v", env)
	}
	if _, ok := entryEnv[llmclient.EnvServiceID]; ok {
		t.Fatal("pluginEnv must not mutate the entry env map")
	}
	if _, ok := entryEnv[notify.EnvServiceID]; ok {
		t.Fatal("pluginEnv must not mutate the entry env map")
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
