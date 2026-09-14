//go:build windows

package core

import (
        "os/exec"
        "syscall"
)

// createNoWindow 对齐 Windows SDK 的 PROCESS_CREATION_FLAGS（同 golang.org/x/sys/windows
// 的 CREATE_NO_WINDOW）：子进程拿不可见控制台而非新建可见窗口。标准库 syscall
// 未定义该常量，故本地声明。
const createNoWindow = 0x08000000

// setChildProcSysProcAttr Windows 实现：隐藏子进程控制台窗口。
// 调用方已自定义 SysProcAttr 时不覆盖（如需合并 CreationFlags 由调用方自行处理）。
func setChildProcSysProcAttr(cmd *exec.Cmd) {
        if cmd.SysProcAttr != nil {
                return
        }
        cmd.SysProcAttr = &syscall.SysProcAttr{
                HideWindow:    true,
                CreationFlags: createNoWindow,
        }
}
