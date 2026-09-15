package core

import (
	"errors"
	"testing"
)

// TestClassifyLLMErrorCode 稳定错误码分类（llm/attempt 与运行日志留痕用）：
// 重试协议不受此分类影响（agent/request-error 仍按 isContextWindowExceeded 判定）。
func TestClassifyLLMErrorCode(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{nil, ""},
		{errors.New("prompt is too long: context length exceeded"), "context_window_exceeded"},
		{errors.New("maximum context window reached"), "context_window_exceeded"},
		{errors.New("HTTP 429: Too Many Requests"), "rate_limited"},
		{errors.New("rate limit exceeded, retry later"), "rate_limited"},
		{errors.New("dial tcp: connection refused"), "network_error"},
		{errors.New("Client.Timeout exceeded while awaiting headers"), "network_error"},
		{errors.New("read: connection reset by peer"), "network_error"},
		{errors.New("invalid api key"), "unknown"},
	}
	for _, c := range cases {
		if got := ClassifyLLMErrorCode(c.err); got != c.want {
			t.Errorf("ClassifyLLMErrorCode(%v) = %q, 期望 %q", c.err, got, c.want)
		}
	}
}
