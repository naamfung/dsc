//go:build !windows

package dsc

import (
	"os"
	"syscall"
)

// pidAlivePlatform POSIX 实现：Signal(0) 不发送信号，仅检查进程是否存在。
// 进程不存在返回 ESRCH，无权限返回 EPERM（视为存活——可能是权限不足）。
func pidAlivePlatform(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	if err == syscall.EPERM {
		return true // 进程存在但无权限发送信号
	}
	return false // ESRCH = 进程不存在
}
