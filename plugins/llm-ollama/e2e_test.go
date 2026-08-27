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
// 以宿主侧 go-core 客户端 spawn exe（OLLAMA_HOST 指向本地 mock 服务），
// 经 gRPC 验证元数据（type=llm）、Name/Version/HealthCheck、Chat（非流式）
// 与 ChatStream（NDJSON 流式 + done + finish_reason）。
func TestE2EWithHostClient(t *testing.T) {
	// 1. mock Ollama 兼容服务：/api/chat 支持非流式与流式（NDJSON）
	var streamFlushed bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}
		var req struct {
			Stream bool `json:"stream"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Stream {
			streamFlushed = true
			w.Header().Set("Content-Type", "application/x-ndjson")
			flusher := w.(http.Flusher)
			write := func(msg string) {
				fmt.Fprintf(w, "%s\n", msg)
				flusher.Flush()
			}
			write(`{"model":"mock","message":{"role":"assistant","content":"Hello"},"done":false}`)
			write(`{"model":"mock","message":{"role":"assistant","content":" from mock"},"done":false}`)
			write(`{"model":"mock","message":{"role":"assistant","content":""},"done":true}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"model":"mock","message":{"role":"assistant","content":"非流式回复"},"done":true}`)
	}))
	defer srv.Close()

	// 2. 构建插件 exe（独立 module 的完整独立开发者路径）
	dir := t.TempDir()
	exe := filepath.Join(dir, "llm-ollama.exe")
	if out, err := exec.Command("go", "build", "-o", exe, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	// 3. 以宿主侧客户端拉起插件进程（env 指向本地 mock，关闭 thinking 简化流式）
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(),
		"OLLAMA_HOST="+srv.URL,
		"OLLAMA_MODEL=mock-model",
		"OLLAMA_THINKING=0",
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
	if err != nil || info.Type != "llm" || info.Name != "ollama" || info.ApiVersion != "1.0" {
		t.Fatalf("GetInfo = %+v, err %v", info, err)
	}

	// 5. LLMService 基础方法（SDK 复用 core.LLMGRPCPlugin）
	llm := proto.NewLLMServiceClient(conn)
	nm, err := llm.Name(ctx, &proto.NameRequest{})
	if err != nil || nm.Name != "ollama" {
		t.Fatalf("Name = %+v, err %v", nm, err)
	}
	if _, err := llm.HealthCheck(ctx, &proto.HealthCheckRequest{}); err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}

	// 6. Chat（非流式）
	resp, err := llm.Chat(ctx, &proto.ChatRequest{
		Messages: []*proto.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil || resp.Content != "非流式回复" {
		t.Fatalf("Chat = %+v, err %v", resp, err)
	}

	// 7. ChatStream（NDJSON 流式：增量拼接 + finish）
	stream, err := llm.ChatStream(ctx, &proto.ChatRequest{
		Messages: []*proto.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	var b strings.Builder
	finish := ""
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
	}
	if b.String() != "Hello from mock" {
		t.Fatalf("流式拼接 = %q, want %q", b.String(), "Hello from mock")
	}
	if finish != "stop" {
		t.Fatalf("finish_reason = %q, want stop", finish)
	}
	if !streamFlushed {
		t.Fatal("mock 服务应收到流式请求（NDJSON 路径被走到）")
	}
}
