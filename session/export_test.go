package session

import (
	"strings"
	"testing"

	"dsc/proto"
)

func TestExportTranscriptFull(t *testing.T) {
	s := New()
	s.Append(TurnStart, &TurnData{Turn: 1}, nil)
	appendSurface(t, s, UserMessage, &UserMessageData{Content: "hello", Source: "user"})
	s.Append(StepStart, &StepData{Turn: 1, Step: 1}, nil)
	appendSurface(t, s, AssistantMessage, &AssistantMessageData{
		Content:   "let me check",
		Reasoning: "need to look",
		ToolCalls: []*proto.ToolCall{{Id: "t1", Name: "plain-tool", ArgumentsJson: `{"q":1}`}},
	})
	s.Append(ToolCallEvent, &ToolCallData{Turn: 1, Step: 1, CallID: "t1", Name: "plain-tool", Arguments: `{"q":1}`}, nil)
	appendSurface(t, s, ToolResult, &ToolResultData{Turn: 1, Step: 1, CallID: "t1", Content: "result-x"})
	s.Append(StepEnd, &StepData{Turn: 1, Step: 1}, nil)
	appendSurface(t, s, CompactionSummary, &CompactionSummaryData{Content: "compressed summary"})
	s.Append(TurnEnd, &TurnData{Turn: 1, Reason: "completed"}, nil)

	out := s.ExportTranscript()
	for _, want := range []string{
		"会话导出",
		"轮次 1",
		"**用户**: hello",
		"**助手**: let me check",
		"> 思考：need to look",
		"plain-tool",
		"→ 工具结果(t1): result-x",
		"**摘要（上下文压缩）**: compressed summary",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("transcript missing %q:\n%s", want, out)
		}
	}
}

func TestExportTranscriptEmpty(t *testing.T) {
	s := New()
	if out := s.ExportTranscript(); !strings.Contains(out, "空会话") {
		t.Fatalf("empty session transcript = %q", out)
	}
}

func TestExportTranscriptSkipsChunks(t *testing.T) {
	s := New()
	s.Append(TurnStart, &TurnData{Turn: 1}, nil)
	appendSurface(t, s, UserMessage, &UserMessageData{Content: "hi", Source: "user"})
	// log-only chunk 不应出现在 transcript
	s.Append(AssistantChunk, &AssistantChunkData{Turn: 1, Step: 1, Content: "incremental"}, nil)
	appendSurface(t, s, AssistantMessage, &AssistantMessageData{Content: "final"})
	s.Append(TurnEnd, &TurnData{Turn: 1, Reason: "completed"}, nil)

	out := s.ExportTranscript()
	if strings.Contains(out, "incremental") {
		t.Fatalf("chunk should be skipped in transcript:\n%s", out)
	}
	if !strings.Contains(out, "final") {
		t.Fatalf("assistant message missing:\n%s", out)
	}
}
