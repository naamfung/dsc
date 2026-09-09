package core

import (
	"context"
	"encoding/json"
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

// mockPolicyClient 模拟 policy 插件的观测服务。
type mockPolicyClient struct {
	mu           sync.Mutex
	observations map[string]*proto.FsObservation
}

func newMockPolicyClient() *mockPolicyClient {
	return &mockPolicyClient{observations: make(map[string]*proto.FsObservation)}
}

func (c *mockPolicyClient) GetObservation(_ context.Context, req *proto.GetObservationRequest, _ ...grpc.CallOption) (*proto.GetObservationResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	obs, ok := c.observations[req.FilePath]
	if !ok {
		return &proto.GetObservationResponse{Found: false}, nil
	}
	return &proto.GetObservationResponse{Found: true, Observation: obs}, nil
}

func (c *mockPolicyClient) UpdateObservation(_ context.Context, req *proto.UpdateObservationRequest, _ ...grpc.CallOption) (*proto.UpdateObservationResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.observations[req.FilePath] = req.Observation
	return &proto.UpdateObservationResponse{Success: true}, nil
}

func (c *mockPolicyClient) observed(path string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.observations[path]
	return ok
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

func TestPolicyBridgeReadBeforeEdit(t *testing.T) {
	m := newPipelineManager(t)
	pc := newMockPolicyClient()
	m.bridgePolicyToPipeline("fs-observation-policy", pc)
	path := "/tmp/a.txt"
	writeArgs := json.RawMessage(fmt.Sprintf(`{"file_path":%q,"action":"replace"}`, path))

	// 1. 未读先写 → veto
	_, err := m.ExecuteTool(context.Background(), "str_replace_editor", writeArgs)
	if err == nil || !strings.Contains(err.Error(), "has not been read yet") {
		t.Fatalf("err = %v, want read-before-edit veto", err)
	}
	// 2. 先记录观测（模拟 read 已执行），再写 → 放行
	_, err = pc.UpdateObservation(context.Background(), &proto.UpdateObservationRequest{
		FilePath:    path,
		Observation: &proto.FsObservation{State: "observed", Version: "1", LastContent: "x"},
	})
	if err != nil {
		t.Fatalf("seed observation: %v", err)
	}
	result, err := m.ExecuteTool(context.Background(), "str_replace_editor", writeArgs)
	if err != nil {
		t.Fatalf("write after read should pass: %v", err)
	}
	if result != "mock-result" {
		t.Fatalf("result = %q", result)
	}
	// 3. post-execute 已记录新观测
	if !pc.observed(path) {
		t.Fatal("post-execute should record observation")
	}
}

func TestPolicyBridgeSkipsNonWriteTool(t *testing.T) {
	m := newPipelineManager(t)
	pc := newMockPolicyClient()
	m.bridgePolicyToPipeline("fs-observation-policy", pc)
	// 非写工具（无 file_path 断言）不受读前检查约束
	_, err := m.ExecuteTool(context.Background(), "plain-tool", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("non-write tool should not be blocked: %v", err)
	}
}

func TestPolicyBridgeUnregisterRemovesListeners(t *testing.T) {
	m := newPipelineManager(t)
	pc := newMockPolicyClient()
	offs := m.bridgePolicyToPipeline("fs-observation-policy", pc)
	for _, off := range offs {
		off()
	}
	// 卸载后 pre 检查不再生效：未读先写也应放行
	writeArgs := json.RawMessage(`{"file_path":"/tmp/b.txt","action":"replace"}`)
	if _, err := m.ExecuteTool(context.Background(), "str_replace_editor", writeArgs); err != nil {
		t.Fatalf("after unregister listeners should not veto: %v", err)
	}
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
