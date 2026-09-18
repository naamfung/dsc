package main

import (
	"testing"

	dsc "dsc-sdk"
)

// TestMaxTokensFromEnvPrecedence env 解析优先级：显式 OPENAI_MAX_OUTPUT_TOKENS
// 压过宿主注入 DSC_MAX_OUTPUT_TOKENS；仅注入时用注入值；均缺席为 0（不携带）。
// 经 dsc.LoadLLMConfig("openai") 统一入口解析（sdk/llm_config.go）。
func TestMaxTokensFromEnvPrecedence(t *testing.T) {
	t.Setenv("OPENAI_MAX_OUTPUT_TOKENS", "8192")
	t.Setenv("DSC_MAX_OUTPUT_TOKENS", "131072")
	cfg := dsc.LoadLLMConfig("openai")
	if got := cfg.MaxOutputTokens; got != 8192 {
		t.Fatalf("显式 env 未压过宿主注入: got %d", got)
	}
}

func TestMaxTokensFromEnvHostInjection(t *testing.T) {
	t.Setenv("OPENAI_MAX_OUTPUT_TOKENS", "")
	t.Setenv("DSC_MAX_OUTPUT_TOKENS", "131072")
	cfg := dsc.LoadLLMConfig("openai")
	if got := cfg.MaxOutputTokens; got != 131072 {
		t.Fatalf("宿主注入未被采用: got %d", got)
	}
}

func TestMaxTokensFromEnvAbsentOrInvalid(t *testing.T) {
	t.Setenv("OPENAI_MAX_OUTPUT_TOKENS", "")
	t.Setenv("DSC_MAX_OUTPUT_TOKENS", "")
	cfg := dsc.LoadLLMConfig("openai")
	if got := cfg.MaxOutputTokens; got != 0 {
		t.Fatalf("缺席应返回 0（不携带）: got %d", got)
	}
	t.Setenv("DSC_MAX_OUTPUT_TOKENS", "abc")
	cfg = dsc.LoadLLMConfig("openai")
	if got := cfg.MaxOutputTokens; got != 0 {
		t.Fatalf("非法值应返回 0: got %d", got)
	}
}

// TestResolveMaxTokens 请求级参数（压缩等场景的窗口净余值）优先于插件级默认。
func TestResolveMaxTokens(t *testing.T) {
	p := &OpenAIProvider{maxTokens: 131072}
	if got := p.resolveMaxTokens(26000); got != 26000 {
		t.Fatalf("请求级参数未被优先: got %d", got)
	}
	if got := p.resolveMaxTokens(0); got != 131072 {
		t.Fatalf("插件级默认未被回填: got %d", got)
	}
	zero := &OpenAIProvider{}
	if got := zero.resolveMaxTokens(0); got != 0 {
		t.Fatalf("无任何配置应返回 0（不携带）: got %d", got)
	}
}
