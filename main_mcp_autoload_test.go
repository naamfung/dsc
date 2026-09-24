package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"dsc/core"
	"dsc/libs/vodka"
	"github.com/hashicorp/go-hclog"
)

// mockMCPServer 用 vodka 起一个假 MCP 服务器：响应 JSON-RPC initialize / tools/list / tools/call。
// MCP 协议经 HTTP POST + JSON-RPC 2.0 通信，DSC 的 MCPClient 走的就是这个路径。
// 使用 vodka（贴合 DSC 代码惯例，admin API 同框架）而非裸 net/http。
func mockMCPServer(t *testing.T) *httptest.Server {
	t.Helper()

	e := vodka.New()
	e.Post("/mcp", func(c *vodka.Context) error {
		var req map[string]any
		if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
			return vodka.NewHTTPError(http.StatusBadRequest, "bad request")
		}

		method, _ := req["method"].(string)
		id := req["id"]

		var resp map[string]any
		switch method {
		case "initialize":
			resp = map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]any{
					"protocolVersion": "2024-11-05",
					"capabilities":    map[string]any{},
					"serverInfo": map[string]any{
						"name":    "mock-mcp",
						"version": "1.0.0",
					},
				},
			}
		case "notifications/initialized":
			// 通知不需响应——返回空 200
			return c.String("")
		case "tools/list":
			resp = map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]any{
					"tools": []map[string]any{
						{
							"name":        "echo",
							"description": "Echo back the input text",
							"inputSchema": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"text": map[string]any{
										"type":        "string",
										"description": "Text to echo back",
									},
								},
								"required": []string{"text"},
							},
						},
					},
				},
			}
		case "tools/call":
			resp = map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]any{
					"content": []map[string]any{
						{"type": "text", "text": "mock response from echo tool"},
					},
				},
			}
		default:
			resp = map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"error": map[string]any{
					"code":    -32601,
					"message": "method not found: " + method,
				},
			}
		}
		return c.JSON(resp)
	})

	// vodka 实现了 http.Handler（ServeHTTP 方法），httptest.NewServer 可直接包装
	return httptest.NewServer(e)
}

// TestMCPPatchEntryNotLoadedAsPlugin 校验：含 config.mcp 的配置条目
// 不被 LoadFromConfig 当作真实插件二进制去加载（修复 GitHub issue #2）。
//
// 旧 bug：type=dsc + config.mcp 的条目被加入 providerEntries →
// loadPluginWithBroker → validatePluginDirectoryName("dsc", "ocr") →
// "ocr" 不以 "dsc-" 开头 → 报错 "invalid core directory name" → 启动失败。
//
// 修复后：LoadFromConfig 跳过含 config.mcp 的条目（它们由
// autoConnectMCPFromConfig 后续处理），不进入插件加载路径。
//
// 测试策略：构造一个只有 MCP 条目（无 agent/llm）的 config，
// 调 LoadFromConfig。如果 MCP 条目仍被误加载，会报
// "invalid core directory name 'ocr' for type 'dsc'"；
// 修复后应报 "no agent core found in config"（说明 MCP 条目被跳过了，
// 没走到 loadPluginWithBroker 的目录名校验）。
func TestMCPPatchEntryNotLoadedAsPlugin(t *testing.T) {
	srv := mockMCPServer(t)
	defer srv.Close()

	cfg := &core.Config{
		Mode: "standard",
		Plugins: []core.PluginEntry{
			// MCP 配置条目——这就是 issue #2 用户声明的形态
			{
				Name:    "ocr",
				Type:    "dsc",
				Enabled: true,
				Config: map[string]any{
					"mcp": map[string]any{
						"server_name": "ocr",
						"endpoint":    srv.URL + "/mcp",
					},
				},
			},
		},
	}

	mgr := core.NewManager(&core.ManagerConfig{
		Handshake: core.Handshake,
	})

	err := mgr.LoadFromConfig(cfg)

	if err == nil {
		t.Fatal("LoadFromConfig should fail (no agent in config), but got nil error")
	}

	errMsg := err.Error()
	t.Logf("LoadFromConfig error: %s", errMsg)

	// 关键断言：错误信息应该是 "no agent core found"（MCP 条目被跳过了），
	// 而非 "invalid core directory name 'ocr'"（MCP 条目被误加载了）
	if contains(errMsg, "invalid core directory name") {
		t.Fatalf("BUG: MCP entry was loaded as plugin binary: %s", errMsg)
	}
	if !contains(errMsg, "no agent core found") {
		t.Fatalf("expected 'no agent core found' error (MCP entry skipped), got: %s", errMsg)
	}
	t.Log("OK: MCP config entry was correctly skipped in LoadFromConfig (no 'invalid core directory name' error)")
}

// TestAutoConnectMCPFromConfig 集成测试：假 MCP 服务 + autoConnectMCPFromConfig
// 验证 MCP 条目经 -patch 注入后，启动时能成功连接假 MCP 服务并发现工具。
func TestAutoConnectMCPFromConfig(t *testing.T) {
	srv := mockMCPServer(t)
	defer srv.Close()

	cfg := &core.Config{
		Plugins: []core.PluginEntry{
			{
				Name:    "mcp-test",
				Type:    "dsc",
				Enabled: true,
				Config: map[string]any{
					"mcp": map[string]any{
						"server_name": "test",
						"endpoint":    srv.URL + "/mcp",
					},
				},
			},
		},
	}

	mgr := core.NewManager(&core.ManagerConfig{
		Handshake: core.Handshake,
	})

	// autoConnectMCPFromConfig 应成功连接假 MCP 服务并发现 echo 工具
	autoConnectMCPFromConfig(mgr, cfg, hclog.NewNullLogger())

	// 验证 MCP 客户端已注册
	client := mgr.GetMCPClient("test")
	if client == nil {
		t.Fatal("MCP client 'test' not registered after autoConnectMCPFromConfig")
	}

	// 验证发现了 echo 工具
	tools := client.Tools()
	if len(tools) == 0 {
		t.Fatal("expected at least 1 tool from mock MCP server, got 0")
	}
	t.Logf("OK: discovered %d tool(s) from mock MCP server", len(tools))
	found := false
	for _, tool := range tools {
		t.Logf("  tool: name=%s desc=%s", tool.Name, tool.Description)
		if tool.Name == "mcp__test__echo" { // publicName = mcp__<server>__<rawName>
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'mcp__test__echo' tool among discovered tools")
	}

	// 验证工具计数
	if count := client.ToolCount(); count != 1 {
		t.Errorf("ToolCount = %d, want 1", count)
	}
}

// contains 简单字符串包含检查（测试 helper）
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && containsStr(s, substr)))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// mockMCPServerSSE 模拟新版 streamable-http MCP 服务器：
// 1. 校验 Accept 头含 application/json 与 text/event-stream（缺则返回 406）
// 2. 以 text/event-stream（SSE）格式响应 JSON-RPC 结果
func mockMCPServerSSE(t *testing.T) *httptest.Server {
	t.Helper()

	e := vodka.New()
	e.Post("/mcp", func(c *vodka.Context) error {
		// 校验 Accept 头（MCP 规范 2025-03-26+ streamable-http 要求）
		accept := c.Request.Header.Get("Accept")
		if !strings.Contains(accept, "application/json") || !strings.Contains(accept, "text/event-stream") {
			return vodka.NewHTTPError(http.StatusNotAcceptable, "Not Acceptable: Client must accept both application/json and text/event-stream")
		}

		var req map[string]any
		if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
			return vodka.NewHTTPError(http.StatusBadRequest, "bad request")
		}

		method, _ := req["method"].(string)
		id := req["id"]

		var resp map[string]any
		switch method {
		case "initialize":
			resp = map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]any{
					"protocolVersion": "2025-03-26",
					"capabilities":    map[string]any{},
					"serverInfo": map[string]any{
						"name":    "mock-mcp-sse",
						"version": "1.0.0",
					},
				},
			}
		case "notifications/initialized":
			return c.String("")
		case "tools/list":
			resp = map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]any{
					"tools": []map[string]any{
						{
							"name":        "ocr",
							"description": "OCR text recognition",
							"inputSchema": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"image": map[string]any{
										"type":        "string",
										"description": "Image path or base64",
									},
								},
								"required": []string{"image"},
							},
						},
					},
				},
			}
		case "tools/call":
			resp = map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]any{
					"content": []map[string]any{
						{"type": "text", "text": "recognized text from OCR"},
					},
				},
			}
		default:
			resp = map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"error":   map[string]any{"code": -32601, "message": "method not found: " + method},
			}
		}

		// 以 SSE 格式响应：data: <json>\n\n
		jsonBytes, _ := json.Marshal(resp)
		c.Response.Header().Set("Content-Type", "text/event-stream")
		c.Response.WriteHeader(http.StatusOK)
		c.Response.Write([]byte("data: " + string(jsonBytes) + "\n\n"))
		return nil
	})

	return httptest.NewServer(e)
}

// TestStreamableHTTPAcceptHeader 校验：MCP 客户端发送 Accept 头含
// application/json 与 text/event-stream——缺则 streamable-http 服务器返回 406。
func TestStreamableHTTPAcceptHeader(t *testing.T) {
	srv := mockMCPServerSSE(t)
	defer srv.Close()

	cfg := &core.Config{
		Plugins: []core.PluginEntry{
			{
				Name:    "ocr-sse",
				Type:    "dsc",
				Enabled: true,
				Config: map[string]any{
					"mcp": map[string]any{
						"server_name": "ocr-sse",
						"endpoint":    srv.URL + "/mcp",
					},
				},
			},
		},
	}

	mgr := core.NewManager(&core.ManagerConfig{
		Handshake: core.Handshake,
	})

	autoConnectMCPFromConfig(mgr, cfg, hclog.NewNullLogger())

	client := mgr.GetMCPClient("ocr-sse")
	if client == nil {
		t.Fatal("MCP client 'ocr-sse' not registered — streamable-http connect failed")
	}

	tools := client.Tools()
	if len(tools) == 0 {
		t.Fatal("expected at least 1 tool from SSE MCP server, got 0")
	}
	t.Logf("OK: streamable-http SSE server connected, discovered %d tool(s)", len(tools))
	for _, tool := range tools {
		t.Logf("  tool: name=%s desc=%s", tool.Name, tool.Description)
	}
	if tools[0].Name != "mcp__ocr-sse__ocr" {
		t.Errorf("expected 'mcp__ocr-sse__ocr' tool, got %q", tools[0].Name)
	}
}

// TestSSEResponseParsing 校验：ParseSSEResponse 能正确解析 SSE 格式响应
// （data: <json>\n\n → 提取 JSON 负载）。
func TestSSEResponseParsing(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		wantErr bool
		wantID  string
	}{
		{
			name:   "single event",
			body:   "data: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"tools\":[]}}\n\n",
			wantID: "1",
		},
		{
			name:   "event with comment lines",
			body:   ": comment\ndata: {\"jsonrpc\":\"2.0\",\"id\":2,\"result\":{}}\n\n",
			wantID: "2",
		},
		{
			name:    "no data line",
			body:    ": only comment\n\n",
			wantErr: true,
		},
		{
			name:    "invalid JSON in data",
			body:    "data: {bad json}\n\n",
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := core.ParseSSEResponse([]byte(tc.body))
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				t.Logf("OK: got expected error: %v", err)
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			id, ok := result["id"]
			if !ok {
				t.Fatal("response missing 'id' field")
			}
			if fmt.Sprintf("%v", id) != tc.wantID {
				t.Errorf("id = %v, want %s", id, tc.wantID)
			}
			t.Logf("OK: parsed SSE response, id=%v", id)
		})
	}
}
