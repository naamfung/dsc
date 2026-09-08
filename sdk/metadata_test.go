package dsc

import (
	"context"
	"testing"

	"dsc/proto/metadata"
)

// TestMetadataServerEncodesRequires 校验 SDK metadataServer 把 Config.Requires
// 编码为 capabilities map 中的 requires/<type>/<cap> 键（值 "true"）。
func TestMetadataServerEncodesRequires(t *testing.T) {
	srv := &metadataServer{cfg: Config{
		Name:    "tool-bench",
		Version: "1.0.0",
		Type:    TypeTool,
		Requires: []CapabilityRequirement{
			{Type: "llm", Capability: "supports_images"},
			{Type: "tool", Capability: "cron"},
		},
	}}
	info, err := srv.GetInfo(context.Background(), &metadata.Empty{})
	if err != nil {
		t.Fatalf("GetInfo: %v", err)
	}
	if info.Name != "tool-bench" || info.Type != "tool" {
		t.Fatalf("metadata mismatch: %+v", info)
	}
	if got := info.Capabilities["requires/llm/supports_images"]; got != "true" {
		t.Errorf("requires/llm/supports_images 应为 \"true\"，got %q", got)
	}
	if got := info.Capabilities["requires/tool/cron"]; got != "true" {
		t.Errorf("requires/tool/cron 应为 \"true\"，got %q", got)
	}
}

// TestMetadataServerNoRequires 无 Requires 时不编码任何 requires/ 前缀键。
func TestMetadataServerNoRequires(t *testing.T) {
	srv := &metadataServer{cfg: Config{
		Name: "tool-foo", Version: "1.0.0", Type: TypeTool,
	}}
	info, _ := srv.GetInfo(context.Background(), &metadata.Empty{})
	for k := range info.Capabilities {
		if len(k) >= len(requiresCapabilityKeyPrefix) && k[:len(requiresCapabilityKeyPrefix)] == requiresCapabilityKeyPrefix {
			t.Errorf("无 Requires 时不应编码 requires/ 键，got %q", k)
		}
	}
	if len(info.Capabilities) != 0 {
		t.Errorf("无 Requires 时 capabilities 应为空 map，got %+v", info.Capabilities)
	}
}

// TestMetadataServerSkipsInvalidRequires 类型或能力为空的声明应被跳过。
func TestMetadataServerSkipsInvalidRequires(t *testing.T) {
	srv := &metadataServer{cfg: Config{
		Name: "tool-foo", Type: TypeTool,
		Requires: []CapabilityRequirement{
			{Type: "", Capability: "x"},         // 空 type
			{Type: "llm", Capability: ""},       // 空 cap
			{Type: "tool", Capability: "valid"}, // 有效
		},
	}}
	info, _ := srv.GetInfo(context.Background(), &metadata.Empty{})
	if _, ok := info.Capabilities["requires//x"]; ok {
		t.Error("空 type 不应编码")
	}
	if _, ok := info.Capabilities["requires/llm/"]; ok {
		t.Error("空 cap 不应编码")
	}
	if v, ok := info.Capabilities["requires/tool/valid"]; !ok || v != "true" {
		t.Errorf("有效项应被编码：requires/tool/valid=true，got %q", v)
	}
	if len(info.Capabilities) != 1 {
		t.Errorf("应仅 1 个有效 requires 键，got %+v", info.Capabilities)
	}
}

// TestEncodeDecodeRequiresRoundTrip EncodeRequires 与 SDK 内 parseRequiresCapability
// 应可往返。
func TestEncodeDecodeRequiresRoundTrip(t *testing.T) {
	cfg := Config{
		Name: "tool-bench", Type: TypeTool,
		Requires: []CapabilityRequirement{
			{Type: "llm", Capability: "supports_images"},
			{Type: "tool", Capability: "cron"},
			{Type: "tool", Capability: "filesystem"},
		},
	}
	encoded := EncodeRequires(cfg)
	if len(encoded) != 3 {
		t.Fatalf("EncodeRequires got %d entries, want 3: %+v", len(encoded), encoded)
	}
	// 把 encoded 当作 PluginInfo.Capabilities 模拟宿主侧解析
	// 宿主侧的 DecodeRequires 在 core 包内；本测试用 SDK 内的 parseRequiresCapability 验证
	for k := range encoded {
		if _, _, ok := parseRequiresCapability(k); !ok {
			t.Errorf("SDK parseRequiresCapability 不能解析自身编码的键 %q", k)
		}
	}
}

// TestLLMSdkMetadataServerFusesCapabilitiesAndRequires llmSdkMetadataServer 应同时
// 暴露 LLM 业务能力 supports_images 与 Config.Requires 编码。
func TestLLMSdkMetadataServerFusesCapabilitiesAndRequires(t *testing.T) {
	// 模拟一个 LLM 插件声明 requires/tool/cron + vision=false
	// 注意：llmSdkMetadataServer 需要 core.LLMProvider（实现 VisionEnabled）。
	// 这里用 nil impl 跳过 supports_images 路径（实际生产 impl 非 nil）。
	s := &SDK{cfg: Config{
		Name:    "llm-x",
		Version: "1.0.0",
		Type:    TypeLLM,
		Requires: []CapabilityRequirement{
			{Type: "tool", Capability: "cron"},
		},
	}}
	srv := &llmSdkMetadataServer{sdk: s, impl: nil}
	info, err := srv.GetInfo(context.Background(), &metadata.Empty{})
	if err != nil {
		t.Fatalf("GetInfo: %v", err)
	}
	if info.Type != "llm" {
		t.Errorf("Type = %q, want llm", info.Type)
	}
	if info.Name != "llm-x" {
		t.Errorf("Name = %q, want llm-x", info.Name)
	}
	if got, ok := info.Capabilities["requires/tool/cron"]; !ok || got != "true" {
		t.Errorf("requires/tool/cron 应编码为 \"true\"，got %q ok=%v", got, ok)
	}
	// nil impl 时 supports_images 不应被设
	if _, ok := info.Capabilities["supports_images"]; ok {
		t.Error("nil impl 时不应设置 supports_images")
	}
}

// TestMetadataServerEncodesProvides 校验 SDK metadataServer 把 Config.Provides
// 编码为 capabilities map 中的普通能力键（不带 requires/ 前缀）。
func TestMetadataServerEncodesProvides(t *testing.T) {
	srv := &metadataServer{cfg: Config{
		Name:    "tool-filesystem",
		Version: "1.0.0",
		Type:    TypeTool,
		Provides: map[string]string{
			"filesystem": "true",
		},
		Requires: []CapabilityRequirement{
			{Type: "llm", Capability: "supports_images"},
		},
	}}
	info, err := srv.GetInfo(context.Background(), &metadata.Empty{})
	if err != nil {
		t.Fatalf("GetInfo: %v", err)
	}
	// Provides 的 "filesystem" 应作为普通能力键出现
	if got := info.Capabilities["filesystem"]; got != "true" {
		t.Errorf("Provides filesystem 应为 \"true\"，got %q", got)
	}
	// Requires 的 "requires/llm/supports_images" 应同时编码
	if got := info.Capabilities["requires/llm/supports_images"]; got != "true" {
		t.Errorf("requires/llm/supports_images 应为 \"true\"，got %q", got)
	}
}

// TestMetadataServerProvidesRejectsRequiresPrefix 校验 Provides 中误用 requires/ 前缀
// 的键被跳过（不与 Requires 编码冲突）。
func TestMetadataServerProvidesRejectsRequiresPrefix(t *testing.T) {
	srv := &metadataServer{cfg: Config{
		Name:    "tool-x",
		Version: "1.0.0",
		Type:    TypeTool,
		Provides: map[string]string{
			"valid-cap":        "true",
			"requires/llm/bad": "true", // 误用前缀，应被跳过
			"":                 "true", // 空键，应被跳过
		},
	}}
	info, _ := srv.GetInfo(context.Background(), &metadata.Empty{})
	if info.Capabilities["valid-cap"] != "true" {
		t.Errorf("valid-cap 应被编码为 \"true\"，got %q", info.Capabilities["valid-cap"])
	}
	if _, ok := info.Capabilities["requires/llm/bad"]; ok {
		t.Error("误用 requires/ 前缀的 Provides 键应被跳过")
	}
	if _, ok := info.Capabilities[""]; ok {
		t.Error("空键应被跳过")
	}
}

// TestAgentMetadataServerEncodesRequires 校验 SDK agentMetadataServer 把
// Config.Requires 与 Config.Provides 正确编码为 PluginInfo.Capabilities。
func TestAgentMetadataServerEncodesRequires(t *testing.T) {
	s := &SDK{cfg: Config{
		Name:    "agent-react-loop",
		Version: "1.1.0",
		Type:    TypeAgent,
		Requires: []CapabilityRequirement{
			{Type: "llm", Capability: "llm"},
		},
	}}
	srv := &agentMetadataServer{sdk: s}
	info, err := srv.GetInfo(context.Background(), &metadata.Empty{})
	if err != nil {
		t.Fatalf("GetInfo: %v", err)
	}
	if info.Type != "agent" {
		t.Errorf("Type = %q, want agent", info.Type)
	}
	if info.Name != "agent-react-loop" {
		t.Errorf("Name = %q, want agent-react-loop", info.Name)
	}
	if got := info.Capabilities["requires/llm/llm"]; got != "true" {
		t.Errorf("requires/llm/llm 应为 \"true\"，got %q", got)
	}
	if info.ApiVersion != "1.0" {
		t.Errorf("ApiVersion = %q, want 1.0", info.ApiVersion)
	}
}
