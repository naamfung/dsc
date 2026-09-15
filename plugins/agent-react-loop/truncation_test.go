package main

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"

	"dsc/core"
	"dsc/proto"
	"dsc/session"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// 本文件回归测试「LLM 输出被 max_tokens 截断」的防护行为：截断（
// finish_reason=max_tokens/length）此前全链路无人感知——纯文本被拦腰切断时
// 轮次静默收尾（用户感知「无故中断」），工具调用参数被切断时以残参执行报错。
// 截断根因（LLM 插件默认携带 max_tokens 上限）已从源头移除；残余截断来自
// provider 侧默认输出上限（真机实测）。agent 侧防护为：截断显式告警 +
// 预算内自动续行（截断不弃任务，每轮最多 maxTruncContinuesPerTurn 次，
// 防退化复读机无限烧 token）+ 残缺参数工具调用不执行（落合成
// tool/result 保持 tool_use/tool_result 配对）。

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

// TestTruncatedTextResponseWarnsAndContinues 回归「话说一半不再静默收轮」：
// 截断的纯文本回复（无工具调用）先向 TUI 告警截断事实，随后预算内注入
// 「从中断处继续」用户消息再入循环（截断不弃任务）；下一轮模型正常
// 完成（finish=stop）后自然收轮。
func TestTruncatedTextResponseWarnsAndContinues(t *testing.T) {
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

	// 1) 两次 LLM 调用：截断后预算内续行一次，续行轮正常完成收轮
	llm.mu.Lock()
	calls := llm.calls
	llm.mu.Unlock()
	if calls != 2 {
		t.Fatalf("LLM 调用次数 = %d, 期望 2（截断续行一次）", calls)
	}

	// 2) 结果状态为 success（正常收轮），输出为续行轮的完整内容
	if !strings.Contains(res.Output, "執行 git status") {
		t.Fatalf("结果输出 = %q, 期望为续行轮内容", res.Output)
	}

	// 3) TUI 必须收到截断告警帧与续行通知帧（中断不再静默）
	mu.Lock()
	defer mu.Unlock()
	notified, continueNotice := false, false
	for _, f := range frames {
		if strings.Contains(f.Output, "被截断") {
			notified = true
		}
		if strings.Contains(f.Output, "已请求模型继续") {
			continueNotice = true
		}
	}
	if !notified {
		t.Fatalf("未输出截断告警帧")
	}
	if !continueNotice {
		t.Fatalf("未输出截断续行通知帧")
	}

	// 4) 会话中落 truncation_continue 用户消息（续行提示进入请求历史）
	continued := false
	for _, ev := range a.sess.Events() {
		if d, ok := ev.Data.(*session.UserMessageData); ok && d.Source == "truncation_continue" {
			continued = true
		}
	}
	if !continued {
		t.Fatalf("会话中未找到 truncation_continue 续行消息")
	}
}

// alwaysTruncLLM 每次调用都返回截断响应（验证续行预算防「退化复读机」无限烧 token）。
type alwaysTruncLLM struct {
	proto.LLMServiceClient

	mu    sync.Mutex
	calls int
}

func (m *alwaysTruncLLM) Chat(ctx context.Context, in *proto.ChatRequest, opts ...grpc.CallOption) (*proto.ChatResponse, error) {
	m.mu.Lock()
	m.calls++
	m.mu.Unlock()
	return &proto.ChatResponse{
		Content:      "架構設計報告：@deepseek-ai/mermaid: Diagrams",
		FinishReason: "max_tokens",
	}, nil
}

func (m *alwaysTruncLLM) ChatStream(ctx context.Context, in *proto.ChatRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[proto.ChatStreamResponse], error) {
	m.mu.Lock()
	m.calls++
	m.mu.Unlock()
	return &truncMockStream{parent: &truncMockLLM{
		firstContent: "架構設計報告：@deepseek-ai/mermaid: Diagrams",
		firstFinish:  "max_tokens",
	}, call: 1}, nil
}

// TestTruncationContinuationBudget 回归「截断续行预算」：模型持续截断（退化复读）
// 时，续行最多 maxTruncContinuesPerTurn 次（1 次初始 + 2 次续行 = 3 次调用），
// 预算耗尽后正常收轮，不无限烧 token。
func TestTruncationContinuationBudget(t *testing.T) {
	a := newTestAgent(t)
	a.llmServiceID = 1
	a.toolServiceID = 1
	llm := &alwaysTruncLLM{}
	a.llmClient = llm
	a.toolClient = &mockToolClient{}

	res, err := a.runLoop(context.Background(), "写一份架构报告", nil, nil)
	if err != nil {
		t.Fatalf("runLoop: %v", err)
	}
	if res.Status != "success" {
		t.Fatalf("结果状态 = %s, 期望 success（预算耗尽后正常收轮）", res.Status)
	}

	llm.mu.Lock()
	calls := llm.calls
	llm.mu.Unlock()
	if want := 1 + maxTruncContinuesPerTurn; calls != want {
		t.Fatalf("LLM 调用次数 = %d, 期望 %d（初始 1 次 + 续行预算 %d 次）", calls, want, maxTruncContinuesPerTurn)
	}

	// 会话中恰好落 maxTruncContinuesPerTurn 条续行消息
	continues := 0
	for _, ev := range a.sess.Events() {
		if d, ok := ev.Data.(*session.UserMessageData); ok && d.Source == "truncation_continue" {
			continues++
		}
	}
	if continues != maxTruncContinuesPerTurn {
		t.Fatalf("truncation_continue 消息数 = %d, 期望 %d", continues, maxTruncContinuesPerTurn)
	}
}

// scriptedStream 单帧脚本流：返回预置响应后 EOF（todo 持续追问测试用）。
type scriptedStream struct {
	resp *proto.ChatStreamResponse
	recv int
}

func (s *scriptedStream) Recv() (*proto.ChatStreamResponse, error) {
	s.recv++
	if s.recv == 1 {
		return s.resp, nil
	}
	return nil, io.EOF
}
func (s *scriptedStream) Header() (metadata.MD, error) { return nil, nil }
func (s *scriptedStream) Trailer() metadata.MD         { return nil }
func (s *scriptedStream) CloseSend() error             { return nil }
func (s *scriptedStream) Context() context.Context     { return context.Background() }
func (s *scriptedStream) SendMsg(m any) error          { return nil }
func (s *scriptedStream) RecvMsg(m any) error          { return nil }

// todoPersistLLM 第 1 次调用登记两项待办（todo_write 完整列表、本轮内写入——
// FoldTodos 遇 turn/start 清空，不能预置），第 2~6 次共 5 次带着未完成待办
// 自然收尾（超过旧预算制上限 3），第 7 次把待办全部标记 completed，第 8 次收尾。
type todoPersistLLM struct {
	proto.LLMServiceClient

	mu    sync.Mutex
	calls int
}

func (m *todoPersistLLM) resp(call int) *proto.ChatResponse {
	switch {
	case call == 1:
		return &proto.ChatResponse{
			Content:      "先登记待办清单。",
			FinishReason: "tool_use",
			ToolCalls:    []*proto.ToolCall{{Id: "t1", Name: "todo_write", ArgumentsJson: `{"todos":[{"content":"探索核心模块","status":"in_progress"},{"content":"撰写报告","status":"pending"}]}`}},
		}
	case call >= 2 && call <= 6:
		return &proto.ChatResponse{Content: "仍在推进。", FinishReason: "end_turn"}
	case call == 7:
		return &proto.ChatResponse{
			Content:      "全部完成，更新清单。",
			FinishReason: "tool_use",
			ToolCalls:    []*proto.ToolCall{{Id: "t2", Name: "todo_write", ArgumentsJson: `{"todos":[{"content":"探索核心模块","status":"completed"},{"content":"撰写报告","status":"completed"}]}`}},
		}
	default:
		return &proto.ChatResponse{Content: "任务已全部完成。", FinishReason: "end_turn"}
	}
}

func (m *todoPersistLLM) Chat(ctx context.Context, in *proto.ChatRequest, opts ...grpc.CallOption) (*proto.ChatResponse, error) {
	m.mu.Lock()
	m.calls++
	call := m.calls
	m.mu.Unlock()
	return m.resp(call), nil
}

func (m *todoPersistLLM) ChatStream(ctx context.Context, in *proto.ChatRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[proto.ChatStreamResponse], error) {
	m.mu.Lock()
	m.calls++
	call := m.calls
	m.mu.Unlock()
	r := m.resp(call)
	return &scriptedStream{resp: &proto.ChatStreamResponse{
		Content:      r.Content,
		FinishReason: r.FinishReason,
		ToolCalls:    r.ToolCalls,
	}}, nil
}

// TestTodoNudgePersistent 回归「待办监督不设预算」（用户裁定：清单非空就持续
// 追问直到处理完，无论完成还是取消）：模型连续 5 次带着未完成待办自然收尾，
// 系统每次都注入追问驱动——旧预算制（maxTodoNudgesPerTurn=3）在第 3 次后放行
// 收轮、待办滞留界面无人过问；新语义直到模型把各项标记 completed（第 7 次调用）
// 后才允许自然收轮。每次收尾（无论自然还是截断预算耗尽）都重新进入追问判定。
func TestTodoNudgePersistent(t *testing.T) {
	a := newTestAgent(t)
	a.llmServiceID = 1
	a.toolServiceID = 1
	llm := &todoPersistLLM{}
	a.llmClient = llm
	a.toolClient = &mockToolClient{}

	res, err := a.runLoop(context.Background(), "开始分析", nil, nil)
	if err != nil {
		t.Fatalf("runLoop: %v", err)
	}
	if res.Status != "success" {
		t.Fatalf("结果状态 = %s, 期望 success", res.Status)
	}

	llm.mu.Lock()
	calls := llm.calls
	llm.mu.Unlock()
	if want := 8; calls != want {
		t.Fatalf("LLM 调用次数 = %d, 期望 %d（登记待办 1 次 + 自然收尾 5 次 + 标记完成 1 次 + 收尾 1 次）", calls, want)
	}

	// 每次带未完成待办的收尾都触发追问：恰好 5 条 todo_nudge
	nudges := 0
	for _, ev := range a.sess.Events() {
		if d, ok := ev.Data.(*session.UserMessageData); ok && d.Source == "todo_nudge" {
			nudges++
		}
	}
	if nudges != 5 {
		t.Fatalf("todo_nudge 消息数 = %d, 期望 5（不设预算，清单非空必追问）", nudges)
	}

	// 收轮前提：清单已清空（全部 completed）
	for _, td := range session.FoldTodos(a.sess.Events()) {
		if td.Status == session.TodoPending || td.Status == session.TodoInProgress {
			t.Fatalf("收轮时仍存在未完成待办: %+v", td)
		}
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
