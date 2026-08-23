package main

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// eventFromJSON 按 SDK 测试惯例用 JSON 字符串构造流事件。
func eventFromJSON(t *testing.T, s string) anthropic.MessageStreamEventUnion {
	t.Helper()
	var ev anthropic.MessageStreamEventUnion
	if err := ev.UnmarshalJSON([]byte(s)); err != nil {
		t.Fatalf("unmarshal event %q: %v", s, err)
	}
	return ev
}

// accumulateAll 按序累积一组事件，断言过程中不报错。
func accumulateAll(t *testing.T, acc *streamAccumulator, events []string) {
	t.Helper()
	for _, s := range events {
		if err := acc.accumulate(eventFromJSON(t, s)); err != nil {
			t.Fatalf("accumulate %q: %v", s, err)
		}
	}
}

// assertContent 断言累积结果的 Content 与期望块数组一致（SDK 测试同款 marshal 对比）。
func assertContent(t *testing.T, acc *streamAccumulator, want []anthropic.ContentBlockUnion) {
	t.Helper()
	got, err := json.Marshal(acc.msg.Content)
	if err != nil {
		t.Fatal(err)
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(wantJSON) {
		t.Fatalf("content mismatch:\n got: %s\nwant: %s", got, wantJSON)
	}
}

func TestStreamAccumulatorNormalOrder(t *testing.T) {
	acc := newStreamAccumulator()
	accumulateAll(t, acc, []string{
		`{"type": "message_start", "message": {}}`,
		`{"type": "content_block_start", "index": 0, "content_block": {"type": "text", "text": ""}}`,
		`{"type": "content_block_delta", "index": 0, "delta": {"type": "text_delta", "text": "Hello"}}`,
		`{"type": "content_block_stop", "index": 0}`,
		`{"type": "content_block_start", "index": 1, "content_block": {"type": "tool_use", "id": "toolu_1", "name": "get_weather", "input": {}}}`,
		`{"type": "content_block_stop", "index": 1}`,
		`{"type": "message_stop"}`,
	})
	assertContent(t, acc, []anthropic.ContentBlockUnion{
		{Type: "text", Text: "Hello"},
		{Type: "tool_use", ID: "toolu_1", Name: "get_weather", Input: []byte(`{}`)},
	})
}

// TestStreamAccumulatorRepeatedStart 对应原有容错：重复 content_block_start 不中断流。
func TestStreamAccumulatorRepeatedStart(t *testing.T) {
	acc := newStreamAccumulator()
	accumulateAll(t, acc, []string{
		`{"type": "message_start", "message": {}}`,
		`{"type": "content_block_start", "index": 0, "content_block": {"type": "text", "text": ""}}`,
		// 重复的 start(index=0)：应被忽略，块不重复追加
		`{"type": "content_block_start", "index": 0, "content_block": {"type": "text", "text": "dup"}}`,
		`{"type": "content_block_delta", "index": 0, "delta": {"type": "text_delta", "text": "Hi"}}`,
		`{"type": "content_block_stop", "index": 0}`,
		`{"type": "message_stop"}`,
	})
	assertContent(t, acc, []anthropic.ContentBlockUnion{{Type: "text", Text: "Hi"}})
}

// TestStreamAccumulatorOutOfOrderStart 对应本次故障：index 3 的 start 先于 index 2 到达，
// 修复前 SDK 严格校验报 "expected index 2" 中断整条流。
func TestStreamAccumulatorOutOfOrderStart(t *testing.T) {
	acc := newStreamAccumulator()
	accumulateAll(t, acc, []string{
		`{"type": "message_start", "message": {}}`,
		`{"type": "content_block_start", "index": 0, "content_block": {"type": "thinking", "thinking": ""}}`,
		`{"type": "content_block_delta", "index": 0, "delta": {"type": "thinking_delta", "thinking": "Let me think."}}`,
		`{"type": "content_block_stop", "index": 0}`,
		`{"type": "content_block_start", "index": 1, "content_block": {"type": "thinking", "thinking": ""}}`,
		`{"type": "content_block_delta", "index": 1, "delta": {"type": "thinking_delta", "thinking": "More thinking."}}`,
		`{"type": "content_block_stop", "index": 1}`,
		// 乱序：index 3 先于 index 2
		`{"type": "content_block_start", "index": 3, "content_block": {"type": "tool_use", "id": "toolu_1", "name": "get_weather", "input": {}}}`,
		`{"type": "content_block_start", "index": 2, "content_block": {"type": "text", "text": ""}}`,
		`{"type": "content_block_delta", "index": 2, "delta": {"type": "text_delta", "text": "The weather is nice."}}`,
		`{"type": "content_block_stop", "index": 2}`,
		`{"type": "content_block_delta", "index": 3, "delta": {"type": "input_json_delta", "partial_json": "{\"city\": "}}`,
		`{"type": "content_block_delta", "index": 3, "delta": {"type": "input_json_delta", "partial_json": "\"LA\"}"}}`,
		`{"type": "content_block_stop", "index": 3}`,
		`{"type": "message_delta", "delta": {"stop_reason": "tool_use"}}`,
		`{"type": "message_stop"}`,
	})
	assertContent(t, acc, []anthropic.ContentBlockUnion{
		{Type: "thinking", Thinking: "Let me think."},
		{Type: "thinking", Thinking: "More thinking."},
		{Type: "text", Text: "The weather is nice."},
		{Type: "tool_use", ID: "toolu_1", Name: "get_weather", Input: []byte(`{"city":"LA"}`)},
	})
	if got := extractToolCalls(acc.msg.Content); len(got) != 1 || got[0].Name != "get_weather" || got[0].ID != "toolu_1" {
		t.Fatalf("tool calls mismatch: %+v", got)
	}
	if acc.msg.StopReason != "tool_use" {
		t.Fatalf("stop reason mismatch: %q", acc.msg.StopReason)
	}
}

// TestStreamAccumulatorDeltaBeforeStart 覆盖 start 缺失/延后时 delta 先到的极端乱序：
// delta 暂存，start 到达后补入。
func TestStreamAccumulatorDeltaBeforeStart(t *testing.T) {
	acc := newStreamAccumulator()
	accumulateAll(t, acc, []string{
		`{"type": "message_start", "message": {}}`,
		`{"type": "content_block_delta", "index": 0, "delta": {"type": "text_delta", "text": "Hi"}}`,
		`{"type": "content_block_start", "index": 0, "content_block": {"type": "text", "text": ""}}`,
		`{"type": "content_block_stop", "index": 0}`,
		`{"type": "message_stop"}`,
	})
	assertContent(t, acc, []anthropic.ContentBlockUnion{{Type: "text", Text: "Hi"}})
}

// TestStreamAccumulatorMissingStart 覆盖缺失块始终未到达：暂存尾部被丢弃而非中断流，
// 已确认的块保持完整。
func TestStreamAccumulatorMissingStart(t *testing.T) {
	acc := newStreamAccumulator()
	accumulateAll(t, acc, []string{
		`{"type": "message_start", "message": {}}`,
		`{"type": "content_block_start", "index": 0, "content_block": {"type": "text", "text": ""}}`,
		`{"type": "content_block_delta", "index": 0, "delta": {"type": "text_delta", "text": "Hello"}}`,
		`{"type": "content_block_stop", "index": 0}`,
		// index 1 的 start 永远缺失，index 2 的块暂存后随流结束丢弃
		`{"type": "content_block_start", "index": 2, "content_block": {"type": "text", "text": ""}}`,
		`{"type": "content_block_delta", "index": 2, "delta": {"type": "text_delta", "text": "world"}}`,
		`{"type": "message_stop"}`,
	})
	assertContent(t, acc, []anthropic.ContentBlockUnion{{Type: "text", Text: "Hello"}})
}

// TestStreamAccumulatorRepeatedStartGap 覆盖乱序 + 重复叠加：index 3 先到暂存，
// index 2 到达后按序补入，期间 index 3 的重复 start 不产生重复块。
func TestStreamAccumulatorRepeatedStartGap(t *testing.T) {
	acc := newStreamAccumulator()
	accumulateAll(t, acc, []string{
		`{"type": "message_start", "message": {}}`,
		`{"type": "content_block_start", "index": 0, "content_block": {"type": "text", "text": ""}}`,
		`{"type": "content_block_delta", "index": 0, "delta": {"type": "text_delta", "text": "Hi"}}`,
		`{"type": "content_block_stop", "index": 0}`,
		`{"type": "content_block_start", "index": 2, "content_block": {"type": "tool_use", "id": "toolu_1", "name": "echo", "input": {}}}`,
		`{"type": "content_block_start", "index": 2, "content_block": {"type": "tool_use", "id": "toolu_1", "name": "echo", "input": {}}}`,
		`{"type": "content_block_start", "index": 1, "content_block": {"type": "text", "text": ""}}`,
		`{"type": "content_block_delta", "index": 1, "delta": {"type": "text_delta", "text": "Again"}}`,
		`{"type": "content_block_stop", "index": 1}`,
		`{"type": "content_block_stop", "index": 2}`,
		`{"type": "message_stop"}`,
	})
	assertContent(t, acc, []anthropic.ContentBlockUnion{
		{Type: "text", Text: "Hi"},
		{Type: "text", Text: "Again"},
		{Type: "tool_use", ID: "toolu_1", Name: "echo", Input: []byte(`{}`)},
	})
}
