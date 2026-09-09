package core

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProfileWatcherStop(t *testing.T) {
	mgr := NewManager(&ManagerConfig{})
	pw, err := NewProfileWatcher(mgr, nil, mgr.logger)
	if err != nil {
		t.Fatalf("NewProfileWatcher: %v", err)
	}
	// Stop should not block or panic
	pw.Stop()
}

func TestProfileWatcherWatchFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte("default_llm: test\nplugins: []\n"), 0644)

	reloadCalled := false
	mgr := NewManager(&ManagerConfig{ExecDir: dir})
	mgr.SetConfigPath(cfgPath)

	// callback 返回 nil（不实际 reload，只验证回调被调用）
	pw, err := NewProfileWatcher(mgr, func() (*Config, error) {
		reloadCalled = true
		return nil, nil
	}, mgr.logger)
	if err != nil {
		t.Fatalf("NewProfileWatcher: %v", err)
	}

	pw.Watch(cfgPath)
	defer pw.Stop()

	// 修改文件触发事件
	time.Sleep(100 * time.Millisecond)
	os.WriteFile(cfgPath, []byte("default_llm: changed\nplugins: []\n"), 0644)

	// 等待防抖 + 延迟
	time.Sleep(3 * time.Second)

	if !reloadCalled {
		t.Error("reload callback should have been called after file change")
	}
}

func TestProfileWatcherDebounce(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte("plugins: []\n"), 0644)

	callCount := 0
	mgr := NewManager(&ManagerConfig{ExecDir: dir})
	mgr.SetConfigPath(cfgPath)

	pw, err := NewProfileWatcher(mgr, func() (*Config, error) {
		callCount++
		return nil, nil
	}, mgr.logger)
	if err != nil {
		t.Fatalf("NewProfileWatcher: %v", err)
	}

	pw.minInterval = 2 * time.Second
	pw.Watch(cfgPath)
	defer pw.Stop()

	// 快速连续修改 5 次
	time.Sleep(100 * time.Millisecond)
	for i := 0; i < 5; i++ {
		os.WriteFile(cfgPath, []byte("plugins: []\n"), 0644)
		time.Sleep(100 * time.Millisecond)
	}

	// 等待处理
	time.Sleep(4 * time.Second)

	// 因防抖，callCount 应远小于 5（理想情况为 1）
	if callCount > 2 {
		t.Errorf("debounce should limit calls to <=2, got %d", callCount)
	}
}

func TestParseConfigYAML(t *testing.T) {
	data := []byte("default_llm: test\nplugins:\n  - name: llm-test\n    type: llm\n    enabled: true\n")
	cfg, err := ParseConfigYAML(data)
	if err != nil {
		t.Fatalf("ParseConfigYAML: %v", err)
	}
	if cfg.DefaultLLM != "test" {
		t.Errorf("DefaultLLM = %q, want 'test'", cfg.DefaultLLM)
	}
	if len(cfg.Plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(cfg.Plugins))
	}
	if cfg.Plugins[0].Name != "llm-test" {
		t.Errorf("plugin name = %q, want 'llm-test'", cfg.Plugins[0].Name)
	}
}
