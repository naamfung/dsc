package core

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"dsc/proto"
	"google.golang.org/grpc"
)

// LLM 请求瀑布（对齐 DSH agent/request + llm/stream 的拦截思想）：
// 宿主 LLM 服务的每次请求都经过 waterfall 事件，监听器可拦截（veto）、
// 改写或重试。内建 LLMRetryListener 提供退避重试。

// EventLLMRequest 每次 LLM 请求的瀑布（waterfall）：
// next 为实际调用；监听器不调 next 即 veto，或包裹 next 做重试/改写。
const EventLLMRequest EventName = "llm/request"

// LLMCall 一次 LLM 请求的瀑布上下文（共享指针，监听器可直接改写）。
type LLMCall struct {
	Provider string
	Request  *proto.ChatRequest
	// Response 非流式调用（Chat）的结果；流式调用（ChatStream）为 nil。
	Response *proto.ChatResponse
	Err      error
	// StreamStarted 流式调用是否已发送过帧；已开始则重试不再安全（避免重复输出）。
	StreamStarted bool
}

// LLMRetryListener 内建退避重试监听器：next 失败且未开始流式输出时按指数退避重试，
// 最多 maxAttempts 次。适用于 unary Chat 与流式建立阶段失败；流中途失败不重试。
func LLMRetryListener(maxAttempts int, backoff time.Duration) WaterfallListener {
	return func(ctx EventContext, next func(EventContext) error) error {
		call, _ := ctx.Data.(*LLMCall)
		var lastErr error
		for attempt := 0; ; attempt++ {
			lastErr = next(ctx)
			if lastErr == nil {
				return nil
			}
			if attempt+1 >= maxAttempts || (call != nil && call.StreamStarted) {
				return lastErr
			}
			time.Sleep(backoff * time.Duration(1<<attempt))
		}
	}
}

// llmProviderServer 将进程内已加载的 LLMProvider（经 LoadLLM 得到的原生 provider）
// 暴露为 agent broker 上的 gRPC LLMService，供 agent 通过 llmServiceID 直连。
// 取代原先由宿主 main 进程承载 LLM 代理的方案，使 LLM 服务的注册也归于 Manager 统一管理。
// 每次请求经 EventLLMRequest 瀑布（拦截/重试），由 events 派发。
type llmProviderServer struct {
	proto.UnimplementedLLMServiceServer
	provider LLMProvider
	events   *EventBus
}

func (s *llmProviderServer) Chat(ctx context.Context, req *proto.ChatRequest) (*proto.ChatResponse, error) {
	call := &LLMCall{Provider: s.provider.Name(ctx), Request: req}
	if err := s.events.Waterfall(EventLLMRequest, EventContext{Data: call}, func(EventContext) error {
		resp, err := chatWithProvider(s.provider, ctx, req)
		call.Response, call.Err = resp, err
		return err
	}); err != nil && call.Err == nil {
		call.Err = err // 瀑布 veto（实际调用未运行）
	}
	return call.Response, call.Err
}

// ChatStream 把 LLMProvider 的流式增量逐帧转发给下游 agent；
// 经 EventLLMRequest 瀑布（建立阶段失败可重试，已发帧后失败不重试）。
func (s *llmProviderServer) ChatStream(req *proto.ChatRequest, stream proto.LLMService_ChatStreamServer) error {
	call := &LLMCall{Provider: s.provider.Name(stream.Context()), Request: req}
	return s.events.Waterfall(EventLLMRequest, EventContext{Data: call}, func(EventContext) error {
		return chatStreamWithProvider(s.provider, req, stream, call)
	})
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
		msg := Message{Role: m.Role, Content: m.Content, Images: m.Images}
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
		proto.RegisterLLMServiceServer(s, &llmProviderServer{provider: provider, events: m.events})
		return s
	})
	m.llmServiceIDs[name] = serviceID
	return serviceID, nil
}
