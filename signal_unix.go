//go:build !windows

package main

import (
	"os"
	"os/signal"
	"syscall"

	"dsc/core"
)

// installShutdownSignals 安装退出信号处理：SIGINT / SIGTERM / SIGHUP（终端/会话关闭）/
// SIGQUIT 到达时先 gracefulShutdown（逐只 Kill 插件子进程）再退出，
// 堵住「直接关闭终端导致孤儿插件进程残留」的漏洞。
func installShutdownSignals(mgr *core.Manager) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
	go func() {
		<-ch
		gracefulShutdown(mgr)
	}()
}
