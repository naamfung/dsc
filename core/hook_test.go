package core

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"dsc/proto"
	"google.golang.org/grpc"
)

// plainHookTool 钩子测试用的简单工具（记录收到的参数）。
type plainHookTool struct {
	gotArgs string
}

func (t *plainHookTool) Name() string                      { return "hook-tool" }
func (t *plainHookTool) Description() string               { return "hook test tool" }
func (t *plainHookTool) ParametersSchema() json.RawMessage { return json.RawMessage(`{}`) }
func (t *plainHookTool) Execute(_ context.Context, args json.RawMessage) (string, error) {
	t.gotArgs = string(args)
	return "orig", nil
}

// fakeHook 模拟插件 PluginHookService 客户端。
type fakeHook struct {
	proto.PluginHookServiceClient
	beforeFn func(req *proto.BeforeToolRequest) *proto.BeforeToolResponse
	afterFn  func(req *proto.AfterToolRequest) *proto.AfterToolResponse
	events   chan *proto.OnEventRequest
}

func (f *fakeHook) BeforeTool(_ context.Context, req *proto.BeforeToolRequest, _ ...grpc.CallOption) (*proto.BeforeToolResponse, error) {
	if f.beforeFn != nil {
		return f.beforeFn(req), nil
	}
	return &proto.BeforeToolResponse{}, nil
}

func (f *fakeHook) AfterTool(_ context.Context, req *proto.AfterToolRequest, _ ...grpc.CallOption) (*proto.AfterToolResponse, error) {
	if f.afterFn != nil {
		return f.afterFn(req), nil
	}
	return &proto.AfterToolResponse{}, nil
}

func (f *fakeHook) OnEvent(_ context.Context, req *proto.OnEventRequest, _ ...grpc.CallOption) (*proto.OnEventResponse, error) {
	if f.events != nil {
		f.events <- req
	}
	return &proto.OnEventResponse{}, nil
}

func hookManager() (*Manager, *plainHookTool) {
	m := NewManager(&ManagerConfig{})
	tool := &plainHookTool{}
	_ = m.toolRegistry.Register(tool)
	return m, tool
}

func TestPluginHookVeto(t *testing.T) {
	m, tool := hookManager()
	m.toolHookClients["p1"] = &fakeHook{beforeFn: func(req *proto.BeforeToolRequest) *proto.BeforeToolResponse {
		return &proto.BeforeToolResponse{Veto: true, Error: "blocked"}
	}}
	m.toolHookOrder = []string{"p1"}

	_, err := m.ExecuteTool(context.Background(), "hook-tool", json.RawMessage(`{"a":1}`))
	if err == nil || !strings.Contains(err.Error(), "vetoed hook-tool: blocked") {
		t.Fatalf("veto error = %v", err)
	}
	if tool.gotArgs != "" {
		t.Fatalf("tool should not run after veto, got args %q", tool.gotArgs)
	}
}

func TestPluginHookRewriteArgs(t *testing.T) {
	m, tool := hookManager()
	m.toolHookClients["p1"] = &fakeHook{beforeFn: func(req *proto.BeforeToolRequest) *proto.BeforeToolResponse {
		return &proto.BeforeToolResponse{ArgumentsJson: `{"a":2}`}
	}}
	m.toolHookOrder = []string{"p1"}

	out, err := m.ExecuteTool(context.Background(), "hook-tool", json.RawMessage(`{"a":1}`))
	if err != nil || out != "orig" {
		t.Fatalf("execute = %q, %v", out, err)
	}
	if tool.gotArgs != `{"a":2}` {
		t.Fatalf("tool should see rewritten args, got %q", tool.gotArgs)
	}
}

func TestPluginHookRewriteResult(t *testing.T) {
	m, _ := hookManager()
	m.toolHookClients["p1"] = &fakeHook{afterFn: func(req *proto.AfterToolRequest) *proto.AfterToolResponse {
		return &proto.AfterToolResponse{Result: "rewritten"}
	}}
	m.toolHookOrder = []string{"p1"}

	out, err := m.ExecuteTool(context.Background(), "hook-tool", json.RawMessage(`{}`))
	if err != nil || out != "rewritten" {
		t.Fatalf("execute = %q, %v", out, err)
	}
}

func TestPluginHookOnEvent(t *testing.T) {
	m, _ := hookManager()
	f := &fakeHook{events: make(chan *proto.OnEventRequest, 1)}
	m.toolHookClients["p1"] = f
	m.toolHookOrder = []string{"p1"}

	// 事件广播是异步的（goroutine），带超时等待
	m.events.Emit(EventName("novelforge/done"), EventContext{Data: map[string]any{"id": "x"}})
	select {
	case req := <-f.events:
		if req.GetName() != "novelforge/done" || !strings.Contains(req.GetDataJson(), `"id":"x"`) {
			t.Fatalf("event = %+v", req)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("core should receive broadcast event")
	}
}
