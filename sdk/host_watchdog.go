package dsc

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// hostLivenessWatchdog 宿主存活看门狗：插件进程定时检测宿主进程是否存活，
// 宿主崩溃/OOM/被强杀（任务管理器、系统关机、kill -9）时插件主动退出，
// 避免残留孤儿进程。
//
// 背景：go-plugin 的插件退出依赖宿主主动通知（gRPC Shutdown RPC 或
// listener close）。宿主异常退出时无法发通知，插件的 plugin.Serve 阻塞在
// doneCh 上不会自行退出。本看门狗补上「宿主失联则自杀」兜底。
//
// 检测方式：经内核级 PID 存活检测（POSIX Signal(0) / Windows
// GetExitCodeProcess），比 gRPC ping 更可靠（不受网络抖动影响）。
// 对齐 DSH Node.js 子进程的父进程 PID 监控（process.ppid + process.kill(ppid, 0)）。
//
// 参数：
//   - hostPID：宿主进程 PID（由宿主经 DSC_HOST_PID env 注入）
//   - interval：检测间隔（默认 5s）
//   - failureThreshold：连续失败阈值（默认 3 次，避免偶发误判）
//
// 调用方应在独立 goroutine 中运行（不阻塞 plugin.Serve）。
func hostLivenessWatchdog(hostPID int, interval time.Duration, failureThreshold int) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	failures := 0
	for range ticker.C {
		if pidAlive(hostPID) {
			failures = 0
			continue
		}
		failures++
		if failures >= failureThreshold {
			// 宿主进程已不存活（连续 N 次检测失败），主动退出。
			// 不走 os.Exit(0) 而用 os.Exit(1) 让宿主/系统可观察到非零退出码
			// （虽宿主已不在，但日志/CI 脚本可能捕获）。但考虑到这是「正常
			// 兜底退出」（宿主已死，插件跟随退出是预期行为），用 0 更合理。
			fmt.Fprintf(os.Stderr, "dsc-sdk: host process %d no longer alive, exiting\n", hostPID)
			os.Exit(0)
		}
	}
}

// pidAlive 检测指定 PID 的进程是否存活（跨平台）。
// POSIX：Signal(0)，见 host_watchdog_posix.go。
// Windows：GetExitCodeProcess，见 host_watchdog_windows.go。
func pidAlive(pid int) bool {
	return pidAlivePlatform(pid)
}

// startHostWatchdog 从 DSC_HOST_PID env 读取宿主 PID 并启动看门狗 goroutine。
// 无 DSC_HOST_PID env 或解析失败时静默跳过（兼容旧宿主不注入的场景）。
// 返回 true 表示已启动，false 表示无需启动（无 env）。
func startHostWatchdog() bool {
	pidStr := os.Getenv("DSC_HOST_PID")
	if pidStr == "" {
		return false
	}
	hostPID, err := strconv.Atoi(pidStr)
	if err != nil || hostPID <= 0 {
		return false
	}
	// 默认：5s 间隔，3 次连续失败才退出（总检测窗口 15s，平衡响应速度与抗抖动）
	go hostLivenessWatchdog(hostPID, 5*time.Second, 3)
	return true
}
