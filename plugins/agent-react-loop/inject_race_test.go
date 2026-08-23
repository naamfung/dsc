package main

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"dsc/plugin"
	"dsc/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// 本文件回归测试「TUI 在模型最后一次输出期间注入新用户消息」的竞态：
// 修复前，runLoop 在模型返回无工具调用时直接收尾返回，注入消息已写入会话却不会再
// 被任何一轮发送给模型——表现为「消息发出但工作停止」。修复后，runLoop 收尾前检测
// 到快照后有新注入则继续下一轮处理。

// mockLLMClient 可阻塞的 mock LLM：第一次 ChatStream 的 Recv 会阻塞（模拟模型输出
// 进行中），放行后返回一轮无工具调用的回答；后续调用直接返回 secondContent。
type mockLLMClient struct {
	proto.LLMServiceClient // 仅需覆盖 ChatStream，其余方法不会被调用

	mu            sync.Mutex
	streamCalls   int
	firstBlocked  chan struct{} // 第一次 Recv 进入阻塞时关闭（测试等待点）
	releaseFirst  chan struct{} // 关闭后第一次 Recv 放行（模型输出结束）
	firstOnce     sync.Once
	secondContent string
}

func (m *mockLLMClient) ChatStream(ctx context.Context, in *proto.ChatRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[proto.ChatStreamResponse], error) {
	m.mu.Lock()
	m.streamCalls++
	call := m.streamCalls
	m.mu.Unlock()
	return &mockLLMStream{parent: m, call: call}, nil
}

type mockLLMStream struct {
	parent *mockLLMClient
	call   int
	recv   int
}

func (s *mockLLMStream) Recv() (*proto.ChatStreamResponse, error) {
	s.recv++
	if s.call == 1 {
		// 第一次调用：阻塞直到测试注入消息后放行，模拟「模型正在输出最后一次内容」
		s.parent.firstOnce.Do(func() { close(s.parent.firstBlocked) })
		<-s.parent.releaseFirst
		if s.recv == 1 {
			return &proto.ChatStreamResponse{Content: "第一轮回答"}, nil
		}
		return nil, io.EOF // 无工具调用 → 模型收尾
	}
	// 后续调用（注入后继续的轮次）：直接给出最终回答
	if s.recv == 1 {
		return &proto.ChatStreamResponse{Content: s.parent.secondContent, FinishReason: "stop"}, nil
	}
	return nil, io.EOF
}

func (s *mockLLMStream) Header() (metadata.MD, error) { return nil, nil }
func (s *mockLLMStream) Trailer() metadata.MD         { return nil }
func (s *mockLLMStream) CloseSend() error             { return nil }
func (s *mockLLMStream) Context() context.Context     { return context.Background() }
func (s *mockLLMStream) SendMsg(m any) error          { return nil }
func (s *mockLLMStream) RecvMsg(m any) error          { return nil }

// mockToolClient 仅需 ListTools/ListContext（无工具调用路径不触达 ExecuteTool）。
type mockToolClient struct {
	proto.ToolServiceClient
}

func (m *mockToolClient) ListTools(ctx context.Context, in *proto.ListToolsRequest, opts ...grpc.CallOption) (*proto.ListToolsResponse, error) {
	return &proto.ListToolsResponse{}, nil
}

func (m *mockToolClient) ListContext(ctx context.Context, in *proto.ListContextRequest, opts ...grpc.CallOption) (*proto.ListContextResponse, error) {
	return &proto.ListContextResponse{}, nil
}

// TestRunLoopContinuesOnInjectedDuringStream 复现并验证竞态修复：
// 在模型最后一次输出进行中注入消息，runLoop 必须继续一轮处理注入，而不是直接收尾。
func TestRunLoopContinuesOnInjectedDuringStream(t *testing.T) {
	a := newTestAgent(t)
	a.llmServiceID = 1
	a.toolServiceID = 1
	llm := &mockLLMClient{
		firstBlocked:  make(chan struct{}),
		releaseFirst:  make(chan struct{}),
		secondContent: "第二轮回答",
	}
	a.llmClient = llm
	a.toolClient = &mockToolClient{}

	var mu sync.Mutex
	var frames []*plugin.RunStreamResponse
	emit := func(f *plugin.RunStreamResponse) {
		mu.Lock()
		frames = append(frames, f)
		mu.Unlock()
	}

	done := make(chan *plugin.AgentResult, 1)
	errCh := make(chan error, 1)
	go func() {
		res, err := a.runLoop(context.Background(), "初始问题", emit)
		if err != nil {
			errCh <- err
			return
		}
		done <- res
	}()

	// 等待模型第一次输出进入阻塞（模拟输出进行中）
	select {
	case <-llm.firstBlocked:
	case <-time.After(5 * time.Second):
		t.Fatal("模型第一次输出未进入阻塞")
	}

	// 竞态窗口：模型输出期间注入新用户消息
	if err := a.InjectMessage(context.Background(), "第二问"); err != nil {
		t.Fatalf("InjectMessage: %v", err)
	}
	close(llm.releaseFirst)

	select {
	case res := <-done:
		if res.Status != "success" {
			t.Fatalf("结果状态 = %s, 期望 success", res.Status)
		}
		// 修复后输出应为第二轮回答：注入消息被继续处理，而非第一轮就收尾
		if !strings.Contains(res.Output, "第二轮") {
			t.Fatalf("结果输出 = %q, 期望包含第二轮回答（注入后未继续处理）", res.Output)
		}
	case err := <-errCh:
		t.Fatalf("runLoop err: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("runLoop 超时未结束")
	}

	// 注入后应触发第二次 ChatStream（第二轮处理注入消息）；修复前只会有一次
	llm.mu.Lock()
	calls := llm.streamCalls
	llm.mu.Unlock()
	if calls < 2 {
		t.Fatalf("ChatStream 调用次数 = %d, 期望 >= 2（注入后应继续一轮）", calls)
	}
}
