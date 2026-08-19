package main

import (
	"context"

	"dsc/proto"
	"dsc/proto/metadata"
	goplugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

// ToolMetadataGRPCPlugin 是實現了 goplugin.GRPCPlugin 接口的適配器，同時註冊 ToolService 和 PluginMetadata
type ToolMetadataGRPCPlugin struct {
	goplugin.Plugin
	ToolImpl     *ToolServiceServer
	MetadataImpl *MetadataServer
}

func (p *ToolMetadataGRPCPlugin) GRPCServer(broker *goplugin.GRPCBroker, s *grpc.Server) error {
	proto.RegisterToolServiceServer(s, p.ToolImpl)
	metadata.RegisterPluginMetadataServer(s, p.MetadataImpl)
	return nil
}

func (p *ToolMetadataGRPCPlugin) GRPCClient(ctx context.Context, broker *goplugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return nil, nil
}
