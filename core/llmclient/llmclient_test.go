package llmclient

import (
	"context"
	"testing"

	"dsc/proto"
	"google.golang.org/grpc"
)

// fakeLLM 模拟 proto.LLMServiceClient（嵌入接口以满足 mustEmbed，Chat 被覆盖）。
type fakeLLM struct {
	proto.LLMServiceClient
	resp *proto.ChatResponse
	err  error
}

func (f *fakeLLM) Chat(_ context.Context, _ *proto.ChatRequest, _ ...grpc.CallOption) (*proto.ChatResponse, error) {
	return f.resp, f.err
}

func TestDialFromEnvMissing(t *testing.T) {
	t.Setenv(EnvServiceID, "")
	c, err := DialFromEnv(nil)
	if err != nil || c != nil {
		t.Fatalf("missing env should yield (nil, nil), got %v, %v", c, err)
	}
}

func TestDialFromEnvBad(t *testing.T) {
	t.Setenv(EnvServiceID, "not-a-number")
	if _, err := DialFromEnv(nil); err == nil {
		t.Fatal("bad env should fail")
	}
}

func TestChatProxy(t *testing.T) {
	c := &Client{c: &fakeLLM{resp: &proto.ChatResponse{Content: "你好", FinishReason: "stop"}}}
	resp, err := c.Chat(context.Background(), []*proto.Message{{Role: "user", Content: "hi"}}, 0)
	if err != nil || resp.Content != "你好" || resp.FinishReason != "stop" {
		t.Fatalf("chat = %+v, %v", resp, err)
	}
	// Close 对 nil/未连接安全
	if err := (&Client{}).Close(); err != nil {
		t.Fatalf("close empty client: %v", err)
	}
	if err := (*Client)(nil).Close(); err != nil {
		t.Fatalf("close nil client: %v", err)
	}
}
