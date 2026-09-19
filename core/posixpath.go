package core

// POSIX 路径 API：filepath 包在 Windows 上会把路径归一化为原生反斜杆，违背
// 「内部 POSIX shell 统一以正斜杆处理路径输入输出」的约定（见 AGENTS.md
// 「路径归一化」与哨兵测试 filepath_guard_test.go）。本文件是核心包内唯一允许
// 直接调用被禁 filepath 函数的文件（守卫测试白名单），对外暴露 P* 系列：
// 先以原生 filepath 计算，再统一归一化为正斜杆——Windows 上 os.* 均接受正斜杆，
// 计算语义不变，而模型可见路径恒为正斜杆。插件经 SDK 二次转发（sdk/fs.go），
// 本仓其它模块直接用 core.P*；libs/sh、plugin 等独立模块（无法引入 dsc/core）
// 使用其本地等价实现。
//
// toSlash 用 strings.ReplaceAll 而非 filepath.ToSlash：后者在 Linux 上是 no-op
// （Separator == '/'，替换 '/' → '/' 不变），无法在 Linux 测试中验证 Windows
// 反斜杆归一化。改用 strings.ReplaceAll 让 P* 在所有平台上都把反斜杆转正斜杆，
// 使 Linux CI 能真正测出 Windows 反斜杆污染路径链路的回归。

import (
        "os"
        "path/filepath"
        "strings"
)

// PSeparator 统一路径分隔符（所有平台一律正斜杆）。
const PSeparator = "/"

// toSlash 把路径中的反斜杆统一为正斜杆——跨平台一致行为。
// 对齐 AGENTS.md §10「路径归一化：禁止使用 filepath.ToSlash」红线：
// 必须两行连续替换，先处理双反斜杆（反引号 raw string `\\` = 两个反斜杆字符），
// 再处理单反斜杆（双引号 "\\\\" 转义后 = 一个反斜杆字符）。顺序不可颠倒——
// 先处理双反斜杆避免被第 2 行拆成两个单反斜杆后各自转换产生多余的 `/`。
//
// 不用 filepath.ToSlash 是因为后者在 Linux 上是 no-op（Separator == '/'，
// 替换 '/' → '/' 不变），无法转换 Windows 反斜杆路径。
func toSlash(path string) string {
        s := strings.ReplaceAll(path, `\\`, "/")  // 先：双反斜杆（raw string，两个 \ 字符）
        s = strings.ReplaceAll(s, "\\", "/")      // 后：单反斜杆（转义后一个 \ 字符）
        return s
}

// PJoin 等价 filepath.Join，结果归一化为正斜杆。
func PJoin(elem ...string) string {
        return toSlash(filepath.Join(elem...))
}

// PAbs 等价 filepath.Abs，结果归一化为正斜杆。
func PAbs(path string) (string, error) {
        abs, err := filepath.Abs(path)
        if err != nil {
                return "", err
        }
        return toSlash(abs), nil
}

// PClean 等价 filepath.Clean，结果归一化为正斜杆。
func PClean(path string) string {
        return toSlash(filepath.Clean(path))
}

// PRel 等价 filepath.Rel，结果归一化为正斜杆。
func PRel(basepath, targpath string) (string, error) {
        rel, err := filepath.Rel(basepath, targpath)
        if err != nil {
                return "", err
        }
        return toSlash(rel), nil
}

// PSplit 等价 filepath.Split，目录部分归一化为正斜杆。
func PSplit(path string) (dir, file string) {
        d, f := filepath.Split(path)
        return toSlash(d), f
}

// PDir 等价 filepath.Dir，结果归一化为正斜杆（Windows 上 filepath.Dir 对
// 正斜杆输入也会返回反斜杆，故必须经本函数统一）。
func PDir(path string) string {
        return toSlash(filepath.Dir(path))
}

// PEvalSymlinks 等价 filepath.EvalSymlinks，结果归一化为正斜杆。
func PEvalSymlinks(path string) (string, error) {
        resolved, err := filepath.EvalSymlinks(path)
        if err != nil {
                return "", err
        }
        return toSlash(resolved), nil
}

// PGlob 等价 filepath.Glob，匹配结果逐条归一化为正斜杆。
func PGlob(pattern string) ([]string, error) {
        matches, err := filepath.Glob(pattern)
        if err != nil {
                return nil, err
        }
        for i, m := range matches {
                matches[i] = toSlash(m)
        }
        return matches, nil
}

// PWalkDir 等价 filepath.WalkDir，回调收到的 path 归一化为正斜杆。
func PWalkDir(root string, fn func(path string, d os.DirEntry, err error) error) error {
        return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
                return fn(toSlash(path), d, err)
        })
}

// PWalk 等价 filepath.Walk，回调收到的 path 归一化为正斜杆。
func PWalk(root string, fn func(path string, info os.FileInfo, err error) error) error {
        return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
                return fn(toSlash(path), info, err)
        })
}
