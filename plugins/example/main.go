package main

import (
	"dsc/plugin"

	gp "github.com/hashicorp/go-plugin"
)

func main() {
	// 啟動插件服務
	gp.Serve(&gp.ServeConfig{
		HandshakeConfig: plugin.Handshake,
		Plugins: map[string]gp.Plugin{
			"dsc_plugin": &plugin.DSCPluginGRPC{Impl: &ExamplePlugin{}},
		},
		GRPCServer: gp.DefaultGRPCServer,
	})
}