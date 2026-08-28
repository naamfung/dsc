package session

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/toon-format/toon-go"

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
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for unknown surface op")
		}
	}()
	s := New()
	s.Append(UserMessage, &UserMessageData{Content: "u"}, &SurfaceOp{Op: "bogus"})
}

// mustEqualJSONDecode 断言 toon 解码结果与原 JSON 解码结构等价（往返等价）。
func mustEqualJSONDecode(t *testing.T, jsonStr, toonStr string) {
	t.Helper()
	var want any
	if err := json.Unmarshal([]byte(jsonStr), &want); err != nil {
		t.Fatalf("bad fixture json: %v", err)
	}
	got, err := toon.Decode([]byte(toonStr))
	if err != nil {
		t.Fatalf("toon.Decode(%q) failed: %v", toonStr, err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("toon decode != json decode\n got: %#v\nwant: %#v", got, want)
	}
}

func TestToonizeToolContentStructuredAndEquivalent(t *testing.T) {
	in := `{"users":[{"id":1,"name":"Alice","role":"admin"},{"id":2,"name":"Bob","role":"user"}]}`
	got := toonizeToolContent(in)
	if got == in {
		t.Fatal("structured JSON tool result was not compacted to TOON")
	}
	// 信息等价：TOON 表示必须能还原为原 JSON 的语义结构。
	mustEqualJSONDecode(t, in, got)
	// 确实更紧凑（token 目标）。
	if len(got) >= len(in) {
		t.Fatalf("TOON result not more compact: len(toon)=%d >= len(json)=%d", len(got), len(in))
	}
}

func TestToonizeToolContentNested(t *testing.T) {
	in := `{"orders":[{"id":1,"customer":{"name":"Ada","country":"UK"},"total":9.5}]}`
	got := toonizeToolContent(in)
	if got == in {
		t.Fatal("nested structured JSON was not compacted")
	}
	mustEqualJSONDecode(t, in, got)
}

func TestToonizeToolContentTextUnchanged(t *testing.T) {
	in := "plain text tool result, not JSON"
	if got := toonizeToolContent(in); got != in {
		t.Fatalf("plain text should stay verbatim, got %q", got)
	}
}

func TestToonizeToolContentScalarUnchanged(t *testing.T) {
	// 标量 / null 无表式结构，转换无益，须原样保留。
	for _, in := range []string{`"hello"`, `42`, `true`, `null`} {
		if got := toonizeToolContent(in); got != in {
			t.Errorf("scalar %q should stay verbatim, got %q", in, got)
		}
	}
}

func TestToonizeToolContentInvalidJSONUnchanged(t *testing.T) {
	for _, in := range []string{`{broken`, `[1, 2`, ``} {
		if got := toonizeToolContent(in); got != in {
			t.Errorf("invalid json %q should stay verbatim, got %q", in, got)
		}
	}
}

func TestToonizeToolContentDeterministic(t *testing.T) {
	in := `{"b":{"x":1,"w":0},"a":{"z":3,"y":2}}`
	first := toonizeToolContent(in)
	second := toonizeToolContent(in)
	if first != second {
		t.Fatalf("TOON encoding not deterministic:\n %q\n %q", first, second)
	}
}

func TestDeriveToolResultIsToonized(t *testing.T) {
	s := New()
	appendSurface(t, s, UserMessage, &UserMessageData{Content: "q1", Source: "user"})
	raw := `{"ok":true,"count":3}`
	appendSurface(t, s, ToolResult, &ToolResultData{CallID: "c1", Content: raw})

	msgs := s.DeriveMessages("")
	if len(msgs) != 2 {
		t.Fatalf("derived %d messages, want 2 (user+tool)", len(msgs))
	}
	tool := msgs[1]
	if tool.Role != "tool" || tool.ToolCallId != "c1" {
		t.Fatalf("unexpected tool message: role=%s id=%s", tool.Role, tool.ToolCallId)
	}
	// 事件日志原文无损，但派生消息 Content 为 TOON 紧凑形式。
	if ev := s.Events(); ev[1].Data.(*ToolResultData).Content != raw {
		t.Fatalf("event log must preserve raw JSON, got %q", ev[1].Data.(*ToolResultData).Content)
	}
	if tool.Content == raw {
		t.Fatal("derived tool message should carry TOON-ized content, not raw JSON")
	}
	mustEqualJSONDecode(t, raw, tool.Content)
}
