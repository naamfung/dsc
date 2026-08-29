package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dsc/proto"
)

// buildSample 构造一个包含全部事件类型、且结束于完整轮次的会话。
func buildSample(t *testing.T) *Session {
	t.Helper()
	s := New()
	s.Append(TurnStart, &TurnData{Turn: 1}, nil)
	appendSurface(t, s, UserMessage, &UserMessageData{Content: "u1", Source: "user"})
	s.Append(StepStart, &StepData{Turn: 1, Step: 1}, nil)
	s.Append(AssistantChunk, &AssistantChunkData{Turn: 1, Step: 1, Content: "hi "}, nil)
	appendSurface(t, s, AssistantMessage, &AssistantMessageData{
		Turn: 1, Step: 1, Content: "hi",
		ToolCalls: []*proto.ToolCall{{Id: "t1", Name: "tool-a", ArgumentsJson: `{"x":1}`}},
		Usage:     &proto.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
	})
	s.Append(ToolCallEvent, &ToolCallData{Turn: 1, Step: 1, CallID: "t1", Name: "tool-a", Arguments: `{"x":1}`}, nil)
	appendSurface(t, s, ToolResult, &ToolResultData{Turn: 1, Step: 1, CallID: "t1", Content: "r1"})
	s.Append(StepEnd, &StepData{Turn: 1, Step: 1}, nil)
	appendSurface(t, s, UserMessage, &UserMessageData{Content: "u2", Source: "user"})
	s.Append(StepStart, &StepData{Turn: 1, Step: 2}, nil)
	appendSurface(t, s, AssistantMessage, &AssistantMessageData{Turn: 1, Step: 2, Content: "a2"})
	s.Append(StepEnd, &StepData{Turn: 1, Step: 2}, nil)
	s.Append(TurnEnd, &TurnData{Turn: 1, Reason: "completed"}, nil)
	return s
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	orig := buildSample(t)
	if err := orig.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	restored, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if restored == nil {
		t.Fatal("load returned nil session")
	}
	if restored.Len() != orig.Len() {
		t.Fatalf("restored len = %d, want %d", restored.Len(), orig.Len())
	}
	// 事件逐一比对（seq/time/type/data/surface）
	oe, re := orig.Events(), restored.Events()
	for i := range oe {
		if re[i].Seq != oe[i].Seq || re[i].Type != oe[i].Type {
			t.Fatalf("event %d mismatch: %+v vs %+v", i, oe[i], re[i])
		}
		if (re[i].Surface == nil) != (oe[i].Surface == nil) {
			t.Fatalf("event %d surface presence mismatch", i)
		}
		if re[i].Surface != nil && (*re[i].Surface != *oe[i].Surface) {
			t.Fatalf("event %d surface = %+v, want %+v", i, re[i].Surface, oe[i].Surface)
		}
	}
	// 派生历史一致（含工具调用）
	om, rm := orig.DeriveMessages("sys"), restored.DeriveMessages("sys")
	if len(om) != len(rm) {
		t.Fatalf("derived %d vs %d messages", len(om), len(rm))
	}
	for i := range om {
		if om[i].Role != rm[i].Role || om[i].Content != rm[i].Content {
			t.Fatalf("derived msg %d mismatch: %+v vs %+v", i, om[i], rm[i])
		}
		if len(om[i].ToolCalls) != len(rm[i].ToolCalls) {
			t.Fatalf("derived msg %d tool calls mismatch", i)
		}
	}
}

func TestLoadMissingFile(t *testing.T) {
	s, err := Load(filepath.Join(t.TempDir(), "nope.jsonl"))
	if err != nil {
		t.Fatalf("load missing: %v", err)
	}
	if s != nil {
		t.Fatalf("expected nil session for missing file, got %+v", s)
	}
}

func TestLoadRejectsNonContiguousSeq(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.jsonl")
	// 手工构造断号日志
	if err := writeLines(path, `{"seq":0,"time":1,"type":"user/message","data":{"content":"u","source":"user"},"surface":{"op":"append"}}`, `{"seq":2,"time":2,"type":"user/message","data":{"content":"u2","source":"user"},"surface":{"op":"append"}}`); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for non-contiguous seq log")
	}
}

func TestLoadRejectsUnknownType(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.jsonl")
	// 首条为合法事件（seq 0），第二条在 seq 1 处放置未知类型事件，
	// 以便验证诊断携带事件名、序列号与原始文件路径（而非先触发非连续校验）。
	seq := 1
	err := writeLines(path,
		`{"seq":0,"time":1,"type":"user/message","data":{"content":"u","source":"user"},"surface":{"op":"append"}}`,
		fmt.Sprintf(`{"seq":%d,"time":1,"type":"future/event","data":{},"surface":null}`, seq))
	if err != nil {
		t.Fatal(err)
	}
	_, err = Load(path)
	if err == nil {
		t.Fatal("expected error for unknown event type")
	}
	msg := err.Error()
	if !strings.Contains(msg, "future/event") {
		t.Fatalf("diagnostic should name the unknown event type, got: %s", msg)
	}
	if !strings.Contains(msg, fmt.Sprintf("seq %d", seq)) {
		t.Fatalf("diagnostic should include the event sequence, got: %s", msg)
	}
	if !strings.Contains(msg, path) {
		t.Fatalf("diagnostic should include the raw artifact path, got: %s", msg)
	}
}

// TestLoadRestoresTodoWrite 回归：todo/write 事件可持久化并恢复。
// 曾因 unmarshalData 漏注册 TodoWrite 类型，导致任何含 todo/write 的会话
// 在恢复/导出时报 unknown event type "todo/write"，进程无法开始对话。
func TestLoadRestoresTodoWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	orig := New()
	orig.Append(TurnStart, &TurnData{Turn: 1}, nil)
	appendSurface(t, orig, UserMessage, &UserMessageData{Content: "u", Source: "user"})
	orig.Append(StepStart, &StepData{Turn: 1, Step: 1}, nil)
	orig.Append(TodoWrite, &TodoWriteData{Todos: []TodoItem{
		{Content: "任务 A", Status: TodoInProgress},
		{Content: "任务 B", Status: TodoPending},
	}}, nil)
	appendSurface(t, orig, AssistantMessage, &AssistantMessageData{Turn: 1, Step: 1, Content: "ok"})
	orig.Append(StepEnd, &StepData{Turn: 1, Step: 1}, nil)
	orig.Append(TurnEnd, &TurnData{Turn: 1, Reason: "completed"}, nil)
	if err := orig.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	restored, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if restored == nil {
		t.Fatal("load returned nil session")
	}
	if restored.Len() != orig.Len() {
		t.Fatalf("restored len = %d, want %d", restored.Len(), orig.Len())
	}
	// todo 投影一致（FoldTodos 从恢复事件重建）
	ot, rt := FoldTodos(orig.Events()), FoldTodos(restored.Events())
	if len(ot) != len(rt) {
		t.Fatalf("folded todos %d vs %d", len(ot), len(rt))
	}
	for i := range ot {
		if ot[i] != rt[i] {
			t.Fatalf("todo %d mismatch: %+v vs %+v", i, ot[i], rt[i])
		}
	}
	// 派生历史一致（todo 为 log-only，不进历史）
	om, rm := orig.DeriveMessages("sys"), restored.DeriveMessages("sys")
	if len(om) != len(rm) {
		t.Fatalf("derived %d vs %d messages", len(om), len(rm))
	}
}

// TestLoadRestoresHistoryLimit 回归：history/limit 事件可持久化并恢复。
// 曾因 unmarshalData 漏注册 HistoryLimit 类型，导致任何含 history/limit 的会话
// 第二次启动恢复时（加载历史注入设置）报 unknown event type "history/limit"。
func TestLoadRestoresHistoryLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	orig := New()
	orig.Append(TurnStart, &TurnData{Turn: 1}, nil)
	appendSurface(t, orig, UserMessage, &UserMessageData{Content: "u", Source: "user"})
	orig.Append(HistoryLimit, &HistoryLimitData{Count: 4}, nil)
	appendSurface(t, orig, AssistantMessage, &AssistantMessageData{Turn: 1, Step: 1, Content: "ok"})
	orig.Append(TurnEnd, &TurnData{Turn: 1, Reason: "completed"}, nil)
	if err := orig.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	restored, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if restored == nil {
		t.Fatal("load returned nil session")
	}
	if restored.Len() != orig.Len() {
		t.Fatalf("restored len = %d, want %d", restored.Len(), orig.Len())
	}
	// history/limit 折叠一致（从恢复事件重建，最后一条生效）
	ol, of := FoldHistoryLimit(orig.Events())
	rl, rf := FoldHistoryLimit(restored.Events())
	if of != rf || ol != rl {
		t.Fatalf("folded history limit %d/%v vs %d/%v", ol, of, rl, rf)
	}
	// 派生历史一致（history/limit 为 log-only，不进历史）
	om, rm := orig.DeriveMessages("sys"), restored.DeriveMessages("sys")
	if len(om) != len(rm) {
		t.Fatalf("derived %d vs %d messages", len(om), len(rm))
	}
}

func TestForkCopiesPrefix(t *testing.T) {
	orig := buildSample(t)
	// boundary = 最后一个事件（完整轮次前缀）→ 可 fork
	child, err := orig.Fork(orig.Len() - 1)
	if err != nil {
		t.Fatalf("fork: %v", err)
	}
	if child.Len() != orig.Len() {
		t.Fatalf("child len = %d, want %d", child.Len(), orig.Len())
	}
	// 子会话独立追加不影响父会话
	appendSurface(t, child, UserMessage, &UserMessageData{Content: "child-msg", Source: "user"})
	if child.Len() != orig.Len()+1 || orig.Len() != buildSample(t).Len() {
		t.Fatalf("child/orig lens = %d/%d after append, want %d/%d",
			child.Len(), orig.Len(), buildSample(t).Len()+1, buildSample(t).Len())
	}
}

func TestForkRejectsOpenTurn(t *testing.T) {
	orig := buildSample(t)
	// boundary = 8（user u2，轮次仍开放）→ 拒绝
	if _, err := orig.Fork(8); err == nil {
		t.Fatal("expected error for fork ending inside an open turn")
	}
}

func TestForkRejectsOutOfRange(t *testing.T) {
	orig := buildSample(t)
	if _, err := orig.Fork(orig.Len()); err == nil {
		t.Fatal("expected error for out-of-range boundary")
	}
}

func TestLastTurn(t *testing.T) {
	orig := buildSample(t)
	if got := orig.LastTurn(); got != 1 {
		t.Fatalf("LastTurn = %d, want 1", got)
	}
	empty := New()
	if got := empty.LastTurn(); got != 0 {
		t.Fatalf("empty LastTurn = %d, want 0", got)
	}
}

func writeLines(path string, lines ...string) error {
	return os.WriteFile(path, []byte(joinLines(lines...)), 0644)
}

func joinLines(lines ...string) string {
	out := ""
	for i, l := range lines {
		if i > 0 {
			out += "\n"
		}
		out += l
	}
	return out + "\n"
}
