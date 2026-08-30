package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dsc/core"
)

// scannerFromInputs 将多行输入拼成一个 bufio.Scanner，供驱动 runSetup 交互。
func scannerFromInputs(inputs []string) *bufio.Scanner {
	return bufio.NewScanner(strings.NewReader(strings.Join(inputs, "\n") + "\n"))
}

// 本文件覆盖 dsc setup 向导的纯逻辑：LLM 提供商动态发现（基于 config 状态 +
// 目录扫描）与 env 键名语义学习。交互部分（bufio 循环）不在此测。

// writeTestConfig 写一份含 LLM/agent 条目的 config.yaml，返回路径。
func writeTestConfig(t *testing.T, dir, body string) string {
	t.Helper()
	p := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(p, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDiscoverLLMProvidersFromConfigState(t *testing.T) {
	dir := t.TempDir()
	// config 声明 anthropic（enabled）+ openai（disabled）+ agent 依赖
	cfg := writeTestConfig(t, dir, `
default_llm: llm-anthropic
plugins:
  - name: llm-anthropic
    type: llm
    enabled: true
    env:
      ANTHROPIC_BASE_URL: "http://localhost:8000"
      ANTHROPIC_MODEL: "model-a"
  - name: llm-openai
    type: llm
    enabled: false
    env:
      OPENAI_BASE_URL: "http://localhost:9000/v1"
  - name: agent-react-loop
    type: agent
    enabled: true
    depends_on:
      llm: llm-anthropic
`)
	// plugins 目录含一个 config 未声明的 llm-ollama 目录
	pluginsDir := filepath.Join(dir, "plugins")
	if err := os.MkdirAll(filepath.Join(pluginsDir, "llm-ollama"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(pluginsDir, "tool-filesystem"), 0755); err != nil {
		t.Fatal(err)
	}

	providers := discoverLLMProviders(cfg, pluginsDir)
	if len(providers) != 3 {
		t.Fatalf("应发现 3 个 LLM 提供商（config 2 + 目录 1），got %d: %+v", len(providers), providers)
	}
	byName := map[string]setupProvider{}
	for _, p := range providers {
		byName[p.name] = p
	}
	// config 条目带 enabled 状态
	if p := byName["llm-anthropic"]; !p.enabled {
		t.Fatal("llm-anthropic 应 enabled=true")
	}
	if p := byName["llm-openai"]; p.enabled {
		t.Fatal("llm-openai 应 enabled=false")
	}
	// 目录补充的未声明插件默认未启用
	if p := byName["llm-ollama"]; p.enabled {
		t.Fatal("llm-ollama 未在 config 声明，应 enabled=false")
	}
	// env 键名语义学习：anthropic 沿用现键
	if p := byName["llm-anthropic"]; p.baseKey != "ANTHROPIC_BASE_URL" || p.modelKey != "ANTHROPIC_MODEL" || p.apiKey != "ANTHROPIC_API_KEY" {
		t.Fatalf("anthropic 键学习错误: %+v", p)
	}
	// 目录新插件按前缀推导（ollama 有 HOST 语义键时也应识别为 base）
	if p := byName["llm-ollama"]; p.baseKey != "OLLAMA_BASE_URL" {
		t.Fatalf("ollama base 键应按前缀推导 OLLAMA_BASE_URL, got %q", p.baseKey)
	}
	// 非 llm 目录不进入
	if _, ok := byName["tool-filesystem"]; ok {
		t.Fatal("tool 目录不应被当 LLM 提供商")
	}
}

func TestEnvKeyForProviderSemanticLearning(t *testing.T) {
	// 已有 OLLAMA_HOST 时 base 键应沿用 HOST，而非推导 BASE_URL
	base, model, key := envKeyForProvider("llm-ollama", map[string]string{
		"OLLAMA_HOST":  "http://localhost:11434",
		"OLLAMA_MODEL": "qwen",
	})
	if base != "OLLAMA_HOST" || model != "OLLAMA_MODEL" || key != "OLLAMA_API_KEY" {
		t.Fatalf("ollama 语义学习: base=%q model=%q key=%q", base, model, key)
	}
	// 无现有配置时按前缀推导（llm-foo → FOO_*）
	base, model, key = envKeyForProvider("llm-foo", nil)
	if base != "FOO_BASE_URL" || model != "FOO_MODEL" || key != "FOO_API_KEY" {
		t.Fatalf("前缀推导: base=%q model=%q key=%q", base, model, key)
	}
	// API key 键名语义：已有 *_TOKEN 应识别
	_, _, key = envKeyForProvider("llm-bar", map[string]string{"BAR_TOKEN": "x"})
	if key != "BAR_TOKEN" {
		t.Fatalf("TOKEN 键识别: key=%q", key)
	}
}

func TestUpsertPluginEnvPreservesComments(t *testing.T) {
	dir := t.TempDir()
	cfg := writeTestConfig(t, dir, `
# 顶部注释
default_llm: llm-anthropic
plugins:
  - name: llm-anthropic
    type: llm
    enabled: true
    env:
      ANTHROPIC_BASE_URL: "http://old:8000"
`)
	changed, err := core.UpsertPluginEnv(cfg, "llm-anthropic", "llm", map[string]string{
		"ANTHROPIC_BASE_URL": "http://new:8000",
		"ANTHROPIC_MODEL":    "model-b",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("应发生变更")
	}
	body, _ := os.ReadFile(cfg)
	s := string(body)
	if !strings.Contains(s, "顶部注释") {
		t.Fatal("注释应保留")
	}
	if !strings.Contains(s, "http://new:8000") || !strings.Contains(s, "model-b") {
		t.Fatalf("新值未写入: %s", s)
	}
}

// TestRunSetupInteractiveEditAndSave 驱动 runSetup 完整交互：编辑 llm-anthropic
// 的基址 → 设默认 → 保存 → 校验 config.yaml 写回且保留注释。输入序列按菜单索引：
// 主菜单[3=llm-anthropic] → 基址/模型/key 三字段 → 主菜单[1=设默认] → 选择默认
// [1=llm-anthropic] → 主菜单[保存] → 确认保存[y]。
func TestRunSetupInteractiveEditAndSave(t *testing.T) {
	dir := t.TempDir()
	cfg := writeTestConfig(t, dir, `
# 顶部注释
default_llm: llm-openai
plugins:
  - name: llm-anthropic
    type: llm
    enabled: true
    env:
      ANTHROPIC_BASE_URL: "http://old:8000"
      ANTHROPIC_MODEL: "old-model"
  - name: llm-openai
    type: llm
    enabled: true
    env:
      OPENAI_BASE_URL: "http://openai:8000/v1"
`)
	pluginsDir := filepath.Join(dir, "plugins")
	if err := os.MkdirAll(filepath.Join(pluginsDir, "llm-anthropic"), 0755); err != nil {
		t.Fatal(err)
	}

	// 输入序列（主菜单: 1=配置, 2=设默认, 3=providers[0](llm-anthropic), 4=providers[1],
	// 5=保存, 6=取消）：
	inputs := []string{
		"3",               // 主菜单 → 直接编辑 llm-anthropic（providers[0]）
		"http://new:9000", // 基址
		"new-model",       // 模型
		"new-key",         // API key
		"2",               // 主菜单 → 设默认
		"1",               // 选择 llm-anthropic 为默认
		"5",               // 主菜单 → 保存并退出
		"y",               // 确认保存
	}
	// 注意：llm-anthropic 已 enabled=true，编辑时不会询问"启用该提供商"。
	var out strings.Builder
	rc := runSetup(scannerFromInputs(inputs), &out, cfg, pluginsDir)
	if rc != 0 {
		t.Fatalf("runSetup 返回 %d\n%s", rc, out.String())
	}
	body, _ := os.ReadFile(cfg)
	s := string(body)
	if !strings.Contains(s, "http://new:9000") || !strings.Contains(s, "new-model") || !strings.Contains(s, "new-key") {
		t.Fatalf("编辑值未写回: %s\n交互输出:\n%s", s, out.String())
	}
	if !strings.Contains(s, "default_llm: llm-anthropic") {
		t.Fatalf("default_llm 未更新: %s", s)
	}
	if !strings.Contains(s, "顶部注释") {
		t.Fatal("注释应保留")
	}
}

// TestRunSetupNewProviderEnable 验证 config 未声明（目录扫描发现）的提供商经
// 向导编辑并启用后，会新增到 config.yaml 且 enabled=true。
func TestRunSetupNewProviderEnable(t *testing.T) {
	dir := t.TempDir()
	cfg := writeTestConfig(t, dir, `
# 顶部注释
plugins:
  - name: llm-openai
    type: llm
    enabled: true
    env:
      OPENAI_BASE_URL: "http://openai:8000/v1"
`)
	pluginsDir := filepath.Join(dir, "plugins")
	if err := os.MkdirAll(filepath.Join(pluginsDir, "llm-anthropic"), 0755); err != nil {
		t.Fatal(err)
	}
	// 主菜单: 1=配置, 2=设默认, 3=providers[0](llm-anthropic), 4=providers[1](llm-openai),
	// 5=保存, 6=取消
	inputs := []string{
		"3",                     // 编辑 llm-anthropic（providers[0]）
		"http://anthropic:8000", // 基址
		"model-x",               // 模型
		"key-x",                 // key
		"y",                     // 启用该提供商
		"2",                     // 设默认
		"1",                     // 默认=llm-anthropic
		"5",                     // 保存
		"y",                     // 确认
	}
	rc := runSetup(scannerFromInputs(inputs), &strings.Builder{}, cfg, pluginsDir)
	if rc != 0 {
		t.Fatalf("runSetup 返回 %d", rc)
	}
	body, _ := os.ReadFile(cfg)
	s := string(body)
	if !strings.Contains(s, "llm-anthropic") || !strings.Contains(s, "http://anthropic:8000") {
		t.Fatalf("新提供商未写回: %s", s)
	}
	if !strings.Contains(s, "enabled: true") {
		t.Fatalf("新提供商应 enabled: %s", s)
	}
	if !strings.Contains(s, "顶部注释") {
		t.Fatal("注释应保留")
	}
}
