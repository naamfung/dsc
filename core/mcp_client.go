package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// MCP 客户端（对齐 DSH/Cordis 的 mcp-client 插件）。
//
// DSH 的 MCP 客户端是一个 Cordis 插件，连接外部 MCP 服务器并把其工具注册为
// mcp__<serverName>__<rawName> 命名的模型工具。每个插件实例连接一个 MCP 服务器；
// 多个实例可连接多个 MCP 服务器。
//
// DSC 的适配：DSC 的 MCP 客户端作为宿主内置工具注册器——连接外部 MCP 服务器的
// stdio 或 HTTP 传输，发现其工具列表，注册到 ToolRegistry。工具命名约定对齐 DSH：
// mcp__<serverName>__<rawName>。
//
// 当前实现支持 HTTP/SSE 传输（MCP 标准传输之一）。stdio 传输需要进程管理，
// 后续按需扩展。

// MCPClient 连接一个外部 MCP 服务器，发现并注册其工具。
type MCPClient struct {
	mu         sync.Mutex
	serverName string
	endpoint   string
	httpClient *http.Client
	tools      map[string]*MCPTool // rawName → tool
	manager    *Manager
}

// MCPTool 一个经 MCP 桥接的工具。
type MCPTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Schema      json.RawMessage `json:"schema"`
	// publicName 模型可见的工具名：mcp__<serverName>__<rawName>
	publicName string
	// rawName MCP 服务器上的原始工具名
	rawName string
	// serverName MCP 服务器名
	serverName string
}

// NewMCPClient 创建 MCP 客户端。serverName 仅允许 [A-Za-z0-9_-]，最长 32 字符。
// endpoint 为 MCP 服务器的 HTTP URL（如 http://localhost:3000/mcp）。
func NewMCPClient(mgr *Manager, serverName, endpoint string) (*MCPClient, error) {
	if !isMCPServerNameValid(serverName) {
		return nil, fmt.Errorf("invalid MCP server name %q: use [A-Za-z0-9_-] only, max 32 chars", serverName)
	}
	return &MCPClient{
		serverName: serverName,
		endpoint:   endpoint,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		tools:      make(map[string]*MCPTool),
		manager:    mgr,
	}, nil
}

// Connect 连接 MCP 服务器并发现工具。
func (c *MCPClient) Connect(ctx context.Context) error {
	// MCP 协议：发送 initialize 请求
	initReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"clientInfo": map[string]any{
				"name":    "dsc-mcp-client",
				"version": "1.0.0",
			},
		},
	}

	resp, err := c.doJSON(ctx, initReq)
	if err != nil {
		return fmt.Errorf("MCP initialize failed: %w", err)
	}
	_ = resp
	// 验证响应中有 capabilities
	if _, ok := resp["result"]; !ok {
		return fmt.Errorf("MCP initialize: no result in response")
	}

	// 发送 initialized 通知
	notif := map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
	}
	_, _ = c.doJSON(ctx, notif) // 通知不需响应

	// 发现工具
	return c.discoverTools(ctx)
}

// discoverTools 列出 MCP 服务器的工具并注册到 ToolRegistry。
func (c *MCPClient) discoverTools(ctx context.Context) error {
	listReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
		"params":  map[string]any{},
	}

	resp, err := c.doJSON(ctx, listReq)
	if err != nil {
		return fmt.Errorf("MCP tools/list failed: %w", err)
	}

	result, ok := resp["result"].(map[string]any)
	if !ok {
		return fmt.Errorf("MCP tools/list: no result")
	}

	toolsRaw, ok := result["tools"].([]any)
	if !ok {
		return fmt.Errorf("MCP tools/list: no tools array")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for _, t := range toolsRaw {
		tool, ok := t.(map[string]any)
		if !ok {
			continue
		}
		rawName, _ := tool["name"].(string)
		if rawName == "" {
			continue
		}
		desc, _ := tool["description"].(string)
		schema, _ := tool["inputSchema"].(map[string]any)
		schemaJSON, _ := json.Marshal(schema)

		publicName := mcpPublicName(c.serverName, rawName)
		mcpTool := &MCPTool{
			Name:        publicName,
			Description: desc,
			Schema:      schemaJSON,
			publicName:  publicName,
			rawName:     rawName,
			serverName:  c.serverName,
		}
		c.tools[rawName] = mcpTool

		// 注册到 ToolRegistry
		if c.manager != nil {
			c.manager.toolRegistry.Register(&RemoteTool{
				name:        publicName,
				description: desc,
				schema:      json.RawMessage(schemaJSON),
				client:      nil, // MCP 工具不经 gRPC 转发，直接经 MCPClient 调用
			})
		}
	}

	return nil
}

// CallTool 调用 MCP 服务器上的工具。
func (c *MCPClient) CallTool(ctx context.Context, rawName string, args json.RawMessage) (string, error) {
	callReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      time.Now().UnixNano(),
		"method":  "tools/call",
		"params": map[string]any{
			"name":      rawName,
			"arguments": json.RawMessage(args),
		},
	}

	resp, err := c.doJSON(ctx, callReq)
	if err != nil {
		return "", fmt.Errorf("MCP tools/call %q failed: %w", rawName, err)
	}

	if errMsg, ok := resp["error"]; ok {
		return "", fmt.Errorf("MCP error: %v", errMsg)
	}

	result, ok := resp["result"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("MCP tools/call: no result")
	}

	// MCP 返回 content 数组（文本块）
	content, ok := result["content"].([]any)
	if !ok {
		// 尝试直接返回 result 的 JSON
		b, _ := json.Marshal(result)
		return string(b), nil
	}

	var sb strings.Builder
	for _, item := range content {
		if block, ok := item.(map[string]any); ok {
			if text, ok := block["text"].(string); ok {
				sb.WriteString(text)
			}
		}
	}
	return sb.String(), nil
}

// Disconnect 断开连接并注销所有工具。
func (c *MCPClient) Disconnect() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.manager != nil {
		for _, t := range c.tools {
			c.manager.toolRegistry.Unregister(t.publicName)
		}
	}
	c.tools = make(map[string]*MCPTool)
}

// ServerName 返回 MCP 服务器名。
func (c *MCPClient) ServerName() string {
	return c.serverName
}

// Tools 返回已发现的工具列表（副本）。
func (c *MCPClient) Tools() []*MCPTool {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]*MCPTool, 0, len(c.tools))
	for _, t := range c.tools {
		out = append(out, t)
	}
	return out
}

// doJSON 发送 JSON-RPC 请求并解析响应。
func (c *MCPClient) doJSON(ctx context.Context, req map[string]any) (map[string]any, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.endpoint, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("invalid JSON response: %w", err)
	}

	return result, nil
}

// mcpPublicName 生成模型可见的工具名：mcp__<serverName>__<rawName>
// 对齐 DSH 的命名约定（参见 dsh/packages/mcp/mcp-client/src/tools.ts）。
func mcpPublicName(serverName, rawName string) string {
	name := "mcp__" + serverName + "__" + rawName
	// 规范化：非 [A-Za-z0-9_-] 字符替换为 _（对齐 DeepSeek 函数名约束）
	name = mcpNormalizeName(name)
	// 截断到 64 字符（对齐 DeepSeek 函数名约束）
	if len(name) > 64 {
		name = name[:64]
	}
	return name
}

// mcpNormalizeName 把非 [A-Za-z0-9_-] 字符替换为 _。
func mcpNormalizeName(s string) string {
	b := []byte(s)
	for i := range b {
		c := b[i]
		if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' || c == '-') {
			b[i] = '_'
		}
	}
	return string(b)
}

// isMCPServerNameValid 校验 MCP 服务器名：仅 [A-Za-z0-9_-]，最长 32 字符。
func isMCPServerNameValid(name string) bool {
	if len(name) == 0 || len(name) > 32 {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' || c == '-') {
			return false
		}
	}
	return true
}
