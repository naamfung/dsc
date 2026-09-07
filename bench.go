//go:build ignore

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func main() {
	// 1. 获取当前可执行文件所在目录（即与 dsc 同级的目录）
	exePath, err := os.Executable()
	if err != nil {
		panic("获取当前程序路径失败: " + err.Error())
	}
	workDir := filepath.Dir(exePath) // 工作目录 = 程序所在目录

	// 2. 构建 dsc 可执行文件路径（根据操作系统决定是否加 .exe）
	dscName := "dsc"
	if runtime.GOOS == "windows" {
		dscName = "dsc.exe"
	}
	dscPath := filepath.Join(workDir, dscName)

	// 3. 准备命令
	cmd := exec.Command(dscPath,
		"-input",
		"加载 tool-agentic-bench 插件，逐项完成所有 bench 测试并汇总报告",
	)

	// 4. 设置工作目录（等价于 cd 到 dsc 所在目录）
	cmd.Dir = workDir

	// 5. 标准输入为空字符串
	cmd.Stdin = strings.NewReader("")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// 6. 设置环境变量（DSC_WORKSPACE_ROOT 使用相对路径 ../tmp-test）
	//    因为 workDir 是 dsc-for-windows-amd64，而 tmp-test 在上一级 dist 目录下
	workspaceRoot := filepath.Join(workDir, "..", "tmp-test")
	// 转换为绝对路径（可选，但更可靠）
	absWorkspace, err := filepath.Abs(workspaceRoot)
	if err == nil {
		workspaceRoot = absWorkspace
	}

	// 继承当前进程环境，并添加所需变量
	env := os.Environ()
	env = append(env, "DSC_WORKSPACE_ROOT="+workspaceRoot)
	env = append(env, "DSC_APPROVAL=never")
	cmd.Env = env

	// 7. 执行
	if err := cmd.Run(); err != nil {
		panic(err)
	}
}