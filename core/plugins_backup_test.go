package core

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPluginsBackupAndRestore(t *testing.T) {
	dir := t.TempDir()
	plugins := filepath.Join(dir, "plugins")
	snap := filepath.Join(dir, "plugins-backup")
	bin := filepath.Join(plugins, "tool-x", "tool-x.exe")
	if err := os.MkdirAll(filepath.Dir(bin), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("bin1"), 0755); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond) // 保证 mtime 差异稳定

	// 1. 快照缺失时首次创建
	done, err := BackupPluginsDir(plugins, snap, nil)
	if err != nil || !done {
		t.Fatalf("first snapshot: done=%v err=%v", done, err)
	}
	if _, err := os.Stat(filepath.Join(snap, "tool-x", "tool-x.exe")); err != nil {
		t.Fatalf("快照未含二进制: %v", err)
	}

	// 2. 无新内容时不刷新
	if done, err := BackupPluginsDir(plugins, snap, nil); err != nil || done {
		t.Fatalf("无新内容不应刷新: done=%v err=%v", done, err)
	}

	// 3. 模拟插件二进制损坏/缺失 → 回拷合并恢复
	if err := os.Remove(bin); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("corrupt"), 0644); err != nil { // 覆盖损坏
		t.Fatal(err)
	}
	if err := RestorePluginsDir(snap, plugins, nil); err != nil {
		t.Fatalf("restore: %v", err)
	}
	got, _ := os.ReadFile(bin)
	if string(got) != "bin1" {
		t.Fatalf("恢复后二进制应为 bin1, got %q", got)
	}

	// 4. 新增插件后快照更新（src 有新 mtime → 刷新）
	time.Sleep(20 * time.Millisecond)
	newBin := filepath.Join(plugins, "tool-y", "tool-y.exe")
	if err := os.MkdirAll(filepath.Dir(newBin), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newBin, []byte("bin2"), 0755); err != nil {
		t.Fatal(err)
	}
	if done, _ := BackupPluginsDir(plugins, snap, nil); !done {
		t.Fatalf("新增插件后应刷新快照")
	}
	if _, err := os.Stat(filepath.Join(snap, "tool-y", "tool-y.exe")); err != nil {
		t.Fatalf("快照应含新插件: %v", err)
	}
}

// TestRequiredPluginDirBases 校验从合并配置抽取「需要的插件目录基名」。
func TestRequiredPluginDirBases(t *testing.T) {
	merged := &Config{Plugins: []PluginEntry{
		{Name: "tool-filesystem", BinaryPath: "./plugins/tool-filesystem/tool-filesystem.exe"},
		{Name: "dsc-notify", BinaryPath: "./plugins/dsc-notify/dsc-notify.exe"},
		{Name: "tool-browser-use"}, // 空 binary_path：Manager 约定解析为 ./plugins/<name>/。<name> 本身作为目录基名保留
		{Name: ""},                 // 无 binary_path 也无 name：跳过
	}}
	keep := RequiredPluginDirBases(merged)
	for _, want := range []string{"tool-filesystem", "dsc-notify", "tool-browser-use"} {
		if !keep[want] {
			t.Fatalf("keep 应含 %s, got %v", want, keep)
		}
	}
	if len(keep) != 3 {
		t.Fatalf("应只有 3 个, got %d: %v", len(keep), keep)
	}
}

// TestReportOrphanPlugins 校验恢复时对「不在配置引用」的孤立插件目录仅告警、不删除。
func TestReportOrphanPlugins(t *testing.T) {
	dir := t.TempDir()
	plugins := filepath.Join(dir, "plugins")
	for _, d := range []string{"tool-a", "tool-stale", "dsc-old"} {
		if err := os.MkdirAll(filepath.Join(plugins, d), 0755); err != nil {
			t.Fatal(err)
		}
	}
	orphans := ReportOrphanPlugins(plugins, map[string]bool{"tool-a": true}, nil)
	// tool-stale 与 dsc-old 不在 keep → 报为孤立；tool-a 在 keep → 不报
	if len(orphans) != 2 {
		t.Fatalf("应报告 2 个孤立, got %v", orphans)
	}
	// 孤立插件仅告警、不删除：所有目录仍须保留
	for _, d := range []string{"tool-a", "tool-stale", "dsc-old"} {
		if _, err := os.Stat(filepath.Join(plugins, d)); err != nil {
			t.Fatalf("%s 应被保留（仅告警不删除）: %v", d, err)
		}
	}
}
