package main

import (
	"context"
	"fmt"
	"sync"

	"dsc-sdk"
	"dsc/proto"
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

// main 以公共 SDK（dsc-sdk）声明式启动：SDK 自动提供 FsObservationPolicyService
// 与 PluginMetadata 的 go-core 组装（重写自旧的
// FsObservationPolicyGRPCPlugin/MetadataServer 样板）。
func main() {
	policyServer := NewFsObservationPolicyServer()

	sdk := dsc.New(dsc.Config{
		Name:    "fs-observation-policy",
		Version: "1.0.0",
		Type:    dsc.TypePolicy,
		// 声明提供 "fs-observation-policy" 能力：其他插件（如编辑器类工具）若需
		// 经策略服务观测文件变更可经 Requires 声明依赖，宿主据此按能力匹配（而非
		// 插件名）。对齐 DSH/Cordis 的 provide + inject 能力边界模型。
		Provides: map[string]string{"fs-observation-policy": "true"},
	})
	sdk.Policy(policyServer)
	sdk.Serve()
}
