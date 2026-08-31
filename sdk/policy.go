package dsc

import (
	"context"

	"dsc/proto"
	"dsc/proto/metadata"
	plugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

// policyGRPCPlugin 是 policy 类型插件的 go-core 适配器：在主 gRPC server 上
// 注册宿主可桥接的策略服务（FsObservationPolicyService）+ 插件元数据。
// 宿主对 policy 类型经主连接直接取对应 proto 客户端（见 core/manager.go），
// 无需本插件提供自定义 GRPCClient。
type policyGRPCPlugin struct {
	plugin.NetRPCUnsupportedPlugin
	sdk *SDK
}

func (p *policyGRPCPlugin) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	proto.RegisterFsObservationPolicyServiceServer(s, p.sdk.policy)
	metadata.RegisterPluginMetadataServer(s, &metadataServer{cfg: p.sdk.cfg})
	// 任何插件类型都可声明 Hook 订阅宿主事件（对齐 DSH cordis：事件广播类型无关）
	proto.RegisterPluginHookServiceServer(s, &hookServiceServer{hook: p.sdk.hook})
	return nil
}

func (p *policyGRPCPlugin) GRPCClient(context.Context, *plugin.GRPCBroker, *grpc.ClientConn) (interface{}, error) {
	return nil, nil
}
