package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"dsc-sdk"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

// Session 表示一個持久的 shell 會話
type Session struct {
	SessionID string
	Cwd       string
	Runner    *interp.Runner
	StdoutBuf *strings.Builder
	StderrBuf *strings.Builder
	mu        sync.Mutex
}

// SessionManager 管理所有持久的 shell 會話
type SessionManager struct {
	sessions map[string]*Session
	mu       sync.RWMutex
}

var globalSessionManager = &SessionManager{
	sessions: make(map[string]*Session),
}

// main 以公共 SDK（dsc-sdk）声明式启动：SDK 自动提供 ToolService /
// PluginMetadata / PluginHookService 与 go-core 组装（重写自旧的
// ToolServiceServer/MetadataServer/ToolMetadataGRPCPlugin 样板）。
func main() {
	// 定義 shell 工具描述
	baseDescription := "Execute a shell command or script using mvdan/sh interpreter internally (POSIX shell standard). Supports persistent sessions via session_id."

	var pathAdvice string
	var extraAdvice string

	if runtime.GOOS == "windows" {
		pathAdvice = "CRITICAL: In Windows environments, terminal path styles vary (e.g., Git Bash uses '/mnt/d/...', while this terminal supports 'D:/...'). You MUST first run the 'pwd' command to obtain the current directory path format before performing any path operations. Always wrap paths in quotes to ensure safe usage."
		extraAdvice = "Note: This interpreter does not support PowerShell (PWSH), CMD, or other Windows-specific shell command interpreters. Please use standard Unix/Linux POSIX shell commands only (e.g., ls, find, cd, grep)."
	} else {
		pathAdvice = "When working with paths, it is mandatory to first use the 'pwd' command to get the current directory path format, and always wrap paths in quotes to ensure safe usage."
	}

	description := baseDescription
	if pathAdvice != "" {
		description += " " + pathAdvice
	}
	if extraAdvice != "" {
		description += " " + extraAdvice
	}

	// 定義 shell 工具
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"command": {
				"type": "string",
				"description": "The shell command or script to execute"
			},
			"cwd": {
				"type": "string",
				"description": "Working directory for the command (optional)"
			},
			"session_id": {
				"type": "string",
				"description": "Persistent session ID to maintain state (cwd, environment variables). If not provided or 'new', a new session is created."
			}
		},
		"required": ["command"]
	}`)
	handler := func(ctx context.Context, args json.RawMessage) (string, error) {
		var params struct {
			Command   string `json:"command"`
			Cwd       string `json:"cwd"`
			SessionID string `json:"session_id"`
		}
		if err := json.Unmarshal(args, &params); err != nil {
			return "", fmt.Errorf("invalid arguments: %w", err)
		}
		if strings.TrimSpace(params.Command) == "" {
			return "", fmt.Errorf("command is required")
		}

		// 處理 session
		sessionID := params.SessionID
		if sessionID == "" || sessionID == "new" {
			// 創建新 session
			sessionID = fmt.Sprintf("session-%d", time.Now().UnixNano())
		}

		// 獲取或創建 session
		session, err := getOrCreateSession(sessionID, params.Cwd)
		if err != nil {
			return "", fmt.Errorf("failed to create or get session: %w", err)
		}

		// 執行命令到 session
		output, exitCode, err := execSessionCommand(session, params.Command)
		if err != nil {
			return "", fmt.Errorf("failed to execute command in session: %w", err)
		}

		return formatShellResult(output, exitCode), nil
	}

	sdk := dsc.New(dsc.Config{Name: "filesystem", Version: "1.0.0", Type: dsc.TypeTool})
	sdk.Tool(dsc.Tool{Name: "shell", Description: description, Schema: schema, Handler: handler})
	sdk.Serve()
}

// formatShellResult 根據命令輸出與退出碼組裝最終返回文本：
// - 非 0 退出碼：無論是否有輸出，都追加 [exit_code: N]。
// - 0 退出碼且有輸出：直接返回輸出，不附加 [exit_code: 0]。
// - 0 退出碼且無輸出：返回 [exit_code: 0]。
func formatShellResult(output string, exitCode int32) string {
	if exitCode != 0 {
		return output + fmt.Sprintf("\n[exit_code: %d]\n", exitCode)
	}
	if strings.TrimSpace(output) == "" {
		return "\n[exit_code: 0]\n"
	}
	return output
}

// getOrCreateSession 獲取或創建 session
func getOrCreateSession(sessionID, cwd string) (*Session, error) {
	globalSessionManager.mu.RLock()
	session, exists := globalSessionManager.sessions[sessionID]
	globalSessionManager.mu.RUnlock()

	if exists {
		return session, nil
	}

	// 創建新 session runner
	initialEnv := expand.ListEnviron(os.Environ()...)
	if cwd == "" {
		// 未显式指定 cwd 时，默认以統一工作空間根（DSC_WORKSPACE_ROOT，即启动 dsc 的
		// 目录 cwd）为 shell 工作目录，而非插件进程自身的可执行目录。这样模型执行
		// pwd 得到的是用户的启动目录（workspace 根），而非程序安装目录（execDir）。
		if ws := os.Getenv("DSC_WORKSPACE_ROOT"); ws != "" {
			cwd = ws
		} else {
			var err error
			cwd, err = os.Getwd()
			if err != nil {
				cwd = os.TempDir()
			}
		}
	}

	stdoutBuf := &strings.Builder{}
	stderrBuf := &strings.Builder{}

	runnerOpts := []interp.RunnerOption{
		interp.Env(initialEnv),
		interp.Dir(cwd),
		interp.StdIO(nil, stdoutBuf, stderrBuf),
	}

	runner, err := interp.New(runnerOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create shell runner: %w", err)
	}

	session = &Session{
		SessionID: sessionID,
		Cwd:       cwd,
		Runner:    runner,
		StdoutBuf: stdoutBuf,
		StderrBuf: stderrBuf,
	}

	globalSessionManager.mu.Lock()
	globalSessionManager.sessions[sessionID] = session
	globalSessionManager.mu.Unlock()

	return session, nil
}

// execSessionCommand 在 session 中執行命令，返回輸出和退出碼
func execSessionCommand(session *Session, command string) (string, int32, error) {
	session.mu.Lock()
	session.StdoutBuf.Reset()
	session.StderrBuf.Reset()
	session.mu.Unlock()

	// 解析命令為 shell 語法樹
	parser := syntax.NewParser()
	file, err := parser.Parse(strings.NewReader(command+"\n"), "")
	if err != nil {
		// 如果解析失敗，嘗試作為單個命令執行
		command = "echo 'Syntax error in command: " + strings.ReplaceAll(err.Error(), "'", "''") + "'"
		file, err = parser.Parse(strings.NewReader(command+"\n"), "")
		if err != nil {
			return "", 0, fmt.Errorf("failed to parse command: %w", err)
		}
	}

	// 執行命令
	ctx := context.Background()
	err = session.Runner.Run(ctx, file)
	exitCode := int32(0)
	if err != nil {
		if exitErr, ok := err.(interp.ExitStatus); ok {
			exitCode = int32(exitErr)
		} else {
			exitCode = 1
		}
	}

	session.mu.Lock()
	output := session.StdoutBuf.String() + session.StderrBuf.String()
	session.mu.Unlock()

	return output, exitCode, nil
}
