package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"dsc-sdk"
	"dsc/core"
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
	// stdoutSignal / stderrSignal 活动信号 writer：包在对应缓冲外（interp 经
	// StdIO 写入它们），每次输出写入即向活跃续命执行域上报活动。随 session
	// 持久存在，回调在每次命令执行时换绑（见 execSessionCommand）。
	stdoutSignal *activityWriter
	stderrSignal *activityWriter
	mu           sync.Mutex
}

// syncedBuilder 是線程安全的輸出累加緩衝：Runner 經 activityWriter 向其寫入
// （interp 可能在後台並發寫 stdout/stderr），讀寫並發安全。
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

// activityWriter 活动信号 writer：包装会话输出缓冲，每次写入（输出到达）即向
// 当前调用的活跃续命执行域上报活动（core.TouchActivity）。超时预算与看门狗由
// timeout-policy 插件经宿主裁决安装（tool/execute 槽），本工具只供给活动信号——
// 输出到达即活动，不感知预算与策略。回调随每次命令执行换绑；未换绑
// （无进行中的命令）时 no-op。
type activityWriter struct {
	mu    sync.Mutex
	w     io.Writer
	touch func()
}

func (a *activityWriter) Write(p []byte) (int, error) {
	a.mu.Lock()
	f, w := a.touch, a.w
	a.mu.Unlock()
	if f != nil {
		f()
	}
	return w.Write(p)
}

// bind 把活动回调换绑到本次调用的 ctx；run 结束后以 bind(nil) 解绑。
func (a *activityWriter) bind(ctx context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if ctx == nil {
		a.touch = nil
		return
	}
	a.touch = func() { core.TouchActivity(ctx) }
}

// main 以公共 SDK（dsc-sdk）声明式启动：SDK 自动提供 ToolService /
// PluginMetadata / PluginHookService 与 go-core 组装（重写自旧的
// ToolServiceServer/MetadataServer/ToolMetadataGRPCPlugin 样板）。
func main() {
	// 定義 shell 工具描述
	baseDescription := "Execute a shell command or script using mvdan/sh interpreter internally (POSIX shell standard). Each call without a reused session_id runs in a fresh shell: no state (cwd, variables, functions) persists between calls — pass `workdir` instead of using `cd`. A consistent session_id maintains state (cwd, environment variables) across calls."

	var pathAdvice string
	var extraAdvice string

	if runtime.GOOS == "windows" {
		pathAdvice = "CRITICAL: In Windows environments, terminal path styles vary (e.g., Git Bash uses '/mnt/d/...', while this terminal supports 'D:/...'). A bare POSIX path like '/docs' resolves to the current drive root (e.g. 'D:/docs'), matching Linux real-root semantics — the session workspace is reached via the '/workspace' prefix or the real path shown by 'pwd'. You MUST first run the 'pwd' command to obtain the current directory path format before performing any path operations. Always wrap paths in quotes to ensure safe usage."
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
                                "description": "The shell command or script to execute."
                        },
                        "description": {
                                "type": "string",
                                "description": "Clear, concise description of what this command does in active voice, 5-10 words (shown in the UI). Examples: \"ls\" → \"List files in current directory\"; \"git status\" → \"Show working tree status\"."
                        },
                        "workdir": {
                                "type": "string",
                                "description": "Working directory for this command. Defaults to the session workspace; a relative path is resolved against it."
                        },
                        "session_id": {
                                "type": "string",
                                "description": "Persistent session ID to maintain state (cwd, environment variables). If not provided or 'new', a new session is created."
                        },
                        "sandbox_permissions": {
                                "type": "string",
                                "enum": ["workspace-write", "danger-full-access"],
                                "description": "Optional sandbox escalation, ONLY for retrying a call that was denied by the sandbox. Do NOT set it on normal or read-only calls: requesting a mode that is not strictly wider than the current one is ignored and the call simply runs under the current mode. To genuinely widen (read-only → workspace-write/danger-full-access; workspace-write → danger-full-access), retry the exact command once with this field plus 'justification' to request one-call user approval."
                        },
                        "justification": {
                                "type": "string",
                                "description": "Required together with 'sandbox_permissions': a one-sentence reason shown to the user in the approval prompt."
                        },
                        "run_in_background": {
                                "type": "boolean",
                                "description": "Run the command in the background and return a job id immediately. Track output with job_output, stop with job_kill. No timeout applies. Omit for foreground execution (default)."
                        }
                },
                "required": ["command", "description"]
        }`)
	handler := func(ctx context.Context, args json.RawMessage) (string, error) {
		var params struct {
			Command   string `json:"command"`
			WorkDir   string `json:"workdir"`
			Cwd       string `json:"cwd"` // 向后兼容旧参数名
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
		// workdir 优先于 cwd（对齐 DSH 参数名）；cwd 保留向后兼容
		workdir := params.WorkDir
		if workdir == "" {
			workdir = params.Cwd
		}
		session, err := getOrCreateSession(sessionID, workdir)
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

	sdk := dsc.New(dsc.Config{
		Name:    "filesystem",
		Version: "1.0.0",
		Type:    dsc.TypeTool,
		Provides: map[string]string{
			// 提供 "filesystem" 能力：含 shell 工具，可执行命令与读写文件
			"filesystem": "true",
		},
	})
	sdk.Tool(dsc.Tool{Name: "shell", Description: description, Schema: schema, Handler: handler, ViewFn: shellView})
	sdk.Serve()
}

// exitCodeMarkRe 匹配结果里追加的退出码标记（[exit_code: N]，见 formatShellResult）。
var exitCodeMarkRe = regexp.MustCompile(`\[exit_code\s*:\s*(-?\d+)\]`)

// mapWorkspacePath 把模型书写的路径按「虚拟根 = 工作区根」契约映射为真实路径。
// 实现已上收至 core.MapWorkspacePath（宿主与各插件进程同源，SDK 层有同名二次
// 封装供第三方插件，各插件不再各自实现归并转换）；此处仅保留 shell AST 字面量
// 重写（mapWorkspacePaths）的接入点。规则与例外详见 core/workspace.go：
// /workspace 前缀（全平台，前缀后必须是分隔符或结尾）、裸 POSIX 绝对路径保持
// 原样（Windows 上经绝对化解析为当前盘根，与 Linux 真实根一致）、
// /dev/null 与 // UNC 例外、WSL 风格 /mnt/<drive>/ 映射（仅 Windows）。
func mapWorkspacePath(p string) string {
	return core.MapWorkspacePath(p)
}

// mapWorkspacePaths 遍历 shell AST，把纯字面量词中的 /workspace 虚拟根前缀重写为真实路径。
// 只改写词首的路径形状词（裸词 / 单双引号内无变量展开），变量展开/命令替换等复杂词不改，
// 避免误伤。cd /workspace、ls -la /workspace/x、cat "/workspace/a b" 等均被覆盖。
func mapWorkspacePaths(node syntax.Node) {
	syntax.Walk(node, func(n syntax.Node) bool {
		switch v := n.(type) {
		case *syntax.Lit:
			v.Value = mapWorkspacePath(v.Value)
		case *syntax.SglQuoted:
			v.Value = mapWorkspacePath(v.Value)
		case *syntax.DblQuoted:
			for _, part := range v.Parts {
				if lit, ok := part.(*syntax.Lit); ok {
					lit.Value = mapWorkspacePath(lit.Value)
				}
			}
		}
		return true
	})
}

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
		if ws := dsc.WorkspaceRoot(); ws != "" {
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
	// 活动信号 writer 包在缓冲外：输出写入即向活跃续命执行域上报活动
	//（core.TouchActivity）；执行域由 timeout-policy 插件经宿主裁决安装。
	stdoutSignal := &activityWriter{w: stdoutBuf}
	stderrSignal := &activityWriter{w: stderrBuf}

	runnerOpts := []interp.RunnerOption{
		interp.Env(initialEnv),
		interp.Dir(cwd),
		interp.StdIO(nil, stdoutSignal, stderrSignal),
		// 内建式常用 POSIX 工具（mkdir/ls/cat/touch/rm/cp/mv/grep/head/tail/wc）进程内
		// 实现，不依赖外部 PATH；未命中的命令仍回退默认 PATH 外部执行（见 internalcmds.go）。
		interp.ExecHandler(shellExecHandler),
	}

	runner, err := interp.New(runnerOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create shell runner: %w", err)
	}

	session = &Session{
		SessionID:    sessionID,
		Cwd:          cwd,
		Runner:       runner,
		StdoutBuf:    stdoutBuf,
		StderrBuf:    stderrBuf,
		stdoutSignal: stdoutSignal,
		stderrSignal: stderrSignal,
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
// ctx 用于传播调用方取消与活跃续命执行域（若有）：命令輸出每次寫入都經
// activityWriter 上報活動（core.TouchActivity），看門狗與預算由 timeout-policy
// 插件經宿主裁決安裝——持續無輸出達預算才取消，避免誤殺仍在產出的長編譯/測試。
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
	// 把 /workspace 虚拟根前缀映射为真实工作区根，使模型初期 `cd /workspace`、
	// `ls /workspace/x` 等探索不再报 no such file or directory（对齐 sandbox 语义）。
	mapWorkspacePaths(file)

	// 活动信号换绑到本次调用 ctx：输出到达 → core.TouchActivity（活跃续命）。
	// 看门狗与预算由 timeout-policy 插件经宿主裁决安装（tool/execute 槽），
	// 本工具只供给活动信号；ctx 未携带执行域时 TouchActivity 为 no-op。
	session.stdoutSignal.bind(ctx)
	session.stderrSignal.bind(ctx)
	defer func() {
		session.stdoutSignal.bind(nil)
		session.stderrSignal.bind(nil)
	}()

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
	output := ensureUTF8(session.StdoutBuf.String() + session.StderrBuf.String())
	session.mu.Unlock()

	// 活跃续命看门狗触发：上抛取消错误，由工具流水线桥替换为裁决文案
	//（timeout-policy 的 message）。其余路径保持既有语义：解释器错误计入
	// 退出码、结果照常返回（非零退出不是 Go 错误），用户中断不在此上抛。
	if err != nil && core.IdleDeadlineExceeded(ctx) {
		return output, exitCode, err
	}
	return output, exitCode, nil
}
