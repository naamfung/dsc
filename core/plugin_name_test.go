package core

import "testing"

// TestValidatePluginDirectoryNameDscGeneric 通用（dsc）类型不设前缀门槛：
// 任意合规名称皆可（dsc-* 仅为惯例）；专用类型仍强制 <type>- 前缀；
// 空名与非法字符集一律拒绝。
func TestValidatePluginDirectoryNameDscGeneric(t *testing.T) {
	// dsc：任意合规名称（含带 dsc- 前缀的惯例名与不带前缀的通用名）
	for _, name := range []string{"dsc-system", "my-plugin", "compaction_core", "Tool01", "a"} {
		if err := validatePluginDirectoryName("dsc", name); err != nil {
			t.Fatalf("dsc 类型应接受任意合规名称 %q: %v", name, err)
		}
	}
	// dsc：空名与非法字符集拒绝
	for _, name := range []string{"", "has space", "斜杠/name", "dot.name"} {
		if err := validatePluginDirectoryName("dsc", name); err == nil {
			t.Fatalf("dsc 类型应拒绝非法名称 %q", name)
		}
	}
	// 专用类型：前缀门槛不变
	if err := validatePluginDirectoryName("tool", "my-tool"); err == nil {
		t.Fatalf("tool 类型仍应强制 tool- 前缀")
	}
	if err := validatePluginDirectoryName("tool", "tool-fs"); err != nil {
		t.Fatalf("tool 类型 tool- 前缀应通过: %v", err)
	}
	if err := validatePluginDirectoryName("llm", "llm-openai"); err != nil {
		t.Fatalf("llm 类型应通过: %v", err)
	}
	if err := validatePluginDirectoryName("agent", "agent-react-loop"); err != nil {
		t.Fatalf("agent 类型应通过: %v", err)
	}
	// 未知类型报错
	if err := validatePluginDirectoryName("unknown", "x"); err == nil {
		t.Fatalf("未知类型应报错")
	}
}

// TestInferPluginTypeFromDirFallback 孤儿目录的类型推断：已知前缀照常推断，
// 无法识别前缀时兜底为通用（dsc）类型（「不确定前缀时可用 dsc-*」）。
func TestInferPluginTypeFromDirFallback(t *testing.T) {
	cases := map[string]string{
		"tool-fs":      "tool",
		"llm-openai":   "llm",
		"agent-loop":   "agent",
		"policy-spill": "policy",
		"dsc-system":   "dsc",
		"my-thing":     "dsc", // 未识别前缀 → 通用兜底
		"plainname":    "dsc",
	}
	for dir, want := range cases {
		if got := inferPluginTypeFromDir(dir); got != want {
			t.Fatalf("inferPluginTypeFromDir(%q) = %q, want %q", dir, got, want)
		}
	}
}
