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
	// 1. 获取当前可执行文件所在目录（与 dsc 同级）
	exePath, err := os.Executable()
	if err != nil {
		panic("获取当前程序路径失败: " + err.Error())
	}
	workDir := filepath.Dir(exePath)

	// 2. 构建 dsc 可执行文件路径（根据操作系统决定是否加 .exe）
	dscName := "dsc"
	if runtime.GOOS == "windows" {
		dscName = "dsc.exe"
	}
	dscPath := filepath.Join(workDir, dscName)

	// 3. 计算工作空间根目录（上一级目录下的 tmp-test）
	workspaceRoot := filepath.Join(workDir, "..", "tmp-test")
	absWorkspace, err := filepath.Abs(workspaceRoot)
	if err == nil {
		workspaceRoot = absWorkspace
	}

	// 4. 自动创建 tmp-test 目录（如果不存在）
	if err := os.MkdirAll(workspaceRoot, 0755); err != nil {
		panic("创建 tmp-test 目录失败: " + err.Error())
	}

	// 5. 准备命令
	cmd := exec.Command(dscPath,
		"-input",
		"加载 tool-agentic-bench 插件，逐项完成所有 bench 测试并汇总报告",
	)

	// 6. 设置工作目录（等价于 cd 到 dsc 所在目录）
	cmd.Dir = workDir

	// 7. 标准输入为空字符串
	cmd.Stdin = strings.NewReader("")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// 8. 设置环境变量（DSC_WORKSPACE_ROOT 指向刚创建的目录）
	env := os.Environ()
	env = append(env, "DSC_WORKSPACE_ROOT="+workspaceRoot)
	env = append(env, "DSC_APPROVAL=never")
	cmd.Env = env

	// 9. 执行
	if err := cmd.Run(); err != nil {
		panic(err)
	}
}