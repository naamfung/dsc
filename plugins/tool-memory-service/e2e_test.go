package main

import (
	"context"
	"encoding/json"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"testing"

	"dsc/core"
	"dsc/proto"
	"dsc/proto/metadata"
	plugin "github.com/hashicorp/go-plugin"
)

// TestJoinMatchTerms FTS5 转义：含标点的词须加引号、词内双引号翻倍、以 OR 连接。
func TestJoinMatchTerms(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"config.yaml"}, `"config.yaml"`},
		{[]string{"a.b", "c-d", "d/e"}, `"a.b" OR "c-d" OR "d/e"`},
		{[]string{`say"hi`}, `"say""hi"`},
		{[]string{"粤语", "config.yaml"}, `"粤语" OR "config.yaml"`},
		{nil, ""},
	}
	for _, c := range cases {
		if got := joinMatchTerms(c.in); got != c.want {
			t.Fatalf("joinMatchTerms(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// assertView 校验工具结果经完整 gRPC 链路透传的 ViewJson：非空且可解析为合法视图。
func assertView(t *testing.T, resp *proto.ExecuteToolResponse) core.ToolView {
	t.Helper()
	if resp.ViewJson == "" {
		t.Fatalf("ViewJson 为空（ViewFn 未生效或 gRPC 透传缺失）: %+v", resp)
	}
	var v core.ToolView
	if err := json.Unmarshal([]byte(resp.ViewJson), &v); err != nil {
		t.Fatalf("ViewJson 非法: %v", err)
	}
	if v.Kind == "" {
		t.Fatalf("ViewJson 缺 kind: %q", resp.ViewJson)
	}
	return v
}

// TestE2EWithHostClient 端到端验证 SDK 化后的记忆服务插件能被宿主侧正常拉起并调用：
// 以 go-plugin 客户端 spawn 本插件 exe，经 gRPC 验证元数据、工具目录，并真实执行
// memory_add → memory_search（记忆库落在宿主可执行目录 memory/ 下，跨会话共享）。
func TestE2EWithHostClient(t *testing.T) {
	// 1. 构建插件 exe（独立 module 的完整独立开发者路径）
	dir := t.TempDir()
	exe := filepath.Join(dir, "tool-memory-service.exe")
	if out, err := osexec.Command("go", "build", "-o", exe, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	// 2. 准备宿主可执行目录（记忆库落点 <exeDir>/memory/memory.db）
	hostDir := filepath.Join(dir, "host")
	if err := os.MkdirAll(hostDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// 3. 以宿主侧客户端拉起插件进程（工作目录=宿主可执行目录，模拟宿主注入 ExecDir）
	cmd := osexec.Command(exe)
	cmd.Dir = hostDir
	cmd.Env = os.Environ()
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

	// 4. 元数据（SDK 自动提供）
	meta := metadata.NewPluginMetadataClient(conn)
	info, err := meta.GetInfo(ctx, &metadata.Empty{})
	if err != nil || info.Type != "tool" || info.Name != "memory-service" || info.ApiVersion != "1.0" {
		t.Fatalf("GetInfo = %+v, err %v", info, err)
	}

	// 5. 工具目录（SDK 自动聚合 memory_search + memory_add）
	tc := proto.NewToolServiceClient(conn)
	list, err := tc.ListTools(ctx, &proto.ListToolsRequest{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	names := map[string]bool{}
	for _, tool := range list.Tools {
		names[tool.Name] = true
	}
	if len(list.Tools) != 2 || !names["memory_search"] || !names["memory_add"] {
		t.Fatalf("ListTools = %+v", list.Tools)
	}

	// 6. 真实执行：memory_add → memory_search
	run := func(tool, args string) *proto.ExecuteToolResponse {
		t.Helper()
		resp, err := tc.ExecuteTool(ctx, &proto.ExecuteToolRequest{ToolName: tool, ArgumentsJson: args})
		if err != nil {
			t.Fatalf("ExecuteTool(%s): %v", tool, err)
		}
		return resp
	}
	if resp := run("memory_add", `{"content":"用户偏好：用粤语交流","source":"user"}`); resp.Error != "" {
		t.Fatalf("memory_add = %+v", resp)
	} else if v := assertView(t, resp); v.Kind != "card" || v.Title != "Memory" || v.Badge == nil || v.Badge.Text != "saved" {
		t.Fatalf("memory_add view = %+v", v)
	}
	if resp := run("memory_search", `{"query":"粤语"}`); resp.Error != "" || !strings.Contains(resp.Content, "粤语") {
		t.Fatalf("memory_search = %+v", resp)
	} else if v := assertView(t, resp); v.Kind != "table" || v.Title != "Memory" || len(v.Rows) == 0 || v.Rows[0]["content"] == "" {
		t.Fatalf("memory_search view = %+v", v)
	}
	// 回归：带标点的检索词（如文件名 config.yaml、README.md）不应触发 FTS5 语法错误
	//（裸词中的 '.' 会被 FTS5 当作语法字符）。修复前此类查询报 "fts5: syntax error near '.'"。
	if resp := run("memory_add", `{"content":"项目约定：使用 config.yaml 与 README.md 描述规范","source":"user"}`); resp.Error != "" {
		t.Fatalf("memory_add(punct) = %+v", resp)
	}
	if resp := run("memory_search", `{"query":"config.yaml"}`); resp.Error != "" {
		t.Fatalf("带标点关键词 memory_search 不应报语法错误 = %+v", resp)
	} else if v := assertView(t, resp); v.Kind != "table" {
		t.Fatalf("带标点关键词 memory_search view = %+v", v)
	}

	// 7. 记忆库落点校验：<hostDir>/memory/memory.db（跨会话，非项目级）
	if _, err := os.Stat(filepath.Join(hostDir, "memory", "memory.db")); err != nil {
		t.Fatalf("memory.db 未创建于宿主可执行目录 memory 下: %v", err)
	}
}
