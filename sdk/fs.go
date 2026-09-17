package dsc

import (
	"fmt"
	"os"
	"path/filepath"

	"dsc/core"
)

// 统一文件 IO 助手：所有插件读写文件统一经此层，获得一致的错误包装
// （"read <path>: <err>" 风格）与防御性 recover（panic 转错误返回，绝不向
// 插件进程外传播导致 LLM 连接中断）。路径规范化（AbsPath）与工作空间根
// （WorkspaceRoot）亦集中于此，消除各插件重复实现。
//
// 错误呈现对齐内部 POSIX shell（tool-filesystem 的 mvdan/sh）约定：错误信息
// 里的路径一律正斜杆（filepath.ToSlash）——Windows 上 os.* 错误内嵌反斜杆
// 路径，直接回显给模型会与 shell 工具的 POSIX 风格不一致、造成混乱。底层
// 错误仍经 Unwrap 保留，插件可照常 errors.Is/As。
//
// 注意：本层不做 workspace 越界检查——沙箱策略由宿主工具流水线 pre-execute
// 瀑布统一判定，插件自行判定反而会在 full-access 模式下误拒 workspace 外路径。

// posixPathErr 包装底层文件错误：Error() 时把路径统一为正斜杆（与内部 POSIX
// shell 风格一致），Unwrap() 保留底层错误供 errors.Is/As 使用。
type posixPathErr struct {
	op   string // 操作名（read / write / mkdir / resolve path）
	path string
	err  error
}

func (e *posixPathErr) Error() string {
	return fmt.Sprintf("%s %s: %s", e.op, filepath.ToSlash(e.path), filepath.ToSlash(e.err.Error()))
}

func (e *posixPathErr) Unwrap() error { return e.err }

// WorkspaceRoot 返回统一工作空间根目录。单一源头在 core（宿主进程按 config
// workspace_root 解析，工具插件等子进程经包 init 读宿主注入的 DSC_WORKSPACE_ROOT，
// 无注入回退 cwd），SDK 层转发供第三方插件使用；所有插件默认输出路径推断统一用
// 此函数。
func WorkspaceRoot() string {
	return core.WorkspaceRoot
}

// MapWorkspacePath 把模型书写的路径按「虚拟根 = 工作空间根」契约映射为真实路径
// （WSL 盘符映射、/workspace 前缀、Windows 裸 / 锚定根；规则详见 core.MapWorkspacePath）。
// 源头实现在 core（宿主本身亦有虚拟根诉求），SDK 导入 core 二次封装，第三方插件
// 无须直接依赖 core 即可在 SDK 层获得工作空间相关方法的完整支持，各插件不再
// 各自实现归并转换。
func MapWorkspacePath(p string) string {
	return core.MapWorkspacePath(p)
}

// ResolveWorkspacePath 把模型书写的路径映射后解析为真实绝对路径：相对路径一律
// 锚定工作空间根（非插件进程 cwd——插件 cwd 是 ExecDir，锚 cwd 会把工作空间
// 相对路径落到安装目录）。错误统一包装为 "resolve path <p>: <err>"（路径正斜杆
// 呈现，见 posixPathErr），底层错误保留供 errors.Is/As。
func ResolveWorkspacePath(p string) (string, error) {
	resolved, err := core.ResolveWorkspacePath(p)
	if err != nil {
		return "", &posixPathErr{op: "resolve path", path: p, err: err}
	}
	return resolved, nil
}

// AbsPath 规范化路径为绝对路径（filepath.Abs）。错误统一包装为
// "resolve path <p>: <err>"（路径正斜杆呈现，见 posixPathErr）。
func AbsPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", &posixPathErr{op: "resolve path", path: path, err: err}
	}
	return abs, nil
}

// ReadFile 读取文件内容。错误统一包装为 "read <path>: <err>"（路径正斜杆呈现，
// 与内部 POSIX shell 风格一致）；panic 转为错误返回（防御性 recover），绝不
// crash 插件进程。
func ReadFile(path string) (data []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			data, err = nil, fmt.Errorf("dsc.ReadFile(%s) panicked: %v", filepath.ToSlash(path), r)
		}
	}()
	data, err = os.ReadFile(path)
	if err != nil {
		return nil, &posixPathErr{op: "read", path: path, err: err}
	}
	return data, nil
}

// WriteFile 写入文件（默认权限 0644，同 os.WriteFile）。错误统一包装为
// "write <path>: <err>"（路径正斜杆呈现）；panic 转为错误返回。不自动创建父
// 目录（与 os.WriteFile 语义一致，由插件自行 MkdirAll）。
func WriteFile(path string, data []byte) error {
	return writeFile(path, data, 0o644)
}

// WriteFilePerm 以显式权限写入文件；错误包装与防御性 recover 同 WriteFile。
func WriteFilePerm(path string, data []byte, perm os.FileMode) error {
	return writeFile(path, data, perm)
}

func writeFile(path string, data []byte, perm os.FileMode) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("dsc.WriteFile(%s) panicked: %v", filepath.ToSlash(path), r)
		}
	}()
	if err := os.WriteFile(path, data, perm); err != nil {
		return &posixPathErr{op: "write", path: path, err: err}
	}
	return nil
}

// MkdirAll 创建目录（含父目录，默认权限 0755）。错误统一包装为
// "mkdir <path>: <err>"（路径正斜杆呈现）；panic 转为错误返回。
func MkdirAll(path string) error {
	return mkdirAll(path, 0o755)
}

// MkdirAllPerm 以显式权限创建目录；错误包装与防御性 recover 同 MkdirAll。
func MkdirAllPerm(path string, perm os.FileMode) error {
	return mkdirAll(path, perm)
}

func mkdirAll(path string, perm os.FileMode) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("dsc.MkdirAll(%s) panicked: %v", filepath.ToSlash(path), r)
		}
	}()
	if err := os.MkdirAll(path, perm); err != nil {
		return &posixPathErr{op: "mkdir", path: path, err: err}
	}
	return nil
}
