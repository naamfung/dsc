package dsc

import (
	"os/exec"

	"dsc/core"
)

// ConfigureChildProcessAttrs 为外部命令子进程配置跨平台启动属性：Windows 上
// 隐藏控制台（HideWindow + CREATE_NO_WINDOW），其余平台 no-op。
//
// 插件需要调用外部命令时（如外部渲染器、转换器、辅助进程），应在
// exec.Command / exec.CommandContext 创建后调用本函数：当插件进程继承不到
// 控制台（宿主以 GUI/计划任务方式启动等）时，console 子系统子进程会新建
// 可见控制台窗口，用户在 TUI 中会看到终端一闪而过；CREATE_NO_WINDOW 令
// 子进程拿到不可见控制台，杜绝新终端弹出。仅影响控制台分配，不影响
// stdio 管道，命令输出捕获行为不变。
func ConfigureChildProcessAttrs(cmd *exec.Cmd) {
	core.ConfigureChildProcessAttrs(cmd)
}
