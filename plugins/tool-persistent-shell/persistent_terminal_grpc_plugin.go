package main

import (
	"context"

	"dsc/proto"
	"dsc/proto/metadata"
	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

type PersistentTerminalGRPCPlugin struct {
	plugin.NetRPCUnsupportedPlugin
	TerminalImpl   proto.PersistentTerminalServiceServer
	MetadataImpl   metadata.PluginMetadataServer
}

func (p *PersistentTerminalGRPCPlugin) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	proto.RegisterPersistentTerminalServiceServer(s, p.TerminalImpl)
	metadata.RegisterPluginMetadataServer(s, p.MetadataImpl)
	return nil
}

func (p *PersistentTerminalGRPCPlugin) GRPCClient(ctx context.Context, broker *plugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return &PersistentTerminalGRPCClient{
		TerminalClient: proto.NewPersistentTerminalServiceClient(c),
		MetadataClient: metadata.NewPluginMetadataClient(c),
	}, nil
}

type PersistentTerminalGRPCClient struct {
	TerminalClient proto.PersistentTerminalServiceClient
	MetadataClient metadata.PluginMetadataClient
}

func (c *PersistentTerminalGRPCClient) CreateSession(ctx context.Context, req *proto.CreateSessionRequest, opts ...grpc.CallOption) (*proto.CreateSessionResponse, error) {
	return c.TerminalClient.CreateSession(ctx, req, opts...)
}

func (c *PersistentTerminalGRPCClient) ExecSession(ctx context.Context, req *proto.ExecSessionRequest, opts ...grpc.CallOption) (*proto.ExecSessionResponse, error) {
	return c.TerminalClient.ExecSession(ctx, req, opts...)
}

func (c *PersistentTerminalGRPCClient) CloseSession(ctx context.Context, req *proto.CloseSessionRequest, opts ...grpc.CallOption) (*proto.CloseSessionResponse, error) {
	return c.TerminalClient.CloseSession(ctx, req, opts...)
}

func (c *PersistentTerminalGRPCClient) GetInfo(ctx context.Context, req *metadata.Empty, opts ...grpc.CallOption) (*metadata.PluginInfo, error) {
	return c.MetadataClient.GetInfo(ctx, req, opts...)
}
