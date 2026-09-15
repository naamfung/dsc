package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	dsc.SafeGoroutine(func() {
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
	})

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
	baseDescription := "Execute a shell command or script using mvdan/sh interpreter internally (POSIX shell standard). Each call without a reused session_id runs in a fresh shell: no state (cwd, variables, functions) persists between calls — pass `workdir` instead of using `cd`. A consistent session_id maintains state (cwd, environment variables) across calls."

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
                                "description": "Optional sandbox escalation: if a command was denied because this interpreter may write files under the current sandbox mode, retry this exact command once with a strictly wider mode (read-only → workspace-write/danger-full-access; workspace-write → danger-full-access) to request user approval for this one call. Omit for a normal call."
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

// workspaceRoot 返回统一工作区真实根（宿主注入的 DSC_WORKSPACE_ROOT），空串表示未注入。
func workspaceRoot() string {
	return os.Getenv("DSC_WORKSPACE_ROOT")
}

// mapWorkspacePath 把模型书写的路径映射到真实路径，覆盖三类别名（shell 是 mvdan
// POSIX 解释器，路径统一正斜杆，不涉及反斜杆；AST 层对字面量词统一调用）：
//
//  1. /workspace 虚拟根（所有平台）：模型常先 `cd /workspace` 探索，原生命令
//     不认虚拟根会报 no such file or directory；边界语义与 sandbox/str-replace-editor
//     一致——前綴後必須是分隔符或結尾，/workspacefoo 之类按独立路径处理。
//
//  2. Windows 裸 POSIX 绝对路径（/x、/ 等，仅 Windows）：Windows 上 "/" 并非
//     真实的文件系统根——Go 的 filepath.IsAbs("/x") 为 false，内建工具把它当
//     工作区相对路径（Join 后落在工作区内）；而 PATH 外部命令（MSYS find 等）
//     却把 "/" 当当前盘符根，同一写法两套语义。真实案例：模型 `find /` 遍历了
//     整个 D:\ 盘根（$RECYCLE.BIN、System Volume Information），既浪费上下文
//     又越出工作区沙箱。统一虚拟根语义：裸 / 前缀路径一律锚定工作区根，与内建
//     工具既有行为一致；要跨出工作区须显式用盘符路径（D:/...，见 pathAdvice）。
//     例外：/dev/null 保持原样——mvdan DefaultOpenHandler 在 Windows 上把它
//     特判重定向到 NUL 设备（2>/dev/null 等重定向依赖此行为）；// 开头的 UNC
//     路径亦不改写。Linux/macOS 不启用：POSIX 系统上 / 是真实根，内建工具本就
//     以真实根解析，改写反而破坏既有语义。
//
//  3. WSL 风格路径 /mnt/<drive>/... → <drive>:/...（仅 Windows）：
//     - Windows 上 DSC 的 shell 是 mvdan POSIX 解释器（非 WSL），无法访问真正的 /mnt/c/
//     挂载点；模型若以 WSL 路径习惯（/mnt/c/Users/...）调用，统一转换为 Windows 盘符路径
//     （C:/Users/...），与 pathAdvice 中对模型的指引保持一致。
//     - Linux/macOS 上 /mnt/c/... 是合法的 POSIX 路径（可能是真实挂载点，也可能是用户目录），
//     不得改写，否则会破坏可访问的真实路径。跨平台是 DSC 的根本约束，此处必须按 GOOS 分支。
func mapWorkspacePath(p string) string {
	// 1. WSL 路径映射：/mnt/c/... → C:/...，/mnt/d/... → D:/...（仅 Windows）
	if runtime.GOOS == "windows" && strings.HasPrefix(p, "/mnt/") {
		rest := p[len("/mnt/"):]
		if len(rest) >= 2 && rest[1] == '/' {
			drive := string(rest[0])
			if drive >= "a" && drive <= "z" || drive >= "A" && drive <= "Z" {
				return strings.ToUpper(drive) + ":/" + rest[2:]
			}
		}
		// /mnt/c (no trailing slash) → C:/
		if len(rest) == 1 {
			drive := rest
			if drive >= "a" && drive <= "z" || drive >= "A" && drive <= "Z" {
				return strings.ToUpper(drive) + ":/"
			}
		}
	}
	ws := workspaceRoot()
	if ws == "" {
		return p
	}
	root := strings.TrimRight(filepath.ToSlash(ws), "/")

	// 2. /workspace 虚拟根映射（所有平台生效；先于裸 / 判定，避免 Windows 上
	//    /workspace/x 被下面的裸 / 规则吃掉而错映射）
	const prefix = "/workspace"
	if strings.HasPrefix(p, prefix) {
		rest := p[len(prefix):]
		// 边界检查：前綴後必須是分隔符或結尾，否則 /workspacefoo 不當作 /workspace 別名
		if rest == "" || strings.HasPrefix(rest, "/") {
			sub := strings.TrimLeft(rest, "/")
			if sub == "" {
				return root
			}
			return root + "/" + sub
		}
	}

	// 3. Windows 裸 POSIX 绝对路径 → 工作区根（仅 Windows，虚拟根语义）
	if runtime.GOOS == "windows" {
		if p == "/" {
			return root
		}
		if strings.HasPrefix(p, "/") && !strings.HasPrefix(p, "//") && p != "/dev/null" {
			return root + p
		}
	}
	return p
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
		// 内建式常用 POSIX 工具（mkdir/ls/cat/touch/rm/cp/mv/grep/head/tail/wc）进程内
		// 实现，不依赖外部 PATH；未命中的命令仍回退默认 PATH 外部执行（见 internalcmds.go）。
		interp.ExecHandler(shellExecHandler),
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
	// 把 /workspace 虚拟根前缀映射为真实工作区根，使模型初期 `cd /workspace`、
	// `ls /workspace/x` 等探索不再报 no such file or directory（对齐 sandbox 语义）。
	mapWorkspacePaths(file)

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
	output := ensureUTF8(session.StdoutBuf.String() + session.StderrBuf.String())
	session.mu.Unlock()

	if idleCancelled {
		return output, exitCode, fmt.Errorf("command idle timeout (no output for > %s)", shellIdleInitial)
	}
	return output, exitCode, nil
}
