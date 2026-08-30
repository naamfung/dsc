package main

import (
	"context"
	"io"
	"sync"
	"testing"

	"dsc/core"
	"dsc/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// 本文件验证「工具结果帧携带 ToolView」：runLoop 在插件工具成功执行后，必须把
// ExecuteToolResponse.ViewJson 原样放进 RunStreamResponse.ToolView，供 TUI 渲染
// 专用视图。此前 react-loop 测试只覆盖本地工具(goal/todo/ask)，插件工具帧的透传
// （main.go 成功结果帧 ToolView: toolResp.ViewJson）未接 stub 验证——属链路盲点。

// toolCallLLMClient 驱动 runLoop：第一次 ChatStream 返回一个工具调用，之后返回最终回答。
type toolCallLLMClient struct {
	proto.LLMServiceClient
	mu          sync.Mutex
	streamCalls int
}

func (m *toolCallLLMClient) ChatStream(ctx context.Context, in *proto.ChatRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[proto.ChatStreamResponse], error) {
	m.mu.Lock()
	m.streamCalls++
	call := m.streamCalls
	m.mu.Unlock()
	return &toolCallLLMStream{call: call}, nil
}

type toolCallLLMStream struct {
	call, recv int
}

func (s *toolCallLLMStream) Recv() (*proto.ChatStreamResponse, error) {
	s.recv++
	if s.call == 1 && s.recv == 1 {
		return &proto.ChatStreamResponse{ToolCalls: []*proto.ToolCall{{Id: "t1", Name: "echo", ArgumentsJson: `{}`}}}, nil
	}
	if s.call == 1 {
		return nil, io.EOF
	}
	if s.recv == 1 {
		return &proto.ChatStreamResponse{Content: "final", FinishReason: "stop"}, nil
	}
	return nil, io.EOF
}

func (s *toolCallLLMStream) Header() (metadata.MD, error) { return nil, nil }
func (s *toolCallLLMStream) Trailer() metadata.MD         { return nil }
func (s *toolCallLLMStream) CloseSend() error             { return nil }
func (s *toolCallLLMStream) Context() context.Context     { return context.Background() }
func (s *toolCallLLMStream) SendMsg(m any) error          { return nil }
func (s *toolCallLLMStream) RecvMsg(m any) error          { return nil }

// viewToolClient 返回固定 Content + ViewJson 的 stub 工具客户端。
type viewToolClient struct {
	proto.ToolServiceClient
}

func (v *viewToolClient) ExecuteTool(ctx context.Context, in *proto.ExecuteToolRequest, opts ...grpc.CallOption) (*proto.ExecuteToolResponse, error) {
	return &proto.ExecuteToolResponse{
		Content:  `{"ok":true}`,
		ViewJson: `{"kind":"card","title":"Echo","badge":{"text":"ok","tone":"green"},"fields":[{"key":"state","value":"done"}]}`,
	}, nil
}
func (v *viewToolClient) ListTools(ctx context.Context, in *proto.ListToolsRequest, opts ...grpc.CallOption) (*proto.ListToolsResponse, error) {
	return &proto.ListToolsResponse{}, nil
}
func (v *viewToolClient) ListContext(ctx context.Context, in *proto.ListContextRequest, opts ...grpc.CallOption) (*proto.ListContextResponse, error) {
	return &proto.ListContextResponse{}, nil
}

// TestRunLoopForwardsPluginViewJsonToToolFrame 验证插件工具的 ViewJson 被原样
// 放进工具结果帧（main.go 成功结果帧 ToolView: toolResp.ViewJson）。
func TestRunLoopForwardsPluginViewJsonToToolFrame(t *testing.T) {
	a := newTestAgent(t)
	a.llmServiceID = 1
	a.toolServiceID = 1
	a.llmClient = &toolCallLLMClient{}
	a.toolClient = &viewToolClient{}

	var frames []*core.RunStreamResponse
	emit := func(f *core.RunStreamResponse) { frames = append(frames, f) }

	res, err := a.runLoop(context.Background(), "do it", emit)
	if err != nil {
		t.Fatalf("runLoop: %v", err)
	}
	if res.Status != "success" {
		t.Fatalf("status = %s", res.Status)
	}

	var toolFrame *core.RunStreamResponse
	for _, f := range frames {
		if f.Status == "tool" && f.ToolName == "echo" && f.ToolResult != "" {
			toolFrame = f
			break
		}
	}
	if toolFrame == nil {
		t.Fatalf("未找到 echo 的工具结果帧: %+v", frames)
	}
	if toolFrame.ToolView == "" {
		t.Fatal("工具结果帧未携带 ToolView（插件 ViewJson 未透传到帧）")
	}
	if toolFrame.ToolView != `{"kind":"card","title":"Echo","badge":{"text":"ok","tone":"green"},"fields":[{"key":"state","value":"done"}]}` {
		t.Fatalf("ToolView 与插件 ViewJson 不一致: %s", toolFrame.ToolView)
	}
}
