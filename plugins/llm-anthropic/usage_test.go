package main

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// TestUsageFromAnthropicCacheSemantics 回归「上下文容量被低估」问题：
// 本地 llama.cpp 等兼容接口启用提示缓存后，input_tokens 仅含未命中缓存的新增 token，
// 已命中前缀单独计入 cache_read_input_tokens（实测第二次请求 input_tokens=4,
// cache_read=173，真实上下文 177）。PromptTokens 必须反映真实上下文长度，
// 否则 TUI 容量显示倒退且自动压缩永不触发。
func TestUsageFromAnthropicCacheSemantics(t *testing.T) {
	// llama.cpp 缓存命中：input 仅新增，cache_read 远大于 input → 需相加
	u := usageFromAnthropic(&anthropic.Usage{InputTokens: 4, OutputTokens: 8, CacheReadInputTokens: 173})
	if u == nil {
		t.Fatal("usageFromAnthropic 返回 nil")
	}
	if u.PromptTokens != 177 {
		t.Fatalf("PromptTokens = %d, 期望 177（input 4 + cache_read 173）", u.PromptTokens)
	}
	if u.TotalTokens != 185 {
		t.Fatalf("TotalTokens = %d, 期望 185", u.TotalTokens)
	}
	if u.CacheReadInputTokens != 173 {
		t.Fatalf("CacheReadInputTokens = %d, 期望 173", u.CacheReadInputTokens)
	}

	// 标准 Anthropic 语义：input_tokens 已含全部输入，cache_read 为其子集 → 保持原值
	u = usageFromAnthropic(&anthropic.Usage{InputTokens: 1000, OutputTokens: 20, CacheReadInputTokens: 500})
	if u.PromptTokens != 1000 {
		t.Fatalf("标准 Anthropic 语义 PromptTokens = %d, 期望 1000（input 已含缓存，不得重复加）", u.PromptTokens)
	}

	// 无缓存（首请求）：cache_read=0 → 保持 input
	u = usageFromAnthropic(&anthropic.Usage{InputTokens: 177, OutputTokens: 5, CacheReadInputTokens: 0})
	if u.PromptTokens != 177 {
		t.Fatalf("无缓存时 PromptTokens = %d, 期望 177", u.PromptTokens)
	}

	// 全缓存命中：input 极小、cache_read 占绝大部分
	u = usageFromAnthropic(&anthropic.Usage{InputTokens: 15, OutputTokens: 60, CacheReadInputTokens: 21000})
	if u.PromptTokens != 21015 {
		t.Fatalf("全缓存命中 PromptTokens = %d, 期望 21015", u.PromptTokens)
	}
}

// TestUsageFromAnthropicZero 全零 usage 返回 nil（保持流结束帧不携带 usage 的旧行为）。
func TestUsageFromAnthropicZero(t *testing.T) {
	if u := usageFromAnthropic(&anthropic.Usage{}); u != nil {
		t.Fatalf("全零 usage 应返回 nil, 得到 %+v", u)
	}
	if usageFromAnthropic(nil) != nil {
		t.Fatal("nil usage 应返回 nil")
	}
}
