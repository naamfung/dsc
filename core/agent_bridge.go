package core

import (
	"context"
	"errors"

	"dsc/proto"
	plugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

// errNoMainAgent 主 agent 未配置或未加载时的统一错误。
var errNoMainAgent = errors.New("main agent not available")

// agentBridgeServer 把宿主主 agent 的「仅对话」能力（RunStream/InjectMessage）
// 桥接给工具插件：宿主把它挂在插件 client 自己的 broker 上，插件经
// agentclient.Dial 复用主 agent 会话。有意只实现这两个方法——不暴露生命周期
// 管理，避免插件能关闭/重载/切换宿主主 agent。
type agentBridgeServer struct {
	proto.UnimplementedAgentServiceServer
	m *Manager
}

func (s *agentBridgeServer) RunStream(req *proto.RunRequest, stream proto.AgentService_RunStreamServer) error {
	agent, err := s.m.getMainAgent(stream.Context())
	if err != nil {
		return err
	}
	ch, err := agent.RunStream(stream.Context(), req.Input, req.Images)
	if err != nil {
		return err
	}
	for item := range ch {
		if err := stream.Send(&proto.RunStreamResponse{
			Output: item.Output,
			Status: item.Status,
			Error:  item.Error,
			Turn:   item.Turn,
			Step:   item.Step,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *agentBridgeServer) InjectMessage(ctx context.Context, req *proto.InjectMessageRequest) (*proto.InjectMessageResponse, error) {
	agent, err := s.m.getMainAgent(ctx)
	if err != nil {
		return nil, err
	}
	if err := agent.InjectMessage(ctx, req.Content, req.Images); err != nil {
		return nil, err
	}
	return &proto.InjectMessageResponse{}, nil
}

// serveAgentBridgeOnBroker 在指定 broker 上挂载主 agent 桥接服务并返回 serviceID。
// 服务在调用时惰性解析主 agent（工具插件通常先于主 agent 加载，故不能握手指令时
// 持有 impl）；主 agent 未加载时相应 RPC 返回错误，插件自行向用户提示。
func (m *Manager) serveAgentBridgeOnBroker(broker *plugin.GRPCBroker) uint32 {
	if broker == nil {
		return 0
	}
	serviceID := broker.NextId()
	go broker.AcceptAndServe(serviceID, func(opts []grpc.ServerOption) *grpc.Server {
		s := grpc.NewServer(opts...)
		proto.RegisterAgentServiceServer(s, &agentBridgeServer{m: m})
		return s
	})
	return serviceID
}

// getMainAgent 惰性返回当前主 agent 实现（未配置或未加载时报错）。
func (m *Manager) getMainAgent(ctx context.Context) (Agent, error) {
	name := m.GetMainAgentName()
	if name == "" {
		return nil, errNoMainAgent
	}
	agent, ok := m.GetAgent(name)
	if !ok {
		return nil, errNoMainAgent
	}
	return agent, nil
}
