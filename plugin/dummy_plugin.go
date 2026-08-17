package plugin

import (
	"context"

	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

// dummyGRPCPlugin 是一個佔位插件，用於獲取 gRPC 連接而不用 Dispense 特定服務
type dummyGRPCPlugin struct {
	plugin.Plugin
}

func (p *dummyGRPCPlugin) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	return nil
}

func (p *dummyGRPCPlugin) GRPCClient(ctx context.Context, broker *plugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return nil, nil
}
