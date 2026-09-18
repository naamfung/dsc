// patch_overlay_test.go — 测试 -patch overlay 加载与合并
package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPatchOverlayTopLevel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "patch.yaml")
	content := `- name: mcp-memory
  type: dsc
  config:
    mcp:
      server_name: memory
      endpoint: http://localhost:3100/mcp
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	ov, err := LoadPatchOverlay(path)
	if err != nil {
		t.Fatalf("LoadPatchOverlay: %v", err)
	}
	if len(ov.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(ov.Entries))
	}
	if ov.Entries[0].Name != "mcp-memory" {
		t.Errorf("name = %q", ov.Entries[0].Name)
	}
	mcp, ok := ov.Entries[0].Config["mcp"].(map[string]any)
	if !ok {
		t.Fatalf("config.mcp not found or wrong type: %T", ov.Entries[0].Config["mcp"])
	}
	if mcp["server_name"] != "memory" {
		t.Errorf("server_name = %v", mcp["server_name"])
	}
}

func TestLoadPatchOverlayInsert(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "patch.yaml")
	content := `- insert:
    - name: mcp-memory
      type: dsc
      config:
        mcp:
          server_name: memory
          endpoint: http://localhost:3100/mcp
    - name: mcp-filesystem
      type: dsc
      config:
        mcp:
          server_name: fs
          endpoint: http://localhost:3101/mcp
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	ov, err := LoadPatchOverlay(path)
	if err != nil {
		t.Fatalf("LoadPatchOverlay: %v", err)
	}
	if len(ov.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(ov.Entries))
	}
	if ov.Entries[0].Name != "mcp-memory" {
		t.Errorf("entry[0].name = %q", ov.Entries[0].Name)
	}
	if ov.Entries[1].Name != "mcp-filesystem" {
		t.Errorf("entry[1].name = %q", ov.Entries[1].Name)
	}
}

func TestApplyPatchOverlaysReplaceConfig(t *testing.T) {
	base := &Config{
		Plugins: []PluginEntry{
			{Name: "mcp-memory", Type: "dsc", Enabled: true, Config: map[string]any{"old": true}},
		},
	}
	overlay := &PatchOverlay{
		Entries: []PluginEntry{
			{Name: "mcp-memory", Config: map[string]any{"mcp": map[string]any{"server_name": "memory"}}},
		},
	}
	merged := ApplyPatchOverlays(base, []*PatchOverlay{overlay})
	if len(merged.Plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(merged.Plugins))
	}
	mcp, ok := merged.Plugins[0].Config["mcp"].(map[string]any)
	if !ok {
		t.Fatalf("config.mcp not replaced: %v", merged.Plugins[0].Config)
	}
	if mcp["server_name"] != "memory" {
		t.Errorf("server_name = %v", mcp["server_name"])
	}
}

func TestApplyPatchOverlaysAppendNew(t *testing.T) {
	base := &Config{
		Plugins: []PluginEntry{
			{Name: "existing", Type: "tool", Enabled: true},
		},
	}
	overlay := &PatchOverlay{
		Entries: []PluginEntry{
			{Name: "mcp-memory", Type: "dsc", Config: map[string]any{"mcp": map[string]any{}}},
		},
	}
	merged := ApplyPatchOverlays(base, []*PatchOverlay{overlay})
	if len(merged.Plugins) != 2 {
		t.Fatalf("expected 2 plugins, got %d", len(merged.Plugins))
	}
	if merged.Plugins[1].Name != "mcp-memory" {
		t.Errorf("appended plugin name = %q", merged.Plugins[1].Name)
	}
	if !merged.Plugins[1].Enabled {
		t.Error("appended plugin should be enabled by default")
	}
}

func TestApplyPatchOverlaysMultiple(t *testing.T) {
	base := &Config{Plugins: []PluginEntry{}}
	overlays := []*PatchOverlay{
		{Entries: []PluginEntry{{Name: "a", Type: "dsc"}}},
		{Entries: []PluginEntry{{Name: "b", Type: "dsc"}}},
	}
	merged := ApplyPatchOverlays(base, overlays)
	if len(merged.Plugins) != 2 {
		t.Fatalf("expected 2 plugins, got %d", len(merged.Plugins))
	}
}

func TestLoadPatchFilesEmpty(t *testing.T) {
	overlays, err := LoadPatchFiles(nil)
	if err != nil {
		t.Fatalf("LoadPatchFiles(nil): %v", err)
	}
	if overlays != nil {
		t.Fatalf("expected nil, got %v", overlays)
	}
}
