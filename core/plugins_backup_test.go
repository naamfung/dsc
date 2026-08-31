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
