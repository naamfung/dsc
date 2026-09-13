// Copyright IBM Corp. 2016, 2025
// SPDX-License-Identifier: MPL-2.0

package runner

import (
	"context"
	"io"
)

// Runner defines the interface required by go-core to manage the lifecycle of
// of a core and attempt to negotiate a connection with it. Note that this
// is orthogonal to the protocol and transport used, which is negotiated over stdout.
type Runner interface {
	// Start should start the core and ensure any work required for servicing
	// other interface methods is done. If the context is cancelled, it should
	// only abort any attempts to _start_ the core. Waiting and shutdown are
	// handled separately.
	Start(ctx context.Context) error

	// Diagnose makes a best-effort attempt to return any debug information that
	// might help users understand why a core failed to start and negotiate a
	// connection.
	Diagnose(ctx context.Context) string

	// Stdout is used to negotiate the go-core protocol.
	Stdout() io.ReadCloser

	// Stderr is used for forwarding core logs to the host process logger.
	Stderr() io.ReadCloser

	// Name is a human-friendly name for the core, such as the path to the
	// executable. It does not have to be unique.
	Name() string

	AttachedRunner
}

// AttachedRunner defines a limited subset of Runner's interface to represent the
// reduced responsibility for core lifecycle when attaching to an already running
// core.
type AttachedRunner interface {
	// Wait should wait until the core stops running, whether in response to
	// an out of band signal or in response to calling Kill().
	Wait(ctx context.Context) error

	// Kill should stop the core and perform any cleanup required.
	Kill(ctx context.Context) error

	// ID is a unique identifier to represent the running core. e.g. pid or
	// container ID.
	ID() string

	AddrTranslator
}

// AddrTranslator translates addresses between the execution context of the host
// process and the core. For example, if the core is in a container, the file
// path for a Unix socket may be different between the host and the container.
//
// It is only intended to be used by the host process.
type AddrTranslator interface {
	// Called before connecting on any addresses received back from the core.
	PluginToHost(coreNet, coreAddr string) (hostNet string, hostAddr string, err error)

	// Called on any host process addresses before they are sent to the core.
	HostToPlugin(hostNet, hostAddr string) (coreNet string, coreAddr string, err error)
}

// GracefulRunner 是可选的 Runner 扩展：实现者支持优雅关闭——
// GracefulKill 发出 SIGTERM（Unix）/ 不做事（Windows），让插件进程有机会跑 defer、
// 排空 in-flight RPC、写状态文件，然后才由 client.Kill 走 gRPC Shutdown RPC + 兜底
// SIGKILL 路径。
//
// 实现者：cmdrunner.CmdRunner / cmdrunner.CmdAttachedRunner（经 process_posix.go
// 的 sendTermination 发 SIGTERM；Windows 上 signal_windows.go 返回 nil）。
//
// 未实现此接口的 Runner 仍走原路径：gRPC Shutdown RPC → grace → SIGKILL。
type GracefulRunner interface {
	// GracefulKill 发出 SIGTERM（或等价信号），让插件进程开始优雅退出。
	// 不等待进程退出——返回后调用方按 GracefulShutdownTimeout 决定是否兜底强杀。
	GracefulKill() error
}

// ReattachFunc can be passed to a client's reattach config to reattach to an
// already running core instead of starting it ourselves.
type ReattachFunc func() (AttachedRunner, error)
