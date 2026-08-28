// tool-ssh：SSH 远程命令执行终端插件。提供 ssh_connect / ssh_exec / ssh_list /
// ssh_close 四类持久会话能力（参考 GhostClaw 的 SSH 工具集）。登录支持密码或
// 私钥；会话按 id 缓存复用，方便模型连续在远程主机上执行命令。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"dsc-sdk"

	"golang.org/x/crypto/ssh"
)

// SSHSessionManager 管理持久化 SSH 连接。
type SSHSessionManager struct {
	mu       sync.Mutex
	sessions map[string]*ssh.Client
}

var globalSSH = &SSHSessionManager{sessions: make(map[string]*ssh.Client)}

// connect 建立并缓存一个 SSH 连接，返回会话 id。支持密码或私钥认证。
func (m *SSHSessionManager) connect(host string, port int, username, password, privateKeyPath string) (string, error) {
	var auth []ssh.AuthMethod
	if password != "" {
		auth = append(auth, ssh.Password(password))
	}
	if privateKeyPath != "" {
		pem, err := os.ReadFile(privateKeyPath)
		if err != nil {
			return "", fmt.Errorf("read private key: %w", err)
		}
		signer, err := ssh.ParsePrivateKey(pem)
		if err != nil {
			return "", fmt.Errorf("parse private key: %w", err)
		}
		auth = append(auth, ssh.PublicKeys(signer))
	}
	if len(auth) == 0 {
		return "", fmt.Errorf("no auth method (password or private_key_path required)")
	}
	cfg := &ssh.ClientConfig{
		User:            username,
		Auth:            auth,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // 演示便利；生产建议换为 known_hosts 校验
		Timeout:         10 * time.Second,
	}
	addr := fmt.Sprintf("%s:%d", host, port)
	client, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return "", fmt.Errorf("dial %s: %w", addr, err)
	}

	id := fmt.Sprintf("ssh-%d", time.Now().UnixNano())
	m.mu.Lock()
	m.sessions[id] = client
	m.mu.Unlock()
	return id, nil
}

// exec 在指定会话上执行命令，返回合并输出与退出码。
func (m *SSHSessionManager) exec(id, command string) (string, int, error) {
	m.mu.Lock()
	c := m.sessions[id]
	m.mu.Unlock()
	if c == nil {
		return "", 0, fmt.Errorf("session %s not found", id)
	}
	sess, err := c.NewSession()
	if err != nil {
		return "", 0, err
	}
	defer sess.Close()
	out, err := sess.CombinedOutput(command)
	code := 0
	if err != nil {
		if ee, ok := err.(*ssh.ExitError); ok {
			code = ee.ExitStatus()
		} else {
			code = 1
		}
	}
	return string(out), code, err
}

func (m *SSHSessionManager) list() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	return ids
}

func (m *SSHSessionManager) close(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.sessions[id]; ok {
		_ = c.Close()
		delete(m.sessions, id)
		return true
	}
	return false
}

// SSHConnectResult ssh_connect 的结果。
type SSHConnectResult struct {
	Success   bool   `json:"success"`
	SessionID string `json:"session_id,omitempty"`
	Error     string `json:"error,omitempty"`
}

// SSHExecResult ssh_exec 的结果。
type SSHExecResult struct {
	Success  bool   `json:"success"`
	Output   string `json:"output"`
	ExitCode int    `json:"exit_code,omitempty"`
	Error    string `json:"error,omitempty"`
}

// SSHListResult ssh_list 的结果。
type SSHListResult struct {
	Success  bool     `json:"success"`
	Sessions []string `json:"sessions"`
}

// SSHCloseResult ssh_close 的结果。
type SSHCloseResult struct {
	Success bool   `json:"success"`
	Closed  bool   `json:"closed"`
	Error   string `json:"error,omitempty"`
}

func main() {
	// ssh_connect
	connectSchema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"host": {"type": "string", "description": "SSH host (IP or hostname)"},
			"port": {"type": "integer", "description": "SSH port, default 22"},
			"username": {"type": "string", "description": "SSH username"},
			"password": {"type": "string", "description": "Password for password auth (optional)"},
			"private_key_path": {"type": "string", "description": "Path to a PEM private key for key auth (optional)"}
		},
		"required": ["host", "username"]
	}`)
	connectHandler := func(ctx context.Context, args json.RawMessage) (string, error) {
		var p struct {
			Host           string `json:"host"`
			Port           int    `json:"port"`
			Username       string `json:"username"`
			Password       string `json:"password"`
			PrivateKeyPath string `json:"private_key_path"`
		}
		if err := json.Unmarshal(args, &p); err != nil {
			return "", fmt.Errorf("invalid arguments: %w", err)
		}
		if p.Host == "" || p.Username == "" {
			return "", fmt.Errorf("host and username are required")
		}
		if p.Port == 0 {
			p.Port = 22
		}
		id, err := globalSSH.connect(p.Host, p.Port, p.Username, p.Password, p.PrivateKeyPath)
		res := SSHConnectResult{Success: err == nil, SessionID: id}
		if err != nil {
			res.Error = err.Error()
		}
		b, _ := json.Marshal(res)
		return string(b), nil
	}

	// ssh_exec
	execSchema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"session_id": {"type": "string", "description": "SSH session ID from ssh_connect"},
			"command": {"type": "string", "description": "Command to run on the remote host"}
		},
		"required": ["session_id", "command"]
	}`)
	execHandler := func(ctx context.Context, args json.RawMessage) (string, error) {
		var p struct {
			SessionID string `json:"session_id"`
			Command   string `json:"command"`
		}
		if err := json.Unmarshal(args, &p); err != nil {
			return "", fmt.Errorf("invalid arguments: %w", err)
		}
		if p.SessionID == "" || p.Command == "" {
			return "", fmt.Errorf("session_id and command are required")
		}
		out, code, err := globalSSH.exec(p.SessionID, p.Command)
		res := SSHExecResult{Success: err == nil, Output: out, ExitCode: code}
		if err != nil {
			res.Error = err.Error()
		}
		b, _ := json.Marshal(res)
		return string(b), nil
	}

	// ssh_list
	listSchema := json.RawMessage(`{
		"type": "object",
		"properties": {}
	}`)
	listHandler := func(ctx context.Context, args json.RawMessage) (string, error) {
		b, _ := json.Marshal(SSHListResult{Success: true, Sessions: globalSSH.list()})
		return string(b), nil
	}

	// ssh_close
	closeSchema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"session_id": {"type": "string", "description": "SSH session ID to close"}
		},
		"required": ["session_id"]
	}`)
	closeHandler := func(ctx context.Context, args json.RawMessage) (string, error) {
		var p struct {
			SessionID string `json:"session_id"`
		}
		if err := json.Unmarshal(args, &p); err != nil {
			return "", fmt.Errorf("invalid arguments: %w", err)
		}
		res := SSHCloseResult{Success: true, Closed: globalSSH.close(p.SessionID)}
		b, _ := json.Marshal(res)
		return string(b), nil
	}

	sdk := dsc.New(dsc.Config{Name: "ssh", Version: "1.0.0", Type: dsc.TypeTool})
	sdk.Tool(dsc.Tool{Name: "ssh_connect", Description: "Connect to a remote host over SSH and return a persistent session ID for subsequent commands", Schema: connectSchema, Handler: connectHandler})
	sdk.Tool(dsc.Tool{Name: "ssh_exec", Description: "Run a command on an SSH session and return its combined output and exit code", Schema: execSchema, Handler: execHandler})
	sdk.Tool(dsc.Tool{Name: "ssh_list", Description: "List active SSH session IDs", Schema: listSchema, Handler: listHandler})
	sdk.Tool(dsc.Tool{Name: "ssh_close", Description: "Close an SSH session by ID", Schema: closeSchema, Handler: closeHandler})
	sdk.Serve()
}
