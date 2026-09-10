package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// Hooks 桥接（对齐 DSH packages/hooks/hooks-claude-code + hooks-codex）。
//
// DSH 的 hooks 桥接允许运行已有的 Claude Code / Codex 钩子配置——
// 外部钩子脚本在工具执行前/后被调用，可 veto（阻止）或改写结果。
//
// DSC 的适配：HookBridge 加载外部钩子配置文件（JSON 格式），
// 在工具流水线 BeforeTool / AfterTool 阶段执行外部脚本。
// 配置格式对齐 Claude Code 的 hooks.json：
//   {
//     "before_tool": [{"matcher": "shell", "command": "/path/to/hook.sh"}],
//     "after_tool": [{"matcher": "*", "command": "/path/to/hook.sh"}]
//   }

// HookConfig 外部钩子配置。
type HookConfig struct {
	BeforeTool []HookEntry `json:"before_tool"`
	AfterTool  []HookEntry `json:"after_tool"`
}

// HookEntry 一个钩子条目。
type HookEntry struct {
	// Matcher 工具名匹配模式（"*" 匹配所有，支持前缀通配 "shell*"）。
	Matcher string `json:"matcher"`
	// Command 要执行的外部命令。
	Command string `json:"command"`
}

// HookBridge 外部钩子桥接器。
type HookBridge struct {
	mu     sync.Mutex
	config *HookConfig
}

// NewHookBridge 创建钩子桥接器。
func NewHookBridge() *HookBridge {
	return &HookBridge{}
}

// LoadConfig 从 JSON 文件加载钩子配置。
func (h *HookBridge) LoadConfig(path string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 无配置文件不报错
		}
		return fmt.Errorf("hook bridge: read config: %w", err)
	}

	var cfg HookConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("hook bridge: parse config: %w", err)
	}

	// 验证命令存在性（仅检查可执行文件是否存在，不执行）
	for _, entries := range [][]HookEntry{cfg.BeforeTool, cfg.AfterTool} {
		for _, e := range entries {
			if e.Command == "" {
				return fmt.Errorf("hook bridge: empty command in config")
			}
			cmdPath := e.Command
			if !filepath.IsAbs(cmdPath) {
				if _, err := exec.LookPath(cmdPath); err != nil {
					return fmt.Errorf("hook bridge: command not found: %s", cmdPath)
				}
			} else {
				if _, err := os.Stat(cmdPath); err != nil {
					return fmt.Errorf("hook bridge: command not found: %s", cmdPath)
				}
			}
		}
	}

	h.config = &cfg
	return nil
}

// RunBeforeTool 执行工具前钩子。
// 返回 (modifiedArgs, vetoError)：vetoError 非 nil 表示阻止执行。
func (h *HookBridge) RunBeforeTool(ctx context.Context, toolName, argsJSON string) (string, error) {
	h.mu.Lock()
	config := h.config
	h.mu.Unlock()

	if config == nil {
		return argsJSON, nil
	}

	for _, entry := range config.BeforeTool {
		if !matchHookTool(entry.Matcher, toolName) {
			continue
		}
		result, veto, err := h.execHook(ctx, entry.Command, toolName, argsJSON, "", "")
		if err != nil {
			// 钩子执行失败不阻止主流程（对齐 DSH 的 contain 语义）
			continue
		}
		if veto {
			return argsJSON, fmt.Errorf("hook %s vetoed tool %s: %s", entry.Command, toolName, result)
		}
		if result != "" {
			argsJSON = result // 钩子可改写参数
		}
	}
	return argsJSON, nil
}

// RunAfterTool 执行工具后钩子。
// 返回 (modifiedResult, modifiedError)。
func (h *HookBridge) RunAfterTool(ctx context.Context, toolName, argsJSON, result, toolErr string) (string, string) {
	h.mu.Lock()
	config := h.config
	h.mu.Unlock()

	if config == nil {
		return result, toolErr
	}

	for _, entry := range config.AfterTool {
		if !matchHookTool(entry.Matcher, toolName) {
			continue
		}
		modified, _, err := h.execHook(ctx, entry.Command, toolName, argsJSON, result, toolErr)
		if err != nil {
			continue
		}
		if modified != "" {
			result = modified
		}
	}
	return result, toolErr
}

// execHook 执行外部钩子命令。
// 输入经 stdin 传入 JSON（含 tool_name、arguments、result、error）。
// 返回 (stdout, veto, error)：veto=true 表示钩子要求阻止执行。
func (h *HookBridge) execHook(ctx context.Context, command, toolName, argsJSON, result, toolErr string) (string, bool, error) {
	input := map[string]any{
		"tool_name": toolName,
		"arguments": argsJSON,
		"result":    result,
		"error":     toolErr,
	}
	inputJSON, _ := json.Marshal(input)

	cmd := exec.CommandContext(ctx, command)
	cmd.Stdin = strings.NewReader(string(inputJSON))
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", false, fmt.Errorf("hook %s failed: %w (stderr: %s)", command, err, stderr.String())
	}

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		return "", false, nil
	}

	// 解析钩子输出：JSON 含 "veto": true 表示阻止
	var resp struct {
		Veto   bool   `json:"veto"`
		Output string `json:"output"`
	}
	if err := json.Unmarshal([]byte(output), &resp); err != nil {
		return output, false, nil // 非 JSON 输出直接当结果
	}
	return resp.Output, resp.Veto, nil
}

// matchHookTool 匹配工具名与钩子模式。
func matchHookTool(matcher, toolName string) bool {
	if matcher == "*" || matcher == "" {
		return true
	}
	if strings.HasSuffix(matcher, "*") {
		prefix := strings.TrimSuffix(matcher, "*")
		return strings.HasPrefix(toolName, prefix)
	}
	return matcher == toolName
}
