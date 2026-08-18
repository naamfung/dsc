package main

import (
	"context"
	"fmt"
	"sync"

	"dsc/plugin"
	"dsc/proto"
	"dsc/proto/metadata"
	goplugin "github.com/hashicorp/go-plugin"
)

// FsObservationPolicyServer 文件系統觀測策略服務服務端實現
type FsObservationPolicyServer struct {
	proto.UnimplementedFsObservationPolicyServiceServer
	observations map[string]*proto.FsObservation
	mu           sync.RWMutex
}

func NewFsObservationPolicyServer() *FsObservationPolicyServer {
	return &FsObservationPolicyServer{
		observations: make(map[string]*proto.FsObservation),
	}
}

func (s *FsObservationPolicyServer) GetObservation(ctx context.Context, req *proto.GetObservationRequest) (*proto.GetObservationResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obs, found := s.observations[req.FilePath]
	if !found {
		return &proto.GetObservationResponse{
			Found: false,
		}, nil
	}

	return &proto.GetObservationResponse{
		Found: true,
		Observation: &proto.FsObservation{
			State:       obs.State,
			Version:     obs.Version,
			LastContent: obs.LastContent,
		},
	}, nil
}

func (s *FsObservationPolicyServer) UpdateObservation(ctx context.Context, req *proto.UpdateObservationRequest) (*proto.UpdateObservationResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.observations[req.FilePath] = &proto.FsObservation{
		State:       req.Observation.State,
		Version:     req.Observation.Version,
		LastContent: req.Observation.LastContent,
	}

	return &proto.UpdateObservationResponse{
		Success: true,
		Message: fmt.Sprintf("Observation updated for file: %s", req.FilePath),
	}, nil
}

// MetadataServer 元數據服務服務端實現
type MetadataServer struct {
	metadata.UnimplementedPluginMetadataServer
}

func (m *MetadataServer) GetInfo(ctx context.Context, _ *metadata.Empty) (*metadata.PluginInfo, error) {
	return &metadata.PluginInfo{
		Type:       "fs-observation-policy",
		Name:       "fs-observation-policy",
		Version:    "1.0.0",
		ApiVersion: "1.0",
	}, nil
}

func main() {
	// 創建文件系統觀測策略服務服務端
	policyServer := NewFsObservationPolicyServer()

	// 創建元數據服務服務端
	metadataServer := &MetadataServer{}

	// 啟動插件服務
	goplugin.Serve(&goplugin.ServeConfig{
		HandshakeConfig: plugin.Handshake,
		Plugins: map[string]goplugin.Plugin{
			"fs_observation_policy": &FsObservationPolicyGRPCPlugin{
				PolicyImpl:     policyServer,
				MetadataImpl:   metadataServer,
			},
		},
		GRPCServer: goplugin.DefaultGRPCServer,
	})
}
