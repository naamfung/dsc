package main

import (
	"context"

	"dsc/proto"
	"dsc/proto/metadata"
	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

type ToolMetadataGRPCPlugin struct {
	plugin.NetRPCUnsupportedPlugin
	ToolImpl     proto.ToolServiceServer
	MetadataImpl metadata.PluginMetadataServer
}

func (p *ToolMetadataGRPCPlugin) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	proto.RegisterToolServiceServer(s, p.ToolImpl)
	metadata.RegisterPluginMetadataServer(s, p.MetadataImpl)
	return nil
}

func (p *ToolMetadataGRPCPlugin) GRPCClient(ctx context.Context, broker *plugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return &ToolMetadataGRPCClient{
		ToolClient:   proto.NewToolServiceClient(c),
		MetadataClient: metadata.NewPluginMetadataClient(c),
	}, nil
}

type ToolMetadataGRPCClient struct {
	ToolClient   proto.ToolServiceClient
	MetadataClient metadata.PluginMetadataClient
}

func (c *ToolMetadataGRPCClient) ExecuteTool(ctx context.Context, req *proto.ExecuteToolRequest, opts ...grpc.CallOption) (*proto.ExecuteToolResponse, error) {
	return c.ToolClient.ExecuteTool(ctx, req, opts...)
}

func (c *ToolMetadataGRPCClient) ListTools(ctx context.Context, req *proto.ListToolsRequest, opts ...grpc.CallOption) (*proto.ListToolsResponse, error) {
	return c.ToolClient.ListTools(ctx, req, opts...)
}

func (c *ToolMetadataGRPCClient) GetInfo(ctx context.Context, req *metadata.Empty, opts ...grpc.CallOption) (*metadata.PluginInfo, error) {
	return c.MetadataClient.GetInfo(ctx, req, opts...)
}
