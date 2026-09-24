package core

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"dsc/libs/vodka"
)

// registerLSPAdminRoutes 注册 /lsp/* admin 端点。
// 对齐 DSH packages/lsp/lsp + lsp-stdio + tool-lsp 的能力暴露面：
//   - POST /lsp/start   启动 LSP 服务器子进程并注册 LSPTool 到 ToolRegistry
//   - POST /lsp/stop    停止 LSP 服务器子进程
//   - GET  /lsp/diagnostics  获取诊断信息（按 file_path 可选过滤）
//
// LSPClient 经 stdio 与外部 LSP 服务器（gopls、typescript-language-server 等）通信，
// 诊断信息由外部组件经 SetDiagnostics 推送缓存（完整 LSP 协议握手留待后续）。
// LSPTool 注册到 ToolRegistry 后模型可调用 lsp_diagnostics 工具查询诊断。
func (m *Manager) registerLSPAdminRoutes(group *vodka.RouteGroup) {
	group.Post("/lsp/start", m.handleLSPStart)
	group.Post("/lsp/stop", m.handleLSPStop)
	group.Get("/lsp/diagnostics", m.handleLSPDiagnostics)
}

// registerMCPAdminRoutes 注册 /mcp/* admin 端点。
// 对齐 DSH packages/mcp/mcp-client 的能力暴露面：
//   - POST /mcp/connect     连接 MCP 服务器并发现注册其工具（mcp__<server>__<rawName>）
//   - POST /mcp/disconnect   断开 MCP 服务器并注销其工具
//   - GET  /mcp/list         列出已连接的 MCP 服务器及其工具
//
// MCPClient 经 JSON-RPC over HTTP 与外部 MCP 服务器通信，discoverTools 自动把
// 服务器工具注册到 ToolRegistry，模型可经 mcp__<server>__<rawName> 名调用。
func (m *Manager) registerMCPAdminRoutes(group *vodka.RouteGroup) {
	group.Post("/mcp/connect", m.handleMCPConnect)
	group.Post("/mcp/disconnect", m.handleMCPDisconnect)
	group.Get("/mcp/list", m.handleMCPList)
}

// ---- LSP handlers ----

// ensureLSPClients 懒初始化 lspClients map（线程安全，需持 m.mu）。
func (m *Manager) ensureLSPClients() {
	if m.lspClients == nil {
		m.lspClients = make(map[string]*LSPClient)
	}
}

// ensureLSPToolRegistered 首次启动 LSP 服务器时把 LSPTool 注册到 ToolRegistry。
// LSPTool.client=nil，由 SetManager 注入聚合源——Execute 时遍历所有已启动 LSPClient。
func (m *Manager) ensureLSPToolRegistered() {
	if m.lspToolRegistered {
		return
	}
	if m.toolRegistry == nil {
		return
	}
	tool := NewLSPTool(nil)
	tool.SetManager(m)
	m.toolRegistry.Register(tool)
	m.lspToolRegistered = true
}

// lspDiagnosticsAll 聚合所有已启动 LSPClient 的诊断（按文件路径合并）。
// 供 LSPTool.Execute 在 client=nil 时调用。
func (m *Manager) lspDiagnosticsAll(ctx context.Context, filePath string) (map[string][]LSPDiagnostic, error) {
	m.mu.Lock()
	clients := make([]*LSPClient, 0, len(m.lspClients))
	for _, c := range m.lspClients {
		clients = append(clients, c)
	}
	m.mu.Unlock()

	out := make(map[string][]LSPDiagnostic)
	for _, c := range clients {
		if filePath != "" {
			diags, err := c.GetDiagnostics(ctx, filePath)
			if err == nil && len(diags) > 0 {
				out[filePath] = append(out[filePath], diags...)
			}
			continue
		}
		all, err := c.GetAllDiagnostics(ctx)
		if err != nil {
			continue
		}
		for f, diags := range all {
			out[f] = append(out[f], diags...)
		}
	}
	return out, nil
}

func (m *Manager) handleLSPStart(c *vodka.Context) error {
	var req struct {
		ServerID string   `json:"server_id"` // 如 "go"、"typescript"
		Language string   `json:"language"`  // 如 "go"、"typescript"、"python"
		Command  string   `json:"command"`   // LSP 服务器可执行文件路径
		Args     []string `json:"args"`      // 启动参数
	}
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		return vodka.NewHTTPError(http.StatusBadRequest, "invalid JSON: "+err.Error())
	}
	if req.ServerID == "" || req.Command == "" {
		return vodka.NewHTTPError(http.StatusBadRequest, "server_id and command are required")
	}
	if req.Language == "" {
		req.Language = req.ServerID
	}

	rootPath := ""
	if m.config != nil {
		rootPath = m.config.ExecDir
	}
	if ws := WorkspaceRoot; ws != "" {
		rootPath = ws
	}

	m.mu.Lock()
	m.ensureLSPClients()
	if existing, ok := m.lspClients[req.ServerID]; ok && existing.started {
		m.mu.Unlock()
		return vodka.NewHTTPError(http.StatusConflict, fmt.Sprintf("LSP server %q already started", req.ServerID))
	}
	client := NewLSPClient(req.Language, rootPath, req.ServerID)
	m.lspClients[req.ServerID] = client
	m.mu.Unlock()

	// 首次启动时注册 LSPTool 到 ToolRegistry（无状态工具，注册一次）
	m.mu.Lock()
	m.ensureLSPToolRegistered()
	m.mu.Unlock()

	ctx := context.Background()
	if err := client.Start(ctx, req.Command, req.Args...); err != nil {
		m.mu.Lock()
		delete(m.lspClients, req.ServerID)
		m.mu.Unlock()
		return vodka.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(map[string]any{"ok": true, "server_id": req.ServerID, "language": req.Language})
}

func (m *Manager) handleLSPStop(c *vodka.Context) error {
	var req struct {
		ServerID string `json:"server_id"`
	}
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		return vodka.NewHTTPError(http.StatusBadRequest, "invalid JSON: "+err.Error())
	}
	if req.ServerID == "" {
		return vodka.NewHTTPError(http.StatusBadRequest, "server_id is required")
	}

	m.mu.Lock()
	client, ok := m.lspClients[req.ServerID]
	if !ok {
		m.mu.Unlock()
		return vodka.NewHTTPError(http.StatusNotFound, fmt.Sprintf("LSP server %q not found", req.ServerID))
	}
	delete(m.lspClients, req.ServerID)
	m.mu.Unlock()

	if err := client.Stop(); err != nil {
		return vodka.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(map[string]any{"ok": true, "server_id": req.ServerID})
}

func (m *Manager) handleLSPDiagnostics(c *vodka.Context) error {
	filePath := c.QueryParam("file_path")
	ctx := context.Background()

	all, err := m.lspDiagnosticsAll(ctx, filePath)
	if err != nil {
		return vodka.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if filePath != "" {
		diags := all[filePath]
		return c.JSON(map[string]any{"file_path": filePath, "diagnostics": diags, "count": len(diags)})
	}
	totalCount := 0
	for _, diags := range all {
		totalCount += len(diags)
	}
	return c.JSON(map[string]any{"files": all, "file_count": len(all), "total_diagnostics": totalCount})
}

// ---- MCP handlers ----

// ensureMCPClients 懒初始化 mcpClients map（线程安全，需持 m.mu）。
func (m *Manager) ensureMCPClients() {
	if m.mcpClients == nil {
		m.mcpClients = make(map[string]*MCPClient)
	}
}

func (m *Manager) handleMCPConnect(c *vodka.Context) error {
	var req struct {
		ServerName string `json:"server_name"` // [A-Za-z0-9_-]{1,32}
		Endpoint   string `json:"endpoint"`    // MCP 服务器 HTTP URL
	}
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		return vodka.NewHTTPError(http.StatusBadRequest, "invalid JSON: "+err.Error())
	}
	if req.ServerName == "" || req.Endpoint == "" {
		return vodka.NewHTTPError(http.StatusBadRequest, "server_name and endpoint are required")
	}
	if !isMCPServerNameValid(req.ServerName) {
		return vodka.NewHTTPError(http.StatusBadRequest, "invalid server_name: use [A-Za-z0-9_-] only, max 32 chars")
	}

	m.mu.Lock()
	m.ensureMCPClients()
	if _, ok := m.mcpClients[req.ServerName]; ok {
		m.mu.Unlock()
		return vodka.NewHTTPError(http.StatusConflict, fmt.Sprintf("MCP server %q already connected", req.ServerName))
	}
	m.mu.Unlock()

	client, err := NewMCPClient(m, req.ServerName, req.Endpoint)
	if err != nil {
		return vodka.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	ctx := context.Background()
	if err := client.Connect(ctx); err != nil {
		return vodka.NewHTTPError(http.StatusBadGateway, "MCP connect failed: "+err.Error())
	}

	m.mu.Lock()
	m.ensureMCPClients()
	m.mcpClients[req.ServerName] = client
	toolCount := len(client.tools)
	m.mu.Unlock()

	return c.JSON(map[string]any{"ok": true, "server_name": req.ServerName, "tools_discovered": toolCount})
}

func (m *Manager) handleMCPDisconnect(c *vodka.Context) error {
	var req struct {
		ServerName string `json:"server_name"`
	}
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		return vodka.NewHTTPError(http.StatusBadRequest, "invalid JSON: "+err.Error())
	}
	if req.ServerName == "" {
		return vodka.NewHTTPError(http.StatusBadRequest, "server_name is required")
	}

	m.mu.Lock()
	client, ok := m.mcpClients[req.ServerName]
	if !ok {
		m.mu.Unlock()
		return vodka.NewHTTPError(http.StatusNotFound, fmt.Sprintf("MCP server %q not found", req.ServerName))
	}
	delete(m.mcpClients, req.ServerName)
	m.mu.Unlock()

	client.Disconnect()

	// 注销该服务器的所有工具
	if m.toolRegistry != nil {
		for rawName := range client.tools {
			publicName := mcpPublicName(req.ServerName, rawName)
			m.toolRegistry.Unregister(publicName)
		}
	}
	return c.JSON(map[string]any{"ok": true, "server_name": req.ServerName, "tools_removed": len(client.tools)})
}

func (m *Manager) handleMCPList(c *vodka.Context) error {
	m.mu.Lock()
	m.ensureMCPClients()
	servers := make([]map[string]any, 0, len(m.mcpClients))
	for name, client := range m.mcpClients {
		tools := make([]map[string]any, 0, len(client.tools))
		for _, t := range client.tools {
			tools = append(tools, map[string]any{
				"name":        t.publicName,
				"raw_name":    t.rawName,
				"description": t.Description,
			})
		}
		servers = append(servers, map[string]any{
			"server_name": name,
			"endpoint":    client.endpoint,
			"tool_count":  len(client.tools),
			"tools":       tools,
		})
	}
	m.mu.Unlock()
	return c.JSON(map[string]any{"servers": servers, "count": len(servers)})
}

// ---- LSPTool.Execute 聚合逻辑 ----
//
// LSPTool.client 非 nil 时查询单实例；client 为 nil 时经 Manager.lspDiagnosticsAll
// 聚合所有已启动 LSPClient 的诊断缓存（由 admin API /lsp/start 注册的实例）。
// SetManager 在 ensureLSPToolRegistered 中注入，无需修改 LSPTool 自身签名。

// RegisterMCPClient 注册一个已连接的 MCPClient 到 Manager.mcpClients。
// 供 main.go 的 autoConnectMCPFromConfig（-patch 注入的 MCP 配置自动连接）
// 与 admin API /mcp/connect 共用同一注册路径。
func (m *Manager) RegisterMCPClient(serverName string, client *MCPClient) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureMCPClients()
	m.mcpClients[serverName] = client
}

// GetMCPClient 取得已注册的 MCP 客户端（供测试与运行时查询）。
func (m *Manager) GetMCPClient(serverName string) *MCPClient {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureMCPClients()
	return m.mcpClients[serverName]
}

// ToolCount 返回 MCP 客户端已发现的工具数量（供日志输出）。
func (c *MCPClient) ToolCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.tools)
}
