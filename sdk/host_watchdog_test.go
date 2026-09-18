package dsc

import (
	"os"
	"testing"
	"time"
)

func TestPidAliveSelf(t *testing.T) {
	if !pidAlive(os.Getpid()) {
		t.Fatal("pidAlive(self) should be true")
	}
}

func TestPidAliveNonexistent(t *testing.T) {
	// 取一个几乎肯定不存在的 PID（max pid 通常是 32768~4194304）
	if pidAlive(99999999) {
		t.Fatal("pidAlive(99999999) should be false")
	}
}

func TestStartHostWatchdogNoEnv(t *testing.T) {
	t.Setenv("DSC_HOST_PID", "")
	if startHostWatchdog() {
		t.Fatal("startHostWatchdog should return false when DSC_HOST_PID is empty")
	}
}

func TestStartHostWatchdogInvalidEnv(t *testing.T) {
	t.Setenv("DSC_HOST_PID", "not-a-number")
	if startHostWatchdog() {
		t.Fatal("startHostWatchdog should return false for invalid DSC_HOST_PID")
	}
	t.Setenv("DSC_HOST_PID", "0")
	if startHostWatchdog() {
		t.Fatal("startHostWatchdog should return false for PID 0")
	}
	t.Setenv("DSC_HOST_PID", "-1")
	if startHostWatchdog() {
		t.Fatal("startHostWatchdog should return false for negative PID")
	}
}

// TestStartHostWatchdogWithLivePID 验证看门狗对存活进程不退出。
// 用当前进程 PID 作为 host PID（自己检测自己），watchdog 应持续运行不退出。
func TestStartHostWatchdogWithLivePID(t *testing.T) {
	t.Setenv("DSC_HOST_PID", "")
	// 启动 watchdog 检测自己（应该一直存活）
	go hostLivenessWatchdog(os.Getpid(), 100*time.Millisecond, 3)
	// 等 500ms 确保看门狗跑了几个 tick 但进程没退出
	time.Sleep(500 * time.Millisecond)
	// 如果到这里说明看门狗没误杀，测试通过
}
