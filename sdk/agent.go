package dsc

import (
	"context"

	"dsc/core"
	"dsc/proto"
	"dsc/proto/metadata"
	plugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

// agentGRPCPlugin 是 agent 类型插件的 go-core 适配器：宿主 broker 就绪后先
// 执行可选注入回调（sdk.AgentBroker，供实现缓存 broker 以 Dial LLM/Tool/
// UserQuestions 等服务），再注册标准 AgentServiceServer（复用宿主
// core.AgentGRPCPlugin，元数据以 sdk.Config 的 Name/Version 为准）。
//
// embed core.AgentGRPCPlugin 以继承其 GRPCClient：宿主经 loadAgentAndGetBroker
// 的 rpcClient.Dispense("agent") 获取 Agent 实例，故客户端侧必须返回实现
// core.Agent 的代理（core.AgentGRPCPlugin.GRPCClient 正是如此）。
//
// 注册 PluginMetadataServer（SDK 的 metadataServer，融合 Config.Provides 与
// Config.Requires）：宿主经 GetPluginInfo 读取 agent 的能力声明，据此解析能力依赖
// 并选择 primary LLM provider。此前 agent 不需要 PluginInfo（用 config.yaml 的
// depends_on 按名依赖），现按能力依赖模型必须注册。
type agentGRPCPlugin struct {
	core.AgentGRPCPlugin
	sdk *SDK
}

func (p *agentGRPCPlugin) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	if p.sdk.agentBroker != nil {
		if err := p.sdk.agentBroker(&AgentBroker{broker: broker}); err != nil {
			return err
		}
	}
	p.AgentGRPCPlugin.Impl = &agentMetaWrapper{Agent: p.sdk.agent, name: p.sdk.cfg.Name, version: p.sdk.cfg.Version}
	if err := p.AgentGRPCPlugin.GRPCServer(broker, s); err != nil {
		return err
	}
	// 注册 PluginMetadataServer：供宿主经 GetPluginInfo 读取 agent 的能力声明
	// （Config.Provides + Config.Requires），据此解析能力依赖。
	metadata.RegisterPluginMetadataServer(s, &agentMetadataServer{sdk: p.sdk})
	// 任何插件类型都可声明 Hook 订阅宿主事件（对齐 DSH cordis：事件广播类型无关）
	proto.RegisterPluginHookServiceServer(s, &hookServiceServer{hook: p.sdk.hook})
	return nil
}

// agentMetadataServer 融合 agent 的 Config.Provides（普通能力键）与 Config.Requires
// （requires/<type>/<cap> 前缀键），与 dsc 类型的 metadataServer 同构。
type agentMetadataServer struct {
	metadata.UnimplementedPluginMetadataServer
	sdk *SDK
}

func (s *agentMetadataServer) GetInfo(ctx context.Context, _ *metadata.Empty) (*metadata.PluginInfo, error) {
	cfg := s.sdk.cfg
	if cfg.APIVersion == "" {
		cfg.APIVersion = "1.0"
	}
	caps := make(map[string]string)
	// Config.Provides：普通能力键（如 "agent": "true"）
	for k, v := range cfg.Provides {
		if k == "" {
			continue
		}
		caps[k] = v
	}
	// Config.Requires：requires/<type>/<cap> 前缀键
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
