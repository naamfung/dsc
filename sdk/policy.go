package dsc

import (
	"context"

	"dsc/proto"
	"dsc/proto/metadata"
	goplugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

// policyGRPCPlugin 是 policy 类型插件的 go-plugin 适配器：在主 gRPC server 上
// 注册宿主可桥接的策略服务（FsObservationPolicyService）+ 插件元数据。
// 宿主对 policy 类型经主连接直接取对应 proto 客户端（见 plugin/manager.go），
// 无需本插件提供自定义 GRPCClient。
type policyGRPCPlugin struct {
	goplugin.NetRPCUnsupportedPlugin
	sdk *SDK
}

func (p *policyGRPCPlugin) GRPCServer(broker *goplugin.GRPCBroker, s *grpc.Server) error {
	proto.RegisterFsObservationPolicyServiceServer(s, p.sdk.policy)
	metadata.RegisterPluginMetadataServer(s, &metadataServer{cfg: p.sdk.cfg})
	return nil
}

func (p *policyGRPCPlugin) GRPCClient(context.Context, *goplugin.GRPCBroker, *grpc.ClientConn) (interface{}, error) {
	return nil, nil
}
