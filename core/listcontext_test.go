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

// TestAllToolsProtoSorted 校验 AllToolsProto 产出的工具目录按名升序（供 ListTools、
// subagent LLM、run_code SDK、PTC 描述内嵌清单复用），使模型输入前缀字节稳定。
func TestAllToolsProtoSorted(t *testing.T) {
	m := NewManager(&ManagerConfig{})
	// 故意乱序注册（map 里顺序不保证），AllToolsProto 应稳定按名排序
	_ = m.toolRegistry.Register(&RemoteTool{name: "zebra"})
	_ = m.toolRegistry.Register(&RemoteTool{name: "apple"})
	_ = m.toolRegistry.Register(&RemoteTool{name: "mango"})

	out := m.AllToolsProto()
	if len(out) == 0 {
		t.Fatal("AllToolsProto 为空")
	}
	if out[0].Name != "apple" {
		t.Fatalf("应按名升序, 首工具应为 apple, got %q", out[0].Name)
	}
	if out[len(out)-1].Name != "zebra" {
		t.Fatalf("应按名升序, 末工具应为 zebra, got %q", out[len(out)-1].Name)
	}
	for i := 1; i < len(out); i++ {
		if out[i-1].Name >= out[i].Name {
			t.Fatalf("非严格升序: %s >= %s", out[i-1].Name, out[i].Name)
		}
	}
}
