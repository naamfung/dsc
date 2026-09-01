package core

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"dsc/proto"
	plugin "github.com/hashicorp/go-plugin"
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

// TestLoadDscPluginValidation 校验 load_dsc_plugin / unload_dsc_plugin：
//  1. 工具已入 dscPluginTools 目录（模型可见）；
//  2. schema 为合法 JSON；
//  3. 坏命名/坏类型/目录缺失/越界 binary_path 均被拒绝（不触碰插件进程）。
func TestLoadDscPluginValidation(t *testing.T) {
	m := NewManager(&ManagerConfig{})
	var names []string
	for _, td := range m.dscPluginTools() {
		names = append(names, td.Name())
	}
	for _, want := range []string{"load_dsc_plugin", "unload_dsc_plugin"} {
		found := false
		for _, n := range names {
			if n == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s 未入 dscPluginTools: %v", want, names)
		}
	}

	loadTool := &loadDscPluginTool{m: m}
	if err := json.Unmarshal(loadTool.ParametersSchema(), &map[string]any{}); err != nil {
		t.Fatalf("load schema 非法 JSON: %v", err)
	}
	unloadTool := &unloadDscPluginTool{m: m}
	if err := json.Unmarshal(unloadTool.ParametersSchema(), &map[string]any{}); err != nil {
		t.Fatalf("unload schema 非法 JSON: %v", err)
	}

	bad := []string{
		`{"name":""}`,
		`{"name":"../evil"}`,
		`{"name":"a/b"}`,
		`{"name":"x","type":"bogus"}`,
		`{"name":"tool-absent"}`,                       // 目录不存在
		`{"name":"x","binary_path":"C:\\evil\\x.exe"}`, // 越界 pluginsRoot
	}
	for _, s := range bad {
		if _, err := loadTool.Execute(context.Background(), json.RawMessage(s)); err == nil {
			t.Errorf("load 非法参数应被拒绝: %s", s)
		}
	}
	// unload：非法名应被拒绝
	for _, s := range []string{`{"name":""}`, `{"name":"../evil"}`, `{"name":"a/b"}`} {
		if _, err := unloadTool.Execute(context.Background(), json.RawMessage(s)); err == nil {
			t.Errorf("unload 非法参数应被拒绝: %s", s)
		}
	}
}

// TestLoadUnloadDscPluginE2E 端到端验证 load_dsc_plugin / unload_dsc_plugin
// 的「可选落盘」对称行为（对齐 AGENTS.md 的真实插件进程宿主链集成测试要求）：
//  1. 真实插件进程（tool-lisp-eval）经 load_dsc_plugin 载入，其工具经宿主聚合
//     ToolGRPCServer 可执行（lisp_eval 真实求值）；
//  2. persist=false：仅当前进程生效，config.yaml 不变；
//  3. persist=true：条目写回 config.yaml（备份生成），重启即保留；
//  4. unload_dsc_plugin(persist=false)：卸载进程、config 保持既有条目；
//  5. unload_dsc_plugin(persist=true)：从 config.yaml 移除条目，进程已停止。
func TestLoadUnloadDscPluginE2E(t *testing.T) {
	tmpDir := t.TempDir()
	repoRoot := filepath.Join("..") // core 包目录之上即仓库根，plugins 直接在其下

	// 真实 agent 提供 broker（生产等价：注入 tool 需 m.broker 非空）
	agentExe := filepath.Join(tmpDir, "agent-react-loop.exe")
	buildAgentBin(t, filepath.Join(repoRoot, "plugins", "agent-react-loop"), agentExe)

	// 真实 tool 插件放到约定目录 plugins/tool-lisp-eval/
	toolDir := filepath.Join(tmpDir, "plugins", "tool-lisp-eval")
	exe := filepath.Join(toolDir, "tool-lisp-eval"+binExt())
	buildToolBin(t, filepath.Join(repoRoot, "plugins", "tool-lisp-eval"), exe)

	// 配置：仅默认 LLM，startPlugins 允许 mock；plugins 空
	cfgPath := filepath.Join(tmpDir, "config", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0755); err != nil {
		t.Fatal(err)
	}
	cfg := "default_llm: mock-llm\nplugins: []\n"
	if err := os.WriteFile(cfgPath, []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}

	m := NewManager(&ManagerConfig{ExecDir: tmpDir})
	m.SetConfigPath(cfgPath)
	loadTestAgent(t, m, "agent-react-loop", agentExe, tmpDir)
	t.Cleanup(func() { m.Shutdown() })

	load := &loadDscPluginTool{m: m}
	unload := &unloadDscPluginTool{m: m}
	agg := NewToolGRPCServer(m)

	isLoaded := func() bool {
		m.mu.RLock()
		defer m.mu.RUnlock()
		_, ok := m.clients["tool-lisp-eval"]
		return ok
	}
	cfgHasEntry := func() bool {
		b, err := os.ReadFile(cfgPath)
		if err != nil {
			t.Fatalf("read config: %v", err)
		}
		return strings.Contains(string(b), "tool-lisp-eval")
	}
	execTool := func() {
		expr := `(+ 41 1)`
		resp, err := agg.ExecuteTool(context.Background(), &proto.ExecuteToolRequest{
			ToolName: "lisp_eval", ArgumentsJson: `{"expression":"` + expr + `"}`,
		})
		if err != nil {
			t.Fatalf("ExecuteTool(lisp_eval): %v", err)
		}
		if resp.Error != "" || !strings.Contains(resp.Content, "42") {
			t.Fatalf("lisp_eval 结果异常: err=%q content=%q", resp.Error, resp.Content)
		}
	}

	// ---- 1. load persist=false：进程载入、工具可用、config 不变 ----
	res, err := load.Execute(context.Background(), json.RawMessage(`{"name":"tool-lisp-eval","persist":false}`))
	if err != nil {
		t.Fatalf("load(false): %v", err)
	}
	if !strings.Contains(res, `"status":"loaded"`) || !isLoaded() {
		t.Fatalf("load(false) 未生效: %s", res)
	}
	execTool()
	if cfgHasEntry() {
		t.Fatalf("persist=false 不应写 config")
	}

	// ---- 2. unload persist=false：进程停止、config 仍无条目 ----
	res, err = unload.Execute(context.Background(), json.RawMessage(`{"name":"tool-lisp-eval","persist":false}`))
	if err != nil {
		t.Fatalf("unload(false): %v", err)
	}
	if isLoaded() {
		t.Fatalf("unload(false) 后仍加载: %s", res)
	}
	if cfgHasEntry() {
		t.Fatalf("unload(false) 不应写 config")
	}

	// ---- 3. load persist=true：进程载入、条目写回 config（含备份） ----
	if _, err := load.Execute(context.Background(), json.RawMessage(`{"name":"tool-lisp-eval","persist":true}`)); err != nil {
		t.Fatalf("load(true): %v", err)
	}
	if !isLoaded() {
		t.Fatalf("load(true) 未载入进程")
	}
	execTool()
	if !cfgHasEntry() {
		t.Fatalf("load(true) 未写回 config")
	}
	if g, err := filepath.Glob(cfgPath + ".*.bak"); err != nil || len(g) == 0 {
		t.Fatalf("load(true) 应生成 config 备份: %v %v", g, err)
	}
	// 幂等：已加载再次 load 不报错
	if _, err := load.Execute(context.Background(), json.RawMessage(`{"name":"tool-lisp-eval","persist":true}`)); err != nil {
		t.Fatalf("load 幂等(已加载再 load)失败: %v", err)
	}

	// ---- 4. unload persist=true：进程停止、条目从 config 移除、改动前备份 ----
	bakBefore := 0
	if g, _ := filepath.Glob(cfgPath + ".*.bak"); g != nil {
		bakBefore = len(g)
	}
	res, err = unload.Execute(context.Background(), json.RawMessage(`{"name":"tool-lisp-eval","persist":true}`))
	if err != nil {
		t.Fatalf("unload(true): %v", err)
	}
	if isLoaded() {
		t.Fatalf("unload(true) 后仍加载: %s", res)
	}
	if cfgHasEntry() {
		t.Fatalf("unload(true) 未从 config 移除条目")
	}
	if g, _ := filepath.Glob(cfgPath + ".*.bak"); len(g) <= bakBefore {
		t.Fatalf("unload(true) 应在改动前备份 config（before=%d after=%d）", bakBefore, len(g))
	}
	// 幂等：未加载再次 unload 不报错
	if _, err := unload.Execute(context.Background(), json.RawMessage(`{"name":"tool-lisp-eval","persist":true}`)); err != nil {
		t.Fatalf("unload 幂等(未加载再 unload)失败: %v", err)
	}
}

// TestListDscPluginsThreeStates 校验 list_dsc_plugins 合并三方来源并按状态打标：
//   - loaded：config 声明 + 运行期已加载；
//   - configured：config 声明但未加载（enabled=false）；
//   - orphan：磁盘存在（含约定可执行文件）但未在 config 声明。
//
// 同时校验非约定命名目录不被当作孤儿。
func TestListDscPluginsThreeStates(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0755); err != nil {
		t.Fatal(err)
	}
	cfg := "plugins:\n  - name: tool-a\n    type: tool\n    enabled: true\n  - name: tool-b\n    type: tool\n    enabled: false\n"
	if err := os.WriteFile(cfgPath, []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}

	m := NewManager(&ManagerConfig{ExecDir: dir})
	m.SetConfigPath(cfgPath)

	// 磁盘：tool-a/tool-b 约定可执行文件齐备；tool-c 仅磁盘（孤儿）；bad-dir 名字不合法
	ext := binExt()
	for _, name := range []string{"tool-a", "tool-b", "tool-c"} {
		pd := filepath.Join(dir, "plugins", name)
		if err := os.MkdirAll(pd, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(pd, name+ext), []byte("bin"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "plugins", "bad dir!"), 0755); err != nil {
		t.Fatal(err)
	}

	// 运行态：tool-a 已加载
	m.mu.Lock()
	m.clients["tool-a"] = &plugin.Client{}
	m.typeMap["tool-a"] = "tool"
	m.mu.Unlock()

	tool := &listDscPluginsTool{m: m}
	res, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var out struct {
		Plugins []struct {
			Name  string `json:"name"`
			State string `json:"state"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal([]byte(res), &out); err != nil {
		t.Fatalf("json: %v", err)
	}
	state := map[string]string{}
	for _, p := range out.Plugins {
		state[p.Name] = p.State
	}
	want := map[string]string{"tool-a": "loaded", "tool-b": "configured", "tool-c": "orphan"}
	for name, s := range want {
		if state[name] != s {
			t.Errorf("%s state = %q, want %q (all=%v)", name, state[name], s, state)
		}
	}
	// 非约定命名目录不应进入候选
	if len(out.Plugins) != 3 {
		t.Errorf("len = %d, want 3 (bad-dir 不应计入): %v", len(out.Plugins), out.Plugins)
	}
}

// TestPluginBinaryNameStrict 校验严格命名规则对可执行文件名称的判定：
// 主名须与目录名一致（可带 -v<semver> 版本后缀），类型前缀不可省略。
func TestPluginBinaryNameStrict(t *testing.T) {
	cases := []struct {
		dir, stem string
		want      bool
	}{
		{"tool-novelforge", "tool-novelforge", true},               // 主名=目录名
		{"tool-novelforge", "tool-novelforge-v2", true},            // 版本后缀
		{"tool-novelforge", "tool-novelforge-v2.3.1", true},        // 版本后缀
		{"tool-novelforge", "novelforge", false},                   // 省略类型前缀 → 不合规
		{"tool-novelforge", "novelforge-v2.3.1", false},            // 省略类型前缀+版本 → 不合规
		{"tool-musicplayer", "dsc-plugin-tool-musicplayer", false}, // 封装前缀残留 → 不合规
		{"tool-novelforge", "novelforge.exe", false},               // 整名含扩展名 → 不合规
	}
	for _, c := range cases {
		if got := pluginBinaryStemValid(c.dir, c.stem); got != c.want {
			t.Errorf("pluginBinaryStemValid(%q, %q) = %v, want %v", c.dir, c.stem, got, c.want)
		}
	}
}

// TestDiskOrphanRequiresCompliantBinary 校验孤儿/未配候选须在目录内存在合规执行文件，
// 而非仅凭目录名或任意 exe——省略类型前缀的不合规文件不把该目录计入。
func TestDiskOrphanRequiresCompliantBinary(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "plugins", "tool-novelforge")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatal(err)
	}
	ext := binExt()
	m := NewManager(&ManagerConfig{ExecDir: dir})

	// 仅有省略前缀的 novelforge.exe（不合规残留）→ 目录不是孤儿候选
	if err := os.WriteFile(filepath.Join(pluginDir, "novelforge"+ext), []byte("bin"), 0755); err != nil {
		t.Fatal(err)
	}
	if got := m.listPluginDiskNames(); len(got) != 0 {
		t.Fatalf("仅残留不合规文件时 listPluginDiskNames = %v, want 空", got)
	}

	// 补上合规的 tool-novelforge.exe（与目录名一致）→ 目录成为孤儿候选
	if err := os.WriteFile(filepath.Join(pluginDir, "tool-novelforge"+ext), []byte("bin"), 0755); err != nil {
		t.Fatal(err)
	}
	got := m.listPluginDiskNames()
	if len(got) != 1 || got[0] != "tool-novelforge" {
		t.Fatalf("有合规文件时 listPluginDiskNames = %v, want [tool-novelforge]", got)
	}
	// binary_path 须归一化为正斜杠（模型/宿主 SHELL 是 POSIX，反斜杠会误导）
	bin := m.diskPluginBinary("tool-novelforge")
	if strings.ContainsRune(bin, '\\') {
		t.Errorf("diskPluginBinary = %q, 不应含反斜杠（应已 ToSlash 为正斜杠）", bin)
	}
	wantDir := filepath.ToSlash(filepath.Join(dir, "plugins", "tool-novelforge", "tool-novelforge"+ext))
	if bin != wantDir {
		t.Errorf("diskPluginBinary = %q, want %q", bin, wantDir)
	}
}
