// Copyright IBM Corp. 2016, 2025
// SPDX-License-Identifier: MPL-2.0

//go:build !windows

package core

import "syscall"

// Unix 专属信号——Windows 上 syscall.SIGTERM 等常量虽存在但不会被内核发出，
// 故经 build tag 隔离，避免 Windows 上 signal.Notify 收到无效信号注册。
//
// 参见 server.go 信号处理段；对齐 AGENTS.md 第7条宿主侧 signal_unix.go 范式。
var (
	unixSigterm  = syscall.SIGTERM
	unixSighup   = syscall.SIGHUP
	unixSigquit  = syscall.SIGQUIT
)
