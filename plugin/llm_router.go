package plugin

import (
	"context"
	"encoding/json"
	"fmt"

	"dsc/proto"
	"google.golang.org/grpc"
)

// 多 provider 路由（对齐 DSH route）：agent 只连接单个「聚合 LLM 服务」，
// 服务按 primary（agent 声明的 provider）→ fallback（其余已加载 provider，
// 按加载顺序）依次尝试；每次尝试都经过 llm/request 瀑布（含内建退避重试），
// 前一个 provider 失败且未产生输出时切到下一个。

// llmAggregateServer 聚合 LLM 服务：动态读取 Manager 中已加载的 provider。
type llmAggregateServer struct {
	proto.UnimplementedLLMServiceServer
	m *Manager
}

// Chat 依次尝试 provider（primary 优先），首个成功即返回；全部失败返回最后的错误。
func (s *llmAggregateServer) Chat(ctx context.Context, req *proto.ChatRequest) (*proto.ChatResponse, error) {
	var lastErr error
	for _, name := range s.m.llmRouteOrderLocked() {
		call := &LLMCall{Provider: name, Request: req}
		err := s.m.events.Waterfall(EventLLMRequest, EventContext{Data: call}, func(EventContext) error {
			provider, ok := s.m.llms[name]
			if !ok {
				call.Err = fmt.Errorf("llm provider %q not loaded", name)
				return call.Err
			}
			resp, err := chatWithProvider(provider, ctx, req)
			call.Response, call.Err = resp, err
			return err
		})
		if err == nil {
			return call.Response, nil
		}
		if call.Err != nil {
			lastErr = call.Err
		} else {
			lastErr = err
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no LLM provider available")
	}
	return nil, lastErr
}

// ChatStream 依次尝试 provider：前一个 provider 在未产生任何帧时失败才切下一个
// （已发帧后失败不切换，避免重复输出）。
func (s *llmAggregateServer) ChatStream(req *proto.ChatRequest, stream proto.LLMService_ChatStreamServer) error {
	var lastErr error
	for _, name := range s.m.llmRouteOrderLocked() {
		call := &LLMCall{Provider: name, Request: req}
		err := s.m.events.Waterfall(EventLLMRequest, EventContext{Data: call}, func(EventContext) error {
			provider, ok := s.m.llms[name]
			if !ok {
				call.Err = fmt.Errorf("llm provider %q not loaded", name)
				return call.Err
			}
			return chatStreamWithProvider(provider, req, stream, call)
		})
		if err == nil {
			return nil
		}
		if call.StreamStarted {
			// 已产生输出：不切换 provider，直接返回
			if call.Err != nil {
				return call.Err
			}
			return err
		}
		if call.Err != nil {
			lastErr = call.Err
		} else {
			lastErr = err
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no LLM provider available")
	}
	return lastErr
}

// llmRouteOrderLocked 返回路由顺序：primary（agent 声明）置顶，其余按加载顺序
// （需已持有 m.mu）。
func (m *Manager) llmRouteOrderLocked() []string {
	order := make([]string, 0, len(m.llmOrder)+1)
	if _, ok := m.llms[m.agentLLMName]; ok {
		order = append(order, m.agentLLMName)
	}
	for _, n := range m.llmOrder {
		if n == m.agentLLMName {
			continue
		}
		if _, ok := m.llms[n]; ok {
			order = append(order, n)
		}
	}
	return order
}

// serveAggregateLLMLocked 在 broker 上挂载「聚合 LLM 服务」并返回其 serviceID
// （需已持有 m.mu）。已挂载则复用并更新 primary；primary 为空仅复用。
func (m *Manager) serveAggregateLLMLocked(primary string) (uint32, error) {
	if m.agentLLMServiceID != 0 {
		if primary != "" {
			m.agentLLMName = primary
		}
		return m.agentLLMServiceID, nil
	}
	if m.broker == nil {
		return 0, fmt.Errorf("broker not available, cannot serve aggregate LLM service")
	}
	m.agentLLMName = primary
	serviceID := m.broker.NextId()
	go m.broker.AcceptAndServe(serviceID, func(opts []grpc.ServerOption) *grpc.Server {
		s := grpc.NewServer(opts...)
		proto.RegisterLLMServiceServer(s, &llmAggregateServer{m: m})
		return s
	})
	m.agentLLMServiceID = serviceID
	return serviceID, nil
}

// chatWithProvider 以 provider 执行一次非流式调用并转 proto 响应。
func chatWithProvider(provider LLMProvider, ctx context.Context, req *proto.ChatRequest) (*proto.ChatResponse, error) {
	messages, tools := protoMessagesToPlugin(req)
	resp, err := provider.Chat(ctx, messages, tools, int(req.MaxTokens))
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

// chatStreamWithProvider 以 provider 执行流式调用并逐帧转发；
// 首个帧起标记 call.StreamStarted（此后失败不再切换/重试）。
func chatStreamWithProvider(provider LLMProvider, req *proto.ChatRequest, stream proto.LLMService_ChatStreamServer, call *LLMCall) error {
	messages, tools := protoMessagesToPlugin(req)
	ch, err := provider.ChatStream(stream.Context(), messages, tools)
	if err != nil {
		call.Err = err
		return err
	}
	for item := range ch {
		call.StreamStarted = true
		if item.Error != "" {
			call.Err = fmt.Errorf("LLM stream error: %s", item.Error)
			return call.Err
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
			call.Err = err
			return err
		}
	}
	return nil
}
