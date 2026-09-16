package core

import (
	"context"
	"fmt"

	"dsc/proto/metadata"
	"google.golang.org/grpc"
)

// GetPluginInfo 通過已建立的 gRPC 連接獲取插件元數據
func GetPluginInfo(conn *grpc.ClientConn) (*metadata.PluginInfo, error) {
	client := metadata.NewPluginMetadataClient(conn)
	resp, err := client.GetInfo(context.Background(), &metadata.Empty{})
	if err != nil {
		return nil, fmt.Errorf("failed to get core info: %w", err)
	}
	return resp, nil
}

// hasService 报告插件元数据是否声明暴露指定服务（PluginInfo.services，
// 取值如 "policy"/"tool"/"hook"）。通用（dsc）类型插件据此获宿主机械桥接
// 对应服务；旧插件未填充时返回 false（宿主保持既有探测行为）。
func hasService(info *metadata.PluginInfo, service string) bool {
	if info == nil {
		return false
	}
	for _, s := range info.GetServices() {
		if s == service {
			return true
		}
	}
	return false
}
