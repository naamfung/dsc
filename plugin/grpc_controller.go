// Copyright IBM Corp. 2016, 2025
// SPDX-License-Identifier: MPL-2.0

package core

import (
	"context"

	"github.com/hashicorp/go-plugin/internal/plugin"
)

// GRPCControllerServer handles shutdown calls to terminate the server when the
// core client is closed.
type grpcControllerServer struct {
	server *GRPCServer
}

// Shutdown stops the grpc server.
//
// 当前实现使用 s.server.Stop() 而非 GracefulStop()。原因：GracefulStop 在 RPC handler
// 内同步调用会自等自身（Shutdown RPC 自身是 in-flight RPC 之一，GracefulStop 等待其
// 完成才能继续，但 RPC handler 必须先返回才算"完成"——死锁）；即使异步触发，
// GracefulStop 仍会等待 Shutdown RPC 之外的其它 in-flight RPC（如 reflection /
// health-check streaming），实测超过 2s（见 client.go:560 的硬编码 grace 期），
// 导致 TestClient_reattachGRPC 等测试 SIGKILL 后退出、被判 "killed"。
//
// 优雅关闭的真正实现路径：
//  1. 宿主侧 client.Kill 先发 Shutdown RPC（→ 此处 Stop 立即生效），插件进程结束；
//  2. 在 Stop 之前，宿主侧已在 SIGTERM 兜底前给插件一个窗口（plugin/server.go
//     的 SIGTERM handler 异步触发 GracefulStop——此时无 Shutdown RPC 自等死锁问题，
//     因 SIGTERM 不是 RPC 调用）；
//  3. 兜底：2s 后 SIGKILL。
//
// 即：宿主主动调 Shutdown RPC 走 Stop（快速路径）；宿主直接发信号 SIGTERM 走
// GracefulStop（优雅路径，给 in-flight RPC 排空机会）。两条路径互补。
func (s *grpcControllerServer) Shutdown(ctx context.Context, _ *core.Empty) (*core.Empty, error) {
	resp := &core.Empty{}
	s.server.Stop()
	return resp, nil
}
