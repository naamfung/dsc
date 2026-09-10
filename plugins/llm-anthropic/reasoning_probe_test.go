package main

import (
	"context"
	"os"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"dsc/core"
)

// TestReasoningProbe 是临时运行时探针：真实调用 ChatStream 检查思考帧是否产出。
// 运行：go test ./plugins/llm-anthropic -run TestReasoningProbe -v
// 完成后应删除本文件。
func TestReasoningProbe(t *testing.T) {
	os.Setenv("ANTHROPIC_BASE_URL", "http://192.168.124.197:8008/")
	os.Setenv("ANTHROPIC_MODEL", "Agentic-Turbo-Coder")
	os.Setenv("ANTHROPIC_API_KEY", "sk-your-real-key")
	// 保持默认开启
	os.Unsetenv("ANTHROPIC_THINKING")

	p := &AnthropicProvider{
		client:         anthropic.NewClient(option.WithAPIKey("sk-your-real-key"), option.WithBaseURL("http://192.168.124.197:8008/")),
		model:          "Agentic-Turbo-Coder",
		thinking:       true,
		thinkingBudget: 4096,
	}

	messages := []core.Message{{Role: "user", Content: "3+4=? reply short"}}
	ch, err := p.ChatStream(context.Background(), messages, nil)
	if err != nil {
		t.Fatalf("ChatStream err: %v", err)
	}
	reasonTotal := 0
	contentTotal := 0
	for fr := range ch {
		if fr.Error != "" {
			t.Logf("frame error: %s", fr.Error)
			continue
		}
		if fr.Reasoning != "" {
			reasonTotal++
			t.Logf("REASON frame[%d] len=%d head=%q", reasonTotal, len(fr.Reasoning), firstRune(fr.Reasoning, 30))
		}
		if fr.Content != "" {
			contentTotal++
			t.Logf("CONTENT frame[%d] len=%d", contentTotal, len(fr.Content))
		}
		if fr.FinishReason != "" {
			t.Logf("FINISH reason=%q", fr.FinishReason)
		}
	}
	t.Logf("TOTAL frames: reason=%d content=%d", reasonTotal, contentTotal)
}

func firstRune(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		r = r[:n]
	}
	return string(r)
}
