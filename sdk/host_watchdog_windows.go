//go:build windows

package dsc

import (
	"syscall"
)

// pidAlivePlatform Windows 实现：经 GetExitCodeProcess 检测。
// exit code == STILL_ACTIVE(259) 则存活。
func pidAlivePlatform(pid int) bool {
	const exitSTILL_ACTIVE = 259
	h, err := syscall.OpenProcess(
		syscall.STANDARD_RIGHTS_READ|syscall.PROCESS_QUERY_INFORMATION|syscall.SYNCHRONIZE,
		false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(h)
	var ec uint32
	if e := syscall.GetExitCodeProcess(h, &ec); e != nil {
		return false
	}
	return ec == exitSTILL_ACTIVE
}
