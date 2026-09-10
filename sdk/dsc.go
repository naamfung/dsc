package dsc

import (
	"context"
	"strings"

	"dsc/core"
	"dsc/proto"
	"dsc/proto/metadata"
	plugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

// dscGRPCPlugin 是通用（dsc）类型插件的 go-core 适配器：注册插件元数据、Hook
// 服务，以及（可选的）ToolServiceServer。
//
// 对齐 DSH/Cordis 的「插件类型与服务正交」模型：TypeDsc 是「通用」类型，可
// 同时声明 hook + tools + 自定义能力（Provides）。宿主加载它时登记 hook
// client，使其能接收宿主事件广播；若插件还经 sdk.Tool 注册了工具，宿主探测
// 到非空工具集后会把它同时登记为 tool provider（model 可见的工具生效）。
//
// 始终注册 ToolServiceServer（即便工具集为空）：让宿主可以无差错地调用
// ListTools 探测，empty 列表时宿主跳过 tool 登记。这与 TypeTool 行为一致，
// 但宿主侧的 case "dsc" 路径不会把 typeMap 改写为 "tool"（保留 dsc 身份）。
type dscGRPCPlugin struct {
	plugin.NetRPCUnsupportedPlugin
	sdk *SDK
}

func (p *dscGRPCPlugin) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	metadata.RegisterPluginMetadataServer(s, &metadataServer{cfg: p.sdk.cfg})
	proto.RegisterPluginHookServiceServer(s, &hookServiceServer{hook: p.sdk.hook})
	// 始终注册 ToolServiceServer：让宿主经 ListTools 探测插件是否暴露工具。
	// 工具集为空时 ListTools 返回空列表，宿主据此跳过 tool 登记——零行为变化
	// （如 dsc-notify 无工具时与历史行为一致）。非空时宿主登记为 tool provider。
	// 对齐 DSH/Cordis：插件类型与服务正交，TypeDsc 亦可暴露模型可见工具。
	proto.RegisterToolServiceServer(s, &toolServiceServer{sdk: p.sdk, broker: broker})
	return nil
}

func (p *dscGRPCPlugin) GRPCClient(context.Context, *plugin.GRPCBroker, *grpc.ClientConn) (interface{}, error) {
	return nil, nil
}

// llmGRPCPlugin 是 llm 类型插件的 go-core 适配器：注册 LLMService、SDK 自己的
// metadataServer（融合 supports_images 业务能力 + Config.Requires 声明式依赖编码），
// 并额外注册 PluginHookService，使 LLM 插件也能声明 Hook 订阅宿主事件
// （对齐 DSH cordis：事件广播类型无关）。
//
// 注意：不复用 core.LLMGRPCPlugin.GRPCServer，因为它内部还会注册一个
// llmMetadataServer，与本 SDK 的 metadataServer 同服务名会引发 panic。
// 直接注册 LLMServiceServer + SDK metadataServer + HookService 即可。
type llmGRPCPlugin struct {
	core.LLMGRPCPlugin
	sdk *SDK
}

func (p *llmGRPCPlugin) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	// 直接注册 LLMService 服务端（Impl 由 LLMProvider 实现提供）。
	llmSrv := core.NewLLMServiceServer(p.LLMGRPCPlugin.Impl)
	proto.RegisterLLMServiceServer(s, llmSrv)
	metadata.RegisterPluginMetadataServer(s, &llmSdkMetadataServer{sdk: p.sdk, impl: p.LLMGRPCPlugin.Impl})
	proto.RegisterPluginHookServiceServer(s, &hookServiceServer{hook: p.sdk.hook})
	return nil
}

// llmSdkMetadataServer 融合 LLM 业务能力（supports_images）与 SDK 声明式依赖（Requires）。
// 这两者最终都落在 PluginInfo.Capabilities 这一个 map 上：业务能力按原键
// （如 "supports_images": "true"），声明式依赖按 requires/<type>/<cap> 形式。
type llmSdkMetadataServer struct {
	metadata.UnimplementedPluginMetadataServer
	sdk  *SDK
	impl core.LLMProvider
}

// CapabilityLLM 是所有 LLM 插件默认提供的能力键（与 core.CapabilityLLM 同值）。
// agent 经 sdk.Config.Requires 声明 Requires: [{Type: "llm", Capability: "llm"}] 即可
// 表达「依赖一个 LLM 插件」——宿主据此按能力（而非插件名）解析 primary LLM provider。
// 对齐 DSH/Cordis 的 provide + inject 模型：所有 LLM 插件 provide "llm" 能力，agent inject "llm"。
const CapabilityLLM = "llm"

func (s *llmSdkMetadataServer) GetInfo(ctx context.Context, _ *metadata.Empty) (*metadata.PluginInfo, error) {
	// 以 SDK cfg 为基准（Name/Version/Type/APIVersion 由 cfg 决定，对齐 llmMetaWrapper 语义）。
	cfg := s.sdk.cfg
	if cfg.APIVersion == "" {
		cfg.APIVersion = "1.0"
	}
	caps := make(map[string]string)
	// 所有 LLM 插件默认提供 "llm" 能力，供 agent 经 Requires 声明依赖
	caps[CapabilityLLM] = "true"
	// LLM 业务能力：保留 supports_images 等键
	if s.impl != nil {
		caps["supports_images"] = boolStr(s.impl.VisionEnabled())
	}
	// 插件作者经 Config.Provides 声明的自定义能力（如 LLM 插件额外提供某能力）
	for k, v := range cfg.Provides {
		if k == "" || strings.HasPrefix(k, requiresCapabilityKeyPrefix) {
			continue
		}
		caps[k] = v
	}
	// 声明式依赖编码（与 dsc 类型 metadataServer 同样的编码方式）
	for k, v := range EncodeRequires(cfg) {
		caps[k] = v
	}
	return &metadata.PluginInfo{
		Type:         string(cfg.Type),
		Name:         cfg.Name,
		Version:      cfg.Version,
		ApiVersion:   cfg.APIVersion,
		Capabilities: caps,
	}, nil
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
