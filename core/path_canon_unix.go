//go:build !windows

package core

import "path/filepath"

// canonicalExistingOS Unix 实现：EvalSymlinks 已由内核解析符号链接，
// dev/ino 由 open() 传递跟随，行为正确。
func canonicalExistingOS(p string) (string, error) {
	return filepath.EvalSymlinks(p)
}
