package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"dsc/proto"
)

// TestOnEventShellspecByEnv 验证 shell 裁决：env 覆盖生效，spec 携带空闲预算
// 与模型可见文案（超时文案归插件，宿主透传）。
func TestOnEventShellspecByEnv(t *testing.T) {
	t.Setenv("DSC_SHELL_TIMEOUT", "250ms")
	s := newPolicyServer()
	dec, err := s.OnEvent(context.Background(), &proto.PolicyEvent{
		Kind:          "tool/execute",
		Tool:          "shell",
		ArgumentsJson: `{"command":"sleep 1"}`,
	})
	if err != nil {
		t.Fatalf("OnEvent: %v", err)
	}
	if dec.GetAction() != "allow" {
		t.Fatalf("action = %q, want allow", dec.GetAction())
	}
	if got := dec.GetTimeout().GetIdleMs(); got != 250 {
		t.Fatalf("idle_ms = %d, want 250", got)
	}
	if !strings.Contains(dec.GetTimeout().GetMessage(), "command idle timeout") {
		t.Fatalf("message = %q, want command idle timeout 文案", dec.GetTimeout().GetMessage())
	}
}

// TestOnEventSubagentDefaultBudget 验证 subagent 裁决：未设 env 时用缺省预算，
// 文案携带可调提示（对齐旧 DSC_SUBAGENT_IDLE_TIMEOUT 语义）。
func TestOnEventSubagentDefaultBudget(t *testing.T) {
	t.Setenv("DSC_SUBAGENT_IDLE_TIMEOUT", "")
	s := newPolicyServer()
	dec, err := s.OnEvent(context.Background(), &proto.PolicyEvent{Kind: "tool/execute", Tool: "subagent"})
	if err != nil {
		t.Fatalf("OnEvent: %v", err)
	}
	if got := dec.GetTimeout().GetIdleMs(); got != int64((10*time.Minute)/time.Millisecond) {
		t.Fatalf("idle_ms = %d, want default 10m", got)
	}
	if !strings.Contains(dec.GetTimeout().GetMessage(), "DSC_SUBAGENT_IDLE_TIMEOUT") {
		t.Fatalf("message = %q, want adjust hint", dec.GetTimeout().GetMessage())
	}
}

// TestOnEventDisabledByZero 验证 env 显式 0s 禁用：不附 spec（无执行域）。
func TestOnEventDisabledByZero(t *testing.T) {
	t.Setenv("DSC_SHELL_TIMEOUT", "0s")
	s := newPolicyServer()
	dec, err := s.OnEvent(context.Background(), &proto.PolicyEvent{Kind: "tool/execute", Tool: "shell"})
	if err != nil {
		t.Fatalf("OnEvent: %v", err)
	}
	if dec.GetTimeout() != nil {
		t.Fatalf("0s 应禁用超时（无 spec）, got %+v", dec.GetTimeout())
	}
}

// TestOnEventUnknownToolAllowed 表外工具与非 execute 槽一律放行（空裁决）：
// 策略只在自己的领域（执行超时）发声，不干预 pre/post 槽的其他策略。
func TestOnEventUnknownToolAllowed(t *testing.T) {
	s := newPolicyServer()
	for _, ev := range []*proto.PolicyEvent{
		{Kind: "tool/execute", Tool: "str_replace_editor"},
		{Kind: "tool/pre-execute", Tool: "shell"},
		{Kind: "tool/post-execute", Tool: "shell"},
	} {
		dec, err := s.OnEvent(context.Background(), ev)
		if err != nil {
			t.Fatalf("OnEvent(%s/%s): %v", ev.GetKind(), ev.GetTool(), err)
		}
		if dec.GetAction() != "" || dec.GetTimeout() != nil {
			t.Fatalf("空裁决 expected for %s/%s, got %+v", ev.GetKind(), ev.GetTool(), dec)
		}
	}
}
