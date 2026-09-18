package main

import (
	"context"
	"fmt"

	"dsc/core"
	"github.com/hashicorp/go-hclog"
)

// autoConnectMCPFromConfig 扫描 merged 配置中含 config.mcp 的插件条目，
// 启动时自动连接对应的 MCP 服务器（对齐 DSH cordis.yml 的 mcp-client
// 插件实例化——配置即声明，启动即连接）。
//
// patch 文件格式（经 -patch 注入）：
//
//   - name: mcp-memory
//     type: dsc
//     config:
//     mcp:
//     server_name: memory
//     endpoint: http://localhost:3000/mcp
//     # 或 stdio 传输：
//     # transport: stdio
//     # command: mcp-server-memory
//     # args: ["--port", "3000"]
//
// 每个条目的 config.mcp 字段含 server_name（必填）与 endpoint（HTTP）或
// transport: stdio + command + args（stdio，当前仅支持 HTTP，stdio 留待后续）。
func autoConnectMCPFromConfig(mgr *core.Manager, cfg *core.Config, logger hclog.Logger) {
	if cfg == nil || len(cfg.Plugins) == 0 {
		return
	}
	for _, entry := range cfg.Plugins {
		if !entry.Enabled || len(entry.Config) == 0 {
			continue
		}
		mcpCfg, ok := entry.Config["mcp"].(map[string]any)
		if !ok {
			continue
		}
		serverName, _ := mcpCfg["server_name"].(string)
		if serverName == "" {
			logger.Warn("patch MCP config missing server_name, skipping", "plugin", entry.Name)
			continue
		}
		endpoint, _ := mcpCfg["endpoint"].(string)
		transport, _ := mcpCfg["transport"].(string)
		if transport == "stdio" {
			// stdio 传输需要进程管理（spawn 子进程 + JSON-RPC over stdin/stdout）
			// 当前 MCPClient 仅支持 HTTP/SSE，stdio 留待后续扩展
			logger.Warn("patch MCP stdio transport not yet supported, skipping",
				"plugin", entry.Name, "server_name", serverName)
			continue
		}
		if endpoint == "" {
			logger.Warn("patch MCP config missing endpoint (HTTP transport), skipping",
				"plugin", entry.Name, "server_name", serverName)
			continue
		}
		// 经 admin API 的同一 Manager 路径连接（复用 ensureMCPClients + NewMCPClient + Connect）
		client, err := core.NewMCPClient(mgr, serverName, endpoint)
		if err != nil {
			logger.Error("patch MCP connect failed",
				"plugin", entry.Name, "server_name", serverName, "error", err)
			continue
		}
		if err := client.Connect(context.Background()); err != nil {
			logger.Error("patch MCP connect failed",
				"plugin", entry.Name, "server_name", serverName, "endpoint", endpoint, "error", err)
			continue
		}
		// 注册到 Manager.mcpClients（与 admin API /mcp/connect 同款路径）
		mgr.RegisterMCPClient(serverName, client)
		logger.Info("patch MCP connected",
			"plugin", entry.Name, "server_name", serverName,
			"endpoint", endpoint, "tools", fmt.Sprintf("%d", client.ToolCount()))
	}
}
