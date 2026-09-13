// Copyright IBM Corp. 2016, 2025
// SPDX-License-Identifier: MPL-2.0

//go:build windows

package core

import "os"

// Windows 无 Unix 信号概念——SIGTERM/SIGHUP/SIGQUIT 在 Windows 上不会被内核发出。
// 用 os.Signal(nil) 占位，使 signal.Notify(ch, nil) 在 Windows 上不注册任何
// Unix 信号，只保留 os.Interrupt（与原行为一致，避免回归）。
//
// Windows 上插件优雅关闭由宿主侧 client.Close() 直接发 gRPC Shutdown RPC 触发
// （见 grpc_controller.Shutdown → 异步 GracefulStop），不依赖信号。
var (
	unixSigterm = os.Signal(nil)
	unixSighup  = os.Signal(nil)
	unixSigquit = os.Signal(nil)
)
