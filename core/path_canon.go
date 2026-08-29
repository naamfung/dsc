package core

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// 路径规范化：解析符号链接与 Windows junction/symlink 指向的真实路径。
//
// filepath.EvalSymlinks 在 Windows 上不解析指向目录的 junction/reparse point
// （实测对 junction 原路返回），而 Windows 上无需管理员即可创建 junction，
// 导致 workspace 内指向外部的 junction 可被用于「写穿」沙箱（P0-3）。
// 因此 Windows 上用 GetFinalPathNameByHandleW 取打开句柄的真实最终路径，
// 该 API 会穿透 junction/symlink 直达目标；Unix 上沿用 EvalSymlinks。

// canonicalExistingOS 解析已存在路径 p 的真实最终路径（穿透 junction/符号链接）。
// 由平台实现（build tag）：windows 用 GetFinalPathNameByHandleW；unix 用 EvalSymlinks。

// CanonicalPath 返回路径 p 的真实最终路径：存在时整条解析（穿透 junction/symlink）；
// 不存在时解析其最深的已存在祖先，再按剩余词法后缀拼接。供沙箱 containment 判定使用。
func CanonicalPath(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p, err
	}
	abs = filepath.Clean(abs)
	if _, err := os.Lstat(abs); err == nil {
		return canonicalExistingOS(abs)
	}
	dir := filepath.Dir(abs)
	for {
		if _, err := os.Lstat(dir); err == nil {
			realDir, err := canonicalExistingOS(dir)
			if err != nil {
				return abs, err
			}
			rel, rerr := filepath.Rel(dir, abs)
			if rerr != nil {
				return abs, nil
			}
			return filepath.Clean(filepath.Join(realDir, rel)), nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return abs, nil
		}
		dir = parent
	}
}

// containsPath 判断 sub 是否位于 base 之下（含相等）。Windows 大小写不敏感。
func containsPath(base, sub string) bool {
	base = filepath.Clean(base)
	sub = filepath.Clean(sub)
	if runtime.GOOS == "windows" {
		base = strings.ToLower(base)
		sub = strings.ToLower(sub)
	}
	if sub == base {
		return true
	}
	return strings.HasPrefix(sub, base+string(os.PathSeparator))
}
