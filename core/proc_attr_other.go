//go:build !windows

package core

import "os/exec"

// setChildProcSysProcAttr 非 Windows 平台无控制台窗口概念，no-op。
func setChildProcSysProcAttr(_ *exec.Cmd) {}
