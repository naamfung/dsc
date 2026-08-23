package tool

import (
	"fmt"
	"os"

	goplugin "github.com/hashicorp/go-plugin"

	"dsc/plugin"
)

var (
	globalRegistry *ToolRegistry
	globalMeta     *PluginMetadata
)

// InitMetadata 初始化插件元數據並創建註冊表
func InitMetadata(name, version, apiVersion string) {
	globalRegistry = &ToolRegistry{tools: make([]*ToolDef, 0)}
	globalMeta = &PluginMetadata{
		Type:       "tool",
		Name:       name,
		Version:    version,
		ApiVersion: apiVersion,
	}
}

// RegisterTool 註冊一個工具
func RegisterTool(t *ToolDef) {
	if globalRegistry == nil {
		panic("tool registry not initialized, call InitMetadata first")
	}
	globalRegistry.Register(t)
}

// Serve 啟動插件服務
func Serve() {
	if globalRegistry == nil || globalMeta == nil {
		panic("tool metadata not initialized, call InitMetadata first")
	}

	toolServer := NewToolServiceServer(globalRegistry)
	metadataServer := NewMetadataServer(globalMeta)

	pluginServer := &ToolMetadataGRPCPlugin{
		ToolImpl:     toolServer,
		MetadataImpl: metadataServer,
	}

	goplugin.Serve(&goplugin.ServeConfig{
		HandshakeConfig: plugin.Handshake,
		Plugins: map[string]goplugin.Plugin{
			"tool": pluginServer,
		},
		GRPCServer: goplugin.DefaultGRPCServer,
	})

	// 如果 Serve 返回，說明有錯誤
	fmt.Fprintf(os.Stderr, "plugin serve exited with error\n")
	os.Exit(1)
}
