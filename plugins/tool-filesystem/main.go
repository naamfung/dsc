package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

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
	// 探測系統中所有可用的 shell
	detectedShells := detectAvailableShells()

	// 定義 shell 工具
	shellTool := &FileTool{
		name:        "shell",
		description: "Execute a shell command or script using an available shell (bash, zsh, ksh, sh, fish, dash, tcsh, csh; on Windows falls back to PowerShell/CMD)",
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
				}
			},
			"required": ["command"]
		}`),
		handler: func(ctx context.Context, args json.RawMessage) (string, error) {
			var params struct {
				Command string `json:"command"`
				Cwd     string `json:"cwd"`
				Shell   string `json:"shell"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("invalid arguments: %w", err)
			}
			if strings.TrimSpace(params.Command) == "" {
				return "", fmt.Errorf("command is required")
			}

			sh, err := resolveShell(detectedShells, params.Shell)
			if err != nil {
				return "", err
			}
			return runShell(*sh, params.Command, params.Cwd)
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

// runShell 在指定 shell 中執行命令，返回合併後的標準輸出/錯誤輸出及退出碼
func runShell(sh ShellInfo, command, cwd string) (string, error) {
	args := append(append([]string{}, sh.Args...), command)
	cmd := exec.Command(sh.Path, args...)
	if cwd != "" {
		cmd.Dir = cwd
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	var builder strings.Builder
	builder.WriteString(stdout.String())
	if stderr.Len() > 0 {
		builder.WriteString("\n[stderr]\n")
		builder.WriteString(stderr.String())
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			builder.WriteString(fmt.Sprintf("\n[exit_code: %d]\n", exitErr.ExitCode()))
			return builder.String(), nil
		}
		return builder.String(), fmt.Errorf("failed to start shell %s: %w", sh.Name, err)
	}
	builder.WriteString("\n[exit_code: 0]\n")
	return builder.String(), nil
}
