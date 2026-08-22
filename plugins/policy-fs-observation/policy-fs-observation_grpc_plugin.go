package main

import (
	"context"

	"dsc/proto"
	"dsc/proto/metadata"
	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

type FsObservationPolicyGRPCPlugin struct {
	plugin.NetRPCUnsupportedPlugin
	PolicyImpl   proto.FsObservationPolicyServiceServer
	MetadataImpl metadata.PluginMetadataServer
}

func (p *FsObservationPolicyGRPCPlugin) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	proto.RegisterFsObservationPolicyServiceServer(s, p.PolicyImpl)
	metadata.RegisterPluginMetadataServer(s, p.MetadataImpl)
	return nil
}

func (p *FsObservationPolicyGRPCPlugin) GRPCClient(ctx context.Context, broker *plugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return &FsObservationPolicyGRPCClient{
		PolicyClient:   proto.NewFsObservationPolicyServiceClient(c),
		MetadataClient: metadata.NewPluginMetadataClient(c),
	}, nil
}

type FsObservationPolicyGRPCClient struct {
	PolicyClient   proto.FsObservationPolicyServiceClient
	MetadataClient metadata.PluginMetadataClient
}

func (c *FsObservationPolicyGRPCClient) GetObservation(ctx context.Context, req *proto.GetObservationRequest, opts ...grpc.CallOption) (*proto.GetObservationResponse, error) {
	return c.PolicyClient.GetObservation(ctx, req, opts...)
}

func (c *FsObservationPolicyGRPCClient) UpdateObservation(ctx context.Context, req *proto.UpdateObservationRequest, opts ...grpc.CallOption) (*proto.UpdateObservationResponse, error) {
	return c.PolicyClient.UpdateObservation(ctx, req, opts...)
}

func (c *FsObservationPolicyGRPCClient) GetInfo(ctx context.Context, req *metadata.Empty, opts ...grpc.CallOption) (*metadata.PluginInfo, error) {
	return c.MetadataClient.GetInfo(ctx, req, opts...)
}
