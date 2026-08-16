package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"dsc/proto"
	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

// LLMProvider 是插件必须实现的业务接口
type LLMProvider interface {
	Chat(ctx context.Context, messages []Message, tools []Tool) (*ChatResponse, error)
	Name(ctx context.Context) string
	Version(ctx context.Context) string
	HealthCheck(ctx context.Context) error
}

// Message 消息结构体
type Message struct {
	Role       string `json:"role"`
	Content    string `json:"content"`
	ToolCallID string `json:"tool_call_id,omitempty"`
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
	return nil
}

func (p *LLMGRPCPlugin) GRPCClient(ctx context.Context, broker *plugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return &llmGRPCClient{client: proto.NewLLMServiceClient(c)}, nil
}

// --- gRPC 服务端实现 ---
type llmGRPCServer struct {
	proto.UnimplementedLLMServiceServer
	impl LLMProvider
}

func (s *llmGRPCServer) Chat(ctx context.Context, req *proto.ChatRequest) (*proto.ChatResponse, error) {
	// 转换 proto 消息到内部结构
	messages := make([]Message, len(req.Messages))
	for i, m := range req.Messages {
		messages[i] = Message{Role: m.Role, Content: m.Content, ToolCallID: m.ToolCallId}
	}
	tools := make([]Tool, len(req.Tools))
	for i, t := range req.Tools {
		tools[i] = Tool{Name: t.Name, Description: t.Description, ParametersJSON: t.ParametersJson}
	}

	resp, err := s.impl.Chat(ctx, messages, tools)
	if err != nil {
		return nil, err
	}

	// 转换返回结果
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

func (c *llmGRPCClient) Chat(ctx context.Context, messages []Message, tools []Tool) (*ChatResponse, error) {
	// 转换内部结构到 proto 消息
	protoMessages := make([]*proto.Message, len(messages))
	for i, m := range messages {
		protoMessages[i] = &proto.Message{Role: m.Role, Content: m.Content, ToolCallId: m.ToolCallID}
	}
	protoTools := make([]*proto.Tool, len(tools))
	for i, t := range tools {
		protoTools[i] = &proto.Tool{Name: t.Name, Description: t.Description, ParametersJson: t.ParametersJSON}
	}

	var resp *proto.ChatResponse
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		req := &proto.ChatRequest{
			Messages: protoMessages,
			Tools:    protoTools,
		}
		resp, err = c.client.Chat(ctx, req)
		if err == nil {
			break
		}
		// 如果是不可恢復錯誤（如參數錯誤），直接返回
		if strings.Contains(err.Error(), "invalid") || strings.Contains(err.Error(), "failed to parse") {
			return nil, err
		}
		time.Sleep(time.Duration(1<<attempt) * time.Second) // 指數退避
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
