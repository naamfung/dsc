package core

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"dsc/proto"
)

// slowSilentTool 感知 ctx 的静默慢工具：执行期间零活动（不 TouchActivity），
// 模拟 provider/命令真挂起——看门狗应按时触发。
type slowSilentTool struct {
	name string
	d    time.Duration
}

func (t *slowSilentTool) Name() string                      { return t.name }
func (t *slowSilentTool) Description() string               { return "silent slow tool" }
func (t *slowSilentTool) ParametersSchema() json.RawMessage { return json.RawMessage(`{}`) }
func (t *slowSilentTool) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(t.d):
		return "done", nil
	}
}

// touchingTool 持续上报活动的工具：每 interval 触发一次 TouchActivity，
// 共 ticks 次——模拟 shell 持续输出 / 子代理持续生成。
type touchingTool struct {
	name     string
	ticks    int
	interval time.Duration
}

func (t *touchingTool) Name() string                      { return t.name }
func (t *touchingTool) Description() string               { return "touching tool" }
func (t *touchingTool) ParametersSchema() json.RawMessage { return json.RawMessage(`{}`) }
func (t *touchingTool) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	for i := 0; i < t.ticks; i++ {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(t.interval):
		}
		TouchActivity(ctx) // 活动上报
	}
	return "done", nil
}

// specFor 构造测试用超时裁决（idle 毫秒预算 + 模型可见文案）。
func specFor(idleMs int64, msg string) *proto.PolicyDecision {
	return &proto.PolicyDecision{
		Action:  "allow",
		Timeout: &proto.TimeoutSpec{IdleMs: idleMs, Message: msg},
	}
}

// TestPolicyBridgeExecuteTimeoutSpecFires 超时裁决端到端：tool/execute 槽的
// TimeoutSpec 由宿主机械安装为活跃续命执行域；静默工具超出预算即取消，
// 裁决文案原样透传为执行错误（对齐 deny reason 的透传语义），inv.Err 同步。
func TestPolicyBridgeExecuteTimeoutSpecFires(t *testing.T) {
	m := newPipelineManager(t)
	if err := m.toolRegistry.Register(&slowSilentTool{name: "silent", d: 800 * time.Millisecond}); err != nil {
		t.Fatalf("register: %v", err)
	}
	pc := newMockPolicyClient()
	pc.decide = func(ev *proto.PolicyEvent) *proto.PolicyDecision {
		if ev.GetKind() == policyEventExecute && ev.GetTool() == "silent" {
			return specFor(100, "tool idle timeout (no activity for > 100ms)")
		}
		return &proto.PolicyDecision{}
	}
	m.bridgePolicyToPipeline("timeout-policy", pc)

	start := time.Now()
	_, err := m.ExecuteTool(context.Background(), "silent", json.RawMessage(`{}`))
	elapsed := time.Since(start)
	if err == nil || err.Error() != "tool idle timeout (no activity for > 100ms)" {
		t.Fatalf("err = %v, want 裁决文案原样透传", err)
	}
	if elapsed > 600*time.Millisecond {
		t.Fatalf("看门狗应按 100ms 预算触发, took %v", elapsed)
	}
}

// TestPolicyBridgeExecuteTimeoutTouchExtends 活动续命端到端：工具持续
// TouchActivity 上报活动，总时长（300ms）远超预算（100ms）也不误杀。
func TestPolicyBridgeExecuteTimeoutTouchExtends(t *testing.T) {
	m := newPipelineManager(t)
	if err := m.toolRegistry.Register(&touchingTool{name: "touchy", ticks: 6, interval: 50 * time.Millisecond}); err != nil {
		t.Fatalf("register: %v", err)
	}
	pc := newMockPolicyClient()
	pc.decide = func(ev *proto.PolicyEvent) *proto.PolicyDecision {
		if ev.GetKind() == policyEventExecute && ev.GetTool() == "touchy" {
			return specFor(100, "tool idle timeout")
		}
		return &proto.PolicyDecision{}
	}
	m.bridgePolicyToPipeline("timeout-policy", pc)

	result, err := m.ExecuteTool(context.Background(), "touchy", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("持续活动的工具不应被看门狗取消: %v", err)
	}
	if result != "done" {
		t.Fatalf("result = %q", result)
	}
}

// TestPolicyBridgeExecuteEventFieldFidelity 执行槽事件字段保真：kind/tool/
// arguments_json/session 逐项抵达插件（跨层转发字段保真，对齐 AGENTS.md 第 3 条）。
func TestPolicyBridgeExecuteEventFieldFidelity(t *testing.T) {
	m := newPipelineManager(t)
	pc := newMockPolicyClient()
	m.bridgePolicyToPipeline("timeout-policy", pc)
	ctx := WithCaller(context.Background(), "session-7")
	if _, err := m.ExecuteTool(ctx, "plain-tool", json.RawMessage(`{"k":1}`)); err != nil {
		t.Fatalf("execute: %v", err)
	}
	var exec *proto.PolicyEvent
	for _, ev := range pc.received() {
		if ev.GetKind() == policyEventExecute {
			exec = ev
		}
	}
	if exec == nil {
		t.Fatalf("应转发 tool/execute 事件, got %v", pc.received())
	}
	if exec.GetTool() != "plain-tool" || exec.GetArgumentsJson() != `{"k":1}` || exec.GetSession() != "session-7" {
		t.Fatalf("execute 事件字段保真: %+v", exec)
	}
}

// TestPolicyBridgeExecuteDenyVetoes execute 槽 deny 同占槽拦截：工具不执行，
// reason 原文透传。
func TestPolicyBridgeExecuteDenyVetoes(t *testing.T) {
	m := newPipelineManager(t)
	pc := newMockPolicyClient()
	pc.decide = func(ev *proto.PolicyEvent) *proto.PolicyDecision {
		if ev.GetKind() == policyEventExecute {
			return &proto.PolicyDecision{Action: "deny", Reason: "denied at execute slot"}
		}
		return &proto.PolicyDecision{}
	}
	m.bridgePolicyToPipeline("timeout-policy", pc)
	_, err := m.ExecuteTool(context.Background(), "plain-tool", json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "denied at execute slot") {
		t.Fatalf("err = %v, want execute 槽 deny 透传", err)
	}
}

// TestPolicyBridgeExecuteNoSpecRunsFree execute 转发存在但无 spec（策略表外
// 工具 / env 禁用）：不安装执行域，工具照常完成——策略缺失降级为无超时。
func TestPolicyBridgeExecuteNoSpecRunsFree(t *testing.T) {
	m := newPipelineManager(t)
	pc := newMockPolicyClient()
	m.bridgePolicyToPipeline("timeout-policy", pc)
	result, err := m.ExecuteTool(context.Background(), "plain-tool", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("无 spec 不应阻塞执行: %v", err)
	}
	if result != "mock-result" {
		t.Fatalf("result = %q", result)
	}
}
