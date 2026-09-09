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
