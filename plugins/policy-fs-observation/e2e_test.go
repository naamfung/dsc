package main

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"

	"dsc/core"
	"dsc/proto"
	"dsc/proto/metadata"
	plugin "github.com/hashicorp/go-plugin"
)

// TestE2EWithHostClient 端到端验证 SDK 重写后的 policy 插件：
// 以宿主侧 go-core 客户端 spawn exe，经 gRPC 验证元数据（type=policy），
// 并经主连接直接调用 FsObservationPolicyService（Get/UpdateObservation）。
func TestE2EWithHostClient(t *testing.T) {
	// 1. 构建插件 exe（独立 module 的完整独立开发者路径）
	dir := t.TempDir()
	exe := filepath.Join(dir, "policy-fs-observation.exe")
	if out, err := exec.Command("go", "build", "-o", exe, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	// 2. 以宿主侧客户端拉起插件进程
	cmd := exec.Command(exe)
	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig:  core.Handshake,
		Plugins:          map[string]plugin.Plugin{},
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
		Cmd:              cmd,
	})
	defer client.Kill()

	rpcClient, err := client.Client()
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	grpcClient, ok := rpcClient.(*plugin.GRPCClient)
	if !ok {
		t.Fatalf("unexpected client type %T", rpcClient)
	}
	conn := grpcClient.Conn
	ctx := context.Background()

	// 3. 元数据（SDK 自动提供，Type 由 Config 决定）
	meta := metadata.NewPluginMetadataClient(conn)
	info, err := meta.GetInfo(ctx, &metadata.Empty{})
	if err != nil || info.Type != "policy" || info.Name != "fs-observation-policy" || info.ApiVersion != "1.0" {
		t.Fatalf("GetInfo = %+v, err %v", info, err)
	}

	// 4. 经主连接直接调用 FsObservationPolicyService（宿主 policy 桥接同款路径）
	pc := proto.NewFsObservationPolicyServiceClient(conn)

	// 初始未观测 → Found=false
	got, err := pc.GetObservation(ctx, &proto.GetObservationRequest{FilePath: "/tmp/a.go"})
	if err != nil || got.Found {
		t.Fatalf("GetObservation(未观测) = %+v, err %v", got, err)
	}

	// UpdateObservation → 再 Get 应命中并返回内容
	upd, err := pc.UpdateObservation(ctx, &proto.UpdateObservationRequest{
		FilePath: "/tmp/a.go",
		Observation: &proto.FsObservation{
			State:       "present",
			Version:     "v1",
			LastContent: "package a",
		},
	})
	if err != nil || !upd.Success {
		t.Fatalf("UpdateObservation = %+v, err %v", upd, err)
	}
	got2, err := pc.GetObservation(ctx, &proto.GetObservationRequest{FilePath: "/tmp/a.go"})
	if err != nil || !got2.Found || got2.Observation.LastContent != "package a" {
		t.Fatalf("GetObservation(已观测) = %+v, err %v", got2, err)
	}
}
