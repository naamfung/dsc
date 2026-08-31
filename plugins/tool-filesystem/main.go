package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strconv"
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
	StdoutBuf *syncedBuilder
	StderrBuf *syncedBuilder
	mu        sync.Mutex
}

// syncedBuilder 是線程安全的輸出累加緩衝：Runner 向其寫入（interp 可能在後台
// 並發寫 stdout/stderr），idle 探測 goroutine 由 Len() 讀長度以偵測活躍，故須加鎖。
type syncedBuilder struct {
	mu sync.Mutex
	b  strings.Builder
}

func (s *syncedBuilder) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncedBuilder) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func (s *syncedBuilder) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Len()
}

func (s *syncedBuilder) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.b.Reset()
}

// SessionManager 管理所有持久的 shell 會話
type SessionManager struct {
	sessions map[string]*Session
	mu       sync.RWMutex
}

var globalSessionManager = &SessionManager{
	sessions: make(map[string]*Session),
}

// maxSessions 上限：防持久 shell 会话 map 无限增长。
const maxSessions = 64

// shell 前台命令超时采用「十分鐘起步、活躍續命」方式（對齊 rex shell）：
// 起步 10 分鐘預算，只要 stdout/stderr 持續有新輸出就不斷重新計時（續命），
// 只有「長時間完全冇輸出」先會超時。避免一刀切固定時長誤殺長耗時但仍在產出嘅編譯/測試。
var (
	// shellIdleInitial 超時起步預算（可 DSC_SHELL_TIMEOUT 覆盖，默认 10 分钟）。
	// 只要 stdout/stderr 持續有輸出就不斷重新計時；只有長時間完全無輸出先超時。
	shellIdleInitial = durEnv("DSC_SHELL_TIMEOUT", 10*time.Minute)
)

// errShellIdleTimeout 標記空闲超时（執行被 idle 管理 ctx 取消的 cause）。
var errShellIdleTimeout = errors.New("shell idle timeout")

// durEnv 读环境变量为时长；空/非法回退默认值。
func durEnv(key string, def time.Duration) time.Duration {
	if s := os.Getenv(key); s != "" {
		if d, err := time.ParseDuration(s); err == nil && d > 0 {
			return d
		}
	}
	return def
}

// runWithIdleTimeout 以「十分鐘起步、活躍續命」方式執行 run（對齊 rex shell）：
// 啟動 shellIdleInitial 預算，只要 stdout/stderr 有新增輸出就重新計時（延長），
// 只有持續 shellIdleInitial 完全無任何新輸出才取消執行並返回 errShellIdleTimeout。
// 緩衝以 syncedBuilder 提供線程安全的 Len()，故輪詢讀長度與 Runner 寫入不衝突。
func runWithIdleTimeout(ctx context.Context, session *Session, run func(context.Context) error) error {
	runCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	const pollInterval = 500 * time.Millisecond
	stop := make(chan struct{})
	go func() {
		tick := time.NewTicker(pollInterval)
		defer tick.Stop()
		lastLen := 0
		lastActive := time.Now()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				cur := session.StdoutBuf.Len() + session.StderrBuf.Len()
				now := time.Now()
				if cur != lastLen {
					lastLen = cur
					lastActive = now
				} else if now.Sub(lastActive) > shellIdleInitial {
					cancel(errShellIdleTimeout)
					return
				}
			}
		}
	}()

	err := run(runCtx)
	close(stop)
	if context.Cause(runCtx) == errShellIdleTimeout {
		return errShellIdleTimeout
	}
	return err
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
		output, exitCode, err := execSessionCommand(ctx, session, params.Command)
		if err != nil {
			return "", fmt.Errorf("failed to execute command in session: %w", err)
		}

		return formatShellResult(output, exitCode), nil
	}

	sdk := dsc.New(dsc.Config{Name: "filesystem", Version: "1.0.0", Type: dsc.TypeTool})
	sdk.Tool(dsc.Tool{Name: "shell", Description: description, Schema: schema, Handler: handler, ViewFn: shellView})
	sdk.Serve()
}

// exitCodeMarkRe 匹配结果里追加的退出码标记（[exit_code: N]，见 formatShellResult）。
var exitCodeMarkRe = regexp.MustCompile(`\[exit_code\s*:\s*(-?\d+)\]`)

// shellView 为 shell 工具声明结构化视图：标题 Shell + 退出码徽标（0 绿 / 非 0 红）
// + 命令输出正文（去掉追加的 [exit_code: N] 标记，避免与徽标重复）。
func shellView(_ context.Context, _ json.RawMessage, result string) (json.RawMessage, error) {
	body := strings.TrimSpace(result)
	exitCode := int64(0)
	if m := exitCodeMarkRe.FindStringSubmatch(body); len(m) == 2 {
		if n, err := strconv.ParseInt(m[1], 10, 32); err == nil {
			exitCode = n
		}
		body = exitCodeMarkRe.ReplaceAllString(body, "")
	}
	body = strings.TrimSpace(body)
	if body == "" {
		body = "(no output)"
	}
	tone := "green"
	if exitCode != 0 {
		tone = "red"
	}
	return dsc.PlainView("Shell", &dsc.ViewBadge{Text: fmt.Sprintf("exit %d", exitCode), Tone: tone}, body), nil
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

	stdoutBuf := &syncedBuilder{}
	stderrBuf := &syncedBuilder{}

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
	// 达上限时任删一个会话，避免 map 无限增长。
	if len(globalSessionManager.sessions) >= maxSessions {
		for k := range globalSessionManager.sessions {
			delete(globalSessionManager.sessions, k)
			break
		}
	}
	globalSessionManager.sessions[sessionID] = session
	globalSessionManager.mu.Unlock()

	return session, nil
}

// execSessionCommand 在 session 中執行命令，返回輸出和退出碼。
// ctx 用于传播调用方取消；並用「十分鐘起步、活躍續命」idle 超時防止命令掛死
// （對齊 rex shell）：只要 stdout/stderr 持續有輸出就續命，只有長時間
// 完全無輸出先超時，避免誤殺仍然活躍嘅長編譯/測試。
func execSessionCommand(ctx context.Context, session *Session, command string) (string, int32, error) {
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

	// 活躍續命超時：持續有輸出就續命，只有長時間完全無輸出先超時（對齊 rex shell）。
	err = runWithIdleTimeout(ctx, session, func(runCtx context.Context) error {
		return session.Runner.Run(runCtx, file)
	})
	idleCancelled := errors.Is(err, errShellIdleTimeout)

	exitCode := int32(0)
	if err != nil && !idleCancelled {
		if exitErr, ok := err.(interp.ExitStatus); ok {
			exitCode = int32(exitErr)
		} else {
			exitCode = 1
		}
	}

	session.mu.Lock()
	output := session.StdoutBuf.String() + session.StderrBuf.String()
	session.mu.Unlock()

	if idleCancelled {
		return output, exitCode, fmt.Errorf("command idle timeout (no output for > %s)", shellIdleInitial)
	}
	return output, exitCode, nil
}
