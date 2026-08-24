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
			Turn:       item.Turn,
			Step:       item.Step,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *agentGRPCServer) Name(ctx context.Context, req *proto.NameRequest) (*proto.NameResponse, error) {
	return &proto.NameResponse{Name: s.impl.Name(ctx)}, nil
}

// RegisterServices 注入宿主挂载的 LLM/Tool 聚合服务 ID（agent 据此 Dial 依赖服务）。
func (s *agentGRPCServer) RegisterServices(ctx context.Context, req *proto.RegisterServicesRequest) (*proto.RegisterServicesResponse, error) {
	err := s.impl.RegisterServices(ctx, req.LlmServiceId, req.ToolServiceId)
	return &proto.RegisterServicesResponse{}, err
}

// SwitchSession 切换 agent 的当前会话（事件溯源日志接管）。
func (s *agentGRPCServer) SwitchSession(ctx context.Context, req *proto.SwitchSessionRequest) (*proto.SwitchSessionResponse, error) {
	if err := s.impl.SwitchSession(ctx, req.SessionId); err != nil {
		return &proto.SwitchSessionResponse{Success: false, Message: err.Error()}, nil
	}
	return &proto.SwitchSessionResponse{Success: true}, nil
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

func (s *agentGRPCServer) SetPlanMode(ctx context.Context, req *proto.SetPlanModeRequest) (*proto.SetPlanModeResponse, error) {
	if err := s.impl.SetPlanMode(ctx, req.Active); err != nil {
		return &proto.SetPlanModeResponse{Success: false, Message: err.Error()}, nil
	}
	return &proto.SetPlanModeResponse{Success: true}, nil
}

func (s *agentGRPCServer) SetUserQuestionsService(ctx context.Context, req *proto.SetUserQuestionsServiceRequest) (*proto.SetUserQuestionsServiceResponse, error) {
	if err := s.impl.SetUserQuestionsService(ctx, req.ServiceId); err != nil {
		return &proto.SetUserQuestionsServiceResponse{Success: false, Message: err.Error()}, nil
	}
	return &proto.SetUserQuestionsServiceResponse{Success: true}, nil
}

func (s *agentGRPCServer) InjectMessage(ctx context.Context, req *proto.InjectMessageRequest) (*proto.InjectMessageResponse, error) {
	if err := s.impl.InjectMessage(ctx, req.Content); err != nil {
		return nil, err
	}
	return &proto.InjectMessageResponse{}, nil
}

func (s *agentGRPCServer) DebugSnapshot(ctx context.Context, req *proto.DebugSnapshotRequest) (*proto.DebugSnapshotResponse, error) {
	snap, err := s.impl.DebugSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	return SnapshotToProto(snap), nil
}

// SnapshotToProto 把 agent 侧的调试快照转换为跨进程 proto 消息（导出供各 agent 插件复用）。
func SnapshotToProto(snap *AgentDebugSnapshot) *proto.DebugSnapshotResponse {
	if snap == nil {
		return &proto.DebugSnapshotResponse{}
	}
	out := &proto.DebugSnapshotResponse{
		SessionId:        snap.SessionID,
		TurnCount:        int32(snap.TurnCount),
		PlanActive:       snap.PlanActive,
		LastPromptTokens: snap.LastPromptTokens,
	}
	if snap.Goal != nil {
		out.Goal = &proto.GoalDebugInfo{
			Phase:          snap.Goal.Phase,
			Revision:       int32(snap.Goal.Revision),
			MaxRounds:      int32(snap.Goal.MaxRounds),
			Activation:     snap.Goal.Activation,
			Objective:      snap.Goal.Objective,
			CompletedSteps: int32(snap.Goal.CompletedSteps),
		}
	}
	for _, msg := range snap.Messages {
		out.Messages = append(out.Messages, &proto.DebugMessage{Role: msg.Role, Content: msg.Content})
	}
	return out
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
			ch <- &RunStreamResponse{Output: resp.Output, Status: resp.Status, Error: resp.Error, Usage: UsageFromProto(resp.Usage), Reasoning: resp.Reasoning, ToolName: resp.ToolName, ToolArgs: resp.ToolArgs, ToolResult: resp.ToolResult, Turn: resp.Turn, Step: resp.Step}
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

func (c *agentGRPCClient) SwitchSession(ctx context.Context, sessionID string) error {
	_, err := c.client.SwitchSession(ctx, &proto.SwitchSessionRequest{SessionId: sessionID})
	return err
}

func (c *agentGRPCClient) SetPlanMode(ctx context.Context, active bool) error {
	_, err := c.client.SetPlanMode(ctx, &proto.SetPlanModeRequest{Active: active})
	return err
}

func (c *agentGRPCClient) SetUserQuestionsService(ctx context.Context, serviceID uint32) error {
	_, err := c.client.SetUserQuestionsService(ctx, &proto.SetUserQuestionsServiceRequest{ServiceId: serviceID})
	return err
}

func (c *agentGRPCClient) Shutdown(ctx context.Context, force bool) error {
	_, err := c.client.Shutdown(ctx, &proto.ShutdownRequest{Force: force})
	return err
}

func (c *agentGRPCClient) InjectMessage(ctx context.Context, content string) error {
	_, err := c.client.InjectMessage(ctx, &proto.InjectMessageRequest{Content: content})
	return err
}

func (c *agentGRPCClient) DebugSnapshot(ctx context.Context) (*AgentDebugSnapshot, error) {
	resp, err := c.client.DebugSnapshot(ctx, &proto.DebugSnapshotRequest{})
	if err != nil {
		return nil, err
	}
	snap := &AgentDebugSnapshot{
		SessionID:        resp.SessionId,
		TurnCount:        int(resp.TurnCount),
		PlanActive:       resp.PlanActive,
		LastPromptTokens: resp.LastPromptTokens,
	}
	if resp.Goal != nil {
		snap.Goal = &AgentGoalDebugInfo{
			Phase:          resp.Goal.Phase,
			Revision:       int(resp.Goal.Revision),
			MaxRounds:      int(resp.Goal.MaxRounds),
			Activation:     resp.Goal.Activation,
			Objective:      resp.Goal.Objective,
			CompletedSteps: int(resp.Goal.CompletedSteps),
		}
	}
	for _, msg := range resp.Messages {
		snap.Messages = append(snap.Messages, &AgentDebugMessage{Role: msg.Role, Content: msg.Content})
	}
	return snap, nil
}
