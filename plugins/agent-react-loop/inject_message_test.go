package main

import (
	"context"
	"strings"
	"testing"

	"dsc/session"
)

// TestInjectMessageVisibleOnNextDerive 验证 InjectMessage 实时注入的核心语义：
// 追加一条 UserMessage 到会话历史末端后，下一次 DeriveMessages 派生出的请求历史
// 即可读到这条注入消息（模型下一次 LLM 迭代无需停止/等待本轮完成即可看到）。
func TestInjectMessageVisibleOnNextDerive(t *testing.T) {
	a := newTestAgent(t)

	// 预置一轮初始用户消息作为会话起点
	a.sess.Append(session.UserMessage,
		&session.UserMessageData{Content: "开始：帮我写个 hello", Source: "user"},
		&session.SurfaceOp{Op: session.SurfaceAppend})

	// 注入一条运行中的实时消息
	got := a.sess.Events()
	if err := a.InjectMessage(context.Background(), "顺便再打印当前时间", nil); err != nil {
		t.Fatalf("InjectMessage: %v", err)
	}

	// 注入后事件日志应多出一条 UserMessage，且位于最后
	evs := a.sess.Events()
	if len(evs) != len(got)+1 {
		t.Fatalf("注入后事件数 = %d, 期望 %d", len(evs), len(got)+1)
	}
	last := evs[len(evs)-1]
	if last.Type != session.UserMessage || last.Data == "" {
		t.Fatalf("最后一条应是被注入的 UserMessage, got type=%v data=%q", last.Type, last.Data)
	}

	// 下一次派生的请求历史（即下一次 LLM 请求的 messages）应包含注入的文本
	msgs := a.sess.DeriveMessages("sys")
	found := false
	for _, msg := range msgs {
		if strings.Contains(msg.Content, "顺便再打印当前时间") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("派生历史未读到注入消息; msgs=%+v", msgs)
	}

	// 关键：DEBUGGER 快照端点读到的是同一份派生历史——自动化测试可经 /debugger/agent
	// 观察代理运行内部状态，无需探入会话内部即可断言注入消息对模型可见。
	snap, err := a.DebugSnapshot(context.Background())
	if err != nil {
		t.Fatalf("DebugSnapshot: %v", err)
	}
	if snap.SessionID != "default" {
		t.Fatalf("snapshot session_id = %q, want default", snap.SessionID)
	}
	snapFound := false
	for _, msg := range snap.Messages {
		if msg.Role == "user" && strings.Contains(msg.Content, "顺便再打印当前时间") {
			snapFound = true
			break
		}
	}
	if !snapFound {
		t.Fatalf("DEBUGGER 快照未包含注入消息; messages=%+v", snap.Messages)
	}
}

// TestInjectMessageNoSession 验证会话未加载时注入返回错误，不 panic。
func TestInjectMessageNoSession(t *testing.T) {
	a := &ReactLoopAgent{}
	if err := a.InjectMessage(context.Background(), "x", nil); err == nil {
		t.Fatalf("期望无会话时 InjectMessage 返回错误")
	}
}
