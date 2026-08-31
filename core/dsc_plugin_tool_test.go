package core

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// TestInstallDscPluginValidation 校验命名约定：非法 type / 非法 name（含路径穿越字符）
// 在触碰任何文件/配置前即被拒绝，模型无法用坏命名安装插件。
func TestInstallDscPluginValidation(t *testing.T) {
	m := NewManager(&ManagerConfig{})
	tool := &installDscPluginTool{m: m}
	bad := []string{
		`{"type":"bogus","name":"x","source":"/tmp"}`,
		`{"type":"tool","name":"","source":"/tmp"}`,
		`{"type":"tool","name":"../evil","source":"/tmp"}`,
		`{"type":"tool","name":"a/b","source":"/tmp"}`,
	}
	for _, s := range bad {
		if _, err := tool.Execute(context.Background(), json.RawMessage(s)); err == nil {
			t.Errorf("非法参数 %s 应被拒绝", s)
		}
	}
	// 合法命名应通过参数解析（后续因 source 不存在而报错，但不再是命名错）
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"type":"tool","name":"myplug","source":"/nonexistent"}`)); err == nil {
		t.Error("source 不存在应报错")
	}
}

// TestBackupConfig 校验写 config 前的备份会生成 .bak 副本且内容一致。
func TestBackupConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("default_llm: x\nplugins:\n"), 0644); err != nil {
		t.Fatal(err)
	}
	m := NewManager(&ManagerConfig{})
	m.SetConfigPath(cfgPath)
	bak, err := m.backupConfig()
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	if !regexp.MustCompile(`config\.yaml\.\d+\.bak$`).MatchString(bak) {
		t.Fatalf("备份路径格式异常: %s", bak)
	}
	orig, _ := os.ReadFile(cfgPath)
	bakData, err := os.ReadFile(bak)
	if err != nil || string(bakData) != string(orig) {
		t.Fatalf("备份内容不一致: err=%v", err)
	}
}

// TestCopyPluginSource 校验来源拷贝：目录（含约定执行文件）成功；缺执行文件被拒。
func TestCopyPluginSource(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "plugins", "tool-probe")
	if err := os.MkdirAll(src, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "tool-probe"+binExt()), []byte("bin"), 0755); err != nil {
		t.Fatal(err)
	}

	// 缺约定执行文件的目录来源 → 拒绝
	missingSrc := filepath.Join(dir, "plugins", "tool-wrong")
	if err := os.MkdirAll(missingSrc, 0755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(missingSrc, "other.txt"), []byte("x"), 0644)
	if err := copyPluginSource(missingSrc, filepath.Join(dir, "out2"), "tool-wrong"+binExt()); err == nil {
		t.Error("缺约定执行文件的目录来源应被拒绝")
	}

	// 合法目录来源 → 成功且含执行文件
	if err := copyPluginSource(src, filepath.Join(dir, "out3"), "tool-probe"+binExt()); err != nil {
		t.Fatalf("合法目录来源应成功: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "out3", "tool-probe"+binExt())); err != nil {
		t.Fatalf("执行文件未拷贝: %v", err)
	}
}

// TestListDscPluginsView 校验 list_dsc_plugins 视图渲染为 table。
func TestListDscPluginsView(t *testing.T) {
	m := NewManager(&ManagerConfig{})
	tool := &listDscPluginsTool{m: m}
	result := `{"plugins":[{"name":"tool-filesystem","type":"tool","enabled":true},{"name":"dsc-notify","type":"dsc","enabled":true}],"count":2}`
	_, view, err := tool.ExecuteWithView(context.Background(), json.RawMessage(`{}`), result)
	if err != nil {
		t.Fatal(err)
	}
	var v ToolView
	if err := json.Unmarshal([]byte(view), &v); err != nil {
		t.Fatalf("view JSON 非法: %v", err)
	}
	if v.Kind != "table" || v.Title != "DscPlugins" || len(v.Rows) != 2 {
		t.Fatalf("view = %+v", v)
	}
}

// TestDscPluginDirBase 校验命名约定拼接。
func TestDscPluginDirBase(t *testing.T) {
	if got := dscPluginDirBase("dsc", "notify"); got != "dsc-notify" {
		t.Errorf("got %q", got)
	}
}

// TestUpgradeDscPlugin 校验升级：植入更高版本的版本化二进制、拒绝非升级/坏版本/不存在目录。
func TestUpgradeDscPlugin(t *testing.T) {
	dir := t.TempDir()
	// 构造 Manager，ExecDir 指向临时目录，pluginsRoot()=dir/plugins
	m := NewManager(&ManagerConfig{ExecDir: dir})
	pluginDir := filepath.Join(m.pluginsRoot(), "tool-probe")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "tool-probe"+binExt()), []byte("bin-v1"), 0755); err != nil {
		t.Fatal(err)
	}
	tool := &upgradeDscPluginTool{m: m}

	// 源：单二进制文件
	src := filepath.Join(dir, "tool-probe-new"+binExt())
	if err := os.WriteFile(src, []byte("bin-v2"), 0755); err != nil {
		t.Fatal(err)
	}

	// 坏版本
	if _, err := tool.upgrade("tool-probe", "not-a-version", src); err == nil {
		t.Fatal("坏版本应报错")
	}
	// 插件目录不存在
	if _, err := tool.upgrade("tool-absent", "2.0.0", src); err == nil {
		t.Fatal("不存在的插件目录应报错")
	}
	// 成功升级 → 植入 tool-probe-v2.0.0.exe，运行版本提升
	res, err := tool.upgrade("tool-probe", "2.0.0", src)
	if err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	if !res["ok"].(bool) {
		t.Fatalf("应升级成功 %v", res)
	}
	if _, err := os.Stat(filepath.Join(pluginDir, "tool-probe-v2.0.0"+binExt())); err != nil {
		t.Fatalf("应植入版本化二进制: %v", err)
	}
	// 重复同版本 → 拒绝
	if _, err := tool.upgrade("tool-probe", "2.0.0", src); err == nil {
		t.Fatal("重复同版本应报错")
	}
	// 降级/平级 → 拒绝
	if _, err := tool.upgrade("tool-probe", "1.0.0", src); err == nil {
		t.Fatal("低于当前运行版本的应报错")
	}
}
