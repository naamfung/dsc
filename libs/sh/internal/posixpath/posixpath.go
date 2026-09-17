// posixpath 提供 libs/sh（本仓维护的 mvdan/sh fork）内部的 POSIX 路径等价实现。
//
// libs/sh 是独立模块（mvdan.cc/sh/v3），无法引入 dsc/core 的 P* 系列，故在本
// 模块内提供同语义的本地等价物：先以原生 filepath 计算，再统一归一化为正斜杠
// （Windows 上 os.* 均接受正斜杠，计算语义不变）。此文件是 libs/sh 模块内唯一
// 允许直接调用被禁 filepath 函数的文件（core/filepath_guard_test.go 白名单）。
package posixpath

import (
	"os"
	"path/filepath"
)

// Separator 统一路径分隔符（所有平台一律正斜杠）。
const Separator = "/"

// Join 等价 filepath.Join，结果归一化为正斜杠。
func Join(elem ...string) string {
	return filepath.ToSlash(filepath.Join(elem...))
}

// Abs 等价 filepath.Abs，结果归一化为正斜杠。
func Abs(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(abs), nil
}

// Clean 等价 filepath.Clean，结果归一化为正斜杠。
func Clean(path string) string {
	return filepath.ToSlash(filepath.Clean(path))
}

// Split 等价 filepath.Split，目录部分归一化为正斜杠。
func Split(path string) (dir, file string) {
	d, f := filepath.Split(path)
	return filepath.ToSlash(d), f
}

// Dir 等价 filepath.Dir，结果归一化为正斜杠。
func Dir(path string) string {
	return filepath.ToSlash(filepath.Dir(path))
}

// EvalSymlinks 等价 filepath.EvalSymlinks，结果归一化为正斜杠。
func EvalSymlinks(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(resolved), nil
}

// WalkDir 等价 filepath.WalkDir，回调收到的 path 归一化为正斜杠。
func WalkDir(root string, fn func(path string, d os.DirEntry, err error) error) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		return fn(filepath.ToSlash(path), d, err)
	})
}
