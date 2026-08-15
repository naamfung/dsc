package plugin

import (
	"context"
	"fmt"

	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"

	"dsc/proto"
)

// DSCPluginGRPC 是實現了 plugin.GRPCPlugin 接口的適配器
type DSCPluginGRPC struct {
	plugin.Plugin
	Impl DSCPlugin
}

// GRPCServer 返回 gRPC 服務端
func (p *DSCPluginGRPC) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	proto.RegisterDSCPluginServiceServer(s, &grpcServer{impl: p.Impl})
	return nil
}

// GRPCClient 返回 gRPC 客戶端
func (p *DSCPluginGRPC) GRPCClient(ctx context.Context, broker *plugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return &grpcClient{client: proto.NewDSCPluginServiceClient(c)}, nil
}

// grpcServer 是 gRPC 服務端實現
type grpcServer struct {
	proto.UnimplementedDSCPluginServiceServer
	impl DSCPlugin
}

func (s *grpcServer) Name(ctx context.Context, req *proto.NameRequest) (*proto.NameResponse, error) {
	return &proto.NameResponse{Name: s.impl.Name(ctx)}, nil
}

func (s *grpcServer) Version(ctx context.Context, req *proto.VersionRequest) (*proto.VersionResponse, error) {
	return &proto.VersionResponse{Version: s.impl.Version(ctx)}, nil
}

func (s *grpcServer) Execute(ctx context.Context, req *proto.ExecuteRequest) (*proto.ExecuteResponse, error) {
	execReq := &ExecuteRequest{
		Input:  req.Input,
		Params: req.Params,
	}
	resp, err := s.impl.Execute(ctx, execReq)
	if err != nil {
		return nil, err
	}
	return &proto.ExecuteResponse{
		Output:  resp.Output,
		Status:  resp.Status,
		Message: resp.Message,
	}, nil
}

func (s *grpcServer) HealthCheck(ctx context.Context, req *proto.HealthCheckRequest) (*proto.HealthCheckResponse, error) {
	err := s.impl.HealthCheck(ctx)
	status := "okay"
	msg := ""
	if err != nil {
		status = "error"
		msg = err.Error()
	}
	return &proto.HealthCheckResponse{
		Status:  status,
		Message: msg,
	}, nil
}

// grpcClient 是 gRPC 客戶端代理實現
type grpcClient struct {
	client proto.DSCPluginServiceClient
}

func (c *grpcClient) Name(ctx context.Context) string {
	resp, err := c.client.Name(ctx, &proto.NameRequest{})
	if err != nil {
		return ""
	}
	return resp.Name
}

func (c *grpcClient) Version(ctx context.Context) string {
	resp, err := c.client.Version(ctx, &proto.VersionRequest{})
	if err != nil {
		return ""
	}
	return resp.Version
}

func (c *grpcClient) Execute(ctx context.Context, req *ExecuteRequest) (*ExecuteResponse, error) {
	protoReq := &proto.ExecuteRequest{
		Input:  req.Input,
		Params: req.Params,
	}
	resp, err := c.client.Execute(ctx, protoReq)
	if err != nil {
		return nil, err
	}
	return &ExecuteResponse{
		Output:  resp.Output,
		Status:  resp.Status,
		Message: resp.Message,
	}, nil
}

func (c *grpcClient) HealthCheck(ctx context.Context) error {
	resp, err := c.client.HealthCheck(ctx, &proto.HealthCheckRequest{})
	if err != nil {
		return err
	}
	if resp.Status != "okay" {
		return fmt.Errorf("health check failed: %s", resp.Message)
	}
	return nil
}