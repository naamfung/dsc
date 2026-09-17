package core

// POSIX 路径 API：filepath 包在 Windows 上会把路径归一化为原生反斜杠，违背
// 「内部 POSIX shell 统一以正斜杠处理路径输入输出」的约定（见 AGENTS.md
// 「路径归一化」与哨兵测试 filepath_guard_test.go）。本文件是核心包内唯一允许
// 直接调用被禁 filepath 函数的文件（守卫测试白名单），对外暴露 P* 系列：
// 先以原生 filepath 计算，再统一归一化为正斜杠——Windows 上 os.* 均接受正斜杠，
// 计算语义不变，而模型可见路径恒为正斜杠。插件经 SDK 二次转发（sdk/fs.go），
// 本仓其它模块直接用 core.P*；libs/sh、plugin 等独立模块（无法引入 dsc/core）
// 使用其本地等价实现。

import (
	"os"
	"path/filepath"
)

// PSeparator 统一路径分隔符（所有平台一律正斜杠）。
const PSeparator = "/"

// PJoin 等价 filepath.Join，结果归一化为正斜杠。
func PJoin(elem ...string) string {
	return filepath.ToSlash(filepath.Join(elem...))
}

// PAbs 等价 filepath.Abs，结果归一化为正斜杠。
func PAbs(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(abs), nil
}

// PClean 等价 filepath.Clean，结果归一化为正斜杠。
func PClean(path string) string {
	return filepath.ToSlash(filepath.Clean(path))
}

// PRel 等价 filepath.Rel，结果归一化为正斜杠。
func PRel(basepath, targpath string) (string, error) {
	rel, err := filepath.Rel(basepath, targpath)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

// PSplit 等价 filepath.Split，目录部分归一化为正斜杠。
func PSplit(path string) (dir, file string) {
	d, f := filepath.Split(path)
	return filepath.ToSlash(d), f
}

// PDir 等价 filepath.Dir，结果归一化为正斜杠（Windows 上 filepath.Dir 对
// 正斜杠输入也会返回反斜杠，故必须经本函数统一）。
func PDir(path string) string {
	return filepath.ToSlash(filepath.Dir(path))
}

// PEvalSymlinks 等价 filepath.EvalSymlinks，结果归一化为正斜杠。
func PEvalSymlinks(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(resolved), nil
}

// PGlob 等价 filepath.Glob，匹配结果逐条归一化为正斜杠。
func PGlob(pattern string) ([]string, error) {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	for i, m := range matches {
		matches[i] = filepath.ToSlash(m)
	}
	return matches, nil
}

// PWalkDir 等价 filepath.WalkDir，回调收到的 path 归一化为正斜杠。
func PWalkDir(root string, fn func(path string, d os.DirEntry, err error) error) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		return fn(filepath.ToSlash(path), d, err)
	})
}

// PWalk 等价 filepath.Walk，回调收到的 path 归一化为正斜杠。
func PWalk(root string, fn func(path string, info os.FileInfo, err error) error) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		return fn(filepath.ToSlash(path), info, err)
	})
}
