package session

import (
	"testing"

	"dsc/proto"
)

// helper 构造一条 surface append 事件。
func appendSurface(t *testing.T, s *Session, typ EventType, data any) *Event {
	t.Helper()
	return s.Append(typ, data, &SurfaceOp{Op: SurfaceAppend})
}

func TestSeqContiguity(t *testing.T) {
	s := New()
	for i := 0; i < 5; i++ {
		ev := appendSurface(t, s, UserMessage, &UserMessageData{Content: "hi", Source: "user"})
		if ev.Seq != i {
			t.Fatalf("event %d seq = %d, want %d", i, ev.Seq, i)
		}
	}
	if s.Len() != 5 {
		t.Fatalf("len = %d, want 5", s.Len())
	}
}

func TestDeriveMessagesBasic(t *testing.T) {
	s := New()
	appendSurface(t, s, UserMessage, &UserMessageData{Content: "q1", Source: "user"})
	appendSurface(t, s, AssistantMessage, &AssistantMessageData{Content: "a1"})
	appendSurface(t, s, ToolResult, &ToolResultData{CallID: "c1", Content: "r1"})

	msgs := s.DeriveMessages("sys")
	if len(msgs) != 4 {
		t.Fatalf("derived %d messages, want 4 (system+3)", len(msgs))
	}
	want := []struct{ role, content string }{
		{"system", "sys"}, {"user", "q1"}, {"assistant", "a1"}, {"tool", "r1"},
	}
	for i, w := range want {
		if msgs[i].Role != w.role || msgs[i].Content != w.content {
			t.Errorf("msg %d = (%s, %q), want (%s, %q)", i, msgs[i].Role, msgs[i].Content, w.role, w.content)
		}
	}
}

func TestDeriveSkipsLogOnly(t *testing.T) {
	s := New()
	// log-only 事件（无 surface）不得投影为消息
	s.Append(TurnStart, &TurnData{Turn: 1}, nil)
	s.Append(AssistantChunk, &AssistantChunkData{Turn: 1, Step: 1, Content: "chunk"}, nil)
	appendSurface(t, s, UserMessage, &UserMessageData{Content: "q1", Source: "user"})

	msgs := s.DeriveMessages("sys")
	if len(msgs) != 2 { // system + user
		t.Fatalf("derived %d messages, want 2 (turn/chunk skipped)", len(msgs))
	}
}

func TestDeriveAssistantCarriesToolCalls(t *testing.T) {
	s := New()
	tc := &proto.ToolCall{Id: "t1", Name: "tool-a", ArgumentsJson: `{"x":1}`}
	appendSurface(t, s, AssistantMessage, &AssistantMessageData{
		Content:   "",
		ToolCalls: []*proto.ToolCall{tc},
	})
	msgs := s.DeriveMessages("")
	if len(msgs) != 1 {
		t.Fatalf("derived %d messages, want 1", len(msgs))
	}
	if len(msgs[0].ToolCalls) != 1 || msgs[0].ToolCalls[0].Name != "tool-a" {
		t.Fatalf("tool calls lost: %+v", msgs[0].ToolCalls)
	}
}

func TestDeriveSkipsEmptyAssistant(t *testing.T) {
	s := New()
	appendSurface(t, s, AssistantMessage, &AssistantMessageData{Content: "", Usage: &proto.Usage{TotalTokens: 5}})
	msgs := s.DeriveMessages("")
	if len(msgs) != 0 {
		t.Fatalf("empty assistant (no tool calls) should be skipped, got %d", len(msgs))
	}
}

func TestSurfaceReplaceShadowsRange(t *testing.T) {
	s := New()
	appendSurface(t, s, UserMessage, &UserMessageData{Content: "u1", Source: "user"})
	appendSurface(t, s, UserMessage, &UserMessageData{Content: "u2", Source: "user"})
	appendSurface(t, s, UserMessage, &UserMessageData{Content: "u3", Source: "user"})
	appendSurface(t, s, UserMessage, &UserMessageData{Content: "u4", Source: "user"})
	// 压缩：摘要 replace 遮蔽 seq 1..2（u2、u3）
	summary := s.Append(CompactionSummary, &CompactionSummaryData{Content: "sum"}, &SurfaceOp{
		Op: SurfaceReplace, Start: 1, End: 2,
	})

	msgs := s.DeriveMessages("sys")
	if len(msgs) != 4 { // system + u1 + sum + u4
		t.Fatalf("derived %d messages, want 4 after replace", len(msgs))
	}
	want := []string{"u1", "sum", "u4"}
	for i, w := range want {
		if msgs[i+1].Content != w {
			t.Errorf("derived msg %d = %q, want %q", i+1, msgs[i+1].Content, w)
		}
	}
	// 日志本体无损：所有事件仍在
	if s.Len() != 5 {
		t.Fatalf("log len = %d, want 5 (events preserved)", s.Len())
	}
	// replace 遮蔽的节点正是声明范围
	nodes := s.SurfaceNodes()
	wantNodes := []int{0, summary.Seq, 3}
	if len(nodes) != len(wantNodes) {
		t.Fatalf("surface nodes = %v, want %v", nodes, wantNodes)
	}
	for i, n := range wantNodes {
		if nodes[i] != n {
			t.Fatalf("surface node %d = %d, want %d (nodes=%v)", i, nodes[i], n, nodes)
		}
	}
}

func TestReplaceGenerationIncrements(t *testing.T) {
	s := New()
	appendSurface(t, s, UserMessage, &UserMessageData{Content: "u1", Source: "user"})
	if g := s.ReplaceGeneration(); g != 0 {
		t.Fatalf("replace generation = %d, want 0", g)
	}
	s.Append(CompactionSummary, &CompactionSummaryData{Content: "s"}, &SurfaceOp{Op: SurfaceReplace, Start: 0, End: 0})
	if g := s.ReplaceGeneration(); g != 1 {
		t.Fatalf("replace generation = %d, want 1", g)
	}
}

func TestNonSurfaceCarriesSurfacePanics(t *testing.T) {
	s := New()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for non-surface event with surface op")
		}
	}()
	s.Append(TurnStart, &TurnData{Turn: 1}, &SurfaceOp{Op: SurfaceAppend})
}

func TestUnknownSurfaceOpPanics(t *testing.T) {
	s := New()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for unknown surface op")
		}
	}()
	s.Append(UserMessage, &UserMessageData{Content: "u"}, &SurfaceOp{Op: "bogus"})
}
