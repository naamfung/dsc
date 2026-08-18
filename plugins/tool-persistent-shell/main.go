package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
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

// CreateSession 創建一個新的 shell 會話
func (sm *SessionManager) CreateSession(ctx context.Context, req *proto.CreateSessionRequest) (*proto.CreateSessionResponse, error) {
	sessionID := fmt.Sprintf("session-%d", time.Now().UnixNano())

	// 確定 shell 類型和命令
	shellType := req.Shell
	if shellType == "" {
		shellType = detectDefaultShell()
	}

	cwd := req.Cwd
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			cwd = "."
		}
	}

	// 構造 shell 命令
	cmd, stdin, stdout, stderr, err := startShellProcess(shellType, cwd)
	if err != nil {
		return &proto.CreateSessionResponse{
			Status:  "error",
			Message: fmt.Sprintf("failed to start shell: %v", err),
		}, nil
	}

	session := &Session{
		SessionID: sessionID,
		ShellType: shellType,
		Cwd:       cwd,
		Cmd:       cmd,
		Stdin:     stdin,
		Stdout:    stdout,
		Stderr:    stderr,
		done:      make(chan struct{}),
	}

	sm.mu.Lock()
	sm.sessions[sessionID] = session
	sm.mu.Unlock()

	// 啟動讀取輸出的 goroutine
	go readSessionOutput(session)

	return &proto.CreateSessionResponse{
		SessionId: sessionID,
		Status:    "success",
		Message:   fmt.Sprintf("session %s created with shell %s in %s", sessionID, shellType, cwd),
	}, nil
}

// ExecSession 在指定的 shell 會話中執行命令
func (sm *SessionManager) ExecSession(ctx context.Context, req *proto.ExecSessionRequest) (*proto.ExecSessionResponse, error) {
	sm.mu.RLock()
	session, exists := sm.sessions[req.SessionId]
	sm.mu.RUnlock()

	if !exists {
		return &proto.ExecSessionResponse{
			Error: fmt.Sprintf("session %s not found", req.SessionId),
		}, nil
	}

	// 寫入命令到 stdin
	cmdWithNewline := req.Command + "\n"
	_, err := session.Stdin.Write([]byte(cmdWithNewline))
	if err != nil {
		return &proto.ExecSessionResponse{
			Error: fmt.Sprintf("failed to write to stdin: %v", err),
		}, nil
	}

	// 等待輸出穩定（短暫休眠）
	time.Sleep(300 * time.Millisecond)

	sm.mu.Lock()
	output := session.OutputBuf.String()
	sm.mu.Unlock()

	return &proto.ExecSessionResponse{
		Output:    output,
		ExitCode:  0, // 持久會話不會退出，返回 0
		Error:     "",
	}, nil
}

// CloseSession 關閉指定的 shell 會話
func (sm *SessionManager) CloseSession(ctx context.Context, req *proto.CloseSessionRequest) (*proto.CloseSessionResponse, error) {
	sm.mu.Lock()
	session, exists := sm.sessions[req.SessionId]
	if exists {
		delete(sm.sessions, req.SessionId)
	}
	sm.mu.Unlock()

	if !exists {
		return &proto.CloseSessionResponse{
			Success: false,
			Message: fmt.Sprintf("session %s not found", req.SessionId),
		}, nil
	}

	// 發送 exit 命令到 shell
	_, _ = session.Stdin.Write([]byte("exit\n"))

	// 等待進程結束
	_ = session.Cmd.Wait()

	if session.Stdin != nil {
		_ = session.Stdin.Close()
	}
	if session.Stdout != nil {
		_ = session.Stdout.Close()
	}
	if session.Stderr != nil {
		_ = session.Stderr.Close()
	}

	close(session.done)

	return &proto.CloseSessionResponse{
		Success: true,
		Message: fmt.Sprintf("session %s closed", req.SessionId),
	}, nil
}

// startShellProcess 啟動 shell 進程並返回 stdin, stdout, stderr pipes
func startShellProcess(shellType, cwd string) (*exec.Cmd, io.WriteCloser, io.ReadCloser, io.ReadCloser, error) {
	var cmd *exec.Cmd

	switch strings.ToLower(shellType) {
	case "pwsh", "powershell":
		cmd = exec.Command("pwsh", "-NoExit", "-NoLogo", "-Command", "-")
	case "cmd":
		cmd = exec.Command("cmd.exe", "/K")
	default:
		// 默認使用 bash 或 sh
		if runtime.GOOS == "windows" {
			// Windows 下默認使用 pwsh
			cmd = exec.Command("pwsh", "-NoExit", "-NoLogo", "-Command", "-")
		} else {
			cmd = exec.Command("bash", "--rcfile", "/dev/null", "-i")
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

// detectDefaultShell 探測默認 shell
func detectDefaultShell() string {
	if runtime.GOOS == "windows" {
		if _, err := exec.LookPath("pwsh"); err == nil {
			return "pwsh"
		}
		if _, err := exec.LookPath("powershell"); err == nil {
			return "powershell"
		}
		return "cmd"
	}
	if _, err := exec.LookPath("bash"); err == nil {
		return "bash"
	}
	if _, err := exec.LookPath("sh"); err == nil {
		return "sh"
	}
	return "bash"
}

// PersistentTerminalServiceServer 持久終端服務服務端實現
type PersistentTerminalServiceServer struct {
	proto.UnimplementedPersistentTerminalServiceServer
}

func (s *PersistentTerminalServiceServer) CreateSession(ctx context.Context, req *proto.CreateSessionRequest) (*proto.CreateSessionResponse, error) {
	return globalSessionManager.CreateSession(ctx, req)
}

func (s *PersistentTerminalServiceServer) ExecSession(ctx context.Context, req *proto.ExecSessionRequest) (*proto.ExecSessionResponse, error) {
	return globalSessionManager.ExecSession(ctx, req)
}

func (s *PersistentTerminalServiceServer) CloseSession(ctx context.Context, req *proto.CloseSessionRequest) (*proto.CloseSessionResponse, error) {
	return globalSessionManager.CloseSession(ctx, req)
}

// MetadataServer 元數據服務服務端實現
type MetadataServer struct {
	metadata.UnimplementedPluginMetadataServer
}

func (m *MetadataServer) GetInfo(ctx context.Context, _ *metadata.Empty) (*metadata.PluginInfo, error) {
	return &metadata.PluginInfo{
		Type:       "tool",
		Name:       "persistent-shell",
		Version:    "1.0.0",
		ApiVersion: "1.0",
	}, nil
}

func main() {
	// 創建持久終端服務服務端
	terminalServer := &PersistentTerminalServiceServer{}

	// 創建元數據服務服務端
	metadataServer := &MetadataServer{}

	// 啟動插件服務
	goplugin.Serve(&goplugin.ServeConfig{
		HandshakeConfig: plugin.Handshake,
		Plugins: map[string]goplugin.Plugin{
			"persistent_terminal": &PersistentTerminalGRPCPlugin{
				TerminalImpl:   terminalServer,
				MetadataImpl:   metadataServer,
			},
		},
		GRPCServer: goplugin.DefaultGRPCServer,
	})
}
