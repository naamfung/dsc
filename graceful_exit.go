package main

import (
	"os"

	"dsc/core"
)

// gracefulShutdown 在收到终止信号（Ctrl+C / 直接关闭终端 / SIGTERM 等）时对宿主做
// 清理后退出：先 mgr.Shutdown 逐只 Kill 所有插件子进程，再结束本进程。
// 与 main 尾部的 mgr.Shutdown() 幂等，可安全重复调用；用于堵住「直接关闭终端
// 导致孤儿插件进程残留」的漏洞（此时 defers 不会执行，需由信号处理显式收尸）。
func gracefulShutdown(mgr *core.Manager) {
	mgr.Shutdown()
	os.Exit(0)
}
