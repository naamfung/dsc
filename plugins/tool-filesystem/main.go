package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"dsc/plugin"
	"dsc/proto"
	"dsc/proto/metadata"
	goplugin "github.com/hashicorp/go-plugin"
)

// FileTool 文件工具實現
type FileTool struct {
	name        string
	description string
	schema      json.RawMessage
	handler     func(ctx context.Context, args json.RawMessage) (string, error)
}

func (f *FileTool) Name() string {
	return f.name
}

func (f *FileTool) Description() string {
	return f.description
}

func (f *FileTool) ParametersSchema() json.RawMessage {
	return f.schema
}

func (f *FileTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	return f.handler(ctx, args)
}

// Session 表示一個持久的 shell 會話
type Session struct {
	SessionID string
	ShellType string
	Cwd       string
	Cmd       *exec.Cmd
	Stdin     io.WriteCloser
	Stdout    io.ReadCloser
	Stderr    io.ReadCloser
	OutputBuf strings.Builder
	mu        sync.Mutex
	done      chan struct{}
	consumed  int // 已被 execSessionCommand 讀取過的 OutputBuf 長度
}

// SessionManager 管理所有持久的 shell 會話
type SessionManager struct {
	sessions map[string]*Session
	mu       sync.RWMutex
}

var globalSessionManager = &SessionManager{
	sessions: make(map[string]*Session),
}

// ToolServiceServer 工具服務服務端實現
type ToolServiceServer struct {
	proto.UnimplementedToolServiceServer
	tools []*FileTool
}

func (s *ToolServiceServer) ExecuteTool(ctx context.Context, req *proto.ExecuteToolRequest) (*proto.ExecuteToolResponse, error) {
	for _, t := range s.tools {
		if t.Name() == req.ToolName {
			res, err := t.Execute(ctx, json.RawMessage(req.ArgumentsJson))
			if err != nil {
				return &proto.ExecuteToolResponse{Error: err.Error()}, nil
			}
			return &proto.ExecuteToolResponse{Content: res}, nil
		}
	}
	return &proto.ExecuteToolResponse{Error: "tool not found"}, nil
}

func (s *ToolServiceServer) ListTools(ctx context.Context, req *proto.ListToolsRequest) (*proto.ListToolsResponse, error) {
	var tools []*proto.Tool
	for _, t := range s.tools {
		tools = append(tools, &proto.Tool{
			Name:           t.Name(),
			Description:    t.Description(),
			ParametersJson: string(t.ParametersSchema()),
		})
	}
	return &proto.ListToolsResponse{Tools: tools}, nil
}

// MetadataServer 元數據服務服務端實現
type MetadataServer struct {
	metadata.UnimplementedPluginMetadataServer
}

func (m *MetadataServer) GetInfo(ctx context.Context, _ *metadata.Empty) (*metadata.PluginInfo, error) {
	return &metadata.PluginInfo{
		Type:       "tool",
		Name:       "filesystem",
		Version:    "1.0.0",
		ApiVersion: "1.0",
	}, nil
}

func main() {
	// 定義 shell 工具
	shellTool := &FileTool{
		name:        "shell",
		description: "Execute a shell command or script using an available shell (bash, zsh, ksh, sh, fish, dash, tcsh, csh; on Windows falls back to PowerShell/CMD). Supports persistent sessions via session_id.",
		schema: json.RawMessage(`{
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
				"shell": {
					"type": "string",
					"description": "Specific shell to use: bash, zsh, ksh, sh, fish, dash, tcsh, csh, pwsh, powershell, cmd. Defaults to the first detected shell."
				},
				"session_id": {
					"type": "string",
					"description": "Persistent session ID to maintain state (cwd, environment variables). If not provided or 'new', a new session is created."
				}
			},
			"required": ["command"]
		}`),
		handler: func(ctx context.Context, args json.RawMessage) (string, error) {
			var params struct {
				Command   string `json:"command"`
				Cwd       string `json:"cwd"`
				Shell     string `json:"shell"`
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
			session, err := getOrCreateSession(sessionID, params.Shell, params.Cwd)
			if err != nil {
				return "", fmt.Errorf("failed to create or get session: %w", err)
			}

			// 執行命令到 session
			output, exitCode, err := execSessionCommand(session, params.Command)
			if err != nil {
				return "", fmt.Errorf("failed to execute command in session: %w", err)
			}

			result := output
			if exitCode != 0 {
				result += fmt.Sprintf("\n[exit_code: %d]\n", exitCode)
			} else {
				result += "\n[exit_code: 0]\n"
			}
			return result, nil
		},
	}

	// 創建工具服務服務端
	toolServer := &ToolServiceServer{
		tools: []*FileTool{shellTool},
	}

	// 創建元數據服務服務端
	metadataServer := &MetadataServer{}

	// 啟動插件服務
	goplugin.Serve(&goplugin.ServeConfig{
		HandshakeConfig: plugin.Handshake,
		Plugins: map[string]goplugin.Plugin{
			"tool": &ToolMetadataGRPCPlugin{
				ToolImpl:   toolServer,
				MetadataImpl: metadataServer,
			},
		},
		GRPCServer: goplugin.DefaultGRPCServer,
	})
}

// getOrCreateSession 獲取或創建 session
func getOrCreateSession(sessionID, shellType, cwd string) (*Session, error) {
	globalSessionManager.mu.RLock()
	session, exists := globalSessionManager.sessions[sessionID]
	globalSessionManager.mu.RUnlock()

	if exists {
		return session, nil
	}

	// 創建新 session
	sh, err := resolveShell(detectAvailableShells(), shellType)
	if err != nil {
		return nil, err
	}

	// 啟動 shell 進程
	cmd, stdin, stdout, stderr, err := startInteractiveShellProcess(sh.Name, cwd)
	if err != nil {
		return nil, err
	}

	session = &Session{
		SessionID: sessionID,
		ShellType: sh.Name,
		Cwd:       cwd,
		Cmd:       cmd,
		Stdin:     stdin,
		Stdout:    stdout,
		Stderr:    stderr,
		done:      make(chan struct{}),
	}

	globalSessionManager.mu.Lock()
	globalSessionManager.sessions[sessionID] = session
	globalSessionManager.mu.Unlock()

	// 啟動讀取輸出的 goroutine
	go readSessionOutput(session)

	return session, nil
}

// execSessionCommand 在 session 中執行命令，返回「本次调用新增」的输出（不含历史输出）
func execSessionCommand(session *Session, command string) (string, int32, error) {
	cmdWithNewline := command + "\n"
	_, err := session.Stdin.Write([]byte(cmdWithNewline))
	if err != nil {
		return "", 0, fmt.Errorf("failed to write to stdin: %w", err)
	}

	// 等待輸出穩定：輸出連續 quiet 時間無新增即認為命令執行完成，
	// 比固定休眠更可靠（可覆蓋 go build 等慢命令），並有上限避免一直等待
	waitForOutputStable(session, 30*time.Second, 300*time.Millisecond)

	session.mu.Lock()
	output := session.OutputBuf.String()
	newOutput := output[session.consumed:]
	session.consumed = len(output)
	session.mu.Unlock()

	return newOutput, 0, nil
}

// waitForOutputStable 輪詢等待 session 輸出穩定：
// 在 maxWait 內，若輸出連續 quiet 時間無增長則返回；否則等到 maxWait 超時。
func waitForOutputStable(session *Session, maxWait, quiet time.Duration) {
	deadline := time.Now().Add(maxWait)
	lastLen := -1
	lastGrow := time.Now()
	for time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		session.mu.Lock()
		cur := session.OutputBuf.Len()
		session.mu.Unlock()
		if cur != lastLen {
			lastLen = cur
			lastGrow = time.Now()
		} else if lastLen >= 0 && time.Since(lastGrow) >= quiet {
			return
		}
	}
}

// shellSupportsNoProfile 探測解析到的 bash/sh 是否支援 GNU 長選項 --noprofile/--norc。
// busybox 等精簡 shell 不支援這些選項，直接傳參會報 "bash: bad option '--noprofile'"。
// 用 "exit 0" 作為探測命令：GNU bash 正常退出（0），不支援的 shell 報錯退出（非 0）。
func shellSupportsNoProfile(shellType string) bool {
	probe := exec.Command(shellType, "--noprofile", "--norc", "-c", "exit 0")
	return probe.Run() == nil
}

// startInteractiveShellProcess 啟動持久 shell 進程並返回 stdin, stdout, stderr pipes
func startInteractiveShellProcess(shellType, cwd string) (*exec.Cmd, io.WriteCloser, io.ReadCloser, io.ReadCloser, error) {
	var cmd *exec.Cmd

	switch strings.ToLower(shellType) {
	case "pwsh", "powershell":
		cmd = exec.Command("pwsh", "-NoExit", "-NoLogo", "-Command", "-")
	case "cmd":
		cmd = exec.Command("cmd.exe", "/K")
	default:
		// UNIX shell (bash, zsh, sh, fish, etc.)
		// 不要傳 -i 交互模式：stdout 是管道而非 TTY 時，交互模式會報
		// "cannot set terminal process group" / "write error: Bad file descriptor"。
		// 非交互模式同樣會從 stdin 逐行讀取命令，並在同一進程內維持會話狀態（cwd、變量等）。
		// 同時跳過啟動文件（如 ~/.bashrc），避免其輸出污染工具結果。
		switch shellType {
		case "bash", "sh":
			// 某些環境的 bash 可能是 busybox 等精簡實現，不支援 GNU 長選項
			// --noprofile/--norc（啟動時會報 "bash: bad option '--noprofile'"）。
			// 先探測解析到的 shell 是否支援這些選項，不支援則不加選項直接啟動。
			if shellSupportsNoProfile(shellType) {
				cmd = exec.Command(shellType, "--noprofile", "--norc")
			} else {
				cmd = exec.Command(shellType)
			}
		case "zsh":
			cmd = exec.Command(shellType, "-f")
		default:
			// fish, ksh, dash, tcsh, csh 等：非交互模式直接從 stdin 讀取
			cmd = exec.Command(shellType)
		}
	}

	if cwd != "" {
		cmd.Dir = cwd
	}

	// 設置 SysProcAttr 以隱藏窗口 (僅在 Windows 下)
	if runtime.GOOS == "windows" {
		if cmd.SysProcAttr == nil {
			cmd.SysProcAttr = &syscall.SysProcAttr{}
		}
		// CREATE_NO_WINDOW = 0x08000000
		cmd.SysProcAttr.CreationFlags = 0x08000000
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, nil, nil, err
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, nil, nil, nil, err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		stdin.Close()
		stdout.Close()
		return nil, nil, nil, nil, err
	}

	if err := cmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		stderr.Close()
		return nil, nil, nil, nil, err
	}

	return cmd, stdin, stdout, stderr, nil
}

// readSessionOutput 持續把 stdout/stderr 讀入 Session 的 OutputBuf（供 execSessionCommand 讀取），
// 並在進程結束時關閉 done。注意必須「邊讀邊寫」——若等進程結束才寫入，持久會話下永遠讀不到輸出。
func readSessionOutput(session *Session) {
	defer close(session.done)

	// 讀取 stdout
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := session.Stdout.Read(buf)
			if n > 0 {
				session.mu.Lock()
				session.OutputBuf.Write(buf[:n])
				session.mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()

	// 讀取 stderr
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := session.Stderr.Read(buf)
			if n > 0 {
				session.mu.Lock()
				session.OutputBuf.Write(buf[:n])
				session.mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()

	// 等待進程結束
	_ = session.Cmd.Wait()
}

// ShellInfo 描述一個可用 shell 及其調用方式
type ShellInfo struct {
	Name string   // shell 名稱（如 bash / zsh / pwsh / cmd）
	Path string   // 可執行文件絕對路徑
	Args []string // 執行命令時的前綴參數
}

// unixShells 候選的 Unix 相容 shell，優先順序排列
var unixShells = []struct {
	name string
	args []string
}{
	{"bash", []string{"-c"}},
	{"zsh", []string{"-c"}},
	{"ksh", []string{"-c"}},
	{"sh", []string{"-c"}},
	{"fish", []string{"-c"}},
	{"dash", []string{"-c"}},
	{"tcsh", []string{"-c"}},
	{"csh", []string{"-c"}},
}

// detectAvailableShells 探測系統中所有可用的 shell。
// Unix 相容 shell 優先；視窗下若完全未檢測到任何 Unix 類 sh 命令，則以 PowerShell / CMD 兜底。
func detectAvailableShells() []ShellInfo {
	var shells []ShellInfo
	for _, s := range unixShells {
		if p, err := exec.LookPath(s.name); err == nil {
			shells = append(shells, ShellInfo{Name: s.name, Path: p, Args: s.args})
		}
	}
	if runtime.GOOS == "windows" {
		// PowerShell 優先（pwsh 若可用則取之，否則退回 powershell）
		for _, psName := range []string{"pwsh", "powershell"} {
			if p, err := exec.LookPath(psName); err == nil {
				shells = append(shells, ShellInfo{Name: psName, Path: p, Args: []string{"-NoProfile", "-Command"}})
				break
			}
		}
		if p, err := exec.LookPath("cmd"); err == nil {
			shells = append(shells, ShellInfo{Name: "cmd", Path: p, Args: []string{"/d", "/c"}})
		}
	}
	return shells
}

// shellNames 返回所有可用 shell 的名稱列表，用於錯誤提示
func shellNames(shells []ShellInfo) string {
	names := make([]string, 0, len(shells))
	for _, s := range shells {
		names = append(names, s.Name)
	}
	return strings.Join(names, ", ")
}

// resolveShell 根據請求選擇 shell；請求為空時自動選用第一個（優先順序最高的）可用 shell
func resolveShell(available []ShellInfo, requested string) (*ShellInfo, error) {
	if requested == "" {
		if len(available) == 0 {
			return nil, fmt.Errorf("no shell detected on this system")
		}
		return &available[0], nil
	}
	req := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(requested), ".exe"))
	for i := range available {
		if available[i].Name == req {
			return &available[i], nil
		}
	}
	return nil, fmt.Errorf("shell %q is not available; available shells: %s", requested, shellNames(available))
}
