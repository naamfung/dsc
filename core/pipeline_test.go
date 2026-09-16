package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"dsc/proto"
	"google.golang.org/grpc"
)

// mockTool 测试用工具实现。
type mockTool struct {
	name string
}

func (t *mockTool) Name() string                      { return t.name }
func (t *mockTool) Description() string               { return "mock tool" }
func (t *mockTool) ParametersSchema() json.RawMessage { return json.RawMessage(`{}`) }
func (t *mockTool) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	return "mock-result", nil
}

// mockPolicyClient 模拟 policy 插件的通用策略服务（proto.PolicyServiceClient）：
// 按预设脚本返回裁决，并记录收到的全部事件供字段保真断言。
type mockPolicyClient struct {
	mu      sync.Mutex
	decide  func(ev *proto.PolicyEvent) *proto.PolicyDecision
	events  []*proto.PolicyEvent
	callErr error // 非 nil 时模拟策略服务不可用（best-effort 放行路径）
}

func newMockPolicyClient() *mockPolicyClient { return &mockPolicyClient{} }

func (c *mockPolicyClient) OnEvent(_ context.Context, ev *proto.PolicyEvent, _ ...grpc.CallOption) (*proto.PolicyDecision, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, ev)
	if c.callErr != nil {
		return nil, c.callErr
	}
	if c.decide == nil {
		return &proto.PolicyDecision{}, nil
	}
	return c.decide(ev), nil
}

func (c *mockPolicyClient) received() []*proto.PolicyEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]*proto.PolicyEvent(nil), c.events...)
}

func newPipelineManager(t *testing.T) *Manager {
	t.Helper()
	m := newRouterManager() // 无默认 retry/sandbox/spill 监听器，测试控制流水线
	if err := m.toolRegistry.Register(&mockTool{name: "str_replace_editor"}); err != nil {
		t.Fatalf("register tool: %v", err)
	}
	if err := m.toolRegistry.Register(&mockTool{name: "plain-tool"}); err != nil {
		t.Fatalf("register tool: %v", err)
	}
	return m
}

func TestExecuteToolPipelineNormal(t *testing.T) {
	m := newPipelineManager(t)
	var order []string
	m.events.OnWaterfall(EventToolPreExecute, func(ctx EventContext, next func(EventContext) error) error {
		order = append(order, "pre")
		return next(ctx)
	})
	m.events.OnWaterfall(EventToolPostExecute, func(ctx EventContext, next func(EventContext) error) error {
		order = append(order, "post")
		return next(ctx)
	})
	result, err := m.ExecuteTool(context.Background(), "plain-tool", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result != "mock-result" {
		t.Fatalf("result = %q, want mock-result", result)
	}
	if strings.Join(order, ",") != "pre,post" {
		t.Fatalf("order = %v, want [pre post]", order)
	}
}

func TestExecuteToolPreVetoBlocksExecution(t *testing.T) {
	m := newPipelineManager(t)
	m.events.OnWaterfall(EventToolPreExecute, func(ctx EventContext, next func(EventContext) error) error {
		return fmt.Errorf("blocked by policy")
	})
	var postSeen bool
	m.events.OnWaterfall(EventToolPostExecute, func(ctx EventContext, next func(EventContext) error) error {
		inv := ctx.Data.(*ToolInvocation)
		postSeen = inv.Err != nil && strings.Contains(inv.Err.Error(), "blocked")
		return next(ctx)
	})
	_, err := m.ExecuteTool(context.Background(), "plain-tool", json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "blocked by policy") {
		t.Fatalf("err = %v, want blocked by policy", err)
	}
	if !postSeen {
		t.Fatal("post-execute should observe the vetoed invocation")
	}
}

func TestExecuteToolPostRewrite(t *testing.T) {
	m := newPipelineManager(t)
	m.events.OnWaterfall(EventToolPostExecute, func(ctx EventContext, next func(EventContext) error) error {
		inv := ctx.Data.(*ToolInvocation)
		if err := next(ctx); err != nil {
			return err
		}
		inv.Result = "rewritten:" + inv.Result
		return nil
	})
	result, err := m.ExecuteTool(context.Background(), "plain-tool", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result != "rewritten:mock-result" {
		t.Fatalf("result = %q, want rewritten:mock-result", result)
	}
}

// TestPolicyBridgeDenyBlocksExecution pre-execute deny 即占槽拦截：工具不执行，
// reason 原文透传调用方；事件字段逐项保真抵达插件（跨层转发字段保真，对齐
// AGENTS.md 第 3 条）。
func TestPolicyBridgeDenyBlocksExecution(t *testing.T) {
	m := newPipelineManager(t)
	pc := newMockPolicyClient()
	pc.decide = func(ev *proto.PolicyEvent) *proto.PolicyDecision {
		if ev.GetKind() == policyEventPreExecute {
			return &proto.PolicyDecision{Action: "deny", Reason: "file has not been read"}
		}
		return &proto.PolicyDecision{}
	}
	m.bridgePolicyToPipeline("fs-observation-policy", pc)
	_, err := m.ExecuteTool(context.Background(), "str_replace_editor", json.RawMessage(`{"path":"/tmp/a.txt"}`))
	if err == nil || !strings.Contains(err.Error(), "file has not been read") {
		t.Fatalf("err = %v, want deny reason 透传", err)
	}
	evs := pc.received()
	if len(evs) != 2 {
		t.Fatalf("应转发 pre+post 两个事件, got %d", len(evs))
	}
	pre := evs[0]
	if pre.GetKind() != policyEventPreExecute || pre.GetTool() != "str_replace_editor" ||
		pre.GetArgumentsJson() != `{"path":"/tmp/a.txt"}` || pre.GetSession() != "" {
		t.Fatalf("pre 事件字段保真: %+v", pre)
	}
	if evs[1].GetKind() != policyEventPostExecute || evs[1].GetError() == "" {
		t.Fatalf("post 事件应携带 veto 错误: %+v", evs[1])
	}
}

// TestPolicyBridgeForwardsSessionID 验证会话标识随事件转发（per-session 属主依据）。
func TestPolicyBridgeForwardsSessionID(t *testing.T) {
	m := newPipelineManager(t)
	pc := newMockPolicyClient()
	m.bridgePolicyToPipeline("fs-observation-policy", pc)
	ctx := WithCaller(context.Background(), "session-42")
	if _, err := m.ExecuteTool(ctx, "plain-tool", json.RawMessage(`{}`)); err != nil {
		t.Fatalf("execute: %v", err)
	}
	for _, ev := range pc.received() {
		if ev.GetSession() != "session-42" {
			t.Fatalf("事件 session 字段应转发调用方会话, got %q", ev.GetSession())
		}
	}
}

// TestPolicyBridgeReplaceRewritesResult post-execute replace 即改写模型可见结果。
func TestPolicyBridgeReplaceRewritesResult(t *testing.T) {
	m := newPipelineManager(t)
	pc := newMockPolicyClient()
	pc.decide = func(ev *proto.PolicyEvent) *proto.PolicyDecision {
		if ev.GetKind() == policyEventPostExecute {
			return &proto.PolicyDecision{Action: "replace", Result: "replaced-by-policy"}
		}
		return &proto.PolicyDecision{}
	}
	m.bridgePolicyToPipeline("fs-observation-policy", pc)
	result, err := m.ExecuteTool(context.Background(), "plain-tool", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result != "replaced-by-policy" {
		t.Fatalf("result = %q, want replaced-by-policy", result)
	}
}

// TestPolicyBridgeServiceUnavailableDoesNotBlock 策略服务不可用时 best-effort 放行：
// 策略缺失降级为无策略，而非工具不可用。
func TestPolicyBridgeServiceUnavailableDoesNotBlock(t *testing.T) {
	m := newPipelineManager(t)
	pc := newMockPolicyClient()
	pc.callErr = errors.New("connection refused")
	m.bridgePolicyToPipeline("fs-observation-policy", pc)
	result, err := m.ExecuteTool(context.Background(), "plain-tool", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("策略服务不可用不应阻塞执行: %v", err)
	}
	if result != "mock-result" {
		t.Fatalf("result = %q", result)
	}
}

// TestPolicyBridgeFailedToolForwardsError 工具执行失败时 post 事件仍转发
// （失败是权威观察：如读到不存在的路径须记录 confirmed absent）。
func TestPolicyBridgeFailedToolForwardsError(t *testing.T) {
	m := newPipelineManager(t)
	if err := m.toolRegistry.Register(&failingTool{name: "failing"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	pc := newMockPolicyClient()
	m.bridgePolicyToPipeline("fs-observation-policy", pc)
	_, _ = m.ExecuteTool(context.Background(), "failing", json.RawMessage(`{}`))
	var post *proto.PolicyEvent
	for _, ev := range pc.received() {
		if ev.GetKind() == policyEventPostExecute && ev.GetTool() == "failing" {
			post = ev
		}
	}
	if post == nil || post.GetError() == "" {
		t.Fatalf("失败工具的 post 事件应携带错误: %+v", post)
	}
}

// TestPolicyBridgeUnregisterRemovesListeners 卸载 policy 后监听器撤销：deny 不再生效。
func TestPolicyBridgeUnregisterRemovesListeners(t *testing.T) {
	m := newPipelineManager(t)
	pc := newMockPolicyClient()
	pc.decide = func(ev *proto.PolicyEvent) *proto.PolicyDecision {
		return &proto.PolicyDecision{Action: "deny", Reason: "blocked"}
	}
	offs := m.bridgePolicyToPipeline("fs-observation-policy", pc)
	for _, off := range offs {
		off()
	}
	if _, err := m.ExecuteTool(context.Background(), "str_replace_editor", json.RawMessage(`{}`)); err != nil {
		t.Fatalf("卸载后 deny 不应生效: %v", err)
	}
}

// failingTool 测试用必败工具。
type failingTool struct{ name string }

func (t *failingTool) Name() string                      { return t.name }
func (t *failingTool) Description() string               { return "always fails" }
func (t *failingTool) ParametersSchema() json.RawMessage { return json.RawMessage(`{}`) }
func (t *failingTool) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	return "", errors.New("boom")
}

// TestToolPipelineEmitsToolResult 验证工具执行完成后广播 tools/result 事件（对齐 DSH tools/result）。
func TestToolPipelineEmitsToolResult(t *testing.T) {
	m := newPipelineManager(t)
	var got []ToolResultInfo
	m.events.On(EventToolResult, func(ctx EventContext) (any, error) {
		if info, ok := ctx.Data.(ToolResultInfo); ok {
			got = append(got, info)
		}
		return nil, nil
	})
	result, err := m.ExecuteTool(context.Background(), "plain-tool", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result != "mock-result" {
		t.Fatalf("result = %q", result)
	}
	if len(got) != 1 {
		t.Fatalf("应广播 1 个 tools/result, got %d: %+v", len(got), got)
	}
	if got[0].ToolName != "plain-tool" || got[0].Result != "mock-result" || got[0].Error != "" {
		t.Fatalf("tools/result 载荷异常: %+v", got[0])
	}
}

// TestToolExecuteWaterfallCanWrap 验证 tools/execute 为独立 waterfall 拦截点：
// 监听器可包围执行（在 next 前后记录），且不调 next 即 veto（不执行）。
func TestToolExecuteWaterfallCanWrap(t *testing.T) {
	m := newPipelineManager(t)
	var order []string
	m.events.OnWaterfall(EventToolExecute, func(ctx EventContext, next func(EventContext) error) error {
		order = append(order, "exec-before")
		err := next(ctx)
		order = append(order, "exec-after")
		return err
	})
	_, err := m.ExecuteTool(context.Background(), "plain-tool", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if strings.Join(order, ",") != "exec-before,exec-after" {
		t.Fatalf("execute 拦截包裹顺序 = %v, want exec-before,exec-after", order)
	}
}

// TestToolExecuteWaterfallVeto 验证 tools/execute 不调 next 即阻止执行。
func TestToolExecuteWaterfallVeto(t *testing.T) {
	m := newPipelineManager(t)
	m.events.OnWaterfall(EventToolExecute, func(ctx EventContext, next func(EventContext) error) error {
		return fmt.Errorf("execute vetoed")
	})
	_, err := m.ExecuteTool(context.Background(), "plain-tool", json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "execute vetoed") {
		t.Fatalf("execute veto 应返回错误, got %v", err)
	}
}

// panickyTool 在 Execute 中 panic，用于验证 executeToolBody 的通用 panic recover。
type panickyTool struct{ name string }

func (t *panickyTool) Name() string                      { return t.name }
func (t *panickyTool) Description() string               { return "panics on execute" }
func (t *panickyTool) ParametersSchema() json.RawMessage { return json.RawMessage(`{}`) }
func (t *panickyTool) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	panic("simulated tool panic — should not crash host")
}

// TestExecuteToolRecoversFromPanic 验证 host 侧 executeToolBody 的通用 panic recover：
// 工具 Execute panic 时返回错误而非崩溃宿主进程。对齐「工具意外不中断会话」
// 的最终防线设计——覆盖所有宿主侧工具（builtin / read_spill / run_code / agent 内部工具等）。
func TestExecuteToolRecoversFromPanic(t *testing.T) {
	m := newPipelineManager(t)
	if err := m.toolRegistry.Register(&panickyTool{name: "panicky"}); err != nil {
		t.Fatalf("register panicky tool: %v", err)
	}
	_, err := m.ExecuteTool(context.Background(), "panicky", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("panicky tool should return error, got nil")
	}
	if !strings.Contains(err.Error(), "panicked") {
		t.Fatalf("err = %v, want contains 'panicked'", err)
	}
	if !strings.Contains(err.Error(), "simulated tool panic") {
		t.Fatalf("err = %v, want contains panic message", err)
	}
}

// TestExecuteToolRecoversFromViewPanic 验证 ViewExecutor 的 ViewFn panic 也被 recover。
type panickyViewExecutor struct{ name string }

func (t *panickyViewExecutor) Name() string                      { return t.name }
func (t *panickyViewExecutor) Description() string               { return "panics on view exec" }
func (t *panickyViewExecutor) ParametersSchema() json.RawMessage { return json.RawMessage(`{}`) }
func (t *panickyViewExecutor) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	return "ok", nil
}
func (t *panickyViewExecutor) ExecuteWithView(_ context.Context, _ json.RawMessage) (string, string, error) {
	panic("simulated view-exec panic — should not crash host")
}

func TestExecuteToolRecoversFromViewPanic(t *testing.T) {
	m := newPipelineManager(t)
	if err := m.toolRegistry.Register(&panickyViewExecutor{name: "panicky-view-exec"}); err != nil {
		t.Fatalf("register panicky view-exec tool: %v", err)
	}
	_, err := m.ExecuteTool(context.Background(), "panicky-view-exec", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("panicky view-exec tool should return error, got nil")
	}
	if !strings.Contains(err.Error(), "panicked") {
		t.Fatalf("err = %v, want contains 'panicked'", err)
	}
}

// TestRemoteToolRecoversFromPanic 验证 RemoteTool.ExecuteWithView 的通用 panic recover：
// 客户端调用 panic 时返回错误而非崩溃宿主。模拟方式：直接调用 nil client 触发 panic。
func TestRemoteToolRecoversFromPanic(t *testing.T) {
	rt := &RemoteTool{name: "nil-client"}
	// client 为 nil，调用 .ExecuteTool 必然 panic（nil pointer dereference）。
	_, _, err := rt.ExecuteWithView(context.Background(), json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("nil client should cause panic that is recovered to error, got nil err")
	}
	if !strings.Contains(err.Error(), "panicked") {
		t.Fatalf("err = %v, want contains 'panicked'", err)
	}
}

// TestReadSpillRejectsFilePath 验证 read_spill 工具拒绝文件路径形式的 locator，
// 引导模型改用 spill:N 格式。这修复了模型误把 read_spill 当成文件读取工具的问题。
func TestReadSpillRejectsFilePath(t *testing.T) {
	store, err := NewSpillStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewSpillStore: %v", err)
	}
	tool := &readSpillTool{store: store}

	// 文件路径形式的 locator 应被拒绝（不返回底层 file-not-found 错误）
	_, err = tool.Execute(context.Background(), json.RawMessage(`{"locator":"D:/foo/spill-3.txt"}`))
	if err == nil {
		t.Fatal("file path locator should be rejected")
	}
	if !strings.Contains(err.Error(), "spill:<id>") {
		t.Fatalf("err = %v, want hint about spill:<id> format", err)
	}
	if !strings.Contains(err.Error(), "filesystem path") {
		t.Fatalf("err = %v, want hint about filesystem path misuse", err)
	}

	// 合法 locator 应正常工作（先保存再读取）
	locator, err := store.SaveText("hello spill")
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	out, err := tool.Execute(context.Background(), json.RawMessage(fmt.Sprintf(`{"locator":%q}`, locator)))
	if err != nil {
		t.Fatalf("read spill: %v", err)
	}
	if out != "hello spill" {
		t.Fatalf("got %q, want 'hello spill'", out)
	}
}
