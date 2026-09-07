package core

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"dsc/proto"
	"dsc/userquestions"
	"google.golang.org/grpc"
)

// capTool 实现 ToolDefinition 并携带能力标签（模拟插件工具声明了 requires-never-approval）。
type capTool struct {
	name string
	caps []string
}

func (t *capTool) Name() string                      { return t.name }
func (t *capTool) Description() string               { return "cap tool" }
func (t *capTool) ParametersSchema() json.RawMessage { return json.RawMessage(`{}`) }
func (t *capTool) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	return "cap-result", nil
}

func (t *capTool) HasCapability(cap string) bool {
	for _, c := range t.caps {
		if c == cap {
			return true
		}
	}
	return false
}

// newNeverGateManager 只挂 neverApprovalGate，注册一把声明该能力的工具与两把不带该能力的控制工具。
func newNeverGateManager(t *testing.T, policy ApprovalPolicy) *Manager {
	t.Helper()
	m := newRouterManager()
	if err := m.RegisterUserQuestionProvider(func(context.Context, *userquestions.Request) (*userquestions.Answer, error) {
		return &userquestions.Answer{Answers: []userquestions.AnswerItem{{ID: "approval", Selected: []string{approvalAllowLabel}}}}, nil
	}); err != nil {
		t.Fatalf("RegisterUserQuestionProvider: %v", err)
	}
	for _, tool := range []ToolDefinition{
		&capTool{name: "bench_start", caps: []string{CapabilityRequiresNeverApproval}},
		&capTool{name: "other-cap-tool", caps: []string{"unrelated-capability"}},
		&mockTool{name: "plain_tool"},
	} {
		if err := m.toolRegistry.Register(tool); err != nil {
			t.Fatalf("register %s: %v", tool.Name(), err)
		}
	}
	m.SetApprovalPolicy(policy)
	m.events.OnWaterfall(EventToolPreExecute, m.neverApprovalGate())
	return m
}

func runTool(m *Manager, name string) error {
	_, err := m.ExecuteTool(context.Background(), name, json.RawMessage("{}"))
	return err
}

// 声明了 requires-never-approval 的工具在 ask 策略下被前置闸门拒绝，提示先设 never。
func TestNeverGateRejectsAskOnCapabilityTool(t *testing.T) {
	m := newNeverGateManager(t, ApprovalAsk)
	err := runTool(m, "bench_start")
	if err == nil {
		t.Fatal("ask 策略下应拒绝声明该能力的工具，却放行了")
	}
	if !strings.Contains(err.Error(), "never") || !strings.Contains(err.Error(), "/approval never") {
		t.Fatalf("错误信息缺少设 never 的指引: %v", err)
	}
}

// 声明了该能力的工具在 never 策略下放行。
func TestNeverGateAllowsNeverOnCapabilityTool(t *testing.T) {
	m := newNeverGateManager(t, ApprovalNever)
	if err := runTool(m, "bench_start"); err != nil {
		t.Fatalf("never 策略下应放行，却报错: %v", err)
	}
}

// 未声明该能力的工具（即使 ask 策略）放行。
func TestNeverGateAllowsToolWithoutCapability(t *testing.T) {
	m := newNeverGateManager(t, ApprovalAsk)
	if err := runTool(m, "plain_tool"); err != nil {
		t.Fatalf("无该能力工具应放行，却报错: %v", err)
	}
}

// 声明了无关能力标签的工具（ask 策略）放行——按能力而非插件名识别。
func TestNeverGateAllowsUnrelatedCapability(t *testing.T) {
	m := newNeverGateManager(t, ApprovalAsk)
	if err := runTool(m, "other-cap-tool"); err != nil {
		t.Fatalf("无关能力工具应放行，却报错: %v", err)
	}
}

// listToolsStub 注入预置 ListTools 响应以验证 listStagedTools 的字段保真。
type listToolsStub struct {
	proto.ToolServiceClient
	tools []*proto.Tool
}

func (s *listToolsStub) ListTools(ctx context.Context, in *proto.ListToolsRequest, opts ...grpc.CallOption) (*proto.ListToolsResponse, error) {
	return &proto.ListToolsResponse{Tools: s.tools}, nil
}

// 字段保真：proto.Tool.capabilities → RemoteTool.capabilities → HasCapability 逐跳存活，
// 防能力标签在聚合层丢失（对齐字段保真往返测试先例）。
func TestListStagedToolsPreservesCapabilities(t *testing.T) {
	stub := &listToolsStub{tools: []*proto.Tool{
		{Name: "bench_start", Description: "d", ParametersJson: `{}`, Capabilities: []string{CapabilityRequiresNeverApproval}},
		{Name: "plain", Description: "d", ParametersJson: `{}`},
	}}
	defs, _, err := listStagedTools(stub)
	if err != nil {
		t.Fatalf("listStagedTools: %v", err)
	}
	rt, ok := defs[0].(*RemoteTool)
	if !ok {
		t.Fatalf("期望 *RemoteTool，得 %T", defs[0])
	}
	if !rt.HasCapability(CapabilityRequiresNeverApproval) {
		t.Fatal("capabilities 在生成 RemoteTool 时丢失")
	}
	if plain, ok := defs[1].(*RemoteTool); ok && plain.HasCapability(CapabilityRequiresNeverApproval) {
		t.Fatal("无能力工具不应具备该能力")
	}
}
