package core

import (
	"context"
	"testing"

	"dsc/proto"
	"google.golang.org/grpc"
)

// lctxMockClient 实现 proto.ToolServiceClient 的最小 mock，ListContext 返回固定片段。
type lctxMockClient struct{ content string }

func (*lctxMockClient) ExecuteTool(context.Context, *proto.ExecuteToolRequest, ...grpc.CallOption) (*proto.ExecuteToolResponse, error) {
	return &proto.ExecuteToolResponse{}, nil
}
func (*lctxMockClient) ListTools(context.Context, *proto.ListToolsRequest, ...grpc.CallOption) (*proto.ListToolsResponse, error) {
	return &proto.ListToolsResponse{}, nil
}
func (m *lctxMockClient) ListContext(context.Context, *proto.ListContextRequest, ...grpc.CallOption) (*proto.ListContextResponse, error) {
	return &proto.ListContextResponse{Content: m.content}, nil
}
func (*lctxMockClient) SetInterconnect(context.Context, *proto.InterconnectRequest, ...grpc.CallOption) (*proto.InterconnectResponse, error) {
	return &proto.InterconnectResponse{}, nil
}

// TestListContextStableOrder 校验 ListContext 按插件名稳定排序拼接上下文片段：
// map 遍历顺序随机，但输出必须按名确定，使 system prompt 前缀稳定、命中前缀缓存。
func TestListContextStableOrder(t *testing.T) {
	m := NewManager(&ManagerConfig{})
	m.mu.Lock()
	m.toolClients["zzz"] = &lctxMockClient{content: "ctx-z"}
	m.toolClients["aaa"] = &lctxMockClient{content: "ctx-a"}
	m.toolClients["mmm"] = &lctxMockClient{content: "ctx-m"}
	m.mu.Unlock()

	got, err := m.ListContext(context.Background())
	if err != nil {
		t.Fatalf("ListContext: %v", err)
	}
	const want = "ctx-a\n\nctx-m\n\nctx-z"
	if got != want {
		t.Fatalf("应按插件名稳定排序, got %q want %q", got, want)
	}
	// 多次调用结果必须一致（确定性前缀）
	for i := 0; i < 5; i++ {
		if again, _ := m.ListContext(context.Background()); again != got {
			t.Fatalf("ListContext 不稳定: %q vs %q", again, got)
		}
	}
}
