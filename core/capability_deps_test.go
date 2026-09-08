package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dsc/proto/metadata"
)

// TestParseRequiresCapability 解析 requires/<type>/<cap> 编码键。
func TestParseRequiresCapability(t *testing.T) {
	cases := []struct {
		key      string
		wantType string
		wantCap  string
		wantOK   bool
	}{
		{"requires/llm/supports_images", "llm", "supports_images", true},
		{"requires/tool/cron", "tool", "cron", true},
		{"requires/llm/", "", "", false},        // 空 cap
		{"requires//foo", "", "", false},        // 空 type
		{"requires/llm/foo/bar", "", "", false}, // cap 含斜杠
		{"supports_images", "", "", false},      // 普通能力键（无 requires/ 前缀）
		{"", "", "", false},
		{"requires", "", "", false},     // 仅前缀
		{"requires/llm", "", "", false}, // 无 /
	}
	for _, c := range cases {
		gotType, gotCap, gotOK := parseRequiresCapability(c.key)
		if gotType != c.wantType || gotCap != c.wantCap || gotOK != c.wantOK {
			t.Errorf("parseRequiresCapability(%q) = (%q, %q, %v), want (%q, %q, %v)",
				c.key, gotType, gotCap, gotOK, c.wantType, c.wantCap, c.wantOK)
		}
	}
}

// TestDecodeRequires 从 PluginInfo.Capabilities 提取声明式依赖列表。
func TestDecodeRequires(t *testing.T) {
	info := &metadata.PluginInfo{
		Capabilities: map[string]string{
			"supports_images":                    "true", // 业务能力（普通键，无前缀）
			"requires/llm/supports_images":       "true",
			"requires/tool/cron":                 "true",
			"requires/tool/agentic-bench-runner": "true",
		},
	}
	reqs := DecodeRequires(info)
	if len(reqs) != 3 {
		t.Fatalf("DecodeRequires got %d requires, want 3: %+v", len(reqs), reqs)
	}
	// 解析后稳定排序：按 Type 升序、再按 Capability 升序
	// llm/supports_images < tool/agentic-bench-runner < tool/cron
	if reqs[0].Type != "llm" || reqs[0].Capability != "supports_images" {
		t.Errorf("DecodeRequires[0] = %+v, want llm/supports_images", reqs[0])
	}
	if reqs[1].Type != "tool" || reqs[1].Capability != "agentic-bench-runner" {
		t.Errorf("DecodeRequires[1] = %+v, want tool/agentic-bench-runner", reqs[1])
	}
	if reqs[2].Type != "tool" || reqs[2].Capability != "cron" {
		t.Errorf("DecodeRequires[2] = %+v, want tool/cron", reqs[2])
	}
}

// TestDecodeRequiresEmpty nil 或无 requires 前缀键时返回 nil。
func TestDecodeRequiresEmpty(t *testing.T) {
	if got := DecodeRequires(nil); got != nil {
		t.Errorf("DecodeRequires(nil) = %+v, want nil", got)
	}
	if got := DecodeRequires(&metadata.PluginInfo{}); got != nil {
		t.Errorf("DecodeRequires(empty caps) = %+v, want nil", got)
	}
	if got := DecodeRequires(&metadata.PluginInfo{
		Capabilities: map[string]string{"supports_images": "true"},
	}); got != nil {
		t.Errorf("DecodeRequires(only business caps) = %+v, want nil", got)
	}
}

// TestResolveRequiredDepsAutoFillLLM 声明式依赖：插件 A 声明 requires/llm/supports_images，
// 已加载 LLM 插件 llm-p 的 capabilities 含 supports_images=true —— 解析结果应含
// ResolvedDep{Type:"llm", Capability:"supports_images", ProviderName:"llm-p"}。
func TestResolveRequiredDepsAutoFillLLM(t *testing.T) {
	m := NewManager(&ManagerConfig{})
	m.mu.Lock()
	m.coreMetadata["llm-p"] = &metadata.PluginInfo{
		Type:         "llm",
		Name:         "llm-p",
		Capabilities: map[string]string{"supports_images": "true"},
	}
	m.mu.Unlock()

	info := &metadata.PluginInfo{
		Type:         "agent",
		Name:         "agent-x",
		Capabilities: map[string]string{"requires/llm/supports_images": "true"},
	}
	m.mu.Lock()
	deps := m.resolveRequiredDeps(info, "agent-x")
	m.mu.Unlock()

	if len(deps) != 1 {
		t.Fatalf("应解析出 1 条依赖，got %+v", deps)
	}
	if deps[0].Type != "llm" || deps[0].Capability != "supports_images" || deps[0].ProviderName != "llm-p" {
		t.Errorf("依赖项异常: %+v, want llm/supports_images→llm-p", deps[0])
	}
}

// TestResolveRequiredDepsNoProvider 插件声明 requires 但无对应能力 provider 时，
// 解析结果中对应项的 ProviderName 为空串（尚未解析到 provider）。
func TestResolveRequiredDepsNoProvider(t *testing.T) {
	m := NewManager(&ManagerConfig{})
	m.mu.Lock()
	m.coreMetadata["tool-foo"] = &metadata.PluginInfo{
		Type:         "tool",
		Name:         "tool-foo",
		Capabilities: map[string]string{"unrelated": "true"},
	}
	m.mu.Unlock()

	info := &metadata.PluginInfo{
		Type:         "tool",
		Name:         "tool-bench",
		Capabilities: map[string]string{"requires/tool/cron": "true"},
	}
	m.mu.Lock()
	deps := m.resolveRequiredDeps(info, "tool-bench")
	m.mu.Unlock()

	if len(deps) != 1 {
		t.Fatalf("应解析出 1 条依赖，got %+v", deps)
	}
	if deps[0].ProviderName != "" {
		t.Errorf("无匹配 provider 时 ProviderName 应为空串，got %q", deps[0].ProviderName)
	}
}

// TestResolveRequiredDepsExcludesSelf 避免自引用：插件自己声明了 capability X 又 requires
// capability X 时，不应把自己作为依赖来源。
func TestResolveRequiredDepsExcludesSelf(t *testing.T) {
	m := NewManager(&ManagerConfig{})
	m.mu.Lock()
	m.coreMetadata["tool-bench"] = &metadata.PluginInfo{
		Type:         "tool",
		Name:         "tool-bench",
		Capabilities: map[string]string{"cron": "true"}, // 自身声明了 cron 能力
	}
	m.mu.Unlock()

	info := &metadata.PluginInfo{
		Type: "tool",
		Name: "tool-bench",
		Capabilities: map[string]string{
			"cron":               "true", // 自身业务能力
			"requires/tool/cron": "true", // 但本插件又声明依赖 cron —— 不应自引用
		},
	}
	m.mu.Lock()
	deps := m.resolveRequiredDeps(info, "tool-bench")
	m.mu.Unlock()

	if len(deps) != 1 {
		t.Fatalf("应解析出 1 条依赖，got %+v", deps)
	}
	if deps[0].ProviderName != "" {
		t.Errorf("自引用不应被解析为依赖：got ProviderName=%q", deps[0].ProviderName)
	}
}

// TestResolveRequiredDepsMultipleToolCapabilities 多个 requires/tool/<cap> 各自
// 匹配到不同的插件，解析结果应含多条 ResolvedDep 且按 (Type, Capability) 升序稳定排序。
func TestResolveRequiredDepsMultipleToolCapabilities(t *testing.T) {
	m := NewManager(&ManagerConfig{})
	m.mu.Lock()
	m.coreMetadata["tool-cron"] = &metadata.PluginInfo{
		Type: "tool", Name: "tool-cron",
		Capabilities: map[string]string{"cron": "true"},
	}
	m.coreMetadata["tool-fs"] = &metadata.PluginInfo{
		Type: "tool", Name: "tool-fs",
		Capabilities: map[string]string{"filesystem": "true"},
	}
	m.mu.Unlock()

	info := &metadata.PluginInfo{
		Type: "agent", Name: "agent-x",
		Capabilities: map[string]string{
			"requires/tool/cron":       "true",
			"requires/tool/filesystem": "true",
		},
	}
	m.mu.Lock()
	deps := m.resolveRequiredDeps(info, "agent-x")
	m.mu.Unlock()

	if len(deps) != 2 {
		t.Fatalf("应解析出 2 条依赖，got %+v", deps)
	}
	// 按 Capability 升序：cron < filesystem
	if deps[0].Capability != "cron" || deps[0].ProviderName != "tool-cron" {
		t.Errorf("deps[0] 异常: %+v, want cron→tool-cron", deps[0])
	}
	if deps[1].Capability != "filesystem" || deps[1].ProviderName != "tool-fs" {
		t.Errorf("deps[1] 异常: %+v, want filesystem→tool-fs", deps[1])
	}
}

// TestResolveRequiredDepsCapabilityValueFalse capability 值为 "false" 的插件
// 不应被识别为该能力的提供者（与 LLM supports_images: "false" 同义：能力未启用）。
func TestResolveRequiredDepsCapabilityValueFalse(t *testing.T) {
	m := NewManager(&ManagerConfig{})
	m.mu.Lock()
	m.coreMetadata["llm-off"] = &metadata.PluginInfo{
		Type:         "llm",
		Name:         "llm-off",
		Capabilities: map[string]string{"supports_images": "false"}, // 显式关
	}
	m.mu.Unlock()

	info := &metadata.PluginInfo{
		Type:         "agent",
		Name:         "agent-x",
		Capabilities: map[string]string{"requires/llm/supports_images": "true"},
	}
	m.mu.Lock()
	deps := m.resolveRequiredDeps(info, "agent-x")
	m.mu.Unlock()

	if len(deps) != 1 {
		t.Fatalf("应解析出 1 条依赖，got %+v", deps)
	}
	if deps[0].ProviderName != "" {
		t.Errorf("capability=false 的 provider 不应被匹配：got ProviderName=%q", deps[0].ProviderName)
	}
}

// TestDepsSatisfiedLocked 校验 depsSatisfiedLocked：
// 所有依赖项的 ProviderName 非空且对应 provider 已加载时返回 true；否则 false。
func TestDepsSatisfiedLocked(t *testing.T) {
	m := NewManager(&ManagerConfig{})
	m.mu.Lock()
	m.llmServiceIDs["llm-p"] = 41
	m.toolServiceIDs["tool-p"] = 42
	m.mu.Unlock()

	cases := []struct {
		name string
		deps []ResolvedDep
		want bool
	}{
		{"all-satisfied", []ResolvedDep{
			{Type: "llm", Capability: "supports_images", ProviderName: "llm-p"},
			{Type: "tool", Capability: "cron", ProviderName: "tool-p"},
		}, true},
		{"empty-provider", []ResolvedDep{
			{Type: "llm", Capability: "supports_images", ProviderName: ""}, // 未解析到 provider
		}, false},
		{"provider-not-loaded", []ResolvedDep{
			{Type: "llm", Capability: "supports_images", ProviderName: "llm-ghost"}, // 未加载
		}, false},
		{"no-deps", nil, true},
	}
	for _, c := range cases {
		m.mu.Lock()
		got := m.depsSatisfiedLocked(c.deps)
		m.mu.Unlock()
		if got != c.want {
			t.Errorf("%s: depsSatisfiedLocked = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestPickPrimaryLLM 校验 pickPrimaryLLM：从 ResolvedDep 列表中选首个 Type=="llm"
// 且 ProviderName 非空的项，多条 llm 依赖时按 Capability 名升序取首个（稳定选择）。
func TestPickPrimaryLLM(t *testing.T) {
	cases := []struct {
		name string
		deps []ResolvedDep
		want string
	}{
		{"single-llm", []ResolvedDep{
			{Type: "llm", Capability: "supports_images", ProviderName: "llm-p"},
		}, "llm-p"},
		{"multi-llm-pick-by-capability", []ResolvedDep{
			{Type: "llm", Capability: "supports_vision", ProviderName: "llm-vision"},
			{Type: "llm", Capability: "supports_images", ProviderName: "llm-p"}, // supports_images < supports_vision
		}, "llm-p"},
		{"no-llm", []ResolvedDep{
			{Type: "tool", Capability: "cron", ProviderName: "tool-cron"},
		}, ""},
		{"empty-provider", []ResolvedDep{
			{Type: "llm", Capability: "supports_images", ProviderName: ""}, // 未解析到
		}, ""},
		{"empty-deps", nil, ""},
	}
	for _, c := range cases {
		if got := pickPrimaryLLM(c.deps); got != c.want {
			t.Errorf("%s: pickPrimaryLLM = %q, want %q", c.name, got, c.want)
		}
	}
}

// TestAutoResolveDoesNotPersistWhenPersistFalse 校验能力依赖自动解析在 persist=false
// 时**绝不写回 config.yaml**——孤儿插件即此种状态（既未启用、又未配置，但已在 plugins/
// 目录内），经 load_dsc_plugin 默认 persist=false 载入后，其自动解析得到的 ResolvedDep
// 仅本进程内存生效，不应污染 config.yaml。
//
// 同时校验：persist=false 时解析结果仍**可在运行时态中使用**——经 m.GetResolvedDeps
// 查询得到（与「不得将依赖项写入配置」契约并存，互不冲突）。
func TestAutoResolveDoesNotPersistWhenPersistFalse(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	original := "plugins: []\n"
	if err := os.WriteFile(cfgPath, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	m := NewManager(&ManagerConfig{ExecDir: dir})
	m.SetConfigPath(cfgPath)

	// 模拟「已加载的 tool-cron 插件」声明了 cron 能力
	m.mu.Lock()
	m.coreMetadata["tool-cron"] = &metadata.PluginInfo{
		Type:         "tool",
		Name:         "tool-cron",
		Capabilities: map[string]string{"cron": "true"},
	}
	m.mu.Unlock()

	// 模拟「新加载的 tool-bench 插件」声明 requires/tool/cron
	entry := PluginEntry{Name: "tool-bench", Type: "tool"}
	m.coreMetadata["tool-bench"] = &metadata.PluginInfo{
		Type:         "tool",
		Name:         "tool-bench",
		Capabilities: map[string]string{"requires/tool/cron": "true"},
	}

	// 调用 persist=false 的路径（孤儿插件经 load_dsc_plugin 默认载入）
	m.mu.Lock()
	m.autoResolveAndPersistDepsLocked(entry, false)
	m.mu.Unlock()

	// 校验 config.yaml 未被改动
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if string(data) != original {
		t.Errorf("persist=false 时 config.yaml 不应被修改：\n原:%q\n新:%q", original, string(data))
	}
	s := string(data)
	if strings.Contains(s, "tool-bench") || strings.Contains(s, "tool-cron") || strings.Contains(s, "depends_on") {
		t.Errorf("persist=false 时 config 不应含插件条目或 depends_on 字段：\n%s", s)
	}

	// **关键契约**：persist=false 时解析结果仍应在运行时态可用——经 GetResolvedDeps 查询。
	got := m.GetResolvedDeps("tool-bench")
	if len(got) != 1 {
		t.Fatalf("persist=false 时 GetResolvedDeps 应返回 1 条依赖，got %+v", got)
	}
	if got[0].Type != "tool" || got[0].Capability != "cron" || got[0].ProviderName != "tool-cron" {
		t.Errorf("GetResolvedDeps(tool-bench) = %+v, want tool/cron→tool-cron", got[0])
	}
}

// TestAutoResolvePersistsWhenPersistTrue 对照组：persist=true 时 autoResolveAndPersistDepsLocked
// 应把 entry 写回 config.yaml——这是 install_dsc_plugin / load_dsc_plugin persist=true 的预期路径。
// 注意：能力依赖解析结果存于 m.resolvedDeps（运行时态），config.yaml 只记插件条目本身
// （name/type/binary_path/enabled/env），不再有 depends_on 字段——能力依赖由插件二进制内
// 的 sdk.Config.Requires 自描述，无需落盘。
func TestAutoResolvePersistsWhenPersistTrue(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("plugins: []\n"), 0644); err != nil {
		t.Fatal(err)
	}
	m := NewManager(&ManagerConfig{ExecDir: dir})
	m.SetConfigPath(cfgPath)

	// 模拟已加载的 tool-cron 声明 cron 能力
	m.mu.Lock()
	m.coreMetadata["tool-cron"] = &metadata.PluginInfo{
		Type: "tool", Name: "tool-cron",
		Capabilities: map[string]string{"cron": "true"},
	}
	m.mu.Unlock()

	// 模拟新加载的 tool-bench 声明 requires/tool/cron + binary_path
	entry := PluginEntry{
		Name: "tool-bench", Type: "tool",
		BinaryPath: "./plugins/tool-bench/tool-bench",
	}
	m.coreMetadata["tool-bench"] = &metadata.PluginInfo{
		Type: "tool", Name: "tool-bench",
		Capabilities: map[string]string{"requires/tool/cron": "true"},
	}

	m.mu.Lock()
	m.autoResolveAndPersistDepsLocked(entry, true)
	m.mu.Unlock()

	// 校验 config.yaml 被修改：应含 tool-bench 条目（但不落盘 depends_on——能力依赖由
	// 插件二进制自描述）
	data, _ := os.ReadFile(cfgPath)
	s := string(data)
	if !strings.Contains(s, "tool-bench") {
		t.Errorf("persist=true 时 config 应含 tool-bench 条目：\n%s", s)
	}
	if strings.Contains(s, "depends_on") {
		t.Errorf("persist=true 时 config 不应含 depends_on 字段（能力依赖不落盘）：\n%s", s)
	}
}

// TestCapabilityLLMResolvedFromAgentRequires 校验端到端能力解析：
// agent 声明 Requires: [{Type:"llm", Capability:"llm"}]，
// LLM 插件的 PluginInfo.Capabilities 含 "llm": "true"（由 llmMetadataServer 默认提供），
// resolveRequiredDeps 应解析到 ProviderName = "llm-p"，pickPrimaryLLM 应返回 "llm-p"。
func TestCapabilityLLMResolvedFromAgentRequires(t *testing.T) {
	m := NewManager(&ManagerConfig{})
	m.mu.Lock()
	m.coreMetadata["llm-p"] = &metadata.PluginInfo{
		Type:         "llm",
		Name:         "llm-p",
		Capabilities: map[string]string{CapabilityLLM: "true"},
	}
	m.mu.Unlock()

	agentInfo := &metadata.PluginInfo{
		Type:         "agent",
		Name:         "agent-x",
		Capabilities: map[string]string{"requires/llm/llm": "true"},
	}
	m.mu.Lock()
	deps := m.resolveRequiredDeps(agentInfo, "agent-x")
	m.mu.Unlock()

	if len(deps) != 1 {
		t.Fatalf("应解析出 1 条依赖，got %+v", deps)
	}
	if deps[0].Type != "llm" || deps[0].Capability != "llm" || deps[0].ProviderName != "llm-p" {
		t.Errorf("依赖项异常: %+v, want llm/llm→llm-p", deps[0])
	}
	if primary := pickPrimaryLLM(deps); primary != "llm-p" {
		t.Errorf("pickPrimaryLLM = %q, want llm-p", primary)
	}
}
