package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"dsc/plugin"
	"dsc/proto"
	"dsc/proto/metadata"
	goplugin "github.com/hashicorp/go-plugin"
)

// StrReplaceEditorTool 文件編輯工具實現
type StrReplaceEditorTool struct {
	name        string
	description string
	schema      json.RawMessage
	server      *ToolServiceServer // 引用 ToolServiceServer 以訪問 observations
}

func (t *StrReplaceEditorTool) Name() string {
	return t.name
}

func (t *StrReplaceEditorTool) Description() string {
	return t.description
}

func (t *StrReplaceEditorTool) ParametersSchema() json.RawMessage {
	return t.schema
}

func (t *StrReplaceEditorTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	return strReplaceEditorHandler(ctx, t.server, args)
}

// ToolServiceServer 工具服務服務端實現
type ToolServiceServer struct {
	proto.UnimplementedToolServiceServer
	tools        []*StrReplaceEditorTool
	observations map[string]*proto.FsObservation
	mu           sync.RWMutex
}

func (s *ToolServiceServer) getObservation(filePath string) (*proto.FsObservation, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obs, found := s.observations[filePath]
	return obs, found
}

func (s *ToolServiceServer) updateObservation(filePath string, state string, version string, lastContent string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.observations == nil {
		s.observations = make(map[string]*proto.FsObservation)
	}
	s.observations[filePath] = &proto.FsObservation{
		State:       state,
		Version:     version,
		LastContent: lastContent,
	}
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
		Name:       "str_replace_editor",
		Version:    "1.0.0",
		ApiVersion: "1.0",
	}, nil
}

// withinBase 判斷 real 路徑是否在 base 目錄（含 base 自身）之內
func withinBase(real, realBase string) bool {
	return strings.HasPrefix(real, realBase+string(os.PathSeparator)) || real == realBase
}

// resolveExistingAncestor 從 dir 開始向上逐級查找第一個存在的路徑並解析符號鏈接；
// 返回其真實路徑。所有層級都不存在時返回 false。
func resolveExistingAncestor(dir string) (string, bool) {
	for {
		real, err := filepath.EvalSymlinks(dir)
		if err == nil {
			return real, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// safePath 檢查並返回安全的路徑（防止路徑遍歷和符號鏈接繞過）。
// base 若不存在會先創建；目標路徑或其父目錄不存在時也能正常解析（中間目錄可由調用方自行創建）。
func safePath(base, reqPath string) (string, error) {
	absBase, err := filepath.Abs(base)
	if err != nil {
		return "", err
	}
	// 工作目錄（base）可能尚未創建（首次使用時），先確保它存在，
	// 否則 EvalSymlinks 會報 “The system cannot find the file specified”
	if err := os.MkdirAll(absBase, 0755); err != nil {
		return "", err
	}
	realBase, err := filepath.EvalSymlinks(absBase)
	if err != nil {
		return "", err
	}

	// 構建絕對路徑並清理（去除 . .. 等）
	absReq, err := filepath.Abs(filepath.Join(realBase, reqPath))
	if err != nil {
		return "", err
	}
	absReq = filepath.Clean(absReq)

	// 詞法前綴檢查：確保在 base 目錄內
	if !withinBase(absReq, realBase) {
		return "", os.ErrPermission
	}

	// 目標本身若已存在，解析其真實路徑並檢查仍在 base 內（防符號鏈接逃逸）
	if _, statErr := os.Lstat(absReq); statErr == nil {
		realReq, err := filepath.EvalSymlinks(absReq)
		if err != nil {
			return "", err
		}
		if !withinBase(realReq, realBase) {
			return "", os.ErrPermission
		}
		return realReq, nil
	}

	// 目標不存在：解析其最深的已存在祖先，確保真實路徑仍在 base 內
	realAncestor, ok := resolveExistingAncestor(filepath.Dir(absReq))
	if !ok {
		return "", os.ErrPermission
	}
	if !withinBase(realAncestor, realBase) {
		return "", os.ErrPermission
	}
	return absReq, nil
}

// normalizeWorkspacePath 剝離模型按工具描述傳入的 /workspace 前綴，
// 使 /workspace/test/fib.go 映射到 workspace 根目錄下的 test/fib.go
func normalizeWorkspacePath(p string) string {
	p = strings.TrimSpace(p)
	for _, prefix := range []string{"/workspace", `\workspace`} {
		if strings.HasPrefix(p, prefix) {
			return strings.TrimLeft(strings.TrimPrefix(p, prefix), `/\`)
		}
	}
	return p
}

type strReplaceEditorArgs struct {
	Command    string `json:"command"`
	Path       string `json:"path"`
	FileText   string `json:"file_text"`
	OldStr     string `json:"old_str"`
	NewStr     string `json:"new_str"`
	InsertLine int    `json:"insert_line"`
}

// computeHash 計算字符串的 sha256 hash
func computeHash(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])
}

func strReplaceEditorHandler(ctx context.Context, server *ToolServiceServer, argsJSON json.RawMessage) (string, error) {
	var args strReplaceEditorArgs
	if err := json.Unmarshal(argsJSON, &args); err != nil {
		return "", err
	}

	// 使用安全路徑檢查，默認 workspace root 為當前目錄
	workspaceRoot := "./workspace"

	// 模型按工具描述會傳入形如 /workspace/test/fib.go 的絕對路徑，
	// 需剝離 /workspace 前綴後再與 workspaceRoot 拼接，避免變成 workspace/workspace/...
	reqPath, err := safePath(workspaceRoot, normalizeWorkspacePath(args.Path))
	if err != nil {
		return "", err
	}

	switch args.Command {
	case "view":
		content, err := os.ReadFile(reqPath)
		if err != nil {
			return "", err
		}
		contentStr := string(content)
		version := computeHash(contentStr)
		// 更新觀測狀態
		server.updateObservation(reqPath, "present", version, contentStr)
		return contentStr, nil

	case "create":
		if args.FileText == "" {
			return "", fmt.Errorf("file_text is required for create command")
		}
		dir := filepath.Dir(reqPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", err
		}
		if err := os.WriteFile(reqPath, []byte(args.FileText), 0644); err != nil {
			return "", err
		}
		version := computeHash(args.FileText)
		// 更新觀測狀態
		server.updateObservation(reqPath, "present", version, args.FileText)
		return "File created successfully.", nil

	case "str_replace":
		if args.OldStr == "" {
			return "", fmt.Errorf("old_str is required for str_replace command")
		}
		if args.NewStr == "" {
			return "", fmt.Errorf("new_str is required for str_replace command")
		}

		// 檢查觀測狀態
		obs, found := server.getObservation(reqPath)
		if !found || obs.State == "unseen" || obs.State == "absent" {
			return "", fmt.Errorf("str_replace failed: file has not been observed (viewed). Please use 'view' command first.")
		}

		content, err := os.ReadFile(reqPath)
		if err != nil {
			return "", err
		}
		contentStr := string(content)

		// 驗證版本/內容是否匹配
		if obs.LastContent != "" && obs.LastContent != contentStr {
			return "", fmt.Errorf("str_replace failed: file content has changed since last observation. Please use 'view' to get the latest content.")
		}

		if !strings.Contains(contentStr, args.OldStr) {
			return "", fmt.Errorf("str_replace failed: old_str not found in file. File content:\n%s", contentStr)
		}
		newContentStr := strings.Replace(contentStr, args.OldStr, args.NewStr, 1)
		if err := os.WriteFile(reqPath, []byte(newContentStr), 0644); err != nil {
			return "", err
		}

		// 更新觀測狀態
		newVersion := computeHash(newContentStr)
		server.updateObservation(reqPath, "present", newVersion, newContentStr)

		return "File replaced successfully.", nil

	case "insert":
		if args.NewStr == "" {
			return "", fmt.Errorf("new_str is required for insert command")
		}
		if args.InsertLine <= 0 {
			return "", fmt.Errorf("insert_line must be a positive integer for insert command")
		}

		// 檢查觀測狀態
		obs, found := server.getObservation(reqPath)
		if !found || obs.State == "unseen" || obs.State == "absent" {
			return "", fmt.Errorf("insert failed: file has not been observed (viewed). Please use 'view' command first.")
		}

		content, err := os.ReadFile(reqPath)
		if err != nil {
			return "", err
		}
		contentStr := string(content)

		// 驗證版本/內容是否匹配
		if obs.LastContent != "" && obs.LastContent != contentStr {
			return "", fmt.Errorf("insert failed: file content has changed since last observation. Please use 'view' to get the latest content.")
		}

		lines := strings.Split(contentStr, "\n")
		// insert_line is 1-based
		// If insert_line is greater than len(lines), append to the end
		var newLines []string
		if args.InsertLine > len(lines) {
			newLines = append(lines, args.NewStr)
		} else {
			// Insert before the line at index args.InsertLine-1
			before := lines[:args.InsertLine-1]
			after := lines[args.InsertLine-1:]
			newLines = append(before, append([]string{args.NewStr}, after...)...)
		}
		newContent := strings.Join(newLines, "\n")
		if err := os.WriteFile(reqPath, []byte(newContent), 0644); err != nil {
			return "", err
		}

		// 更新觀測狀態
		newVersion := computeHash(newContent)
		server.updateObservation(reqPath, "present", newVersion, newContent)

		return "File inserted successfully.", nil

	default:
		return "", fmt.Errorf("unsupported command: %s", args.Command)
	}
}

func main() {
	// 創建工具服務服務端
	toolServer := &ToolServiceServer{
		observations: make(map[string]*proto.FsObservation),
	}

	// 定義 str_replace_editor 工具
	strReplaceEditorTool := &StrReplaceEditorTool{
		name:        "str_replace_editor",
		description: "Custom editor tool for viewing, creating, and editing files. Supports commands: view, create, str_replace, insert.",
		schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"command": {
					"type": "string",
					"enum": ["view", "create", "str_replace", "insert"],
					"description": "The commands to run. Allowed options are: view, create, str_replace, insert."
				},
				"path": {
					"type": "string",
					"description": "Absolute path to the file, e.g. /workspace/file.py."
				},
				"file_text": {
					"type": "string",
					"description": "Required for 'create' command. The content of the file to be created."
				},
				"old_str": {
					"type": "string",
					"description": "Required for 'str_replace' command. The string in the file to replace."
				},
				"new_str": {
					"type": "string",
					"description": "Required for 'str_replace' and 'insert' commands. The new string to replace with or insert."
				},
				"insert_line": {
					"type": "integer",
					"description": "Required for 'insert' command. The 1-based line number where the new_str should be inserted."
				}
			},
			"required": ["command", "path"]
		}`),
		server: toolServer,
	}

	toolServer.tools = []*StrReplaceEditorTool{strReplaceEditorTool}

	// 創建元數據服務服務端
	metadataServer := &MetadataServer{}

	// 啟動插件服務
	goplugin.Serve(&goplugin.ServeConfig{
		HandshakeConfig: plugin.Handshake,
		Plugins: map[string]goplugin.Plugin{
			"tool": &ToolMetadataGRPCPlugin{
				ToolImpl:     toolServer,
				MetadataImpl: metadataServer,
			},
		},
		GRPCServer: goplugin.DefaultGRPCServer,
	})
}
