package main

import (
	dsc "dsc-sdk"
	"strings"
	"testing"
)

// TestInterjectionSystemMessageKeepsSystemPrompt 插播消息（role=system 追加在消息末尾）
// 不得覆盖顶层 system prompt：Anthropic 只接受顶层 system，列表里的后续 system 必须
// 降级为 user 轮并标注来源，否则一次插播会把整套 system prompt 替换掉。
func TestInterjectionSystemMessageKeepsSystemPrompt(t *testing.T) {
	msgs := []dsc.Message{
		{Role: "system", Content: "REAL SYSTEM PROMPT"},
		{Role: "user", Content: "do the task"},
		{Role: "assistant", Content: "working"},
		{Role: "user", Content: "tool output"},
		{Role: "system", Content: "task done, write memory"},
	}
	p := &AnthropicProvider{model: "m", maxTokens: 4096}
	params, _ := p.buildMessageParams(msgs, nil, 4096)

	if got := params.System[0].Text; got != "REAL SYSTEM PROMPT" {
		t.Fatalf("system prompt 被覆盖为 %q", got)
	}
	// 插播内容必须送达模型（并入上一条 user 消息，避免同角色相邻）
	last := params.Messages[len(params.Messages)-1]
	var texts []string
	for _, blk := range last.Content {
		if blk.OfText != nil {
			texts = append(texts, blk.OfText.Text)
		}
	}
	joined := strings.Join(texts, "\n")
	if !strings.Contains(joined, "task done, write memory") {
		t.Fatalf("插播内容应并入末条 user 消息，got %q", joined)
	}
	if !strings.Contains(joined, "[system]") {
		t.Fatalf("插播内容应带来源标注，got %q", joined)
	}
	if last.Role != "user" {
		t.Fatalf("降级后角色应为 user，got %v", last.Role)
	}
}
