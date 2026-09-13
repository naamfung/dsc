// Copyright IBM Corp. 2016, 2025
// SPDX-License-Identifier: MPL-2.0

//go:build !windows

package cmdrunner

import (
	"os"
	"syscall"
)

// sendTermination 优雅关闭第一阶：发 SIGTERM 信号给进程，让其有时间跑 defer /
// 关闭 DB / 写状态文件。返回是否成功发出（失败时调用方应直接走 SIGKILL 兜底）。
//
// 对齐 AGENTS.md 第7条宿主侧 graceful_shutdown 范式：先礼后兵，SIGTERM → 等待 → SIGKILL。
func sendTermination(proc *os.Process) error {
	return proc.Signal(syscall.SIGTERM)
}
