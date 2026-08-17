package main

import (
	"context"

	"dsc/plugin"
	"dsc/proto"
	"dsc/proto/metadata"
	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

// ToolMetadataGRPCPlugin 是實現了 plugin.GRPCPlugin 接口的適配器，同時註冊 ToolService 和 PluginMetadata
type ToolMetadataGRPCPlugin struct {
	plugin.Plugin
	ToolImpl   *ToolServiceServer
	MetadataImpl *MetadataServer
}

func (p *ToolMetadataGRPCPlugin) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	proto.RegisterToolServiceServer(s, p.ToolImpl)
	metadata.RegisterPluginMetadataServer(s, p.MetadataImpl)
	return nil
}

func (p *ToolMetadataGRPCPlugin) GRPCClient(ctx context.Context, broker *plugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return nil, nil
}
