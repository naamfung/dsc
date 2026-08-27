package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"dsc/core"
	"dsc/proto"
	"dsc/proto/metadata"
	plugin "github.com/hashicorp/go-plugin"
)

// TestE2EWithHostClient 端到端验证 SDK 重写后的 LLM 插件：
// 以宿主侧 go-core 客户端 spawn exe（ANTHROPIC_BASE_URL 指向本地 mock 服务），
// 经 gRPC 验证元数据（type=llm）、Name/Version/HealthCheck、Chat（非流式）
// 与 ChatStream（SSE 流式 + finish_reason + usage）。
func TestE2EWithHostClient(t *testing.T) {
	// 1. mock Anthropic 兼容服务：/v1/messages 支持非流式与流式（SSE）
	var streamFlushed bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			http.NotFound(w, r)
			return
		}
		var req struct {
			Stream bool `json:"stream"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Stream {
			streamFlushed = true
			w.Header().Set("Content-Type", "text/event-stream")
			flusher := w.(http.Flusher)
			write := func(ev, data string) {
				fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev, data)
				flusher.Flush()
			}
			write("message_start", `{"type":"message_start","message":{"id":"msg_01","type":"message","role":"assistant","model":"mock","content":[],"usage":{"input_tokens":5,"output_tokens":1}}}`)
			write("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
			write("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`)
			write("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" from mock"}}`)
			write("content_block_stop", `{"type":"content_block_stop","index":0}`)
			write("message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":7}}`)
			write("message_stop", `{"type":"message_stop"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"msg_01","type":"message","role":"assistant","model":"mock","content":[{"type":"text","text":"非流式回复"}],"stop_reason":"end_turn","usage":{"input_tokens":5,"output_tokens":7,"cache_creation_input_tokens":3,"cache_read_input_tokens":2}}`)
	}))
	defer srv.Close()

	// 2. 构建插件 exe（独立 module 的完整独立开发者路径）
	dir := t.TempDir()
	exe := filepath.Join(dir, "llm-anthropic.exe")
	if out, err := exec.Command("go", "build", "-o", exe, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	// 3. 以宿主侧客户端拉起插件进程（env 指向本地 mock，关闭 thinking 简化流式）
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(),
		"ANTHROPIC_BASE_URL="+srv.URL,
		"ANTHROPIC_API_KEY=test-key",
		"ANTHROPIC_MODEL=mock-model",
		"ANTHROPIC_THINKING=0",
	)
	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig:  core.Handshake,
		Plugins:          map[string]plugin.Plugin{},
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
		Cmd:              cmd,
	})
	defer client.Kill()

	rpcClient, err := client.Client()
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	grpcClient, ok := rpcClient.(*plugin.GRPCClient)
	if !ok {
		t.Fatalf("unexpected client type %T", rpcClient)
	}
	conn := grpcClient.Conn
	ctx := context.Background()

	// 4. 元数据（SDK 自动提供，Type 由 Config 决定）
	meta := metadata.NewPluginMetadataClient(conn)
	info, err := meta.GetInfo(ctx, &metadata.Empty{})
	if err != nil || info.Type != "llm" || info.Name != "anthropic" || info.ApiVersion != "1.0" {
		t.Fatalf("GetInfo = %+v, err %v", info, err)
	}

	// 5. LLMService 基础方法（SDK 复用 core.LLMGRPCPlugin）
	llm := proto.NewLLMServiceClient(conn)
	nm, err := llm.Name(ctx, &proto.NameRequest{})
	if err != nil || nm.Name != "anthropic" {
		t.Fatalf("Name = %+v, err %v", nm, err)
	}
	ver, err := llm.Version(ctx, &proto.VersionRequest{})
	if err != nil || ver.Version == "" {
		t.Fatalf("Version = %+v, err %v", ver, err)
	}
	if _, err := llm.HealthCheck(ctx, &proto.HealthCheckRequest{}); err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}

	// 6. Chat（非流式）
	resp, err := llm.Chat(ctx, &proto.ChatRequest{
		Messages: []*proto.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil || resp.Content != "非流式回复" || resp.FinishReason != "end_turn" {
		t.Fatalf("Chat = %+v, err %v", resp, err)
	}

	// 7. ChatStream（SSE 流式：增量拼接 + finish_reason + usage）
	stream, err := llm.ChatStream(ctx, &proto.ChatRequest{
		Messages: []*proto.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	var b strings.Builder
	finish := ""
	var usage *proto.Usage
	for {
		frame, recvErr := stream.Recv()
		if recvErr != nil {
			if recvErr != io.EOF {
				t.Fatalf("ChatStream Recv: %v", recvErr)
			}
			break
		}
		b.WriteString(frame.Content)
		if frame.FinishReason != "" {
			finish = frame.FinishReason
		}
		if frame.Usage != nil {
			usage = frame.Usage
		}
	}
	if b.String() != "Hello from mock" {
		t.Fatalf("流式拼接 = %q, want %q", b.String(), "Hello from mock")
	}
	if finish != "end_turn" {
		t.Fatalf("finish_reason = %q, want end_turn", finish)
	}
	if usage == nil || usage.CompletionTokens != 7 {
		t.Fatalf("usage = %+v, want completion=7", usage)
	}
	if !streamFlushed {
		t.Fatal("mock 服务应收到流式请求（SSE 路径被走到）")
	}
}
