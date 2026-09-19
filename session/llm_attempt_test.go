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

// TestMigrateV2ToV3PrependsSystemMessage v2→v3 不再是 no-op——v3 引入
// system/message surface node 0 不变量，迁移在 v2 事件序列首部追加空
// system/message 占位节点。LLMAttempt 等 v2 已有事件类型保留不变。
func TestMigrateV2ToV3PrependsSystemMessage(t *testing.T) {
        events := []*Event{
                {Seq: 1, Type: TurnStart, Data: &TurnData{Turn: 1}},
                {Seq: 2, Type: LLMAttempt, Data: &LLMAttemptData{Turn: 1, Step: 1}},
        }
        got, err := MigrateToCurrent(2, events)
        if err != nil {
                t.Fatalf("migrate v2→v3: %v", err)
        }
        if len(got) != 3 {
                t.Fatalf("迁移后应 3 个事件（1 占位 + 2 原始），got %d: %+v", len(got), got)
        }
        if got[0].Type != SystemMessage {
                t.Fatalf("事件 0 应为 SystemMessage 占位，got %q", got[0].Type)
        }
        if d, ok := got[0].Data.(*SystemMessageData); !ok || d.Content != "" {
                t.Fatalf("SystemMessage 占位应空 content，got %+v", got[0].Data)
        }
        if got[0].Surface == nil || got[0].Surface.Op != SurfaceAppend {
                t.Fatalf("SystemMessage 占位应 surface append，got %+v", got[0].Surface)
        }
        // 原始事件保留：TurnStart + LLMAttempt
        if got[1].Type != TurnStart || got[2].Type != LLMAttempt {
                t.Fatalf("原事件类型应保留，got %+v %+v", got[1].Type, got[2].Type)
        }
        // Seq 重排后占位为 0，原事件后移 +1
        if got[1].Seq != 2 || got[2].Seq != 3 {
                t.Fatalf("原事件 Seq 应 +1（重排），got %d %d", got[1].Seq, got[2].Seq)
        }
}
