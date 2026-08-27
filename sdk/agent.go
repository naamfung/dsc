package dsc

import (
	"dsc/core"
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
	return p.AgentGRPCPlugin.GRPCServer(broker, s)
}
