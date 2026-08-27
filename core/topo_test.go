package core

import (
	"reflect"
	"testing"
)

// TestTopoSortPluginsOrder 校验依赖先于依赖者（稳定排序，保持声明顺序）。
func TestTopoSortPluginsOrder(t *testing.T) {
	entries := []PluginEntry{
		{Name: "tool-b", Type: "tool", Enabled: true, DependsOn: &PluginDepends{LLM: "llm-a"}},
		{Name: "llm-a", Type: "llm", Enabled: true},
		{Name: "tool-c", Type: "tool", Enabled: true, DependsOn: &PluginDepends{LLM: "llm-a", Tools: []string{"tool-b"}}},
	}
	declared := map[string]bool{"llm-a": true, "tool-b": true, "tool-c": true}

	sorted, pending := topoSortPlugins(entries, declared)
	if len(pending) != 0 {
		t.Fatalf("expected no pending, got %v", pending)
	}
	got := make([]string, 0, len(sorted))
	for _, e := range sorted {
		got = append(got, e.Name)
	}
	// llm-a 必须先于 tool-b，tool-b 必须先于 tool-c
	want := []string{"llm-a", "tool-b", "tool-c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("order mismatch: got %v, want %v", got, want)
	}
}

// TestTopoSortPluginsPending 校验：依赖指向「已声明但不在本批 provider 之列」的插件（如单独的 agent）
// 时，该条目无法即时排定，进入 PENDING（等待其依赖就绪）。
func TestTopoSortPluginsPending(t *testing.T) {
	entries := []PluginEntry{
		{Name: "llm-a", Type: "llm", Enabled: true},
		// tool-b 依赖 agent（agent 已声明但排除在 provider 批外，另有单独加载流程）
		{Name: "tool-b", Type: "tool", Enabled: true, DependsOn: &PluginDepends{Tools: []string{"agent-react-loop"}}},
	}
	// declared 含全部启用插件（含 agent）；provider 批不含 agent
	declared := map[string]bool{"llm-a": true, "tool-b": true, "agent-react-loop": true}

	sorted, pending := topoSortPlugins(entries, declared)
	if len(pending) != 1 || pending[0].Name != "tool-b" {
		t.Fatalf("expected tool-b pending, got sorted=%v pending=%v", sorted, pending)
	}
	if len(sorted) != 1 || sorted[0].Name != "llm-a" {
		t.Fatalf("expected only llm-a sorted, got %v", sorted)
	}
}

// TestTopoSortPluginsToolNameNotPending 校验 DependsOn 指向「工具名」（非插件名）不触发 PENDING——
// 这类引用由运行时解析，不参与拓扑排序。
func TestTopoSortPluginsToolNameNotPending(t *testing.T) {
	entries := []PluginEntry{
		{Name: "tool-b", Type: "tool", Enabled: true, DependsOn: &PluginDepends{LLM: "llm-a", Tools: []string{"read_file"}}},
	}
	declared := map[string]bool{"tool-b": true}

	sorted, pending := topoSortPlugins(entries, declared)
	if len(pending) != 0 {
		t.Fatalf("expected no pending for runtime-resolved tool name, got %v", pending)
	}
	// 依赖顶级 llm-a 未在本批声明（由 agent/外部提供），不算缺依赖 → 仍可排序
	if len(sorted) != 1 || sorted[0].Name != "tool-b" {
		t.Fatalf("expected tool-b sorted, got %v", sorted)
	}
}
