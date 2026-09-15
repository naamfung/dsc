package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/option"
)

// captureNext 捕获中间件交给下游的请求体。
func captureNext(out *[]byte) option.MiddlewareNext {
	return func(req *http.Request) (*http.Response, error) {
		b, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		*out = b
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(""))}, nil
	}
}

func runMiddleware(t *testing.T, body string) []byte {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://example.com/v1/messages", bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	var got []byte
	if _, err := omitZeroMaxTokens(req, captureNext(&got)); err != nil {
		t.Fatalf("middleware: %v", err)
	}
	return got
}

// TestOmitZeroMaxTokensStripped 回归「max_tokens 零值必须不上送」：SDK 无 omitempty，
// 不设 MaxTokens 会序列化出 "max_tokens":0——中间件必须摘除该键，其余字段保真。
func TestOmitZeroMaxTokensStripped(t *testing.T) {
	got := runMiddleware(t, `{"model":"m","max_tokens":0,"messages":[],"system":"s"}`)
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(got, &payload); err != nil {
		t.Fatalf("rewritten body not JSON: %v (%s)", err, got)
	}
	if _, ok := payload["max_tokens"]; ok {
		t.Fatalf("max_tokens 零值未被摘除: %s", got)
	}
	for key, want := range map[string]string{"model": `"m"`, "system": `"s"`} {
		if string(payload[key]) != want {
			t.Fatalf("字段 %s 保真失败: got %s want %s", key, payload[key], want)
		}
	}
}

// TestOmitZeroMaxTokensKeepsExplicit 显式配置 >0 的 max_tokens 不受中间件影响。
func TestOmitZeroMaxTokensKeepsExplicit(t *testing.T) {
	got := runMiddleware(t, `{"model":"m","max_tokens":32768,"messages":[]}`)
	if !strings.Contains(string(got), `"max_tokens":32768`) {
		t.Fatalf("显式 max_tokens 被误摘: %s", got)
	}
}

// TestOmitZeroMaxTokensPassthrough 无该字段的请求原样放行；重建后的请求体
// 须经 GetBody 可重读（SDK 重试依赖）。
func TestOmitZeroMaxTokensPassthrough(t *testing.T) {
	got := runMiddleware(t, `{"model":"m","messages":[]}`)
	if string(got) != `{"model":"m","messages":[]}` {
		t.Fatalf("无 max_tokens 的请求体被改动: %s", got)
	}
}

// TestMaxTokensFromEnvPrecedence env 解析优先级：显式 ANTHROPIC_MAX_OUTPUT_TOKENS
// 压过宿主注入 DSC_MAX_OUTPUT_TOKENS；仅注入时用注入值；均缺席为 0（不携带）。
func TestMaxTokensFromEnvPrecedence(t *testing.T) {
	t.Setenv("ANTHROPIC_MAX_OUTPUT_TOKENS", "8192")
	t.Setenv("DSC_MAX_OUTPUT_TOKENS", "131072")
	if got := maxTokensFromEnv(); got != 8192 {
		t.Fatalf("显式 env 未压过宿主注入: got %d", got)
	}
}

func TestMaxTokensFromEnvHostInjection(t *testing.T) {
	t.Setenv("DSC_MAX_OUTPUT_TOKENS", "131072")
	if got := maxTokensFromEnv(); got != 131072 {
		t.Fatalf("宿主注入未被采用: got %d", got)
	}
}

func TestMaxTokensFromEnvAbsent(t *testing.T) {
	// t.Setenv 置空而非 os.Unsetenv：空串与缺席同义（parsePositiveInt64 返回 0）
	t.Setenv("ANTHROPIC_MAX_OUTPUT_TOKENS", "")
	t.Setenv("DSC_MAX_OUTPUT_TOKENS", "")
	if got := maxTokensFromEnv(); got != 0 {
		t.Fatalf("缺席应返回 0（不携带）: got %d", got)
	}
}

func TestMaxTokensFromEnvInvalid(t *testing.T) {
	t.Setenv("DSC_MAX_OUTPUT_TOKENS", "-1")
	if got := maxTokensFromEnv(); got != 0 {
		t.Fatalf("非法值应返回 0: got %d", got)
	}
}

// TestResolveMaxTokens 请求级参数（压缩等场景的窗口净余值）优先于插件级默认。
func TestResolveMaxTokens(t *testing.T) {
	p := &AnthropicProvider{maxTokens: 131072}
	if got := p.resolveMaxTokens(26000); got != 26000 {
		t.Fatalf("请求级参数未被优先: got %d", got)
	}
	if got := p.resolveMaxTokens(0); got != 131072 {
		t.Fatalf("插件级默认未被回填: got %d", got)
	}
	zero := &AnthropicProvider{}
	if got := zero.resolveMaxTokens(0); got != 0 {
		t.Fatalf("无任何配置应返回 0（不携带）: got %d", got)
	}
}
