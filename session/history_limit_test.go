package session

import (
	"testing"

	"dsc/proto"
)

// TestDeriveMessagesLimitedOff 历史注入关闭（0）：只保留当前轮（最后一条 user 起），
// 不注入任何先前轮次；模型始终能看到本次输入。
func TestDeriveMessagesLimitedOff(t *testing.T) {
	s := New()
	appendSurface(t, s, UserMessage, &UserMessageData{Content: "q0", Source: "user"})
	appendSurface(t, s, AssistantMessage, &AssistantMessageData{Content: "a0"})
	// 当前轮：q1 → a1（最后一条 user 是 q1）
	appendSurface(t, s, UserMessage, &UserMessageData{Content: "q1", Source: "user"})
	appendSurface(t, s, AssistantMessage, &AssistantMessageData{Content: "a1"})

	msgs := s.DeriveMessagesLimited("sys", 0)
	// 只保留当前轮 [q1, a1]，先前轮 [q0, a0] 被截断
	if len(msgs) != 3 {
		t.Fatalf("off 应保留 system+当前轮, 实际 %d 条: %+v", len(msgs), msgs)
	}
	if msgs[1].Role != "user" || msgs[1].Content != "q1" {
		t.Fatalf("当前轮首条应为 user(q1), 实际 (%s, %q)", msgs[1].Role, msgs[1].Content)
	}
}

// TestDeriveMessagesLimitedCount 按条数截取最近 N 条，并回退到最近的 user 边界。
func TestDeriveMessagesLimitedCount(t *testing.T) {
	s := New()
	for i := 0; i < 5; i++ {
		appendSurface(t, s, UserMessage, &UserMessageData{Content: "q", Source: "user"})
		appendSurface(t, s, AssistantMessage, &AssistantMessageData{Content: "a"})
	}
	// 10 条 surface；limit=3 → 最近 3 条 [a3,q4,a4] 起点是 assistant，回退到 q3 起，
	// 保留 [q3,a3,q4,a4] = 4 条（完整轮次，工具配对完整）。
	msgs := s.DeriveMessagesLimited("sys", 3)
	if len(msgs) != 1+4 {
		t.Fatalf("limit=3 应保留 system+4 条（回退到 user 边界）, 实际 %d 条", len(msgs))
	}
	if msgs[1].Role != "user" {
		t.Fatalf("首条应为 user 边界, 实际 role=%q", msgs[1].Role)
	}
	if msgs[len(msgs)-1].Role != "assistant" {
		t.Fatalf("末条应为最近消息(assistant), 实际 role=%q", msgs[len(msgs)-1].Role)
	}
}

// TestDeriveMessagesLimitedUserBoundary 截断回退到 user 边界，避免落在孤立的 tool
// 结果或未配对 tool_calls 的 assistant 上（Anthropic 要求 tool_use 与 tool_result 成对）。
func TestDeriveMessagesLimitedUserBoundary(t *testing.T) {
	s := New()
	// q1 → assistant(tool_call) → tool 结果 → q2 → assistant
	appendSurface(t, s, UserMessage, &UserMessageData{Content: "q1", Source: "user"})
	appendSurface(t, s, AssistantMessage, &AssistantMessageData{Content: "", ToolCalls: []*proto.ToolCall{{Id: "c1", Name: "x"}}})
	appendSurface(t, s, ToolResult, &ToolResultData{CallID: "c1", Content: "r1"})
	appendSurface(t, s, UserMessage, &UserMessageData{Content: "q2", Source: "user"})
	appendSurface(t, s, AssistantMessage, &AssistantMessageData{Content: "a2"})

	// limit=2 → 最近 2 条 [q2,a2] 起点恰为 user，保留完整
	msgs := s.DeriveMessagesLimited("sys", 2)
	if len(msgs) != 1+2 {
		t.Fatalf("期望保留 system+user(q2)+assistant(a2), 实际 %d 条", len(msgs))
	}
	if msgs[1].Role != "user" || msgs[1].Content != "q2" {
		t.Fatalf("首条应为 user(q2), 实际 (%s, %q)", msgs[1].Role, msgs[1].Content)
	}
	if msgs[2].Content != "a2" {
		t.Fatalf("第二条应为 assistant(a2), 实际 %q", msgs[2].Content)
	}
}

// TestDeriveMessagesLimitedKeepsCurrentTurn 当前轮较长时（limit 落在轮内），
// 回退到当前轮 user 边界，绝不丢弃本次输入。
func TestDeriveMessagesLimitedKeepsCurrentTurn(t *testing.T) {
	s := New()
	appendSurface(t, s, UserMessage, &UserMessageData{Content: "q0", Source: "user"})
	appendSurface(t, s, AssistantMessage, &AssistantMessageData{Content: "a0"})
	// 当前轮：q1 → assistant(tool_call) → tool 结果 → assistant
	appendSurface(t, s, UserMessage, &UserMessageData{Content: "q1", Source: "user"})
	appendSurface(t, s, AssistantMessage, &AssistantMessageData{Content: "", ToolCalls: []*proto.ToolCall{{Id: "c1", Name: "x"}}})
	appendSurface(t, s, ToolResult, &ToolResultData{CallID: "c1", Content: "r1"})
	appendSurface(t, s, AssistantMessage, &AssistantMessageData{Content: "a1"})

	// limit=2 落在当前轮内 → 回退到 q1（当前轮 user 边界），保留整个当前轮
	msgs := s.DeriveMessagesLimited("sys", 2)
	if len(msgs) != 1+4 {
		t.Fatalf("当前轮应完整保留（q1..a1 共 4 条）, 实际 %d 条", len(msgs))
	}
	if msgs[1].Role != "user" || msgs[1].Content != "q1" {
		t.Fatalf("首条应为当前轮 user(q1), 实际 (%s, %q)", msgs[1].Role, msgs[1].Content)
	}
}

// TestFoldHistoryLimit 折叠 history/limit 事件：最后一条生效；无记录返回 (-1, false)。
func TestFoldHistoryLimit(t *testing.T) {
	s := New()
	limit, found := FoldHistoryLimit(s.Events())
	if found || limit != -1 {
		t.Fatalf("无记录: limit=%d found=%v, 期望 -1/false", limit, found)
	}

	s.Append(HistoryLimit, &HistoryLimitData{Count: 10}, nil)
	s.Append(HistoryLimit, &HistoryLimitData{Count: 0}, nil)
	s.Append(HistoryLimit, &HistoryLimitData{Count: 5}, nil)
	limit, found = FoldHistoryLimit(s.Events())
	if !found || limit != 5 {
		t.Fatalf("最后一条生效: limit=%d found=%v, 期望 5/true", limit, found)
	}
}
