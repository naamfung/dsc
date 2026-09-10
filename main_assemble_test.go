package main

import (
	"testing"

	core "dsc/core"
)

// TestAssembleMergedConfigToolsDedup 验证 config.yaml 中启用的 tool/dsc 条目
// 并入合并集（模型安装的插件可跨重启生效），并与 preset 按名去重——同名冲突取 preset。
func TestAssembleMergedConfigToolsDedup(t *testing.T) {
	mainCfg := &core.Config{Plugins: []core.PluginEntry{
		{Name: "agent-react-loop", Type: "agent", Enabled: true, BinaryPath: "./plugins/agent-react-loop/a.exe"},
		{Name: "llm-openai", Type: "llm", Enabled: true, BinaryPath: "./plugins/llm-openai/l.exe"},
		// config.yaml 里安装的（preset 没有的）工具——install_dsc_plugin 写入的位置
		{Name: "tool-installed", Type: "tool", Enabled: true, BinaryPath: "./plugins/tool-installed/t.exe"},
		// config.yaml 里与 preset 同名的（duplicate）——preset 优先，保留 preset 的 binary_path
		{Name: "tool-filesystem", Type: "tool", Enabled: true, BinaryPath: "./plugins/config/tool-filesystem/cf.exe"},
		// config.yaml 里对 preset 同名插件做 env 补充——不生效，preset 条目主导
		{Name: "tool-lua-host", Type: "tool", Enabled: true, BinaryPath: "./plugins/config/tool-lua-host/cf.exe"},
	}}
	presetCfg := &core.Config{Plugins: []core.PluginEntry{
		{Name: "tool-filesystem", Type: "tool", Enabled: true, BinaryPath: "./plugins/tool-filesystem/pf.exe"},
		{Name: "tool-lua-host", Type: "tool", Enabled: true, BinaryPath: "./plugins/tool-lua-host/pl.exe"},
	}}
	merged := assembleMerged(extractLLMs(mainCfg), extractAgent(mainCfg), mainCfg, presetCfg, 128000, false, "")

	names := map[string]bool{}
	binary := map[string]string{}
	for _, p := range merged.Plugins {
		names[p.Name] = true
		binary[p.Name] = p.BinaryPath
	}
	// config-only 安装的工具在
	if !names["tool-installed"] {
		t.Fatalf("config.yaml 安装的工具未被并入, got %v", keySet(names))
	}
	// 与 preset 同名的只保留一份（去重）
	if !names["tool-filesystem"] || !names["tool-lua-host"] {
		t.Fatalf("应保留 tool-filesystem/tool-lua-host")
	}
	if n := countName(merged, "tool-filesystem"); n != 1 {
		t.Fatalf("tool-filesystem 应去重为 1 份, got %d", n)
	}
	// 同名冲突：preset 优先
	if binary["tool-filesystem"] != "./plugins/tool-filesystem/pf.exe" {
		t.Fatalf("tool-filesystem 应取 preset 的 binary_path, got %q", binary["tool-filesystem"])
	}
	if binary["tool-lua-host"] != "./plugins/tool-lua-host/pl.exe" {
		t.Fatalf("tool-lua-host 应取 preset 的 binary_path, got %q", binary["tool-lua-host"])
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
