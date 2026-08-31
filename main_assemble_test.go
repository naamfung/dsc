package main

import (
	"testing"

	core "dsc/core"
)

// TestAssembleMergedConfigToolsDedup 验证 P1 修复：config.yaml 中启用的 tool/dsc 条目
// 并入合并集（模型安装的插件可跨重启生效），并与 preset 按名去重（config 优先）。
func TestAssembleMergedConfigToolsDedup(t *testing.T) {
	mainCfg := &core.Config{Plugins: []core.PluginEntry{
		{Name: "agent-react-loop", Type: "agent", Enabled: true, BinaryPath: "./plugins/agent-react-loop/a.exe"},
		{Name: "llm-openai", Type: "llm", Enabled: true, BinaryPath: "./plugins/llm-openai/l.exe"},
		// config.yaml 里安装的（preset 没有的）工具——install_go_plugin 写入的位置
		{Name: "tool-installed", Type: "tool", Enabled: true, BinaryPath: "./plugins/tool-installed/t.exe"},
		// config.yaml 里与 preset 同名的（duplicate）——只保留一份
		{Name: "tool-filesystem", Type: "tool", Enabled: true, BinaryPath: "./plugins/tool-filesystem/f.exe"},
	}}
	presetCfg := &core.Config{Plugins: []core.PluginEntry{
		{Name: "tool-filesystem", Type: "tool", Enabled: true, BinaryPath: "./plugins/tool-filesystem/f.exe"},
		{Name: "tool-lua-host", Type: "tool", Enabled: true, BinaryPath: "./plugins/tool-lua-host/l.exe"},
	}}
	merged := assembleMerged(extractLLMs(mainCfg), extractAgent(mainCfg), mainCfg, presetCfg, 128000, false, "")

	names := map[string]bool{}
	for _, p := range merged.Plugins {
		names[p.Name] = true
	}
	// config-only 安装的工具在
	if !names["tool-installed"] {
		t.Fatalf("config.yaml 安装的工具未被并入, got %v", keySet(names))
	}
	// 与 preset 同名的只保留一份（去重）
	if !names["tool-filesystem"] {
		t.Fatalf("tool-filesystem 应在")
	}
	if n := countName(merged, "tool-filesystem"); n != 1 {
		t.Fatalf("tool-filesystem 应去重为 1 份, got %d", n)
	}
	if !names["tool-lua-host"] {
		t.Fatalf("preset 的 tool-lua-host 应在")
	}
}

func extractLLMs(cfg *core.Config) []core.PluginEntry {
	var out []core.PluginEntry
	for _, e := range cfg.Plugins {
		if e.Enabled && e.Type == "llm" {
			out = append(out, e)
		}
	}
	return out
}

func extractAgent(cfg *core.Config) *core.PluginEntry {
	for i := range cfg.Plugins {
		if cfg.Plugins[i].Enabled && cfg.Plugins[i].Type == "agent" {
			return &cfg.Plugins[i]
		}
	}
	return nil
}

func countName(merged *core.Config, name string) int {
	n := 0
	for _, p := range merged.Plugins {
		if p.Name == name {
			n++
		}
	}
	return n
}

func keySet(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}
