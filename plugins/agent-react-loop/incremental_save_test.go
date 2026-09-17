package main

import (
	"context"
	"io"
	"sync"
	"testing"

	"dsc/proto"
	"dsc/session"
	"github.com/hashicorp/go-hclog"
	"google.golang.org/grpc"
)

// 本文件回归「会话增量落盘」：事件发生即持久化，不再只依赖 Run 结束的 defer Save。
// 场景：ask_user_question 等阻塞工具在等待用户回答期间（Run 未结束），/export 与
// 崩溃恢复必须能看到已发生的调用记录（历史上表现为「导出会话失败: session not
// found」——整轮事件滞留内存，磁盘上无会话文件）。

// toolUseThenStopLLM 非流式 mock：第一次 Chat 返回一次 tool_use 调用，第二次返回 stop 收尾。
type toolUseThenStopLLM struct {
	proto.LLMServiceClient // 仅需覆盖 Chat
	mu                     sync.Mutex
	chatCalls              int
}

func (m *toolUseThenStopLLM) Chat(ctx context.Context, in *proto.ChatRequest, opts ...grpc.CallOption) (*proto.ChatResponse, error) {
	m.mu.Lock()
	m.chatCalls++
	call := m.chatCalls
	m.mu.Unlock()
	if call == 1 {
		return &proto.ChatResponse{
			Content:      "调用工具",
			FinishReason: "tool_use",
			ToolCalls: []*proto.ToolCall{
				{Id: "call-incr-1", Name: "probe_tool", ArgumentsJson: `{}`},
			},
		}, nil
	}
	return &proto.ChatResponse{Content: "完成", FinishReason: "stop"}, nil
}

// probeToolClient 工具执行探针：ExecuteTool 进入时用独立 Store 实例从磁盘重读
// 会话文件，断言本次调用的 tool/call 记录已落盘（「调用发起前落盘」语义——
// 此后可能长时间阻塞，磁盘必须先行）。
type probeToolClient struct {
	proto.ToolServiceClient
	t   *testing.T
	dir string
}

func (m *probeToolClient) ListTools(ctx context.Context, in *proto.ListToolsRequest, opts ...grpc.CallOption) (*proto.ListToolsResponse, error) {
	return &proto.ListToolsResponse{}, nil
}

func (m *probeToolClient) ListContext(ctx context.Context, in *proto.ListContextRequest, opts ...grpc.CallOption) (*proto.ListContextResponse, error) {
	return &proto.ListContextResponse{}, nil
}

func (m *probeToolClient) ExecuteTool(ctx context.Context, req *proto.ExecuteToolRequest, opts ...grpc.CallOption) (*proto.ExecuteToolResponse, error) {
	m.t.Helper()
	st, err := session.NewStore(m.dir)
	if err != nil {
		m.t.Fatalf("reopen store: %v", err)
	}
	sess, err := st.Load("default")
	if err != nil {
		m.t.Fatalf("load session from disk: %v", err)
	}
	if sess == nil {
		m.t.Fatal("工具执行时会话文件尚未落盘（增量持久化失效）")
	}
	for _, ev := range sess.Events() {
		if ev.Type == session.ToolCallEvent {
			if d, ok := ev.Data.(*session.ToolCallData); ok && d.CallID == "call-incr-1" {
				return &proto.ExecuteToolResponse{Content: "ok"}, nil
			}
		}
	}
	m.t.Fatal("工具执行时磁盘会话已存在但缺本次 tool/call 记录（增量持久化失效）")
	return nil, nil
}

// TestSessionSavedBeforeBlockingToolExecute 阻塞类工具执行时，本次调用的
// tool/call 记录必须已从磁盘可见（Run 仍在进行中、defer Save 未触发）。
func TestSessionSavedBeforeBlockingToolExecute(t *testing.T) {
	dir := t.TempDir()
	store, err := session.NewStore(dir)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	sess, err := store.Ensure("default")
	if err != nil {
		t.Fatalf("ensure session: %v", err)
	}
	a := &ReactLoopAgent{
		store:                         store,
		sess:                          sess,
		planSection:                   defaultPlanSection,
		defaultMaxGoalRounds:          256,
		blockedAfterConsecutiveRounds: 3,
		logger:                        hclog.New(&hclog.LoggerOptions{Output: io.Discard}),
		historyInjection:              -1,
		llmServiceID:                  1,
		toolServiceID:                 1,
		llmClient:                     &toolUseThenStopLLM{},
		toolClient:                    &probeToolClient{t: t, dir: dir},
	}
	res, err := a.runLoop(context.Background(), "测试增量落盘", nil, nil)
	if err != nil {
		t.Fatalf("runLoop: %v", err)
	}
	if res == nil || res.Status != "success" {
		t.Fatalf("runLoop result = %+v", res)
	}
}
