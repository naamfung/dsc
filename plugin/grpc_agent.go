package plugin

import (
	"context"
	"io"

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

func (s *agentGRPCServer) RunStream(req *proto.RunRequest, stream proto.AgentService_RunStreamServer) error {
	ch, err := s.impl.RunStream(stream.Context(), req.Input)
	if err != nil {
		return err
	}
	for item := range ch {
		if err := stream.Send(&proto.RunStreamResponse{
			Output:     item.Output,
			Status:     item.Status,
			Error:      item.Error,
			Usage:      UsageToProto(item.Usage),
			Reasoning:  item.Reasoning,
			ToolName:   item.ToolName,
			ToolArgs:   item.ToolArgs,
			ToolResult: item.ToolResult,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *agentGRPCServer) Name(ctx context.Context, req *proto.NameRequest) (*proto.NameResponse, error) {
	return &proto.NameResponse{Name: s.impl.Name(ctx)}, nil
}

func (s *agentGRPCServer) Version(ctx context.Context, req *proto.VersionRequest) (*proto.VersionResponse, error) {
	return &proto.VersionResponse{Version: s.impl.Version(ctx)}, nil
}

func (s *agentGRPCServer) Shutdown(ctx context.Context, req *proto.ShutdownRequest) (*proto.ShutdownResponse, error) {
	err := s.impl.Shutdown(ctx, req.Force)
	if err != nil {
		return &proto.ShutdownResponse{Success: false, Message: err.Error()}, err
	}
	return &proto.ShutdownResponse{Success: true, Message: "shutdown successful"}, nil
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

func (c *agentGRPCClient) RunStream(ctx context.Context, input string) (<-chan *RunStreamResponse, error) {
	stream, err := c.client.RunStream(ctx, &proto.RunRequest{Input: input})
	if err != nil {
		return nil, err
	}

	ch := make(chan *RunStreamResponse)
	go func() {
		defer close(ch)
		for {
			resp, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				ch <- &RunStreamResponse{Status: "error", Error: err.Error()}
				return
			}
			ch <- &RunStreamResponse{Output: resp.Output, Status: resp.Status, Error: resp.Error, Usage: UsageFromProto(resp.Usage), Reasoning: resp.Reasoning, ToolName: resp.ToolName, ToolArgs: resp.ToolArgs, ToolResult: resp.ToolResult}
		}
	}()
	return ch, nil
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

func (c *agentGRPCClient) RegisterServices(ctx context.Context, llmServiceID, toolServiceID uint32) error {
	_, err := c.client.RegisterServices(ctx, &proto.RegisterServicesRequest{LlmServiceId: llmServiceID, ToolServiceId: toolServiceID})
	return err
}

func (c *agentGRPCClient) Shutdown(ctx context.Context, force bool) error {
	_, err := c.client.Shutdown(ctx, &proto.ShutdownRequest{Force: force})
	return err
}
