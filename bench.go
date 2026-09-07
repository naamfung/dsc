//go:build ignore

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"
)

// ensureAgenticBenchInConfig 确保 <workDir>/config/config.yaml 的 plugins 里注册了
// tool-agentic-bench。若不预置，模型只能在会话里靠 load_dsc_plugin 加载，而在
// DSC_APPROVAL=never（无人值守）下该操作会被审批拦截自动拒绝，bench 工具将永远
// 不可用。已存在同名条目时保持不变，避免覆盖既有插件配置。
func ensureAgenticBenchInConfig(configPath string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("读取配置失败: %w", err)
	}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("解析配置失败: %w", err)
	}
	doc := root.Content[0] // 根 mapping

	// 定位 plugins 序列
	var plugins *yaml.Node
	for i := 0; i+1 < len(doc.Content); i += 2 {
		if doc.Content[i].Value == "plugins" {
			plugins = doc.Content[i+1]
			break
		}
	}
	if plugins == nil {
		plugins = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		doc.Content = append(doc.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "plugins"},
			plugins,
		)
	}
	// 已注册则直接返回
	for _, it := range plugins.Content {
		for j := 0; j+1 < len(it.Content); j += 2 {
			if it.Content[j].Value == "name" && it.Content[j+1].Value == "tool-agentic-bench" {
				return nil
			}
		}
	}
	// 末尾追加 tool-agentic-bench 条目
	entry := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Tag: "!!str", Value: "name"},
		{Kind: yaml.ScalarNode, Tag: "!!str", Value: "tool-agentic-bench"},
		{Kind: yaml.ScalarNode, Tag: "!!str", Value: "type"},
		{Kind: yaml.ScalarNode, Tag: "!!str", Value: "tool"},
		{Kind: yaml.ScalarNode, Tag: "!!str", Value: "binary_path"},
		{Kind: yaml.ScalarNode, Tag: "!!str", Value: "./plugins/tool-agentic-bench/tool-agentic-bench.exe"},
		{Kind: yaml.ScalarNode, Tag: "!!str", Value: "enabled"},
		{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"},
	}}
	plugins.Content = append(plugins.Content, entry)

	out, err := yaml.Marshal(&root)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}
	return os.WriteFile(configPath, out, 0644)
}

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

	// 3. 把 tool-agentic-bench 预置进同级的 config/config.yaml，绕开运行时 load 被
	//    DSC_APPROVAL=never 拦截的问题（详见 ensureAgenticBenchInConfig）。
	configPath := filepath.Join(workDir, "config", "config.yaml")
	if err := ensureAgenticBenchInConfig(configPath); err != nil {
		fmt.Fprintf(os.Stderr, "预置 tool-agentic-bench 插件配置失败: %v\n", err)
		os.Exit(1)
	}

	// 4. 计算工作空间根目录（上一级目录下的 tmp-test）
	workspaceRoot := filepath.Join(workDir, "..", "tmp-test")
	absWorkspace, err := filepath.Abs(workspaceRoot)
	if err == nil {
		workspaceRoot = absWorkspace
	}

	// 5. 自动创建 tmp-test 目录（如果不存在）
	if err := os.MkdirAll(workspaceRoot, 0755); err != nil {
		panic("创建 tmp-test 目录失败: " + err.Error())
	}

	// 6. 准备命令
	cmd := exec.Command(dscPath,
		"-input",
		"加载 tool-agentic-bench 插件，逐项完成所有 bench 测试并汇总报告",
	)

	// 7. 设置工作目录（等价于 cd 到 dsc 所在目录）
	cmd.Dir = workDir

	// 8. 标准输入为空字符串
	cmd.Stdin = strings.NewReader("")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// 9. 设置环境变量（DSC_WORKSPACE_ROOT 指向刚创建的目录）
	env := os.Environ()
	env = append(env, "DSC_WORKSPACE_ROOT="+workspaceRoot)
	env = append(env, "DSC_APPROVAL=never")
	cmd.Env = env

	// 10. 执行
	if err := cmd.Run(); err != nil {
		panic(err)
	}
}
