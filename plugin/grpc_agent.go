package plugin

import (
	"context"

	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"

	"dsc/proto"
)

// AgentGRPCPlugin 是 go-plugin 的 Agent 适配器
type AgentGRPCPlugin struct {
	plugin.Plugin
	Impl Agent
}

// GRPCServer 实现
func (p *AgentGRPCPlugin) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	proto.RegisterAgentServiceServer(s, &agentGRPCServer{impl: p.Impl})
	return nil
}

// GRPCClient 实现
func (p *AgentGRPCPlugin) GRPCClient(ctx context.Context, broker *plugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return &agentGRPCClient{client: proto.NewAgentServiceClient(c)}, nil
}

// agentGRPCServer 是 gRPC 服务端实现
type agentGRPCServer struct {
	proto.UnimplementedAgentServiceServer
	impl Agent
}

func (s *agentGRPCServer) Run(ctx context.Context, req *proto.RunRequest) (*proto.RunResponse, error) {
	result, err := s.impl.Run(ctx, req.Input)
	if err != nil {
		return nil, err
	}
	return &proto.RunResponse{
		Output: result.Output,
		Status: result.Status,
	}, nil
}

func (s *agentGRPCServer) Name(ctx context.Context, req *proto.NameRequest) (*proto.NameResponse, error) {
	return &proto.NameResponse{Name: s.impl.Name(ctx)}, nil
}

func (s *agentGRPCServer) Version(ctx context.Context, req *proto.VersionRequest) (*proto.VersionResponse, error) {
	return &proto.VersionResponse{Version: s.impl.Version(ctx)}, nil
}

// agentGRPCClient 是 gRPC 客户端代理
type agentGRPCClient struct {
	client proto.AgentServiceClient
}

func (c *agentGRPCClient) Run(ctx context.Context, input string) (*AgentResult, error) {
	resp, err := c.client.Run(ctx, &proto.RunRequest{Input: input})
	if err != nil {
		return nil, err
	}
	return &AgentResult{
		Output: resp.Output,
		Status: resp.Status,
	}, nil
}

func (c *agentGRPCClient) Name(ctx context.Context) string {
	resp, err := c.client.Name(ctx, &proto.NameRequest{})
	if err != nil {
		return ""
	}
	return resp.Name
}

func (c *agentGRPCClient) Version(ctx context.Context) string {
	resp, err := c.client.Version(ctx, &proto.VersionRequest{})
	if err != nil {
		return ""
	}
	return resp.Version
}
