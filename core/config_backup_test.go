package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigBackupRotationAndRestore(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfg, []byte("default_llm: openai\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// 成功启动备份 config
	p1, err := BackupGoodFile(cfg, nil)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	if !strings.Contains(filepath.Base(p1), "config.") || !strings.HasSuffix(p1, ".yaml") {
		t.Fatalf("备份路径异常: %s", p1)
	}
	// 再次备份（模拟第二次成功启动）
	p2, err := BackupGoodFile(cfg, nil)
	if err != nil {
		t.Fatalf("backup2: %v", err)
	}
	// LatestGoodBackup 应取较新那份
	latest, err := LatestGoodBackup(cfg)
	if err != nil || latest == "" {
		t.Fatalf("latest: %v", err)
	}
	if latest != p2 {
		t.Fatalf("latest 应是最新备份, got %s want %s", latest, p2)
	}
	// 还原 = 用备份覆盖源文件
	if err := os.WriteFile(cfg, []byte("broken: [[["), 0644); err != nil {
		t.Fatal(err)
	}
	if err := RestoreGoodFile(cfg, latest, nil); err != nil {
		t.Fatalf("restore: %v", err)
	}
	got, _ := os.ReadFile(cfg)
	if string(got) != "default_llm: openai\n" {
		t.Fatalf("还原内容不对: %q", got)
	}
}

func TestConfigBackupSeparatePerFile(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.yaml")
	preset := filepath.Join(dir, "presets", "standard.yaml")
	if err := os.MkdirAll(filepath.Dir(preset), 0755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(cfg, []byte("a: 1\n"), 0644)
	_ = os.WriteFile(preset, []byte("b: 2\n"), 0644)
	// config 与 preset 各自独立备份，互不覆盖、前缀区分
	pc, _ := BackupGoodFile(cfg, nil)
	pp, _ := BackupGoodFile(preset, nil)
	if filepath.Base(pc) == filepath.Base(pp) {
		t.Fatalf("config 与 preset 备份名应区分: %s vs %s", filepath.Base(pc), filepath.Base(pp))
	}
	if !strings.HasPrefix(filepath.Base(pc), "config.") || !strings.HasPrefix(filepath.Base(pp), "standard.") {
		t.Fatalf("备份名前缀应按源文件区分: %q %q", filepath.Base(pc), filepath.Base(pp))
	}
	// 各自 LatestGoodBackup 只认各自前缀
	if l, _ := LatestGoodBackup(preset); l != pp {
		t.Fatalf("preset latest 应只认 preset 备份, got %s want %s", l, pp)
	}
}

func TestBackupConfigMethod(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(&ManagerConfig{})
	cfg := filepath.Join(dir, "config.yaml")
	_ = os.WriteFile(cfg, []byte("x: 1\n"), 0644)
	m.SetConfigPath(cfg)
	_, err := m.BackupGoodConfig()
	if err != nil {
		t.Fatalf("BackupGoodConfig: %v", err)
	}
}
