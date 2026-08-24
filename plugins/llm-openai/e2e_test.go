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

	"dsc/plugin"
	"dsc/proto"
	"dsc/proto/metadata"
	goplugin "github.com/hashicorp/go-plugin"
)

// TestE2EWithHostClient 端到端验证 SDK 重写后的 LLM 插件：
// 以宿主侧 go-plugin 客户端 spawn exe（OPENAI_BASE_URL 指向本地 mock 服务），
// 经 gRPC 验证元数据（type=llm）、Name/Version/HealthCheck、Chat（非流式）
// 与 ChatStream（SSE 流式 + finish_reason + usage）。
func TestE2EWithHostClient(t *testing.T) {
	// 1. mock OpenAI 兼容服务：/v1/chat/completions 支持非流式与流式（SSE）
	var streamFlushed bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// openai SDK 请求路径为 <baseURL>/chat/completions（main 里 baseURL 不含 /v1）
		if r.URL.Path != "/chat/completions" {
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
			for _, c := range []string{"Hello", " from", " mock"} {
				fmt.Fprintf(w, "data: %s\n\n", `{"choices":[{"delta":{"role":"assistant","content":"`+c+`"}}]}`)
				flusher.Flush()
			}
			fmt.Fprintf(w, "data: %s\n\n", `{"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":7,"total_tokens":12}}`)
			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"非流式回复"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":7,"total_tokens":12}}`)
	}))
	defer srv.Close()

	// 2. 构建插件 exe（独立 module 的完整独立开发者路径）
	dir := t.TempDir()
	exe := filepath.Join(dir, "llm-openai.exe")
	if out, err := exec.Command("go", "build", "-o", exe, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	// 3. 以宿主侧客户端拉起插件进程（env 指向本地 mock）
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(),
		"OPENAI_BASE_URL="+srv.URL,
		"OPENAI_API_KEY=test-key",
		"OPENAI_MODEL=mock-model",
	)
	client := goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig:  plugin.Handshake,
		Plugins:          map[string]goplugin.Plugin{},
		AllowedProtocols: []goplugin.Protocol{goplugin.ProtocolGRPC},
		Cmd:              cmd,
	})
	defer client.Kill()

	rpcClient, err := client.Client()
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	grpcClient, ok := rpcClient.(*goplugin.GRPCClient)
	if !ok {
		t.Fatalf("unexpected client type %T", rpcClient)
	}
	conn := grpcClient.Conn
	ctx := context.Background()

	// 4. 元数据（SDK 自动提供，Type 由 Config 决定）
	meta := metadata.NewPluginMetadataClient(conn)
	info, err := meta.GetInfo(ctx, &metadata.Empty{})
	if err != nil || info.Type != "llm" || info.Name != "openai" || info.ApiVersion != "1.0" {
		t.Fatalf("GetInfo = %+v, err %v", info, err)
	}

	// 5. LLMService 基础方法（SDK 复用 plugin.LLMGRPCPlugin）
	llm := proto.NewLLMServiceClient(conn)
	nm, err := llm.Name(ctx, &proto.NameRequest{})
	if err != nil || nm.Name != "openai" {
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
	if err != nil || resp.Content != "非流式回复" || resp.FinishReason != "stop" {
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
	if finish != "stop" {
		t.Fatalf("finish_reason = %q, want stop", finish)
	}
	if usage == nil || usage.CompletionTokens != 7 {
		t.Fatalf("usage = %+v, want completion=7", usage)
	}
	if !streamFlushed {
		t.Fatal("mock 服务应收到流式请求（SSE 路径被走到）")
	}
}
