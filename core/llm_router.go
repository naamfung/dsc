package core

import (
        "context"
        "encoding/json"
        "fmt"
        "strings"

        "dsc/proto"
        plugin "github.com/hashicorp/go-plugin"
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
// 路由顺序与 provider 在 llmRouteSnapshot 的 RLock 下打成快照，调用期间不持锁
// （避免长调用阻塞热重载，同时消除与热重载写 m.llms 的并发 map 竞态）。
//
// 在调 provider 前先经 dispatchEventToPlugins 分发 agent/pre-step 事件（waterfall
// 模式）：插件（如 billion-context）可改写消息列表——注入 <acp> 标签、应用 prune、
// 注入 nudge 等。改写后的消息列表透传给实际 provider。无插件监听时事件直接通过，
// 行为与原先一致（零开销）。
//
// 失败时经 agent/request-error 事件让插件决定是否重试：插件返回 {"retry": true}
// 时重新走一轮 applyPreStepHook + provider 调用（如 billion-context 触发溢出压缩后重试）。
// 最多重试 maxRetries 次防止无限循环。
func (s *llmAggregateServer) Chat(ctx context.Context, req *proto.ChatRequest) (*proto.ChatResponse, error) {
        const maxRetries = 3
        for attempt := 0; attempt <= maxRetries; attempt++ {
                req = s.applyPreStepHook(ctx, req)
                var lastErr error
                for _, np := range s.m.llmRouteSnapshot() {
                        call := &LLMCall{Provider: np.name, Request: req}
                        err := s.m.events.Waterfall(EventLLMRequest, EventContext{Data: call}, func(EventContext) error {
                                resp, err := chatWithProvider(np.p, ctx, req)
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
                // 所有 provider 失败：检查插件是否要求重试
                if attempt < maxRetries {
                        if shouldRetry := s.emitRequestError(ctx, req, lastErr); shouldRetry {
                                s.m.logger.Info("agent/request-error: plugin requested retry",
                                        "attempt", attempt+1, "error", lastErr.Error())
                                continue
                        }
                }
                if lastErr == nil {
                        lastErr = fmt.Errorf("no LLM provider available")
                }
                return nil, lastErr
        }
        return nil, fmt.Errorf("all providers failed after %d retries", maxRetries)
}

// ChatStream 依次尝试 provider：前一个 provider 在未产生任何帧时失败才切下一个
// （已发帧后失败不切换，避免重复输出）。
//
// 同 Chat：调 provider 前先经 agent/pre-step 事件让插件改写消息列表。
// 失败时经 agent/request-error 事件让插件决定是否重试（最多 maxRetries 次）。
func (s *llmAggregateServer) ChatStream(req *proto.ChatRequest, stream proto.LLMService_ChatStreamServer) error {
        const maxRetries = 3
        for attempt := 0; attempt <= maxRetries; attempt++ {
                req = s.applyPreStepHook(stream.Context(), req)
                var lastErr error
                providerTried := false
                for _, np := range s.m.llmRouteSnapshot() {
                        call := &LLMCall{Provider: np.name, Request: req}
                        err := s.m.events.Waterfall(EventLLMRequest, EventContext{Data: call}, func(EventContext) error {
                                return chatStreamWithProvider(np.p, req, stream, call)
                        })
                        if err == nil {
                                return nil
                        }
                        providerTried = true
                        if call.StreamStarted {
                                // 已产生输出：不切换 provider，不重试，直接返回
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
                // 所有 provider 失败（或无 provider）：检查插件是否要求重试
                if providerTried && attempt < maxRetries {
                        if shouldRetry := s.emitRequestError(stream.Context(), req, lastErr); shouldRetry {
                                s.m.logger.Info("agent/request-error: plugin requested retry",
                                        "attempt", attempt+1, "error", lastErr.Error())
                                continue
                        }
                }
                if lastErr == nil {
                        lastErr = fmt.Errorf("no LLM provider available")
                }
                return lastErr
        }
        return fmt.Errorf("all providers failed after %d retries", maxRetries)
}

// applyPreStepHook 在 LLM 请求前分发 agent/pre-step 事件（waterfall 模式），
// 让插件（如 billion-context）有机会改写消息列表。无插件监听或返回空 result
// 时原样返回 req（零开销）。
//
// 改写协议：插件返回的 result_json 应为 {"messages": [...]} 形式，其中 messages
// 为改写后的 proto.Message 数组。宿主解析后替换 req.Messages。
func (s *llmAggregateServer) applyPreStepHook(ctx context.Context, req *proto.ChatRequest) *proto.ChatRequest {
        if len(s.m.hookClientsSnapshot()) == 0 {
                return req // 无插件监听：直接返回，零开销
        }
        msgsJSON, _ := json.Marshal(req.Messages)
        // 估算当前 token 数（字节/CJK 启发式，供插件 nudge 决策）
        tokenCount := 0
        for _, m := range req.Messages {
                tokenCount += len(m.Content) / 4
                if len(m.ToolCalls) > 0 {
                        tokenCount += 8
                }
        }
        result, err := s.m.dispatchEventToPlugins(EventAgentPreStep, AgentPreStepEvent{
                Agent:        s.m.GetMainAgentName(),
                MessagesJSON: string(msgsJSON),
                TokenCount:   tokenCount,
        })
        if err != nil {
                s.m.logger.Warn("agent/pre-step hook vetoed request", "error", err.Error())
                return req
        }
        // 解析改写后的消息列表
        if result == nil {
                return req
        }
        resultMap, ok := result.(map[string]any)
        if !ok {
                return req
        }
        msgsRaw, ok := resultMap["messages"]
        if !ok {
                return req
        }
        newMsgsJSON, _ := json.Marshal(msgsRaw)
        var newMsgs []*proto.Message
        if err := json.Unmarshal(newMsgsJSON, &newMsgs); err != nil || len(newMsgs) == 0 {
                return req
        }
        // 返回改写后的请求（不修改原 req，避免影响重试路径的原始数据）
        rewritten := &proto.ChatRequest{
                Messages:  newMsgs,
                Tools:     req.Tools,
                MaxTokens: req.MaxTokens,
        }
        return rewritten
}

// emitRequestError 在 LLM 请求失败后分发 agent/request-error 事件（waterfall 模式），
// 让插件（如 billion-context）有机会决定是否重试。返回 true 表示插件要求重试——
// 调用方（Chat/ChatStream）据此重新走一轮 applyPreStepHook + provider 调用。
//
// 插件经 result_json 返回 {"retry": true} 时视为重试请求。
// 典型场景：上下文溢出时 billion-context 触发紧急压缩后要求重试。
func (s *llmAggregateServer) emitRequestError(ctx context.Context, req *proto.ChatRequest, err error) bool {
        if err == nil {
                return false
        }
        code := "unknown"
        if isContextWindowExceeded(err) {
                code = "context_window_exceeded"
        }
        result, _ := s.m.dispatchEventToPlugins(EventAgentRequestError, AgentRequestErrorEvent{
                Agent: s.m.GetMainAgentName(),
                Error: err.Error(),
                Code:  code,
        })
        // 检查插件是否返回 {"retry": true}
        if result == nil {
                return false
        }
        if m, ok := result.(map[string]any); ok {
                if retry, ok := m["retry"]; ok {
                        if b, ok := retry.(bool); ok && b {
                                return true
                        }
                }
        }
        return false
}

// isContextWindowExceeded 报告错误是否为上下文窗口溢出（粗略匹配常见 provider 错误文本）。
func isContextWindowExceeded(err error) bool {
        if err == nil {
                return false
        }
        msg := err.Error()
        return containsAny(msg, "context length", "context window", "maximum context", "too long")
}

// containsAny 报告 s 是否包含 any 子串。
func containsAny(s string, subs ...string) bool {
        for _, sub := range subs {
                if strings.Contains(s, sub) {
                        return true
                }
        }
        return false
}

// namedLLMProvider 路由快照中的单个 provider。
type namedLLMProvider struct {
        name string
        p    LLMProvider
}

// llmRouteSnapshot 在 RLock 下取路由顺序与 provider 快照后立即释放锁：
// 返回 [primary（若已加载）+ 其余按加载顺序] 的有序列表。供 Chat/ChatStream 在
// 调用期间不持锁（避免长调用阻塞热重载），同時消除 handler 侧无锁读 m.llms 与
// 热重载写 m.llms 的并发 map 竞态）。
func (m *Manager) llmRouteSnapshot() []namedLLMProvider {
        m.mu.RLock()
        defer m.mu.RUnlock()
        order := make([]namedLLMProvider, 0, len(m.llmOrder)+1)
        if p, ok := m.llms[m.agentLLMName]; ok {
                order = append(order, namedLLMProvider{name: m.agentLLMName, p: p})
        }
        for _, n := range m.llmOrder {
                if n == m.agentLLMName {
                        continue
                }
                if p, ok := m.llms[n]; ok {
                        order = append(order, namedLLMProvider{name: n, p: p})
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
        m.agentLLMName = primary
        serviceID, err := m.serveAggregateLLMOnBroker(m.broker, primary)
        if err != nil {
                return 0, err
        }
        m.agentLLMServiceID = serviceID
        return serviceID, nil
}

// serveAggregateLLMOnBroker 在指定 broker 上挂载聚合 LLM 服务（provider 请求时
// 动态路由）；返回 serviceID。互通机制 1 中，该服务须挂在本插件 client 的
// broker 上（插件进程经自身 broker.Dial 访问），而不仅是 agent broker。
func (m *Manager) serveAggregateLLMOnBroker(broker *plugin.GRPCBroker, primary string) (uint32, error) {
        if broker == nil {
                return 0, fmt.Errorf("broker not available, cannot serve aggregate LLM service")
        }
        serviceID := broker.NextId()
        go broker.AcceptAndServe(serviceID, func(opts []grpc.ServerOption) *grpc.Server {
                s := grpc.NewServer(opts...)
                proto.RegisterLLMServiceServer(s, &llmAggregateServer{m: m})
                return s
        })
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
                toolCalls[i] = &proto.ToolCall{Id: tc.ID, Name: tc.Name, ArgumentsJson: string(argsJSON)}
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
                        toolCalls[i] = &proto.ToolCall{Id: tc.ID, Name: tc.Name, ArgumentsJson: string(argsJSON)}
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
