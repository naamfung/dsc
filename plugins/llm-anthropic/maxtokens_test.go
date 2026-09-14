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
