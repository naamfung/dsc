package main

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"

	"dsc/core"
	"dsc/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// 本文件回归测试「LLM 输出被 max_tokens 截断导致轮次静默中断/空参工具调用」的修复：
// 截断（finish_reason=max_tokens/length）此前全链路无人感知——无工具调用时轮次直接
// 收尾（用户感知「无故中断」，模型答话悬着冒号戛然而止）；工具调用参数被拦腰切断时
// 以残缺参数执行报错。修复后：截断显式告警 + 纯文本截断自动续行一次 + 残缺参数不执行
// （落合成 tool/result 保持 tool_use/tool_result 配对）。

// truncMockLLM 第一轮流返回截断响应（可带残缺工具调用），后续轮次返回正常完成。
type truncMockLLM struct {
	proto.LLMServiceClient

	mu         sync.Mutex
	calls      int
	streamMsgs [][]*proto.Message // 每次主请求的消息列表快照

	firstContent   string            // 第一轮截断文本
	firstFinish    string            // 第一轮 finish_reason（如 max_tokens）
	firstToolCalls []*proto.ToolCall // 第一轮携带的（残缺）工具调用
	secondContent  string            // 第二轮正常完成文本
}

func (m *truncMockLLM) Chat(ctx context.Context, in *proto.ChatRequest, opts ...grpc.CallOption) (*proto.ChatResponse, error) {
	m.mu.Lock()
	m.calls++
	msgs := make([]*proto.Message, len(in.Messages))
	copy(msgs, in.Messages)
	m.streamMsgs = append(m.streamMsgs, msgs)
	call := m.calls
	m.mu.Unlock()
	if call == 1 {
		return &proto.ChatResponse{Content: m.firstContent, FinishReason: m.firstFinish, ToolCalls: m.firstToolCalls}, nil
	}
	return &proto.ChatResponse{Content: m.secondContent, FinishReason: "stop"}, nil
}

func (m *truncMockLLM) ChatStream(ctx context.Context, in *proto.ChatRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[proto.ChatStreamResponse], error) {
	m.mu.Lock()
	m.calls++
	msgs := make([]*proto.Message, len(in.Messages))
	copy(msgs, in.Messages)
	m.streamMsgs = append(m.streamMsgs, msgs)
	call := m.calls
	m.mu.Unlock()
	return &truncMockStream{parent: m, call: call}, nil
}

type truncMockStream struct {
	parent *truncMockLLM
	call   int
	recv   int
}

func (s *truncMockStream) Recv() (*proto.ChatStreamResponse, error) {
	s.recv++
	if s.call == 1 {
		if s.recv == 1 {
			return &proto.ChatStreamResponse{
				Content:      s.parent.firstContent,
				FinishReason: s.parent.firstFinish,
				ToolCalls:    s.parent.firstToolCalls,
			}, nil
		}
		return nil, io.EOF
	}
	if s.recv == 1 {
		return &proto.ChatStreamResponse{Content: s.parent.secondContent, FinishReason: "stop"}, nil
	}
	return nil, io.EOF
}

func (s *truncMockStream) Header() (metadata.MD, error) { return nil, nil }
func (s *truncMockStream) Trailer() metadata.MD         { return nil }
func (s *truncMockStream) CloseSend() error             { return nil }
func (s *truncMockStream) Context() context.Context     { return context.Background() }
func (s *truncMockStream) SendMsg(m any) error          { return nil }
func (s *truncMockStream) RecvMsg(m any) error          { return nil }

// recordingToolClient 记录 ExecuteTool 调用（验证残缺参数未被下发执行）。
type recordingToolClient struct {
	proto.ToolServiceClient

	mu        sync.Mutex
	execTools []string
}

func (m *recordingToolClient) ListTools(ctx context.Context, in *proto.ListToolsRequest, opts ...grpc.CallOption) (*proto.ListToolsResponse, error) {
	return &proto.ListToolsResponse{}, nil
}

func (m *recordingToolClient) ListContext(ctx context.Context, in *proto.ListContextRequest, opts ...grpc.CallOption) (*proto.ListContextResponse, error) {
	return &proto.ListContextResponse{}, nil
}

func (m *recordingToolClient) ExecuteTool(ctx context.Context, in *proto.ExecuteToolRequest, opts ...grpc.CallOption) (*proto.ExecuteToolResponse, error) {
	m.mu.Lock()
	m.execTools = append(m.execTools, in.ToolName)
	m.mu.Unlock()
	return &proto.ExecuteToolResponse{Content: "ok"}, nil
}

// TestTruncatedTextResponseAutoContinues 回归「话说一半静默收轮」（用户感知为
// 「无故中断」）：截断的纯文本回复（无工具调用）应自动续行一次让模型从中断处
// 继续，且 TUI 收到截断告警，而不是直接结束轮次把用户晾在半句答复上。
func TestTruncatedTextResponseAutoContinues(t *testing.T) {
	a := newTestAgent(t)
	a.llmServiceID = 1
	a.toolServiceID = 1
	llm := &truncMockLLM{
		firstContent:  "Remote origin 已建立成功。現在檢查變更是否已提交：",
		firstFinish:   "max_tokens",
		secondContent: "master 有未提交變更，我來執行 git status 確認後推送。",
	}
	a.llmClient = llm
	a.toolClient = &mockToolClient{}

	var mu sync.Mutex
	var frames []*core.RunStreamResponse
	emit := func(f *core.RunStreamResponse) {
		mu.Lock()
		frames = append(frames, f)
		mu.Unlock()
	}

	res, err := a.runLoop(context.Background(), "帮我推送 master 到 github", nil, emit)
	if err != nil {
		t.Fatalf("runLoop: %v", err)
	}
	if res.Status != "success" {
		t.Fatalf("结果状态 = %s, 期望 success", res.Status)
	}

	// 1) 必须发生两次 LLM 调用（截断后自动续行，而非一轮收尾）
	llm.mu.Lock()
	calls := llm.calls
	var secondMsgs []*proto.Message
	if len(llm.streamMsgs) >= 2 {
		secondMsgs = llm.streamMsgs[1]
	}
	llm.mu.Unlock()
	if calls != 2 {
		t.Fatalf("LLM 调用次数 = %d, 期望 2（截断续行应发起第二次请求）", calls)
	}

	// 2) 第二次请求历史必须包含截断续行提示
	hasNudge := false
	for _, m := range secondMsgs {
		if m.Role == "user" && strings.Contains(m.Content, "max_tokens") && strings.Contains(m.Content, "截断") {
			hasNudge = true
			break
		}
	}
	if !hasNudge {
		t.Fatalf("第二次请求历史缺少截断续行提示")
	}

	// 3) 结果输出应为续行后的第二轮内容
	if !strings.Contains(res.Output, "git status") {
		t.Fatalf("结果输出 = %q, 期望包含续行后的第二轮回答", res.Output)
	}

	// 4) TUI 应收到截断告警帧
	mu.Lock()
	defer mu.Unlock()
	notified := false
	for _, f := range frames {
		if strings.Contains(f.Output, "被截断") {
			notified = true
			break
		}
	}
	if !notified {
		t.Fatalf("未输出截断告警帧")
	}
}

// TestTruncatedToolCallInvalidArgsSkipped 回归「無參數被提供」类顽疾：截断响应
// 携带残缺参数 JSON 的工具调用时，不得以残缺参数下发执行，而应落合成 tool/result
// （保持 tool_use/tool_result 配对，Anthropic 协议要求）并让模型重发完整调用。
func TestTruncatedToolCallInvalidArgsSkipped(t *testing.T) {
	a := newTestAgent(t)
	a.llmServiceID = 1
	a.toolServiceID = 1
	llm := &truncMockLLM{
		firstContent:   "現在檢查 SSH 連線...</text>",
		firstFinish:    "max_tokens",
		firstToolCalls: []*proto.ToolCall{{Id: "call_1", Name: "shell", ArgumentsJson: `{"command": "git ls-rem`}},
		secondContent:  "已改用完整参数重新发出工具调用，任务完成。",
	}
	a.llmClient = llm
	tools := &recordingToolClient{}
	a.toolClient = tools

	res, err := a.runLoop(context.Background(), "帮我推送 master 到 github", nil, nil)
	if err != nil {
		t.Fatalf("runLoop: %v", err)
	}
	if res.Status != "success" {
		t.Fatalf("结果状态 = %s, 期望 success", res.Status)
	}

	// 1) 残缺参数的工具调用不得被执行
	tools.mu.Lock()
	execCalls := append([]string(nil), tools.execTools...)
	tools.mu.Unlock()
	if len(execCalls) != 0 {
		t.Fatalf("被执行的工具 = %v, 期望空（残缺参数不应下发执行）", execCalls)
	}

	// 2) 第二次请求历史必须包含 call_1 的合成 tool/result（配对完整），说明截断原因
	llm.mu.Lock()
	var secondMsgs []*proto.Message
	if len(llm.streamMsgs) >= 2 {
		secondMsgs = llm.streamMsgs[1]
	}
	llm.mu.Unlock()
	paired := false
	for _, m := range secondMsgs {
		if m.Role == "tool" && m.ToolCallId == "call_1" && strings.Contains(m.Content, "截断") {
			paired = true
			break
		}
	}
	if !paired {
		t.Fatalf("第二次请求历史缺少 call_1 的合成 tool/result（tool_use/tool_result 未配对）")
	}

	// 3) LLM 调用次数 = 2（截断轮处置后继续，第二轮正常完成收尾）
	llm.mu.Lock()
	calls := llm.calls
	llm.mu.Unlock()
	if calls != 2 {
		t.Fatalf("LLM 调用次数 = %d, 期望 2", calls)
	}
}

// TestIsTruncatedFinishReason 单元覆盖两种主流 finish_reason 词汇（Anthropic/OpenAI）。
func TestIsTruncatedFinishReason(t *testing.T) {
	for _, fr := range []string{"max_tokens", "length", "MAX_TOKENS", " length "} {
		if !isTruncatedFinishReason(fr) {
			t.Fatalf("isTruncatedFinishReason(%q) = false, 期望 true", fr)
		}
	}
	for _, fr := range []string{"", "stop", "end_turn", "tool_use", "tool_calls", "stop_sequence"} {
		if isTruncatedFinishReason(fr) {
			t.Fatalf("isTruncatedFinishReason(%q) = true, 期望 false", fr)
		}
	}
}
