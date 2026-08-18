package main

import (
	"bytes"
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

// execSessionCommand 在 session 中執行命令
func execSessionCommand(session *Session, command string) (string, int32, error) {
	cmdWithNewline := command + "\n"
	_, err := session.Stdin.Write([]byte(cmdWithNewline))
	if err != nil {
		return "", 0, fmt.Errorf("failed to write to stdin: %w", err)
	}

	// 等待輸出穩定（短暫休眠）
	time.Sleep(300 * time.Millisecond)

	session.mu.Lock()
	output := session.OutputBuf.String()
	session.mu.Unlock()

	return output, 0, nil
}

// startInteractiveShellProcess 啟動交互式 shell 進程並返回 stdin, stdout, stderr pipes
func startInteractiveShellProcess(shellType, cwd string) (*exec.Cmd, io.WriteCloser, io.ReadCloser, io.ReadCloser, error) {
	var cmd *exec.Cmd

	switch strings.ToLower(shellType) {
	case "pwsh", "powershell":
		cmd = exec.Command("pwsh", "-NoExit", "-NoLogo", "-Command", "-")
	case "cmd":
		cmd = exec.Command("cmd.exe", "/K")
	default:
		// UNIX shell (bash, zsh, sh, etc.)
		// 使用互動式模式 (-i) 以維持狀態
		if shellType == "fish" {
			cmd = exec.Command(shellType, "-i")
		} else {
			// bash, zsh, sh, ksh, dash, tcsh, csh 等
			// 對於 bash/sh，使用 --rcfile /dev/null 避免讀取啟動文件
			if shellType == "bash" || shellType == "sh" {
				cmd = exec.Command(shellType, "--rcfile", "/dev/null", "-i")
			} else if shellType == "zsh" {
				cmd = exec.Command(shellType, "-f", "-i")
			} else {
				cmd = exec.Command(shellType, "-i")
			}
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

// readSessionOutput 讀取 shell 輸出並累積到 Session 的 OutputBuf
func readSessionOutput(session *Session) {
	defer close(session.done)

	var stdoutBuf, stderrBuf bytes.Buffer

	// 讀取 stdout
	go func() {
		io.Copy(&stdoutBuf, session.Stdout)
	}()

	// 讀取 stderr
	go func() {
		io.Copy(&stderrBuf, session.Stderr)
	}()

	// 等待進程結束
	_ = session.Cmd.Wait()

	// 累積輸出
	session.mu.Lock()
	session.OutputBuf.WriteString(stdoutBuf.String())
	session.OutputBuf.WriteString(stderrBuf.String())
	session.mu.Unlock()
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
