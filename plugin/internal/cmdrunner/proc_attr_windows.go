// Copyright IBM Corp. 2016, 2025
// SPDX-License-Identifier: MPL-2.0

//go:build windows

package cmdrunner

import (
	"os/exec"
	"syscall"
)

// createNoWindow 对齐 Windows SDK 的 PROCESS_CREATION_FLAGS（同 golang.org/x/sys/windows
// 的 CREATE_NO_WINDOW）。标准库 syscall 未定义该常量，故本地声明。
const createNoWindow = 0x08000000

// hideChildConsole 设置 Windows 隐藏控制台启动参数。插件子进程是管道化 gRPC
// 服务（stdio 经 go-plugin 管道转发），不需要可见控制台；缺省创建时子进程
// 继承宿主控制台，而当宿主继承不到控制台（GUI 拉起、计划任务启动、控制台
// 已分离）时，console 子系统子进程会新建可见控制台窗口，在 TUI 上一闪而过。
// CREATE_NO_WINDOW 令子进程拿到不可见控制台，杜绝窗口闪烁；仅影响控制台
// 分配，不影响 stdio 管道，握手与 RPC 行为不变。
func hideChildConsole(cmd *exec.Cmd) {
	if cmd.SysProcAttr != nil {
		return // 调用方已自定义进程属性，不覆盖
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}
