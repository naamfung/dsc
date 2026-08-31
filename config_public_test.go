package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dsc/core"
)

// TestPublicConfigNoNovelforge 验证公开 config.yaml 不引用内部插件 novelforge，
// 且替换后的 dsc-notify 正确声明启用：确保开源仓库不带内部插件痕迹。
func TestPublicConfigNoNovelforge(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("config", "config.yaml"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if strings.Contains(string(data), "novelforge") {
		t.Fatal("公开 config.yaml 不应包含内部插件 tool-novelforge")
	}

	cfg, err := core.LoadConfig(filepath.Join("config", "config.yaml"))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	var hasNotify, noNovelforge bool
	for i := range cfg.Plugins {
		p := &cfg.Plugins[i]
		if p.Name == "tool-novelforge" {
			t.Fatal("config 中不应有 tool-novelforge 条目")
		}
		if p.Name == "dsc-notify" && p.Enabled {
			hasNotify = true
		}
		if p.Type == "agent" && p.DependsOn != nil {
			for _, tool := range p.DependsOn.Tools {
				if tool == "tool-novelforge" {
					noNovelforge = true
				}
			}
		}
	}
	if !hasNotify {
		t.Fatal("config 应声明 dsc-notify 并启用（通用 dsc 类型，不在 agent 工具依赖中）")
	}
	if noNovelforge {
		t.Fatal("agent 依赖不应含 tool-novelforge")
	}
}
