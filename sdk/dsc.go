package dsc

import (
	"context"

	"dsc/core"
	"dsc/proto"
	"dsc/proto/metadata"
	plugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

// dscGRPCPlugin 是通用（dsc）类型插件的 go-core 适配器：不注册任何
// tool/llm/agent/policy 服务，只注册插件元数据与可选的 Hook 服务。
// 宿主加载它时同样登记 hook client（见 core/manager.go 的 loadPluginWithBroker），
// 使这类「纯后台/程序性」插件能经 OnEvent 订阅宿主事件广播。
type dscGRPCPlugin struct {
	plugin.NetRPCUnsupportedPlugin
	sdk *SDK
}

func (p *dscGRPCPlugin) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	metadata.RegisterPluginMetadataServer(s, &metadataServer{cfg: p.sdk.cfg})
	proto.RegisterPluginHookServiceServer(s, &hookServiceServer{hook: p.sdk.hook})
	return nil
}

func (p *dscGRPCPlugin) GRPCClient(context.Context, *plugin.GRPCBroker, *grpc.ClientConn) (interface{}, error) {
	return nil, nil
}

// llmGRPCPlugin 是 llm 类型插件的 go-core 适配器：复用宿主 core.LLMGRPCPlugin
// 注册 LLMService + 元数据，并额外注册 PluginHookService，使 LLM 插件也能声明
// Hook 订阅宿主事件（对齐 DSH cordis：事件广播类型无关）。
type llmGRPCPlugin struct {
	core.LLMGRPCPlugin
	sdk *SDK
}

func (p *llmGRPCPlugin) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	if err := p.LLMGRPCPlugin.GRPCServer(broker, s); err != nil {
		return err
	}
	proto.RegisterPluginHookServiceServer(s, &hookServiceServer{hook: p.sdk.hook})
	return nil
}
