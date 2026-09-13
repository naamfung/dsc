// Copyright IBM Corp. 2016, 2025
// SPDX-License-Identifier: MPL-2.0

//go:build windows

package cmdrunner

import "os"

// sendTermination Windows 占位实现：Windows 没有 SIGTERM 信号概念。
// 客户端 gRPC Shutdown RPC 已经触发服务端 Stop（见 grpc_controller.go），
// 这是 Windows 上的优雅关闭路径。此处返回 nil 不发信号，让调用方走
// GracefulShutdownTimeout 后兜底调用 proc.Kill()（= TerminateProcess）。
func sendTermination(_ *os.Process) error {
	return nil
}
