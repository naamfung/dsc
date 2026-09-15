package session

import (
	"path/filepath"
	"strings"
	"testing"

	"dsc/proto"
)

// 本文件回归「LLM 调用结算留痕」：llm/attempt（log-only）成败皆录，
// 是排障一手证据——截断（finish_reason=max_tokens）、provider 报错、
// 上下文溢出、流中断、耗时与 token 消耗一眼可查，无需审计代码。

func TestLLMAttemptRoundTrip(t *testing.T) {
	s := New()
	s.Append(TurnStart, &TurnData{Turn: 1}, nil)
	s.Append(LLMAttempt, &LLMAttemptData{
		Turn: 1, Step: 1,
		Streaming:    true,
		FinishReason: "max_tokens",
		Usage:        &proto.Usage{PromptTokens: 100, CompletionTokens: 4096, TotalTokens: 4196},
		DurationMS:   12345,
		ContentChars: 8192,
	}, nil)
	s.Append(LLMAttempt, &LLMAttemptData{
		Turn: 1, Step: 2,
		Streaming:  false,
		DurationMS: 5,
		Error:      "connection refused",
		Code:       "network_error",
	}, nil)

	path := filepath.Join(t.TempDir(), "s.jsonl")
	if err := s.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	var got []*LLMAttemptData
	for _, ev := range loaded.Events() {
		if ev.Type != LLMAttempt {
			continue
		}
		d, ok := ev.Data.(*LLMAttemptData)
		if !ok {
			t.Fatalf("event %d: data type %T, want *LLMAttemptData", ev.Seq, ev.Data)
		}
		got = append(got, d)
	}
	if len(got) != 2 {
		t.Fatalf("llm/attempt 事件数 = %d, 期望 2", len(got))
	}
	if got[0].FinishReason != "max_tokens" || !got[0].Streaming || got[0].DurationMS != 12345 ||
		got[0].Usage == nil || got[0].Usage.CompletionTokens != 4096 || got[0].ContentChars != 8192 {
		t.Fatalf("成功留痕字段不符: %+v", got[0])
	}
	if got[0].Error != "" || got[0].Code != "" {
		t.Fatalf("成功留痕不应携带错误: %+v", got[0])
	}
	if got[1].Streaming || got[1].Error != "connection refused" || got[1].Code != "network_error" {
		t.Fatalf("失败留痕字段不符: %+v", got[1])
	}
}

// TestLLMAttemptLogOnly llm/attempt 不进入 surface（不参与派生模型历史），
// 与 DSH 的 log-only 事件语义一致。
func TestLLMAttemptLogOnly(t *testing.T) {
	s := New()
	s.Append(UserMessage, &UserMessageData{Content: "hi", Source: "user"}, &SurfaceOp{Op: SurfaceAppend})
	n := len(s.SurfaceNodes())
	s.Append(LLMAttempt, &LLMAttemptData{Turn: 1, Step: 1, FinishReason: "stop"}, nil)
	if len(s.SurfaceNodes()) != n {
		t.Fatalf("llm/attempt 进入 surface，应为 log-only")
	}
	if surfaceEventTypes[LLMAttempt] {
		t.Fatalf("llm/attempt 不应登记为 surface 类型")
	}
}

func TestExportTranscriptRendersLLMAttempt(t *testing.T) {
	s := New()
	s.Append(TurnStart, &TurnData{Turn: 1}, nil)
	s.Append(LLMAttempt, &LLMAttemptData{
		Turn: 1, Step: 1, Streaming: true,
		FinishReason: "stop", DurationMS: 800,
		Usage: &proto.Usage{PromptTokens: 10, CompletionTokens: 20, TotalTokens: 30},
	}, nil)
	s.Append(LLMAttempt, &LLMAttemptData{
		Turn: 1, Step: 2, Streaming: false,
		DurationMS: 5, Error: "rate limit exceeded", Code: "rate_limited",
	}, nil)
	out := s.ExportTranscript()
	if !strings.Contains(out, "LLM 流式调用") || !strings.Contains(out, "finish=stop") {
		t.Fatalf("导出未渲染成功留痕:\n%s", out)
	}
	if !strings.Contains(out, "tokens{10/20/30}") {
		t.Fatalf("导出未渲染用量:\n%s", out)
	}
	if !strings.Contains(out, "rate limit exceeded") || !strings.Contains(out, "rate_limited") {
		t.Fatalf("导出未渲染失败留痕:\n%s", out)
	}
}

// TestMigrateV2ToV3Noop v2→v3 系纯增量（新增 llm/attempt 诊断类型），
// 迁移必须为 no-op；v2 头文件可被当前版本正常检测并迁移。
func TestMigrateV2ToV3Noop(t *testing.T) {
	events := []*Event{
		{Seq: 1, Type: TurnStart, Data: &TurnData{Turn: 1}},
		{Seq: 2, Type: LLMAttempt, Data: &LLMAttemptData{Turn: 1, Step: 1}},
	}
	got, err := MigrateToCurrent(2, events)
	if err != nil {
		t.Fatalf("migrate v2→v3: %v", err)
	}
	if len(got) != 2 || got[1].Type != LLMAttempt {
		t.Fatalf("迁移结果不符: %+v", got)
	}
}
