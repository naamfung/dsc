package dsc

import (
	"context"

	"dsc/proto/metadata"
)

// metadataServer 插件元数据服务：宿主按配置加载时校验类型/版本。
type metadataServer struct {
	metadata.UnimplementedPluginMetadataServer
	cfg Config
}

func (s *metadataServer) GetInfo(ctx context.Context, _ *metadata.Empty) (*metadata.PluginInfo, error) {
	return &metadata.PluginInfo{
		Type:       string(s.cfg.Type),
		Name:       s.cfg.Name,
		Version:    s.cfg.Version,
		ApiVersion: s.cfg.APIVersion,
	}, nil
}
