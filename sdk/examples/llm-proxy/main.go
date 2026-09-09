// llm-proxy 示例：最小 LLM 插件（实现 core.LLMProvider）。
// 独立插件经 SDK 声明 LLMProvider 即可，gRPC 服务与元数据（Type "llm"）由
// core.LLMGRPCPlugin 自动提供——插件作者无需接触 proto/go-core。
// 构建：go build -o llm-proxy.exe .  → 放进宿主 plugins/llm-llm-proxy/，
// 并在 config.yaml 的 plugins.llm 下声明。
package main

import (
	"context"
	"fmt"
	"strings"

	"dsc-sdk"
	"dsc/core"
)

// mockLLM 一个回显型 LLM：把提示词拼接后返回，供离线演示与 SDK 冒烟。
type mockLLM struct{}

func (m *mockLLM) Name(ctx context.Context) string    { return "llm-mock" }
func (m *mockLLM) Version(ctx context.Context) string { return "1.0.0" }
func (m *mockLLM) HealthCheck(ctx context.Context) error {
	return nil
}

func (m *mockLLM) Chat(ctx context.Context, messages []core.Message, tools []core.Tool, maxTokens int) (*core.ChatResponse, error) {
	var b strings.Builder
	for _, msg := range messages {
		b.WriteString(fmt.Sprintf("%s: %s\n", msg.Role, msg.Content))
	}
	return &core.ChatResponse{Content: b.String(), FinishReason: "stop"}, nil
}

func (m *mockLLM) ChatStream(ctx context.Context, messages []core.Message, tools []core.Tool) (<-chan *core.ChatStreamResponse, error) {
	ch := make(chan *core.ChatStreamResponse, 2)
	go func() {
		defer close(ch)
		for _, msg := range messages {
			if msg.Content != "" {
				ch <- &core.ChatStreamResponse{Content: msg.Content}
			}
		}
		ch <- &core.ChatStreamResponse{FinishReason: "stop"}
	}()
	return ch, nil
}

func main() {
	sdk := dsc.New(dsc.Config{Name: "llm-mock", Version: "1.0.0", Type: dsc.TypeLLM})
	sdk.LLM(&mockLLM{})
	sdk.Serve()
}
