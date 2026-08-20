package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"dsc/proto"
	"dsc/proto/metadata"
	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// LLMProvider 是插件必须实现的业务接口
type LLMProvider interface {
	// Chat 以非流式方式调用 LLM；maxTokens <= 0 表示使用服务端默认
	Chat(ctx context.Context, messages []Message, tools []Tool, maxTokens int) (*ChatResponse, error)
	// ChatStream 以流式方式调用 LLM，返回增量内容通道；通道关闭表示该轮结束
	ChatStream(ctx context.Context, messages []Message, tools []Tool) (<-chan *ChatStreamResponse, error)
	Name(ctx context.Context) string
	Version(ctx context.Context) string
	HealthCheck(ctx context.Context) error
}

// Message 消息结构体
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"` // assistant 消息回传的工具调用
}

// Tool 工具结构体
type Tool struct {
	Name             string `json:"name"`
	Description      string `json:"description"`
	ParametersJSON   string `json:"parameters_json"`
}

// ChatResponse 聊天响应结构体
type ChatResponse struct {
	Content      string      `json:"content"`
	FinishReason string      `json:"finish_reason"`
	ToolCalls    []ToolCall  `json:"tool_calls"`
}

// ChatStreamResponse 是 LLM 流式响应中的一帧增量内容
type ChatStreamResponse struct {
	Content      string     `json:"content"`       // 增量文本
	FinishReason string     `json:"finish_reason"` // 非空表示该轮结束
	ToolCalls    []ToolCall `json:"tool_calls"`
	Error        string     `json:"error,omitempty"`
	Usage        *Usage     `json:"usage,omitempty"` // 本轮 token 用量（finish_reason 帧携带）
	Reasoning    string     `json:"reasoning,omitempty"` // 思考过程增量文本（DeepSeek reasoning_content / Anthropic thinking 等）
}

// ToolCall 工具调用结构体
type ToolCall struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// LLMGRPCPlugin 是 go-plugin 的 LLM 适配器
type LLMGRPCPlugin struct {
	plugin.Plugin
	Impl LLMProvider
}

func (p *LLMGRPCPlugin) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	proto.RegisterLLMServiceServer(s, &llmGRPCServer{impl: p.Impl})
	metadata.RegisterPluginMetadataServer(s, &llmMetadataServer{impl: p.Impl})
	return nil
}

// llmMetadataServer 提供 LLM 插件的元數據服務，供宿主按配置加載時做類型/版本校驗
type llmMetadataServer struct {
	metadata.UnimplementedPluginMetadataServer
	impl LLMProvider
}

func (s *llmMetadataServer) GetInfo(ctx context.Context, _ *metadata.Empty) (*metadata.PluginInfo, error) {
	return &metadata.PluginInfo{
		Type:       "llm",
		Name:       s.impl.Name(ctx),
		Version:    s.impl.Version(ctx),
		ApiVersion: "1.0",
	}, nil
}

func (p *LLMGRPCPlugin) GRPCClient(ctx context.Context, broker *plugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return &llmGRPCClient{client: proto.NewLLMServiceClient(c)}, nil
}

// --- gRPC 服务端实现 ---
type llmGRPCServer struct {
	proto.UnimplementedLLMServiceServer
	impl LLMProvider
}

// convertLLMChatRequest 将 proto 请求转换为内部消息与工具列表
func convertLLMChatRequest(req *proto.ChatRequest) ([]Message, []Tool) {
	messages := make([]Message, len(req.Messages))
	for i, m := range req.Messages {
		msg := Message{Role: m.Role, Content: m.Content, ToolCallID: m.ToolCallId}
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

// convertToolCallsToProto 将内部工具调用列表转换为 proto 形式
func convertToolCallsToProto(toolCalls []ToolCall) []*proto.ToolCall {
	out := make([]*proto.ToolCall, len(toolCalls))
	for i, tc := range toolCalls {
		argsJSON, _ := json.Marshal(tc.Arguments)
		out[i] = &proto.ToolCall{Id: tc.ID, Name: tc.Name, ArgumentsJson: string(argsJSON)}
	}
	return out
}

// UsageToProto 将内部 Usage 转换为 proto 形式（nil 返回 nil）
func UsageToProto(u *Usage) *proto.Usage {
	if u == nil {
		return nil
	}
	return &proto.Usage{
		PromptTokens:     u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
		TotalTokens:      u.TotalTokens,
	}
}

// UsageFromProto 将 proto Usage 转换为内部形式（nil 返回 nil）
func UsageFromProto(u *proto.Usage) *Usage {
	if u == nil {
		return nil
	}
	return &Usage{
		PromptTokens:     u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
		TotalTokens:      u.TotalTokens,
	}
}

func (s *llmGRPCServer) Chat(ctx context.Context, req *proto.ChatRequest) (*proto.ChatResponse, error) {
	messages, tools := convertLLMChatRequest(req)

	resp, err := s.impl.Chat(ctx, messages, tools, int(req.MaxTokens))
	if err != nil {
		return nil, err
	}

	return &proto.ChatResponse{
		Content:      resp.Content,
		FinishReason: resp.FinishReason,
		ToolCalls:    convertToolCallsToProto(resp.ToolCalls),
	}, nil
}

func (s *llmGRPCServer) ChatStream(req *proto.ChatRequest, stream proto.LLMService_ChatStreamServer) error {
	messages, tools := convertLLMChatRequest(req)

	ch, err := s.impl.ChatStream(stream.Context(), messages, tools)
	if err != nil {
		return err
	}
	for item := range ch {
		if item.Error != "" {
			return status.Errorf(codes.Unknown, "LLM stream error: %s", item.Error)
		}
		if err := stream.Send(&proto.ChatStreamResponse{
			Content:      item.Content,
			FinishReason: item.FinishReason,
			ToolCalls:    convertToolCallsToProto(item.ToolCalls),
			Usage:        UsageToProto(item.Usage),
			Reasoning:    item.Reasoning,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *llmGRPCServer) Name(ctx context.Context, req *proto.NameRequest) (*proto.NameResponse, error) {
	return &proto.NameResponse{Name: s.impl.Name(ctx)}, nil
}

func (s *llmGRPCServer) Version(ctx context.Context, req *proto.VersionRequest) (*proto.VersionResponse, error) {
	return &proto.VersionResponse{Version: s.impl.Version(ctx)}, nil
}

func (s *llmGRPCServer) HealthCheck(ctx context.Context, req *proto.HealthCheckRequest) (*proto.HealthCheckResponse, error) {
	err := s.impl.HealthCheck(ctx)
	status := "okay"
	msg := ""
	if err != nil {
		status = "error"
		msg = err.Error()
	}
	return &proto.HealthCheckResponse{Status: status, Message: msg}, nil
}

// --- gRPC 客户端代理 ---
type llmGRPCClient struct {
	client proto.LLMServiceClient
}

// convertToProtoMessages 将内部消息列表转换为 proto 形式
func convertToProtoMessages(messages []Message) []*proto.Message {
	protoMessages := make([]*proto.Message, len(messages))
	for i, m := range messages {
		pm := &proto.Message{Role: m.Role, Content: m.Content, ToolCallId: m.ToolCallID}
		if len(m.ToolCalls) > 0 {
			pm.ToolCalls = make([]*proto.ToolCall, len(m.ToolCalls))
			for j, tc := range m.ToolCalls {
				argsJSON, _ := json.Marshal(tc.Arguments)
				pm.ToolCalls[j] = &proto.ToolCall{Id: tc.ID, Name: tc.Name, ArgumentsJson: string(argsJSON)}
			}
		}
		protoMessages[i] = pm
	}
	return protoMessages
}

// convertToProtoTools 将内部工具列表转换为 proto 形式
func convertToProtoTools(tools []Tool) []*proto.Tool {
	protoTools := make([]*proto.Tool, len(tools))
	for i, t := range tools {
		protoTools[i] = &proto.Tool{Name: t.Name, Description: t.Description, ParametersJson: t.ParametersJSON}
	}
	return protoTools
}

func (c *llmGRPCClient) Chat(ctx context.Context, messages []Message, tools []Tool, maxTokens int) (*ChatResponse, error) {
	req := &proto.ChatRequest{
		Messages:  convertToProtoMessages(messages),
		Tools:     convertToProtoTools(tools),
		MaxTokens: int32(maxTokens),
	}

	var resp *proto.ChatResponse
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		resp, err = c.client.Chat(ctx, req)
		if err == nil {
			break
		}
		// 如果是參數錯誤（InvalidArgument），直接返回
		if s, ok := status.FromError(err); ok && s.Code() == codes.InvalidArgument {
			return nil, err
		}
		// 其他錯誤重試
		time.Sleep(time.Duration(1<<attempt) * time.Second)
	}
	if err != nil {
		return nil, fmt.Errorf("LLM call failed after retries: %w", err)
	}

	toolCalls := make([]ToolCall, len(resp.ToolCalls))
	for i, tc := range resp.ToolCalls {
		var args map[string]interface{}
		if err := json.Unmarshal([]byte(tc.ArgumentsJson), &args); err != nil {
			return nil, fmt.Errorf("failed to parse tool call arguments for %s: %w", tc.Name, err)
		}
		toolCalls[i] = ToolCall{ID: tc.Id, Name: tc.Name, Arguments: args}
	}

	return &ChatResponse{
		Content:      resp.Content,
		FinishReason: resp.FinishReason,
		ToolCalls:    toolCalls,
	}, nil
}

func (c *llmGRPCClient) ChatStream(ctx context.Context, messages []Message, tools []Tool) (<-chan *ChatStreamResponse, error) {
	req := &proto.ChatRequest{
		Messages: convertToProtoMessages(messages),
		Tools:    convertToProtoTools(tools),
	}
	stream, err := c.client.ChatStream(ctx, req)
	if err != nil {
		return nil, err
	}

	ch := make(chan *ChatStreamResponse)
	go func() {
		defer close(ch)
		for {
			resp, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				ch <- &ChatStreamResponse{Error: err.Error()}
				return
			}
			toolCalls := make([]ToolCall, len(resp.ToolCalls))
			for i, tc := range resp.ToolCalls {
				var args map[string]interface{}
				if err := json.Unmarshal([]byte(tc.ArgumentsJson), &args); err != nil {
					args = map[string]interface{}{}
				}
				toolCalls[i] = ToolCall{ID: tc.Id, Name: tc.Name, Arguments: args}
			}
			ch <- &ChatStreamResponse{
				Content:      resp.Content,
				FinishReason: resp.FinishReason,
				ToolCalls:    toolCalls,
				Usage:        UsageFromProto(resp.Usage),
				Reasoning:    resp.Reasoning,
			}
		}
	}()
	return ch, nil
}

func (c *llmGRPCClient) Name(ctx context.Context) string {
	resp, _ := c.client.Name(ctx, &proto.NameRequest{})
	if resp == nil {
		return ""
	}
	return resp.Name
}

func (c *llmGRPCClient) Version(ctx context.Context) string {
	resp, _ := c.client.Version(ctx, &proto.VersionRequest{})
	if resp == nil {
		return ""
	}
	return resp.Version
}

func (c *llmGRPCClient) HealthCheck(ctx context.Context) error {
	resp, err := c.client.HealthCheck(ctx, &proto.HealthCheckRequest{})
	if err != nil {
		return err
	}
	if resp.Status != "okay" {
		return fmt.Errorf("health check failed: %s", resp.Message)
	}
	return nil
}
