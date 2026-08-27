package main

import (
	"context"
	"strings"
	"testing"

	"dsc/core"
	"dsc/session"
)

// TestRunLoopLimitedHistoryInjection 验证 /settings history N 生效：
// SetHistoryInjection 以 history/limit 事件持久化，runLoop 折叠后派生请求历史
// 只包含最近 N 条（回退到 user 边界，含当前输入），其余历史不再发给模型——
// 直接决定本地模型预填充长度。
func TestRunLoopLimitedHistoryInjection(t *testing.T) {
	a := newTestAgent(t)
	a.llmServiceID = 1
	a.toolServiceID = 1
	a.contextWindow = 50000

	// 预置 4 轮历史（8 条 surface）
	for i := 0; i < 4; i++ {
		a.sess.Append(session.UserMessage, &session.UserMessageData{Content: "q" + itoa(i), Source: "user"},
			&session.SurfaceOp{Op: session.SurfaceAppend})
		a.sess.Append(session.AssistantMessage, &session.AssistantMessageData{Content: "a" + itoa(i)},
			&session.SurfaceOp{Op: session.SurfaceAppend})
	}

	// 通过 SetHistoryInjection 持久化会话级限制（事件溯源，可折叠还原）
	if err := a.SetHistoryInjection(context.Background(), 2); err != nil {
		t.Fatalf("SetHistoryInjection: %v", err)
	}

	llm := &compactMockLLM{done: make(chan struct{})}
	a.llmClient = llm
	a.toolClient = &mockToolClient{}

	// emit 非空走 ChatStream 路径（与 TUI 一致），compactMockLLM 在该路径记录入参消息
	res, err := a.runLoop(context.Background(), "继续", func(*core.RunStreamResponse) {})
	if err != nil {
		t.Fatalf("runLoop: %v", err)
	}
	if res.Status != "success" {
		t.Fatalf("结果状态 = %s, 期望 success", res.Status)
	}

	llm.mu.Lock()
	msgs := llm.streamMsgs
	llm.mu.Unlock()
	// 折叠 history/limit=2 → 只保留最近 2 条 surface（回退到 user 边界）：
	// [q3, a3] + 当前输入「继续」= 3 条；全量历史是 9 条
	t.Logf("historyInjection=%d streamCalls=%d streamMsgs=%d", a.historyInjection, llm.streamCalls, len(msgs))
	if len(msgs) != 3 {
		t.Fatalf("受限后主请求消息数 = %d, 期望 3（最近 2 条历史 + 当前输入）", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Content != "q3" {
		t.Fatalf("首条应为 user(q3), 实际 (%s, %q)", msgs[0].Role, msgs[0].Content)
	}
	if last := msgs[len(msgs)-1]; last.Role != "user" || last.Content != "继续" {
		t.Fatalf("末条应为当前输入, 实际 (%s, %q)", last.Role, last.Content)
	}
	// 早期历史不得出现
	for _, m := range msgs {
		if strings.Contains(m.Content, "q0") || strings.Contains(m.Content, "q1") {
			t.Fatalf("早期历史 q0/q1 不应出现在受限请求中: %q", m.Content)
		}
	}
}

// itoa 小型辅助（避免引入 strconv 噪音）。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
