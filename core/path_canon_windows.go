//go:build windows

package core

import (
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

var (
	procGetFinalPathNameByHandleW = syscall.NewLazyDLL("kernel32.dll").NewProc("GetFinalPathNameByHandleW")
)

const (
	winOpenExisting         = syscall.OPEN_EXISTING
	winFileFlagBackupSemant = 0x02000000 // FILE_FLAG_BACKUP_SEMANTICS 允许打开目录句柄
	winVolumeNameDos        = 0x00000000 // VOLUME_NAME_DOS 返回 \\?\<drive>:\... 形式
)

// canonicalExistingOS Windows 实现：用 GetFinalPathNameByHandleW 取打开句柄的
// 真实最终路径（穿透 junction 与符号链接）。失败时回退 EvalSymlinks。
func canonicalExistingOS(p string) (string, error) {
	u16, err := syscall.UTF16PtrFromString(p)
	if err != nil {
		return filepath.EvalSymlinks(p)
	}
	h, err := syscall.CreateFile(u16, syscall.GENERIC_READ,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil, winOpenExisting, winFileFlagBackupSemant, 0)
	if err != nil {
		return filepath.EvalSymlinks(p)
	}
	defer syscall.CloseHandle(h)

	buf := make([]uint16, 4096)
	n, _, _ := procGetFinalPathNameByHandleW.Call(
		uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), winVolumeNameDos)
	if n == 0 || int(n) > len(buf) {
		return filepath.EvalSymlinks(p)
	}
	return normWinPath(syscall.UTF16ToString(buf[:n])), nil
}

// normWinPath 把 GetFinalPathNameByHandleW 的 \\?\<path> 形式归一化为普通路径。
func normWinPath(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, `\\?\UNC\`) {
		return `\\` + s[len(`\\?\UNC\`):]
	}
	if strings.HasPrefix(s, `\\?\`) {
		return s[len(`\\?\`):]
	}
	return s
}
