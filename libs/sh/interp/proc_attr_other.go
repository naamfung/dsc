//go:build !windows

package interp

import "os/exec"

// hideChildConsole 非 Windows 平台无控制台窗口概念，no-op。
func hideChildConsole(_ *exec.Cmd) {}
