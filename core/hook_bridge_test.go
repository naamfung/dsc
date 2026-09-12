package core

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestHookBridgeLoadConfig(t *testing.T) {
	dir := t.TempDir()
	// 创建一个可执行的 hook 脚本
	hookPath := filepath.Join(dir, "hook.sh")
	os.WriteFile(hookPath, []byte("#!/bin/sh\ncat > /dev/null\necho '{\"veto\":false}'"), 0755)

	configPath := filepath.Join(dir, "hooks.json")
	// 用 json.Marshal 构造配置：自动转义路径中的反斜杆（Windows 路径含 \U 等，
	// 直接字符串拼接会把非法转义塞进 JSON，导致解析失败）
	cfg := HookConfig{
		BeforeTool: []HookEntry{{Matcher: "shell", Command: hookPath}},
		AfterTool:  []HookEntry{{Matcher: "*", Command: hookPath}},
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	hb := NewHookBridge()
	if err := hb.LoadConfig(configPath); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
}

func TestHookBridgeNoConfig(t *testing.T) {
	hb := NewHookBridge()
	// 无配置文件 → 不报错，不拦截
	args, err := hb.RunBeforeTool(context.Background(), "shell", `{"command":"ls"}`)
	if err != nil {
		t.Fatalf("RunBeforeTool: %v", err)
	}
	if args != `{"command":"ls"}` {
		t.Errorf("args should be unchanged, got %q", args)
	}
}

func TestHookBridgeMatchPattern(t *testing.T) {
	cases := []struct {
		matcher, toolName string
		want              bool
	}{
		{"*", "anything", true},
		{"", "anything", true},
		{"shell", "shell", true},
		{"shell", "read_file", false},
		{"shell*", "shell", true},
		{"shell*", "shell_exec", true},
		{"shell*", "read_file", false},
		{"tool*", "tool-anything", true},
	}
	for _, c := range cases {
		if got := matchHookTool(c.matcher, c.toolName); got != c.want {
			t.Errorf("matchHookTool(%q, %q) = %v, want %v", c.matcher, c.toolName, got, c.want)
		}
	}
}

func TestHookBridgeBeforeToolVeto(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("executes a /bin/sh shebang script; not supported on Windows")
	}
	dir := t.TempDir()
	// 创建一个 veto 钩子
	hookPath := filepath.Join(dir, "veto.sh")
	os.WriteFile(hookPath, []byte("#!/bin/sh\ncat > /dev/null\necho '{\"veto\":true,\"output\":\"blocked by hook\"}'"), 0755)

	configPath := filepath.Join(dir, "hooks.json")
	cfg := HookConfig{
		BeforeTool: []HookEntry{{Matcher: "shell", Command: hookPath}},
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	hb := NewHookBridge()
	if err := hb.LoadConfig(configPath); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	_, err = hb.RunBeforeTool(context.Background(), "shell", `{"command":"ls"}`)
	if err == nil {
		t.Error("should be vetoed")
	}
}

func TestHookBridgeAfterToolModify(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("executes a /bin/sh shebang script; not supported on Windows")
	}
	dir := t.TempDir()
	// 创建一个修改结果的钩子
	hookPath := filepath.Join(dir, "modify.sh")
	os.WriteFile(hookPath, []byte("#!/bin/sh\ncat > /dev/null\necho '{\"veto\":false,\"output\":\"modified result\"}'"), 0755)

	configPath := filepath.Join(dir, "hooks.json")
	cfg := HookConfig{
		AfterTool: []HookEntry{{Matcher: "*", Command: hookPath}},
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	hb := NewHookBridge()
	if err := hb.LoadConfig(configPath); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	result, toolErr := hb.RunAfterTool(context.Background(), "shell", `{"command":"ls"}`, "original result", "")
	if result != "modified result" {
		t.Errorf("result = %q, want 'modified result'", result)
	}
	if toolErr != "" {
		t.Errorf("toolErr should be empty, got %q", toolErr)
	}
}
