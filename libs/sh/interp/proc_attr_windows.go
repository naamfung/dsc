//go:build windows

package interp

import (
        "os/exec"
        "syscall"
)

// createNoWindow 对齐 Windows SDK 的 PROCESS_CREATION_FLAGS（同 golang.org/x/sys/windows
// 的 CREATE_NO_WINDOW）。标准库 syscall 未定义该常量，故本地声明。
const createNoWindow = 0x08000000

// hideChildConsole 设置 Windows 隐藏控制台启动参数。shell 解释器回退执行
// PATH 外部命令时，子进程缺省继承解释器进程的控制台；当继承不到控制台
// （GUI 拉起、计划任务启动、控制台已分离）时，console 子系统子进程会新建
// 可见控制台窗口，宿主 TUI 中表现为终端一闪而过。CREATE_NO_WINDOW 令
// 子进程拿到不可见控制台，杜绝新终端弹出；仅影响控制台分配，不影响
// stdin/stdout/stderr 管道，命令输出捕获行为不变。
func hideChildConsole(cmd *exec.Cmd) {
        cmd.SysProcAttr = &syscall.SysProcAttr{
                HideWindow:    true,
                CreationFlags: createNoWindow,
        }
}
