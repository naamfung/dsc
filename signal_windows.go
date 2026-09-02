//go:build windows

package main

import (
	"syscall"

	"dsc/core"
	"golang.org/x/sys/windows"
)

// shutdownMgr 供控制台事件回调使用：回调签名固定、无法接收参数，只能经包级变量取宿主 manager。
var shutdownMgr *core.Manager

// ctrlHandler 保留 SetConsoleCtrlHandler 注册的原生回调指针（包级持有，避免被 GC 回收）。
var ctrlHandler = windows.NewCallback(consoleCtrlHandler)

// consoleCtrlHandler 处理 Windows 控制台事件：任何控制台信号（Ctrl+C / Ctrl+Break /
// console 关闭（点窗口 X）/ 注销 / 关机）都意味着本进程应当退出，故统一先
// gracefulShutdown 收尸插件子进程再退出，并返回 1 表明已处理，避免 OS 立即强杀
// 留不出收尸时间。回调须为「零参 + 单个 uint 大小返回值」方能经 NewCallback 编译。
func consoleCtrlHandler() uintptr {
	gracefulShutdown(shutdownMgr)
	return 1
}

// installShutdownSignals 安装 Windows 退出信号处理：console 关闭（点窗口 X）等事件
// 无法被 os/signal 捕获（Go 默认直接终止、不跑 defer），须用 kernel32.SetConsoleCtrlHandler
// 拦截，使宿主关闭前先 mgr.Shutdown 逐只 Kill 插件子进程，避免孤儿进程残留。
// x/sys/windows 已移除 SetConsoleCtrlHandler，故此处经 syscall 直接 P/Invoke。
func installShutdownSignals(mgr *core.Manager) {
	shutdownMgr = mgr
	proc := syscall.NewLazyDLL("kernel32.dll").NewProc("SetConsoleCtrlHandler")
	// 注册失败不阻塞启动；退化为 OS 默认行为（关闭终端时可能残留孤儿进程）
	proc.Call(ctrlHandler, 1)
}
