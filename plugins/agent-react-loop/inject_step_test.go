package main

import (
	"context"
	dsc "dsc-sdk"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"dsc/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// 本文件锁定「步（step）级插入」的确切语义，回答「通知能不能一步之内送达」：
//
// 一次 step = 一次 LLM 请求 + 该请求要求的全部工具执行。注入的消息写入会话表面后，
// 下一次**派生请求历史**时必然出现在消息列表里——即注入最迟在下一次 LLM 请求
// （下一个 step）被模型看到，不需要等本轮结束，也不需要等模型「空闲」。
//
// 唯一无法突破的边界：模型不在推理时（正在阻塞执行工具）无法被插入任何东西——
// 注入只能等这个 step 的工具执行结束、下一次请求发出。下面的测试正是钉住这一点：
// 工具阻塞期间注入，step2 的请求必须已经带上该文本。

// stepLLM 第 1 步返回一个慢工具调用；第 2 步返回最终回答。每次请求都留档，
// 供测试断言「第几步的请求里看到了注入文本」。
type stepLLM struct {
	proto.LLMServiceClient

	mu    sync.Mutex
	calls int
	reqs  []*proto.ChatRequest
}

func (m *stepLLM) ChatStream(ctx context.Context, in *proto.ChatRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[proto.ChatStreamResponse], error) {
	m.mu.Lock()
	m.calls++
	call := m.calls
	m.reqs = append(m.reqs, in)
	m.mu.Unlock()
	return &stepLLMStream{call: call}, nil
}

type stepLLMStream struct {
	call int
	recv int
}

func (s *stepLLMStream) Recv() (*proto.ChatStreamResponse, error) {
	s.recv++
	if s.call == 1 {
		if s.recv == 1 {
			return &proto.ChatStreamResponse{
				ToolCalls:    []*proto.ToolCall{{Id: "c1", Name: "slow_tool", ArgumentsJson: `{}`}},
				FinishReason: "tool_calls",
			}, nil
		}
		return nil, io.EOF
	}
	if s.recv == 1 {
		return &proto.ChatStreamResponse{Content: "全部完成", FinishReason: "stop"}, nil
	}
	return nil, io.EOF
}

func (s *stepLLMStream) Header() (metadata.MD, error) { return nil, nil }
func (s *stepLLMStream) Trailer() metadata.MD         { return nil }
func (s *stepLLMStream) CloseSend() error             { return nil }
func (s *stepLLMStream) Context() context.Context     { return context.Background() }
func (s *stepLLMStream) SendMsg(m any) error          { return nil }
func (s *stepLLMStream) RecvMsg(m any) error          { return nil }

// stepTool 提供一个可阻塞执行的工具，模拟「模型正卡在长工具里」的时间窗。
type stepTool struct {
	proto.ToolServiceClient

	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (m *stepTool) ListTools(ctx context.Context, in *proto.ListToolsRequest, opts ...grpc.CallOption) (*proto.ListToolsResponse, error) {
	return &proto.ListToolsResponse{}, nil
}

func (m *stepTool) ListContext(ctx context.Context, in *proto.ListContextRequest, opts ...grpc.CallOption) (*proto.ListContextResponse, error) {
	return &proto.ListContextResponse{}, nil
}

func (m *stepTool) ExecuteTool(ctx context.Context, in *proto.ExecuteToolRequest, opts ...grpc.CallOption) (*proto.ExecuteToolResponse, error) {
	m.once.Do(func() { close(m.started) })
	<-m.release
	return &proto.ExecuteToolResponse{Content: "slow done"}, nil
}

// TestInjectVisibleOnNextStepWhileToolRunning 验证工具执行期间的注入会在**下一个
// step** 的请求历史里出现（不是等本轮结束、更不是等离线）。
func TestInjectVisibleOnNextStepWhileToolRunning(t *testing.T) {
	a := newTestAgent(t)
	a.llmServiceID = 1
	a.toolServiceID = 1
	llm := &stepLLM{}
	tool := &stepTool{started: make(chan struct{}), release: make(chan struct{})}
	a.llmClient = llm
	a.toolClient = tool

	done := make(chan *dsc.AgentResult, 1)
	errCh := make(chan error, 1)
	go func() {
		res, err := a.runLoop(context.Background(), "开始：起服务并盯可用性", nil, func(*dsc.RunStreamResponse) {})
		if err != nil {
			errCh <- err
			return
		}
		done <- res
	}()

	// 等模型进入「工具执行」阶段——此时第 1 步的请求早已发出，模型不在推理。
	select {
	case <-tool.started:
	case <-time.After(5 * time.Second):
		t.Fatal("工具未开始执行")
	}

	notice := "background job shell-3 (shell: sleep 90 && curl -s localhost) finished [status: completed]. Read its output with job_output."
	if err := a.InjectMessage(context.Background(), notice, nil); err != nil {
		t.Fatalf("InjectMessage: %v", err)
	}
	close(tool.release)

	select {
	case <-done:
	case err := <-errCh:
		t.Fatalf("runLoop err: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("runLoop 超时未结束")
	}

	llm.mu.Lock()
	reqs := llm.reqs
	llm.mu.Unlock()
	if len(reqs) < 2 {
		t.Fatalf("LLM 请求次数 = %d, 期望 >= 2（注入后必须还有一步）", len(reqs))
	}
	if hasInjected(reqs[0], "shell-3") {
		t.Fatal("第 1 步请求不应包含注入（注入发生在该请求发出之后）")
	}
	if !hasInjected(reqs[1], "shell-3") {
		t.Fatalf("第 2 步请求未包含注入文本——步级插入失效; messages=%s", dumpMessages(reqs[1]))
	}
}

func hasInjected(req *proto.ChatRequest, needle string) bool {
	for _, m := range req.GetMessages() {
		if strings.Contains(m.GetContent(), needle) {
			return true
		}
	}
	return false
}

func dumpMessages(req *proto.ChatRequest) string {
	var b strings.Builder
	for _, m := range req.GetMessages() {
		b.WriteString("[" + m.GetRole() + "] " + m.GetContent() + "\n")
	}
	return b.String()
}
