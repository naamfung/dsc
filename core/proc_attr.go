package core

import "os/exec"

// ConfigureChildProcessAttrs 为外部命令子进程配置跨平台启动属性：Windows 上
// 隐藏控制台（HideWindow + CREATE_NO_WINDOW），其余平台 no-op。
//
// 背景：宿主运行期会调用外部命令（LSP 服务器、外部钩子脚本、测评拉起 dsc 等），
// 子进程缺省继承宿主控制台；当宿主继承不到控制台（GUI 拉起、计划任务启动、
// 控制台已分离）时，console 子系统子进程会新建一个可见控制台窗口，用户在 TUI
// 中会看到终端一闪而过。Windows 上以 CREATE_NO_WINDOW 启动，子进程拿到的是
// 不可见控制台，杜绝新终端窗口弹出。CREATE_NO_WINDOW 仅影响控制台分配，不
// 影响 stdio 管道重定向，子进程输入输出行为不变。
//
// 所有 exec.Command / exec.CommandContext 创建的子进程都应调用本函数；go-plugin
// 插件子进程由 plugin/internal/cmdrunner 统一处理，无需各调用点重复设置。
func ConfigureChildProcessAttrs(cmd *exec.Cmd) {
	setChildProcSysProcAttr(cmd)
}
