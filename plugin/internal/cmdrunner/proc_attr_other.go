// Copyright IBM Corp. 2016, 2025
// SPDX-License-Identifier: MPL-2.0

//go:build !windows

package cmdrunner

import "os/exec"

// hideChildConsole 非 Windows 平台无控制台窗口概念，no-op。
func hideChildConsole(_ *exec.Cmd) {}
