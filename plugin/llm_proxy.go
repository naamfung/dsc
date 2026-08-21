package plugin

import (
	"context"
	"encoding/json"
	"fmt"

	"dsc/proto"
	"google.golang.org/grpc"
)

// llmProviderServer 将进程内已加载的 LLMProvider（经 LoadLLM 得到的原生 provider）
// 暴露为 agent broker 上的 gRPC LLMService，供 agent 通过 llmServiceID 直连。
// 取代原先由宿主 main 进程承载 LLM 代理的方案，使 LLM 服务的注册也归于 Manager 统一管理。
type llmProviderServer struct {
	proto.UnimplementedLLMServiceServer
	provider LLMProvider
}

func (s *llmProviderServer) Chat(ctx context.Context, req *proto.ChatRequest) (*proto.ChatResponse, error) {
	messages, tools := protoMessagesToPlugin(req)
	resp, err := s.provider.Chat(ctx, messages, tools, int(req.MaxTokens))
	if err != nil {
		return nil, err
	}
	toolCalls := make([]*proto.ToolCall, len(resp.ToolCalls))
	for i, tc := range resp.ToolCalls {
		argsJSON, _ := json.Marshal(tc.Arguments)
		toolCalls[i] = &proto.ToolCall{Name: tc.Name, ArgumentsJson: string(argsJSON)}
	}
	return &proto.ChatResponse{
		Content:      resp.Content,
		FinishReason: resp.FinishReason,
		ToolCalls:    toolCalls,
	}, nil
}

// ChatStream 把 LLMProvider 的流式增量逐帧转发给下游 agent
func (s *llmProviderServer) ChatStream(req *proto.ChatRequest, stream proto.LLMService_ChatStreamServer) error {
	messages, tools := protoMessagesToPlugin(req)
	ch, err := s.provider.ChatStream(stream.Context(), messages, tools)
	if err != nil {
		return err
	}
	for item := range ch {
		if item.Error != "" {
			return fmt.Errorf("LLM stream error: %s", item.Error)
		}
		toolCalls := make([]*proto.ToolCall, len(item.ToolCalls))
		for i, tc := range item.ToolCalls {
			argsJSON, _ := json.Marshal(tc.Arguments)
			toolCalls[i] = &proto.ToolCall{Name: tc.Name, ArgumentsJson: string(argsJSON)}
		}
		if err := stream.Send(&proto.ChatStreamResponse{
			Content:      item.Content,
			FinishReason: item.FinishReason,
			ToolCalls:    toolCalls,
			Usage:        UsageToProto(item.Usage),
			Reasoning:    item.Reasoning,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *llmProviderServer) Name(ctx context.Context, req *proto.NameRequest) (*proto.NameResponse, error) {
	return &proto.NameResponse{Name: s.provider.Name(ctx)}, nil
}

func (s *llmProviderServer) Version(ctx context.Context, req *proto.VersionRequest) (*proto.VersionResponse, error) {
	return &proto.VersionResponse{Version: s.provider.Version(ctx)}, nil
}

func (s *llmProviderServer) HealthCheck(ctx context.Context, req *proto.HealthCheckRequest) (*proto.HealthCheckResponse, error) {
	err := s.provider.HealthCheck(ctx)
	status := "okay"
	msg := ""
	if err != nil {
		status = "error"
		msg = err.Error()
	}
	return &proto.HealthCheckResponse{Status: status, Message: msg}, nil
}

// protoMessagesToPlugin 将 proto.ChatRequest 转换为插件侧的消息与工具列表
func protoMessagesToPlugin(req *proto.ChatRequest) ([]Message, []Tool) {
	messages := make([]Message, len(req.Messages))
	for i, m := range req.Messages {
		msg := Message{Role: m.Role, Content: m.Content}
		if len(m.ToolCalls) > 0 {
			msg.ToolCalls = make([]ToolCall, len(m.ToolCalls))
			for j, tc := range m.ToolCalls {
				var args map[string]interface{}
				if err := json.Unmarshal([]byte(tc.ArgumentsJson), &args); err != nil {
					args = map[string]interface{}{}
				}
				msg.ToolCalls[j] = ToolCall{ID: tc.Id, Name: tc.Name, Arguments: args}
			}
		}
		messages[i] = msg
	}
	tools := make([]Tool, len(req.Tools))
	for i, t := range req.Tools {
		tools[i] = Tool{Name: t.Name, Description: t.Description, ParametersJSON: t.ParametersJson}
	}
	return messages, tools
}

// serveLLMProviderLocked 把已加载的 LLMProvider 挂载到 agent broker 上并记录其 serviceID（需已持有 m.mu）。
func (m *Manager) serveLLMProviderLocked(name string) (uint32, error) {
	provider, ok := m.llms[name]
	if !ok {
		return 0, fmt.Errorf("LLM provider %q not loaded", name)
	}
	if m.broker == nil {
		return 0, fmt.Errorf("broker not available, cannot serve LLM %q", name)
	}
	serviceID := m.broker.NextId()
	go m.broker.AcceptAndServe(serviceID, func(opts []grpc.ServerOption) *grpc.Server {
		s := grpc.NewServer(opts...)
		proto.RegisterLLMServiceServer(s, &llmProviderServer{provider: provider})
		return s
	})
	m.llmServiceIDs[name] = serviceID
	return serviceID, nil
}